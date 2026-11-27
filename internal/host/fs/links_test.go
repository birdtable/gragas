package fs_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/filestest"
	testenv "github.com/birdtable/go-testkit"
	hostfs "github.com/birdtable/gragas/internal/host/fs"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// plant puts each link of links under dir, its target as given.
func plant(t *testing.T, dir string, links map[string]string) {
	t.Helper()
	for p, target := range links {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		testenv.Link(t, filepath.FromSlash(target), full)
	}
}

func TestTheAdaptersPassTheLinksSuite(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	contracttest.Links(t, func(t *testing.T, files, links map[string]string) (port.Tree, port.Writer, string) {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "root")
		write(t, dir, files)
		write(t, filepath.Join(filepath.Dir(dir), contracttest.Beyond), contracttest.BeyondFiles())
		plant(t, dir, links)
		tr := newTree(t)
		return tr, hostfs.Writer{Tree: tr}, dir
	})
}

// dotGit is a checkout whose .git holds a config and a HEAD, and the bytes
// of every file under .git, so a test sees that nothing there changed.
func dotGit(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	write(t, dir, map[string]string{
		".git/config": "[core]\n\tbare = false\n",
		".git/HEAD":   "ref: refs/heads/main\n",
	})
	return snapshot(t, filepath.Join(dir, ".git"))
}

// snapshot is every entry under dir by path, a file as its bytes.
func snapshot(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	got := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			got[p+"/"] = nil
			return nil
		}
		data, err := os.ReadFile(p)
		got[p] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestALinkIntoDotGitCarriesNoWrite is a reviewer's probe, over
// real host/fs and apply's order: a checkout that commits
// state -> .git and history.jsonl -> .git/config, both inside the root, and
// a run whose scope is state/* and history.jsonl. The check's Read of each
// target, the Replace of state/HEAD and the Append to history.jsonl are
// each refused as ErrOutside, which apply's check turns into a refusal, and
// nothing under .git changes. The control is the same run with the two
// paths real files: it writes them.
func TestALinkIntoDotGitCarriesNoWrite(t *testing.T) {
	t.Parallel()
	for _, linked := range []bool{true, false} {
		dir := t.TempDir()
		before := dotGit(t, dir)
		if linked {
			plant(t, dir, map[string]string{"state": ".git", "history.jsonl": ".git/config"})
		} else {
			write(t, dir, map[string]string{"state/HEAD": "ref: refs/heads/main\n", "history.jsonl": "[core]\n"})
		}
		tr := newTree(t)
		w := hostfs.Writer{Tree: tr}
		root, err := tr.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		var errs []error
		for _, p := range []string{"state/HEAD", "history.jsonl"} {
			_, err := tr.Read(root, p)
			errs = append(errs, err)
		}
		errs = append(errs,
			w.Replace(root, "state/HEAD", []byte("written by apply\n")),
			w.Append(root, "history.jsonl", []byte(`{"who":"x"}`)))
		for i, err := range errs {
			if linked && !errors.Is(err, port.ErrOutside) {
				t.Errorf("step %d through a link into .git = %v, want ErrOutside", i, err)
			}
			if !linked && err != nil {
				t.Errorf("the control: step %d = %v", i, err)
			}
		}
		if after := snapshot(t, filepath.Join(dir, ".git")); !equalTrees(before, after) {
			t.Errorf("linked=%v: .git changed:\nbefore %q\nafter  %q", linked, before, after)
		}
		if !linked {
			if got, _ := os.ReadFile(filepath.Join(dir, "state", "HEAD")); string(got) != "written by apply\n" {
				t.Errorf("the control: state/HEAD = %q", got)
			}
		}
	}
}

func equalTrees(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for p, data := range a {
		other, ok := b[p]
		if !ok || !bytes.Equal(data, other) {
			return false
		}
	}
	return true
}

// TestALinkIsRefusedWhateverCaseNamesIt: on a file system that folds case,
// as darwin's does, State/HEAD walks to the link at state and is refused
// too. Where case is kept, State is missing and the write makes a real
// folder, which is the control.
func TestALinkIsRefusedWhateverCaseNamesIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := dotGit(t, dir)
	plant(t, dir, map[string]string{"state": ".git"})
	folds := false
	if _, err := os.Lstat(filepath.Join(dir, "STATE")); err == nil {
		folds = true
	}
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = hostfs.Writer{Tree: tr}.Replace(root, "State/HEAD", []byte("x\n"))
	switch {
	case folds && !errors.Is(err, port.ErrOutside):
		t.Errorf("Replace(State/HEAD) on a folding file system = %v, want ErrOutside", err)
	case !folds && err != nil:
		t.Errorf("the control, a file system that keeps case: Replace(State/HEAD) = %v", err)
	}
	if after := snapshot(t, filepath.Join(dir, ".git")); !equalTrees(before, after) {
		t.Errorf(".git changed: %q", after)
	}
	if !folds {
		t.Log("this volume keeps case; only the control ran")
	}
}

