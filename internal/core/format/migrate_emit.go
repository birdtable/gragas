package format

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// yamlWords are the plain words YAML reads as something other than a string.
func yamlWords() map[string]bool {
	return map[string]bool{"true": true, "false": true, "null": true, "yes": true, "no": true, "on": true, "off": true, "y": true, "n": true}
}

// plainRE is a string that reads back as itself when written without quotes.
var plainRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _./()+-]*$`)

// migrateScalar is a string as a YAML scalar: plain when it reads back as itself,
// else a double-quoted one, which a JSON string always is.
func migrateScalar(s string) string {
	if plainRE.MatchString(s) && !strings.HasSuffix(s, " ") && !yamlWords()[strings.ToLower(s)] {
		return s
	}
	return ordered.JSON(s, 0)
}

// emitYAML writes v, a map made of *ordered.Map, []any, strings, ints and bools,
// as block YAML, one entry per line, in the order the maps hold.
func emitYAML(b *strings.Builder, v any, indent int) {
	pad := strings.Repeat("  ", indent)
	switch x := v.(type) {
	case *ordered.Map:
		if x.Len() == 0 {
			b.WriteString(pad + "{}\n")
			return
		}
		for _, k := range x.Keys() {
			emitEntry(b, pad, k+":", x.Val(k), indent, false)
		}
	case []any:
		for _, it := range x {
			emitItem(b, pad, it, indent)
		}
	}
}

// emitEntry writes "key: value", or the key and its block below it.
func emitEntry(b *strings.Builder, pad, head string, v any, indent int, inItem bool) {
	switch x := v.(type) {
	case *ordered.Map:
		if x.Len() == 0 {
			b.WriteString(pad + head + " {}\n")
			return
		}
		b.WriteString(pad + head + "\n")
		emitYAML(b, x, indent+1)
	case []any:
		if len(x) == 0 {
			b.WriteString(pad + head + " []\n")
			return
		}
		b.WriteString(pad + head + "\n")
		emitYAML(b, x, indent+1)
	default:
		b.WriteString(pad + head + " " + scalarOf(v) + "\n")
	}
}

// emitItem writes one "- " entry of a list at indent.
func emitItem(b *strings.Builder, pad string, v any, indent int) {
	m, ok := v.(*ordered.Map)
	if !ok {
		if l, isList := v.([]any); isList && len(l) > 0 {
			b.WriteString(pad + "-\n")
			emitYAML(b, l, indent+1)
			return
		}
		b.WriteString(pad + "- " + scalarOf(v) + "\n")
		return
	}
	if m.Len() == 0 {
		b.WriteString(pad + "- {}\n")
		return
	}
	var inner strings.Builder
	emitYAML(&inner, m, indent+1)
	lines := strings.SplitAfter(inner.String(), "\n")
	childPad := strings.Repeat("  ", indent+1)
	b.WriteString(pad + "- " + strings.TrimPrefix(lines[0], childPad))
	for _, l := range lines[1:] {
		b.WriteString(l)
	}
}

func scalarOf(v any) string {
	switch x := v.(type) {
	case string:
		return migrateScalar(x)
	case int:
		return fmt.Sprintf("%d", x)
	case bool:
		return fmt.Sprintf("%t", x)
	case nil:
		return "null"
	}
	return migrateScalar(fmt.Sprint(v))
}

// newDoc is an empty ordered map to fill key by key.
func newDoc() *ordered.Map { return ordered.NewMap(0) }

// put sets key on m to v.
func put(m *ordered.Map, key string, v any) { m.Set(key, v, 0) }
