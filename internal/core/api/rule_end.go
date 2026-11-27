package coreapi

// rule retire, restore and replace: the three plans that end a rule's life or
// return it. Each is one composition of rule.go's steps; the rules of what
// may be done are the lifecycle context's.

import (
	"context"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// RuleRetire is the change that retires a rule of the rulebook in ask.Root.
func RuleRetire(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.RetireRule) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, req.ID, withFiles(&req, &req.Files, lifecycle.PlanRuleRetire))
}

// RuleRestore is the change that returns a retired rule as active.
func RuleRestore(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.RestoreRule) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, req.ID, withFiles(&req, &req.Files, lifecycle.PlanRuleRestore))
}

// RuleReplace is the change that adds a successor and retires the old rule.
func RuleReplace(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.ReplaceRule) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, req.ID, withFiles(&req, &req.Files, lifecycle.PlanRuleReplace))
}
