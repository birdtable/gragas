// Package source is the source command: it records the sources a rule cites
// and changes them, each word a plan shown first and written only with --apply.
// A source is recorded and edited, never replaced or retired (S15), so the
// family has two words. It lives in a package of its own below
// internal/cli/cmd; the root joins it from the generated join_gen.go.
package source

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 50

// Row is the one declaration of source (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "source", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Then:    []string{"new", "edit"},
		Summary: "Record or change a source a rule cites",
		Examples: [2]kit.Example{
			{Line: "gragas source new", Does: "record a source"},
			{Line: "gragas source edit <slug>", Does: "change a source"},
		},
		Writes:   []string{"sources.yaml", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined, model.Refused},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "source",
		Short: "Record or change a source, with its history",
		Long: `A source is a thing a rule cites: a standard, a guide, a study,
an incident, a decision record, a vendor document or an own
decision. Each word shows a plan and writes only with --apply:
sources.yaml and the history. An edit is noted in the history of
every rule that cites the source, in the same plan.

  gragas source new
  gragas source edit <slug> --locator "https://example.com/new" --apply`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	c.AddCommand(newCmd(s), editCmd(s))
	return c
}
