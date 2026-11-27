package decision

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// supersedeCmd is gragas decision supersede.
func supersedeCmd(s kit.Session) *cobra.Command {
	var slug, title, text string
	var rejected []string
	c := &cobra.Command{
		Use:   "supersede <slug>",
		Short: "Supersede a decision by a new one",
		Long: `Record a new decision that replaces <slug>. The old record stays,
marked superseded by the new one, and the new one names it back.
Every rule that stood on the old decision stands on the new one, and
the plan lists those rules. Only the head of a chain can be
superseded: a decision that is superseded already is refused, with
the head named. At a terminal the missing answers are asked.

  gragas decision supersede wrap
  gragas decision supersede wrap --decision-slug wrap-with-context --title "Wrap with context" --text "Say what was being done." --rejected "bare cause: loses where it failed" --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		req := model.SupersedeDecision{Old: args[0], Slug: slug, Title: title, Text: text, Rejected: rejected}
		return s.Exit(s.DecisionSupersede(cmd.Context(), cmd, req))
	}
	f := c.Flags()
	f.StringVar(&slug, "decision-slug", "", "The slug of the new decision")
	f.StringVar(&title, "title", "", "What it is called")
	f.StringVar(&text, "text", "", "What was decided, on one line")
	f.StringArrayVar(&rejected, "rejected", nil, "An option turned down, and why (repeat; at least one)")
	f.Bool("apply", false, "Write the files")
	return c
}
