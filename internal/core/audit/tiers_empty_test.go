package audit

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestAMemberThatReadsTheCountThroughTestsCountEndsAnEmptyRunRed: gotestsum alone is green on a
// package with no tests, so the tier needs a member that reads the count and exits 1. gragas-dev tests
// count is one: it reads the member's gotestsum json file and is red when no test passed in it, and it
// prints DONE 0 tests. The audit knows it by the words `tests count`, which the tier table lists beside
// the two words a shell grep carries. Planted: gotestsum with no reader is a finding, and so is a call
// that names the file and no count.
func TestAMemberThatReadsTheCountThroughTestsCountEndsAnEmptyRunRed(t *testing.T) {
	t.Parallel()
	empty := func(member string) bool {
		repo := fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\ntasks:\n  pre-push:\n    cmds:\n" + member)}}
		in := TierInput{
			Repo:    repo,
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{
				Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "tests", Rule: "t.3", All: []string{"gotestsum"}}}}},
				Empty: []string{"EMPTY", "DONE 0 tests", "tests count"},
				Rules: TierRules{Tier: "t.1", Runs: "t.2", Empty: "t.4"},
			},
		}
		for _, f := range CheckTiers(in).Findings {
			if f.Rule == "t.4" && strings.Contains(f.Missing, "count line") {
				return true
			}
		}
		return false
	}
	const run = "      - |\n        gotestsum --jsonfile \"$j\" -- ./... || exit 1\n"
	for name, c := range map[string]struct {
		member string
		red    bool
	}{
		"gotestsum alone":             {run, true},
		"a count reader after it":     {run + "        {{.GATE}} tests count \"$j\" test\n", false},
		"the grep that reads a count": {run + "        grep -q \"DONE 0 tests\" out.txt && exit 1 || true\n", false},
		"a call that names no count":  {run + "        {{.GATE}} tests packages \"$j\"\n", true},
	} {
		if got := empty(c.member); got != c.red {
			t.Errorf("%s: an empty-run finding = %v, want %v", name, got, c.red)
		}
	}
}

// TestTheEmptyRuleReadsGotestsumAmongTheAnyOfTheTestsMember: the tests member is
// any: [gotestsum, "hatch tests"], and a tier whose member runs gotestsum with no count reader is
// still red, as with all: [gotestsum]. hatch tests judges an empty run itself (--red-when none-ran),
// so a tier that holds the member through it alone is asked for no reader. The first case is the
// plant: the rule once read All alone, and was silent on it.
func TestTheEmptyRuleReadsGotestsumAmongTheAnyOfTheTestsMember(t *testing.T) {
	t.Parallel()
	empty := func(member string) bool {
		repo := fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\ntasks:\n  pre-push:\n    cmds:\n" + member)}}
		in := TierInput{
			Repo:    repo,
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{
				Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "tests", Rule: "t.3", Any: []string{"gotestsum", "hatch tests"}}}}},
				Empty: []string{"EMPTY", "DONE 0 tests", "tests count"},
				Rules: TierRules{Tier: "t.1", Runs: "t.2", Empty: "t.4"},
			},
		}
		for _, f := range CheckTiers(in).Findings {
			if f.Rule == "t.4" && strings.Contains(f.Missing, "count line") {
				return true
			}
		}
		return false
	}
	const gotestsum = "      - |\n        gotestsum --jsonfile \"$j\" -- ./... || exit 1\n"
	const hatch = "      - |\n        hatch tests --timeout 12m --red-when failed,none-ran --green-word done --red-word failed -- ./...\n"
	for name, c := range map[string]struct {
		member string
		red    bool
	}{
		"gotestsum alone, held by any":  {gotestsum, true},
		"gotestsum with a count reader": {gotestsum + "        grep -q \"DONE 0 tests\" out.txt && exit 1 || true\n", false},
		"hatch tests alone":             {hatch, false},
		"both, gotestsum unread":        {hatch + gotestsum, true},
	} {
		if got := empty(c.member); got != c.red {
			t.Errorf("%s: an empty-run finding = %v, want %v", name, got, c.red)
		}
	}
}
