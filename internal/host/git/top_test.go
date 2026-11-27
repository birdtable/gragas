package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/model"
)

// outerWithSub is a repository that tracks a.txt, sub/a.txt and sub/only.txt,
// and the folder sub of it as a root.
func outerWithSub(t *testing.T) (home string, outer, sub model.Root) {
	t.Helper()
	home, outer = fixture(t, nil)
	dir := string(outer)
	write(t, dir, map[string]string{"a.txt": "outer\n", "sub/a.txt": "sub\n", "sub/only.txt": "only\n"})
	story(t, home, dir, "add", "-A", ".")
	story(t, home, dir, "commit", "-q", "-m", "outer")
	return home, outer, model.Root(filepath.Join(dir, "sub"))
}

func paths(es []model.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Path
	}
	return out
}

// TestAFolderBelowTheTopOfItsRepositoryIsNotReady: a root whose .git is an
// empty folder, below a real repository, is listed by git from that
// repository's top, so a.txt would be read as sub/a.txt's text and the other
// two would be unreadable. The
// listing of the index, of a rev and a blob of it are each not ready, and
// none lists the outer repository's paths. Planted failure: the guard gone
// from the listing.
func TestAFolderBelowTheTopOfItsRepositoryIsNotReady(t *testing.T) {
	t.Parallel()
	home, _, sub := outerWithSub(t)
	if err := os.Mkdir(filepath.Join(string(sub), ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	g := adapter(t, configtest.Git(t), home)
	for name, call := range map[string]func() ([]model.Entry, error){
		"Files(index)": func() ([]model.Entry, error) { return g.Files(t.Context(), sub, "") },
		"Files(HEAD)":  func() ([]model.Entry, error) { return g.Files(t.Context(), sub, "HEAD") },
		"Blob": func() ([]model.Entry, error) {
			_, err := g.Blob(t.Context(), sub, "HEAD", "a.txt")
			return nil, err
		},
	} {
		es, err := call()
		if nr, ok := errors.AsType[*git.NotReady](err); !ok || strings.Contains(nr.Observed(), string(sub)) {
			t.Errorf("%s below the top: %v, want not ready, naming no path", name, err)
		}
		if len(es) != 0 {
			t.Errorf("%s below the top listed %v", name, paths(es))
		}
	}
}

// TestARealRepositoryBelowAnotherRealOneStillOpens: a folder that holds its
// own repository is the top of it, whatever repository is above, and lists
// its own files from its own top. Planted failure: the guard refusing any
// folder that has a repository above it.
func TestARealRepositoryBelowAnotherRealOneStillOpens(t *testing.T) {
	t.Parallel()
	home, outer, sub := outerWithSub(t)
	story(t, home, string(sub), "init", "-q")
	story(t, home, string(sub), "add", "-A", ".")
	story(t, home, string(sub), "commit", "-q", "-m", "inner")
	g := adapter(t, configtest.Git(t), home)
	for _, rev := range []string{"", "HEAD"} {
		es, err := g.Files(t.Context(), sub, rev)
		if want := []string{"a.txt", "only.txt"}; err != nil || !slices.Equal(paths(es), want) {
			t.Errorf("Files(%q) of the inner repository: %v, %v, want %v", rev, paths(es), err, want)
		}
	}
	if b, err := g.Blob(t.Context(), sub, "HEAD", "only.txt"); err != nil || string(b) != "only\n" {
		t.Errorf("Blob of the inner repository: %q, %v", b, err)
	}
	es, err := g.Files(t.Context(), outer, "")
	if want := []string{"a.txt", "sub/a.txt", "sub/only.txt"}; err != nil || !slices.Equal(paths(es), want) {
		t.Errorf("Files of the outer repository: %v, %v, want %v", paths(es), err, want)
	}
}

// TestARootInsideTheGitFolderIsNotReady: git run in .git or in .git/objects
// is in no work tree and prints no prefix, so a prefix alone would call it the
// top and list the repository's paths under it.
// Planted failure: the guard reading the prefix alone.
func TestARootInsideTheGitFolderIsNotReady(t *testing.T) {
	t.Parallel()
	home, outer, _ := outerWithSub(t)
	g := adapter(t, configtest.Git(t), home)
	for name, root := range map[string]model.Root{
		".git":         model.Root(filepath.Join(string(outer), ".git")),
		".git/objects": model.Root(filepath.Join(string(outer), ".git", "objects")),
	} {
		for _, rev := range []string{"", "HEAD"} {
			es, err := g.Files(t.Context(), root, rev)
			if _, ok := errors.AsType[*git.NotReady](err); !ok || len(es) != 0 {
				t.Errorf("Files(%q) in %s: %v, %v, want not ready and nothing listed", rev, name, paths(es), err)
			}
		}
	}
}
