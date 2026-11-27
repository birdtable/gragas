package coreapi

// rule renumber: the plan that gives a rule a new id and keeps the old one as
// an alias. One composition of rule.go's steps; the rules of what may be done
// are the lifecycle context's, and the file a clashed rule is written in is the
// load's.

import (
	"context"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// RuleRenumber is the change that renumbers a rule of the rulebook in
// ask.Root. Where two rules took the id, it is the later one that is renumbered
// and the file written is the later one's.
func RuleRenumber(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.RenumberRule) (*Change, *model.Refusal, error) {
	return composeAt(ctx, p, ask, req.ID, withFiles(&req, &req.Files, lifecycle.PlanRuleRenumber), true)
}
