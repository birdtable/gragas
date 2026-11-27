package fs

import (
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// HeldError is the error of a Take that found the lock held: which lock of
// which root, and since when. Since is the holder's start time in seconds,
// UTC, and 0 when the holder has not recorded one yet, which is said as it is:
// a time that could not be read is unread, never a guess.
type HeldError struct {
	Root  model.Root
	Which model.Lock
	Since int64
}

func (e *HeldError) Error() string {
	name := "the " + string(e.Which) + " lock of " + string(e.Root) + " is held by another run"
	if e.Which == model.ScratchLock {
		name = "the scratch lock, one for every folder on this machine, is held by another run"
	}
	if e.Since <= 0 {
		return name + ", since a time it has not recorded"
	}
	return name + " since " + time.Unix(e.Since, 0).UTC().Format(time.RFC3339)
}

// Is makes a HeldError match port.ErrHeld.
func (e *HeldError) Is(target error) bool { return target == port.ErrHeld }
