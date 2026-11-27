// Package generate turns the contract schemas into the other models of the
// contract: Go types, TypeScript types, zod schemas, the field reference and
// the explorer's list of fields (A13, docs/architecture.md section 7.3). The
// schema is the one source; every file made here is marked and is never
// edited by hand.
package generate

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// Kinds of a type in the tree.
const (
	kString  = "string"
	kInteger = "integer"
	kNumber  = "number"
	kBool    = "boolean"
	kArray   = "array"
	kObject  = "object"
	kEnum    = "enum"
	kConst   = "const"
	kRef     = "ref"
	kAny     = "any"
	kMap     = "map"
)

// typ is one schema node, reduced to what the generators write.
type typ struct {
	Kind     string
	Ref      string // the declaration a kRef names
	Elem     *typ
	Fields   []field
	Enum     []string
	Const    string
	Pattern  string
	MinLen   *float64
	Min, Max *float64
	MinItems *float64
	MaxItems *float64
	Desc     string
	Nullable bool // the schema takes the type or null
}

type field struct {
	Key        string
	T          *typ
	Required   bool
	HasDefault bool
	Default    any
	Since      string
	Desc       string
	// Decides is the one decision the field drives, from x-decides, and "" for
	// a field that is display only (arch.ports.19, A46).
	Decides string
}

// decl is a named type: a definition of the common schema, the root of an
// entity or a document, or an object or enum written inline in a field.
type decl struct {
	Name  string
	File  string // the schema file the declaration comes from
	Title string
	T     *typ
}

// tree is every declaration of the schemas, in schema-file order.
type tree struct {
	Decls []*decl
	index map[string]*decl
}

const commonFile = "common.schema.json"

func stem(file string) string { return strings.TrimSuffix(file, ".schema.json") }

// rootName is the name of a schema file's root declaration.
func rootName(file string) string {
	s := stem(file)
	switch {
	case strings.HasPrefix(s, "document-"):
		return camel(strings.TrimPrefix(s, "document-")) + "Document"
	case strings.HasPrefix(s, "entity-"):
		return camel(strings.TrimPrefix(s, "entity-"))
	}
	return camel(s)
}

// camel writes a key, a slug or a value as UpperCamelCase.
func camel(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			if up {
				b.WriteString(strings.ToUpper(string(r)))
			} else {
				b.WriteRune(r)
			}
			up = false
		default:
			up = true
		}
	}
	return b.String()
}

// build reads every schema into the tree. files holds each schema parsed.
func build(files map[string]*ordered.Map) (*tree, error) {
	t := &tree{index: map[string]*decl{}}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	var errs []error
	for _, n := range names {
		if err := t.file(n, files[n]); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return t, t.check()
}

func (t *tree) file(name string, root *ordered.Map) error {
	if name == commonFile {
		defs := root.Sub("$defs")
		if defs == nil {
			return fmt.Errorf("%s has no $defs", name)
		}
		for _, k := range defs.Keys() {
			d := defs.Sub(k)
			if d == nil {
				return fmt.Errorf("%s: $defs/%s is not a schema", name, k)
			}
			if err := t.declare(name, camel(k), d.Str("title"), d); err != nil {
				return err
			}
		}
		return nil
	}
	return t.declare(name, rootName(name), root.Str("title"), root)
}

// declare adds one named type read from node.
func (t *tree) declare(file, name, title string, node *ordered.Map) error {
	if t.index[name] != nil {
		return fmt.Errorf("%s: the name %s is already taken by %s", file, name, t.index[name].File)
	}
	d := &decl{Name: name, File: file, Title: title}
	t.index[name] = d
	t.Decls = append(t.Decls, d)
	ty, err := t.typeOf(file, name, node)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", file, name, err)
	}
	d.T = ty
	return nil
}

// refName turns a $ref into the name of the declaration it points at.
func refName(file, ref string) (string, error) {
	base, frag, has := strings.Cut(ref, "#")
	if base == "" {
		base = file
	}
	switch {
	case !has && base != commonFile:
		return rootName(base), nil
	case has && base == commonFile && strings.HasPrefix(frag, "/$defs/"):
		return camel(strings.TrimPrefix(frag, "/$defs/")), nil
	}
	return "", fmt.Errorf("the reference %q is not one this generator reads", ref)
}

// typeOf reads a schema node. Objects and enums written inline become
// declarations named from where they sit, so every generator sees a name.
func (t *tree) typeOf(file, at string, n *ordered.Map) (*typ, error) {
	if br := n.List("anyOf"); br != nil {
		return t.nullable(file, at, n, br)
	}
	if ref := n.Str("$ref"); ref != "" {
		name, err := refName(file, ref)
		if err != nil {
			return nil, err
		}
		return &typ{Kind: kRef, Ref: name, Desc: n.Str("description")}, nil
	}
	if _, ok := n.Get("enum"); ok {
		return &typ{Kind: kEnum, Enum: ordered.Strings(n.List("enum")), Desc: n.Str("description")}, nil
	}
	_, isConst := n.Get("const")
	if _, typed := n.Get("type"); !typed && !isConst {
		// no type and no const: any JSON value
		return &typ{Kind: kAny, Desc: n.Str("description")}, nil
	}
	ty := &typ{Kind: n.Str("type"), Desc: n.Str("description"), Pattern: n.Str("pattern")}
	if c, ok := n.Get("const"); ok {
		s, isStr := c.(string)
		if !isStr {
			return nil, errors.New("a const that is not a string")
		}
		ty.Kind, ty.Const = kConst, s
	}
	ty.MinLen, ty.Min, ty.Max = num(n, "minLength"), num(n, "minimum"), num(n, "maximum")
	ty.MinItems, ty.MaxItems = num(n, "minItems"), num(n, "maxItems")
	switch ty.Kind {
	case kString, kInteger, kNumber, kBool, kConst:
	case kArray:
		items := n.Sub("items")
		if items == nil {
			return nil, errors.New("an array with no items")
		}
		el, err := t.typeOf(file, at, items)
		if err != nil {
			return nil, err
		}
		ty.Elem = t.hoist(file, at, items, el)
	case kObject:
		return t.object(file, at, n, ty)
	default:
		return nil, fmt.Errorf("the type %q is not one this generator reads", ty.Kind)
	}
	return ty, nil
}

