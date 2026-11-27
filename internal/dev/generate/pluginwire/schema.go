// Package pluginwire makes the Go types of plugin contract 1 from its two
// schemas, contract/plugin/v1/wire.schema.json and manifest.schema.json.
// The types and each type's ToCanon and FromCanon
// are written into internal/model/plugin, which imports canon alone, so the
// message a plugin sends and the engine reads is a typed value and never a
// map.
//
// It is a second, small generator beside internal/dev/generate's, which is
// specialised to the contract's entity and document schemas and refuses the
// forms these two use: a const that is a number, $ref to $defs, allOf,
// oneOf and an open value. This one reads exactly the keywords the two schemas
// use and fails on any other, so a keyword added to a schema is a failure
// here until the generator reads it, never a silent gap in the types.
//
// What the types check on reading is what the schema says and a type can hold:
// each member's type, a closed object's members, the required members, an
// enum's values, a pattern, and the least and most of a string, a number and a
// list. What a schema cannot say (order, size in bytes, legal paths, command
// words) is internal/core/wire's, which reads through these types.
package pluginwire

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// The folder of the two schemas, from the repository root, and the folder the
// Go written from them goes to.
const (
	SchemaDir = "contract/plugin/v1/"
	GoDir     = "internal/model/plugin/"
)

// Files are the schemas read, in the order their types are made: the wire's
// first, so a manifest enum that lists what a wire enum lists is that type.
func Files() []string { return []string{"wire.schema.json", "manifest.schema.json"} }

// kind is what a schema node is in Go.
type kind int

const (
	kString kind = iota
	kInt
	kBool
	kEnum
	kStruct
	kList
	kAny
	kObject
	kUnion
)

// typ is one Go type, as a schema node gives it.
type typ struct {
	kind kind
	name string // an enum's, a struct's or a union's Go name
	doc  string

	// A string.
	minLen, maxLen int // maxLen -1: none
	pattern        string
	also           []string // patterns a $ref's own pattern adds to the def's

	// A whole number.
	lo, hi int64

	// A list.
	elem             *typ
	minItems, maxIts int // maxIts -1: none
	unique           bool

	// An enum.
	values []string

	// A struct.
	fields     []*field
	closed     bool
	exactlyOne [][]string
	minProps   int
	whens      []when

	// A union: its alternatives, and the member they differ by, when each has
	// a constant one.
	alts []*typ
	disc string
	// altFields is the field name each alternative has in the union.
	altFields []string
}

// field is one member of a struct.
type field struct {
	json     string
	name     string
	t        *typ
	required bool
	// constant is the Go expression of a member that is a constant; the
	// struct holds no field for it, and writes and reads it.
	constant string
	doc      string
}

// when is a rule of if/then: when member prop is the string is, the members
// require are required.
type when struct {
	prop, is string
	require  []string
}

// handled is every keyword the generator reads. A node with any other is
// refused.
func handled() []string {
	return []string{
		"$schema", "$id", "$defs", "$ref", "type", "minLength", "maxLength", "pattern", "enum",
		"additionalProperties", "oneOf", "required", "properties", "const", "maxItems", "items",
		"minimum", "maximum", "if", "then", "allOf", "minItems", "title", "description",
		"uniqueItems", "minProperties",
	}
}

// names are the Go names of what a schema names no def for: an inline object, an
// inline enum and a union, each by its place (the def's key, then the
// members down to it, "[]" for the items of a list and oneOfN for an
// alternative). A place with no name here fails the generator, so a new inline
// type is named by a person.
func names() map[string]string {
	return map[string]string{
		"entries.response/entries/[]":   "Entry",
		"findings.response/findings/[]": "Finding",
		"rank.request/pairs/[]":         "Pair",
		"rank.request/pairs/[]/aspect":  "Aspect",
		"rank.response/words/[]":        "Word",
		"preset.request/entries/[]":     "PresetEntry",
		"preset.response/files/[]":      "PresetFile",
		"plan.response/steps/[]":        "Step",
		"plan.request/for":              "For",
		"request/op":                    "Op",
		"error/code":                    "Code",
		"result/kind":                   "Kind",

		"manifest":                             "Manifest",
		"manifest/preset":                      "ManifestPreset",
		"manifest/preset/readback":             "PresetReadback",
		"manifest/counted":                     "Counted",
		"manifest/counted/loosening/[]":        "Loosening",
		"manifest/counted/loosening/[]/oneOf0": "LooseningValues",
		"manifest/counted/loosening/[]/oneOf1": "LooseningUnless",
		"manifest/wiring":                      "Wiring",
		"manifest/wiring/import":               "WiringImport",
		"manifest/programs/[]":                 "Program",
		"manifest/programs/[]/oneOf0":          "ProgramFound",
		"manifest/programs/[]/oneOf1":          "ProgramAt",
		"manifest/programs/[]/oneOf2":          "ProgramScript",
		"manifest/wiring/import/form":          "WiringForm",
	}
}

