package coreapi

// source new and edit: the two plans of the source family. Each is one
// composition of rule.go's steps, with no rule's file: a source plan writes
// sources.yaml and the history. An edit also notes itself in the history of
// each rule that cites the source, which is more lines of the one history
// file and no other file. What a source plan may do is the lifecycle context's;
// this file decides nothing.

import (
	"context"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// sourcePlan is the id of a plan that writes sources.yaml and no rule's file.
// No rule has an id of one word, so it never names a rule.
const sourcePlan = "source"

// SourceNew is the change that records a source in the rulebook in ask.Root.
func SourceNew(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.NewSource) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, sourcePlan, withFiles(&req, &req.Files, lifecycle.PlanSourceNew))
}

// SourceEdit is the change that edits a source and notes the edit on each rule
// that cites it.
func SourceEdit(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.EditSource) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, sourcePlan, withFiles(&req, &req.Files, lifecycle.PlanSourceEdit))
}
