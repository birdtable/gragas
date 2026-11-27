package audit

import (
	"strings"
	"testing"
	"testing/fstest"
)

// cannotFailIn reads a repository's tier pre-push and says whether a finding calls a member one that
// cannot fail.
func cannotFailIn(t *testing.T, repo fstest.MapFS) bool {
	t.Helper()
	in := TierInput{
		Repo:    repo,
		Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
		Tiers:   Tiers{Tiers: []Tier{{Name: "pre-push"}}, Rules: TierRules{Tier: "t.1", Runs: "t.2"}},
	}
	for _, f := range CheckTiers(in).Findings {
		if strings.Contains(f.Missing, "cannot fail") {
			return true
		}
	}
	return false
}

// TestAMemberReachedByAnyRouteIsJudgedOnPipefailAsACmdsMemberIs: a pipe to tee
// with no pipefail cannot fail the member, whether the member is a cmds: entry of the tier, of a task
// the tier lists under deps:, or of a task of an included Taskfile; pipefail set on its own task or on the
// root file makes it fail; an included file's own set: is dropped by go-task, and the caller's task does
// not reach a callee.
func TestAMemberReachedByAnyRouteIsJudgedOnPipefailAsACmdsMemberIs(t *testing.T) {
	t.Parallel()
	const pipe = "go vet ./... | tee vet.log"
	file := func(head, tier, tasks string) fstest.MapFS {
		return fstest.MapFS{
			"Taskfile.yml": {Data: []byte("version: '3'\n" + head + "includes:\n  inc: {taskfile: inc.yml}\ntasks:\n  pre-push:\n" + tier + tasks)},
			"inc.yml":      {Data: []byte("version: '3'\ntasks:\n  vet:\n    cmds: [" + pipe + "]\n")},
		}
	}
	cases := []struct {
		name string
		repo fstest.MapFS
		red  bool
	}{
		{"cmds, no pipefail", file("", "    cmds: ['"+pipe+"']\n", ""), true},
		{"cmds, task pipefail", file("", "    set: [pipefail]\n    cmds: ['"+pipe+"']\n", ""), false},
		{"cmds, file pipefail", file("set: [pipefail]\n", "    cmds: ['"+pipe+"']\n", ""), false},
		{"deps, no pipefail", file("", "    deps: [vet]\n", "  vet:\n    cmds: ['"+pipe+"']\n"), true},
		{"deps, callee pipefail", file("", "    deps: [vet]\n", "  vet:\n    set: [pipefail]\n    cmds: ['"+pipe+"']\n"), false},
		{"deps, caller pipefail only", file("", "    set: [pipefail]\n    deps: [vet]\n", "  vet:\n    cmds: ['"+pipe+"']\n"), true},
		{"task call, no pipefail", file("", "    cmds:\n      - task: vet\n", "  vet:\n    cmds: ['"+pipe+"']\n"), true},
		{"included, no pipefail", file("", "    deps: [inc:vet]\n", ""), true},
		{"included, its own file's pipefail is dropped by go-task", fstest.MapFS{
			"Taskfile.yml": {Data: []byte("version: '3'\nincludes:\n  inc: {taskfile: inc.yml}\ntasks:\n  pre-push:\n    deps: [inc:vet]\n")},
			"inc.yml":      {Data: []byte("version: '3'\nset: [pipefail]\ntasks:\n  vet:\n    cmds: ['" + pipe + "']\n")},
		}, true},
		{"included, the root file's pipefail", fstest.MapFS{
			"Taskfile.yml": {Data: []byte("version: '3'\nset: [pipefail]\nincludes:\n  inc: {taskfile: inc.yml}\ntasks:\n  pre-push:\n    deps: [inc:vet]\n")},
			"inc.yml":      {Data: []byte("version: '3'\ntasks:\n  vet:\n    cmds: ['" + pipe + "']\n")},
		}, false},
		{"included, its task's own pipefail", fstest.MapFS{
			"Taskfile.yml": {Data: []byte("version: '3'\nincludes:\n  inc: {taskfile: inc.yml}\ntasks:\n  pre-push:\n    deps: [inc:vet]\n")},
			"inc.yml":      {Data: []byte("version: '3'\ntasks:\n  vet:\n    set: [pipefail]\n    cmds: ['" + pipe + "']\n")},
		}, false},
		{"included, called from cmds", file("", "    cmds:\n      - task: inc:vet\n", ""), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := cannotFailIn(t, c.repo); got != c.red {
				t.Errorf("a pipe to tee: red = %v, want %v", got, c.red)
			}
		})
	}
}