func num(n *ordered.Map, k string) *float64 {
	var f float64
	switch v := n.Val(k).(type) {
	case int:
		f = float64(v)
	case int64:
		f = float64(v)
	case float64:
		f = v
	default:
		return nil
	}
	return &f
}

func (t *tree) object(file, at string, n *ordered.Map, ty *typ) (*typ, error) {
	if n.Sub("properties") == nil {
		if _, closed := n.Get("additionalProperties"); closed {
			return nil, errors.New("an object with no properties that is closed")
		}
		ty.Kind = kMap
		return ty, nil
	}
	if ap, ok := n.Get("additionalProperties"); !ok || ap != false {
		return nil, errors.New("an object that is not closed with additionalProperties: false")
	}
	props := n.Sub("properties")
	if props == nil {
		return nil, errors.New("an object with no properties")
	}
	req := ordered.Strings(n.List("required"))
	for _, k := range props.Keys() {
		p := props.Sub(k)
		if p == nil {
			return nil, fmt.Errorf("the property %s is not a schema", k)
		}
		ft, err := t.typeOf(file, at+camel(k), p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		decides, err := decisionOf(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		f := field{Key: k, T: t.hoist(file, at+camel(k), p, ft), Required: slices.Contains(req, k), Since: p.Str("x-introduced"), Desc: p.Str("description"), Decides: decides}
		if d, has := p.Get("default"); has {
			f.HasDefault, f.Default = true, ordered.Plain(d)
		}
		ty.Fields = append(ty.Fields, f)
	}
	return ty, nil
}

// decisionOf is the one decision a property drives, from its x-decides: ""
// when it has none, so the field is display only. An x-decides that is not
// words is refused, so a mark can never be there and say nothing.
func decisionOf(p *ordered.Map) (string, error) {
	v, has := p.Get("x-decides")
	if !has {
		return "", nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", errors.New("x-decides must name, in words, the one decision the field drives")
	}
	return s, nil
}

// hoist makes a declaration of an inline object or enum, and returns the
// reference to it; any other type stays where it is.
func (t *tree) hoist(file, name string, n *ordered.Map, ty *typ) *typ {
	if ty.Kind != kObject && ty.Kind != kEnum {
		return ty
	}
	if t.index[name] != nil {
		// a second hoist under one name is reported by check
		return &typ{Kind: kRef, Ref: name}
	}
	d := &decl{Name: name, File: file, Title: n.Str("title"), T: ty}
	t.index[name] = d
	t.Decls = append(t.Decls, d)
	return &typ{Kind: kRef, Ref: name}
}

// check holds the tree to what the generators rely on: every reference
// resolves, and no file's declarations need another's in a circle.
func (t *tree) check() error {
	var errs []error
	for _, d := range t.Decls {
		for _, r := range refsOf(d.T) {
			if t.index[r] == nil {
				errs = append(errs, fmt.Errorf("%s: %s refers to %s, which no schema declares", d.File, d.Name, r))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if bad := t.defaultProblems(); len(bad) > 0 {
		for _, b := range bad {
			errs = append(errs, errors.New(b))
		}
		return errors.Join(errs...)
	}
	return t.cycles()
}

// refsOf lists the declarations a type names directly.
func refsOf(ty *typ) []string {
	var out []string
	var walk func(*typ)
	walk = func(x *typ) {
		if x == nil {
			return
		}
		if x.Kind == kRef {
			out = append(out, x.Ref)
		}
		walk(x.Elem)
		for _, f := range x.Fields {
			walk(f.T)
		}
	}
	walk(ty)
	return out
}

// cycles refuses a circle of references: zod needs the inner schema first,
// and the schemas have none.
func (t *tree) cycles() error {
	state := map[string]int{}
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("the schemas refer to one another in a circle: %s", strings.Join(append(path, name), " -> "))
		case 2:
			return nil
		}
		state[name] = 1
		for _, r := range refsOf(t.index[name].T) {
			if err := visit(r, append(path, name)); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	for _, d := range t.Decls {
		if err := visit(d.Name, nil); err != nil {
			return err
		}
	}
	return nil
}

// nullable reads an anyOf of one schema and null: the type, taking null too.
// Any other anyOf is a shape the generators do not write.
func (t *tree) nullable(file, at string, n *ordered.Map, branches []any) (*typ, error) {
	var inner *ordered.Map
	nulls := 0
	for _, b := range branches {
		m, ok := b.(*ordered.Map)
		if !ok {
			return nil, errors.New("an anyOf branch that is not a schema")
		}
		if m.Str("type") == "null" && m.Sub("properties") == nil {
			nulls++
			continue
		}
		if inner != nil {
			return nil, errors.New("an anyOf of more than one schema besides null")
		}
		inner = m
	}
	if nulls != 1 || inner == nil {
		return nil, errors.New("an anyOf that is not one schema or null")
	}
	ty, err := t.typeOf(file, at, inner)
	if err != nil {
		return nil, err
	}
	ty = t.hoist(file, at, inner, ty)
	c := *ty
	c.Nullable = true
	if d := n.Str("description"); d != "" {
		c.Desc = d
	}
	return &c, nil
}
