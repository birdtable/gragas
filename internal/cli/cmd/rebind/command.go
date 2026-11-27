// Package rebind is the rebind command: it changes a binding of a rule, moving
// it to another entry or changing its value, severity or remedy. A severity
// that loosens needs a decision (S70). It shows a plan first and writes only
// with --apply.
package rebind

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 160

// Row is the one declaration of rebind (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "rebind", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Summary: "Move a rule's binding to another tool setting",
		Examples: [2]kit.Example{
			{Line: "gragas rebind <id> --to-key <new>", Does: "show the plan"},
			{Line: "gragas rebind <id> --apply", Does: "carry it out"},
		},
		Writes:   []string{"the concern's rules file, or a record in bindings/ for a borrowed rule", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined, model.Refused, model.NotReady},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	var req model.RebindRule
	var target, toTarget, toTool, toKey, severity, remedy, value, decision string
	var options []string
	c := &cobra.Command{
		Use:   "rebind <id>",
		Short: "Move a rule's binding to another tool setting",
		Long: `Change the binding of rule <id> found by --tool, --target and --key:
move it with --to-tool, --to-target or --to-key, or change its
--value, --severity or --remedy. A move to another entry, a severity
that reports less loudly, and a value that reports less or that the
tool's scale cannot rank are refused unless --decision names the
decision that says why. The
plan shows the tool entries that move and the standing before and
after. A rule the rulebook borrows, a parent's or an import's, takes
only a change that tightens it, as a record in bindings/; a move or
any change that weakens it is refused, naming the field. Writes only
with --apply.

  gragas rebind <id> --tool lint-a --target config --key wrap-errors --to-key wrap
  gragas rebind <id> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		req.ID, req.Target = args[0], model.TargetType(target)
		ch := cmd.Flags().Changed
		if ch("to-tool") {
			req.ToTool = &toTool
		}
		if ch("to-target") {
			t := model.TargetType(toTarget)
			req.ToTarget = &t
		}
		if ch("to-key") {
			req.ToKey = &toKey
		}
		if ch("value") {
			v := kit.Scalar(value)
			req.Value = &v
		}
		if ch("option") {
			opts, err := kit.Options(options)
			if err != nil {
				return err
			}
			req.Options = opts
		}
		if ch("severity") {
			sv := model.Severity(severity)
			req.Severity = &sv
		}
		if ch("remedy") {
			req.Remedy = &remedy
		}
		if ch("decision") {
			req.Why = &model.Why{Cite: decision}
		}
		return s.Exit(s.Rebind(cmd.Context(), cmd, req))
	}
	f := c.Flags()
	f.StringVar(&req.Tool, "tool", "", "The tool's slug, as gragas tools shows")
	f.StringVar(&target, "target", "", "What the entry is about, such as config")
	f.StringVar(&req.Key, "key", "", "The tool's entry")
	f.StringVar(&toTool, "to-tool", "", "Move it to this tool")
	f.StringVar(&toTarget, "to-target", "", "Move it to this target type")
	f.StringVar(&toKey, "to-key", "", "Move it to this entry")
	f.StringVar(&value, "value", "", "The entry's value: true, false, a number or a word")
	f.StringArrayVar(&options, "option", nil, "An option of the entry, name=value; a name given twice holds a list")
	f.StringVar(&severity, "severity", "", "How loudly the tool reports it: error, warning or info")
	f.StringVar(&remedy, "remedy", "", "The tool's fixer, shown and never run")
	f.StringVar(&decision, "decision", "", "The slug of a decision the book holds")
	f.Bool("apply", false, "Write the files")
	return c
}
