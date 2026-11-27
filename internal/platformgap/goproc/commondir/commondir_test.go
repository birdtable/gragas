package commondir_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-proc/gitenv/gittest"

	"github.com/birdtable/gragas/internal/platformgap/goproc/commondir"
)

// TestCommonDirIsTheMainCheckoutsGitDirFromAWorktree: in a clone it is the
// clone's .git, and in a linked worktree, where .git is a file, it is the same
// folder; a folder that is no checkout is an error.
func TestCommonDirIsTheMainCheckoutsGitDirFromAWorktree(t *testing.T) {
	t.Parallel()
	clone := gittest.Repo(t, map[string]string{"a.md": "a\n"}, nil)
	cmd := exec.CommandContext(t.Context(), "git", "worktree", "add", "-q", "--detach", "../wt")
	cmd.Dir = clone
	cmd.Env = gittest.Git(t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	worktree := filepath.Join(clone, "..", "wt")
	want, err := filepath.EvalSymlinks(filepath.Join(clone, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{"clone": clone, "worktree": worktree} {
		got, err := commondir.CommonDir(t.Context(), dir)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, err = filepath.EvalSymlinks(got); err != nil || got != want {
			t.Errorf("%s: common dir %q (%v), want %q", name, got, err, want)
		}
	}
	if _, err := commondir.CommonDir(t.Context(), t.TempDir()); err == nil {
		t.Error("a folder that is no checkout had a common dir")
	}
}
