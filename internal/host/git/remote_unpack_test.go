package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/confine"
)

// unpacking is an adapter and an open cache in a folder of the test's own,
// with the folder a fetch of the release r of commit c unpacks to.
func unpacking(t *testing.T) (g *Git, cache *os.Root, dir, folder string) {
	t.Helper()
	g, err := New(Config{Run: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	cache, err = os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return g, cache, dir, filepath.Join(dir, "r", "c")
}

// noFetchLeft fails t when the folder of r holds a fetch's own folder.
func noFetchLeft(t *testing.T, dir string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dir, "r", ".fetch-*"))
	if err != nil || len(left) != 0 {
		t.Errorf("the fetch left %v behind (%v)", left, err)
	}
}

// A system whose rename cannot refuse a name that is there is passed on as
// it answers, once: never tried again as a rename that may replace.
func TestUnpackPassesOnARenameItCannotMake(t *testing.T) {
	g, cache, dir, folder := unpacking(t)
	tries := 0
	g.renaming = func(*os.Root, string, string) error {
		tries++
		return fmt.Errorf("this system cannot move without replacing: %w", errors.ErrUnsupported)
	}
	err := g.unpack(context.Background(), cache, "", "r", folder, nil)
	if _, ok := errors.AsType[*Failed](err); !ok || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("unpack = %v, not a *Failed of errors.ErrUnsupported", err)
	}
	if tries != 1 {
		t.Errorf("the rename was tried %d times, not once", tries)
	}
	if _, err := os.Lstat(folder); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%s) = %v, and nothing was to be moved there", folder, err)
	}
	noFetchLeft(t, dir)
}

// A folder the race made first is kept as it is, and the fetch is done.
func TestUnpackKeepsTheFolderTheRaceMadeFirst(t *testing.T) {
	g, cache, dir, folder := unpacking(t)
	if err := private(cache, filepath.Join("r", "c")); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(folder, "first")
	f, err := cache.Create(filepath.Join("r", "c", "first"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("the race's")
	if err := errors.Join(err, f.Close()); err != nil {
		t.Fatal(err)
	}
	if err := g.unpack(context.Background(), cache, "", "r", folder, nil); err != nil {
		t.Fatalf("unpack = %v, and the folder the race made is the fetch done", err)
	}
	if got, err := os.ReadFile(first); err != nil || string(got) != "the race's" {
		t.Errorf("the race's folder holds %q (%v), not what it held", got, err)
	}
	noFetchLeft(t, dir)
}

// A folder that is gone by the move is refused as it was before
// confine.RenameNoReplace: its fs.ErrNotExist is no race won.
func TestUnpackRefusesAFolderGoneByTheMove(t *testing.T) {
	g, cache, dir, folder := unpacking(t)
	g.renaming = func(root *os.Root, from, to string) error {
		if err := os.RemoveAll(filepath.Join(dir, "r")); err != nil {
			t.Fatal(err)
		}
		return confine.RenameNoReplace(root, from, to)
	}
	err := g.unpack(context.Background(), cache, "", "r", folder, nil)
	if _, ok := errors.AsType[*Failed](err); !ok {
		t.Fatalf("unpack = %v, not a *Failed", err)
	}
	if _, err := os.Lstat(folder); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%s) = %v, and nothing was to be moved there", folder, err)
	}
}
