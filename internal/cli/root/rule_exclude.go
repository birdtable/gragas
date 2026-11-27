package root

import (
	"context"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

const excludeLong = `Drop a rule of the parent this overlay extends, on a
decision. The exclusion is a record in exclusions.yaml with a
sequence number, and the parent is never edited. This is the
only way an overlay weakens a parent's rule. A rule that is
excluded already is left as it is. The parent's rules are read
from its release in the cache, at the pin.

  gragas rule exclude <id> --decision-title "..." --decision-text "..."
  gragas rule exclude <id> --decision <slug> --apply`

const includeLong = `Lift this overlay's exclusion of a parent's rule, on a
decision. The record stays in exclusions.yaml, marked lifted.
A rule that is not excluded is refused, and so is one the
parent has retired. The plan names the overlay's own rules
that cover it, since both then apply.

  gragas rule include <id> --decision-title "..." --decision-text "..."
  gragas rule include <id> --decision <slug> --apply`

// ruleExclusions are rule exclude and rule include: one parent's rule id and
// a decision, the parent read over the releases cache, written only with
// --apply.
func (s *session) ruleExclusions(ctx context.Context) []*cobra.Command {
	forms := []struct {
		use, short, long string
		include          bool
	}{
		{"exclude <id>", "Drop a parent's rule from this overlay, by decision", excludeLong, false},
		{"include <id>", "Bring back an excluded parent's rule, by decision", includeLong, true},
	}
	var out []*cobra.Command
	for _, f := range forms {
		out = append(out, s.ruleOnDecision(ctx, &cobra.Command{Use: f.use, Short: f.short, Long: f.long},
			func(w port.Ports, ask model.ChangeAsk, id string, why model.Why) (*coreapi.Change, *model.Refusal, error) {
				ask.Cache = s.p.Config.CacheDir
				over := coreapi.OverlayAsk{ChangeAsk: ask, Contracts: s.p.Contract}
				if f.include {
					return coreapi.RuleInclude(ctx, w, over, model.IncludeRule{ID: id, Why: why})
				}
				return coreapi.RuleExclude(ctx, w, over, model.ExcludeRule{ID: id, Why: why})
			}))
	}
	return out
}
