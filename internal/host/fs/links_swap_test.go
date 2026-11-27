package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	testenv "github.com/birdtable/go-testkit"
	hostfs "github.com/birdtable/gragas/internal/host/fs"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// linkedCheckout is a checkout with .git and a real history.jsonl, opened,
// and what .git holds.
func linkedCheckout(t *testing.T) (dir string, tr *hostfs.Tree, root model.Root, before map[string][]byte) {
	t.Helper()
	dir = t.TempDir()
	before = dotGit(t, dir)
	write(t, dir, map[string]string{"history.jsonl": "{\"n\":1}\n"})
	tr = newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, tr, root, before
}

// swapIn is the seam that puts a link to .git/config in place of path's
// file between the walk and the open, once.
func swapIn(t *testing.T, dir string) func(string) {
	t.Helper()
	done := false
	return func(p string) {
		if done || p != "history.jsonl" {
			return
		}
		done = true
		full := filepath.Join(dir, "history.jsonl")
		if err := os.Remove(full); err != nil {
			t.Fatal(err)
		}
		testenv.Link(t, filepath.Join(".git", "config"), full)
	}
}

// TestALinkSwappedInAfterTheWalkIsRefused: the last component, a file when
// the walk passes it, is a link to .git/config when Read or Append opens
// it, and the open is refused before a byte moves, once per call site of
// the check. The control is the same seam doing nothing.
func TestALinkSwappedInAfterTheWalkIsRefused(t *testing.T) {
	t.Parallel()
	for _, swap := range []bool{true, false} {
		dir, tr, root, before := linkedCheckout(t)
		if swap {
			hostfs.WithOpening(tr, swapIn(t, dir))
		}
		got, err := tr.Read(root, "history.jsonl")
		switch {
		case swap && (!errors.Is(err, port.ErrOutside) || got != nil):
			t.Errorf("Read of a file swapped for a link after the walk = %q, %v, want ErrOutside", got, err)
		case !swap && (err != nil || string(got) != "{\"n\":1}\n"):
			t.Errorf("the control: Read = %q, %v", got, err)
		}
		if after := snapshot(t, filepath.Join(dir, ".git")); !equalTrees(before, after) {
			t.Errorf("swap=%v: Read changed .git: %q", swap, after)
		}
	}
	for _, swap := range []bool{true, false} {
		dir, tr, root, before := linkedCheckout(t)
		if swap {
			hostfs.WithOpening(tr, swapIn(t, dir))
		}
		err := hostfs.Writer{Tree: tr}.Append(root, "history.jsonl", []byte(`{"n":2}`))
		switch {
		case swap && !errors.Is(err, port.ErrOutside):
			t.Errorf("Append to a file swapped for a link after the walk = %v, want ErrOutside", err)
		case !swap && err != nil:
			t.Errorf("the control: Append = %v", err)
		}
		if after := snapshot(t, filepath.Join(dir, ".git")); !equalTrees(before, after) {
			t.Errorf("swap=%v: Append changed .git: %q", swap, after)
		}
	}
}

// TestAnAppendToAHardLinkIsRefused: history.jsonl is a second name of
// .git/config, which no walk sees, and Append refuses it by name with
// .git unchanged. Replace gives the path a new file and leaves the other
// name as it was, which is the control.
func TestAnAppendToAHardLinkIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := dotGit(t, dir)
	if err := os.Link(filepath.Join(dir, ".git", "config"), filepath.Join(dir, "history.jsonl")); err != nil {
		t.Fatal(err)
	}
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := hostfs.Writer{Tree: tr}
	if err := w.Append(root, "history.jsonl", []byte(`{"who":"x"}`)); !errors.Is(err, port.ErrOutside) {
		t.Errorf("Append to a hard link of .git/config = %v, want ErrOutside", err)
	}
	if err := w.Replace(root, "history.jsonl", []byte("new\n")); err != nil {
		t.Errorf("the control: Replace of the hard link = %v", err)
	}
	if after := snapshot(t, filepath.Join(dir, ".git")); !equalTrees(before, after) {
		t.Errorf(".git changed: %q", after)
	}
	if err := w.Append(root, "history.jsonl", []byte(`{"n":2}`)); err != nil {
		t.Errorf("the control: Append once the path has one name = %v", err)
	}
}

