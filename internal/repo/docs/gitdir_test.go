package docs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-proc/gitenv/gittest"

	"github.com/birdtable/gragas/internal/platformgap/goproc/commondir"
)

// commonGitDir is the git dir a path below .git is looked up in, as git itself
// names it, and only where .git is a file, in a worktree, naming a git dir
// elsewhere. In a clone .git is a folder and in an export there
// is none, so it is "" and the path is read from dir as before. A git that
// cannot answer ends the test, on the asker's own t. The lookup is
// commondir's, until the process library gives one.
func commonGitDir(t *testing.T, dir string) string {
	t.Helper()
	fi, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil || fi.IsDir() {
		return ""
	}
	out, err := commondir.CommonDir(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAPathBelowGitResolvesInAWorktree: in a worktree .git is a file, so a
// citation of .git/config (AGENTS.md and docs/checks.md name it) joined onto
// the tree found nothing, and both path tests were red in every worktree.
// The tree is a real git worktree add, made under the test's own
// folder; the same paths still resolve in a clone, and a path below .git that
// the common git dir lacks still does not.
func TestAPathBelowGitResolvesInAWorktree(t *testing.T) {
	t.Parallel()
	clone := gittest.Repo(t, map[string]string{"a.md": "a\n"}, nil)
	// A folder beside the clone, in the test's own temp folder, named by a literal.
	worktree := filepath.Join(clone, "..", "wt")
	cmd := exec.CommandContext(t.Context(), "git", "worktree", "add", "-q", "--detach", "../wt")
	cmd.Dir = clone
	cmd.Env = gittest.Git(t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	if fi, err := os.Lstat(filepath.Join(worktree, ".git")); err != nil || fi.IsDir() {
		t.Fatalf("the worktree's .git is not a file: %v, %v", fi, err)
	}
	for name, dir := range map[string]string{"worktree": worktree, "clone": clone} {
		exists := existsIn(t, dir)
		for _, rel := range []string{".git", ".git/config", ".git/hooks"} {
			if !exists(rel) {
				t.Errorf("in the %s, %s resolves to nothing", name, rel)
			}
		}
		if exists(".git/not-in-any-git-dir") {
			t.Errorf("in the %s, a path the git dir lacks resolved", name)
		}
	}
}
