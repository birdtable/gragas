// Package undo is the undo command: it takes back the last change the history
// records, by a new entry, shown as a plan first and written only with
// --apply. It is one word with no verb under it. It lives in a package of its
// own below internal/cli/cmd; the root joins it from the generated join_gen.go.
package undo

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 70

// Row is the one declaration of undo (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "undo", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Summary: "Take back the last change the history records",
		Examples: [2]kit.Example{
			{Line: "gragas undo", Does: "show the plan"},
			{Line: "gragas undo --entry 12 --apply", Does: "carry it out"},
		},
		Writes:   []string{"the file of the entity the entry changed", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined, model.Refused, model.NotReady},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "undo",
		Short: "Take back the last change the history records, with a new entry",
		Long: `Undo reads the last entry of the history that no undo has taken
back, and shows the plan that returns the lifecycle fields it moved
to the values it found. It writes only with --apply: the field in
its file, and a new entry (undo) that names the one it takes back.
Nothing is removed from the history. A second undo takes back the
entry before, one step at a time.

It is refused when a field has changed since, naming the later
entry, and for an entry that has no inverse in the lifecycle fields
(a rule or a term made is retired; a source, a decision, an
exclusion or a renumbering is corrected by the command that owns
it). With no history it ends "not ready". Undo is not the journal's
restore of a run's files; see gragas journal.

  gragas undo
  gragas undo --entry 12 --apply

The dry run ends on the apply line with the digest of the entry it
showed (a number you type is taken only when one entry has it;
start a digest prefix with sha256:). --apply without --entry is refused, and so is an --entry that
is not the one undo would take back now: an entry appended since the
dry run is never taken back unseen.`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{kit.Writes: "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, _ []string) error { return s.Exit(s.Undo(cmd.Context(), cmd)) }
	c.Flags().Bool("apply", false, "Write the files")
	c.Flags().String("entry", "", "The entry to take back, by the number or the digest the dry run shows; --apply needs it")
	return c
}
