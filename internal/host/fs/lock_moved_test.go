package fs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/confine"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
)

// TestALockWhoseFolderIsSwappedBeforeTheTakeIsTakenInTheCheckedFolder: Take and
// Wait each take the lock in the state folder they checked, never in one put at
// its path after the check, for the root lock and the scratch lock.
//
// The seam is the adapter's taking step (hostfs.LockTaking). It runs after
// the state folder is opened as a root and its owner checked, and before the
// lock is taken. There the test moves the checked folder aside and makes a
// new one at its path. go-files takes the lock by its name below the checked
// folder's own handle (lock.TakeIn), so the lock file and its note are in the
// folder that was checked and the new folder, which no check saw, holds
// nothing: nothing is written outside the held root. Before go-files took the
// lock by path, the adapter answered a lock-moved refusal and put the lock
// back; now the swap is never reached.
//
// Plants: with the lock taken by the state folder's path, the lock file is
// made in the new folder and every row is red. Controls: before the swap the
// same adapter takes and releases the lock, and after the release a second run
// takes the lock at the folder's new path, so the first holds nothing.
func TestALockWhoseFolderIsSwappedBeforeTheTakeIsTakenInTheCheckedFolder(t *testing.T) {
	t.Parallel()
	for _, caller := range []struct {
		name string
		take func(l *hostfs.Lock, ctx context.Context, root model.Root, which model.Lock) (model.Hold, error)
	}{
		{"Take", (*hostfs.Lock).Take},
		{"Wait", (*hostfs.Lock).Wait},
	} {
		for _, which := range []model.Lock{model.RootLock, model.ScratchLock} {
			t.Run(caller.name+" "+string(which), func(t *testing.T) {
				t.Parallel()
				a, b, root, _ := twoRuns(t)
				h, err := caller.take(a, t.Context(), root, which)
				if err != nil {
					t.Fatalf("control: %s before the swap: %v", caller.name, err)
				}
				if err := a.Release(h); err != nil {
					t.Fatal(err)
				}

				swapped, blocked := false, false
				var renameErr error
				hostfs.LockTaking(a, func(string) {
					if swapped || blocked {
						return
					}
					if err := os.Rename(a.Dir, a.Dir+".checked"); err != nil {
						// A system that keeps a folder with an open handle from being
						// renamed makes the swap impossible while the adapter holds
						// the checked folder open: the system is the guard there.
						blocked, renameErr = true, err
						return
					}
					swapped = true
					if err := os.Mkdir(a.Dir, 0o700); err != nil {
						t.Fatal(err)
					}
				})
				got, err := caller.take(a, t.Context(), root, which)
				if blocked {
					t.Logf("%s %s: the system refused the swap: %v", caller.name, which, renameErr)
					if _, statErr := os.Stat(a.Dir); statErr != nil {
						t.Fatalf("the system refused the swap (%v), yet the checked folder is not at its path: %v", renameErr, statErr)
					}
					if err != nil {
						t.Fatalf("%s with the swap refused by the system: %v, want the lock taken, no folder having moved", caller.name, err)
					}
					if err := a.Release(got); err != nil {
						t.Fatal(err)
					}
					return
				}
				switch {
				case !swapped:
					t.Fatalf("the taking step never ran: %+v, %v", got, err)
				case err != nil:
					t.Fatalf("%s after the swap: %v, want the lock taken in the folder that was checked", caller.name, err)
				}
				if names := namesIn(t, a.Dir, ""); len(names) != 0 {
					t.Errorf("%s wrote %q in the folder put at the state folder's path, which no check saw: nothing is written outside the held root", caller.name, names)
				}
				for _, suffix := range []string{".lock", ".holder"} {
					if names := namesIn(t, a.Dir+".checked", suffix); len(names) != 1 {
						t.Errorf("%s left %q of %s in the folder that was checked, want its lock and its note there", caller.name, names, suffix)
					}
				}
				if err := a.Release(got); err != nil {
					t.Fatalf("release of the hold taken after the swap: %v", err)
				}
				g, err := b.Take(t.Context(), root, which)
				if err != nil {
					t.Fatalf("control: a second run at the folder's path: %v", err)
				}
				if err := b.Release(g); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestALockWhoseStateFolderIsALinkTurnedBeforeTheTakeIsTakenInTheCheckedFolder
// (the row beside the swap above): where the system
// keeps a folder with an open handle from being renamed, the swap above cannot
// be made, and a link above the checked folder can still be turned. The state
// folder is a link made by confine.LinkFolder (a symbolic link, a junction on
// Windows, which needs no privilege) to one folder; at the adapter's taking
// step it is removed with confine.Remove and made again to another. The lock
// is taken by its name below the checked folder's handle, so its file and its
// note are in the first folder and the second holds nothing, for Take and Wait
// and the root and the scratch lock. The controls: the same adapter takes and
// releases the lock before the turn, and a second run after the release takes
// it at the link's new target.
func TestALockWhoseStateFolderIsALinkTurnedBeforeTheTakeIsTakenInTheCheckedFolder(t *testing.T) {
	t.Parallel()
	for _, caller := range []struct {
		name string
		take func(l *hostfs.Lock, ctx context.Context, root model.Root, which model.Lock) (model.Hold, error)
	}{
		{"Take", (*hostfs.Lock).Take},
		{"Wait", (*hostfs.Lock).Wait},
	} {
		for _, which := range []model.Lock{model.RootLock, model.ScratchLock} {
			t.Run(caller.name+" "+string(which), func(t *testing.T) {
				t.Parallel()
				base := t.TempDir()
				first, second := filepath.Join(base, "first"), filepath.Join(base, "second")
				for _, d := range []string{first, second} {
					if err := os.Mkdir(d, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				r, err := os.OpenRoot(base)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = r.Close() })
				if err := confine.LinkFolder(r, first, "state"); err != nil {
					t.Fatalf("the state folder as a link: %v", err)
				}
				dir := filepath.Join(base, "state")
				a, b := &hostfs.Lock{Dir: dir, Clock: lockInstant()}, &hostfs.Lock{Dir: dir, Clock: lockInstant()}
				root := model.Root(filepath.Join(t.TempDir(), "r1"))
				h, err := caller.take(a, t.Context(), root, which)
				if err != nil {
					t.Fatalf("control: %s before the turn: %v", caller.name, err)
				}
				if err := a.Release(h); err != nil {
					t.Fatal(err)
				}

				turned := false
				hostfs.LockTaking(a, func(string) {
					if turned {
						return
					}
					turned = true
					if err := confine.Remove(r, "state"); err != nil {
						t.Fatalf("the link taken away: %v", err)
					}
					if err := confine.LinkFolder(r, second, "state"); err != nil {
						t.Fatalf("the link made again to the other folder: %v", err)
					}
				})
				got, err := caller.take(a, t.Context(), root, which)
				switch {
				case !turned:
					t.Fatalf("the taking step never ran: %+v, %v", got, err)
				case err != nil:
					t.Fatalf("%s after the turn: %v, want the lock taken in the folder that was checked", caller.name, err)
				}
				if names := namesIn(t, second, ""); len(names) != 0 {
					t.Errorf("%s wrote %q in the folder the link was turned to, which no check saw", caller.name, names)
				}
				for _, suffix := range []string{".lock", ".holder"} {
					if names := namesIn(t, first, suffix); len(names) != 1 {
						t.Errorf("%s left %q of %s in the folder that was checked, want its lock and its note there", caller.name, names, suffix)
					}
				}
				if err := a.Release(got); err != nil {
					t.Fatalf("release of the hold taken after the turn: %v", err)
				}
				g, err := b.Take(t.Context(), root, which)
				if err != nil {
					t.Fatalf("control: a second run at the link's new target: %v", err)
				}
				if err := b.Release(g); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// namesIn is the names in dir that end in suffix, every name for "".
func namesIn(t *testing.T, dir, suffix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			names = append(names, e.Name())
		}
	}
	return names
}
