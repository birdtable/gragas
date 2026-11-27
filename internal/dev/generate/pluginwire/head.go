package pluginwire

// codecHead is the top of codec.gen.go: what every generated FromCanon reads
// with. It is generated Go, so it is a string here and a file there, and the
// generated tests of the package hold it.
const codecHead = `import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
)

// Fault is why a message does not read: where, as a JSON Pointer into it
// (empty for the whole message), and what is wrong there.
type Fault struct {
	At  string
	Msg string
}

// Error says where and what. The place is written by Shown: it can hold the
// plugin's own key, the member no message names, so a caller that prints a
// Fault as it is prints no character a screen does not show.
// What is wrong is gragas's words, from the schema, and is written as it is.
func (f *Fault) Error() string {
	if f.At == "" {
		return "the message " + f.Msg
	}
	return Shown(f.At) + " " + f.Msg
}

// fault is a Fault at p.
func fault(p, format string, args ...any) *Fault {
	return &Fault{At: p, Msg: fmt.Sprintf(format, args...)}
}

// pointer is the JSON Pointer of member k of the value at p.
func pointer(p, k string) string {
	return p + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(k)
}

// index is the JSON Pointer of element i of the value at p.
func index(p string, i int) string { return p + "/" + strconv.Itoa(i) }

// object is v as an object; null and every other kind are refused.
func object(v canon.Value, p string) (canon.Object, error) {
	o, ok := v.(canon.Object)
	if !ok {
		return nil, fault(p, "is %s, not an object", kindOf(v))
	}
	return o, nil
}

// array is v as an array of at least lo and, when hi is not negative, at most
// hi elements.
func array(v canon.Value, p string, lo, hi int) (canon.Array, error) {
	a, ok := v.(canon.Array)
	if !ok {
		return nil, fault(p, "is %s, not an array", kindOf(v))
	}
	if len(a) < lo {
		return nil, fault(p, "has %d elements, fewer than %d", len(a), lo)
	}
	if hi >= 0 && len(a) > hi {
		return nil, fault(p, "has %d elements, more than %d", len(a), hi)
	}
	return a, nil
}

// text is v as a string of lo to hi UTF-8 bytes (hi negative: no most), as the
// design holds a cap (4.3), that
// re, when it is not nil, finds in it.
func text(v canon.Value, p string, lo, hi int, re *regexp.Regexp) (string, error) {
	s, ok := v.(canon.String)
	if !ok {
		return "", fault(p, "is %s, not a string", kindOf(v))
	}
	n := len(s)
	if n < lo {
		return "", fault(p, "has %d bytes, fewer than %d", n, lo)
	}
	if hi >= 0 && n > hi {
		return "", fault(p, "has %d bytes, more than %d", n, hi)
	}
	if re != nil && !re.MatchString(string(s)) {
		return "", fault(p, "does not match %s", re)
	}
	return string(s), nil
}

// whole is v as a whole number from lo to hi.
func whole(v canon.Value, p string, lo, hi int64) (int, error) {
	n, ok := v.(canon.Int)
	if !ok {
		return 0, fault(p, "is %s, not a whole number", kindOf(v))
	}
	if int64(n) < lo || int64(n) > hi {
		return 0, fault(p, "is %d, outside %d to %d", n, lo, hi)
	}
	return int(n), nil
}

// truth is v as a boolean.
func truth(v canon.Value, p string) (bool, error) {
	b, ok := v.(canon.Bool)
	if !ok {
		return false, fault(p, "is %s, not true or false", kindOf(v))
	}
	return bool(b), nil
}

// known refuses the first member of o, by name, that is not one of names.
func known(o canon.Object, p string, names ...string) error {
	for _, k := range canon.SortedKeys(o) {
		if !slices.Contains(names, k) {
			return fault(pointer(p, k), "is not a member of this message")
		}
	}
	return nil
}

// exactlyOne refuses o unless exactly one of names is a member of it.
func exactlyOne(o canon.Object, p string, names ...string) error {
	n := 0
	for _, k := range names {
		if _, ok := o[k]; ok {
			n++
		}
	}
	if n != 1 {
		return fault(p, "has %d of the members %s, and must have one", n, strings.Join(names, ", "))
	}
	return nil
}

// needs refuses o when its member prop is is and one of names is not a member.
func needs(o canon.Object, p, prop string, is canon.Value, names ...string) error {
	if m, ok := o[prop]; !ok || m != is {
		return nil
	}
	for _, k := range names {
		if _, ok := o[k]; !ok {
			return fault(p, "has no member %s, which a %s of that kind has", k, prop)
		}
	}
	return nil
}

// distinct refuses a list with an element twice.
func distinct[T comparable](xs []T, p string) error {
	seen := map[T]int{}
	for i, x := range xs {
		if j, ok := seen[x]; ok {
			return fault(index(p, i), "repeats element %d", j)
		}
		seen[x] = i
	}
	return nil
}

// distinctValues refuses a list with two elements that are the same value.
func distinctValues[T any](xs []T, p string, of func(T) canon.Value) error {
	vs := make([]canon.Value, len(xs))
	for i, x := range xs {
		vs[i] = of(x)
		for j := range i {
			if reflect.DeepEqual(vs[j], vs[i]) {
				return fault(index(p, i), "repeats element %d", j)
			}
		}
	}
	return nil
}

// listOf is xs as canon's array, each element by of; a nil list is an empty one.
func listOf[T any](xs []T, of func(T) canon.Value) canon.Value {
	out := make(canon.Array, len(xs))
	for i, x := range xs {
		out[i] = of(x)
	}
	return out
}

// kindOf names what v is, for a refusal.
func kindOf(v canon.Value) string {
	switch v.(type) {
	case canon.Null:
		return "null"
	case canon.Bool:
		return "a boolean"
	case canon.Int, canon.Float:
		return "a number"
	case canon.String:
		return "a string"
	case canon.Array:
		return "an array"
	case canon.Object:
		return "an object"
	}
	return "no value"
}
`