// reader makes the types of the schemas.
type reader struct {
	file    string
	defs    *ordered.Map
	made    map[string]*typ // by file and def key
	enums   map[string]*typ // by values
	order   []*typ          // every named type, in the order made
	byName  map[string]*typ
	pending map[string]bool
}

// Generate reads the schemas and gives the Go files, by repository-relative
// path. in holds the schema files by name.
func Generate(in map[string][]byte) (map[string][]byte, error) {
	r := &reader{made: map[string]*typ{}, enums: map[string]*typ{}, byName: map[string]*typ{}, pending: map[string]bool{}}
	byFile := map[string][]*typ{}
	for _, f := range Files() {
		data, ok := in[f]
		if !ok {
			return nil, fmt.Errorf("%s%s: not read", SchemaDir, f)
		}
		v, err := ordered.ParseYAML(data)
		if err != nil {
			return nil, fmt.Errorf("%s%s: %w", SchemaDir, f, err)
		}
		root, ok := v.(*ordered.Map)
		if !ok {
			return nil, fmt.Errorf("%s%s: not a JSON object", SchemaDir, f)
		}
		before := len(r.order)
		r.file = f
		if err := r.read(root); err != nil {
			return nil, fmt.Errorf("%s%s: %w", SchemaDir, f, err)
		}
		byFile[f] = slices.Clone(r.order[before:])
	}
	return emit(r, byFile)
}

// read makes the types of one schema: every def of a wire, or the manifest.
func (r *reader) read(root *ordered.Map) error {
	if err := refuseUnread(root, "the schema"); err != nil {
		return err
	}
	defs := root.Sub("$defs")
	r.defs = defs
	if root.Sub("properties") != nil {
		// The manifest: one object, with defs its members refer to.
		t, err := r.node(root, "manifest")
		if err != nil {
			return err
		}
		if t.kind != kStruct {
			return fmt.Errorf("the manifest is not an object")
		}
		return nil
	}
	if defs == nil {
		return fmt.Errorf("the schema has neither properties nor $defs")
	}
	for _, k := range defs.Keys() {
		if _, err := r.def(k); err != nil {
			return fmt.Errorf("$defs/%s: %w", k, err)
		}
	}
	return nil
}

// refuseUnread fails on a keyword the generator does not read.
func refuseUnread(n *ordered.Map, where string) error {
	for _, k := range n.Keys() {
		if !slices.Contains(handled(), k) {
			return fmt.Errorf("%s uses %q, which the generator does not read", where, k)
		}
	}
	return nil
}

// def is the type of a def of this file.
func (r *reader) def(key string) (*typ, error) {
	id := r.file + "#" + key
	if t, ok := r.made[id]; ok {
		return t, nil
	}
	if r.pending[id] {
		return nil, fmt.Errorf("$defs/%s refers to itself", key)
	}
	r.pending[id] = true
	defer delete(r.pending, id)
	n := r.defs.Sub(key)
	if n == nil {
		return nil, fmt.Errorf("$defs/%s is not an object", key)
	}
	t, err := r.node(n, key)
	if err != nil {
		return nil, err
	}
	r.made[id] = t
	return t, nil
}

// refined is the def key with the string keywords its $ref node adds: both
// apply, so the least and most are the tighter of the two and each pattern must
// match. Another keyword beside a $ref is refused.
func (r *reader) refined(n *ordered.Map, place, key string) (*typ, error) {
	base, err := r.def(key)
	if err != nil {
		return nil, err
	}
	var own []string
	for _, k := range n.Keys() {
		switch k {
		case "$ref", "description":
		case "type", "minLength", "maxLength", "pattern":
			own = append(own, k)
		default:
			return nil, fmt.Errorf("%s: %q beside a $ref, which the generator does not read", place, k)
		}
	}
	if len(own) == 0 {
		return base, nil
	}
	if base.kind != kString {
		return nil, fmt.Errorf("%s: string keywords beside a $ref to a def that is not a string", place)
	}
	t := *base
	if _, ok := n.Get("minLength"); ok {
		t.minLen = max(t.minLen, intOr(n, "minLength", 0))
	}
	if _, ok := n.Get("maxLength"); ok {
		m := intOr(n, "maxLength", -1)
		if t.maxLen < 0 || m < t.maxLen {
			t.maxLen = m
		}
	}
	if p := n.Str("pattern"); p != "" && p != t.pattern {
		if _, err := regexp.Compile(p); err != nil {
			return nil, fmt.Errorf("%s: the pattern %q is not one Go reads: %w", place, p, err)
		}
		if t.pattern == "" {
			t.pattern = p
		} else {
			t.also = append(slices.Clone(t.also), p)
		}
	}
	return &t, nil
}

