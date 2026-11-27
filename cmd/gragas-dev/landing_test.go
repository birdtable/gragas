package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// landingFixture is a repository whose origin is a second repository of the test's own: main holds one commit and
// a clone of it has main merged into a branch (the tree under landing). The origin can move on.
func landingFixture(t *testing.T) (origin, clone string) {
	t.Helper()
	origin = fixture(t, map[string]string{"a.txt": "one\n"})
	git(t, origin, "init", "-q", "-b", "main")
	git(t, origin, "add", "-A")
	git(t, origin, "commit", "-q", "-m", "one")
	clone = t.TempDir()
	git(t, clone, "clone", "-q", origin, ".")
	git(t, clone, "checkout", "-q", "-b", "pr")
	return origin, clone
}

// landingMoves commits to the origin's branch, as another lander's merge does.
func landingMoves(t *testing.T, origin, file string) {
	t.Helper()
	write(t, origin, file, "more\n")
	git(t, origin, "add", "-A")
	git(t, origin, "commit", "-q", "-m", file)
}

func landingSha(t *testing.T, dir, rev string) string {
	t.Helper()
	got, err := withArgs(exec.CommandContext(t.Context(), "git"), []string{"-C", dir, "rev-parse", rev}).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(got))
}

// landingGHOf is a GitHub of the test's own: the pull request's base, and a merge that says it was asked.
type landingGHOf struct {
	base   string
	merged []string
}

func (g *landingGHOf) gh() landingGH {
	return landingGH{
		base: func(context.Context, string, string) (string, error) { return g.base, nil },
		merge: func(_ context.Context, _, number, head string) error {
			g.merged = append(g.merged, number+"@"+head)
			return nil
		},
	}
}

// landingRunOf runs the landing command with the test's environment less the tools' switches: the test
// environment sets GOENV=off, which the landing refuses as it refuses any GOENV.
func landingRunOf(t *testing.T, gh landingGH, args ...string) (int, string) {
	t.Helper()
	return landingUnder(t, slices.DeleteFunc(configtest.Inherited(), isSwitch), gh, args...)
}

// landingUnder is landingRunOf with environ the environment the command is given.
func landingUnder(t *testing.T, environ []string, gh landingGH, args ...string) (int, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := runLanding(t.Context(), args, environ, gh, &out, &errs)
	return code, out.String() + errs.String()
}

// TestLandingRunGivesTaskNoSwitchOfATool (gragas#2169): landing run starts the clone's task landing, and a red
// landing is red, under a variable of each kind that switches a tool of the tiers. The stand-in task, as go-task
// does, exits 0 for any task under TASK_DRY, and logs each switch it is given; nor is task given any of plantedLeaks,
// which the hooks' allowlist drops (gragas#2181). Planted failure: run started with the caller's environment whole
// is green over the red tree and logs every switch and leak.
func TestLandingRunGivesTaskNoSwitchOfATool(t *testing.T) {
	t.Parallel()
	environ := append(append(configtest.Inherited(), plantedSwitches()...), plantedLeaks()...)
	dir, log, _, _ := hookClone(t, fakeTaskConfig{}, "")
	if code, out := landingUnder(t, environ, landingGH{}, "run", "-C", dir); code != exitDone {
		t.Fatalf("landing run of a green tree: %d\n%s", code, out)
	}
	red, redLog, _, _ := hookClone(t, fakeTaskConfig{Fail: []string{"landing"}}, "")
	if code, out := landingUnder(t, environ, landingGH{}, "run", "-C", red); code == exitDone {
		t.Errorf("landing run of a red tree under TASK_DRY is green\n%s", out)
	}
	for _, c := range [][]string{taskCalls(t, log), taskCalls(t, redLog)} {
		if !slices.Equal(c, []string{"landing @here"}) {
			t.Errorf("task was called as %q, want once as landing with no switch", c)
		}
	}
}

// TestLandingRunGivesTaskNoIndexOfTheCallers (gragas#2181): a GIT_INDEX_FILE in the lander's environment is the
// lander's, never git's, and an index at the base, or an empty one, would make every member's inputs read as
// unchanged or list no file; landing run hands task landing none. Planted failure: GIT_INDEX_FILE back in hookKept,
// and task is given the lander's index.
func TestLandingRunGivesTaskNoIndexOfTheCallers(t *testing.T) {
	t.Parallel()
	dir, log, _, _ := hookClone(t, fakeTaskConfig{Show: []string{"GIT_INDEX_FILE"}}, "")
	environ := append(slices.DeleteFunc(configtest.Inherited(), isSwitch), "GIT_INDEX_FILE="+filepath.Join(t.TempDir(), "callers-index"))
	if code, out := landingUnder(t, environ, landingGH{}, "run", "-C", dir); code != exitDone {
		t.Fatalf("landing run of a green tree: %d\n%s", code, out)
	}
	if got, want := taskCalls(t, log), []string{"landing GIT_INDEX_FILE= @here"}; !slices.Equal(got, want) {
		t.Errorf("task was called as %q, want %q: the lander's index reached it", got, want)
	}
}

