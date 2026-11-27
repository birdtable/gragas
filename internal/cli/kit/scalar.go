package kit

import (
	"fmt"
	"strconv"
	"strings"
)

// Scalar reads the text of a value flag as the YAML scalar it spells: true and
// false are booleans, a whole number is a number, anything else is the text.
func Scalar(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return s
}

// Options reads the texts of an option flag, each spelled name=value, as the
// options of a binding: a value reads as Scalar does, and a name given more
// than once holds a list of its values in the order given. A text with no name
// or no equals sign is an error naming it.
func Options(texts []string) (map[string]any, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	for _, t := range texts {
		name, val, ok := strings.Cut(t, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--option %q is not name=value", t)
		}
		v := Scalar(val)
		switch old := out[name].(type) {
		case nil:
			out[name] = v
		case []any:
			out[name] = append(old, v)
		default:
			out[name] = []any{old, v}
		}
	}
	return out, nil
}
