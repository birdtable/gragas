package coreapi

// term new, edit and retire: the three plans of the term family. Each is one
// composition of rule.go's steps, with no rule's file: a term plan writes
// terms.yaml and the history. What a term plan may do is the lifecycle
// context's; this file decides nothing.

import (
	"context"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TermNew is the change that defines a term in the rulebook in ask.Root.
func TermNew(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.NewTerm) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, "", withFiles(&req, &req.Files, lifecycle.PlanTermNew))
}

// TermEdit is the change that edits a term's literal.
func TermEdit(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.EditTerm) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, "", withFiles(&req, &req.Files, lifecycle.PlanTermEdit))
}

// TermRetire is the change that retires a term no rule uses.
func TermRetire(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.RetireTerm) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, "", withFiles(&req, &req.Files, lifecycle.PlanTermRetire))
}
