package root

import (
	"context"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

func (s *session) ruleConfirm(ctx context.Context) *cobra.Command {
	c := &cobra.Command{
		Use:   "confirm <id>",
		Short: "Confirm an active rule and move its review date on",
		Long: `Confirm that an active rule still holds. Its review-by date
moves to today plus the rulebook's review cadence, and the
history records who confirmed it. Only an active rule is
confirmed. A rule confirmed to that day already is left as it is.

  gragas rule confirm <id>
  gragas rule confirm <id> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		req := model.ConfirmRule{ID: args[0]}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleConfirm(ctx, w, ask, req)
		}))
	}
	c.Flags().BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}
