package decision

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// newCmd is gragas decision new.
func newCmd(s kit.Session) *cobra.Command {
	var slug, title, text string
	var rejected, sources, rules, bindings []string
	c := &cobra.Command{
		Use:   "new",
		Short: "Record a decision",
		Long: `Record a decision: its slug, its title, what was decided and at
least one option that was turned down, with why. It may cite
sources, rules and bindings. At a terminal the missing answers are
asked, each with the flag that gives it in a script. A decision
that is recorded already, with the same words, ends "nothing to do".

  gragas decision new
  gragas decision new --decision-slug wrap --title "Wrap errors" --text "Wrap with context." --rejected "bare: loses where it failed" --apply`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		req := model.NewDecisionRecord{Slug: slug, Title: title, Text: text, Rejected: rejected, Sources: sources, Bindings: bindings}
		for _, r := range rules {
			req.Rules = append(req.Rules, model.RuleID(r))
		}
		return s.Exit(s.DecisionNew(cmd.Context(), cmd, req))
	}
	f := c.Flags()
	f.StringVar(&slug, "decision-slug", "", "The slug of the new decision")
	f.StringVar(&title, "title", "", "What it is called")
	f.StringVar(&text, "text", "", "What was decided, on one line")
	f.StringArrayVar(&rejected, "rejected", nil, "An option turned down, and why (repeat; at least one)")
	f.StringArrayVar(&sources, "cite-source", nil, "A source it cites (repeat)")
	f.StringArrayVar(&rules, "cite-rule", nil, "A rule it cites (repeat)")
	f.StringArrayVar(&bindings, "cite-binding", nil, "A binding it cites, as <rule id> <tool> <key> (repeat)")
	f.Bool("apply", false, "Write the files")
	return c
}
