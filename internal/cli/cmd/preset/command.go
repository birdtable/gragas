// Package preset is the preset command: it writes every tool's preset from the
// rulebook's bindings. It shows a plan first and writes only with --apply;
// --check writes nothing and is red when a file differs.
package preset

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of the commands.
const Order = 235

// Row is the one declaration of preset (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "preset", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Summary: "Write every tool's preset from the rulebook's bindings",
		Examples: [2]kit.Example{
			{Line: "gragas preset", Does: "show the preset files that are new or differ"},
			{Line: "gragas preset --check", Does: "name each file that differs, writing nothing"},
		},
		Writes:   []string{"presets/<tool>/ files"},
		Exits:    []model.Word{model.Done, model.Findings, model.Declined, model.Refused, model.NotReady},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	var check bool
	c := &cobra.Command{
		Use:   "preset",
		Short: "Write every tool's preset from the rulebook's bindings",
		Long: `Write the preset of each tool of the rulebook from its bindings: the
dry run lists each file that is new or differs and ends on the apply
line. The rulebook's own preset is written again on every run; a file a
project copied once is written when it is not there and never
overwritten. A second run says there is nothing to write.

--check writes nothing: it names each file that differs, prints how many
it compared, and exits 1 when one differs.

  gragas preset
  gragas preset --apply
  gragas preset --check`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.Exit(s.Preset(cmd.Context(), cmd, check))
	}
	f := c.Flags()
	f.BoolVar(&check, "check", false, "Fail when a file would change")
	f.Bool("apply", false, "Write the files")
	return c
}
