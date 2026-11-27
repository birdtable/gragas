package contract

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// window is how many minors a deprecated shape is emitted beside the new one
// before it is past its window (spec section 9, architecture 7.3).
const window = 2

// unfollowed are the keywords of JSON Schema 2020-12 that hold a schema for a
// place in the document and that the deprecation walk does not follow. A
// schema that uses one is an error, so a mark under it is never missed
// quietly.
func unfollowed() []string {
	return []string{
		"if", "then", "else", "not", "patternProperties", "prefixItems", "contains",
		"propertyNames", "dependentSchemas", "unevaluatedProperties", "unevaluatedItems",
		"$dynamicRef",
	}
}

// deprecated is a finding for each place in obj, a document of kind k that
// names contract ver, which a schema of ver marks x-deprecated and whose
// window has ended by ver. It walks the document beside its schema, read from
// files: a schema reached through $ref, allOf, anyOf or oneOf applies at the
// same place, and properties, additionalProperties and items lead to the
// places below it. A mark in any branch of anyOf or oneOf counts, since a
// field a branch names is that field wherever it is present. The findings
// are one per place, sorted by it.
func deprecated(k Kind, obj canon.Object, ver Version, files port.Assets) ([]model.Finding, error) {
	w := &walker{
		kind: k, found: ver, files: files, dir: fmt.Sprintf("%s/v%d", SchemaDir, ver.Major),
		loaded: map[string]canon.Value{}, seen: map[string]bool{},
	}
	root := fmt.Sprintf("document-%s.schema.json", k)
	top, err := w.load(root)
	if err != nil {
		return nil, err
	}
	if err := w.walk(obj, "", root, top, map[string]bool{}); err != nil {
		return nil, err
	}
	slices.SortFunc(w.out, func(a, b model.Finding) int { return strings.Compare(a.Entity, b.Entity) })
	return w.out, nil
}

type walker struct {
	kind   Kind
	found  Version
	files  port.Assets
	dir    string
	loaded map[string]canon.Value
	seen   map[string]bool // the places already found
	out    []model.Finding
}

// load is the schema file name, read once.
func (w *walker) load(name string) (canon.Value, error) {
	if v, ok := w.loaded[name]; ok {
		return v, nil
	}
	if name == "" || strings.Contains(name, ":") || path.Clean(name) != name || path.IsAbs(name) || strings.HasPrefix(name, "..") {
		return nil, fmt.Errorf("the schema %s refers to %q, which is not a file beside it", w.dir, name)
	}
	b, err := w.files.Read(path.Join(w.dir, name))
	if err != nil {
		return nil, fmt.Errorf("read the schema %s: %w", name, err)
	}
	v, err := canon.Decode(b)
	if err != nil {
		return nil, fmt.Errorf("read the schema %s: %w", name, err)
	}
	w.loaded[name] = v
	return v, nil
}

// walk visits doc, the value at the JSON Pointer at, beside schema, a schema
// in the file file. refs are the references already followed at this place,
// so a reference that comes back to itself ends.
func (w *walker) walk(doc canon.Value, at, file string, schema canon.Value, refs map[string]bool) error {
	s, isObject := schema.(canon.Object)
	if !isObject {
		return nil // true or false: no subschema and no mark
	}
	for _, kw := range unfollowed() {
		if _, present := s[kw]; present {
			return fmt.Errorf("the schema %s uses %s, which the deprecation walk does not follow", file, kw)
		}
	}
	if err := w.mark(at, file, s); err != nil {
		return err
	}
	if err := w.follow(doc, at, file, s, refs); err != nil {
		return err
	}
	for _, kw := range []string{"allOf", "anyOf", "oneOf"} {
		raw, present := s[kw]
		if !present {
			continue
		}
		subs, isArray := raw.(canon.Array)
		if !isArray {
			return fmt.Errorf("the schema %s has an %s that is not an array", file, kw)
		}
		for _, sub := range subs {
			if err := w.walk(doc, at, file, sub, refs); err != nil {
				return err
			}
		}
	}
	return w.descend(doc, at, file, s)
}