// node is the type of a schema node at place.
func (r *reader) node(n *ordered.Map, place string) (*typ, error) {
	if err := refuseUnread(n, place); err != nil {
		return nil, err
	}
	if ref := n.Str("$ref"); ref != "" {
		key, ok := strings.CutPrefix(ref, "#/$defs/")
		if !ok {
			return nil, fmt.Errorf("%s: $ref %q is not a def of this file", place, ref)
		}
		return r.refined(n, place, key)
	}
	if enum := n.List("enum"); enum != nil {
		return r.enum(n, place, ordered.Strings(enum))
	}
	if one := n.List("oneOf"); one != nil {
		return r.oneOf(n, place, ordered.Maps(one))
	}
	if all := n.List("allOf"); all != nil {
		return r.allOf(n, place, ordered.Maps(all))
	}
	switch n.Str("type") {
	case "string", "integer", "boolean":
		return scalar(n, place)
	case "array":
		return r.list(n, place)
	case "object", "":
		if n.Sub("properties") != nil {
			return r.object(n, place, nil)
		}
		if n.Str("type") == "" && n.Len() == 0 {
			return &typ{kind: kAny}, nil
		}
		if n.Str("type") == "object" {
			return &typ{kind: kObject}, nil
		}
	}
	return nil, fmt.Errorf("%s: a node the generator does not read", place)
}

// scalar is the type of a string, an integer or a boolean node.
func scalar(n *ordered.Map, place string) (*typ, error) {
	switch n.Str("type") {
	case "string":
		t := &typ{kind: kString, minLen: intOr(n, "minLength", 0), maxLen: intOr(n, "maxLength", -1), pattern: n.Str("pattern")}
		if t.pattern != "" {
			if _, err := regexp.Compile(t.pattern); err != nil {
				return nil, fmt.Errorf("%s: the pattern %q is not one Go reads: %w", place, t.pattern, err)
			}
		}
		return t, nil
	case "integer":
		return &typ{kind: kInt, lo: int64Or(n, "minimum", -(1<<53 - 1)), hi: int64Or(n, "maximum", 1<<53-1)}, nil
	}
	return &typ{kind: kBool}, nil
}

func isFalse(n *ordered.Map, k string) bool {
	v, ok := n.Get(k)
	return ok && v == false
}

func intOr(n *ordered.Map, k string, d int) int {
	if v, ok := n.Get(k); ok {
		if i, ok := v.(int64); ok {
			return int(i)
		}
	}
	return d
}

func int64Or(n *ordered.Map, k string, d int64) int64 {
	if v, ok := n.Get(k); ok {
		if i, ok := v.(int64); ok {
			return i
		}
	}
	return d
}

// enum is the type of an enum: the one made for its values before, or a new
// one named for its def or its place.
func (r *reader) enum(n *ordered.Map, place string, values []string) (*typ, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%s: an enum with no value", place)
	}
	key := strings.Join(values, "\x00")
	if t, ok := r.enums[key]; ok {
		return t, nil
	}
	name, err := r.nameOf(place)
	if err != nil {
		return nil, err
	}
	t := &typ{kind: kEnum, name: name, values: values}
	r.enums[key] = t
	return r.register(t)
}

// nameOf is the Go name of a type at place: that of its def, and for an inline
// type the one names gives.
func (r *reader) nameOf(place string) (string, error) {
	if name, ok := names()[place]; ok {
		return name, nil
	}
	if !strings.Contains(place, "/") {
		return defName(place), nil
	}
	return "", fmt.Errorf("%s: an inline type with no name in the generator's names", place)
}

