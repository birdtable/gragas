package fs_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/birdtable/go-files/trust/trusttest"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// TestALockHeldElsewhereIsHeldUntilReleased: a second take of a lock that is
// held fails as legacy.ErrHeld, and once it is released it can be taken again.
func TestALockHeldElsewhereIsHeldUntilReleased(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "gragas.lock")
	first, err := hostfs.LegacyLocks{Clock: lockInstant()}.Take(path, "")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if _, err := (hostfs.LegacyLocks{Clock: lockInstant()}).Take(path, ""); !errors.Is(err, legacy.ErrHeld) || err.Error() == legacy.ErrHeld.Error() {
		t.Fatalf("a second Take = %v, want go-files' own words matching legacy.ErrHeld", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	again, err := hostfs.LegacyLocks{Clock: lockInstant()}.Take(path, "")
	if err != nil {
		t.Fatalf("Take after Release: %v", err)
	}
	if err := again.Release(); err != nil {
		t.Fatal(err)
	}
}

// TestALockWithNowhereToLiveIsAnErrorNotHeld: a folder that cannot be made is
// an error of its own, never legacy.ErrHeld.
func TestALockWithNowhereToLiveIsAnErrorNotHeld(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (hostfs.LegacyLocks{Clock: lockInstant()}).Take(filepath.Join(file, "gragas.lock"), ""); err == nil || errors.Is(err, legacy.ErrHeld) {
		t.Errorf("Take under a file = %v, want an error that is not held", err)
	}
}

// TestAHeldLegacyLockSaysSinceWhenInItsNote: the old lock of
// gen --apply writes <name>.holder beside the lock file, naming this process
// and the instant its Clock gave, so a run that finds it held can say since
// when; Release removes the note, and a second run that finds the lock held
// reads it back. Planted: a take that skips writing the note leaves no file
// to read, and this fails on the read.
func TestAHeldLegacyLockSaysSinceWhenInItsNote(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "gragas.lock")
	held, err := hostfs.LegacyLocks{Clock: lockInstant()}.Take(path, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path + ".holder")
	var note struct {
		PID   int       `json:"pid"`
		Since time.Time `json:"since"`
	}
	if err == nil {
		err = json.Unmarshal(data, &note)
	}
	if err != nil || note.PID != os.Getpid() || !note.Since.Equal(time.Time(lockInstant())) {
		t.Errorf("the note holds %q, %v, want this process and %v", data, err, time.Time(lockInstant()))
	}
	trusttest.Perm(t, path+".holder", 0o600)
	if _, err := (hostfs.LegacyLocks{Clock: lockInstant()}).Take(path, ""); !errors.Is(err, legacy.ErrHeld) {
		t.Errorf("a second Take = %v, want held", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path + ".holder"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the note outlived the release: %v", err)
	}
}
