package coreapi_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// heldRoot is a change over a root whose lock another process holds since a
// time, built over ports the test may wrap.
func heldRoot(t *testing.T, wrap func(port.Lock) port.Lock) (*coreapi.Change, *porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("/work/rulebook", map[string]string{"identity.yaml": "name: demo\n", "rules/a.yaml": "status: active\n"})
	w.HoldLock(root, model.RootLock, 1794132000)
	ports := w.Ports()
	if wrap != nil {
		ports.Lock = wrap(ports.Lock)
	}
	c := coreapi.NewChange(ports, root, coreapi.Spec{
		Command: "gragas test", Applies: "gragas test --apply", Engine: "v0.0.0-test", Run: "run-1",
		Scope: []string{"rules/*.yaml"},
		Plan: func(context.Context) (model.Plan, error) {
			return model.Plan{
				Delta:  model.Delta{Changes: []model.Change{{Entity: "rule:a", Field: "status", Before: "active", After: "retired"}}},
				Inputs: []model.Input{{Path: "identity.yaml", Digest: digestOf(t, w, root, "identity.yaml")}},
			}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) {
			return []model.Target{{Path: "rules/a.yaml", Before: digestOf(t, w, root, "rules/a.yaml"), Data: []byte("status: retired\n")}}, nil
		},
	})
	return c, w, root
}

// freeingLock lets go of another process's hold the moment a Wait begins.
type freeingLock struct {
	port.Lock
	free func()
}

func (l freeingLock) Wait(ctx context.Context, root model.Root, which model.Lock) (model.Hold, error) {
	l.free()
	return l.Lock.Wait(ctx, root, which)
}

// TestAHeldLockEndsBusyWithTheSameWordsAfterTheWait (gragas#1690): an apply
// that waits for the root lock and finds it held to the end says what an
// apply that never waited says: the words of busy name the holder's time and
// nothing of the wait, so exit 3 and its line do not change.
// Risk: a script that reads busy's line, and a person who is told why.
func TestAHeldLockEndsBusyWithTheSameWordsAfterTheWait(t *testing.T) {
	t.Parallel()
	c, w, _ := heldRoot(t, nil)
	c.WaitFor(0)
	plain := c.Apply(t.Context(), "")
	c.WaitFor(80 * time.Millisecond)
	waited := c.Apply(t.Context(), "")

	if waited.Word != model.Busy || waited.Refusal == nil || plain.Refusal == nil {
		t.Fatalf("ended %q %+v, want busy", waited.Word, waited.Refusal)
	}
	if got, want := waited.Refusal.Observed(), plain.Refusal.Observed(); got != want {
		t.Errorf("busy after the wait says %q, want the words of an apply that did not wait: %q", got, want)
	}
	if len(w.Writes) != 0 {
		t.Errorf("a busy apply wrote %v", w.Writes)
	}
}

// TestALockFreedDuringTheWaitIsTakenAndTheApplyGoesOn (gragas#1690): the other
// run ends inside the wait, and this apply takes the lock and writes, with the
// waiting line told once.
// Risk: a command typed a moment after another is refused for no reason.
func TestALockFreedDuringTheWaitIsTakenAndTheApplyGoesOn(t *testing.T) {
	t.Parallel()
	c, w, root := heldRoot(t, nil)
	var told atomic.Int32
	c.WaitFor(200 * time.Millisecond)
	c.OnWaiting(func() {
		told.Add(1)
		w.FreeLock(root, model.RootLock)
	})

	o := c.Apply(t.Context(), "")

	if o.Word != model.Done {
		t.Fatalf("ended %q %+v, want done once the holder let go", o.Word, o.Refusal)
	}
	if told.Load() != 1 {
		t.Errorf("the waiting line was told %d times, want once", told.Load())
	}
}

// TestALockFreedAtOnceTellsNoWaitingLine (gragas#1690): a holder that lets go
// as the wait begins costs the person no line.
func TestALockFreedAtOnceTellsNoWaitingLine(t *testing.T) {
	t.Parallel()
	var free func()
	c, w, root := heldRoot(t, func(l port.Lock) port.Lock {
		return freeingLock{Lock: l, free: func() { free() }}
	})
	free = func() { w.FreeLock(root, model.RootLock) }
	var told atomic.Int32
	c.WaitFor(2 * time.Second)
	c.OnWaiting(func() { told.Add(1) })

	o := c.Apply(t.Context(), "")

	if o.Word != model.Done || told.Load() != 0 {
		t.Errorf("ended %q, told %d times, want done with no line", o.Word, told.Load())
	}
}
