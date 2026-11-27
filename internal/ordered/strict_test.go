package ordered_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// TestParseStrictRefusesEveryFormThatStandsForAnotherPlace is the refusal
// table of ParseStrict: each case is refused as a SyntaxError naming its line
// and its form, where ParseYAML reads the same text.
func TestParseStrictRefusesEveryFormThatStandsForAnotherPlace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
		yamlReads      bool
	}{
		{"an alias of no anchor", "a: x\nb:\n  - *a\n", "unknown anchor", false},
		{"an anchor", "a: &base\n  b: 1\n", "line 1: the anchor &base", true},
		{"an anchor on a scalar", "a: [1, &n 2]\n", "line 1: the anchor &n", true},
		{"an anchor and its alias", "a: &x [1]\nb: *x\n", "line 1: the anchor &x", true},
		{"a merge key", "a:\n  <<: {b: 1}\n", "line 2: merge keys (<<) are not supported", false},
		{"a core tag", "a: !!str 12\n", "line 1: the tag !!str is not read here", true},
		{"a local tag", "a: !env HOME\n", "line 1: the tag !env is not read here", false},
		{"a tag on a mapping", "a: !thing {b: 1}\n", "line 1: the tag !thing", true},
		{"a key written twice", "a: 1\nb:\n  c: 1\n  c: 2\n", "line 4: the key c is written again, first at line 3", true},
		{"two documents", "a: 1\n---\nb: 2\n", "expected a single document", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ordered.ParseStrict([]byte(tc.in))
			var se *ordered.SyntaxError
			if !errors.As(err, &se) || !errors.Is(err, ordered.ErrSyntax) {
				t.Fatalf("ParseStrict error = %v, want a SyntaxError", err)
			}
			if !strings.Contains(se.Detail, tc.want) {
				t.Errorf("detail = %q, want it to hold %q", se.Detail, tc.want)
			}
			var line int
			if _, err := fmt.Sscanf(tc.want, "line %d:", &line); err == nil && se.Line != line {
				t.Errorf("Line = %d, want %d", se.Line, line)
			}
			if _, err := ordered.ParseYAML([]byte(tc.in)); (err == nil) != tc.yamlReads {
				t.Errorf("ParseYAML error = %v; the table says ParseYAML reads it: %v", err, tc.yamlReads)
			}
		})
	}
}

// TestParseStrictReadsPlainDataAsParseYAMLDoes is the clean control: a
// document with none of the refused forms reads to the same value, the same
// YAML 1.1 scalars and the same lines.
func TestParseStrictReadsPlainDataAsParseYAMLDoes(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"",
		"a: 1\nb: [x, 'y', \"z\"]\nc:\n  d: yes\n  e: 2026-01-02\n",
		"- one\n- {two: 2}\n- |\n  three\n",
		"argv: ['{config}', --flag, \"&not-an-anchor\", '*not-an-alias', '!not-a-tag']\n",
	} {
		got, err := ordered.ParseStrict([]byte(in))
		if err != nil {
			t.Fatalf("ParseStrict(%q): %v", in, err)
		}
		want, err := ordered.ParseYAML([]byte(in))
		if err != nil {
			t.Fatalf("ParseYAML(%q): %v", in, err)
		}
		if g, w := ordered.JSON(got, 1), ordered.JSON(want, 1); g != w {
			t.Errorf("ParseStrict(%q) = %s, ParseYAML = %s", in, g, w)
		}
		if m, ok := got.(*ordered.Map); ok && m.Sub("c") != nil && m.Sub("c").KeyLine("e") != 5 {
			t.Errorf("the line of c.e = %d, want 5", m.Sub("c").KeyLine("e"))
		}
	}
}

// TestParseStrictRefusesAnAliasBombBeforeItGrows: nine levels of ten aliases
// each, a few hundred bytes that ParseYAML would grow to a billion values, is
// refused at the first alias, in the time it takes to read the text.
func TestParseStrictRefusesAnAliasBombBeforeItGrows(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&b, "a%d: &a%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*a%d, ", i-1), 10), ", "))
	}
	_, err := ordered.ParseStrict([]byte(b.String()))
	if !errors.Is(err, ordered.ErrSyntax) || !strings.Contains(err.Error(), "line 1: the anchor &a0") {
		t.Errorf("ParseStrict error = %v, want the anchor at line 1 refused", err)
	}
}

// FuzzParseStrict: whatever ParseStrict reads, ParseYAML reads to the same
// value, and it encodes to valid JSON; what it refuses is a SyntaxError.
func FuzzParseStrict(f *testing.F) {
	for _, seed := range []string{
		"format: 3\ntool: lint-a\nrun:\n  argv: ['{fixtures}']\n",
		"a: &x [1, 2]\nb: *x\n",
		"a: !!str 1\n",
		"a: 1\na: 2\n",
		"a:\n  <<: {b: 1}\n",
		"- - - [\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := ordered.ParseStrict(data)
		if err != nil {
			if !errors.Is(err, ordered.ErrSyntax) {
				t.Fatalf("ParseStrict error %v is not a SyntaxError", err)
			}
			return
		}
		loose, err := ordered.ParseYAML(data)
		if err != nil {
			t.Fatalf("ParseStrict read what ParseYAML refuses: %v", err)
		}
		if g, w := ordered.JSON(v, -1), ordered.JSON(loose, -1); g != w {
			t.Fatalf("ParseStrict = %s, ParseYAML = %s", g, w)
		}
		if _, err := json.Marshal(ordered.Plain(v)); err != nil {
			t.Fatalf("Plain does not marshal: %v", err)
		}
	})
}
