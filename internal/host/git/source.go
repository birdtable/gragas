package git

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// holdSource holds the source of a fetch, and its git dir, to this account
// or root before anything runs, and answers the git dir, the
// one path the fetch hands git. upload-pack runs under the source's config,
// and git 2.45.1 and later refuse a repository another account owns, but the
// version band reads only major.minor, so it cannot rule the fixed patch
// level in or out (CVE-2024-32004). Each is walked as a program is:
// every name from / down, every link judged before it is followed, and the
// last a folder that root or this account owns and no other account can
// write.
func (g *Git) holdSource(_ context.Context, from string) (string, error) {
	return g.sourceHeld(from)
}

// sourceHeld is holdSource, past what a fetch hands it.
//
// It picks the git dir as git's enter_repo does, and narrower. A work tree's
// git dir is its .git folder and a bare repository's is the source itself.
// enter_repo opens the first of from/.git, from, from.git/.git and from.git
// that is a git dir, so a .git folder git would not open, or a source that
// is no git dir and has no .git, would send git past the held folder to one
// beside it, which any account may make in a folder such as /tmp: each is
// refused. A .git that is a file, as a linked worktree's or a submodule's,
// and a git dir holding commondir are refused too: each points git at
// another folder, whose config and hooks are then the ones that run. Fetch
// hands git the answered path with upload-pack --strict, which opens that
// path as it is or nothing, so git opens the folder held here even where
// git would read its HEAD otherwise.
func (g *Git) sourceHeld(from string) (string, error) {
	s := fetchSource()
	dir, err := g.walkFor(s, from)
	if err != nil {
		return "", err
	}
	// none is what git would do were gitDir no git dir
	gitDir := filepath.Join(dir, ".git")
	var none string
	f, err := os.Lstat(gitDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		gitDir = dir
		none = from + " is no git dir and holds no .git, and git would then open " + from + ".git beside it"
	case err != nil:
		return "", g.failed("the git dir of the source "+from+" could not be read", err)
	case f.Mode().IsRegular():
		return "", s.refused(gitDir+", the git dir of the source at "+from+", is a file that names another folder, as a linked worktree's or a submodule's", "fetch from the repository it names")
	default:
		s = fetchGitDir()
		if gitDir, err = g.walkFor(s, gitDir); err != nil {
			return "", err
		}
		none = gitDir + " is a folder git would not open as a git dir, and git would then look past it for another"
	}
	common := filepath.Join(gitDir, "commondir")
	switch _, err := os.Lstat(common); {
	case err == nil:
		return "", s.refused(common+" names another folder whose config and hooks git would read for the source at "+from, "fetch from the repository it names")
	case !errors.Is(err, fs.ErrNotExist):
		return "", g.failed("the git dir of the source "+from+" could not be read", err)
	}
	switch err := gitDirAt(gitDir); {
	case errors.Is(err, errNoGitDir):
		return "", s.refused(none, "fetch from the repository itself, or its work tree")
	case err != nil:
		return "", g.failed("the git dir of the source "+from+" could not be read", err)
	}
	return gitDir, nil
}

// errNoGitDir is a folder that is not a git dir.
var errNoGitDir = errors.New("no git dir")

// gitDirAt is whether dir is a folder git opens as a git dir, read as git's
// is_git_directory reads one and narrower: a HEAD that is a plain file, and
// objects and refs that are folders, none of them a link. A name that is
// missing or of another kind is errNoGitDir; one that cannot be read is
// its error.
func gitDirAt(dir string) error {
	for _, n := range []struct {
		name   string
		folder bool
	}{{"HEAD", false}, {"objects", true}, {"refs", true}} {
		f, err := os.Lstat(filepath.Join(dir, n.name))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return errNoGitDir
		case err != nil:
			return err
		case n.folder && !f.IsDir(), !n.folder && !f.Mode().IsRegular():
			return errNoGitDir
		}
	}
	return nil
}
