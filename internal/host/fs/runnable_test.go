package fs_test

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/model"
)

// TestListLeavesARunnableFileToStat: List opens no file, so a
// program it lists is not runnable in its answer, and Stat of the same name,
// the control, is. A whole-root List that judged each file opened every one,
// about 38,000 under a checkout's ignored folders, and the opens took minutes
// on a Mac whose scanner reads each. Planted failure: List judging each file
// again, as store.entry did, answers the program runnable.
func TestListLeavesARunnableFileToStat(t *testing.T) {
	t.Parallel()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog := path.Join("deep", "er", "prog"+filepath.Ext(self))
	dir := t.TempDir()
	write(t, dir, map[string]string{"deep/er/keep": "x"})
	filestest.WriteProgram(t, filepath.Join(dir, filepath.FromSlash(prog)), []byte("x\n"))
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	e, err := tr.Stat(root, prog)
	if err != nil || !e.Exec {
		t.Fatalf("Stat(%s) = %+v, %v: the control is not runnable", prog, e, err)
	}
	for _, from := range []string{"", "deep", "deep/er"} {
		got, err := tr.List(root, from, model.Every)
		if err != nil {
			t.Fatal(err)
		}
		listed := false
		for _, e := range got {
			listed = listed || e.Path == prog
			if e.Exec {
				t.Errorf("List(%q): %s runnable = true; List opens no file, so it judges none", from, e.Path)
			}
		}
		if !listed {
			t.Errorf("List(%q) = %+v, missing %s", from, got, prog)
		}
	}
}

// TestStatOfALinkToAProgramInTheRootIsNotRunnable: List keeps a
// link out of the judging, since its walk reports the link as itself, but Stat
// follows a link inside the root, so the entry it reaches is a regular file
// that a path through a link leads to. The answer is confine.IsRunnable's, which
// refuses a link at any component, so it is false; a program asked by its own
// name is true, the control. Risk: exec.LookPath on the joined name follows the
// link and answers the program's mode, true. The
// program takes this test binary's own suffix, so this system would run it by
// its name.
func TestStatOfALinkToAProgramInTheRootIsNotRunnable(t *testing.T) {
	t.Parallel()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog := "prog" + filepath.Ext(self)
	dir := t.TempDir()
	filestest.WriteProgram(t, filepath.Join(dir, prog), []byte("x\n"))
	plant(t, dir, map[string]string{"via-" + prog: prog})
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{prog: true, "via-" + prog: false} {
		e, err := tr.Stat(root, name)
		if err != nil {
			t.Errorf("Stat(%s) = %v", name, err)
			continue
		}
		if e.Exec != want {
			t.Errorf("Stat(%s) runnable = %v, want %v", name, e.Exec, want)
		}
	}
}
