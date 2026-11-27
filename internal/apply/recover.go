package apply

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/apply/jwrite"
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// An Interrupted is an open record of a root: a run that journalled and
// never closed, because it was killed, lost its power or could not finish
// its own roll-back (architecture 5.2, A37).
type Interrupted struct {
	// Run is its run id, Command the command as it was typed and Plan its
	// plan digest, as its first record gave them.
	Run     string
	Command string
	Plan    string
	// Records are its records in the order they were written.
	Records []model.Record
}

// A Recovery is one open record a writing command rolled back before its
// own plan, for it to say so.
type Recovery struct {
	// Run and Command are the interrupted run's.
	Run     string
	Command string
}

// Open answers the open records of root, oldest first: each run with a
// record and no close. A torn line is skipped, as the journal skips it: a
// torn close leaves its run open, and rolling a finished run back leaves its
// root before the plan, which is one of the two states A37 allows.
func Open(ctx context.Context, p port.Ports, root model.Root) ([]Interrupted, error) {
	recs, err := records(ctx, p, root)
	if err != nil {
		return nil, err
	}
	return openOf(recs), nil
}

// records is every record of root's journal that reads.
func records(ctx context.Context, p port.Ports, root model.Root) ([]model.Record, error) {
	recs, _, err := p.Journal.Records(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("the journal could not be read: %w", err)
	}
	return recs, nil
}

// openOf is the runs of recs with a record and no close, oldest first.
func openOf(recs []model.Record) []Interrupted {
	at := map[string]int{}
	closed := map[string]bool{}
	var runs []Interrupted
	for _, rec := range recs {
		if rec.Kind == model.KindClose {
			closed[rec.Run] = true
		}
		i, ok := at[rec.Run]
		if !ok {
			i = len(runs)
			at[rec.Run] = i
			runs = append(runs, Interrupted{Run: rec.Run, Command: rec.Command, Plan: rec.Plan})
		}
		runs[i].Records = append(runs[i].Records, rec)
	}
	return slices.DeleteFunc(runs, func(o Interrupted) bool { return closed[o.Run] })
}

// recoverOpen rolls back every open record of the root under this run's
// lock, newest first, before the run plans. One that cannot be rolled back
// ends this run "failed" and stays open; nothing of this run's own is
// written.
//
// A run id the journal holds already is refused first: a run id names one
// run, and a record filed after that run's close would never read as open.
func (r *run) recoverOpen() (Outcome, bool) {
	recs, err := records(r.jctx, r.p, r.root)
	if err != nil {
		return r.fail(FailJournal, err), false
	}
	if slices.ContainsFunc(recs, func(rec model.Record) bool { return rec.Run == r.s.Run }) {
		return r.fail(FailWiring, fmt.Errorf("run id %s is in the journal already, and a run id names one run", r.s.Run)), false
	}
	open := openOf(recs)
	var done []Recovery
	for _, o := range slices.Backward(open) {
		err := r.rollBack(o)
		if errors.Is(err, port.ErrNotOwner) {
			return r.notMine(fmt.Errorf("run %s (%s) was interrupted and is still open: %w", o.Run, o.Command, err)), false
		}
		if err != nil {
			return r.fail(FailRecover, fmt.Errorf("run %s (%s) was interrupted and is still open: %w", o.Run, o.Command, err)), false
		}
		done = append(done, Recovery{Run: o.Run, Command: o.Command})
	}
	slices.Reverse(done)
	r.out.Recovered = done
	return Outcome{}, true
}

// undo rolls this run back from its own records in the journal, the same
// roll-back an interrupted run gets, and ends failed as it was ending. A
// roll-back that cannot finish leaves the record open and says why.
func (r *run) undo() Outcome {
	open, err := Open(r.jctx, r.p, r.root)
	if err == nil {
		i := slices.IndexFunc(open, func(o Interrupted) bool { return o.Run == r.s.Run })
		if i < 0 {
			err = errors.New("the journal holds no open record of this run")
		} else {
			err = r.rollBack(open[i])
		}
	}
	if err != nil {
		r.out.Cause = errors.Join(r.out.Cause,
			fmt.Errorf("the roll-back did not finish, and the record stays open for the next command that writes here: %w", err))
		return r.out
	}
	r.open = false
	r.out.RolledBack = true
	return r.out
}

