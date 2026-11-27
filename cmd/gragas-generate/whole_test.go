package main

import (
	"os"
	"path/filepath"
	"testing"

	testenv "github.com/birdtable/go-testkit"
)

// putWhole is replaceIn as the tests below call it.
func putWhole(root *os.Root, path string, data []byte) error {
	return replaceIn(root, path, data)
}

// TestReplaceInHoldsTheWriteToTheRoot (gragas#726, gragas#837): replaceIn writes through
// go-files' write.FileIn, which walks from the root again at every step,
// so the root's own refusals hold through the write, with no check of its own.
//   - A folder on the way that is a link out of the root, written as an
//     absolute path or climbing with "..": refused, and the folder outside
//     stays empty. Planted failure: FileIn replaced by File at the root's
//     name joined with the path.
//   - A path that names the root itself (".", "a/..") or leaves it ("",
//     "../x", an absolute path): refused, and nothing is written beside
//     the root. Planted failure: the same File, which writes beside the
//     root.
//   - The control: a file inside the root is written whole, and written
//     again whole. Its mode is not this test's subject:
//     go-files' write tests hold Mode and KeepMode.
func TestReplaceInHoldsTheWriteToTheRoot(t *testing.T) {
	t.Parallel()
	for _, climb := range []bool{false, true} {
		above := t.TempDir()
		dir, outside := filepath.Join(above, "repo"), filepath.Join(above, "outside")
		for _, d := range []string{dir, outside} {
			if err := os.Mkdir(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		target := outside
		if climb {
			target = filepath.Join("..", "outside")
		}
		testenv.Link(t, target, filepath.Join(dir, "out"))
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = root.Close() })
		if err := putWhole(root, filepath.Join("out", "x"), []byte("x")); err == nil {
			t.Errorf("climb %v: a write through a link out of the root was not refused", climb)
		}
		if got, err := os.ReadDir(outside); err != nil || len(got) != 0 {
			t.Errorf("climb %v: the folder outside holds %v (%v)", climb, got, err)
		}
	}

	above := t.TempDir()
	dir := filepath.Join(above, "repo")
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	for _, p := range []string{".", filepath.Join("a", ".."), "", filepath.Join("..", "x"), filepath.Join(above, "x")} {
		if err := putWhole(root, p, []byte("x")); err == nil {
			t.Errorf("%q: a path that is no file inside the root was not refused", p)
		}
	}
	if got, err := os.ReadDir(above); err != nil || len(got) != 1 {
		t.Errorf("the folder above the root holds %v (%v), want repo alone", got, err)
	}

	if err := putWhole(root, filepath.Join("a", "f"), []byte("whole")); err != nil {
		t.Fatalf("the control: %v", err)
	}
	f := filepath.Join(dir, "a", "f")
	if got, err := os.ReadFile(f); err != nil || string(got) != "whole" {
		t.Errorf("the control holds %q (%v)", got, err)
	}
	if err := putWhole(root, filepath.Join("a", "f"), []byte("again")); err != nil {
		t.Fatalf("the control, written again: %v", err)
	}
	if got, err := os.ReadFile(f); err != nil || string(got) != "again" {
		t.Errorf("the control written again holds %q (%v)", got, err)
	}
	if got, err := os.ReadDir(filepath.Join(dir, "a")); err != nil || len(got) != 1 {
		t.Errorf("the control's folder holds %v (%v), want the file alone", got, err)
	}
}
