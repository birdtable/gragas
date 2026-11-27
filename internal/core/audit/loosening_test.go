package audit_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// loosened is the count's data of a linter whose presets list and generated
// scalar are counted: the test hands them in, and no package
// names the tool's paths or words.
func loosened() []audit.Counting {
	return []audit.Counting{{
		Configs:    []string{".golangci.yml"},
		Exclusions: [][]string{{"linters", "exclusions", "presets"}},
		Loosening:  []audit.Loosening{{At: []string{"linters", "exclusions", "generated"}, Values: []string{"lax"}}},
	}}
}

func rowsOf(t *testing.T, text string) []string {
	t.Helper()
	var out []string
	for _, c := range audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: text}, loosened()) {
		if want := audit.KindExclusion; strings.Contains(c.Text, ": ") {
			if c.Kind != audit.KindLoosening {
				t.Errorf("row %+v is no loosening", c)
			}
		} else if c.Kind != want {
			t.Errorf("row %+v is no exclusion", c)
		}
		out = append(out, c.Text)
	}
	return out
}

// TestEachPresetAndALaxGeneratedIsOneExclusion: each word of the
// presets list is one exclusion row, and the generated scalar is one only at
// a loosening value; any other value, the key absent, or a value that is no
// string, counts nothing and is no error. A formatter's own generated is not
// at the counted path.
// Risk: a preset, or generated files let through, silences findings and the
// count stays where it was.
func TestEachPresetAndALaxGeneratedIsOneExclusion(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		yml  string
		want []string
	}{
		"two presets":               {"version: \"2\"\nlinters:\n  exclusions:\n    presets: [comments, legacy]\n", []string{"comments", "legacy"}},
		"no preset":                 {"version: \"2\"\nlinters:\n  exclusions:\n    presets: []\n", nil},
		"lax":                       {"version: \"2\"\nlinters:\n  exclusions:\n    generated: lax\n", []string{"linters.exclusions.generated: lax"}},
		"strict, the default":       {"version: \"2\"\nlinters:\n  exclusions:\n    generated: strict\n", nil},
		"disable, tighter":          {"version: \"2\"\nlinters:\n  exclusions:\n    generated: disable\n", nil},
		"generated absent":          {"version: \"2\"\nlinters:\n  exclusions: {}\n", nil},
		"a value that is no string": {"version: \"2\"\nlinters:\n  exclusions:\n    generated: [lax]\n", nil},
		"a mapping at the key":      {"version: \"2\"\nlinters:\n  exclusions:\n    generated: {lax: true}\n", nil},
		"the formatters' generated": {"version: \"2\"\nformatters:\n  exclusions:\n    generated: lax\n", nil},
		"a preset beside lax":       {"version: \"2\"\nlinters:\n  exclusions:\n    generated: lax\n    presets:\n      - std-error-handling\n", []string{"linters.exclusions.generated: lax", "std-error-handling"}},
	} {
		got := rowsOf(t, tc.yml)
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: rows %q, want %q", name, got, tc.want)
		}
	}
}

// TestALaxGeneratedIsANewExclusionAgainstTheMergeBase: strict at the merge
// base and lax at head is a new loosening of the count (it was once counted
// among the exclusions), naming the line; the reverse lowers the count and is
// none.
func TestALaxGeneratedIsANewExclusionAgainstTheMergeBase(t *testing.T) {
	t.Parallel()
	count := func(text string) []audit.Counted {
		return audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: text}, loosened())
	}
	strict := count("version: \"2\"\nlinters:\n  exclusions:\n    generated: strict\n")
	lax := count("version: \"2\"\nlinters:\n  exclusions:\n    generated: lax\n")
	got := audit.CheckCounts(rules(), strict, lax)
	if len(got) != 1 || got[0].Line != 4 || !strings.Contains(got[0].Missing, "is a new loosening (1 against 0 at the merge base): linters.exclusions.generated: lax") {
		t.Errorf("strict to lax: %+v, want one new exclusion at line 4", got)
	}
	if back := audit.CheckCounts(rules(), lax, strict); len(back) != 0 {
		t.Errorf("lax to strict: %+v, want none", back)
	}
}
