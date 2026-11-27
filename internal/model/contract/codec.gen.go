// Written by gragas-generate from internal/dev/generate/gocodec.go; edits are lost, so edit the source and run task gen.

package contract

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
