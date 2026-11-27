package link

import (
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// conditionals are keys that would make a value depend on something, and
// references keys that would make one place stand for another. Neither is a
// key of the adapter's shape; each is refused as what it is, so a person sees
// why the shape has no such thing (gates.registry.8).
func conditionals() []string {
	return []string{"if", "when", "unless", "then", "else", "switch", "case"}
}

func references() []string { return []string{"ref", "include", "import", "use", "inherit"} }

// keys refuses each key of m that known does not name, saying what it would
// have been.
func (r *reader) keys(m *ordered.Map, where string, known ...string) {
	for _, k := range m.Keys() {
		if slices.Contains(known, k) {
			continue
		}
		line, low := m.KeyLine(k), strings.ToLower(k)
		switch {
		case slices.Contains(conditionals(), low):
			r.refuse(line, CheckConditional, "%s is a conditional, and %s states one value for every case; write it without %s", at(where, k), r.an(), k)
		case strings.HasPrefix(k, "$") || slices.Contains(references(), low):
			r.refuse(line, CheckReference, "%s refers to a value elsewhere, and %s writes every value out", at(where, k), r.an())
		case expressive(k):
			r.refuse(line, CheckExpression, "the key %q is written as an expression, and %s holds none", k, r.an())
		default:
			r.refuse(line, CheckKey, "%s is not a field of %s; the fields here are %s", at(where, k), r.an(), strings.Join(known, ", "))
		}
	}
}

// at is the dotted place of key k below where.
func at(where, k string) string {
	if where == "" {
		return k
	}
	return where + "." + k
}

// expressive is whether s holds a character an expression or a substitution
// is written with.
func expressive(s string) bool { return strings.ContainsAny(s, "{}$`\\") }

// text reads the string under k, refusing another type and one over n bytes.
// A key that is absent is "" with ok false; required says whether that is a
// refusal.
func (r *reader) text(m *ordered.Map, where, k string, n int, required bool) (string, bool) {
	v, has := m.Get(k)
	if !has {
		if required {
			r.refuse(m.Line, CheckShape, "%s is missing", at(where, k))
		}
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		r.refuse(m.KeyLine(k), CheckShape, "%s must be text, and it is %s", at(where, k), ordered.Repr(v))
		return "", false
	}
	if len(s) > n {
		r.refuse(m.KeyLine(k), CheckSize, "%s is %d bytes, over the cap of %d", at(where, k), len(s), n)
		return "", false
	}
	return s, true
}

// word reads the text under k and refuses it outside form, after an
// expression's characters, which are refused as an expression.
func (r *reader) word(m *ordered.Map, where, k string, form *regexp.Regexp, says string, required bool) string {
	s, ok := r.text(m, where, k, maxText, required)
	if !ok {
		return ""
	}
	if expressive(s) {
		r.refuse(m.KeyLine(k), CheckExpression, "%s holds %q, an expression's characters, and %s holds no expression", at(where, k), s, r.an())
		return ""
	}
	if !form.MatchString(s) {
		r.refuse(m.KeyLine(k), CheckShape, "%s is %q, and it must be %s", at(where, k), s, says)
		return ""
	}
	return s
}

// oneOf reads the text under k and refuses it outside set.
func oneOf[T ~string](r *reader, m *ordered.Map, where, k string, set []T, required bool) T {
	s, ok := r.text(m, where, k, maxText, required)
	if !ok {
		return ""
	}
	if !slices.Contains(set, T(s)) {
		r.refuse(m.KeyLine(k), CheckShape, "%s is %q, and it must be one of %s", at(where, k), s, joined(set))
		return ""
	}
	return T(s)
}

func joined[T ~string](set []T) string {
	out := make([]string, len(set))
	for i, v := range set {
		out[i] = string(v)
	}
	return strings.Join(out, ", ")
}

// sub reads the mapping under k.
func (r *reader) sub(m *ordered.Map, where, k string, required bool) *ordered.Map {
	v, has := m.Get(k)
	if !has {
		if required {
			r.refuse(m.Line, CheckShape, "%s is missing", at(where, k))
		}
		return nil
	}
	s, ok := v.(*ordered.Map)
	if !ok {
		r.refuse(m.KeyLine(k), CheckShape, "%s must be a mapping, and it is %s", at(where, k), ordered.Repr(v))
	}
	return s
}

// format refuses a document of any format but this engine's: an older one,
// since adapters began at format 3 and there is nothing to carry forward, and
// a newer one, which is left as it is (store.schemas.2).
func (r *reader) format(m *ordered.Map) bool {
	v, has := m.Get("format")
	if !has {
		r.refuse(m.Line, CheckFormat, "format is missing; %s says the format it is written in, and this engine reads format %d", r.an(), model.FormatVersion)
		return false
	}
	n, ok := v.(int64)
	switch {
	case !ok:
		r.refuse(m.KeyLine("format"), CheckFormat, "format must be a whole number, and it is %s", ordered.Repr(v))
	case n < model.FormatVersion:
		r.refuse(m.KeyLine("format"), CheckFormat, "the document is format %d, older than %d, and %s began at format %d: write it in format %d", n, model.FormatVersion, r.many(), model.FormatVersion, model.FormatVersion)
	case n > model.FormatVersion:
		r.refuse(m.KeyLine("format"), CheckFormat, "the document is format %d, newer than this engine's %d: it is left as it is, and a newer gragas reads it", n, model.FormatVersion)
	default:
		return true
	}
	return false
}
