package wire

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/readglob"
)

// rules holds what a schema cannot say, as functions that answer why not, or
// "" when the thing is as it should be. A caller puts the answer at a place.
type rules struct {
	names port.FileNames
}

// pathFault says why p is not a path of the contract: relative, slash-separated
// and every part one file's name on every system. It is asked a part at a time,
// as port.FileNames is, so an empty part (an empty path, a leading, a
// trailing or a doubled slash), "." and "..", a backslash and a control
// character are each refused by the port's table.
func (r rules) pathFault(p string) string {
	for part := range strings.SplitSeq(p, "/") {
		if _, err := r.names.Portable(part); err != nil {
			return fmt.Sprintf("is not a path: %q is refused: %v", part, err)
		}
	}
	return ""
}

// globFault says why g is not a glob of the contract: a path whose parts may
// hold * and ? and which may be ** and nothing else. The grammar is
// internal/readglob (Parts), the one every reader of a glob shares; what is
// asked here is the port's table, that each part could be one file's name.
func (r rules) globFault(g string) string {
	parts, err := readglob.Parts(g)
	if err != nil {
		return err.Error()
	}
	for _, part := range parts {
		sample := strings.NewReplacer("**", "x", "*", "x", "?", "x").Replace(part)
		if _, err := r.names.Portable(sample); err != nil {
			return fmt.Sprintf("is not a glob: %q is refused: %v", part, err)
		}
	}
	return ""
}

// wordFault says why w is not a word of an argv: it is literal, except the
// words {scratch} and {null} and one that starts with {scratch}/; any other { is
// refused.
func wordFault(w string) string {
	rest := w
	switch {
	case w == "{scratch}" || w == "{null}":
		return ""
	case strings.HasPrefix(w, "{scratch}/"):
		rest = strings.TrimPrefix(w, "{scratch}/")
	}
	if strings.Contains(rest, "{") {
		return "has a { that is not {scratch} or {null}"
	}
	return ""
}

// depth is the levels of arrays and objects v nests, counting its own.
func depth(v canon.Value) int {
	deepest := 0
	switch x := v.(type) {
	case canon.Array:
		for _, e := range x {
			deepest = max(deepest, depth(e))
		}
		return deepest + 1
	case canon.Object:
		for _, e := range x {
			deepest = max(deepest, depth(e))
		}
		return deepest + 1
	}
	return 0
}

// valueFault says why v, an opaque value, is over its caps: 32 levels, and 64
// KiB in canonical form. A nil v is no value and is as it should be.
func valueFault(v canon.Value) string {
	if v == nil {
		return ""
	}
	if d := depth(v); d > MaxValueDepth {
		return fmt.Sprintf("nests %d levels, more than %d", d, MaxValueDepth)
	}
	b, err := Canonical(v)
	if err != nil {
		return "has no canonical form: " + err.Error()
	}
	if len(b) > MaxValue {
		return fmt.Sprintf("is %d bytes in canonical form, more than %d", len(b), MaxValue)
	}
	return ""
}

// sizeOf is the bytes b holds, or why it holds none: base64 is the standard
// padded alphabet and nothing else: no line end, which the decoder would skip,
// and no bits after the last byte that are not zero, which it would take.
func sizeOf(b plugin.Bytes) (int, string) {
	if b.Base64 == nil {
		if b.Text == nil {
			return 0, ""
		}
		return len(*b.Text), ""
	}
	raw, err := decodeBase64(*b.Base64)
	if err != nil {
		return 0, "is not base64: " + err.Error()
	}
	return len(raw), ""
}

// decodeBase64 is s as bytes, in the one spelling of the contract: the strict
// standard padded form, with no CR or LF anywhere.
func decodeBase64(s string) ([]byte, error) {
	if strings.ContainsAny(s, "\r\n") {
		return nil, errors.New("has a line end")
	}
	return base64.StdEncoding.Strict().DecodeString(s)
}

// bytesFault says why b is over cap bytes, or not base64.
func bytesFault(b plugin.Bytes, cap int) string {
	n, why := sizeOf(b)
	if why != "" {
		return why
	}
	if n > cap {
		return fmt.Sprintf("is %d bytes, more than %d", n, cap)
	}
	return ""
}

// bytesForm is bytesFault and the form of the contract: text is valid UTF-8,
// and base64 is for bytes that are not.
func bytesForm(b plugin.Bytes, cap int) string {
	if why := bytesFault(b, cap); why != "" {
		return why
	}
	if b.Text != nil && !utf8.ValidString(*b.Text) {
		return "has text that is not valid UTF-8; such bytes go in base64"
	}
	if b.Base64 != nil {
		if raw, err := decodeBase64(*b.Base64); err == nil && utf8.Valid(raw) {
			return "has base64 of bytes that are valid UTF-8; they go in text"
		}
	}
	return ""
}

// pointerFault says why p is not a JSON Pointer (RFC 6901): empty, or each
// part after a slash, with ~ only as ~0 and ~1.
func pointerFault(p string) string {
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		return "is not a JSON Pointer: it does not start with /"
	}
	for i := 0; i < len(p); i++ {
		if p[i] == '~' && (i+1 == len(p) || (p[i+1] != '0' && p[i+1] != '1')) {
			return "is not a JSON Pointer: ~ is neither ~0 nor ~1"
		}
	}
	return ""
}
