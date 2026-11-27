package root

import (
	"context"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ruleDecided is the rule commands that take an id and a decision: renumber and
// promote. Each asks for the decision, makes its request from the id and the
// why, and writes with --apply.
func (s *session) ruleDecided(ctx context.Context) []*cobra.Command {
	type call = func(port.Ports, model.ChangeAsk, string, model.Why) (*coreapi.Change, *model.Refusal, error)
	forms := []struct {
		use, short, long string
		run              call
	}{
		{"renumber <id>", "Give a rule a new id, the old one kept as its alias", `Give a rule the next number its concern has not held, on a
decision, and keep the old id as an alias for good, so every
link that names it still finds the rule. Where two rules took
one id, the later one is renumbered. A new id is never one that
was held before.

  gragas rule renumber <id> --decision-title "..." --decision-text "..."
  gragas rule renumber <id> --decision <slug> --apply`,
			func(w port.Ports, ask model.ChangeAsk, id string, why model.Why) (*coreapi.Change, *model.Refusal, error) {
				return coreapi.RuleRenumber(ctx, w, ask, model.RenumberRule{ID: id, Why: why})
			}},
		{"promote <id>", "Move a rule one step up: proposed, trial, active", `Move a rule one step up its life, on a decision. A must or a
should goes from proposed to trial, and from trial to active once
a release has gone by; a may goes straight to active. A rule with
no source, no example pair or no decision is refused, naming
what is missing. A rule that is active already is left as it is.

  gragas rule promote <id> --decision <slug>
  gragas rule promote <id> --decision-title "..." \
    --decision-text "..." --apply`,
			func(w port.Ports, ask model.ChangeAsk, id string, why model.Why) (*coreapi.Change, *model.Refusal, error) {
				return coreapi.RulePromote(ctx, w, ask, model.PromoteRule{ID: id, Why: why})
			}},
	}
	var out []*cobra.Command
	for _, f := range forms {
		out = append(out, s.ruleOnDecision(ctx, &cobra.Command{Use: f.use, Short: f.short, Long: f.long}, f.run))
	}
	return out
}
