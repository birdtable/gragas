package ordered_test

import (
	"fmt"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/birdtable/gragas/internal/ordered"
)

// readBoth reads out through the strict reader and through yaml.v3, and
// returns the one key and its value as each read them.
func readBoth(t *testing.T, out string) (ordered.Map, map[string]any) {
	t.Helper()
	got, err := ordered.ParseStrict([]byte(out))
	if err != nil {
		t.Fatalf("ParseStrict refused\n%s\n%v", out, err)
	}
	m, ok := got.(*ordered.Map)
	if !ok {
		t.Fatalf("read back %T, want a map\n%s", got, out)
	}
	var v3 map[string]any
	if err := yaml.Unmarshal([]byte(out), &v3); err != nil {
		t.Fatalf("yaml.v3 refused\n%s\n%v", out, err)
	}
	return *m, v3
}

// TestYAMLQuotesWhatYAMLCannotHoldRaw: DEL, each C1 control (NEL among them),
// LS, PS, the byte order mark and the two top non-characters, in a key and in a
// value, read back as the same string through both readers.
func TestYAMLQuotesWhatYAMLCannotHoldRaw(t *testing.T) {
	t.Parallel()
	runes := []rune{0x7f, 0x85, 0x2028, 0x2029, 0xfeff, 0xfffe, 0xffff, 0x00, 0x1b}
	for r := rune(0x80); r <= 0x9f; r++ {
		runes = append(runes, r)
	}
	for _, r := range runes {
		s := "a" + string(r) + "b"
		name := fmt.Sprintf("U+%04X", r)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, shape := range []string{"value", "key"} {
				doc := ordered.NewMap(0)
				if shape == "value" {
					doc.Set("k", s, 0)
				} else {
					doc.Set(s, "v", 0)
				}
				out := ordered.YAML(doc)
				for _, raw := range []rune{r} {
					if raw >= 0x20 && strings.ContainsRune(out, raw) {
						t.Fatalf("%s: the output holds U+%04X raw:\n%q", shape, raw, out)
					}
				}
				m, v3 := readBoth(t, out)
				if ordered.JSON(&m, -1) != ordered.JSON(doc, -1) {
					t.Fatalf("%s: ParseStrict read %s, wrote %s", shape, ordered.JSON(&m, -1), ordered.JSON(doc, -1))
				}
				want := map[string]any{"k": s}
				if shape == "key" {
					want = map[string]any{s: "v"}
				}
				if fmt.Sprint(v3) != fmt.Sprint(want) {
					t.Fatalf("%s: yaml.v3 read %q, want %q", shape, v3, want)
				}
			}
		})
	}
}

// TestYAMLWritesALongKeyInTheExplicitForm: a key over 1024 characters is not
// an implicit key; it is written `? key` then `: value`, a scalar or a block
// value, at the top and nested, and both readers give it back.
// boundary: maxImplicitKey (1024 stays implicit, 1025 goes explicit)
func TestYAMLWritesALongKeyInTheExplicitForm(t *testing.T) {
	t.Parallel()
	for _, n := range []int{1024, 1025, 1100} {
		key := strings.Repeat("k", n)
		inner := ordered.NewMap(0)
		inner.Set(key, "x", 0)
		inner.Set("short", int64(1), 0)
		doc := ordered.NewMap(0)
		doc.Set(key, "scalar", 0)
		doc.Set("nested", inner, 0)
		other := key[:n-1] + "b"
		doc.Set(other, []any{"p", "q"}, 0)
		out := ordered.YAML(doc)
		if explicit := strings.Contains(out, "? "); explicit != (n > 1024) {
			t.Fatalf("%d-character key: explicit form %v\n%.200s", n, explicit, out)
		}
		m, v3 := readBoth(t, out)
		if ordered.JSON(&m, -1) != ordered.JSON(doc, -1) {
			t.Fatalf("%d-character key: ParseStrict read %.200s", n, ordered.JSON(&m, -1))
		}
		if v3[key] != "scalar" || len(v3[other].([]any)) != 2 {
			t.Fatalf("%d-character key: yaml.v3 read the wrong document", n)
		}
	}
}

// TestYAMLCountsALongKeyAsWritten: the 1024 limit is on the key as written, so
// a key short in runes but long once quoted or escaped is written explicit too:
// a leading quote, a colon-and-space, and a run of NELs, each at the edge where
// the written key is 1024 and 1025 characters.
// boundary: maxImplicitKey (the written length, not the rune count)
func TestYAMLCountsALongKeyAsWritten(t *testing.T) {
	t.Parallel()
	for name, mk := range map[string]func(n int) (string, int){
		// the written key is the quoted form: the quote pair, plus one extra
		// character for the escaped quote.
		"quote": func(n int) (string, int) { return `"` + strings.Repeat("k", n), n + 4 },
		"colon": func(n int) (string, int) { return "a: " + strings.Repeat("k", n), n + 5 },
		"nel":   func(n int) (string, int) { return strings.Repeat("\u0085", n), 2*n + 2 },
	} {
		for _, written := range []int{1024, 1025} {
			// find n whose written length is the one asked for
			n := 1
			for {
				_, w := mk(n)
				if w >= written || n > 2000 {
					break
				}
				n++
			}
			key, w := mk(n)
			if w != written {
				continue // this shape cannot reach that exact length
			}
			doc := ordered.NewMap(0)
			doc.Set(key, "v", 0)
			out := ordered.YAML(doc)
			if explicit := strings.HasPrefix(out, "? "); explicit != (written > 1024) {
				t.Fatalf("%s, written %d: explicit form %v", name, written, explicit)
			}
			m, v3 := readBoth(t, out)
			if ordered.JSON(&m, -1) != ordered.JSON(doc, -1) || v3[key] != "v" {
				t.Fatalf("%s, written %d: not read back", name, written)
			}
		}
	}
	for name, key := range map[string]string{
		"quote": `"` + strings.Repeat("k", 1022),
		"colon": "a: " + strings.Repeat("k", 1020),
		"nel":   strings.Repeat("\u0085", 600),
	} {
		doc := ordered.NewMap(0)
		doc.Set(key, "v", 0)
		out := ordered.YAML(doc)
		if !strings.HasPrefix(out, "? ") {
			t.Fatalf("%s: a key over 1024 as written is implicit", name)
		}
		m, v3 := readBoth(t, out)
		if ordered.JSON(&m, -1) != ordered.JSON(doc, -1) || v3[key] != "v" {
			t.Fatalf("%s: not read back", name)
		}
	}
}
