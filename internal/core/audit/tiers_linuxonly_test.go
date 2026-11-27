package audit

import (
	"testing"
	"testing/fstest"
)

// TestATaskDeclaredLinuxOnlyByDesignIsExcusedAndNothingLessIs: a task a tier calls is a member
// that is Linux-only by design when its own platforms: is a list of plain system names that has linux in it
// and its own vars: names why, in a literal; then the platforms: on that task is no skip, and that is all it
// excuses. A task that is merely platform-bound, one with no reason, an empty or a computed one, one whose list
// leaves out linux, takes linux out ("!linux") or holds a non-string, a platforms: on a command inside a declared
// task (a member command that never runs on Linux, the only host it runs on, among them), a platforms: on a task
// it calls, in either order of the calls, and the tier's own task, stay findings, in the push and in the release
// alike. The control is the declared task, which passes. Risk: a skip that hides a red member excused because it
// says "Linux only", or an excusal that depends on which caller reaches a shared task first.
func TestATaskDeclaredLinuxOnlyByDesignIsExcusedAndNothingLessIs(t *testing.T) {
	t.Parallel()
	why := "    vars:\n      PLATFORMS_WHY: \"needs a PID namespace, which only Linux has\"\n"
	finds := func(tier Tier, calls, tasks string) bool {
		task := "version: '3'\ntasks:\n  " + tier.Name + ":\n    cmds:\n      - go vet ./...\n" + calls + tasks
		tier.Members = []Member{{ID: "vet", Rule: "t.3", All: []string{"go vet"}}}
		in := TierInput{
			Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte(task)}},
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers:   Tiers{Tiers: []Tier{tier}, Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
		}
		return len(CheckTiers(in).Findings) > 0
	}
	ns := func(head, cmds string) string { return "  ns:\n" + head + "    cmds:\n" + cmds }
	plain := "      - go test ./...\n"
	call := "      - task: ns\n"
	push, release := Tier{Name: "pre-push"}, Tier{Name: "release", Whole: true}
	for name, c := range map[string]struct {
		tier         Tier
		calls, tasks string
		red          bool
	}{
		"declared, in the push":      {push, call, ns("    platforms: [linux]\n"+why, plain), false},
		"declared, in the release":   {release, call, ns("    platforms: [linux]\n"+why, plain), false},
		"declared over more systems": {push, call, ns("    platforms: [linux, darwin]\n"+why, plain), false},
		"platform-bound, no reason":  {push, call, ns("    platforms: [linux]\n", plain), true},
		"an empty reason":            {push, call, ns("    platforms: [linux]\n    vars:\n      PLATFORMS_WHY: \" \"\n", plain), true},
		"a computed reason":          {push, call, ns("    platforms: [linux]\n    vars:\n      PLATFORMS_WHY: \"{{.WHY}}\"\n", plain), true},
		"a reason and no linux":      {push, call, ns("    platforms: [darwin]\n"+why, plain), true},
		"a reason and no platforms":  {push, call, ns(why, plain), false}, // nothing is skipped, so nothing is excused
		"linux taken out again":      {push, call, ns("    platforms: [linux, '!linux']\n"+why, plain), true},
		"a non-string in the list":   {push, call, ns("    platforms: [linux, 7]\n"+why, plain), true},
		"no reason, in the release":  {release, call, ns("    platforms: [linux]\n", plain), true},
		// What a declared task excuses is its own platforms: and no more.
		"a command's platforms in it":           {push, call, ns("    platforms: [linux]\n"+why, "      - {cmd: go test ./..., platforms: [linux]}\n"), true},
		"a member that never runs on linux":     {push, call, ns("    platforms: [linux]\n"+why, "      - {cmd: go test ./..., platforms: [darwin]}\n"), true},
		"a platform-bound task it calls":        {push, call, ns("    platforms: [linux]\n"+why, "      - task: inner\n") + "  inner:\n    platforms: [darwin]\n    cmds: [go test ./...]\n", true},
		"a shared task, the declared one first": {push, call + "      - task: shared\n", ns("    platforms: [linux]\n"+why, "      - task: shared\n") + "  shared:\n    cmds:\n      - {cmd: go test ./..., platforms: [darwin]}\n", true},
		"a shared task, the declared one last":  {push, "      - task: shared\n" + call, ns("    platforms: [linux]\n"+why, "      - task: shared\n") + "  shared:\n    cmds:\n      - {cmd: go test ./..., platforms: [darwin]}\n", true},
	} {
		if got := finds(c.tier, c.calls, c.tasks); got != c.red {
			t.Errorf("%s: a finding = %v, want %v", name, got, c.red)
		}
	}
	// The tier's own task is never read this way: platforms: and a reason on it is still a finding.
	if !tierOwnKeyFinds(why) {
		t.Error("the tier's own task, with platforms: and a reason, was excused")
	}
}

// tierOwnKeyFinds is whether the tier's own task, with platforms: linux and a reason, is a finding.
func tierOwnKeyFinds(why string) bool {
	task := "version: '3'\ntasks:\n  pre-push:\n    platforms: [linux]\n" + why + "    cmds:\n      - go vet ./...\n"
	in := TierInput{
		Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte(task)}},
		Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
		Tiers: Tiers{Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "vet", Rule: "t.3", All: []string{"go vet"}}}}},
			Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
	}
	return len(CheckTiers(in).Findings) > 0
}

// TestADeclaredLinuxOnlyTaskExcusesOnlyMembersTheTableListsAsLinuxOnly: a task
// that carries a member of the tier's table is excused by its reason only when the table lists that member
// as linux_only. The vet member holds `go vet`; a declared task that runs it is a finding when vet is not
// listed, and is excused when it is. Risk: a declared task that hides a member every host must run.
func TestADeclaredLinuxOnlyTaskExcusesOnlyMembersTheTableListsAsLinuxOnly(t *testing.T) {
	t.Parallel()
	finds := func(listed bool) bool {
		task := "version: '3'\ntasks:\n  pre-push:\n    cmds:\n      - task: ns\n  ns:\n    platforms: [linux]\n" +
			"    vars:\n      PLATFORMS_WHY: \"needs a PID namespace, which only Linux has\"\n    cmds:\n      - go vet ./...\n"
		in := TierInput{
			Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte(task)}},
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "vet", Rule: "t.3", All: []string{"go vet"}, LinuxOnly: listed}}}},
				Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
		}
		return len(CheckTiers(in).Findings) > 0
	}
	if !finds(false) {
		t.Error("vet declared Linux-only but not listed was excused")
	}
	if finds(true) {
		t.Error("vet declared Linux-only and listed was refused")
	}
}
