package coreapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Spec is one plan command's write, as the write path takes it: the command,
// the run id, the scope and the plan, render and record functions
// (architecture 5). It is internal/apply's, named here because a client
// reaches the write path through the operations and never imports it.
type Spec = apply.Spec

// Outcome is how a dry run or a write ended, internal/apply's.
type Outcome = apply.Outcome

// A Change is one plan command's write through the one write path: the plan
// it shows and the apply of the plan a person agreed to. The command's
// operation makes it, and cli/root's form runs it: the dry run, the question
// at a terminal and the apply. One Change has one Spec, so the plan shown
// and the plan applied are made by one function from one command line, and
// their digests compare (cli.commands.17).
type Change struct {
	p     port.Ports
	root  model.Root
	spec  Spec
	tries int
	// waiting is told when an apply waits long for the root lock.
	waiting  func()
	lockWait time.Duration
	// unwritten answers the tools the change binds whose preset no plugin
	// writes, as the plan of the last apply found them; nil for a change that
	// binds none.
	unwritten func() []string
	// closing, when set, is what an applied run says it left and the line to
	// run next, read from the plan made under the lock: plugin upgrade says
	// the new version is installed and not approved by this upgrade.
	closing func() (what, next string)
}

// PresetsUnwritten are the tools the applied change binds that no installed
// plugin writes a preset for, in canon order: the record is written, their
// presets are not.
func (c *Change) PresetsUnwritten() []string {
	if c.unwritten == nil {
		return nil
	}
	return c.unwritten()
}

// Closing is what the applied run says it left beside the files written, and
// the next line to run: "" for a change that says nothing more. It is read
// after Apply, from the plan Apply made.
func (c *Change) Closing() (what, next string) {
	if c.closing == nil {
		return "", ""
	}
	return c.closing()
}

// NewChange is the change s makes to root through p.
func NewChange(p port.Ports, root model.Root, s Spec) *Change {
	return &Change{p: p, root: root, spec: s, lockWait: LockWait}
}

// recorded is who a plan records as making it: the command's own name, which
// the command hands in. It is never read from git: a rulebook is shared and
// published, and the runner's name and address are a person's data, which the
// names rule keeps out of it.
func recorded(who string) (string, error) {
	if who == "" {
		return "", errors.New("the change names no command to record as who made it")
	}
	return who, nil
}

// Preview is the dry run: the open-record check, then the plan, with no lock
// and no write. ok is true with the plan in the outcome, waiting for a yes.
func (c *Change) Preview(ctx context.Context) (Outcome, bool) {
	return apply.Preview(ctx, c.p, c.root, c.spec)
}

// Apply runs the write path: the lock, the plan made under it, and the
// write. agreed is the digest of the plan a person said yes to at a
// terminal, or "" under --apply. A plan that moved since it was shown ends
// refused with the new plan, and the form asks of it again; each apply files
// its journal under a run id of its own, since a run id names one run: the
// first under the spec's, each later one under the spec's with its try.
func (c *Change) Apply(ctx context.Context, agreed model.Digest) Outcome {
	s := c.spec
	if c.tries++; c.tries > 1 {
		s.Run = fmt.Sprintf("%s-%d", s.Run, c.tries)
	}
	s.Agreed = agreed
	s.TakeRoot = waitForRoot(c.lockWait, c.waiting)
	return apply.Run(ctx, c.p, c.root, s)
}

// LockWait is how long an apply waits for another run that holds the root
// lock before it ends busy (gragas#1690): a command typed while another ends
// within the time it takes is not refused.
const LockWait = 2 * time.Second

// OnWaiting sets what an apply calls once when it has waited a while for the
// root lock, a line for the person; none by default.
func (c *Change) OnWaiting(f func()) { c.waiting = f }

// WaitFor sets how long an apply waits for the root lock; LockWait by default,
// and zero takes it at once.
func (c *Change) WaitFor(d time.Duration) { c.lockWait = d }

// waitForRoot is the take of the root lock that waits up to wait for another
// run to put it back: a quarter of wait, then waiting() once, then the rest,
// then one Take more, so a lock still held answers what Take answers and the
// busy that follows says the same words as a run that never waited. A wait
// of zero is Take.
func waitForRoot(wait time.Duration, waiting func()) func(context.Context, port.Lock, model.Root) (model.Hold, error) {
	return func(ctx context.Context, l port.Lock, root model.Root) (model.Hold, error) {
		hold, err := l.Take(ctx, root, model.RootLock)
		if !errors.Is(err, port.ErrHeld) || wait <= 0 {
			return hold, err
		}
		notice := wait / 4
		for i, limit := range []time.Duration{notice, wait - notice} {
			if i == 1 && waiting != nil {
				waiting()
			}
			wctx, cancel := context.WithTimeout(ctx, limit)
			hold, err = l.Wait(wctx, root, model.RootLock)
			cancel()
			if err == nil || ctx.Err() != nil || !errors.Is(err, port.ErrHeld) {
				return hold, err
			}
		}
		return l.Take(ctx, root, model.RootLock)
	}
}
