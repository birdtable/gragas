package fs_test

import (
	"errors"
	"github.com/birdtable/go-files/confine"
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"
	hostfs "github.com/birdtable/gragas/internal/host/fs"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// newTree is a Tree that is closed when t ends, before t's temporary folders
// are removed: Windows refuses to remove a folder that is held open, and a
// Tree holds every root it opened. It asks for t's
// temporary folder first, so that the folder's removal is registered before
// the close and so runs after it, whichever comes first in the test.
func newTree(t *testing.T) *hostfs.Tree {
	t.Helper()
	t.TempDir()
	tr := &hostfs.Tree{}
	t.Cleanup(func() {
		if err := tr.Close(); err != nil {
			t.Errorf("close the tree: %v", err)
		}
	})
	return tr
}

// write puts each of files under dir.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTheTreeAdapterPassesTheFileNamesSuite: the Tree adapter is the adapter of
// port.FileNames too, its Portable
// being go-files' confine.Portable.
func TestTheTreeAdapterPassesTheFileNamesSuite(t *testing.T) {
	t.Parallel()
	contracttest.FileNames(t, func(*testing.T) port.FileNames { return &hostfs.Tree{} })
}

func TestTheTreeAdapterPassesTheTreeSuite(t *testing.T) {
	t.Parallel()
	contracttest.Tree(t, func(t *testing.T, files map[string]string) (port.Tree, string, contracttest.Outside) {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "root")
		write(t, dir, files)
		out := t.TempDir()
		write(t, out, map[string]string{"names": "outside\n"})
		return newTree(t), dir, contracttest.Outside{File: filepath.Join(out, "names")}
	})
}

// TestTheTreeAdapterPassesTheOwnedSuite: the one adapter that fills Tree
// fills Owned too, so the owned read is judged on its own.
func TestTheTreeAdapterPassesTheOwnedSuite(t *testing.T) {
	t.Parallel()
	contracttest.Owned(t, func(t *testing.T) (port.Owned, contracttest.Outside) {
		t.Helper()
		out := t.TempDir()
		write(t, out, map[string]string{"names": "outside\n", "loose": "outside\n"})
		trusttest.Loosen(t, filepath.Join(out, "loose"), trusttest.GroupWrites)
		return newTree(t), contracttest.Outside{File: filepath.Join(out, "names"), Refused: filepath.Join(out, "loose")}
	})
}

func TestTheWriterAdapterPassesTheWriterSuite(t *testing.T) {
	t.Parallel()
	contracttest.Writer(t, func(t *testing.T, files map[string]string) (port.Tree, port.Writer, string) {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "root")
		write(t, dir, files)
		tr := newTree(t)
		return tr, hostfs.Writer{Tree: tr}, dir
	})
}

// TestAWriterWritesOnlyInTheRootsItsTreeOpened: a root the tree never opened
// is refused, and the control, one it opened, writes.
func TestAWriterWritesOnlyInTheRootsItsTreeOpened(t *testing.T) {
	t.Parallel()
	dir, other := t.TempDir(), t.TempDir()
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := hostfs.Writer{Tree: tr}
	if err := w.Replace(model.Root(filepath.ToSlash(other)), "f", []byte("x")); err == nil {
		t.Error("Replace in a root the tree never opened succeeded")
	}
	if err := w.Append(model.Root(filepath.ToSlash(other)), "f", []byte("x")); err == nil {
		t.Error("Append in a root the tree never opened succeeded")
	}
	if _, err := os.Stat(filepath.Join(other, "f")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused write left %v", err)
	}
	if err := w.Replace(root, "f", []byte("x")); err != nil {
		t.Errorf("the control: Replace = %v", err)
	}
}

func TestALinkThatLeavesTheRootIsOutside(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	write(t, base, map[string]string{"secret": "s", "root/a": "a"})
	testenv.Link(t, "../secret", filepath.Join(base, "root", "link"))
	tr := newTree(t)
	root, err := tr.Open(filepath.Join(base, "root"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Read(root, "link"); !errors.Is(err, port.ErrOutside) {
		t.Errorf("Read through a link out of the root = %v, want ErrOutside", err)
	}
	if got, err := tr.Read(root, "a"); err != nil || string(got) != "a" {
		t.Errorf("Read(a) = %q, %v", got, err)
	}
}

func TestARootThatWasNotOpenedHereIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := (&hostfs.Tree{}).Read(model.Root("/repo"), "a"); err == nil {
		t.Error("Read of a root the tree never opened succeeded")
	}
}

func TestTheDiskListsOnlyEveryFile(t *testing.T) {
	t.Parallel()
	tr := newTree(t)
	root, err := tr.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.List(root, "", model.Tracked); err == nil {
		t.Error("a tracked listing was served from the disk")
	}
}

// TestTheDiskListsALinkAsSpecialAndAFileOrFolderAsNot: the listing marks an
// entry that is neither a folder
// nor a regular file Special, as a reader of entry files needs before it opens
// one; a link is planted here as the one such file every system makes, and a
// regular file and a folder are not marked. Planted failure: a listing that
// leaves Special unset, so a pipe is opened and read.
func TestTheDiskListsALinkAsSpecialAndAFileOrFolderAsNot(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rules", "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules", "a.yml"), []byte("id: a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, filepath.Join(dir, "rules", "a.yml"), filepath.Join(dir, "rules", "linked.yml"))
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	list, err := tr.List(root, "rules", model.Every)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"rules/a.yml": false, "rules/sub": false, "rules/linked.yml": true}
	for _, e := range list {
		if special, ok := want[e.Path]; ok {
			if e.Special != special {
				t.Errorf("%s: Special = %v, want %v", e.Path, e.Special, special)
			}
			delete(want, e.Path)
		}
	}
	if len(want) != 0 {
		t.Errorf("the listing lacks %v", want)
	}
}

func TestAnEmptyDirIsTheWorkingFolder(t *testing.T) {
	t.Chdir(t.TempDir())
	tr := newTree(t)
	root, err := tr.Open("")
	if err != nil {
		t.Fatal(err)
	}
	want, err := confine.Resolve(mustGetwd(t))
	if err != nil {
		t.Fatal(err)
	}
	if string(root) != filepath.ToSlash(want) {
		t.Errorf("Open(\"\") = %q, want %q", root, want)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}
