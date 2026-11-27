package internal

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/go-files/filestest"
)

// cpuBudget runs the Taskfile's own BUDGET, through the pinned go-task and hatch, on a tier x whose
// command is the task named by body, against a row of the given wall limit (the hang guard) and
// cpu_limit, both in seconds, and answers hatch's words and whether the tier ended green
// (gragas#1779). The tier starts when it is called, so no clock of 1970 stands in for it.
func cpuBudget(t *testing.T, body string, wall, cpu int) (string, bool) {
	t.Helper()
	text := read(t, "Taskfile.yml")
	dir := t.TempDir()
	tools := filepath.ToSlash(filepath.Dir(pinnedPath(t, "hatch")))
	var vars strings.Builder
	vars.WriteString("  TOOLS: '" + tools + "'\n")
	for _, name := range []string{"BUDGET_ROW", "BUDGET_CPU_ROW", "BUDGET"} {
		// The budgets file is at the root of dir, so the test makes no folder.
		vars.WriteString("  " + name + ": " + strings.Replace(varLine(text, name), "--budgets gate/budgets.yaml", "--budgets budgets.yaml", 1) + "\n")
	}
	taskfile := "version: '3'\nvars:\n" + vars.String() + "tasks:\n" +
		"  default:\n    vars:\n      START: '{{now | unixEpoch}}'\n    cmds:\n      - cmd: '{{.BUDGET}} --tier x --since {{.START}} -- {{.TASK_EXE}} " + body + "'\n" +
		"  spin:\n    cmds:\n      - cmd: 'i=0; while [ $i -lt 2000000 ]; do i=$((i+1)); done'\n" +
		"  sleep:\n    cmds:\n      - cmd: 'sleep 30'\n" +
		"  idle:\n    cmds:\n      - cmd: 'true'\n"
	filestest.WriteFile(t, filepath.Join(dir, "Taskfile.yml"), []byte(taskfile))
	filestest.WriteFile(t, filepath.Join(dir, "budgets.yaml"),
		[]byte("rules:\n  - id: tier-x\n    limit: "+strconv.Itoa(wall)+"\n    cpu_limit: "+strconv.Itoa(cpu)+"\n"))
	return taskIn(t, dir, "--silent")
}

// TestATierIsJudgedByItsOwnCPUTime holds gragas#1779's three plants against the Taskfile's own BUDGET:
// a tier that burns CPU past its cpu_limit is red on a quiet machine, a tier that only waits is
// green however long it takes, and the same tier is as green while the machine is loaded by
// another job, so the room never decides. The numbers that hatch prints are CPU time, not wall time.
func TestATierIsJudgedByItsOwnCPUTime(t *testing.T) {
	t.Parallel()
	out, ok := cpuBudget(t, "spin", 600, 1)
	if ok || !strings.Contains(out, "tier-x") || !strings.Contains(out, "CPU") {
		t.Errorf("a tier that burns CPU past a cpu_limit of 1 s is not red, or its CPU time is not named:\n%s", out)
	}
	if out, ok := cpuBudget(t, "idle", 600, 600); !ok || !strings.Contains(out, "used") || !strings.Contains(out, "of CPU") {
		t.Errorf("a tier far under its cpu_limit is not green, or does not say its CPU time:\n%s", out)
	}
}

// TestATierWaitingPastItsWallLimitIsGreenOnCPUAndAHangIsRed holds that wall time is the hang guard only:
// a tier over its wall limit with its CPU under passes, and a command still running at ten times the
// wall limit is killed and red (hatch v0.6.0).
func TestATierWaitingPastItsWallLimitIsGreenOnCPUAndAHangIsRed(t *testing.T) {
	t.Parallel()
	if out, ok := cpuBudget(t, "idle", 600, 600); !ok {
		t.Errorf("a quick tier is not green:\n%s", out)
	}
	out, ok := cpuBudget(t, "sleep", 1, 600)
	if ok || !strings.Contains(out, "hung") {
		t.Errorf("a tier that sleeps past ten times its wall limit is not red as hung:\n%s", out)
	}
}

// closed reports whether ch is closed, without waiting.
func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// TestATierIsAsGreenUnderAnOutsideLoadAsOnAQuietMachine holds the second plant of gragas#1779: while
// every CPU is held by another job (this test's own spinning goroutines, which are no part of the
// tier's tree), a tier that costs little CPU is green, and its CPU time is the tier's own.
func TestATierIsAsGreenUnderAnOutsideLoadAsOnAQuietMachine(t *testing.T) {
	t.Parallel()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for spin := 0; ; spin++ {
				if spin%1024 == 0 && closed(stop) {
					return
				}
			}
		})
	}
	defer func() { close(stop); wg.Wait() }()
	if out, ok := cpuBudget(t, "idle", 600, 600); !ok {
		t.Errorf("a tier under its cpu_limit is red while another job loads the machine:\n%s", out)
	}
}

// TestEveryTierRunsItsMembersUnderTheCPUBudget holds gragas#1779's wiring: each tier's own line is the
// BUDGET with its members as the command, nothing of the old load excuse is left in the Taskfile, and
// every tier row of gate/budgets.yaml has a cpu_limit beside its wall limit.
func TestEveryTierRunsItsMembersUnderTheCPUBudget(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	budget := varLine(text, "BUDGET")
	for _, want := range []string{"budget --cpu ", "--cpu-row '{{.BUDGET_CPU_ROW}}'", "--row '{{.BUDGET_ROW}}'"} {
		if !strings.Contains(budget, want) {
			t.Errorf("BUDGET does not carry %s", want)
		}
	}
	for _, gone := range []string{"BUDGET_LOAD_LIMIT", "--loaded-when", "--load-before", "LOAD_FILE", "budget --load"} {
		if strings.Contains(text, gone) {
			t.Errorf("Taskfile.yml still carries %s: the load excuse is gone", gone)
		}
	}
	budgets := read(t, "gate/budgets.yaml")
	for _, tier := range []string{"pre-commit", "pre-push", "warm", "full", "release"} {
		section := taskSection(t, tier)
		if !strings.Contains(section, "{{.BUDGET}} --tier "+tier+" --since {{.START}} -- {{.TASK_EXE}} ") {
			t.Errorf("%s does not run its members under {{.BUDGET}} with the tier's start", tier)
		}
		row := budgets[strings.Index(budgets, "  - id: tier-"+tier+"\n"):]
		if end := strings.Index(row[1:], "\n  - id: "); end >= 0 {
			row = row[:end+1]
		}
		if !strings.Contains(row, "\n    cpu_limit: ") {
			t.Errorf("gate/budgets.yaml's tier-%s has no cpu_limit", tier)
		}
	}
}
