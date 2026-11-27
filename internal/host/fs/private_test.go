package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/go-files/filestest"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TestNothingIsMadeThroughAStateFolderNameTheWalkRefuses: the lock, the old
// lock and the journal each
// judge every name of the state folder that is there before anything is
// made, so a refused Take or Append leaves nothing wherever a name another
// account controls leads. Risk: a link in a folder others may write sends the
// making of the state folder, and the lock file or the journal in it, into a
// folder of that account's choosing. The probe is the state folder open/c/
// state, open written by others and c a link to the victim, the state folder
// itself a link, open/c2 to the victim, and open/new, not there yet, whose
// nearest folder that is there, open, others may write. The control is a
// state folder that is not there yet in a private folder, made, and the lock
// taken in it, as before. Planted: privateDirApart making the folders before the
// walk (each victim then holds the state folder and a lock or a journal, red
// here).
func TestNothingIsMadeThroughAStateFolderNameTheWalkRefuses(t *testing.T) {
	t.Parallel()
	for _, use := range []struct {
		what string
		call func(t *testing.T, dir string) error
	}{
		{"the lock", func(t *testing.T, dir string) error {
			t.Helper()
			h, err := (&hostfs.Lock{Dir: dir, Clock: lockInstant()}).Take(t.Context(), model.Root(filepath.Join(t.TempDir(), "r")), model.RootLock)
			if err == nil {
				t.Errorf("Take = %v, want refused", h)
			}
			return err
		}},
		{"the old lock", func(t *testing.T, dir string) error {
			t.Helper()
			l, err := hostfs.LegacyLocks{Clock: lockInstant()}.Take(filepath.Join(dir, "gragas.lock"), "")
			if err == nil {
				t.Errorf("Take = %v, want refused", l)
			}
			return err
		}},
		{"the journal", func(t *testing.T, dir string) error {
			t.Helper()
			j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
			return j.Append(t.Context(), openRoot(t, tree, "a"), model.Record{Run: "run-1", Kind: model.KindIntent})
		}},
	} {
		for _, tc := range []struct {
			what string
			// dir is the state folder, and empty the folder that must hold
			// nothing after the refusal.
			dir func(t *testing.T, open, victim string) (dir, empty string)
		}{
			{"below a link in a folder others may write", func(t *testing.T, open, victim string) (string, string) {
				t.Helper()
				testenv.Link(t, victim, filepath.Join(open, "c"))
				return filepath.Join(open, "c", "state"), victim
			}},
			{"that is a link in a folder others may write", func(t *testing.T, open, victim string) (string, string) {
				t.Helper()
				testenv.Link(t, victim, filepath.Join(open, "c2"))
				return filepath.Join(open, "c2"), victim
			}},
			{"not there yet, whose nearest folder others may write", func(_ *testing.T, open, _ string) (string, string) {
				return filepath.Join(open, "new"), open
			}},
		} {
			t.Run(use.what+", a state folder "+tc.what, func(t *testing.T) {
				t.Parallel()
				open, victim := filepath.Join(private(t), "open"), private(t)
				if err := os.Mkdir(open, 0o700); err != nil {
					t.Fatal(err)
				}
				dir, empty := tc.dir(t, open, victim)
				trusttest.Loosen(t, open, trusttest.OthersWrite)
				err := use.call(t, dir)
				if err == nil || errors.Is(err, port.ErrHeld) || !strings.Contains(err.Error(), dir) || !strings.Contains(err.Error(), open) {
					t.Errorf("%s in %s = %v; want refused by the state folder's name and %s's", use.what, dir, err, open)
				}
				if left := names(t, empty); len(left) != 0 {
					t.Errorf("%s holds %v after %s was refused, want nothing", empty, left, use.what)
				}
			})
		}
	}
	t.Run("the control, a state folder that is not there yet", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(private(t), "new", "state")
		l := &hostfs.Lock{Dir: dir, Clock: lockInstant()}
		h, err := l.Take(t.Context(), model.Root(filepath.Join(t.TempDir(), "r")), model.RootLock)
		if err != nil {
			t.Fatalf("Take in a state folder not there yet = %v; want it made and the lock taken", err)
		}
		if err := l.Release(h); err != nil {
			t.Errorf("Release = %v", err)
		}
		if left := names(t, dir); len(left) == 0 {
			t.Errorf("the state folder %s holds nothing; want the lock file", dir)
		}
	})
}

