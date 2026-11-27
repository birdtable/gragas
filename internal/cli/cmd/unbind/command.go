// Package unbind is the unbind command: it takes a binding out of a rule. It
// shows a plan first and writes only with --apply.
package unbind

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 170

// Row is the one declaration of unbind (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "unbind", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Summary: "Remove a rule's binding",
		Examples: [2]kit.Example{
			{Line: "gragas unbind <id> --key <entry>", Does: "show the plan"},
			{Line: "gragas unbind <id> --apply", Does: "carry it out"},
		},
		Writes:   []string{"the concern's rules file", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined, model.Refused, model.NotReady},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	var req model.UnbindRule
	var target string
	c := &cobra.Command{
		Use:   "unbind <id>",
		Short: "Remove a rule's binding",
		Long: `Take the binding of rule <id> found by --tool, --target and --key out
of its rules file. The plan shows the tool entry the presets lose and
the rule's standing before and after. A binding that is not there
ends "nothing to do". A binding a parent or an import lends is
refused, as it would weaken the rule; a record of the rulebook's own
in bindings/ is removed with git rm. Writes only with --apply.

  gragas unbind <id> --tool lint-a --target config --key wrap-errors
  gragas unbind <id> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		req.ID, req.Target = args[0], model.TargetType(target)
		return s.Exit(s.Unbind(cmd.Context(), cmd, req))
	}
	f := c.Flags()
	f.StringVar(&req.Tool, "tool", "", "The tool's slug, as gragas tools shows")
	f.StringVar(&target, "target", "", "What the entry is about, such as config")
	f.StringVar(&req.Key, "key", "", "The tool's entry")
	f.Bool("apply", false, "Write the files")
	return c
}
