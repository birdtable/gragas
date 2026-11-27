package fs_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/port"
)

// TestLocateGivesTheAnswersOfItsOsStatWalk: Locate's walk is
// go-files' confine.Nearest, which asks each folder with Lstat, and Locate
// gives the answers its os.Stat walk gave, so no caller sees the move. A
// marker that is a link to nothing does not count and the walk goes on above
// it, while one that is a link to a file counts where it is; and a folder
// where the marker's inner name is a file ends the walk on that error, whether
// a folder above holds the marker or none does, as does a start that is a
// file. Planted failures: the os.Stat
// check of the found folder taken out answers the folder of the dangling link;
// innerFile taken out walks past the file to the marker above, or to "not
// found"; and a start that is a file ends the walk on that error for a marker
// of one name too, where innerFile skipping such a marker walks up past it.
func TestLocateGivesTheAnswersOfItsOsStatWalk(t *testing.T) {
	t.Parallel()
	t.Run("a marker that is a dangling link does not count, a link to a file does", func(t *testing.T) {
		t.Parallel()
		testenv.Links(t)
		dir := filepath.Join(t.TempDir(), "top")
		write(t, dir, map[string]string{"near/deep/keep": "", "linked/deep/keep": "", ".mark": "above\n"})
		plant(t, dir, map[string]string{"near/.mark": "gone", "linked/.mark": "../.mark"})
		tr := newTree(t)
		for start, at := range map[string]string{"near": dir, "linked": filepath.Join(dir, "linked")} {
			want, err := tr.Open(at)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := tr.Locate(filepath.Join(dir, start, "deep"), ".mark"); err != nil || got != want {
				t.Errorf("Locate from %s = %q, %v; want %q", start, got, err, want)
			}
		}
	})
	t.Run("a marker whose inner name is a file ends the walk on that error", func(t *testing.T) {
		t.Parallel()
		for name, files := range map[string]map[string]string{
			"a marker above": {"near/deep/keep": "", "near/.gragas": "a file\n", ".gragas/pin.yaml": "pin\n"},
			"no marker":      {"near/deep/keep": "", "near/.gragas": "a file\n"},
		} {
			dir := filepath.Join(t.TempDir(), "top")
			write(t, dir, files)
			got, err := newTree(t).Locate(filepath.Join(dir, "near", "deep"), ".gragas/pin.yaml")
			if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, port.ErrOutside) {
				t.Errorf("%s: Locate = %q, %v; want the error of the file where a folder is asked for", name, got, err)
			}
		}
	})
	t.Run("a start that is a file ends the walk on that error, for a marker of one name", func(t *testing.T) {
		t.Parallel()
		for name, files := range map[string]map[string]string{
			"a marker above": {"near/file": "a file\n", ".mark": "above\n"},
			"no marker":      {"near/file": "a file\n"},
		} {
			dir := filepath.Join(t.TempDir(), "top")
			write(t, dir, files)
			got, err := newTree(t).Locate(filepath.Join(dir, "near", "file"), ".mark")
			if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, port.ErrOutside) {
				t.Errorf("%s: Locate from a file = %q, %v; want the error of the file where a folder is asked for", name, got, err)
			}
		}
	})
	t.Run("no marker and a marker outside the folder are refused as before", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "top")
		write(t, dir, map[string]string{"near/deep/keep": ""})
		tr := newTree(t)
		if _, err := tr.Locate(filepath.Join(dir, "near", "deep"), "no-such-marker-2281"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Locate of no marker = %v, want fs.ErrNotExist", err)
		}
		for _, marker := range []string{"", "../up", "/abs"} {
			if _, err := tr.Locate(filepath.Join(dir, "near"), marker); !errors.Is(err, port.ErrOutside) {
				t.Errorf("Locate(%q) = %v, want port.ErrOutside", marker, err)
			}
		}
	})
}
