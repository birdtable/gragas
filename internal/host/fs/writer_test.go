package fs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"
	hostfs "github.com/birdtable/gragas/internal/host/fs"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// opened is a tree with dir opened as its root, and a writer over it.
func opened(t *testing.T, dir string) (*hostfs.Tree, hostfs.Writer, model.Root) {
	t.Helper()
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return tr, hostfs.Writer{Tree: tr}, root
}

// names is what the folder dir holds, sorted.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestAReplaceLeavesNothingAside(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"f": "old"})
	_, w, root := opened(t, dir)
	for range 3 {
		if err := w.Replace(root, "f", []byte("new")); err != nil {
			t.Fatal(err)
		}
	}
	if got := names(t, dir); !slices.Equal(got, []string{"f"}) {
		t.Errorf("the folder holds %v, want only f", got)
	}
}

// TestAReplaceWritesOnlyOverAFile: a folder, and a link inside the root,
// at the path are refused and left as they were; the control, a file,
// is replaced.
func TestAReplaceWritesOnlyOverAFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"folder/a": "a", "target": "t", "file": "f"})
	testenv.Link(t, "target", filepath.Join(dir, "link"))
	tr, w, root := opened(t, dir)
	for _, p := range []string{"folder", "link"} {
		if err := w.Replace(root, p, []byte("x")); err == nil {
			t.Errorf("Replace(%q) wrote over something that is not a file", p)
		}
	}
	if info, err := os.Lstat(filepath.Join(dir, "link")); err != nil || info.Mode().Type() != fs.ModeSymlink {
		t.Errorf("the link is now %v, %v", info, err)
	}
	holdsFile(t, tr, root, "target", "t")
	holdsFile(t, tr, root, "folder/a", "a")
	if err := w.Replace(root, "file", []byte("x")); err != nil {
		t.Errorf("the control: %v", err)
	}
	holdsFile(t, tr, root, "file", "x")
}

func holdsFile(t *testing.T, tr *hostfs.Tree, root model.Root, p, want string) {
	t.Helper()
	if got, err := tr.Read(root, p); err != nil || string(got) != want {
		t.Errorf("Read(%q) = %q, %v, want %q", p, got, err, want)
	}
}

// TestAWriteThroughALinkOutOfTheRootIsRefused: a folder link and a file link
// that lead out of the root are refused, matching ErrOutside where the write
// would have followed them, and nothing outside changes; the control, a file
// inside, is written.
func TestAWriteThroughALinkOutOfTheRootIsRefused(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	write(t, base, map[string]string{"outside/secret": "s", "root/in": "in"})
	dir := filepath.Join(base, "root")
	testenv.Link(t, filepath.Join(base, "outside"), filepath.Join(dir, "out"))
	testenv.Link(t, "../outside/secret", filepath.Join(dir, "secret"))
	testenv.Link(t, "../outside/gone", filepath.Join(dir, "gone"))
	tr, w, root := opened(t, dir)
	for _, p := range []string{"out/new", "out/secret", "out/deep/new"} {
		if err := w.Replace(root, p, []byte("x")); !errors.Is(err, port.ErrOutside) {
			t.Errorf("Replace(%q) = %v, want ErrOutside", p, err)
		}
	}
	for _, p := range []string{"out/new", "out/secret", "secret", "gone"} {
		if err := w.Append(root, p, []byte("x")); !errors.Is(err, port.ErrOutside) {
			t.Errorf("Append(%q) = %v, want ErrOutside", p, err)
		}
	}
	if err := w.Replace(root, "secret", []byte("x")); err == nil {
		t.Error("Replace wrote over a link that leads out")
	}
	if got := names(t, filepath.Join(base, "outside")); !slices.Equal(got, []string{"secret"}) {
		t.Errorf("outside the root there is now %v", got)
	}
	if got, err := os.ReadFile(filepath.Join(base, "outside", "secret")); err != nil || string(got) != "s" {
		t.Errorf("the file outside holds %q, %v", got, err)
	}
	if err := w.Append(root, "in", []byte("more")); err != nil {
		t.Errorf("the control: %v", err)
	}
	holdsFile(t, tr, root, "in", "in\nmore\n")
}

