package contracttest

import (
	"errors"
	"io/fs"
	"math"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// NameFiles are what the folder holds when the Names suite starts: files
// that keep one name, and files the suite gives a second.
func NameFiles() map[string]string {
	return map[string]string{
		"rules/a.yaml": "a\n",
		"rules/b.yaml": "b\n",
		"deep/c.txt":   "c\n",
	}
}

// NamePlants are the second names the Names suite gives, each to the file
// it names, slash-separated and relative to the root, as a hard link
// gives one: a second name beside the first, and one in another folder.
func NamePlants() map[string]string {
	return map[string]string{
		"rules/twin.yaml": "rules/a.yaml",
		"elsewhere.yaml":  "deep/c.txt",
	}
}

// ample is a bound no file of the suite reaches.
const ample = 1 << 20

// NamesMaker makes the Tree under test over a folder holding files, as
// NameFiles gives them, with each second name of names made for the file it
// names: the Tree, which opened nothing yet, and the folder.
type NamesMaker func(t *testing.T, files, names map[string]string) (tree port.Tree, dir string)

// Names runs the suite of a file's names: ReadSoleWithin refuses a
// file with more than one name by every name it has, matching
// port.ErrManyNames and answering no bytes, while Read reads it by either;
// a file with one name reads by ReadSoleWithin as by Read, and ReadSoleWithin answers a
// missing file, a folder and a path that leaves the root as Read does.
func Names(t *testing.T, build NamesMaker) {
	t.Helper()
	for _, c := range []struct {
		name  string
		check func(*testing.T, port.Tree, model.Root)
	}{
		{"a file with a second name is ErrManyNames by each of its names, with no bytes", manyNamesRefused},
		{"Read reads a file with a second name by either name", manyNamesRead},
		{"the control: a file with one name reads by ReadSoleWithin as by Read", soleNameRead},
		{"a missing file, a folder and a path that leaves the root are answered as Read answers them", soleNameAsRead},
		{"ReadSoleWithin reads a file of max bytes and refuses one of max+1 with ErrOverBound and no bytes", withinBound},
	} {
		t.Run(c.name, func(t *testing.T) {
			tr, dir := build(t, NameFiles(), NamePlants())
			root, err := tr.Open(dir)
			if err != nil {
				t.Fatalf("Open(%q) = %v", dir, err)
			}
			c.check(t, tr, root)
		})
	}
}

// manyNamesRefused: ReadSoleWithin refuses a file with a second name by each name.
func manyNamesRefused(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	for _, p := range []string{"rules/a.yaml", "rules/twin.yaml", "deep/c.txt", "elsewhere.yaml"} {
		if data, err := tr.ReadSoleWithin(root, p, ample); !errors.Is(err, port.ErrManyNames) || data != nil {
			t.Errorf("ReadSoleWithin(%q) = %q, %v, want no bytes and ErrManyNames", p, data, err)
		}
	}
}

// manyNamesRead: Read reads a file with a second name by either name.
func manyNamesRead(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	for p, want := range map[string]string{"rules/a.yaml": "a\n", "rules/twin.yaml": "a\n", "elsewhere.yaml": "c\n"} {
		if data, err := tr.Read(root, p); err != nil || string(data) != want {
			t.Errorf("Read(%q) = %q, %v, want %q", p, data, err, want)
		}
	}
}

// soleNameRead: a file with one name reads by ReadSoleWithin as by Read.
func soleNameRead(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	if data, err := tr.ReadSoleWithin(root, "rules/b.yaml", ample); err != nil || string(data) != "b\n" {
		t.Errorf("ReadSoleWithin(rules/b.yaml) = %q, %v, want %q", data, err, "b\n")
	}
}

// withinBound: ReadSoleWithin of rules/b.yaml (2 bytes) reads it at 2 and at more,
// and at 1 answers no bytes and pluginbundle.ErrOverBound.
func withinBound(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	for _, max := range []int64{2, 3} {
		if data, err := tr.ReadSoleWithin(root, "rules/b.yaml", max); err != nil || string(data) != "b\n" {
			t.Errorf("ReadSoleWithin(rules/b.yaml, %d) = %q, %v, want %q", max, data, err, "b\n")
		}
	}
	if data, err := tr.ReadSoleWithin(root, "rules/b.yaml", 1); !errors.Is(err, pluginbundle.ErrOverBound) || data != nil {
		t.Errorf("ReadSoleWithin(rules/b.yaml, 1) = %q, %v, want no bytes and ErrOverBound", data, err)
	}
	if data, err := tr.ReadSoleWithin(root, "rules/b.yaml", 0); !errors.Is(err, pluginbundle.ErrOverBound) || data != nil {
		t.Errorf("ReadSoleWithin(rules/b.yaml, 0) = %q, %v, want no bytes and ErrOverBound", data, err)
	}
	// A bound outside [0, MaxInt64) is no bound: it is refused, with no bytes,
	// never read as unbounded or as an empty read.
	for _, max := range []int64{-1, math.MinInt64, math.MaxInt64} {
		if data, err := tr.ReadSoleWithin(root, "rules/b.yaml", max); err == nil || data != nil {
			t.Errorf("ReadSoleWithin(rules/b.yaml, %d) = %q, %v, want no bytes and an error", max, data, err)
		}
	}
}

// soleNameAsRead: ReadSoleWithin answers a missing file, a folder and a path that
// leaves the root as Read does.
func soleNameAsRead(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	if _, err := tr.ReadSoleWithin(root, "rules/gone.yaml", ample); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadSoleWithin(rules/gone.yaml) = %v, want fs.ErrNotExist", err)
	}
	if data, err := tr.ReadSoleWithin(root, "rules", ample); err == nil {
		t.Errorf("ReadSoleWithin(rules) = %q, want an error, since it is a folder", data)
	}
	if _, err := tr.ReadSoleWithin(root, "../away.yaml", ample); !errors.Is(err, port.ErrOutside) {
		t.Errorf("ReadSoleWithin(../away.yaml) = %v, want ErrOutside", err)
	}
}
