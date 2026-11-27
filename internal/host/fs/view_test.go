package fs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	testenv "github.com/birdtable/go-testkit"
	hostfs "github.com/birdtable/gragas/internal/host/fs"

	"github.com/birdtable/gragas/internal/model"
)

// seen is what a walk of a view finds at one path: a folder, a file's
// content, or that the read was refused.
type seen struct {
	dir     bool
	content string
	refused bool
}

// walkAll walks fsys from its root and reads every file it lists.
func walkAll(t *testing.T, fsys fs.FS) map[string]seen {
	t.Helper()
	got := map[string]seen{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			got[p] = seen{dir: true}
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		got[p] = seen{content: string(data), refused: err != nil}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return got
}

// TestTheViewReadsWhatTheOldRepoReads: audit reads a
// repository through Tree.FS as it reads one through the old Repo today, on
// one tree that holds a tool's config, a nested folder, a link inside the
// root to a file and to a folder, and a link that leads out. Each path is
// listed by both, and each file reads the same bytes or is refused by both:
// the links, inside and out, are refused, the files behind
// them read by their own paths. Planted failure: FS back on os.Root's bare
// FS(), which follows the link inside, and the file behind it reads.
func TestTheViewReadsWhatTheOldRepoReads(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	base := t.TempDir()
	dir := filepath.Join(base, "repo")
	write(t, dir, map[string]string{
		".golangci.yml":       "version: \"2\"\n",
		"rulebook/parts.yaml": "part: base\n",
		"rulebook/deep/a.md":  "a\n",
		"empty":               "",
	})
	write(t, filepath.Join(base, "beyond"), map[string]string{"secret": "outside\n"})
	plant(t, dir, map[string]string{
		"linked.yml": ".golangci.yml",
		"book":       "rulebook",
		"out":        "../beyond",
	})
	old, err := hostfs.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = old.Close() })
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatalf("Tree.Open: %v", err)
	}
	view, err := tr.FS(root)
	if err != nil {
		t.Fatalf("FS: %v", err)
	}
	want, got := walkAll(t, old), walkAll(t, view)
	for p, w := range want {
		if g, ok := got[p]; !ok || g != w {
			t.Errorf("%s: the view sees %+v (listed %v), the old Repo %+v", p, g, ok, w)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			t.Errorf("%s: the view lists it, the old Repo does not", p)
		}
	}
	for _, p := range []string{"linked.yml", "book", "out"} {
		if w := want[p]; !w.refused {
			t.Errorf("%s: the old Repo read it through the link as %+v, so the two agree on no refusal", p, w)
		}
	}
	for _, p := range []string{"book/parts.yaml", "out/secret"} {
		if data, err := fs.ReadFile(view, p); err == nil {
			t.Errorf("ReadFile(%s) in the view = %q through a link, want refused", p, data)
		}
	}
	if w := want["rulebook/parts.yaml"]; w.refused || w.content != "part: base\n" {
		t.Errorf("the control: rulebook/parts.yaml is %+v, want it read", w)
	}
}

// TestAViewOrAnOpenInOfARootNotOpenedIsAnError: a root this tree never
// opened has no view and nothing opens in it; an empty view would read as a
// repository with no file. The control: the same calls on a root it opened.
func TestAViewOrAnOpenInOfARootNotOpenedIsAnError(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "root")
	write(t, dir, map[string]string{"rules/a.yaml": "a: 1\n"})
	tr := newTree(t)
	never := model.Root(filepath.ToSlash(dir))
	if v, err := tr.FS(never); err == nil {
		t.Errorf("FS of a root never opened = %v, nil; want an error", v)
	}
	if got, err := tr.OpenIn(never, "rules"); err == nil {
		t.Errorf("OpenIn in a root never opened = %q, nil; want an error", got)
	}
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := tr.FS(root); err != nil {
		t.Errorf("the control: FS = %v", err)
	}
	if _, err := tr.OpenIn(root, "rules"); err != nil {
		t.Errorf("the control: OpenIn(rules) = %v", err)
	}
}

// TestAViewOfARootWhoseFolderIsGoneIsAnError: a root whose folder was
// removed after it was opened has no view, never an empty one. A system
// that refuses to remove a folder held open leaves it there, and the view
// then still reads it: either way the view never answers a folder that is
// not the one opened. RemoveAll empties a folder before it removes it, so the
// kept folder may already have lost its file: the view then says the file is
// not there, and never reads other bytes. Planted failure: FS without its
// check of the folder at the root's name, which answers an empty view of the
// removed folder.
func TestAViewOfARootWhoseFolderIsGoneIsAnError(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "root")
	write(t, dir, map[string]string{"a.yaml": "a: 1\n"})
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		view, err := tr.FS(root)
		if err != nil {
			t.Fatalf("FS of a folder the system kept = %v", err)
		}
		if data, err := fs.ReadFile(view, "a.yaml"); err != nil && !errors.Is(err, fs.ErrNotExist) || err == nil && string(data) != "a: 1\n" {
			t.Errorf("ReadFile(a.yaml) of a folder the system kept = %q, %v", data, err)
		}
		return
	}
	view, err := tr.FS(root)
	if err == nil {
		t.Errorf("FS of a root whose folder is gone = %v, nil; want an error", view)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FS of a root whose folder is gone = %v, want fs.ErrNotExist", err)
	}
}