// TestLandingRefusesAnEnvironmentThatHoldsASwitch (gragas#2169): begin, check and merge refuse while a tool's
// switch is set, naming it, so a lander who ran task landing under TASK_DRY, not landing run, never merges; and
// merge asks GitHub for nothing. Each of plantedSwitches is held, one name of every prefix the run cuts, and a
// lower-case name too. Planted failures: no refusal, and merge merges; a prefix left out of switchPrefixes.
func TestLandingRefusesAnEnvironmentThatHoldsASwitch(t *testing.T) {
	t.Parallel()
	_, clone := landingFixture(t)
	tested := landingSha(t, clone, "origin/main")
	for _, sw := range append(plantedSwitches(), "TASK_DRY=true", "task_dry=true") {
		g := &landingGHOf{base: "main"}
		environ := append(slices.DeleteFunc(configtest.Inherited(), isSwitch), sw)
		for _, args := range [][]string{
			{"begin", "-C", clone},
			{"check", "--tested", tested, "-C", clone},
			{"merge", "7", "--head", tested, "--tested", tested, "-C", clone},
		} {
			name, _, _ := strings.Cut(sw, "=")
			code, out := landingUnder(t, environ, g.gh(), args...)
			if code != exitFailed || !strings.Contains(out, "refused: "+name+" is set") {
				t.Errorf("%s under %s: %d, want refused naming it\n%s", args[0], sw, code, out)
			}
		}
		if len(g.merged) != 0 {
			t.Errorf("under %s GitHub was asked to merge %q", sw, g.merged)
		}
	}
}

// TestLandingMergeRefusesAMainThatMovedAfterTheRun (gragas#1819): the tree was merged with main at one commit,
// another lander merged a second commit before this one's merge, and merge refuses, naming the sha it tested on
// and the one origin/main now holds, and asks GitHub for no merge. Planted failures: a merge that never fetches,
// compares nothing, or merges first.
func TestLandingMergeRefusesAMainThatMovedAfterTheRun(t *testing.T) {
	t.Parallel()
	origin, clone := landingFixture(t)
	tested := landingSha(t, clone, "refs/remotes/origin/main")
	head := strings.Repeat("a", 40)
	g := &landingGHOf{base: "main"}

	if code, out := landingRunOf(t, g.gh(), "begin", "-C", clone); code != exitDone || !strings.Contains(out, tested) {
		t.Fatalf("begin = %d %q, want 0 naming %s", code, out, tested)
	}
	if code, out := landingRunOf(t, g.gh(), "merge", "7", "--head", head, "--tested", tested, "-C", clone); code != exitDone || len(g.merged) != 1 || g.merged[0] != "7@"+head {
		t.Fatalf("merge with main unmoved = %d %q, merged %v, want 0 and the merge of 7 at the head", code, out, g.merged)
	}

	g.merged = nil
	landingMoves(t, origin, "b.txt")
	moved := landingSha(t, origin, "main")
	code, out := landingRunOf(t, g.gh(), "merge", "7", "--head", head, "--tested", tested, "-C", clone)
	if code != exitFailed || !strings.Contains(out, tested) || !strings.Contains(out, moved) || len(g.merged) != 0 {
		t.Errorf("merge with main moved = %d %q, merged %v, want a refusal naming %s and %s, and no merge", code, out, g.merged, tested, moved)
	}
}

// TestLandingMergeIsNotSatisfiedByATreeThatHoldsTheNewMain: a lander refused at the merge who merges the new main
// into the tree and asks again with the sha of the run that was refused gets a refusal still: the sha is the
// run's, never the tree's. Planted failure: a merge that compares against the tree's own main.
func TestLandingMergeIsNotSatisfiedByATreeThatHoldsTheNewMain(t *testing.T) {
	t.Parallel()
	origin, clone := landingFixture(t)
	tested := landingSha(t, clone, "refs/remotes/origin/main")
	landingMoves(t, origin, "b.txt")
	git(t, clone, "pull", "-q", "--no-rebase", "origin", "main")
	g := &landingGHOf{base: "main"}
	code, out := landingRunOf(t, g.gh(), "merge", "7", "--head", strings.Repeat("b", 40), "--tested", tested, "-C", clone)
	if code != exitFailed || !strings.Contains(out, tested) || len(g.merged) != 0 {
		t.Errorf("merge on a re-merged tree = %d %q, merged %v, want a refusal and no merge", code, out, g.merged)
	}
}

