package audit_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// formed is a linter's count with each loosening written as unless the
// tool's default: a word, a list, a
// boolean, an integer and a free string. The test hands it in, and no
// package names the tool's keys.
func formed() []audit.Counting {
	return []audit.Counting{{
		Configs: []string{".golangci.yml"},
		Loosening: []audit.Loosening{
			{At: []string{"linters", "exclusions", "paths-except"}, Unless: new("[]")},
			{At: []string{"linters", "exclusions", "generated"}, Unless: new("strict")},
			{At: []string{"run", "tests"}, Unless: new("true")},
			{At: []string{"run", "issues-exit-code"}, Unless: new("1")},
			{At: []string{"issues", "new"}, Unless: new("false")},
			{At: []string{"issues", "new-from-rev"}, Unless: new("")},
		},
	}}
}

func formedRows(text string) []audit.Counted {
	return audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: text}, formed())
}

func texts(rows []audit.Counted) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Text)
	}
	slices.Sort(out)
	return out
}

// TestEachFormOfLooseningCountsByTheValuesText: with unless,
// a value is one row when it is there and anything but the default, a value
// with no text (a float, a mapping) too, as Repr writes it; a value's text is
// a string as it is, a boolean as true or false, an integer in decimal and a
// list in [ ]. The tool decodes through viper with weak typing, so each
// spelling it casts to a loosening is a row, and a
// tighter value is one too, the safe side. At its default, absent, or null, a
// value is no row and no error.
// Risk: a setting that drops findings, written in a spelling the tool's
// reader casts (0, "f", "0x0", 1, Strict), passes the count unseen.
func TestEachFormOfLooseningCountsByTheValuesText(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		yml  string
		want []string
	}{
		"tests off":                       {"run:\n  tests: false\n", []string{"run.tests: false"}},
		"tests off, as 0":                 {"run:\n  tests: 0\n", []string{"run.tests: 0"}},
		"tests off, as f":                 {"run:\n  tests: \"f\"\n", []string{"run.tests: f"}},
		"exit code 0, as false":           {"run:\n  issues-exit-code: false\n", []string{"run.issues-exit-code: false"}},
		"exit code 0, as 0x0":             {"run:\n  issues-exit-code: \"0x0\"\n", []string{"run.issues-exit-code: 0x0"}},
		"only new issues, as 1":           {"issues:\n  new: 1\n", []string{"issues.new: 1"}},
		"lax, as Strict":                  {"linters:\n  exclusions:\n    generated: Strict\n", []string{"linters.exclusions.generated: Strict"}},
		"lax":                             {"linters:\n  exclusions:\n    generated: lax\n", []string{"linters.exclusions.generated: lax"}},
		"strict, the default":             {"linters:\n  exclusions:\n    generated: strict\n", nil},
		"disable, tighter, counted":       {"linters:\n  exclusions:\n    generated: disable\n", []string{"linters.exclusions.generated: disable"}},
		"tests off, as a string":          {"run:\n  tests: \"false\"\n", []string{"run.tests: false"}},
		"tests on, the default":           {"run:\n  tests: true\n", nil},
		"exit code 0":                     {"run:\n  issues-exit-code: 0\n", []string{"run.issues-exit-code: 0"}},
		"exit code 1, the default":        {"run:\n  issues-exit-code: 1\n", nil},
		"exit code 2, tighter, counted":   {"run:\n  issues-exit-code: 2\n", []string{"run.issues-exit-code: 2"}},
		"only new issues":                 {"issues:\n  new: true\n", []string{"issues.new: true"}},
		"all issues, the default":         {"issues:\n  new: false\n", nil},
		"a revision":                      {"issues:\n  new-from-rev: HEAD~1\n", []string{"issues.new-from-rev: HEAD~1"}},
		"no revision, the default":        {"issues:\n  new-from-rev: \"\"\n", nil},
		"a keep-list":                     {"linters:\n  exclusions:\n    paths-except: [lib/bad.go, cmd/x.go]\n", []string{"linters.exclusions.paths-except: [\"lib/bad.go\", \"cmd/x.go\"]"}},
		"an empty keep-list, the default": {"linters:\n  exclusions:\n    paths-except: []\n", nil},
		"all absent":                      {"version: \"2\"\n", nil},
		"a null":                          {"run:\n  tests: null\n", nil},
		"a float":                         {"run:\n  issues-exit-code: 0.0\n", []string{"run.issues-exit-code: 0.0"}},
		"a mapping":                       {"issues:\n  new-from-rev: {x: 1}\n", []string{"issues.new-from-rev: {'x': 1}"}},
		"a list with a mapping in it":     {"linters:\n  exclusions:\n    paths-except: [a.go, {x: 1}]\n", []string{"linters.exclusions.paths-except: ['a.go', {'x': 1}]"}},
	} {
		if got := texts(formedRows(tc.yml)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: rows %q, want %q", name, got, tc.want)
		}
	}
}

// TestAKeepListIsAFindingWhenItChangesAndAnItemAddedNeverLowersTheCount:
// paths-except keeps only the files it names, so the list is one row, its whole value, of the loosening kind, which
// is held strictly. An item taken off drops more files and is a finding at an
// unchanged total (1 against 1); an item added keeps more files, and the
// count never falls for it (one row before, one after), though the changed
// value is a finding too, the safe side; emptying the list lowers the count
// and is none; a revision moved is a finding at an unchanged total.
// Risk: an item removed from the keep-list lowers the count, or leaves it
// where it was, so a loosening passes as a tightening, as the count of each
// item did before.
func TestAKeepListIsAFindingWhenItChangesAndAnItemAddedNeverLowersTheCount(t *testing.T) {
	t.Parallel()
	keep := func(items string) []audit.Counted {
		return formedRows("linters:\n  exclusions:\n    paths-except: [" + items + "]\n")
	}
	two, one, three := keep("a.go, b.go"), keep("a.go"), keep("a.go, b.go, c.go")
	if got := audit.CheckCounts(rules(), two, one); len(got) != 1 || !strings.Contains(got[0].Missing, "is a new loosening (1 against 1 at the merge base): linters.exclusions.paths-except: [\"a.go\"]") {
		t.Errorf("an item taken off: %+v, want one new loosening at an unchanged total, naming the shorter list", got)
	}
	if len(three) != len(two) || len(three) != 1 {
		t.Errorf("an item added: %d rows against %d, want one before and one after", len(three), len(two))
	}
	if got := audit.CheckCounts(rules(), two, three); len(got) != 1 {
		t.Errorf("an item added: %+v, want the changed value a finding, the safe side", got)
	}
	if got := audit.CheckCounts(rules(), two, keep("")); len(got) != 0 {
		t.Errorf("the list emptied: %+v, want none", got)
	}
	rev := func(r string) []audit.Counted { return formedRows("issues:\n  new-from-rev: " + r + "\n") }
	if got := audit.CheckCounts(rules(), rev("v1.0.0"), rev("v2.0.0")); len(got) != 1 || !strings.Contains(got[0].Missing, "is a new loosening (1 against 1 at the merge base): issues.new-from-rev: v2.0.0") {
		t.Errorf("a revision moved: %+v, want one new loosening at an unchanged total, naming it", got)
	}
	if got := audit.CheckCounts(rules(), rev("v1.0.0"), rev("v1.0.0")); len(got) != 0 {
		t.Errorf("the same revision: %+v, want none", got)
	}
}