// rollBack rolls o back through the journalled write's roll back, which
// names none of the plan, the canonical form or the ports: this binds them to
// it. Its steps and its close are journalled under o, with this engine's
// version, so o's record is what closes, and the close names this run as
// "by".
func (r *run) rollBack(o Interrupted) error {
	run := jwrite.Run{ID: o.Run, Command: o.Command, Plan: o.Plan, Engine: r.s.Engine}
	lines := make([]jwrite.Line, 0, len(o.Records))
	for _, rec := range o.Records {
		lines = append(lines, jwrite.Line{Kind: rec.Kind, Detail: rec.Detail})
	}
	return jwrite.RollBack(seamJournal{r, run}, seamFiles{r},
		jwrite.Back{Run: run, By: r.s.Run, Unsafe: r.unsafePath}, lines)
}

// A Check is the open-record check of one command (A37): the run id a
// failure carries, the command as it was typed, for the hint of busy, and
// the line that recovers an interrupted run, which "not ready" names as its
// hint. The recovery line is the caller's to give, never prose made here.
type Check struct {
	Run     string
	Command string
	Recover string
}

// Ready is the check every composition makes before it reads a root (A37,
// architecture 5.2): ok when no record of the root is open. An open record
// whose root lock another run holds is that run writing now, and ends
// "busy". One whose lock is free was interrupted, which the journal read
// again under the lock confirms, and ends "not ready", naming each such run
// and with c.Recover as the line to type: a reading command never reads a
// half-applied tree. The lock is put back before Ready answers. Each
// refusal is journalled, as a close of c.Run with the runs it found open.
func Ready(ctx context.Context, p port.Ports, root model.Root, c Check) (Outcome, bool) {
	r := &run{ctx: ctx, jctx: context.WithoutCancel(ctx), p: p, root: root, s: Spec{Run: c.Run, Command: c.Command}, out: Outcome{Run: c.Run}}
	if c.Run == "" || c.Command == "" || c.Recover == "" {
		return r.fail(FailWiring, errors.New("the check needs a run id, the command and the line that recovers")), false
	}
	open, err := Open(ctx, p, root)
	if err != nil || len(open) == 0 {
		return r.readyOr(err)
	}
	hold, err := p.Lock.Take(ctx, root, model.RootLock)
	if errors.Is(err, port.ErrHeld) {
		return r.refuseOpen(context.WithoutCancel(ctx), model.Busy, open, err.Error()+": "+runsOf(open)+" is writing this root now",
			"wait for the other run to end, then type "+c.Command+" again"), false
	}
	if errors.Is(err, port.ErrNotOwner) {
		return r.notMine(err), false
	}
	if err != nil {
		return r.fail(FailLock, err), false
	}
	open, err = Open(ctx, p, root)
	if rerr := p.Lock.Release(hold); rerr != nil {
		return r.fail(FailLock, errors.Join(fmt.Errorf("the lock could not be put back: %w", rerr), err)), false
	}
	if err != nil || len(open) == 0 {
		return r.readyOr(err)
	}
	return r.refuseOpen(context.WithoutCancel(ctx), model.NotReady, open, runsOf(open)+" was interrupted and its record is open, so the files may be half written",
		c.Recover), false
}

// readyOr is Ready's answer when the journal said no run is open, or could
// not be read.
func (r *run) readyOr(err error) (Outcome, bool) {
	if err != nil {
		return r.fail(FailJournal, err), false
	}
	return r.out, true
}

// refuseOpen journals the refusal under ctx and builds it.
func (r *run) refuseOpen(ctx context.Context, w model.Word, open []Interrupted, observed, hint string) Outcome {
	ids := canon.Array{}
	for _, o := range open {
		ids = append(ids, canon.String(o.Run))
	}
	if err := r.journal(ctx, model.KindClose, canon.Object{"outcome": canon.String(string(w)), "open": ids}); err != nil {
		return r.fail(FailJournal, err)
	}
	return r.refusal(w, observed, nil, hint)
}

// runsOf names the open runs, oldest first: "run <id> (<command>)".
func runsOf(open []Interrupted) string {
	names := make([]string, 0, len(open))
	for _, o := range open {
		names = append(names, fmt.Sprintf("run %s (%s)", o.Run, o.Command))
	}
	return strings.Join(names, " and ")
}
