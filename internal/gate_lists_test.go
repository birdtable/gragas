package internal

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// gateListsProblems holds the member gate-lists (gragas#1733): it is a call of
// push-checks, it runs the tests LISTS_RUN names over ./internal, and each
// name is a test the repository defines. checks is the text of push-checks,
// member the text of gate-lists, taskfile the whole file and tests the test
// sources.
func gateListsProblems(checks, member, taskfile, tests string) []string {
	var bad []string
	if !strings.Contains(checks, "\n      - task: gate-lists\n") {
		bad = append(bad, "gate-lists is not a call of push-checks, so the list tests do not run at push")
	}
	if !strings.Contains(member, "-run '{{.LISTS_RUN}}'") || !strings.Contains(member, "./internal") {
		bad = append(bad, "gate-lists does not run LISTS_RUN over ./internal")
	}
	m := regexp.MustCompile(`(?m)^  LISTS_RUN: \^\(([^)]*)\)\$$`).FindStringSubmatch(taskfile)
	if m == nil {
		return append(bad, "the Taskfile has no LISTS_RUN of the form ^(TestA|TestB)$")
	}
	names := strings.Split(m[1], "|")
	for _, want := range []string{"TestTheSmokeListHoldsTheCommandsOfTheTree", "TestEveryClassLeftHasItsCatch", "TestTheGoldensListNamesEveryTestThatReadsAGolden", "TestNoToolIsNamedInGragasGo", "TestTheToolListsDifferByTheLeftOut", "TestTheTreesSuppressionsAreTheListsOwn"} {
		found := false
		for _, n := range names {
			found = found || n == want
		}
		if !found {
			bad = append(bad, fmt.Sprintf("LISTS_RUN does not name %s", want))
		}
	}
	for _, n := range names {
		if !strings.Contains(tests, "func "+n+"(") {
			bad = append(bad, fmt.Sprintf("LISTS_RUN names %s, which no test file defines, so it would run nothing", n))
		}
	}
	return bad
}

// TestGateListsRunsAtPushAndRunsRealTests holds the real Taskfile to
// gateListsProblems and plants each way it goes wrong: the call dropped from
// push-checks, the run flag changed, a name that is no test, a required list
// test dropped. Risk: a member that is dropped from the push, or runs a
// pattern that matches nothing, and the lists are held again only at landing.
func TestGateListsRunsAtPushAndRunsRealTests(t *testing.T) {
	t.Parallel()
	checks, member, taskfile, tests := taskSection(t, "push-checks"), taskSection(t, "gate-lists"), read(t, "Taskfile.yml"), testSources(t)
	for _, p := range gateListsProblems(checks, member, taskfile, tests) {
		t.Error(p)
	}
	for name, plant := range map[string][4]string{
		"the call dropped from push-checks": {strings.Replace(checks, "      - task: gate-lists\n", "", 1), member, taskfile, tests},
		"the run flag dropped":              {checks, strings.Replace(member, "-run '{{.LISTS_RUN}}'", "", 1), taskfile, tests},
		"a name that is no test":            {checks, member, strings.Replace(taskfile, "TestEveryClassLeftHasItsCatch|", "TestNoSuchListTest|", 1), tests},
		"a required test dropped":           {checks, member, strings.Replace(taskfile, "TestEveryClassLeftHasItsCatch|", "", 1), tests},
	} {
		if len(gateListsProblems(plant[0], plant[1], plant[2], plant[3])) == 0 {
			t.Errorf("%s is not red", name)
		}
	}
}