// TestTheStateFolderWalkTrustsTheConfiguredMounts:
// the lock, the old lock and the journal walk the state folder with
// the configured mounts, as every other walk of the product does, so a state
// folder on a nosuid disk GRAGAS_MOUNTS names passes there. Risk: on such a
// disk, each refused with a fix line that names the setting the person
// already set. A nosuid disk needs a namespace to plant; what shows the
// mounts reach the walk on any system is go-files' judgment of each entry
// before it walks: a configured mount that names nothing is
// refused by its name, with MountEntryFix, where go-files judges entries, and
// the call passes where it judges none. Planted: the walk with hosttrust.Who{}
// (each call passes, red where entries are judged, as on the Mac).
func TestTheStateFolderWalkTrustsTheConfiguredMounts(t *testing.T) {
	t.Parallel()
	for _, use := range []struct {
		what string
		call func(t *testing.T, dir string, mounts []string) error
	}{
		{"the lock", func(t *testing.T, dir string, mounts []string) error {
			t.Helper()
			l := &hostfs.Lock{Dir: dir, Mounts: mounts, Clock: lockInstant()}
			h, err := l.Take(t.Context(), model.Root(filepath.Join(t.TempDir(), "r")), model.RootLock)
			if err == nil {
				return l.Release(h)
			}
			return err
		}},
		{"the old lock", func(t *testing.T, dir string, mounts []string) error {
			t.Helper()
			l, err := hostfs.LegacyLocks{Mounts: mounts, Clock: lockInstant()}.Take(filepath.Join(dir, "gragas.lock"), "")
			if err == nil {
				return l.Release()
			}
			return err
		}},
		{"the journal", func(t *testing.T, dir string, mounts []string) error {
			t.Helper()
			j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
			j.Mounts = mounts
			return j.Append(t.Context(), openRoot(t, tree, "a"), model.Record{Run: "run-1", Kind: model.KindIntent})
		}},
	} {
		t.Run(use.what, func(t *testing.T) {
			t.Parallel()
			base := private(t)
			judged := mountsJudged(t, base)
			m := filepath.Join(base, "gone")
			err := use.call(t, filepath.Join(base, "state"), []string{m})
			u, ok := errors.AsType[*hosttrust.Unsafe](err)
			refused := ok && strings.HasPrefix(u.Observed, "the mount "+m) && u.Fix == hosttrust.MountEntryFix
			if judged && !refused || !judged && err != nil {
				t.Errorf("%s with the configured mount %s, which names nothing = %v; want the mount refused by its name, with MountEntryFix, where go-files judges entries (%v), else none", use.what, m, err, judged)
			}
		})
	}
}

// mountsJudged is whether go-files judges a configured mount on this system,
// asked of go-files itself: a walk of dir passes with no
// mount, and is refused with an entry that is not absolute wherever go-files
// judges one. Where it judges none, the test holds that the call passes and
// says so in its log, since a skip needs an allow-list entry
// (tests.admission.5).
func mountsJudged(t *testing.T, dir string) bool {
	t.Helper()
	probe := hosttrust.Subject{Name: "the probe", Last: hosttrust.Folder}
	if _, err := hosttrust.Walk(probe, dir, hosttrust.Who{}); err != nil {
		t.Fatalf("the probe's walk of %s with no mount: %v", dir, err)
	}
	if _, err := hosttrust.Walk(probe, dir, hosttrust.Who{Mounts: []string{"relative"}}); err == nil {
		t.Log("go-files judges no configured mount on this system: the mounts are held to pass unjudged")
		return false
	}
	return true
}

// private is a folder of this account's that the trust walk passes on every
// system, its links resolved, so a test's own names are all it plants.
func private(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(filestest.Private(t))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
