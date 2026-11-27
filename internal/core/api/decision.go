package coreapi

import (
	"context"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// decisionPlan is the id of a plan that writes decisions.yaml and the rules
// that stand on a decision. No rule has an id of one word, so it never names
// a rule.
const decisionPlan = "decision"

// DecisionNew is the change that records a decision in the rulebook in ask.Root.
func DecisionNew(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.NewDecisionRecord) (*Change, *model.Refusal, error) {
	return decided(ctx, p, ask, &req, &req.Files, lifecycle.PlanDecisionNew)
}

// DecisionSupersede is the change that replaces a decision with a new record,
// marks the old one superseded and moves the rules that stood on it.
func DecisionSupersede(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.SupersedeDecision) (*Change, *model.Refusal, error) {
	return decided(ctx, p, ask, &req, &req.Files, lifecycle.PlanDecisionSupersede)
}

// decided composes a decision plan over the request it was given.
func decided[R any](ctx context.Context, p port.Ports, ask model.ChangeAsk, req *R, files *model.PlanFiles, plan func(model.Book, R, lifecycle.Moment) (lifecycle.Plan, error)) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, decisionPlan, withFiles(req, files, plan))
}
