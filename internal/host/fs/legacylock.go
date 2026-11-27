package fs

// The adapter of legacy.Locks, folded here from the old kit adapter: the lock
// the old `gen --apply` takes, through go-files/lock, as Lock in lock.go takes
// the new write path's. It serves the old commands only: api.Apply, which
// cmd/gragas and cmd/gragas-subset reach, and the run recorder of
// cli/root/recorder.go. It goes with the old commands; Lock's
// files are named by the root, and a link where one goes is refused.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/birdtable/go-files/lock"

	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// LegacyLocks fills legacy.Locks. Mounts are the configured mounts the walk
// of a lock's folder trusts, as Lock's. Clock says since when a
// take holds the lock, in its note, as Lock's Clock does.
//
// Roots, when set, answers the folders a clone carries for this run, as
// Lock.Roots does: the lock's folder must lie outside each, judged
// before anything is made or taken there, and it is made with them held apart,
// so a take there matches port.ErrCarried. The root a take names, the folder
// -C names, is held apart with them, as the Lock adds its call's root.
// The recorder takes this lock before the run's record is
// begun, so the record is never begun there either. nil judges nothing
// (runApart), the root a take names included.
type LegacyLocks struct {
	Mounts []string
	Clock  port.Clock
	Roots  func() ([]string, error)
	as     func() string
}

// Take takes the lock at path without waiting, in a folder this account owns.
// A holder's go-files error matches legacy.ErrHeld.
func (k LegacyLocks) Take(path, root string) (legacy.Lock, error) {
	var also []string
	if root != "" && k.Roots != nil {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("the folder %s the run writes in could not be found: %w", root, err)
		}
		also = append(also, abs)
	}
	apart, err := runApart(filepath.Dir(path), k.Roots, also...)
	if err != nil {
		return nil, err
	}
	l, err := oldLock(path, k.Mounts, apart, k.as, k.Clock)
	switch {
	case lock.IsHeld(err):
		return nil, heldBy{err}
	case err != nil:
		return nil, err
	}
	return l, nil
}

// oldLock takes the lock file at path through go-files' lock.TakeIn, once
// the folder that holds it is this run's own (as is a test's account, or nil):
// a link the folder's owner put at the lock's name would have the lock cut
// short the file it names. The folder is made when missing, at 0700 as Lock
// makes the state folder.
//
// As Lock's, the folder is opened as a root once and held open
// until Release: the lock file go-files took is held to the file at its name
// there, and the note is written and removed through it, never by the path, so
// a folder on the way swapped after the take takes neither.
func oldLock(path string, mounts, apart []string, as func() string, clock port.Clock) (*heldOld, error) {
	dir := filepath.Dir(path)
	if err := privateDirApart(dir, mounts, apart); err != nil {
		return nil, err
	}
	state, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	if err := mine("the state folder", state, as); err != nil {
		return nil, errors.Join(oldNotOwner{err}, state.Close())
	}
	// go-files is given no note, as Lock gives it none: the note is written
	// here through state, at the name it had before, so a run that finds the
	// lock held says since when as it did.
	l, err := lock.TakeIn(state, filepath.Base(path), "")
	if lock.IsHeld(err) {
		// A note that cannot be read names nobody, and the lock is held all
		// the same: go-files' zero Held says so.
		held, _, err := lock.HolderIn(state, holderName(filepath.Base(path)))
		if err != nil {
			held = lock.Held{}
		}
		return nil, closed(held, state)
	}
	if err != nil {
		return nil, closed(err, state)
	}
	h := &heldOld{holding{lock: l, state: state, note: holderName(filepath.Base(path))}}
	if err := writeNote(state, h.h.note, clock.Now()); err != nil {
		return nil, errors.Join(err, h.h.put())
	}
	return h, nil
}

// heldOld is the old lock held: Release removes its note through the folder
// it was taken in, drops the lock and closes the folder.
type heldOld struct{ h holding }

// Release puts the lock back. A second Release is nil, as go-files' is.
func (o *heldOld) Release() error {
	if o.h.lock == nil {
		return nil
	}
	err := o.h.put()
	o.h = holding{}
	return err
}

// heldBy is go-files' lock.Held, said as it is, that matches legacy.ErrHeld.
type heldBy struct{ error }

func (h heldBy) Is(target error) bool { return target == legacy.ErrHeld }

func (h heldBy) Unwrap() error { return h.error }
