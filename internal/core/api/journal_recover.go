package coreapi

// journal recover and cache clear: the two calls that write, each through the
// write path's own functions. Recover is apply.Run with a plan that changes
// nothing, so the roll-back is the one apply makes before every plan, under
// the root lock, and it rolls back and never resumes (cli.commands.7).

import (
	"context"
	"errors"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The types below live in the model, so the kit can name them.
type (
	// RecoverAsk is what a recovery is asked for, as the model holds it.
	RecoverAsk = model.RecoverAsk
	// Recovered is one open run a recovery settled, as the model holds it.
	Recovered = model.Recovered
	// Recovery is how a recover ended, as the model holds it.
	Recovery = model.Recovery
)

// JournalRecover rolls back every open record of root, under the root lock.
// An open record that cannot be rolled back ends failed and stays open.
func JournalRecover(ctx context.Context, ports port.Ports, root model.Root, ask RecoverAsk) (Recovery, error) {
	out := apply.Run(ctx, ports, root, apply.Spec{
		Command: "gragas journal recover --apply", Engine: ask.Engine, Run: ask.Run,
		Plan:   func(context.Context) (model.Plan, error) { return model.Plan{}, nil },
		Render: func(model.Delta) ([]model.Target, error) { return nil, nil },
	})
	got := Recovery{Schema: JournalSchema, Word: out.Word, Run: out.Run, Recovered: []Recovered{}}
	for _, r := range out.Recovered {
		got.Recovered = append(got.Recovered, Recovered{Run: r.Run, Command: r.Command})
	}
	if out.Refusal != nil {
		got.Observed, got.Hint = out.Refusal.Observed(), out.Refusal.Hint()
	}
	if out.Failure != nil {
		got.Why = string(out.Failure.Kind())
		if out.Cause != nil {
			got.Why += ": " + out.Cause.Error()
		}
	}
	if got.Word != model.Done && got.Word != model.Busy && got.Word != model.Failed && got.Word != model.Refused {
		return Recovery{}, errors.New("the write path ended on " + string(got.Word))
	}
	return got, nil
}
