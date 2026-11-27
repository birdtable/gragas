package audit_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// folding is a linter's count whose reader takes keys without their case
// or, with fold false, one that reads them as written. The
// test hands it in, and no package names the tool's keys.
func folding(fold bool) []audit.Counting {
	return []audit.Counting{{
		Configs:    []string{".golangci.yml"},
		Exclusions: [][]string{{"linters", "exclusions", "rules"}},
		Loosening:  []audit.Loosening{{At: []string{"run", "tests"}, Unless: new("true")}, {At: []string{"issues", "new"}, Unless: new("false")}},
		FoldKeys:   fold,
	}}
}

func foldRows(fold bool, text string) []string {
	var out []string
	for _, c := range audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: text}, folding(fold)) {
		out = append(out, c.Kind+": "+c.Text)
	}
	slices.Sort(out)
	return out
}

// TestAKeyInAnyCaseIsReadWhereTheToolReadsKeysWithoutCase: viper lower-cases every key the tool reads, so
// with FoldKeys each step of a path matches a key in any case, and a
// capitalised exclusion list or loosening is counted as the plain one is.
// Without FoldKeys, for a tool that reads keys as written, it is not.
// Risk: Run: {Tests: false} drops every test file and the count reads no
// run.tests, so the loosening passes unseen.
func TestAKeyInAnyCaseIsReadWhereTheToolReadsKeysWithoutCase(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		yml  string
		want []string
	}{
		"Run Tests":            {"Run:\n  Tests: false\n", []string{"loosening: run.tests: false"}},
		"issues New":           {"issues:\n  New: true\n", []string{"loosening: issues.new: true"}},
		"LINTERS rules":        {"LINTERS:\n  exclusions:\n    Rules:\n      - {path: x, linters: [gosec]}\n", []string{"linter exclusion: {'path': 'x', 'linters': ['gosec']}"}},
		"plain, the control":   {"run:\n  tests: false\n", []string{"loosening: run.tests: false"}},
		"a key that is no key": {"runs:\n  tests: false\n", nil},
	} {
		if got := foldRows(true, tc.yml); !slices.Equal(got, tc.want) {
			t.Errorf("%s, folded: %q, want %q", name, got, tc.want)
		}
	}
	if got := foldRows(false, "Run:\n  Tests: false\n"); len(got) != 0 {
		t.Errorf("Run Tests, as written: %q, want none", got)
	}
}

// TestTwoKeysThatDifferOnlyInCaseAreALooseningRow: a reader
// that lower-cases keys takes two keys of one mapping that differ only in
// case as one key, so what either holds may stand in for the other. With
// FoldKeys each such pair, at any depth, is a loosening row naming the folded
// path and both keys, held strictly against the merge base; without it, none.
func TestTwoKeysThatDifferOnlyInCaseAreALooseningRow(t *testing.T) {
	t.Parallel()
	twin := "run:\n  tests: true\nRun:\n  issues-exit-code: 1\n"
	got := foldRows(true, twin)
	if !slices.Contains(got, "loosening: run: keys of one name to the tool, run and Run") {
		t.Errorf("a twin at the root: %q, want the twin named", got)
	}
	deep := foldRows(true, "linters:\n  exclusions:\n    rules: []\n    Rules: []\n")
	if !slices.Contains(deep, "loosening: linters.exclusions.rules: keys of one name to the tool, rules and Rules") {
		t.Errorf("a twin below: %q, want it named at its folded path", deep)
	}
	if got := foldRows(false, twin); len(got) != 0 {
		t.Errorf("a twin, read as written: %q, want none", got)
	}
	rows := audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: twin}, folding(true))
	if found := audit.CheckCounts(rules(), nil, rows); len(found) == 0 || !strings.Contains(found[0].Missing, "is a new loosening") {
		t.Errorf("a twin against a base without it: %+v, want a new loosening", found)
	}
}

// TestEveryCaseTwinsSubtreeIsRead: viper keeps one
// of two case twins by a map's range, so either may be the one the tool
// reads; the walk follows every key that folds to a step, so what the second
// twin holds is counted even once the twin is at the merge base.
// Risk: with the twin at base, Run: {tests: false} or Linters.exclusions.rules
// under the second twin passes with no finding.
func TestEveryCaseTwinsSubtreeIsRead(t *testing.T) {
	t.Parallel()
	tests := foldRows(true, "run:\n  tests: true\nRun:\n  tests: false\n")
	if !slices.Contains(tests, "loosening: run.tests: false") {
		t.Errorf("Run: {tests: false} under the second twin: %q, want it counted", tests)
	}
	rules := foldRows(true, "linters:\n  settings: {}\nLinters:\n  exclusions:\n    rules:\n      - {path: x}\n")
	if !slices.Contains(rules, "linter exclusion: {'path': 'x'}") {
		t.Errorf("Linters.exclusions.rules under the second twin: %q, want it counted", rules)
	}
}
