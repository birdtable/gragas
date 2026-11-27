package ordered

import (
	"strconv"
	"strings"
)

// Pointer walks a JSON pointer (RFC 6901) over a parsed document, a value
// ParseStrict or ParseYAML answers: the value p points at, and whether there
// is one. The empty pointer is the whole document; an index into a list is a
// whole number written as itself, with no zero before it. The adapter reader
// and host's step runner, which reads a script grant's script_at,
// walk a document by this one reading.
func Pointer(v any, p string) (any, bool) {
	if p == "" {
		return v, true
	}
	if !strings.HasPrefix(p, "/") {
		return nil, false
	}
	for step := range strings.SplitSeq(p[1:], "/") {
		step = strings.ReplaceAll(strings.ReplaceAll(step, "~1", "/"), "~0", "~")
		switch c := v.(type) {
		case *Map:
			next, ok := c.Get(step)
			if !ok {
				return nil, false
			}
			v = next
		case []any:
			i, err := strconv.Atoi(step)
			if err != nil || i < 0 || i >= len(c) || strconv.Itoa(i) != step {
				return nil, false
			}
			v = c[i]
		default:
			return nil, false
		}
	}
	return v, true
}