// TestAFileAsAMiddleComponentIsRefusedByTheWalk: the walk names it, as the
// Path of its *fs.PathError, so no later step meets it.
func TestAFileAsAMiddleComponentIsRefusedByTheWalk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"f": "f\n"})
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := hostfs.Writer{Tree: tr}
	for name, do := range map[string]func() error{
		"Read":    func() error { _, err := tr.Read(root, "f/x"); return err },
		"Replace": func() error { return w.Replace(root, "f/x", []byte("x")) },
		"Append":  func() error { return w.Append(root, "f/x", []byte("x")) },
	} {
		pe, ok := errors.AsType[*fs.PathError](do())
		if !ok || pe.Op != "walk" || pe.Path != "f" {
			t.Errorf("%s(f/x) = %v, want the walk to name f", name, pe)
		}
	}
	if got, err := tr.Read(root, "f"); err != nil || string(got) != "f\n" {
		t.Errorf("the control: Read(f) = %q, %v", got, err)
	}
}

// TestAFileSwappedAfterTheWalkIsRefused: the open found a file other than
// the one the walk found, as when a link is put in its place between the
// two, and nothing is read or written. The control is the same file.
func TestAFileSwappedAfterTheWalkIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"walked": "w\n", "opened": "o\n"})
	walked, err := os.Lstat(filepath.Join(dir, "walked"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"opened", "walked"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = hostfs.Same("append", "walked", f, walked, false)
		if cerr := f.Close(); cerr != nil {
			t.Fatal(cerr)
		}
		if name == "opened" && !errors.Is(err, port.ErrOutside) {
			t.Errorf("a swapped file = %v, want ErrOutside", err)
		}
		if name == "walked" && err != nil {
			t.Errorf("the control, the walked file = %v", err)
		}
	}
	f, err := os.Open(filepath.Join(dir, "opened"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := hostfs.Same("append", "walked", f, nil, false); !errors.Is(err, port.ErrOutside) {
		t.Errorf("a file the walk found missing, opened without making it = %v, want ErrOutside", err)
	}
	if err := hostfs.Same("append", "walked", f, nil, true); err != nil {
		t.Errorf("the control, a file this open made = %v", err)
	}
}

// TestALinkBesideTheRootIsNotOnThePath: a link to a folder in the root at
// a path no write names leaves every other write alone, Stat and List
// included, which follow it (they are on no written or checked path).
func TestALinkBesideTheRootIsNotOnThePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"real/a": "a\n"})
	plant(t, dir, map[string]string{"alias": "real"})
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := (hostfs.Writer{Tree: tr}).Replace(root, "real/a", []byte("b\n")); err != nil {
		t.Errorf("Replace(real/a) = %v", err)
	}
	if e, err := tr.Stat(root, "alias/a"); err != nil || e.Size != 2 {
		t.Errorf("Stat(alias/a) = %+v, %v", e, err)
	}
	if _, err := tr.List(root, "", model.Every); err != nil {
		t.Errorf("List = %v", err)
	}
}

// TestALinkToAProgramOutsideIsNotRunnable: List reports a link as itself, and
// a link is never runnable, however its target is judged; a program in the
// root is, by Stat, as the control that the plant would read as runnable if
// followed, and a plain file in it is not. Each program takes
// this test binary's own suffix, so this system would run it by its name. List
// judges nothing, so every entry it answers is not runnable, the
// program's too; Stat's answer is confine.IsRunnable's, from the file in the
// root. TestStatOfALinkToAProgramInTheRootIsNotRunnable holds the judge on a
// link inside.
func TestALinkToAProgramOutsideIsNotRunnable(t *testing.T) {
	t.Parallel()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog := "prog" + filepath.Ext(self)
	top := t.TempDir()
	dir, away := filepath.Join(top, "tree"), filepath.Join(top, "away")
	write(t, dir, map[string]string{"keep": "x"})
	write(t, away, map[string]string{"keep": "x"})
	filestest.WriteProgram(t, filepath.Join(away, prog), []byte("x\n"))
	filestest.WriteProgram(t, filepath.Join(dir, prog), []byte("x\n"))
	testenv.Link(t, filepath.Join(away, prog), filepath.Join(dir, "out-"+prog))
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.List(root, "", model.Every)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{prog: true, "out-" + prog: true, "keep": true}
	for _, e := range got {
		if want[e.Path] {
			if e.Exec {
				t.Errorf("List: %s runnable = true, want false", e.Path)
			}
			delete(want, e.Path)
		}
	}
	if len(want) != 0 {
		t.Errorf("List = %+v, missing %v", got, want)
	}
	for name, runs := range map[string]bool{prog: true, "keep": false} {
		e, err := tr.Stat(root, name)
		if err != nil || e.Exec != runs {
			t.Errorf("Stat(%s) = %+v, %v, want runnable %v", name, e, err, runs)
		}
	}
}