// TestASweepFollowsNoLinkInTheStateFolder: a link where the sweep would
// remove a run's copies, at the root's copies folder, at the run's folder
// or as a copy, all leading to a folder outside the state folder, refuses
// the sweep as ErrOutside. Nothing outside changes, and
// the journal is not rewritten. A folder among a run's copies is refused
// too, before a copy is removed. The control is the same journal with
// nothing planted, which sweeps the one run closed long ago.
func TestASweepFollowsNoLinkInTheStateFolder(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		outside bool
		plant   func(t *testing.T, copies, run, away string)
	}{
		{"nothing planted", false, func(*testing.T, string, string, string) {}},
		{"the root's copies folder is a link", true, func(t *testing.T, copies, _, away string) {
			t.Helper()
			moveTo(t, copies, filepath.Join(away, "copies"))
			link(t, filepath.Join(away, "copies"), copies)
		}},
		{"the run's folder is a link", true, func(t *testing.T, _, run, away string) {
			t.Helper()
			moveTo(t, run, filepath.Join(away, "closed-long-ago"))
			link(t, filepath.Join(away, "closed-long-ago"), run)
		}},
		{"a copy is a link", true, func(t *testing.T, _, run, away string) {
			t.Helper()
			kept := only(t, run)
			moveTo(t, kept, filepath.Join(away, "kept"))
			link(t, filepath.Join(away, "kept"), kept)
		}},
		{"a folder is among the copies", false, func(t *testing.T, _, run, _ string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(run, "sub"), 0o750); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, root, j := swept(t, 0, 31*24*time.Hour)
			copies := only(t, dir, "*.copies")
			run := filepath.Join(copies, "closed-long-ago")
			away := t.TempDir()
			c.plant(t, copies, run, away)
			journal := journalFile(t, dir)
			before, err := os.ReadFile(journal)
			if err != nil {
				t.Fatal(err)
			}
			outside := snapshot(t, away)
			n, err := j.Sweep(t.Context(), root)
			if c.name == "nothing planted" {
				if err != nil || n != 1 {
					t.Fatalf("Sweep = %d, %v, want 1 run swept", n, err)
				}
				return
			}
			if err == nil || n != 0 || errors.Is(err, port.ErrOutside) != c.outside {
				t.Fatalf("Sweep = %d, %v, want a refusal, ErrOutside %v", n, err, c.outside)
			}
			if after, err := os.ReadFile(journal); err != nil || string(after) != string(before) {
				t.Errorf("the journal was rewritten under a refused sweep: %v", err)
			}
			if got := snapshot(t, away); !equalTrees(got, outside) {
				t.Errorf("outside the state folder after a refused sweep = %v, want %v", got, outside)
			}
			if c.name == "a folder is among the copies" {
				if got, err := j.Kept(root, "closed-long-ago", "a"); err != nil || string(got) != "closed-long-ago" {
					t.Errorf("the copy beside the folder = %q, %v, want it kept", got, err)
				}
			}
		})
	}
}

// only is the one name in dir matching glob, "*" when none is given.
func only(t *testing.T, dir string, glob ...string) string {
	t.Helper()
	pattern := "*"
	if len(glob) > 0 {
		pattern = glob[0]
	}
	names, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(names) != 1 {
		t.Fatalf("%s holds %v, %v, want one %s", dir, names, err, pattern)
	}
	return names[0]
}

func moveTo(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, at string) {
	t.Helper()
	testenv.Link(t, target, at)
}

// TestASweepWalksEveryRunBeforeItRemovesAny: of two runs closed long ago,
// a link among the copies of the one walked second refuses the sweep
// before the first run's copies go, and the journal is not rewritten.
// Planted failure: each run walked and removed in turn.
func TestASweepWalksEveryRunBeforeItRemovesAny(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	then := time.Time(lockInstant())
	old, tree := journalOf(t, dir, then, 0)
	root := openRoot(t, tree, "a")
	for _, run := range []string{"first", "second"} {
		for _, k := range []string{model.KindIntent, model.KindClose} {
			if err := old.Append(t.Context(), root, model.Record{Run: run, Kind: k}); err != nil {
				t.Fatal(err)
			}
		}
		if err := old.Keep(root, run, "a", []byte(run)); err != nil {
			t.Fatal(err)
		}
	}
	j, _ := journalOf(t, dir, then.Add(31*24*time.Hour), 0)
	j.Tree, j.Writer = tree, hostfs.Writer{Tree: tree}
	copies := only(t, dir, "*.copies")
	away := t.TempDir()
	kept := only(t, filepath.Join(copies, "second"))
	moveTo(t, kept, filepath.Join(away, "kept"))
	link(t, filepath.Join(away, "kept"), kept)
	journal := journalFile(t, dir)
	before, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := j.Sweep(t.Context(), root); err == nil || n != 0 || !errors.Is(err, port.ErrOutside) {
		t.Fatalf("Sweep = %d, %v, want refused, ErrOutside", n, err)
	}
	if got, err := j.Kept(root, "first", "a"); err != nil || string(got) != "first" {
		t.Errorf("the first run's copy = %q, %v, want it kept", got, err)
	}
	if after, err := os.ReadFile(journal); err != nil || string(after) != string(before) {
		t.Errorf("the journal was rewritten under a refused sweep: %v", err)
	}
}