// TestARemoveTakesAwayOnlyAFileAndNeverWhatALinkNames: a folder, an empty
// one too, a link to a file inside and a link to a file outside are refused
// and stay, as do the files they name; a path through a folder link that leads out is
// ErrOutside, and nothing outside goes. The control, a file, is removed and
// its folder flushed.
func TestARemoveTakesAwayOnlyAFileAndNeverWhatALinkNames(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	write(t, base, map[string]string{"outside/secret": "s", "root/folder/a": "a", "root/target": "t", "root/file": "f"})
	dir := filepath.Join(base, "root")
	for link, to := range map[string]string{"link": "target", "secret": "../outside/secret", "out": filepath.Join(base, "outside")} {
		testenv.Link(t, to, filepath.Join(dir, link))
	}
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o750); err != nil {
		t.Fatal(err)
	}
	tr, w, root := opened(t, dir)
	for _, p := range []string{"folder", "empty", "link", "secret"} {
		if err := w.Remove(root, p); err == nil {
			t.Errorf("Remove(%q) took away something that is not a file", p)
		}
	}
	if info, err := os.Lstat(filepath.Join(dir, "empty")); err != nil || !info.IsDir() {
		t.Errorf("the empty folder is now %v, %v", info, err)
	}
	if err := w.Remove(root, "out/secret"); !errors.Is(err, port.ErrOutside) {
		t.Errorf("Remove(out/secret) = %v, want ErrOutside", err)
	}
	for _, link := range []string{"link", "secret", "out"} {
		if info, err := os.Lstat(filepath.Join(dir, link)); err != nil || info.Mode().Type() != fs.ModeSymlink {
			t.Errorf("the link %s is now %v, %v", link, info, err)
		}
	}
	holdsFile(t, tr, root, "target", "t")
	holdsFile(t, tr, root, "folder/a", "a")
	if got, err := os.ReadFile(filepath.Join(base, "outside", "secret")); err != nil || string(got) != "s" {
		t.Errorf("the file outside holds %q, %v", got, err)
	}
	if err := w.Remove(root, "file"); err != nil {
		t.Errorf("the control: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "file")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the control is still there: %v", err)
	}
}

// appendFlags counts, per file, the times the append flag is named in the Go
// files below dir, skipping the folders git and the build keep.
func appendFlags(t *testing.T, dir string) map[string]int {
	t.Helper()
	flag := "O_" + "APPEND"
	found := map[string]int{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && slices.Contains([]string{".git", ".tools", "dist", "node_modules"}, d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if n := strings.Count(string(data), flag); n > 0 {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			found[filepath.ToSlash(rel)] = n
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// TestTheAppendFlagIsNamedInOnePlace (store.state-files.5): of every Go file
// in the module, only Writer.Append opens a file to append. The count is
// checked to find a planted second use.
func TestTheAppendFlagIsNamedInOnePlace(t *testing.T) {
	t.Parallel()
	module, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(module, "go.mod")); err != nil {
		t.Fatalf("the module root is not at %s: %v", module, err)
	}
	got := appendFlags(t, module)
	if len(got) != 1 || got["internal/host/fs/writer.go"] != 1 {
		t.Errorf("the append flag is named in %v, want once, in internal/host/fs/writer.go", got)
	}
	t.Run("a second use is found", func(t *testing.T) {
		plant := t.TempDir()
		write(t, plant, map[string]string{"a.go": "os." + "O_" + "APPEND", "b/c.go": "syscall." + "O_" + "APPEND", "d.txt": "O_" + "APPEND", "e.go": "x"})
		if got := appendFlags(t, plant); len(got) != 2 || got["a.go"] != 1 || got["b/c.go"] != 1 {
			t.Errorf("in the planted module the flag is named in %v, want a.go and b/c.go", got)
		}
	})
}
