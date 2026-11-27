package contracttest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// WriterFiles are what the folder holds when the Writer suite starts: an old
// file, and a log whose last line is torn.
func WriterFiles() map[string]string {
	return map[string]string{
		"old.txt":    "old\n",
		"log.jsonl":  "{\"a\":1}\n{\"b\":",
		"rules/a.md": "a\n",
	}
}

// WriterMaker makes the Writer under test over a folder holding files: the
// Tree it reads back through, which opened nothing yet, the Writer that writes
// in the Tree's roots, and the folder to open.
type WriterMaker func(t *testing.T, files map[string]string) (tree port.Tree, w port.Writer, dir string)

// leaving are paths that name something outside a root: climbing, absolute,
// and drive-style, which one system reads as absolute (testdata/leaving.txt).
func leaving(t testing.TB) []string {
	t.Helper()
	return table(t, "leaving")
}

// Writer runs the Writer suite against the port build answers.
func Writer(t *testing.T, build WriterMaker) {
	t.Helper()
	open := func(t *testing.T) (port.Tree, port.Writer, model.Root) {
		t.Helper()
		tr, w, dir := build(t, WriterFiles())
		root, err := tr.Open(dir)
		if err != nil {
			t.Fatalf("Open(%q) = %v", dir, err)
		}
		return tr, w, root
	}
	writerReplace(t, open)
	writerAppend(t, open)
	writerRemove(t, open)
	writerRemoveRefuses(t, open)
	writerRefuses(t, open)
}

// holds fails the test unless the file at path in root holds want.
func holds(t *testing.T, tr port.Tree, root model.Root, path, want string) {
	t.Helper()
	if got, err := tr.Read(root, path); err != nil || string(got) != want {
		t.Errorf("Read(%q) = %q, %v, want %q", path, got, err, want)
	}
}

// absent fails the test unless no file is at path in root.
func absent(t *testing.T, tr port.Tree, root model.Root, path string) {
	t.Helper()
	if _, err := tr.Read(root, path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read(%q) after Remove = %v, want fs.ErrNotExist", path, err)
	}
}

type writerOpener func(t *testing.T) (port.Tree, port.Writer, model.Root)

func writerReplace(t *testing.T, open writerOpener) {
	t.Helper()
	t.Run("a replace makes the file whole, with its folder", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Replace(root, "new/deep/f.txt", []byte("new\n")); err != nil {
			t.Fatal(err)
		}
		holds(t, tr, root, "new/deep/f.txt", "new\n")
	})
	t.Run("a replace puts the new content in place of the old", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Replace(root, "old.txt", []byte("newer")); err != nil {
			t.Fatal(err)
		}
		holds(t, tr, root, "old.txt", "newer")
		holds(t, tr, root, "rules/a.md", "a\n")
	})
	t.Run("a replace with nil is an empty file", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Replace(root, "old.txt", nil); err != nil {
			t.Fatal(err)
		}
		holds(t, tr, root, "old.txt", "")
	})
}

func writerAppend(t *testing.T, open writerOpener) {
	t.Helper()
	t.Run("an append makes a missing file with one line", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Append(root, "state/history.jsonl", []byte(`{"n":1}`)); err != nil {
			t.Fatal(err)
		}
		if err := w.Append(root, "state/history.jsonl", []byte(`{"n":2}`)); err != nil {
			t.Fatal(err)
		}
		holds(t, tr, root, "state/history.jsonl", "{\"n\":1}\n{\"n\":2}\n")
	})
	t.Run("an append ends a torn last line first, then writes one line", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Append(root, "log.jsonl", []byte(`{"c":3}`)); err != nil {
			t.Fatal(err)
		}
		holds(t, tr, root, "log.jsonl", "{\"a\":1}\n{\"b\":\n{\"c\":3}\n")
	})
	t.Run("an append of nil is an empty line", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Append(root, "old.txt", nil); err != nil {
			t.Fatal(err)
		}
		holds(t, tr, root, "old.txt", "old\n\n")
	})
	t.Run("several lines joined by a newline are one append", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Append(root, "old.txt", []byte("one\ntwo\nthree")); err != nil {
			t.Fatal(err)
		}
		holds(t, tr, root, "old.txt", "old\none\ntwo\nthree\n")
	})
	t.Run("a newline at either end or an empty line between two writes nothing", func(t *testing.T) {
		for _, bad := range []string{"one\n", "\none", "one\n\ntwo", "\n"} {
			tr, w, root := open(t)
			if err := w.Append(root, "old.txt", []byte(bad)); err == nil {
				t.Errorf("%q was appended", bad)
			}
			holds(t, tr, root, "old.txt", "old\n")
		}
	})
}

