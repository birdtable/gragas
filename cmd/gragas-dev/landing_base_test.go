package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestLandingRunHandsItsBaseToTheTask (gragas#2391): landing run takes --base and hands it to task landing as a task
// variable, never an environment one, so the Taskfile's landing begin names the base of a release-1 landing; a run on
// main hands none, and a base that is no branch name is refused before task is called. Planted failure: run ignoring
// --base, and every release-1 landing demands main.
func TestLandingRunHandsItsBaseToTheTask(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		args []string
		want []string
		code int
	}{
		{[]string{"--base", "release-1"}, []string{"landing LANDING_BASE=release-1 @here"}, exitDone},
		{[]string{"--base", "main"}, []string{"landing @here"}, exitDone},
		{nil, []string{"landing @here"}, exitDone},
		{[]string{"--base", "-x"}, nil, exitFailed},
	} {
		dir, log, _, _ := hookClone(t, fakeTaskConfig{}, "")
		code, out := landingUnder(t, slices.DeleteFunc(configtest.Inherited(), isSwitch), landingGH{}, append([]string{"run", "-C", dir}, c.args...)...)
		if code != c.code || !slices.Equal(taskCalls(t, log), c.want) {
			t.Errorf("landing run %q = %d, task called as %q, want %d and %q\n%s", c.args, code, taskCalls(t, log), c.code, c.want, out)
		}
	}
}

// TestTheTaskfileLandingBeginNamesTheBaseOfTheRun (gragas#2391): the first step of task landing passes landing begin
// the LANDING_BASE that landing run hands it, main by default. Planted failure: the step with no --base.
func TestTheTaskfileLandingBeginNamesTheBaseOfTheRun(t *testing.T) {
	t.Parallel()
	task, err := os.ReadFile(filepath.Join("..", "..", "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `landing begin --base {{.LANDING_BASE | default "main"}}`; !strings.Contains(string(task), want) {
		t.Errorf("Taskfile.yml's landing task does not begin with %s", want)
	}
}

// TestLandingBeginOnAReleaseTreeNeedsItsBase (gragas#2391): a tree merged with origin/release-1, which lacks a newer
// origin/main, passes begin --base release-1, and is refused by begin with no base, naming main. Planted failure: a
// begin that reads no base.
func TestLandingBeginOnAReleaseTreeNeedsItsBase(t *testing.T) {
	t.Parallel()
	origin, clone := landingFixture(t)
	git(t, origin, "branch", "release-1")
	landingMoves(t, origin, "b.txt")
	git(t, clone, "fetch", "-q", "origin")
	git(t, clone, "checkout", "-q", "-B", "pr", "origin/release-1")
	environ := slices.DeleteFunc(configtest.Inherited(), isSwitch)
	if code, out := landingUnder(t, environ, landingGH{}, "begin", "--base", "release-1", "-C", clone); code != exitDone {
		t.Errorf("begin --base release-1 on a release-1 tree = %d, want done\n%s", code, out)
	}
	if code, out := landingUnder(t, environ, landingGH{}, "begin", "-C", clone); code != exitFailed || !strings.Contains(out, "main") {
		t.Errorf("begin with no base on a release-1 tree = %d, want a refusal naming main\n%s", code, out)
	}
}
