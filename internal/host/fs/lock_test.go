package fs_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-files/trust/trusttest"

	"github.com/birdtable/gragas/internal/digest"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// at is a clock stopped at an instant, the one the journal tests read.
type at time.Time

func (a at) Now() time.Time { return time.Time(a) }

// lockInstant is the instant the tests' journals are written at.
func lockInstant() at { return at(time.Date(2026, 11, 8, 6, 0, 0, 0, time.UTC)) }

// twoRuns are two views of one state folder, as two runs of gragas are, and the
// two roots they lock. The state folder is short (tests.isolation.5).
func twoRuns(t *testing.T) (a, b *hostfs.Lock, root, other model.Root) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	a = &hostfs.Lock{Dir: dir, Clock: lockInstant()}
	b = &hostfs.Lock{Dir: dir, Clock: lockInstant()}
	return a, b, model.Root(filepath.Join(t.TempDir(), "r1")), model.Root(filepath.Join(t.TempDir(), "r2"))
}

func TestTheLockAdapterPassesTheLockSuite(t *testing.T) {
	t.Parallel()
	contracttest.Lock(t, func(t *testing.T) contracttest.LockSetup {
		t.Helper()
		a, b, root, other := twoRuns(t)
		return contracttest.LockSetup{A: a, B: b, Root: root, Other: other, Since: func(err error) (int64, bool) {
			var held *hostfs.HeldError
			if errors.As(err, &held) && held.Since > 0 {
				return held.Since, true
			}
			return 0, false
		}}
	})
}

// TestALockHoldsItsHoldersStartTimeAndNothingElse: the lock file and its
// note, <name>.holder, are all the state folder holds; the note says the
// holder's process and the second it took the lock, as go-files writes them,
// and no more; the hold carries that second; both are private to the
// account, and the lock is keyed by the root, not named for it.
func TestALockHoldsItsHoldersStartTimeAndNothingElse(t *testing.T) {
	t.Parallel()
	a, _, root, _ := twoRuns(t)
	h, err := a.Take(t.Context(), root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	names, err := os.ReadDir(a.Dir)
	if err != nil || len(names) != 2 {
		t.Fatalf("the state folder holds %v, %v, want the lock file and its note", names, err)
	}
	name := names[0].Name()
	if strings.Contains(name, filepath.Base(string(root))) || !strings.HasSuffix(name, ".root.lock") || names[1].Name() != name+".holder" {
		t.Errorf("the state folder holds %v, want the digest of the root, not its name, .root.lock and its note", names)
	}
	data, err := os.ReadFile(filepath.Join(a.Dir, name+".holder"))
	var note struct {
		PID   int       `json:"pid"`
		Since time.Time `json:"since"`
	}
	if err == nil {
		err = json.Unmarshal(data, &note)
	}
	if err != nil || h.Since <= 0 || note.PID != os.Getpid() || note.Since.Unix() != h.Since {
		t.Errorf("the note holds %q, %v, and the hold %d, want this process and the hold's second", data, err, h.Since)
	}
	for _, n := range []string{name, name + ".holder"} {
		trusttest.Perm(t, filepath.Join(a.Dir, n), 0o600)
	}
	if err := a.Release(h); err != nil {
		t.Fatal(err)
	}
}

// TestALockWithNoStateFolderOrAnUnknownLockIsRefused: a Lock the wiring left
// without a state folder takes nothing, rather than a lock in the working
// folder, and a lock that is neither of the two, or of no root, is no lock.
func TestALockWithNoStateFolderOrAnUnknownLockIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := (&hostfs.Lock{Clock: lockInstant()}).Take(t.Context(), "/r", model.RootLock); err == nil || errors.Is(err, port.ErrHeld) {
		t.Errorf("Take with no state folder = %v, want an error that is not held", err)
	}
	a, _, root, _ := twoRuns(t)
	for name, c := range map[string]struct {
		root  model.Root
		which model.Lock
	}{"no root": {"", model.RootLock}, "a third lock": {root, "third"}} {
		if _, err := a.Take(t.Context(), c.root, c.which); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
}

// TestAStateFolderThatCannotBeMadeIsAnErrorNotHeld: a state folder that cannot be
// made is an error of its own, never ErrHeld.
func TestAStateFolderThatCannotBeMadeIsAnErrorNotHeld(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	l := &hostfs.Lock{Dir: filepath.Join(file, "state"), Clock: lockInstant()}
	if _, err := l.Take(t.Context(), "/r", model.RootLock); err == nil || errors.Is(err, port.ErrHeld) {
		t.Errorf("Take under a file = %v, want an error that is not held", err)
	}
}

// TestALockIsReleasedOnEveryPath: a function that takes a lock and releases it
// in a defer gives it back when it returns, when it panics and when its context
// ended.
func TestALockIsReleasedOnEveryPath(t *testing.T) {
	t.Parallel()
	a, b, root, _ := twoRuns(t)
	free := func(when string) {
		t.Helper()
		h, err := b.Take(t.Context(), root, model.RootLock)
		if err != nil {
			t.Fatalf("after %s the lock is still held: %v", when, err)
		}
		if err := b.Release(h); err != nil {
			t.Fatal(err)
		}
	}
	use := func(fail func()) {
		h, err := a.Take(t.Context(), root, model.RootLock)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := a.Release(h); err != nil {
				t.Error(err)
			}
		}()
		fail()
	}
	use(func() {})
	free("a return")
	func() {
		defer func() { _ = recover() }()
		use(func() { panic("the work failed") })
	}()
	free("a panic")
}

