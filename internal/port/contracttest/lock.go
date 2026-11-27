package contracttest

import (
	"context"
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// LockSetup is the Lock under test as two views that contend, as two runs of
// gragas do: A and B over one state folder, two roots they may lock, both
// opened, and Since, which reads from a busy answer the start time it names.
type LockSetup struct {
	A, B        port.Lock
	Root, Other model.Root
	Since       func(err error) (since int64, named bool)
}

// LockMaker makes the setup for one subtest.
type LockMaker func(t *testing.T) LockSetup

// Lock runs the Lock suite against the port build answers.
func Lock(t *testing.T, build LockMaker) {
	t.Helper()
	lockTaking(t, build)
	lockSeparate(t, build)
	lockOrder(t, build)
	lockReleasing(t, build)
}

// takeFor takes a lock and fails the test when it is not given.
func takeFor(t *testing.T, l port.Lock, root model.Root, which model.Lock) model.Hold {
	t.Helper()
	h, err := l.Take(t.Context(), root, which)
	if err != nil {
		t.Fatalf("Take(%s, %s) = %v", root, which, err)
	}
	return h
}

// giveBack releases a hold and fails the test when it is refused.
func giveBack(t *testing.T, l port.Lock, h model.Hold) {
	t.Helper()
	if err := l.Release(h); err != nil {
		t.Fatalf("Release(%+v) = %v", h, err)
	}
}

func lockTaking(t *testing.T, build LockMaker) {
	t.Helper()
	t.Run("a lock taken records the time and is held until released", func(t *testing.T) {
		s := build(t)
		a, b, root := s.A, s.B, s.Root
		h := takeFor(t, a, root, model.RootLock)
		if h.Root != root || h.Which != model.RootLock || h.Since <= 0 {
			t.Errorf("Take = %+v, want a hold of the root lock with its start time", h)
		}
		if _, err := b.Take(t.Context(), root, model.RootLock); !errors.Is(err, port.ErrHeld) {
			t.Errorf("a second take = %v, want ErrHeld", err)
		}
		giveBack(t, a, h)
		giveBack(t, b, takeFor(t, b, root, model.RootLock))
	})
	t.Run("a busy answer says since when and gives no hold", func(t *testing.T) {
		s := build(t)
		a, b, root := s.A, s.B, s.Root
		h := takeFor(t, a, root, model.RootLock)
		got, err := b.Take(t.Context(), root, model.RootLock)
		if since, named := s.Since(err); !errors.Is(err, port.ErrHeld) || !named || since != h.Since {
			t.Errorf("a held lock = %v, want ErrHeld naming since %d", err, h.Since)
		}
		if got != (model.Hold{}) {
			t.Errorf("a held lock answered the hold %+v, want the zero hold", got)
		}
		if err := b.Release(got); err == nil {
			t.Error("the zero hold was released")
		}
		// Given back, as every case gives back what it takes: a lock file
		// still open is one Windows will not let the test's cleanup remove.
		giveBack(t, a, h)
	})
}

func lockSeparate(t *testing.T, build LockMaker) {
	t.Helper()
	t.Run("the root lock and the scratch lock, and two roots, are separate", func(t *testing.T) {
		s := build(t)
		a, b, root, other := s.A, s.B, s.Root, s.Other
		cases := []struct {
			root  model.Root
			which model.Lock
		}{{root, model.ScratchLock}, {other, model.RootLock}}
		if len(cases) == 0 {
			t.Fatal("no lock to take beside the root lock")
		}
		for _, c := range cases {
			h := takeFor(t, a, root, model.RootLock)
			giveBack(t, b, takeFor(t, b, c.root, c.which))
			giveBack(t, a, h)
		}
	})
	// The scratch folders of every root are made in one folder, so a clear
	// from one root that took only its own scratch lock would remove the
	// folder a run of another root holds.
	t.Run("the scratch lock is one for every root", func(t *testing.T) {
		s := build(t)
		a, b, root, other := s.A, s.B, s.Root, s.Other
		h := takeFor(t, a, root, model.ScratchLock)
		if got, err := b.Take(t.Context(), other, model.ScratchLock); !errors.Is(err, port.ErrHeld) || got != (model.Hold{}) {
			t.Errorf("the scratch lock of another root while one is held = %+v, %v, want ErrHeld and no hold", got, err)
		}
		giveBack(t, a, h)
		giveBack(t, b, takeFor(t, b, other, model.ScratchLock))
	})
}

func lockOrder(t *testing.T, build LockMaker) {
	t.Helper()
	t.Run("root then scratch is the order", func(t *testing.T) {
		su := build(t)
		a, root := su.A, su.Root
		r := takeFor(t, a, root, model.RootLock)
		s := takeFor(t, a, root, model.ScratchLock)
		giveBack(t, a, s)
		giveBack(t, a, r)
	})
	t.Run("scratch then root is refused, and not as held", func(t *testing.T) {
		su := build(t)
		a, root := su.A, su.Root
		s := takeFor(t, a, root, model.ScratchLock)
		if h, err := a.Take(t.Context(), root, model.RootLock); err == nil || errors.Is(err, port.ErrHeld) || h != (model.Hold{}) {
			t.Errorf("root after scratch = %+v, %v, want an error of its own and no hold", h, err)
		}
		giveBack(t, a, s)
		giveBack(t, a, takeFor(t, a, root, model.RootLock))
	})
}

func lockReleasing(t *testing.T, build LockMaker) {
	t.Helper()
	t.Run("a release of a lock not held, or twice, is an error", func(t *testing.T) {
		s := build(t)
		a, root := s.A, s.Root
		if err := a.Release(model.Hold{Root: root, Which: model.RootLock, Since: 1}); err == nil {
			t.Error("a lock never taken was released")
		}
		h := takeFor(t, a, root, model.RootLock)
		giveBack(t, a, h)
		if err := a.Release(h); err == nil {
			t.Error("a lock was released twice")
		}
	})
	t.Run("a context that ended takes nothing, and a release needs none", func(t *testing.T) {
		s := build(t)
		a, b, root, other := s.A, s.B, s.Root, s.Other
		ctx, cancel := context.WithCancel(t.Context())
		h, err := a.Take(ctx, root, model.RootLock)
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		if _, err := b.Take(ctx, other, model.RootLock); !errors.Is(err, context.Canceled) {
			t.Errorf("a take on an ended context = %v, want context.Canceled", err)
		}
		giveBack(t, a, h)
		giveBack(t, b, takeFor(t, b, root, model.RootLock))
		giveBack(t, b, takeFor(t, b, other, model.RootLock))
	})
}
