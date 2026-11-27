package internal

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// standinMemberProblems holds the push member standin (gragas#2041): push-checks calls it,
// it runs the digest tests with -count=1 over ./internal and the stand-in's two modules over ./internal/host/plugin,
// each name its run lists is a test the repository defines and the two runs name the tests the ticket
// asks for, its skip inputs name the stand-in's source, the host and the digest file, and gate/budgets.yaml
// has its row. checks is the text of push-checks, member the text of standin, taskfile the whole file,
// tests the test sources, specs the STANDIN_INPUTS and budgets the text of gate/budgets.yaml.
func standinMemberProblems(checks, member, taskfile, tests string, specs []string, budgets string) []string {
	var bad []string
	if !strings.Contains(checks, "\n      - task: standin\n") {
		bad = append(bad, "standin is not a call of push-checks, so the plugin path is not held at push")
	}
	if !regexp.MustCompile(`-count=1 [^\n]*-run '\{\{\.STANDIN_RUN\}\}' \./internal\n`).MatchString(member) {
		bad = append(bad, "standin does not run STANDIN_RUN over ./internal with -count=1: a cached pass outlives a change of what the stand-in imports and of the toolchain")
	}
	if !regexp.MustCompile(`-run '\{\{\.C24_RUN\}\}' \./internal/host/plugin\n`).MatchString(member) {
		bad = append(bad, "standin does not run C24_RUN over ./internal/host/plugin")
	}
	for name, want := range map[string][]string{
		"STANDIN_RUN": {"TestTheStandinModuleIsTheOneTheTreeHolds", "TestAChangedStandinSourceIsRedAgainstTheHeldDigest"},
		"C24_RUN":     {"TestC24AnEndlessLoopEndsWithinTheTimeLimit", "TestC24AnAllocationLoopEndsAtTheMemoryLimit"},
	} {
		m := regexp.MustCompile(`(?m)^  ` + name + `: \^\(([^)]*)\)\$$`).FindStringSubmatch(taskfile)
		if m == nil {
			bad = append(bad, "the Taskfile has no "+name+" of the form ^(TestA|TestB)$")
			continue
		}
		names := strings.Split(m[1], "|")
		for _, n := range want {
			if !slices.Contains(names, n) {
				bad = append(bad, fmt.Sprintf("%s does not name %s", name, n))
			}
		}
		for _, n := range names {
			if !strings.Contains(tests, "func "+n+"(") {
				bad = append(bad, fmt.Sprintf("%s names %s, which no test file defines, so it would run nothing", name, n))
			}
		}
	}
	if got := uncovered(specs, []string{"internal/dev/standin/answer.go", "internal/host/plugin/host.go", "internal/config/testing/standin.go", "gate/standin.digest", "go.mod", "Taskfile.yml"}); got != nil {
		bad = append(bad, fmt.Sprintf("the skip inputs leave out %v, which the member reads", got))
	}
	if !regexp.MustCompile(`(?m)^  - id: member-standin\n(?:    [a-z_]+: .*\n)*?    limit: [0-9]+\n`).MatchString(budgets) {
		bad = append(bad, "gate/budgets.yaml has no row member-standin with a limit")
	}
	return bad
}

// TestStandinRunsAtPushAndNamesWhatItReads holds the real Taskfile to standinMemberProblems and plants each
// way it goes wrong: the call dropped from push-checks, the cache not bypassed, a name that is no test, the
// host's tests not run, the digest file or the Go files out of the skip inputs, and no budget row. Risk: a
// member dropped from the push, or that runs a pattern that matches nothing or skips on a change it reads,
// and a piece into release-1, which takes no landing run, is red only after it merged.
func TestStandinRunsAtPushAndNamesWhatItReads(t *testing.T) {
	t.Parallel()
	checks, member, taskfile, tests := taskSection(t, "push-checks"), taskSection(t, "standin"), read(t, "Taskfile.yml"), testSources(t)
	specs, budgets := strings.Fields(taskVar(t, "STANDIN_INPUTS")), read(t, "gate/budgets.yaml")
	for _, p := range standinMemberProblems(checks, member, taskfile, tests, specs, budgets) {
		t.Error(p)
	}
	type in struct {
		checks, member, taskfile string
		specs                    []string
		budgets                  string
	}
	base := in{checks, member, taskfile, specs, budgets}
	for name, plant := range map[string]func(in) in{
		"the call dropped from push-checks": func(i in) in { i.checks = strings.Replace(i.checks, "      - task: standin\n", "", 1); return i },
		"the cache not bypassed":            func(i in) in { i.member = strings.Replace(i.member, "-count=1 ", "", 1); return i },
		"the host's tests not run":          func(i in) in { i.member = strings.Replace(i.member, "-run '{{.C24_RUN}}'", "", 1); return i },
		"a name that is no test": func(i in) in {
			i.taskfile = strings.Replace(i.taskfile, "TestC24AnEndlessLoopEndsWithinTheTimeLimit|", "TestNoSuchC24Test|", 1)
			return i
		},
		"the digest file out of the inputs": func(i in) in {
			i.specs = slices.DeleteFunc(slices.Clone(i.specs), func(s string) bool { return s == "gate/standin.digest" })
			return i
		},
		"the Go files out of the inputs": func(i in) in {
			i.specs = slices.DeleteFunc(slices.Clone(i.specs), func(s string) bool { return s == "'*.go'" })
			return i
		},
		"no budget row": func(i in) in { i.budgets = strings.Replace(i.budgets, "member-standin", "member-other", 1); return i },
	} {
		p := plant(base)
		if len(standinMemberProblems(p.checks, p.member, p.taskfile, tests, p.specs, p.budgets)) == 0 {
			t.Errorf("%s is not red", name)
		}
	}
}