// lockName is the lock file's name in the state folder for which lock of root.
func lockName(root model.Root, which model.Lock) string {
	return digest.Hex([]byte(root)) + "." + string(which) + ".lock"
}

// TestALockFileNameThatIsNoFileIsRefused: a folder where the lock file goes
// is an error of its own, never ErrHeld, and nothing is taken; the control is
// the same name once the folder is gone.
func TestALockFileNameThatIsNoFileIsRefused(t *testing.T) {
	t.Parallel()
	a, _, root, _ := twoRuns(t)
	at := filepath.Join(a.Dir, lockName(root, model.RootLock))
	if err := os.MkdirAll(at, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Take(t.Context(), root, model.RootLock); err == nil || errors.Is(err, port.ErrHeld) || !strings.Contains(err.Error(), "not a file") {
		t.Errorf("Take over a folder = %v, want refused as no file, not held", err)
	}
	if err := os.Remove(at); err != nil {
		t.Fatal(err)
	}
	h, err := a.Take(t.Context(), root, model.RootLock)
	if err != nil {
		t.Fatalf("with the folder gone the lock was not taken: %v", err)
	}
	if err := a.Release(h); err != nil {
		t.Fatal(err)
	}
}

// TestTheStartTimeIsReadBackOnlyFromThisProcesssNote: Take reads the second
// back from a note, <name>.holder, that names this process and a time after
// 1970, and from nothing else: another process's note, a note with no time,
// a torn one, a time it cannot parse and an empty one are each an error.
func TestTheStartTimeIsReadBackOnlyFromThisProcesssNote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pid := strconv.Itoa(os.Getpid())
	then := time.Time(lockInstant()).UTC().Format(time.RFC3339)
	for name, c := range map[string]struct {
		note string
		want int64
	}{
		"this process's":   {`{"pid":` + pid + `,"since":"` + then + `"}` + "\n", time.Time(lockInstant()).Unix()},
		"another's":        {`{"pid":` + strconv.Itoa(os.Getpid()+1) + `,"since":"` + then + `"}` + "\n", 0},
		"no time":          {`{"pid":` + pid + `}` + "\n", 0},
		"torn":             {`{"pid":` + pid + `,"since":"` + then, 0},
		"no time it reads": {`{"pid":` + pid + `,"since":"yesterday"}` + "\n", 0},
		"before 1970":      {`{"pid":` + pid + `,"since":"1969-12-31T23:59:59Z"}` + "\n", 0},
		"empty":            {"", 0},
	} {
		if err := os.WriteFile(filepath.Join(dir, "x.lock.holder"), []byte(c.note), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := hostfs.Noted(dir, "x.lock")
		if c.want != 0 && (err != nil || got != c.want) {
			t.Errorf("%s: %d, %v, want %d", name, got, err, c.want)
		}
		if c.want == 0 && (err == nil || got != 0) {
			t.Errorf("%s: %d, %v, want an error", name, got, err)
		}
	}
}