// defName is the Go name of a def's key: plan.request is PlanRequest.
func defName(key string) string {
	var b strings.Builder
	for _, w := range strings.FieldsFunc(key, func(r rune) bool { return r == '.' || r == '-' || r == '_' }) {
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// register adds a named type, which may not share its name with another.
func (r *reader) register(t *typ) (*typ, error) {
	if other, ok := r.byName[t.name]; ok && other != t {
		return nil, fmt.Errorf("two types are named %s", t.name)
	}
	r.byName[t.name] = t
	r.order = append(r.order, t)
	return t, nil
}

// list is the type of an array.
func (r *reader) list(n *ordered.Map, place string) (*typ, error) {
	items := n.Sub("items")
	if items == nil {
		return nil, fmt.Errorf("%s: an array with no items", place)
	}
	e, err := r.node(items, place+"/[]")
	if err != nil {
		return nil, err
	}
	t := &typ{kind: kList, elem: e, minItems: intOr(n, "minItems", 0), maxIts: intOr(n, "maxItems", -1), unique: n.Truthy("uniqueItems")}
	if t.unique && e.kind == kList {
		return nil, fmt.Errorf("%s: uniqueItems is not read of lists of lists", place)
	}
	return t, nil
}

// object is the type of an object with members. base is the members of an allOf.
func (r *reader) object(n *ordered.Map, place string, base *typ) (*typ, error) {
	name, err := r.nameOf(place)
	if err != nil {
		return nil, err
	}
	t := &typ{kind: kStruct, name: name, doc: n.Str("description"), closed: isFalse(n, "additionalProperties"), minProps: intOr(n, "minProperties", 0)}
	if base != nil {
		for _, f := range base.fields {
			c := *f
			t.fields = append(t.fields, &c)
		}
	}
	required := ordered.Strings(n.List("required"))
	if props := n.Sub("properties"); props != nil {
		for _, k := range props.Keys() {
			f, err := r.member(props, k, place, slices.Contains(required, k))
			if err != nil {
				return nil, err
			}
			t.fields = append(t.fields, f)
		}
	}
	for _, k := range required {
		if !slices.ContainsFunc(t.fields, func(f *field) bool { return f.json == k }) {
			// A member of a base the allOf brought, or one the object lacks.
			return nil, fmt.Errorf("%s: requires %q, which it has no property for", place, k)
		}
	}
	if err := r.rules(n, place, t); err != nil {
		return nil, err
	}
	if base != nil {
		// The members of the base keep the base's required; the allOf's own
		// required names are applied above to the whole.
		for _, f := range t.fields {
			if slices.Contains(required, f.json) {
				f.required = true
			}
		}
	}
	return r.register(t)
}

// member is the member k of props of the object at place: a constant, or a
// typed member.
func (r *reader) member(props *ordered.Map, k, place string, required bool) (*field, error) {
	pn := props.Sub(k)
	if pn == nil {
		return nil, fmt.Errorf("%s/%s: not an object", place, k)
	}
	f := &field{json: k, name: fieldName(k), required: required, doc: pn.Str("description")}
	c, ok := pn.Get("const")
	if e := pn.List("enum"); !ok && len(e) == 1 {
		// A one-value enum is a const spelled another way, and tags a
		// branch as one does.
		c, ok = e[0], true
	}
	if !ok {
		var err error
		f.t, err = r.node(pn, place+"/"+k)
		return f, err
	}
	switch v := c.(type) {
	case string:
		f.constant = "canon.String(" + strconv.Quote(v) + ")"
	case int64:
		f.constant = "canon.Int(" + strconv.FormatInt(v, 10) + ")"
	default:
		return nil, fmt.Errorf("%s/%s: a const that is neither a string nor a whole number", place, k)
	}
	if !required {
		return nil, fmt.Errorf("%s/%s: a const that is not required", place, k)
	}
	return f, nil
}

// rules reads the oneOf of required, and the if and then, of an object.
func (r *reader) rules(n *ordered.Map, place string, t *typ) error {
	if one := n.List("oneOf"); one != nil {
		if err := oneOfGroup(ordered.Maps(one), place, t); err != nil {
			return err
		}
	}
	return ifThen(n, place, t)
}

// oneOfGroup reads a oneOf of required lists as one group, of which exactly one
// member stands.
func oneOfGroup(alts []*ordered.Map, place string, t *typ) error {
	var group []string
	for _, alt := range alts {
		req := alt.List("required")
		if alt.Len() != 1 || req == nil {
			return fmt.Errorf("%s: a oneOf that is not a list of required members", place)
		}
		group = append(group, ordered.Strings(req)...)
	}
	t.exactlyOne = append(t.exactlyOne, group)
	// Every member of a group is optional on its own.
	for _, g := range t.exactlyOne {
		for _, k := range g {
			i := slices.IndexFunc(t.fields, func(f *field) bool { return f.json == k })
			if i < 0 {
				return fmt.Errorf("%s: oneOf requires %q, which it has no property for", place, k)
			}
			t.fields[i].required = false
		}
	}
	return nil
}

// ifThen reads the if and then of an object: when a member is a string, others
// are required.
func ifThen(n *ordered.Map, place string, t *typ) error {
	cond, then := n.Sub("if"), n.Sub("then")
	if (cond == nil) != (then == nil) {
		return fmt.Errorf("%s: an if with no then, or a then with no if", place)
	}
	if cond == nil {
		return nil
	}
	props := cond.Sub("properties")
	if cond.Len() != 1 || props == nil || props.Len() != 1 {
		return fmt.Errorf("%s: an if that is not one member's constant", place)
	}
	prop := props.Keys()[0]
	is, ok := props.Sub(prop).Get("const")
	s, isStr := is.(string)
	if !ok || !isStr || props.Sub(prop).Len() != 1 {
		return fmt.Errorf("%s: an if on %s that is not a string constant", place, prop)
	}
	if then.Len() != 1 || then.List("required") == nil {
		return fmt.Errorf("%s: a then that is not a list of required members", place)
	}
	req := ordered.Strings(then.List("required"))
	for _, k := range req {
		if !slices.ContainsFunc(t.fields, func(f *field) bool { return f.json == k }) {
			return fmt.Errorf("%s: then requires %q, which it has no property for", place, k)
		}
	}
	t.whens = append(t.whens, when{prop, s, req})
	return nil
}

// allOf is the type of an allOf of one $ref to an object, with its own members.
func (r *reader) allOf(n *ordered.Map, place string, all []*ordered.Map) (*typ, error) {
	if len(all) != 1 || all[0].Str("$ref") == "" {
		return nil, fmt.Errorf("%s: an allOf that is not one $ref", place)
	}
	base, err := r.node(all[0], place+"/allOf0")
	if err != nil {
		return nil, err
	}
	if base.kind != kStruct || base.closed {
		return nil, fmt.Errorf("%s: an allOf of what is not an open object", place)
	}
	return r.object(n, place, base)
}

// oneOf is the type of a oneOf: an object with a group of members of which one
// stands, or a union of objects.
func (r *reader) oneOf(n *ordered.Map, place string, alts []*ordered.Map) (*typ, error) {
	if n.Sub("properties") != nil {
		return r.object(n, place, nil)
	}
	name, err := r.nameOf(place)
	if err != nil {
		return nil, err
	}
	u := &typ{kind: kUnion, name: name}
	for i, a := range alts {
		at, err := r.node(a, fmt.Sprintf("%s/oneOf%d", place, i))
		if err != nil {
			return nil, err
		}
		if at.kind != kStruct {
			return nil, fmt.Errorf("%s: an alternative that is not an object", place)
		}
		u.alts = append(u.alts, at)
		u.altFields = append(u.altFields, strings.TrimPrefix(at.name, name))
	}
	u.disc = discriminator(u.alts)
	return r.register(u)
}

// discriminator is the one member every alternative has as a constant, and
// that differs, or "" when no member is one.
func discriminator(alts []*typ) string {
	if len(alts) == 0 {
		return ""
	}
	for _, f := range alts[0].fields {
		if f.constant == "" {
			continue
		}
		seen := map[string]bool{}
		ok := true
		for _, a := range alts {
			i := slices.IndexFunc(a.fields, func(g *field) bool { return g.json == f.json && g.constant != "" })
			if i < 0 || seen[a.fields[i].constant] {
				ok = false
				break
			}
			seen[a.fields[i].constant] = true
		}
		if ok {
			return f.json
		}
	}
	return ""
}

// fieldName is the Go name of a member: words joined by underscores, each
// capitalised, and an initialism in capitals.
func fieldName(key string) string {
	var b strings.Builder
	for _, w := range strings.FieldsFunc(key, func(r rune) bool { return r == '_' || r == '-' }) {
		w = strings.ToUpper(w[:1]) + w[1:]
		switch w {
		case "Id", "Url", "Api", "Uri", "Sha":
			w = strings.ToUpper(w)
		}
		b.WriteString(w)
	}
	return b.String()
}
