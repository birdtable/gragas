package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/birdtable/go-files/trust/trusttest"
)

// placed is a folder with a program src and, when have is not nil, a dst of
// those bytes whose modification time is long past, so a write shows.
func placed(t *testing.T, src string, have *string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "src"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if have != nil {
		dst := filepath.Join(dir, "dst")
		if err := os.WriteFile(dst, []byte(*have), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		if err := os.Chtimes(dst, old, old); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runPlaceIn(t *testing.T, dir string) (code int, out string) {
	t.Helper()
	var o, e bytes.Buffer
	code = runPlace([]string{filepath.Join(dir, "src"), filepath.Join(dir, "dst")}, &o, &e)
	return code, o.String() + e.String()
}

// TestPlaceLeavesAnUnchangedProgramAndItsModificationTimeAlone (gragas#1381):
// dst holding src's bytes is not written again: its modification time stays,
// the source is removed, and the run says so. This is the plant of the build
// member: a build with no source change leaves bin/gragas's mtime alone.
func TestPlaceLeavesAnUnchangedProgramAndItsModificationTimeAlone(t *testing.T) {
	t.Parallel()
	same := "the same program"
	dir := placed(t, same, &same)
	before, err := os.Stat(filepath.Join(dir, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	code, out := runPlaceIn(t, dir)
	if code != exitDone {
		t.Fatalf("place of the same bytes ended %d\n%s", code, out)
	}
	after, err := os.Stat(filepath.Join(dir, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("dst was written again: its mtime went from %v to %v", before.ModTime(), after.ModTime())
	}
	if _, err := os.Stat(filepath.Join(dir, "src")); err == nil {
		t.Error("src is still there")
	}
}

// TestPlaceReplacesADifferentProgramAndMakesAMissingOneExecutable: a dst with
// other bytes is replaced and a missing one is made, each executable and each
// holding src's bytes; the source is removed.
func TestPlaceReplacesADifferentProgramAndMakesAMissingOneExecutable(t *testing.T) {
	t.Parallel()
	old := "the old program"
	for name, have := range map[string]*string{"different": &old, "missing": nil} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := placed(t, "the new program", have)
			code, out := runPlaceIn(t, dir)
			if code != exitDone {
				t.Fatalf("place ended %d\n%s", code, out)
			}
			got, err := os.ReadFile(filepath.Join(dir, "dst"))
			if err != nil || string(got) != "the new program" {
				t.Errorf("dst holds %q (%v), want the new program", got, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "src")); err == nil {
				t.Error("src is still there")
			}
			// Executable, whatever mode the replaced file had: a 0644 write is red.
			// go-files' trusttest holds the mode this system keeps for 0755: itself where there are
			// mode bits, and on Windows the read-only attribute alone.
			trusttest.Perm(t, filepath.Join(dir, "dst"), 0o755)
		})
	}
}

// TestPlaceWithoutTwoArgumentsOrWithoutASourceIsRefused: the usage for the
// wrong number of arguments, and a failure naming the missing source, with
// nothing written.
func TestPlaceWithoutTwoArgumentsOrWithoutASourceIsRefused(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	if code := runPlace([]string{"only-one"}, &o, &e); code != exitUsage {
		t.Errorf("one argument ended %d, want usage", code)
	}
	dir := t.TempDir()
	if code := runPlace([]string{filepath.Join(dir, "no-src"), filepath.Join(dir, "dst")}, &o, &e); code != exitFailed {
		t.Errorf("a missing source ended %d, want failed", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "dst")); err == nil {
		t.Error("a refused place wrote dst")
	}
}

// TestPlaceMakesTheFolderOfAMissingDestination: a fresh clone has no bin/, and
// the first build makes it.
func TestPlaceMakesTheFolderOfAMissingDestination(t *testing.T) {
	t.Parallel()
	dir := placed(t, "a program", nil)
	dst := filepath.Join(dir, "bin", "gragas")
	var o, e bytes.Buffer
	if code := runPlace([]string{filepath.Join(dir, "src"), dst}, &o, &e); code != exitDone {
		t.Fatalf("place into a missing folder ended %d\n%s%s", code, o.String(), e.String())
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "a program" {
		t.Errorf("dst holds %q (%v)", got, err)
	}
}
