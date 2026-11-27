package git

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/model"
)

// source is a fetch's source planted for real, in a folder of its own only
// this account can write, with its links resolved.
type source struct{ base, book string }

func newSource(t *testing.T) source {
	t.Helper()
	base, err := filepath.EvalSymlinks(filestest.Private(t))
	if err != nil {
		t.Fatal(err)
	}
	return source{base: base, book: filepath.Join(base, "book")}
}

// repo makes a git dir at p, as git's is_git_directory reads one.
func repo(t *testing.T, p string) {
	t.Helper()
	for _, d := range []string{p, filepath.Join(p, "objects"), filepath.Join(p, "refs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file(t, filepath.Join(p, "HEAD"))
}

func file(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func remove(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
}

// TestAFetchSourceIsHeldToAGitDirGitOpensAsItIs: the source of
// a fetch and its git dir are walked as a program is, so a source or
// a git dir others can write is refused with the chmod line. A .git that is
// a file, and a git dir that holds commondir, are refused: each points git
// at another folder's config and hooks. So is a source git would look past,
// to a folder beside it: one that is no git dir and holds no .git, and one
// whose .git is a folder git would not open. A source that is not there is
// not ready, and one the walk refuses with no fix of its own, as a file, is
// refused; each gives the fetch's one fix, since no setting names a source.
// A bare source and a work tree of this account's pass, answering the git
// dir. Every name is planted for real: another account's source is
// go-files's own row, and its fix this adapter's (the next test). Planted
// failures: the source not walked; its .git not walked; the .git file
// taken; commondir not looked for; a bare source taken with no git dir; a
// .git folder taken that git would not open; the fetch's fix dropped for
// the settings line a programs.* path gets.
func TestAFetchSourceIsHeldToAGitDirGitOpensAsItIs(t *testing.T) {
	t.Parallel()
	const itself = "fetch from the repository itself, or its work tree"
	const names = "fetch from the repository it names"
	// theWalks stands for the trust walk's own words and fix of the folder a
	// case loosens, gitDir under book: go-files' on this system in
	// host/trust's words, which the refusal must hand on as they are, and no
	// line of one system's written here. Such
	// a case has no says of its own.
	const theWalks = "the trust walk's own"
	const fix = "fetch from a repository or a mirror this account or root owns, which no other account can write"
	bare := func(t *testing.T, s source) { t.Helper(); repo(t, s.book) }
	tree := func(t *testing.T, s source) { t.Helper(); repo(t, filepath.Join(s.book, ".git")) }
	cases := []struct {
		name  string
		plant func(*testing.T, source)
		word  model.Word
		says  func(source) string
		hint  string
		// gitDir is where a fetch is handed, under book, when it passes, and
		// the folder whose words and fix are wanted when the hint is theWalks
		gitDir string
	}{
		{"a source others can write", func(t *testing.T, s source) {
			t.Helper()
			bare(t, s)
			trusttest.Loosen(t, s.book, trusttest.OthersWrite)
		}, model.Refused, nil, theWalks, "."},
		{"a git dir others can write", func(t *testing.T, s source) {
			t.Helper()
			tree(t, s)
			trusttest.Loosen(t, filepath.Join(s.book, ".git"), trusttest.OthersWrite)
		}, model.Refused, nil, theWalks, ".git"},
		{"a .git that is a file", func(t *testing.T, s source) { t.Helper(); bare(t, s); file(t, filepath.Join(s.book, ".git")) }, model.Refused,
			func(s source) string {
				return filepath.Join(s.book, ".git") + ", the git dir of the source at " + s.book + ", is a file"
			}, names, ""},
		{"a bare source holding commondir", func(t *testing.T, s source) { t.Helper(); bare(t, s); file(t, filepath.Join(s.book, "commondir")) }, model.Refused,
			func(s source) string { return filepath.Join(s.book, "commondir") + " names another folder" }, names, ""},
		{"a git dir holding commondir", func(t *testing.T, s source) {
			t.Helper()
			tree(t, s)
			file(t, filepath.Join(s.book, ".git", "commondir"))
		}, model.Refused,
			func(s source) string { return filepath.Join(s.book, ".git", "commondir") + " names another folder" }, names, ""},
		{"a source that is no git dir and holds no .git, as an empty folder beside book.git", func(t *testing.T, s source) { t.Helper(); mkdirs(t, s.book) }, model.Refused,
			func(s source) string {
				return s.book + " is no git dir and holds no .git, and git would then open " + s.book + ".git beside it"
			}, itself, ""},
		{"a bare source whose HEAD is a link", func(t *testing.T, s source) {
			t.Helper()
			bare(t, s)
			remove(t, filepath.Join(s.book, "HEAD"))
			testenv.Link(t, "refs", filepath.Join(s.book, "HEAD"))
		}, model.Refused, func(s source) string { return s.book + " is no git dir" }, itself, ""},
		{"a bare source whose objects is a file", func(t *testing.T, s source) {
			t.Helper()
			bare(t, s)
			remove(t, filepath.Join(s.book, "objects"))
			file(t, filepath.Join(s.book, "objects"))
		}, model.Refused, func(s source) string { return s.book + " is no git dir" }, itself, ""},
		{"a .git folder with no refs", func(t *testing.T, s source) {
			t.Helper()
			tree(t, s)
			remove(t, filepath.Join(s.book, ".git", "refs"))
			file(t, filepath.Join(s.book, ".git", "refs"))
		}, model.Refused, func(s source) string {
			return filepath.Join(s.book, ".git") + " is a folder git would not open as a git dir, and git would then look past it"
		}, itself, ""},
		{"a source that is not there", func(*testing.T, source) {}, model.NotReady,
			func(s source) string {
				return "the source is " + s.book + ", where there is no file, so nothing is fetched"
			}, fix, ""},
		{"a source that is a file", func(t *testing.T, s source) { t.Helper(); file(t, s.book) }, model.Refused,
			func(s source) string { return "the source at " + s.book + " is a regular file, not a folder" }, fix, ""},
		{"a bare source of this account's", bare, "", nil, "", "."},
		{"a work tree of this account's", tree, "", nil, "", ".git"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := newSource(t)
			c.plant(t, s)
			g := &Git{cfg: Config{Run: "run-1"}}
			got, err := g.sourceHeld(s.book)
			if c.word == "" {
				if want := filepath.Join(s.book, c.gitDir); err != nil || got != want {
					t.Errorf("%q, %v: want it fetched from %s", got, err, want)
				}
				return
			}
			var r interface {
				Word() model.Word
				Observed() string
				Hint() string
			}
			if nr, ok := errors.AsType[*NotReady](err); ok {
				r = nr
			} else if rf, ok := errors.AsType[*Refused](err); ok {
				r = rf
			}
			hint, fixed := c.hint, r != nil && strings.HasPrefix(r.Hint(), c.hint)
			var says string
			if c.hint == theWalks {
				as := fetchSource()
				if c.gitDir != "." {
					as = fetchGitDir()
				}
				u := walked(t, as, filepath.Join(s.book, c.gitDir))
				says, hint = u.Observed, u.Fix
				fixed = r != nil && r.Hint() == hint
			} else {
				says = c.says(s)
			}
			switch {
			case r == nil || got != "" || r.Word() != c.word || !strings.Contains(r.Observed(), says) || !fixed:
				t.Errorf("%q, %v: want %s, %q, with the fix %q", got, err, c.word, says, hint)
			case !strings.Contains(r.Observed(), "nothing is fetched"):
				t.Errorf("%q: want it to say nothing is fetched", r.Observed())
			}
		})
	}
}

// TestEachAnswerOfTheWalkIsTheAdaptersOwn: what the
// walk answers is turned into this adapter's words. A name another account
// owns, Owned, is refused with the mirror line for a fetch's source, which
// names CVE-2024-32004, and with the walk's chown line for git, which a
// setting names; a refusal with no fix of the walk's own takes the fix of
// its subject; a read that fails is *Failed, naming the path walked;
// nothing there is not ready. A test cannot make a name another account
// owns, so the walk's answers are given. Planted failures: Owned ignored;
// the mirror line given to git; a fix of the walk's replaced; the walk's
// failure handed on without the path.
func TestEachAnswerOfTheWalkIsTheAdaptersOwn(t *testing.T) {
	t.Parallel()
	g := &Git{cfg: Config{Run: "run-1"}}
	const chown = "sudo chown -h root '/repo/book'"
	owned := &hosttrust.Unsafe{Observed: "the source at /repo/book: /repo/book is owned by uid:4242, which is not trusted here", Fix: chown, Owned: true}
	for _, c := range []struct {
		name string
		s    subject
		err  error
		hint string
	}{
		{"another account's source", fetchSource(), owned, "fetch from a mirror this account or root owns"},
		{"another account's git", gitProgram(), owned, chown},
		{"a source others can write", fetchSource(), &hosttrust.Unsafe{Observed: "x", Fix: "chmod go-w '/repo/book'"}, "chmod go-w '/repo/book'"},
		{"a git the walk has no fix for", gitProgram(), &hosttrust.Unsafe{Observed: "x"}, "set programs.git in the settings file to the path of git as installed"},
	} {
		_, err := g.answered(c.s, "/repo/book", "", c.err)
		if rf, ok := errors.AsType[*Refused](err); !ok || !strings.HasPrefix(rf.Hint(), c.hint) {
			t.Errorf("%s: %v, want refused with the fix %q", c.name, err, c.hint)
		}
	}
	if _, err := g.answered(fetchSource(), "/repo/book", "", owned); func() bool {
		rf, ok := errors.AsType[*Refused](err)
		return !ok || !strings.Contains(rf.Hint(), "CVE-2024-32004")
	}() {
		t.Errorf("another account's source: the fix %v does not name CVE-2024-32004", err)
	}
	if _, err := g.answered(gitProgram(), "/x/git", "", &hosttrust.Failed{What: "the path of git could not be read", Err: fs.ErrPermission}); !isFailedErr(err) || !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "the path of git could not be read, at /x/git") {
		t.Errorf("a read that fails: %v, want *Failed naming /x/git", err)
	}
	if _, err := g.answered(gitProgram(), "/x/git", "", fs.ErrNotExist); !errors.As(err, new(*NotReady)) {
		t.Errorf("nothing there: %v, want not ready", err)
	}
	if got, err := g.answered(gitProgram(), "/x/git", "/y/git", nil); err != nil || got != "/y/git" {
		t.Errorf("a pass: %q, %v, want /y/git", got, err)
	}
}

func isFailedErr(err error) bool {
	_, ok := errors.AsType[*Failed](err)
	return ok
}

// TestAFetchSourceThatCannotBeReadFails: a source whose .git
// cannot be read, since its folder may not be searched, is a failure naming
// the source, not a pass and not a refusal. Planted failure: the read's
// error taken as no .git.
func TestAFetchSourceThatCannotBeReadFails(t *testing.T) {
	t.Parallel()
	s := newSource(t)
	repo(t, s.book)
	trusttest.Deny(t, s.book)
	g := &Git{cfg: Config{Run: "run-1"}}
	got, err := g.sourceHeld(s.book)
	if f, ok := errors.AsType[*Failed](err); !ok || got != "" || !strings.Contains(f.What, s.book) || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("%q, %v, want a failure naming the source", got, err)
	}
}

func mkdirs(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

// walked is the trust walk's refusal of at, as the adapter walks it as s:
// what it saw and its fix. A test that loosened at wants the refusal to
// hand both on as they are, since the detail is go-files's on this system,
// mode bits on Unix and an access list on Windows, and no words of one
// system's are written in a test. A walk
// that passes, or refuses with no words or no fix, fails the test, so the
// plant is proved before the refusal is read.
func walked(t testing.TB, s subject, at string) *hosttrust.Unsafe {
	t.Helper()
	_, err := hosttrust.Walk(s.trust(), at, hosttrust.Who{})
	u, ok := errors.AsType[*hosttrust.Unsafe](err)
	if !ok || u.Observed == "" || u.Fix == "" {
		t.Fatalf("the trust walk of %s = %v: want a refusal with its words and its fix", at, err)
	}
	return u
}
