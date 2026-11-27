// Package decision is the decision command: it records a decision and
// supersedes one, each word a plan shown first and written only with --apply.
// A decision is never edited and never deleted (S59), so the family has two
// words. It lives in a package of its own below internal/cli/cmd; the root
// joins it from the generated join_gen.go.
package decision

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 60

// Row is the one declaration of decision (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "decision", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Then:    []string{"new", "supersede"},
		Summary: "Record a decision or supersede one",
		Examples: [2]kit.Example{
			{Line: "gragas decision new", Does: "record a decision"},
			{Line: "gragas decision supersede <slug>", Does: "supersede a decision by a new one"},
		},
		Writes:   []string{"decisions.yaml", "the rules files of the rules that move (supersede)", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined, model.Refused},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "decision",
		Short: "Record a decision or supersede one, with a history entry for each change",
		Long: `A decision is a choice the rulebook made, with the options it
turned down. It is never edited and never deleted: a change of
meaning is a new decision that supersedes the old one. Each word
shows a plan and writes only with --apply: decisions.yaml, the rules
that move and the history.

  gragas decision new
  gragas decision supersede <slug> --apply`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	c.AddCommand(newCmd(s), supersedeCmd(s))
	return c
}
