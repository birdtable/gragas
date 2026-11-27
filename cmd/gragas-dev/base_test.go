package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/surface"
)

// lostAgainstABase is the files of names that some base of the head at dir
// holds and the head's own tree lacks: what the shrink checks would call a row
// the base holds and the head dropped.
func lostAgainstABase(ctx context.Context, t *testing.T, dir string, names ...string) []string {
	t.Helper()
	var lost []string
	for _, ref := range basesFor(ctx, os.Environ(), dir, "HEAD") {
		for _, n := range names {
			held, err := gitBaseFile(ctx, os.Environ(), dir, ref, n)
			if err != nil {
				t.Fatal(err)
			}
			if held == nil || slices.Contains(lost, n) {
				continue
			}
			head, err := gitBaseFile(ctx, os.Environ(), dir, "HEAD", n)
			if err != nil {
				t.Fatal(err)
			}
			if head == nil {
				lost = append(lost, n)
			}
		}
	}
	return lost
}

// baseFixture is a repository of the test's own: main and origin/release-1
// part at a cut; main then adds main-only.txt and release-1 adds release.txt.
func baseFixture(t *testing.T) string {
	t.Helper()
	dir := fixture(t, map[string]string{"cut.txt": "cut\n"})
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "the cut")
	git(t, dir, "branch", "release")
	write(t, dir, "main-only.txt", "main\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "main goes on")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, dir, "switch", "-q", "release")
	write(t, dir, "release.txt", "row\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "the release's own list")
	git(t, dir, "update-ref", "refs/remotes/origin/release-1", "HEAD")
	return dir
}

// TestAReleaseBranchIsHeldToTheReleaseAndTheMainItCarries (gragas#1866): a
// head on the release is held to origin/release-1 and to the main it carries.
// A row main added after the cut, which the head never took, is no loss; the
// release's own row, dropped, is. Planted failure: the bases are main's tip
// alone, and the release's own row is dropped unseen.
func TestAReleaseBranchIsHeldToTheReleaseAndTheMainItCarries(t *testing.T) {
	t.Parallel()
	dir := baseFixture(t)
	git(t, dir, "switch", "-q", "-c", "feature")
	git(t, dir, "rm", "-q", "release.txt")
	git(t, dir, "commit", "-q", "-m", "a row of the release's list removed")
	got := lostAgainstABase(t.Context(), t, dir, "main-only.txt", "release.txt", "cut.txt")
	if !slices.Equal(got, []string{"release.txt"}) {
		t.Errorf("lost against a base = %v, want only release.txt: main's row is not the release head's to keep, the release's is", got)
	}
}

// TestAHeadBoundForMainThatMergedTheReleaseKeepsMainsRows: a branch from main that merges release-1 carries release-only
// commits, and is held to the main it carries all the same, so dropping a row
// main added is refused. Planted failure: the bases are the release's alone,
// and the drop passes.
func TestAHeadBoundForMainThatMergedTheReleaseKeepsMainsRows(t *testing.T) {
	t.Parallel()
	dir := baseFixture(t)
	git(t, dir, "switch", "-q", "-c", "home", "refs/remotes/origin/main")
	git(t, dir, "merge", "-q", "--no-edit", "refs/remotes/origin/release-1")
	git(t, dir, "rm", "-q", "main-only.txt")
	git(t, dir, "commit", "-q", "-m", "a row main added dropped")
	got := lostAgainstABase(t.Context(), t, dir, "main-only.txt", "release.txt")
	if !slices.Equal(got, []string{"main-only.txt"}) {
		t.Errorf("lost against a base = %v, want main-only.txt", got)
	}
}

// TestTheGateHoldsTheHeadToEveryBaseAndSaysAFindingOnce: with two bases, the marks record at the second alone, a head
// that moved the record back fails the gate, and a loss both bases show is one
// line, not two. Planted failure: gateCommand holds the head to bases[:1], and
// the loss at the second base passes.
func TestTheGateHoldsTheHeadToEveryBaseAndSaysAFindingOnce(t *testing.T) {
	t.Parallel()
	b := strings.Repeat("b", 64)
	old := "changes:\n  - {digest: \"sha256:" + strings.Repeat("a", 64) + "\", reason: \"an old line of the record\"}\n"
	want := "the change record has no base, and main's has the base " + b
	run := func(t *testing.T, held ...string) (int, string) {
		t.Helper()
		dir := copyGate(t)
		p := filepath.Join(dir, filepath.FromSlash(surface.MarksFile))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
		m := fakeMachine(t, dir)
		m.bases = func(context.Context, string) []string { return []string{"first", "second"} }
		m.baseMarks = func(_ context.Context, _, ref string) ([]byte, error) {
			if slices.Contains(held, ref) {
				return []byte(marksRecord(b)), nil
			}
			return nil, nil
		}
		code, out, errs := doGate(m, dir)
		return code, out + errs
	}
	if code, out := run(t, "second"); code != exitFailed || !strings.Contains(out, want) {
		t.Errorf("a loss against the second base alone: exit %d, want %d saying %q\n%s", code, exitFailed, want, out)
	}
	if code, out := run(t, "first", "second"); code != exitFailed || strings.Count(out, want) != 1 {
		t.Errorf("a loss against both: exit %d, %d times in\n%s\nwant a failure naming it once", code, strings.Count(out, want), out)
	}
}
