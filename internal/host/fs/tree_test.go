package fs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-files/confine"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/port"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestARepoLetsGoOfItsFolderOnClose: Close closes the folder a Repo holds
// open, so nothing of it holds the folder, which Windows would refuse to
// remove. The zero Repo closes as nil. Planted
// failure: a Close that does nothing, after which the Repo still reads.
func TestARepoLetsGoOfItsFolderOnClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := hostfs.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := src.ReadFile("a.yaml"); err != nil {
		t.Fatalf("ReadFile before Close: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if got, err := src.ReadFile("a.yaml"); !errors.Is(err, os.ErrClosed) {
		t.Errorf("ReadFile after Close = %q, %v; want os.ErrClosed", got, err)
	}
	if err := (hostfs.Repo{}).Close(); err != nil {
		t.Errorf("Close of the zero Repo = %v, want nil", err)
	}
}

func TestAFileInTheRepositoryIsRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := hostfs.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := fs.ReadFile(src, "a.yaml")
	if err != nil || string(got) != "x: 1\n" {
		t.Errorf("ReadFile = %q, %v; want the file", got, err)
	}
	if _, err := fs.ReadFile(src, "missing.yaml"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: %v, want fs.ErrNotExist", err)
	}
}

func TestALinkOutOfTheRepositoryIsNotFollowed(t *testing.T) {
	t.Parallel()
	outside, dir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, filepath.Join(outside, "secret"), filepath.Join(dir, "link"))
	src, err := hostfs.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, err := fs.ReadFile(src, "link"); err == nil {
		t.Errorf("read %q through a link out of the root, want a refusal", got)
	}
	if _, err := fs.ReadFile(src, "../secret"); err == nil {
		t.Error("read ../secret, want a refusal")
	}
}

func TestAFolderThatIsNotThereIsAnError(t *testing.T) {
	t.Parallel()
	if _, err := hostfs.Open(filepath.Join(t.TempDir(), "none")); err == nil {
		t.Error("Open of a missing folder: want an error")
	}
}

// chainOfLinks makes links l1 to l<n> inside base, each to the next and the
// last to a path that does not exist outside base, and answers l1.
func chainOfLinks(t *testing.T, base string, n int) string {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "missing")
	for i := 1; i <= n; i++ {
		target := filepath.Join(base, "l"+strconv.Itoa(i+1))
		if i == n {
			target = missing
		}
		testenv.Link(t, target, filepath.Join(base, "l"+strconv.Itoa(i)))
	}
	return filepath.Join(base, "l1")
}

// TestAChainOrALoopOfLinksIsOutside: a chain of links that ends outside the
// folder is outside however long it is, and one past go-files' limit, as a
// loop is, is outside too: no length of chain is given up on and judged
// inside. Planted failure: outside answering false on
// confine.ErrLoop (the 40 links and the loop are judged inside).
func TestAChainOrALoopOfLinksIsOutside(t *testing.T) {
	t.Parallel()
	for _, links := range []int{1, 40} {
		base, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if first := chainOfLinks(t, base, links); !hostfs.Outside(base, first) {
			t.Errorf("a chain of %d links that ends outside: outside = false, want true", links)
		}
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, filepath.Join(base, "b"), filepath.Join(base, "a"))
	testenv.Link(t, filepath.Join(base, "a"), filepath.Join(base, "b"))
	if !hostfs.Outside(base, filepath.Join(base, "a")) {
		t.Error("a loop of two links: outside = false, want true")
	}
}

