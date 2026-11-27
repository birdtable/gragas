package coreapi

// cache clear: the closed runs of a root's journal older than the keep age
// are removed with their copies, and every open run stays, whatever its age
// (an open record is evidence a roll-back needs). The scratch folders a stopped
// run left are removed with them: a run of any root takes the scratch lock,
// one for the machine, while it holds them, so a clear that holds it holds
// none. The folders a grant's caches are kept in from one step to the next
// go with them, through the Steps, whatever their age: a cache is the tool's
// to fill again, and gragas keeps no record of when one was last used. No
// lock keeps a step off them: a step that runs while the clear sweeps may
// lose its cache and fail, and the next step makes the folders again. The
// plan reads alone, and counts the scratch folders through the Sandbox and
// the cache folders through the Steps, each by the rule its sweep removes
// by; the clear takes the root lock and then the scratch lock
// (store.consistency.1), and calls the Journal's own Sweep, the Sandbox's
// and the Steps'.

import (
	"context"
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The types below live in the model, so the kit can name them.
type (
	// ClearPlan is what a clear would do, as the model holds it.
	ClearPlan = model.ClearPlan
	// Cleared is how a clear ended, as the model holds it.
	Cleared = model.Cleared
)

// CacheClearPlan counts, from the journal, the Sandbox and the Steps alone,
// what a clear would remove at keepSeconds: the scratch folders are the ones
// the Sandbox's Sweep would remove now, and the cache folders the ones the
// Steps' SweepCaches would. It writes nothing.
func CacheClearPlan(ctx context.Context, ports port.Ports, root model.Root, keepSeconds int64) (ClearPlan, error) {
	recs, _, err := ports.Journal.Records(ctx, root)
	if err != nil {
		return ClearPlan{}, fmt.Errorf("the journal could not be read: %w", err)
	}
	cutoff := ports.Clock.Now().Unix() - keepSeconds
	out := ClearPlan{Schema: JournalSchema, Runs: []string{}}
	for _, g := range groups(recs) {
		switch {
		case g.shut == nil:
			out.Open++
		case g.shut.At <= cutoff:
			out.Runs = append(out.Runs, g.run.Run)
			out.Remove++
		default:
			out.Keep++
		}
	}
	if out.Scratch, err = ports.Sandbox.Left(); err != nil {
		return ClearPlan{}, fmt.Errorf("the scratch folders could not be counted: %w", err)
	}
	if out.Tools, err = ports.Steps.CachesLeft(); err != nil {
		return ClearPlan{}, fmt.Errorf("the tools' cache folders could not be counted: %w", err)
	}
	return out, nil
}

// CacheClear removes the closed runs past the keep age from the journal of
// root, every scratch folder no run of any root holds, and every folder a
// grant's caches are kept in, under the root lock and then the scratch lock.
// Another run holding either lock ends it busy and removes nothing. A sweep
// that fails after an earlier one removed something says how many runs and
// folders were removed.
func CacheClear(ctx context.Context, ports port.Ports, root model.Root) (Cleared, error) {
	hold, err := ports.Lock.Take(ctx, root, model.RootLock)
	if errors.Is(err, port.ErrHeld) {
		return Cleared{Schema: JournalSchema, Word: model.Busy, Observed: err.Error()}, nil
	}
	if err != nil {
		return Cleared{}, fmt.Errorf("the lock could not be taken: %w", err)
	}
	scratch, err := ports.Lock.Take(ctx, root, model.ScratchLock)
	if errors.Is(err, port.ErrHeld) {
		busy := Cleared{Schema: JournalSchema, Word: model.Busy, Observed: err.Error()}
		if rerr := ports.Lock.Release(hold); rerr != nil {
			return Cleared{}, fmt.Errorf("the lock could not be put back: %w", rerr)
		}
		return busy, nil
	}
	if err != nil {
		rerr := ports.Lock.Release(hold)
		return Cleared{}, errors.Join(fmt.Errorf("the scratch lock could not be taken: %w", err), rerr)
	}
	n, serr := ports.Journal.Sweep(ctx, root)
	if serr != nil {
		serr = fmt.Errorf("the journal could not be swept: %w", serr)
	}
	var folders, tools int
	if serr == nil {
		if folders, serr = ports.Sandbox.Sweep(); serr != nil {
			serr = fmt.Errorf("%s and %s were removed, and then the scratch folders could not be swept: %w",
				model.Plural(n, "run"), model.Plural(folders, "scratch folder"), serr)
		}
	}
	if serr == nil {
		if tools, serr = ports.Steps.SweepCaches(); serr != nil {
			serr = fmt.Errorf("%s, %s and %s were removed, and then the tools' cache folders could not be swept: %w",
				model.Plural(n, "run"), model.Plural(folders, "scratch folder"), model.Plural(tools, "cache folder"), serr)
		}
	}
	for _, l := range []struct {
		h    model.Hold
		name string
	}{{scratch, "the scratch lock"}, {hold, "the lock"}} {
		if rerr := ports.Lock.Release(l.h); rerr != nil {
			serr = errors.Join(serr, fmt.Errorf("%s could not be put back: %w", l.name, rerr))
		}
	}
	got := Cleared{Schema: JournalSchema, Word: model.Done, Removed: n, Scratch: folders, Tools: tools}
	if serr != nil {
		got.Word = model.Failed
		return got, serr
	}
	return got, nil
}