// follow walks doc beside the schema s's $ref points at, once for each
// reference at this place.
func (w *walker) follow(doc canon.Value, at, file string, s canon.Object, refs map[string]bool) error {
	raw, present := s["$ref"]
	if !present {
		return nil
	}
	ref, isString := raw.(canon.String)
	if !isString {
		return fmt.Errorf("the schema %s has a $ref that is not a string", file)
	}
	to, target, err := w.resolve(file, string(ref))
	if err != nil {
		return err
	}
	_, pointer, _ := strings.Cut(string(ref), "#")
	key := to + "#" + pointer
	if refs[key] {
		return nil
	}
	refs[key] = true
	return w.walk(doc, at, to, target, refs)
}

// descend walks each member or item of doc beside the schema s gives it:
// properties, else additionalProperties, for an object, and items for an
// array.
func (w *walker) descend(doc canon.Value, at, file string, s canon.Object) error {
	switch d := doc.(type) {
	case canon.Object:
		return w.members(d, at, file, s)
	case canon.Array:
		items, present := s["items"]
		if !present {
			return nil
		}
		for i, e := range d {
			if err := w.walk(e, fmt.Sprintf("%s/%d", at, i), file, items, map[string]bool{}); err != nil {
				return err
			}
		}
	}
	return nil
}

// members walks each member of the object doc beside the schema s gives it,
// in properties or else additionalProperties.
func (w *walker) members(doc canon.Object, at, file string, s canon.Object) error {
	props, isObject := s["properties"].(canon.Object)
	if !isObject {
		props = nil
	}
	for _, key := range canon.SortedKeys(doc) {
		sub, named := props[key]
		if !named {
			sub, named = s["additionalProperties"]
		}
		if !named {
			continue
		}
		if err := w.walk(doc[key], at+"/"+escape(key), file, sub, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

// mark records a finding for the place at when s marks it x-deprecated and
// its window has ended by the version the document names.
func (w *walker) mark(at, file string, s canon.Object) error {
	raw, present := s["x-deprecated"]
	if !present {
		return nil
	}
	dep, isObject := raw.(canon.Object)
	since, okSince := dep["since"].(canon.String)
	removed, okRemoved := dep["removed_in"].(canon.String)
	if !isObject || !okSince || !okRemoved {
		return fmt.Errorf("the schema %s has an x-deprecated that is not {since, removed_in}", file)
	}
	from, err := ParseVersion(string(since))
	if err != nil {
		return fmt.Errorf("the schema %s: x-deprecated since: %w", file, err)
	}
	gone, err := ParseVersion(string(removed))
	if err != nil {
		return fmt.Errorf("the schema %s: x-deprecated removed_in: %w", file, err)
	}
	if at == "" || w.seen[at] || !past(w.found, from, gone) {
		return nil
	}
	w.seen[at] = true
	w.out = append(w.out, model.Finding{
		Entity: string(w.kind) + " " + at,
		Msg: fmt.Sprintf("%s is a shape deprecated since contract %s, and its window of %d minors ended before %s, the contract the %s names",
			at, from, window, w.found, w.kind),
		Fix: fmt.Sprintf("write the shape that replaced it; contract %s removes this one", gone),
	})
	return nil
}

// past is whether a shape deprecated since from, and removed in gone, is
// past its window in a document of contract found: both shapes are emitted
// for two minors, from and the one after it.
func past(found, from, gone Version) bool {
	if found.Major > gone.Major || (found.Major == gone.Major && found.Minor >= gone.Minor) {
		return true
	}
	return found.Major == from.Major && found.Minor >= from.Minor+window
}

// resolve is the schema ref points at, from the file file: a file beside it,
// a JSON Pointer inside one, or both.
func (w *walker) resolve(file, ref string) (string, canon.Value, error) {
	name, pointer, _ := strings.Cut(ref, "#")
	if name == "" {
		name = file
	}
	v, err := w.load(name)
	if err != nil {
		return "", nil, err
	}
	if pointer == "" {
		return name, v, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return "", nil, fmt.Errorf("the schema %s refers to %q, which is not a JSON Pointer", file, ref)
	}
	for tok := range strings.SplitSeq(pointer[1:], "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		o, isObject := v.(canon.Object)
		next, present := o[tok]
		if !isObject || !present {
			return "", nil, fmt.Errorf("the schema %s refers to %q, which names nothing", file, ref)
		}
		v = next
	}
	return name, v, nil
}

// escape is key as one token of a JSON Pointer (RFC 6901).
func escape(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}
