package root

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The decision family's two plans reach the core through the one form every
// rule change ends in (ruleChange). The words live in internal/cli/cmd/decision.

// DecisionNew asks for what the request leaves out, then plans the record of
// the decision. A rejected option is a list, so it is asked once: the first.
func (k kitSession) DecisionNew(ctx context.Context, cmd kit.Cmd, req model.NewDecisionRecord) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the decision command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	slug, rejected := req.Slug, ""
	qs := []prompted{
		{flag: "--decision-slug", ask: "What is the decision, as lower-case words joined by hyphens?", to: &slug},
		{flag: "--title", ask: "What is it called?", to: &req.Title},
		{flag: "--text", ask: "What was decided, on one line?", to: &req.Text},
	}
	if len(req.Rejected) == 0 {
		qs = append(qs, prompted{flag: "--rejected", ask: "Which option was turned down, and why?", to: &rejected})
	}
	if code, ok := s.asked(ctx, c, qs); !ok {
		return code
	}
	req.Slug = slug
	if rejected != "" {
		req.Rejected = []string{rejected}
	}
	return s.ruleChange(ctx, c, nil, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.DecisionNew(ctx, w, ask, req)
	})
}

// DecisionSupersede asks for the new decision's words, then plans the
// supersession of the decision the request names.
func (k kitSession) DecisionSupersede(ctx context.Context, cmd kit.Cmd, req model.SupersedeDecision) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the decision command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	slug, rejected := req.Slug, ""
	qs := []prompted{
		{flag: "--decision-slug", ask: "What is the new decision, as lower-case words joined by hyphens?", to: &slug},
		{flag: "--title", ask: "What is it called?", to: &req.Title},
		{flag: "--text", ask: "What was decided, on one line?", to: &req.Text},
	}
	if len(req.Rejected) == 0 {
		qs = append(qs, prompted{flag: "--rejected", ask: "Which option was turned down, and why?", to: &rejected})
	}
	if code, ok := s.asked(ctx, c, qs); !ok {
		return code
	}
	req.Slug = slug
	if rejected != "" {
		req.Rejected = []string{rejected}
	}
	return s.ruleChange(ctx, c, []string{req.Old}, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.DecisionSupersede(ctx, w, ask, req)
	})
}