func writerRemove(t *testing.T, open writerOpener) {
	t.Helper()
	t.Run("a remove takes the file away and leaves the rest", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Remove(root, "rules/a.md"); err != nil {
			t.Fatal(err)
		}
		absent(t, tr, root, "rules/a.md")
		holds(t, tr, root, "old.txt", "old\n")
	})
	t.Run("a remove of a file that is not there is no error and makes nothing", func(t *testing.T) {
		tr, w, root := open(t)
		for _, p := range []string{"gone.txt", "no/such/folder/f.txt"} {
			if err := w.Remove(root, p); err != nil {
				t.Errorf("Remove(%q) = %v, want nil", p, err)
			}
			absent(t, tr, root, p)
		}
	})
	t.Run("a file a replace made is gone after its remove", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Replace(root, "new.txt", []byte("new\n")); err != nil {
			t.Fatal(err)
		}
		if err := w.Remove(root, "new.txt"); err != nil {
			t.Fatal(err)
		}
		absent(t, tr, root, "new.txt")
	})
}

func writerRemoveRefuses(t *testing.T, open writerOpener) {
	t.Helper()
	t.Run("a folder is refused and stays with what it holds", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Remove(root, "rules"); err == nil {
			t.Error("Remove of a folder succeeded")
		}
		holds(t, tr, root, "rules/a.md", "a\n")
	})
	t.Run("a path that leaves the root, or the root itself, is refused and removes nothing", func(t *testing.T) {
		tr, w, root := open(t)
		for _, p := range leaving(t) {
			if err := w.Remove(root, p); !errors.Is(err, port.ErrOutside) {
				t.Errorf("Remove(%q) = %v, want ErrOutside", p, err)
			}
		}
		if err := w.Remove(root, ""); err == nil {
			t.Error("Remove of the root itself succeeded")
		}
		if got, err := tr.List(root, "", model.Every); err != nil || len(got) != 4 {
			t.Errorf("after the refusals the root holds %+v, %v, want only its four entries", got, err)
		}
	})
}

func writerRefuses(t *testing.T, open writerOpener) {
	t.Helper()
	t.Run("a path that leaves the root is ErrOutside and writes nothing", func(t *testing.T) {
		tr, w, root := open(t)
		paths := leaving(t)
		if len(paths) == 0 {
			t.Fatal("no path to refuse")
		}
		for _, p := range paths {
			if err := w.Replace(root, p, []byte("x")); !errors.Is(err, port.ErrOutside) {
				t.Errorf("Replace(%q) = %v, want ErrOutside", p, err)
			}
			if err := w.Append(root, p, []byte("x")); !errors.Is(err, port.ErrOutside) {
				t.Errorf("Append(%q) = %v, want ErrOutside", p, err)
			}
		}
		got, err := tr.List(root, "", model.Every)
		if err != nil || len(got) != 4 {
			t.Errorf("after the refusals the root holds %+v, %v, want only its four entries", got, err)
		}
	})
	t.Run("the root itself is no file to write", func(t *testing.T) {
		_, w, root := open(t)
		if err := w.Replace(root, "", []byte("x")); err == nil {
			t.Error("Replace of the root itself succeeded")
		}
		if err := w.Append(root, "", []byte("x")); err == nil {
			t.Error("Append to the root itself succeeded")
		}
	})
}
