package git_test

import (
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/model"
)

// TestGitRunsInAFolderWhosePathHoldsAnEquals: go-proc starts no program
// whose path holds =, and the folder a program runs in is no
// such path: a user's repository, or the home git reads, may be named with
// one, and git runs there as anywhere. It runs on every system, Linux among
// them, where the red run of tree cd39c006 was.
// Risk: a repository whose path holds =, as a test's folder named for a
// subtest "--apply=true" does, that no command of gragas can run git in.
func TestGitRunsInAFolderWhosePathHoldsAnEquals(t *testing.T) {
	t.Parallel()
	base := filestest.Private(t)
	home, repo := filepath.Join(base, "home=1"), filepath.Join(base, "book=1")
	for _, d := range []string{home, repo} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, home, repo, "init", "-q")
	gitIn(t, home, repo, "config", "user.name", "Ada Example")
	gitIn(t, home, repo, "config", "user.email", "ada@example.com")
	g := adapter(t, configtest.Git(t), home)
	if got, err := g.Who(t.Context(), model.Root(repo)); err != nil || got.Name != "Ada Example" {
		t.Errorf("git in %s: %+v, %v; want it run, and the repository's identity", repo, got, err)
	}
}

// TestAGitWhosePathHoldsAnEqualsIsRefusedNamingThePathAndTheFix: a git
// setting whose path, links followed, holds = is refused before anything
// runs, in this adapter's words: the setting, the path, why, and a fix that
// works, a folder whose path holds none. go-proc's own refusal names
// neither the path nor a fix. The control is the same program
// in a folder whose path holds no =.
// Risk: a git the user set that is never run, with no word of which path or
// what to change.
func TestAGitWhosePathHoldsAnEqualsIsRefusedNamingThePathAndTheFix(t *testing.T) {
	t.Parallel()
	home, root := fixture(t, nil)
	dir := filepath.Join(filestest.Private(t), "bin=1")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := git.StandIn(t, dir, mallory())
	g := adapter(t, path, home)
	_, err := g.Who(t.Context(), root)
	rf, ok := errors.AsType[*git.Refused](err)
	if !ok || err.Error() != rf.Error() || rf.Word() != model.Refused {
		t.Fatalf("git at %s: %v, want refused", path, err)
	}
	// the refusal quotes the path as Go does, a backslash doubled, which a native Windows path holds
	quoted := strings.Trim(strconv.Quote(path), `"`)
	if o := rf.Error(); !strings.Contains(o, "programs.git") || !strings.Contains(o, quoted) || !strings.Contains(o, "holds =") {
		t.Errorf("the refusal says %q; want the setting, the path %s, quoted as Go does, and that it holds =", o, path)
	}
	if h := rf.Hint(); !strings.HasPrefix(h, "install git in a folder whose path holds no =") {
		t.Errorf("the fix is %q; want a folder whose path holds no =", h)
	}
	if len(g.Ran()) != 0 {
		t.Errorf("git at %s ran: %s", path, git.Shown(g.Ran()))
	}
	_, ok2 := fakeGit(t, 0)
	if got, err := adapter(t, ok2, home).Who(t.Context(), root); err != nil || got.Name != "Mallory Example" {
		t.Errorf("the control, the same program in a folder with no =: %+v, %v", got, err)
	}
}
