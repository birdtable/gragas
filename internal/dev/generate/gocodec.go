package generate

import (
	"fmt"
	"strings"
)

// The Go codec (gragas#563). Every declaration gets ToCanon, the value as
// canon's Value tree with the members encoding/json wrote for the type, and
// FromCanon, which reads a value canon decoded back into the type, refusing a
// member no field has under its exact name. So the contract context turns a
// model value into canon's tree and back with no encoding/json between them,
// and no decoder that matches a key without regard to case or keeps the last
// of two keys that fold to one name.
//
// The methods sit beside their types and hold no branch of their own: each is
// a list of calls into the helpers of CodecFile, so the code a schema makes
// stays flat however many fields it has. They read the fields and decide
// nothing; the decides walk (internal/decides_test.go) holds that by name.

// CodecFile is the generated file of the helpers every ToCanon and FromCanon
// is written in. Its name is the generator's: no schema file is named codec.
const CodecFile = GoDir + "codec.gen.go"

// goCodec is the ToCanon and FromCanon methods of the declaration d, named
// name. A declaration of any JSON value is an interface in Go, which takes no
// method, so it is refused.
func goCodec(t *tree, name string, d *decl) (string, error) {
	var to, from string
	switch d.T.Kind {
	case kObject:
		to, from = structTo(t, d.T), structFrom(t, d.T)
	case kRef:
		base := goName(d.T.Ref)
		to = "return " + base + "(v).ToCanon()"
		from = "return (*" + base + ")(v).fromAt(c, at)"
	case kAny:
		return "", fmt.Errorf("%s: %s is any JSON value, which a Go type with methods cannot be: give it a type", d.File, d.Name)
	default:
		under := goType(d.T)
		if d.T.Kind == kEnum {
			under = "string"
		}
		to = "return " + toFunc(d.T) + "(" + under + "(v))"
		from = "x, err := " + readFunc(d.T) + "(c, at)\n\t*v = " + name + "(x)\n\treturn err"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n// ToCanon is v as canon's value tree, as encoding/json wrote %s.\nfunc (v %s) ToCanon() canon.Value {\n\t%s\n}\n", name, name, to)
	fmt.Fprintf(&b, "\n// FromCanon reads c, a value canon decoded, into v: a member %s has no\n// field for, under its exact name, is refused.\nfunc (v *%s) FromCanon(c canon.Value) error { return v.fromAt(c, \"\") }\n", name, name)
	fmt.Fprintf(&b, "\n// fromAt is FromCanon of the value at the JSON Pointer at.\nfunc (v *%s) fromAt(c canon.Value, at string) error {\n\t%s\n}\n", name, from)
	return b.String(), nil
}

// structTo is the body of a struct's ToCanon: a required member is always
// written, an omitempty one only when encoding/json would have written it.
// An optional pointer or value of any JSON type is tested for nil inside
// keepPtr or keepValue, so no generated line compares a field with nil: the
// port check (arch.ports.5) reads such a test by the field's name, and a
// field named as a port is (SarifDocument.Schema).
func structTo(t *tree, ty *typ) string {
	if len(ty.Fields) == 0 {
		return "return canon.Object{}"
	}
	var b strings.Builder
	b.WriteString("o := canon.Object{}\n")
	for _, f := range ty.Fields {
		expr := "v." + goField(f.Key)
		pointer := goPointer(t, f)
		has := hasTest(t, f.T, expr)
		switch {
		case f.Required && pointer:
			fmt.Fprintf(&b, "\to[%q] = canon.Null{}\n\tkeepPtr(o, %q, %s, %s)\n", f.Key, f.Key, expr, toFunc(f.T))
		case f.Required:
			fmt.Fprintf(&b, "\to[%q] = %s\n", f.Key, toExpr(f.T, expr))
		case pointer:
			fmt.Fprintf(&b, "\tkeepPtr(o, %q, %s, %s)\n", f.Key, expr, toFunc(f.T))
		case kindUnder(t, f.T) == kAny:
			fmt.Fprintf(&b, "\tkeepValue(o, %q, %s)\n", f.Key, expr)
		case has == "":
			fmt.Fprintf(&b, "\to[%q] = %s\n", f.Key, toExpr(f.T, expr))
		default:
			fmt.Fprintf(&b, "\tkeep(o, %q, %s, %s)\n", f.Key, toExpr(f.T, expr), has)
		}
	}
	b.WriteString("\treturn o")
	return b.String()
}

// structFrom is the body of a struct's fromAt: the object's keys are held to
// the fields', and each member present is read into its field.
func structFrom(t *tree, ty *typ) string {
	keys := make([]string, 0, len(ty.Fields))
	for _, f := range ty.Fields {
		keys = append(keys, fmt.Sprintf("%q", f.Key))
	}
	var b strings.Builder
	b.WriteString("o, err := objectAt(c, at")
	for _, k := range keys {
		b.WriteString(", " + k)
	}
	b.WriteString(")\n\tif o == nil {\n\t\treturn err\n\t}\n\treturn first(\n")
	for _, f := range ty.Fields {
		read := readFunc(f.T)
		if goPointer(t, f) {
			read = "ptrRead(" + read + ")"
		}
		fmt.Fprintf(&b, "\t\tfield(o, at, %q, &v.%s, %s),\n", f.Key, goField(f.Key), read)
	}
	b.WriteString("\t)")
	return b.String()
}

// toFunc is a func(T) canon.Value for the Go type of ty.
func toFunc(ty *typ) string {
	switch ty.Kind {
	case kString, kConst, kEnum:
		return "textOf"
	case kInteger:
		return "wholeOf"
	case kNumber:
		return "numberOf"
	case kBool:
		return "truthOf"
	case kRef:
		return goName(ty.Ref) + ".ToCanon"
	case kMap:
		return "objectOf"
	case kArray:
		return "func(x " + goType(ty) + ") canon.Value { return listOf(x, " + toFunc(ty.Elem) + ") }"
	}
	return "valueOf"
}

// toExpr is the canon value of expr, a field of the Go type of ty and no
// pointer: structTo writes a pointer through keepPtr.
func toExpr(ty *typ, expr string) string {
	switch ty.Kind {
	case kArray:
		return "listOf(" + expr + ", " + toFunc(ty.Elem) + ")"
	case kRef:
		return expr + ".ToCanon()"
	}
	return toFunc(ty) + "(" + expr + ")"
}

// readFunc is a func(canon.Value, string) (T, error) for the Go type of ty.
func readFunc(ty *typ) string {
	switch ty.Kind {
	case kString, kConst, kEnum:
		return "readText"
	case kInteger:
		return "readWhole"
	case kNumber:
		return "readNumber"
	case kBool:
		return "readTruth"
	case kRef:
		return "readVia[" + goName(ty.Ref) + "]"
	case kMap:
		return "readObject"
	case kArray:
		return "listRead(" + readFunc(ty.Elem) + ")"
	}
	return "readValue"
}

// hasTest is the Go test that expr, a field of the Go type of ty and no
// pointer, is not what encoding/json's omitempty leaves out, and "" for a
// struct, which omitempty never leaves out, or a value of any JSON type,
// which structTo writes through keepValue.
func hasTest(t *tree, ty *typ, expr string) string {
	switch kindUnder(t, ty) {
	case kString, kConst, kEnum:
		return expr + ` != ""`
	case kInteger, kNumber:
		return expr + " != 0"
	case kBool:
		return expr
	case kArray, kMap:
		return "len(" + expr + ") > 0"
	}
	return ""
}

// kindUnder is the kind of ty with every reference followed to the
// declaration it names. The tree has no circle (tree.cycles).
func kindUnder(t *tree, ty *typ) string {
	for ty.Kind == kRef {
		ty = t.index[ty.Ref].T
	}
	return ty.Kind
}

// codecFile is CodecFile's text.
func codecFile() []byte {
	return formatted([]byte(header("//", "internal/dev/generate/gocodec.go") + "\n\n" + codecHelpers))
}

const codecHelpers = `package contract

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
)

// The codec every generated ToCanon and FromCanon is written in.
// ToCanon gives the members encoding/json gave the type: a field marked
// omitempty is left out when it is empty, a nil slice, map or pointer that is
// required is null, and a string's bytes that are not UTF-8 are each U+FFFD.
// FromCanon reads as encoding/json's decoder read with unknown fields
// disallowed, but by exact name: a member no field has is refused, a member
// absent or null leaves its field the zero value, and a member of another
// type is refused, naming where by its JSON Pointer.

// keep sets key of o to val when has holds.
func keep(o canon.Object, key string, val canon.Value, has bool) {
	if has {
		o[key] = val
	}
}

// keepPtr sets key of o to of(*p) when p is not nil.
func keepPtr[T any](o canon.Object, key string, p *T, of func(T) canon.Value) {
	if p != nil {
		o[key] = of(*p)
	}
}

// keepValue sets key of o to a copy of v when v is not nil.
func keepValue(o canon.Object, key string, v canon.Value) {
	if v != nil {
		o[key] = valueOf(v)
	}
}

// textOf is s as canon's string, each byte that is not UTF-8 replaced by
// U+FFFD as encoding/json replaced it.
func textOf(s string) canon.Value { return canon.String([]rune(s)) }

// wholeOf is n as canon's whole number.
func wholeOf(n int) canon.Value { return canon.Int(n) }

// numberOf is f as canon's number.
func numberOf(f float64) canon.Value { return canon.Float(f) }

// truthOf is b as canon's boolean.
func truthOf(b bool) canon.Value { return canon.Bool(b) }

// valueOf is v, any JSON value, copied, and null for no value.
func valueOf(v canon.Value) canon.Value {
	if v == nil {
		return canon.Null{}
	}
	return clone(v, 1)
}

// objectOf is o copied, and null for a nil object.
func objectOf(o canon.Object) canon.Value {
	if o == nil {
		return canon.Null{}
	}
	return cloneObject(o, 1)
}

// listOf is xs as canon's array, each element by of, and null for a nil
// slice.
func listOf[T any](xs []T, of func(T) canon.Value) canon.Value {
	if xs == nil {
		return canon.Null{}
	}
	out := make(canon.Array, len(xs))
	for i, x := range xs {
		out[i] = of(x)
	}
	return out
}

// clone is v sharing no array or object with what it was read from or
// copied to, to canon.MaxDepth levels; past them it is v itself, which
// canon's encoder refuses as too deep.
func clone(v canon.Value, depth int) canon.Value {
	if depth > canon.MaxDepth {
		return v
	}
	switch x := v.(type) {
	case canon.Array:
		out := make(canon.Array, len(x))
		for i, e := range x {
			out[i] = clone(e, depth+1)
		}
		return out
	case canon.Object:
		return cloneObject(x, depth)
	}
	return v
}

// cloneObject is clone of an object.
func cloneObject(o canon.Object, depth int) canon.Object {
	out := make(canon.Object, len(o))
	for k, e := range o {
		out[k] = clone(e, depth+1)
	}
	return out
}

// where is a JSON Pointer as a person reads it.
func where(at string) string {
	if at == "" {
		return "the document"
	}
	return at
}

// pointer is at one member deeper, at key, escaped as RFC 6901 escapes it.
func pointer(at, key string) string {
	return at + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

// found names what c is, for a refusal.
func found(c canon.Value) string {
	switch c.(type) {
	case nil:
		return "no value"
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
	return "a value outside canon's set"
}

// mismatch refuses c at at, where want was due.
func mismatch(c canon.Value, at, want string) error {
	return fmt.Errorf("%s: want %s, found %s", where(at), want, found(c))
}

// readText reads a string.
func readText(c canon.Value, at string) (string, error) {
	switch x := c.(type) {
	case canon.Null:
		return "", nil
	case canon.String:
		return string(x), nil
	}
	return "", mismatch(c, at, "a string")
}

// readWhole reads a whole number into an int, and refuses one an int on
// this platform cannot hold.
func readWhole(c canon.Value, at string) (int, error) {
	switch x := c.(type) {
	case canon.Null:
		return 0, nil
	case canon.Int:
		if n := int(x); int64(n) == int64(x) {
			return n, nil
		}
		return 0, fmt.Errorf("%s: %d is past what the field holds", where(at), int64(x))
	}
	return 0, mismatch(c, at, "a whole number")
}

// readNumber reads a number, whole or not.
func readNumber(c canon.Value, at string) (float64, error) {
	switch x := c.(type) {
	case canon.Null:
		return 0, nil
	case canon.Int:
		return float64(x), nil
	case canon.Float:
		return float64(x), nil
	}
	return 0, mismatch(c, at, "a number")
}

// readTruth reads a boolean.
func readTruth(c canon.Value, at string) (bool, error) {
	switch x := c.(type) {
	case canon.Null:
		return false, nil
	case canon.Bool:
		return bool(x), nil
	}
	return false, mismatch(c, at, "a boolean")
}

// readValue reads any JSON value, copied; null is no value.
func readValue(c canon.Value, at string) (canon.Value, error) {
	switch c.(type) {
	case nil:
		return nil, mismatch(c, at, "a value")
	case canon.Null:
		return nil, nil
	}
	return clone(c, 1), nil
}

// readObject reads an object of any members, copied.
func readObject(c canon.Value, at string) (canon.Object, error) {
	switch x := c.(type) {
	case canon.Null:
		return nil, nil
	case canon.Object:
		return cloneObject(x, 1), nil
	}
	return nil, mismatch(c, at, "an object")
}

// listRead is a reader of an array whose elements read reads.
func listRead[T any](read func(canon.Value, string) (T, error)) func(canon.Value, string) ([]T, error) {
	return func(c canon.Value, at string) ([]T, error) {
		switch x := c.(type) {
		case canon.Null:
			return nil, nil
		case canon.Array:
			out := make([]T, len(x))
			for i, e := range x {
				v, err := read(e, at+"/"+strconv.Itoa(i))
				if err != nil {
					return nil, err
				}
				out[i] = v
			}
			return out, nil
		}
		return nil, mismatch(c, at, "an array")
	}
}

// ptrRead is a reader of what read reads or null, into a pointer that null
// leaves nil.
func ptrRead[T any](read func(canon.Value, string) (T, error)) func(canon.Value, string) (*T, error) {
	return func(c canon.Value, at string) (*T, error) {
		if _, null := c.(canon.Null); null {
			return nil, nil
		}
		v, err := read(c, at)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
}

// reader is a generated type whose pointer reads it.
type reader[T any] interface {
	*T
	fromAt(c canon.Value, at string) error
}

// readVia reads a generated type by its own fromAt.
func readVia[T any, P reader[T]](c canon.Value, at string) (T, error) {
	var v T
	if err := P(&v).fromAt(c, at); err != nil {
		var zero T
		return zero, err
	}
	return v, nil
}

// objectAt is c as an object of a generated type whose fields are keys. It is nil and no error for null, which leaves every field its zero
// value, and it refuses any other value and a member whose key is not one of
// keys, exactly.
func objectAt(c canon.Value, at string, keys ...string) (canon.Object, error) {
	switch x := c.(type) {
	case canon.Null:
		return nil, nil
	case canon.Object:
		for _, k := range canon.SortedKeys(x) {
			if !slices.Contains(keys, k) {
				return nil, fmt.Errorf("%s has no field %q", where(at), k)
			}
		}
		return x, nil
	}
	return nil, mismatch(c, at, "an object")
}

// field reads the member key of o, when o has it, into into by read.
func field[T any](o canon.Object, at, key string, into *T, read func(canon.Value, string) (T, error)) error {
	c, has := o[key]
	if !has {
		return nil
	}
	v, err := read(c, pointer(at, key))
	if err != nil {
		return err
	}
	*into = v
	return nil
}

// first is the first of errs that is not nil.
func first(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
`