// TestAFolderLinkIsJudgedWhereItLeads: a folder link inside the folder, made
// by go-files' confine.LinkFolder (a junction on Windows, a symbolic link
// elsewhere), is outside when it leads out and inside when it leads in; and a
// link whose relative target passes another link and then .. (l -> d/../x,
// d leading out) is judged where the system takes it, outside, never cleaned
// to the folder's own x first. Each path past a link is
// joined by hand, never by filepath.Join, which cleans a .. away. Planted
// failure: outside back on filepath.EvalSymlinks (on Windows the junction out
// is judged inside).
func TestAFolderLinkIsJudgedWhereItLeads(t *testing.T) {
	t.Parallel()
	top, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base, away := filepath.Join(top, "tree"), filepath.Join(top, "away")
	write(t, base, map[string]string{"in/a": "a", "x": "x"})
	write(t, away, map[string]string{"sub/x": "x", "x": "x"})
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	for at, target := range map[string]string{"junc-out": away, "junc-in": filepath.Join(base, "in")} {
		if err := confine.LinkFolder(root, target, at); err != nil {
			t.Fatalf("LinkFolder(%s -> %s): %v", at, target, err)
		}
	}
	for p, want := range map[string]bool{
		filepath.Join(base, "junc-out"):        true,
		filepath.Join(base, "junc-out", "x"):   true,
		filepath.Join(base, "junc-out", "new"): true,
		filepath.Join(base, "junc-in"):         false,
		filepath.Join(base, "junc-in", "a"):    false,
		filepath.Join(base, "x"):               false,
	} {
		if got := hostfs.Outside(base, p); got != want {
			t.Errorf("outside(%s) = %v, want %v", p, got, want)
		}
	}
	testenv.Link(t, filepath.Join(away, "sub"), filepath.Join(base, "d"))
	l := filepath.Join(base, "l")
	testenv.Link(t, filepath.FromSlash("d/../x"), l)
	// The want is the system's own: which x it opens through l. A system
	// that walks .. from where d leads opens away's; one that reads the
	// target by its spelling opens the tree's.
	opened, err := os.Stat(l)
	if err != nil {
		t.Fatal(err)
	}
	awayX, err := os.Stat(filepath.Join(away, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if want := os.SameFile(opened, awayX); hostfs.Outside(base, l) != want {
		t.Errorf("l -> d/../x with d leading out: outside = %v, want %v, as the system opens it", !want, want)
	}
}

func TestClosingATreeLetsGoOfItsFoldersAndIsSafeTwice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tree := &hostfs.Tree{}
	root, err := tree.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := tree.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := tree.Read(root, "a.yaml"); err == nil {
		t.Error("a read after Close answered nothing wrong, want the root to be unknown")
	}
	if err := tree.Close(); err != nil {
		t.Errorf("a second Close: %v, want nil", err)
	}
	if err := (&hostfs.Tree{}).Close(); err != nil {
		t.Errorf("the zero Tree closed: %v, want nil", err)
	}
}

// TestALinkInsideTheRepositoryIsNotFollowed: Repo opens every file through go-files'
// confine.Open, so a file that is a link, or sits in a folder that is one, is
// refused even when the link leads to a file of the repository, and the
// refusal says why, for audit's "not checked" says it as it is. The control:
// the file the links lead to reads. Planted failure: Repo.open back on
// root.OpenFile, which follows a link inside the root, and both links read.
func TestALinkInsideTheRepositoryIsNotFollowed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real", "a.yaml"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, filepath.Join("real", "a.yaml"), filepath.Join(dir, "a.yaml"))
	testenv.Link(t, "real", filepath.Join(dir, "linked"))
	src, err := hostfs.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = src.Close() })
	for _, name := range []string{"a.yaml", "linked/a.yaml"} {
		got, err := src.ReadFile(name)
		if err == nil {
			t.Errorf("ReadFile(%s) = %q through a link inside the repository, want a refusal", name, got)
			continue
		}
		if !errors.Is(err, port.ErrOutside) || errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "no link in the repository is followed") {
			t.Errorf("ReadFile(%s) = %v, want port.ErrOutside that says no link is followed", name, err)
		}
	}
	if got, err := src.ReadFile("real/a.yaml"); err != nil || string(got) != "x: 1\n" {
		t.Errorf("the control: ReadFile(real/a.yaml) = %q, %v, want the file", got, err)
	}
}
