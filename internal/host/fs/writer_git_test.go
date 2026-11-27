package fs_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNoWriterWritesOutsideTheRoot: a path that climbs out, deep or at once, and
// an absolute one are refused by Replace, Append and Remove, and nothing is made.
func TestNoWriterWritesOutsideTheRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, w, root := opened(t, dir)
	for _, p := range []string{"../outside.txt", "sub/../../x", "/outside.txt"} {
		if err := w.Replace(root, p, []byte("x\n")); err == nil {
			t.Errorf("Replace of %s was not refused", p)
		}
		if err := w.Append(root, p, []byte("x")); err == nil {
			t.Errorf("Append to %s was not refused", p)
		}
		if err := w.Remove(root, p); err == nil {
			t.Errorf("Remove of %s was not refused", p)
		}
	}
	if got := names(t, dir); len(got) != 0 {
		t.Errorf("the root holds %v, want nothing", got)
	}
}

// TestAPathInTheSystemsFormIsWritten: a path made with the system's own
// separator and turned to the slash form every Writer path is in, sub/f, is
// written, appended to and removed by the Writer. The separator is never
// part of a name: the path with the backslash itself is refused on every
// system (TestABackslashInANameIsRefusedOnEverySystem).
func TestAPathInTheSystemsFormIsWritten(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, w, root := opened(t, dir)
	p := filepath.ToSlash(filepath.Join("sub", "f"))
	if err := w.Replace(root, p, []byte("x\n")); err != nil {
		t.Fatalf("Replace of %s = %v", p, err)
	}
	if err := w.Append(root, p, []byte("y")); err != nil {
		t.Errorf("Append to %s = %v", p, err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, p)); err != nil || string(b) != "x\ny\n" {
		t.Errorf("%s is %q, %v; want %q", p, b, err, "x\ny\n")
	}
	if err := w.Remove(root, p); err != nil {
		t.Errorf("Remove of %s = %v", p, err)
	}
}

// TestABackslashInANameIsRefusedOnEverySystem: a
// name with a \ is refused by Replace, Append and Remove, and nothing is made:
// confine.Local, through which the Writer takes every path, refuses a
// backslash on each system, so the split in the slash form never sees one.
// Planted failure: the Writer not going through the tree's local.
func TestABackslashInANameIsRefusedOnEverySystem(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, w, root := opened(t, dir)
	// built of parts: a literal of letters round a backslash reads as a path of
	// another system to the os check.
	const p = "a" + `\` + "b"
	if err := w.Replace(root, p, []byte("x\n")); err == nil {
		t.Errorf("Replace of %s was not refused", p)
	}
	if err := w.Append(root, p, []byte("x")); err == nil {
		t.Errorf("Append to %s was not refused", p)
	}
	if err := w.Remove(root, p); err == nil {
		t.Errorf("Remove of %s was not refused", p)
	}
	if got := names(t, dir); len(got) != 0 {
		t.Errorf("the root holds %v, want nothing", got)
	}
}

// TestNoWriterWritesUnderTheGitFolder: Replace, Append and Remove of a path
// with a .git component, in any case and at any depth, are refused, and
// nothing in the root is made or changed; the control is a file beside it.
// Planted failure: the
// refusal gone from Writer.at, which writes .git/config and the hook.
func TestNoWriterWritesUnderTheGitFolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{".git/config": "[core]\n", "a.txt": "a\n"})
	_, w, root := opened(t, dir)
	for _, p := range []string{".git/config", ".git/hooks/pre-commit", ".GIT/hooks/pre-commit", "sub/.git/config", "sub/.Git"} {
		if err := w.Replace(root, p, []byte("x\n")); err == nil {
			t.Errorf("Replace of %s was not refused", p)
		}
		if err := w.Append(root, p, []byte("x")); err == nil {
			t.Errorf("Append to %s was not refused", p)
		}
		if err := w.Remove(root, p); err == nil {
			t.Errorf("Remove of %s was not refused", p)
		}
	}
	for p, want := range map[string]string{".git/config": "[core]\n", "a.txt": "a\n"} {
		if b, err := os.ReadFile(filepath.Join(dir, p)); err != nil || string(b) != want {
			t.Errorf("%s is %q, %v; want %q", p, b, err, want)
		}
	}
	for _, p := range []string{".git/hooks", "sub"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			t.Errorf("%s was made", p)
		}
	}
	if err := w.Replace(root, "b.txt", []byte("b\n")); err != nil {
		t.Errorf("a file beside .git is refused: %v", err)
	}
}
