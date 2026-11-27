package journeys

import (
	"strings"
	"testing"
)

// The fast-forward journeys of check: git runs no hook when HEAD
// fast-forwards, so a commit made with no hook lands unjudged by the hooks.
// The gate is where it is judged: check holds each commit since the merge
// base, and a commit HEAD fast-forwarded to is one of them. Each plant is a
// way in that a review named, beside a control
// that fast-forwards to a command's own change and passes.

// fastForwarded is HEAD moved to rev by a merge git makes as a fast-forward,
// and fails the journey when git made a merge commit instead: a journey that
// meant to plant a fast-forward and planted a merge would prove the merge
// hook's rule, not the gate's.
func fastForwarded(t *testing.T, dir string, env map[string]string, rev string, args ...string) {
	t.Helper()
	want := strings.TrimSpace(gitIn(t, dir, env, "rev-parse", rev))
	gitIn(t, dir, env, append([]string{"merge", "-q"}, append(args, rev)...)...)
	if got := strings.TrimSpace(gitIn(t, dir, env, "rev-parse", "HEAD")); got != want {
		t.Fatalf("git merge %s made %s, not a fast-forward to %s", rev, got, want)
	}
}

// stashed is gateRepo with make's change stashed by stash push and its
// arguments, then merged with git merge stash@{0}, which fast-forwards: HEAD
// is the stash commit's first parent.
func stashed(t *testing.T, make func(dir string, env map[string]string), args ...string) (dir string, env map[string]string) {
	t.Helper()
	dir, env = gateRepo(t)
	make(dir, env)
	gitIn(t, dir, env, append([]string{"stash", "push", "-q", "-m", "a journey's own"}, args...)...)
	fastForwarded(t, dir, env, "stash@{0}")
	return dir, env
}

// TestCheckFindsAHandEditAFastForwardBroughtIn: a fast-forward
// to a stash of a hand edit, staged when it was stashed or not, and to a
// branch whose commit was made with no hook, is each a finding of the gate
// naming the field and the command that records it. A stash made with its
// untracked files is a commit of three parents, which the gate holds to no
// one other side, so it is a finding naming the commit, never a pass. A
// fast-forward to a stash of a promote staged whole ends done.
// Risk: a hand edit that lands by a fast-forward, past every hook, and past
// the gate too.
func TestCheckFindsAHandEditAFastForwardBroughtIn(t *testing.T) {
	t.Parallel()
	dir, env := stashed(t, func(dir string, env map[string]string) {
		promoteApplied(t, dir, env, "go.errors.2")
		gitIn(t, dir, env, "add", "-A")
	})
	if r := gateCheck(t, dir, env); r.code != 0 {
		t.Fatalf("the control, a fast-forward to a stash of a promote staged whole: exit %d, want done (0)\n%s%s", r.code, r.stdout, r.stderr)
	}

	field := []string{"lifecycle field changed without its entry", "rule:go.errors.2", "gragas rule promote go.errors.2"}
	for name, c := range map[string]struct {
		plant func() (string, map[string]string)
		names []string
	}{
		"a stash of a hand edit staged": {func() (string, map[string]string) {
			return stashed(t, func(dir string, env map[string]string) {
				handPromote(t, dir)
				gitIn(t, dir, env, "add", "-A")
			})
		}, field},
		"a stash of a hand edit not staged": {func() (string, map[string]string) {
			return stashed(t, func(dir string, _ map[string]string) { handPromote(t, dir) })
		}, field},
		"a stash with its untracked files": {func() (string, map[string]string) {
			return stashed(t, func(dir string, _ map[string]string) {
				handPromote(t, dir)
				write(t, dir, "notes.txt", "an untracked file\n")
			}, "--include-untracked")
		}, []string{"lifecycle field changed without its entry", "commit:", "cannot be held to its own entries", "merges 3 branches at once"}},
		"a branch committed with no hook": {func() (string, map[string]string) {
			dir, env := gateRepo(t)
			gitIn(t, dir, env, "checkout", "-q", "-b", "side")
			handPromote(t, dir)
			commitAll(t, dir, env)
			gitIn(t, dir, env, "checkout", "-q", "-")
			fastForwarded(t, dir, env, "side", "--ff-only")
			return dir, env
		}, field},
	} {
		dir, env := c.plant()
		r := gateCheck(t, dir, env)
		if r.code != 1 {
			t.Errorf("%s, fast-forwarded: exit %d, want findings (1)\n%s%s", name, r.code, r.stdout, r.stderr)
			continue
		}
		for _, w := range c.names {
			if !strings.Contains(flat(r.stdout), flat(w)) {
				t.Errorf("%s: the listing does not name %q:\n%s%s", name, w, r.stdout, r.stderr)
			}
		}
	}
}
