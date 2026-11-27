package fs_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/confine"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
)

// swappable is a state folder reached through a folder link, a junction on
// Windows: top/a leads to top/x, and the state folder is top/a/state. turn
// points a at top/y instead, as go-files' confine.Remove and LinkFolder make
// it, the way a hostile link or junction would be turned once a lock is taken.
type swappable struct {
	top, x, y, dir string
	root           *os.Root
}

func newSwappable(t *testing.T) *swappable {
	t.Helper()
	top, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &swappable{top: top, x: filepath.Join(top, "x"), y: filepath.Join(top, "y"), dir: filepath.Join(top, "a", "state")}
	for _, d := range []string{s.x, s.y} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if s.root, err = os.OpenRoot(top); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.root.Close() })
	if err := confine.LinkFolder(s.root, s.x, "a"); err != nil {
		t.Fatalf("link a to x: %v", err)
	}
	return s
}

// turn points a at y.
func (s *swappable) turn(t *testing.T) {
	t.Helper()
	if err := confine.Remove(s.root, "a"); err != nil {
		t.Fatalf("remove the link a: %v", err)
	}
	if err := confine.LinkFolder(s.root, s.y, "a"); err != nil {
		t.Fatalf("link a to y: %v", err)
	}
}

// holds is what the folder at dir holds, sorted, nil when it is missing.
func holds(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	slices.Sort(out)
	return out
}

// TestAWriteUnderTheLockGoesThroughTheFolderItWasTakenIn: once
// the lock is taken in the state folder top/a/state, with a leading to x, a is
// turned to y. Every write under the lock still lands in x/state, the folder
// the lock was taken in: the journal's record and its copy, the read of the
// records back, and the removal of the note at Release. Nothing is made in y.
// The old lock of gen --apply removes its note the same way.
//
// Plants, each run by hand: Journal.state back on the state folder's path
// (j.byPath) puts the record and the copy in y/state, and the journal row is
// red; the note's removal back on its path (os.Remove of filepath.Join(Dir,
// note)) leaves x's note behind, and the release check of both rows is red.
// TestALockWhoseFolderIsSwappedBeforeTheTakeIsRefused
// plants the swap before the take; this one plants it after.
func TestAWriteUnderTheLockGoesThroughTheFolderItWasTakenIn(t *testing.T) {
	t.Parallel()
	t.Run("the lock and the journal", func(t *testing.T) {
		t.Parallel()
		s := newSwappable(t)
		tree := newTree(t)
		l := &hostfs.Lock{Dir: s.dir, Clock: lockInstant(), Tree: tree}
		j := hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: lockInstant(), Dir: s.dir, Lock: l}
		root := openRoot(t, tree, "r")
		h, err := l.Take(t.Context(), root, model.RootLock)
		if err != nil {
			t.Fatal(err)
		}
		state := filepath.Join(s.x, "state")
		if got := holds(t, state); len(got) != 2 {
			t.Fatalf("control: x/state holds %v after the take, want the lock file and its note", got)
		}

		s.turn(t)
		if err := j.Append(t.Context(), root, model.Record{Run: "run1", Kind: model.KindIntent, Command: "apply"}); err != nil {
			t.Fatalf("append after the turn: %v", err)
		}
		if err := j.Keep(root, "run1", "f.txt", []byte("old")); err != nil {
			t.Fatalf("keep after the turn: %v", err)
		}
		records, _, err := j.Records(t.Context(), root)
		if err != nil || len(records) != 1 || records[0].Run != "run1" {
			t.Errorf("the records read after the turn are %+v, %v, want the one written", records, err)
		}
		if got := holds(t, s.y); got != nil {
			t.Errorf("y holds %v after the turn, want nothing: a write under the lock went by the state folder's path", got)
		}
		journal := slices.ContainsFunc(holds(t, state), func(n string) bool { return strings.HasSuffix(n, ".journal") })
		copies := slices.ContainsFunc(holds(t, state), func(n string) bool { return strings.HasSuffix(n, ".copies") })
		if !journal || !copies {
			t.Errorf("x/state holds %v, want the journal and the copies beside the lock", holds(t, state))
		}

		if err := l.Release(h); err != nil {
			t.Fatalf("release after the turn: %v", err)
		}
		if slices.ContainsFunc(holds(t, state), func(n string) bool { return strings.HasSuffix(n, ".holder") }) {
			t.Errorf("x/state holds %v after Release, want the note removed where it was written", holds(t, state))
		}
		if got := holds(t, s.y); got != nil {
			t.Errorf("y holds %v after Release, want nothing", got)
		}
	})
	t.Run("the old lock", func(t *testing.T) {
		t.Parallel()
		s := newSwappable(t)
		old, err := hostfs.LegacyLocks{Clock: lockInstant()}.Take(filepath.Join(s.dir, "gragas.lock"), "")
		if err != nil {
			t.Fatal(err)
		}
		state := filepath.Join(s.x, "state")
		if got := holds(t, state); !slices.Equal(got, []string{"gragas.lock", "gragas.lock.holder"}) {
			t.Fatalf("control: x/state holds %v after the take, want the lock file and its note", got)
		}

		s.turn(t)
		if err := old.Release(); err != nil {
			t.Fatalf("release after the turn: %v", err)
		}
		if got := holds(t, state); !slices.Equal(got, []string{"gragas.lock"}) {
			t.Errorf("x/state holds %v after Release, want the note removed where it was written", got)
		}
		if got := holds(t, s.y); got != nil {
			t.Errorf("y holds %v after Release, want nothing", got)
		}
	})
}
