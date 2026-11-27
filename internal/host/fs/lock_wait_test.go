package fs_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// holdMode is the rerun helper's one mode: hold a lock in a process of its own.
const holdMode = "hold"

// gateRoot is the root of the gate lock, the lock the test holds and the
// helper waits for before it lets its own lock go.
const gateRoot model.Root = "gate"

// TestTheHelperRunsOnlyWhenRerun is a second process that holds a lock, run
// only as a child of configtest.Rerun: hold <state> <root> <which> <gate>
// <held> takes the lock which of root in the state folder, writes the file
// held to say so, waits for the gate lock in the folder gate, which the test
// holds, and then lets both go. In a run of the tests it does nothing.
func TestTheHelperRunsOnlyWhenRerun(t *testing.T) {
	t.Parallel() // in a rerun child this is the only test that runs
	args, ok := configtest.Helper()
	if !ok {
		return
	}
	if len(args) != 6 || args[0] != holdMode {
		t.Fatalf("the helper was given %q", args)
	}
	state, root, which, gateDir, held := args[1], model.Root(args[2]), model.Lock(args[3]), args[4], args[5]
	l := &hostfs.Lock{Dir: state, Clock: lockInstant()}
	h, err := l.Take(t.Context(), root, which)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(held, []byte("held\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gate := &hostfs.Lock{Dir: gateDir, Clock: lockInstant()}
	g, err := gate.Wait(t.Context(), gateRoot, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Release(h); err != nil {
		t.Error(err)
	}
	if err := gate.Release(g); err != nil {
		t.Error(err)
	}
}

// elsewhere is a lock a helper process holds until letGo.
type elsewhere struct {
	gate *hostfs.Lock
	g    model.Hold
	once sync.Once
	done chan rerun
}

// rerun is what configtest.Rerun answered for the helper.
type rerun struct {
	code int
	out  []byte
	err  error
}

// holdElsewhere starts a helper process that holds the lock which of root in
// the state folder dir, and returns once the helper says it holds it. The
// helper holds it until letGo; the test's end lets it go and waits for it.
func holdElsewhere(t *testing.T, dir string, root model.Root, which model.Lock) *elsewhere {
	t.Helper()
	signals := t.TempDir()
	e := &elsewhere{gate: &hostfs.Lock{Dir: filepath.Join(signals, "gate"), Clock: lockInstant()}, done: make(chan rerun, 1)}
	g, err := e.gate.Take(t.Context(), gateRoot, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	e.g = g
	held := filepath.Join(signals, "held")
	// The helper's run outlives t.Context, which ends before the cleanup
	// below lets the helper go; it has a minute of its own.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Minute)
	go func() {
		code, out, err := configtest.Rerun(ctx, holdMode, dir, string(root), string(which), e.gate.Dir, held)
		e.done <- rerun{code: code, out: out, err: err}
	}()
	t.Cleanup(func() {
		e.letGo(t)
		e.ended(t)
		cancel()
	})
	if !e.says(t, held) {
		t.FailNow()
	}
	return e
}

// says is whether the helper wrote the file at path before it ended or the
// test's minute ran out. A helper that ended first is reported, and its result
// kept for ended.
func (e *elsewhere) says(t *testing.T, path string) bool {
	t.Helper()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	limit := time.After(time.Minute)
	for {
		switch _, err := os.Stat(path); {
		case err == nil:
			return true
		case !errors.Is(err, fs.ErrNotExist):
			t.Error(err)
			return false
		}
		select {
		case r := <-e.done:
			t.Errorf("the helper ended before it held the lock: exit %d, %v\n%s", r.code, r.err, r.out)
			e.done <- r
			return false
		case <-limit:
			t.Error("the helper did not hold the lock within a minute")
			return false
		case <-tick.C:
		}
	}
}

// letGo lets the helper's wait for the gate end, so it lets its lock go. A
// second call does nothing.
func (e *elsewhere) letGo(t *testing.T) {
	t.Helper()
	e.once.Do(func() {
		if err := e.gate.Release(e.g); err != nil {
			t.Error(err)
		}
	})
}

// ended waits for the helper, and fails the test unless it exited 0.
func (e *elsewhere) ended(t *testing.T) {
	t.Helper()
	r := <-e.done
	e.done <- r
	if r.code != 0 || r.err != nil {
		t.Errorf("the helper exited %d, %v\n%s", r.code, r.err, r.out)
	}
}

// TestWaitTakesALockOnceAnotherProcessLetsItGo: a root lock another process
// holds is refused by Take as held, and taken by a Wait that started while it
// was held, once the holder lets it go; the hold carries the second its own
// note says. Planted failure: a Wait that only takes, which ends held at once
// while the helper still waits for the gate.
func TestWaitTakesALockOnceAnotherProcessLetsItGo(t *testing.T) {
	t.Parallel()
	a, _, root, _ := twoRuns(t)
	e := holdElsewhere(t, a.Dir, root, model.RootLock)
	if _, err := a.Take(t.Context(), root, model.RootLock); !errors.Is(err, port.ErrHeld) {
		t.Fatalf("Take of a lock another process holds: %v, want port.ErrHeld", err)
	}
	waiting := make(chan struct{})
	hostfs.LockTaking(a, func(string) { close(waiting) })
	type took struct {
		h   model.Hold
		err error
	}
	got := make(chan took, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		h, err := a.Wait(ctx, root, model.RootLock)
		got <- took{h: h, err: err}
	}()
	<-waiting
	e.letGo(t)
	r := <-got
	if r.err != nil {
		t.Fatalf("Wait: %v", r.err)
	}
	if r.h.Root != root || r.h.Which != model.RootLock || r.h.Since <= 0 {
		t.Errorf("Wait's hold is %+v, want the root lock of %s with its start time", r.h, root)
	}
	if err := a.Release(r.h); err != nil {
		t.Error(err)
	}
	e.ended(t)
}

// TestWaitPastItsDeadlineIsHeldAndTheDeadline: a scratch lock another process
// holds past the Wait's deadline ends in an error that is both port.ErrHeld,
// a HeldError with the holder's start time, and context.DeadlineExceeded.
// Planted failures: the HeldError alone, or ctx's error alone.
func TestWaitPastItsDeadlineIsHeldAndTheDeadline(t *testing.T) {
	t.Parallel()
	a, _, root, _ := twoRuns(t)
	e := holdElsewhere(t, a.Dir, root, model.ScratchLock)
	defer e.ended(t)
	defer e.letGo(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	h, err := a.Wait(ctx, root, model.ScratchLock)
	if err == nil {
		t.Fatalf("Wait took a lock another process holds: %+v", h)
	}
	if !errors.Is(err, port.ErrHeld) || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait past its deadline: %v, want port.ErrHeld and context.DeadlineExceeded", err)
	}
	var held *hostfs.HeldError
	if !errors.As(err, &held) || held.Which != model.ScratchLock || held.Since <= 0 {
		t.Errorf("Wait past its deadline: %v, want a HeldError of the scratch lock with the holder's start time", err)
	}
}

// TestWaitNeverWaitsForThisAdapter: a lock this adapter holds is refused by
// Wait at once as held, with its own hold's start time, and never with the
// deadline, as only this adapter's Release could free it; the root lock while
// it holds the scratch lock is refused as Take refuses it, and a Wait whose
// context has ended takes nothing. Planted failure: a Wait that skips the
// check of its own holds, which waits out the deadline.
func TestWaitNeverWaitsForThisAdapter(t *testing.T) {
	t.Parallel()
	a, b, root, other := twoRuns(t)
	h, err := a.Take(t.Context(), root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = a.Wait(ctx, root, model.RootLock)
	var held *hostfs.HeldError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		t.Fatalf("Wait for a lock this adapter holds waited out its deadline: %v", err)
	case !errors.As(err, &held) || !errors.Is(err, port.ErrHeld):
		t.Fatalf("Wait for a lock this adapter holds: %v, want a HeldError", err)
	case held.Since != h.Since:
		t.Errorf("the HeldError says since %d, the hold %d", held.Since, h.Since)
	}
	if err := a.Release(h); err != nil {
		t.Fatal(err)
	}

	s, err := b.Take(t.Context(), other, model.ScratchLock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Wait(ctx, root, model.RootLock); err == nil || errors.Is(err, port.ErrHeld) || !strings.Contains(err.Error(), "before the scratch lock") {
		t.Errorf("Wait for the root lock while the scratch lock is held: %v, want the order refused", err)
	}
	if err := b.Release(s); err != nil {
		t.Fatal(err)
	}

	ended, stop := context.WithCancel(t.Context())
	stop()
	if _, err := a.Wait(ended, root, model.RootLock); !errors.Is(err, context.Canceled) {
		t.Errorf("Wait with its context ended: %v, want context.Canceled", err)
	}
}