// TestLandingMergeTakesTheBaseFromThePullRequest: a pull request into release-1 is held to release-1's tip, and
// a move of main does not refuse it. Planted failure: a merge that always reads main.
func TestLandingMergeTakesTheBaseFromThePullRequest(t *testing.T) {
	t.Parallel()
	origin, clone := landingFixture(t)
	git(t, origin, "branch", "release-1")
	git(t, clone, "fetch", "-q", "origin")
	tested := landingSha(t, clone, "refs/remotes/origin/release-1")
	head := strings.Repeat("c", 40)
	landingMoves(t, origin, "b.txt") // main moves; release-1 does not
	g := &landingGHOf{base: "release-1"}
	if code, out := landingRunOf(t, g.gh(), "merge", "9", "--head", head, "--tested", tested, "-C", clone); code != exitDone || len(g.merged) != 1 {
		t.Fatalf("merge into release-1 = %d %q, merged %v, want 0 and the merge", code, out, g.merged)
	}
	git(t, origin, "checkout", "-q", "release-1")
	landingMoves(t, origin, "c.txt")
	g.merged = nil
	if code, out := landingRunOf(t, g.gh(), "merge", "9", "--head", head, "--tested", tested, "-C", clone); code != exitFailed || len(g.merged) != 0 || !strings.Contains(out, "release-1") {
		t.Errorf("merge after release-1 moved = %d %q, merged %v, want a refusal naming release-1", code, out, g.merged)
	}
}

// TestLandingBeginRefusesATreeThatDoesNotHoldMain: a tree not merged with origin/main was not tested on it.
func TestLandingBeginRefusesATreeThatDoesNotHoldMain(t *testing.T) {
	t.Parallel()
	origin, clone := landingFixture(t)
	landingMoves(t, origin, "b.txt")
	git(t, clone, "fetch", "-q", "origin")
	g := &landingGHOf{base: "main"}
	if code, out := landingRunOf(t, g.gh(), "begin", "-C", clone); code != exitFailed || !strings.Contains(out, "does not hold") {
		t.Errorf("begin on a tree behind origin/main = %d %q, want a refusal", code, out)
	}
}

// TestLandingCheckAndUsage: check refuses a moved base by name, and a missing word, sha or branch is a usage error
// or a refusal, never a merge.
func TestLandingCheckAndUsage(t *testing.T) {
	t.Parallel()
	origin, clone := landingFixture(t)
	tested := landingSha(t, clone, "refs/remotes/origin/main")
	g := &landingGHOf{base: "main"}
	if code, _ := landingRunOf(t, g.gh(), "check", "--tested", tested, "-C", clone); code != exitDone {
		t.Errorf("check unmoved = %d, want 0", code)
	}
	landingMoves(t, origin, "b.txt")
	if code, out := landingRunOf(t, g.gh(), "check", "--tested", tested, "-C", clone); code != exitFailed || !strings.Contains(out, tested) {
		t.Errorf("check moved = %d %q, want a refusal naming %s", code, out, tested)
	}
	for _, args := range [][]string{{}, {"land"}, {"check", "-C", clone}, {"merge", "7", "--tested", tested, "-C", clone}, {"merge", "--head", tested}, {"merge", "feature", "--head", tested, "--tested", tested, "-C", clone}, {"merge", "7x", "--head", tested, "--tested", tested, "-C", clone}, {"check", "--tested", "abc", "-C", clone}} {
		if code, _ := landingRunOf(t, g.gh(), args...); code != exitUsage || len(g.merged) != 0 {
			t.Errorf("landing %v = %d, want usage and no merge", args, code)
		}
	}
	bad := &landingGHOf{base: "--upload-pack=x"}
	if code, _ := landingRunOf(t, bad.gh(), "merge", "7", "--head", tested, "--tested", tested, "-C", clone); code != exitFailed || len(bad.merged) != 0 {
		t.Errorf("a base that is no branch name = %d, merged %v, want a refusal and no merge", code, bad.merged)
	}
}

// TestLandingIsNotRefusedForTheEnvironmentLandingRunBuilds (gragas#2389): landing run gives the landing the
// environment allowed builds, which holds GOENV=off, and the Taskfile's first step, landing begin, then judges that
// environment; so begin, check and merge are not refused for a switch under it. Planted failure: isSwitch refusing
// GOENV=off, as it did, and every landing stops at its first step.
func TestLandingIsNotRefusedForTheEnvironmentLandingRunBuilds(t *testing.T) {
	t.Parallel()
	_, clone := landingFixture(t)
	tested := landingSha(t, clone, "origin/main")
	environ, err := allowed(t.Context(), configtest.Inherited())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(environ, "GOENV=off") {
		t.Fatalf("the environment allowed builds holds no GOENV=off: %q", environ)
	}
	g := &landingGHOf{base: "main"}
	for _, args := range [][]string{
		{"begin", "-C", clone},
		{"check", "--tested", tested, "-C", clone},
		{"merge", "7", "--head", tested, "--tested", tested, "-C", clone},
	} {
		if _, out := landingUnder(t, environ, g.gh(), args...); strings.Contains(out, "which a tool of the landing obeys") {
			t.Errorf("%s was refused for the environment landing run builds:\n%s", args[0], out)
		}
	}
}
