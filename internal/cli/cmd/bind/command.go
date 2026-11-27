// Package bind is the bind command: it adds a binding to a rule, the entry of
// a tool's setting that holds the rule (S34). It shows a plan first, with the
// tool entry, the standing before and after and the proof to run, and writes
// only with --apply. It lives in a package of its own below internal/cli/cmd;
// the root joins it from the generated join_gen.go.
package bind

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 150

// Row is the one declaration of bind (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "bind", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Summary: "Bind a rule to the tool setting that holds it",
		Examples: [2]kit.Example{
			{Line: "gragas bind <id> --key <entry>", Does: "show the plan"},
			{Line: "gragas bind <id> --apply", Does: "carry it out"},
		},
		Writes:   []string{"the concern's rules file, or a record in bindings/ for a borrowed rule", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined, model.Refused, model.NotReady},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	var b model.Binding
	var target, severity, runPoint, value string
	var options []string
	var rung int
	c := &cobra.Command{
		Use:   "bind <id>",
		Short: "Bind a rule to the tool setting that holds it",
		Long: `Add a binding to rule <id>: the tool, the target type, the tool's
entry, how loudly it reports, the rung it holds the rule at and where
in the work it runs. The plan shows the tool entry the presets gain,
the rule's standing before and after, and the proof to run. Binding
the same entry again ends "nothing to do"; the same entry with other
fields is refused, naming gragas rebind. A rule the rulebook borrows,
a parent's or an import's, is never written: a binding that tightens
it is a record in bindings/, and one that weakens it is refused,
naming the field. Writes only with --apply.

  gragas bind <id> --tool lint-a --target config --key wrap-errors --severity error --rung 1 --run-point commit
  gragas bind <id> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		b.Target, b.Severity, b.RunPoint = model.TargetType(target), model.Severity(severity), model.RunPoint(runPoint)
		b.Rung = model.Rung(rung)
		if value != "" {
			b.Value = kit.Scalar(value)
		}
		opts, err := kit.Options(options)
		if err != nil {
			return err
		}
		b.Options = opts
		return s.Exit(s.Bind(cmd.Context(), cmd, model.BindRule{ID: args[0], Binding: b}))
	}
	f := c.Flags()
	f.StringVar(&b.Tool, "tool", "", "The tool's slug, as gragas tools shows")
	f.StringVar(&target, "target", "", "What the entry is about, such as config")
	f.StringVar(&b.Key, "key", "", "The tool's entry")
	f.StringVar(&value, "value", "", "The entry's value: true, false, a number or a word")
	f.StringArrayVar(&options, "option", nil, "An option of the entry, name=value; a name given twice holds a list")
	f.StringVar(&severity, "severity", "", "How loudly the tool reports it: error, warning or info")
	f.IntVar(&rung, "rung", 0, "How the binding holds the rule, 0 to 6")
	f.StringVar(&runPoint, "run-point", "", "Where in the work it runs: edit, commit, push, land, review, release or deploy")
	f.StringVar(&b.Remedy, "remedy", "", "The tool's fixer, shown and never run")
	f.IntVar(&b.Pair, "pair", 0, "The number of the pair that proves it")
	f.Bool("apply", false, "Write the files")
	return c
}
