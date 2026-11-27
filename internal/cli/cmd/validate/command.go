// Package validate is the validate command: it reads the rulebook of the
// working folder and lists what is wrong with it (findings) and what is only
// worth saying (notices). It lives in a package of its own below
// internal/cli/cmd; the root joins it from the generated join_gen.go.
package validate

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 10

// Limit is how many findings a listing shows. A longer one says how many
// there are and how many it shows, and --json carries the same count; a list
// is never cut without saying so (cli.look.3).
const Limit = 50

// Row is the one declaration of validate (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "validate", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsListing, State: kit.Built,
		Summary: "Check a rulebook and list what is wrong in it",
		Examples: [2]kit.Example{
			{Line: "gragas validate", Does: "list the findings and notices"},
			{Line: "gragas validate --json", Does: "as one JSON document"},
		},
		Exits:    []model.Word{model.Done, model.Findings},
		JSONKind: "validate",
		SARIF:    true,
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check a rulebook and list what is wrong in it",
		Long: `Reads the rulebook in this folder and lists each finding
with its file and line, and the notices beside it. A finding
is something the format needs written differently; any one
ends the run on findings, exit 1. A notice is information, a
check that did not run or a translation that went stale,
and never changes the word. It reads and changes nothing.
--sarif writes every finding and notice as one SARIF
document, each under the validate check's own id, which
is no rulebook rule.

  gragas validate
  gragas validate --json
  gragas validate --sarif`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return s.Exit(run(c, s)) },
	}
}

func run(c *cobra.Command, s kit.Session) int {
	v, err := s.Validate(c.Context())
	if err != nil {
		return notReady(s, err)
	}
	next := ""
	if v.Rules == 0 && len(v.Findings) == 0 {
		next = "gragas rule new"
	}
	if s.JSON() {
		s.Document(coreapi.ValidationDoc(v, Limit, next))
		return s.End(kit.Ending{Word: v.Word(), What: said(v)}) // draws nothing under --json
	}
	if s.SARIF() {
		doc, err := coreapi.ValidationSARIF(v, s.Build().Version)
		if err != nil {
			return s.End(kit.Ending{Word: model.Failed, What: "the findings could not be written as SARIF: " + err.Error()})
		}
		s.Encoded(doc)
		return s.End(kit.Ending{Word: v.Word(), What: said(v)}) // on standard error under --sarif
	}
	return s.End(kit.Ending{Word: v.Word(), What: said(v), Next: nextLine(v, next), Lead: lead(v, next)})
}

// notReady ends the run on a rulebook that cannot be read: an open run, a
// format newer than this engine, or no rulebook here.
func notReady(s kit.Session, err error) int {
	switch {
	case coreapi.IsOpenRecord(err):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: coreapi.RecoverLine})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas version"})
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.End(kit.Ending{Word: model.Refused, What: err.Error() + ". validate reads a rulebook", Next: "gragas init"})
	}
	return s.End(kit.Ending{Word: model.Failed, What: "the rulebook could not be read: " + err.Error()})
}

// said is the sentence beside the closing word.
func said(v model.Validation) string {
	if len(v.Findings) == 0 {
		return fmt.Sprintf("%s, 0 findings, %s", model.Plural(v.Rules, "rule"), model.Plural(len(v.Notices), "notice"))
	}
	return fmt.Sprintf("%s, %s", model.Plural(len(v.Findings), "finding"), model.Plural(len(v.Notices), "notice"))
}

// nextLine is the line to type after the word: only a rulebook with no rule
// has one to say.
func nextLine(v model.Validation, next string) string {
	if len(v.Findings) > 0 {
		return ""
	}
	return next
}

// lead is the listing above the closing word: the findings as a table, the
// worst first as the files were read, then the notices apart.
func lead(v model.Validation, next string) []model.Block {
	var out []model.Block
	if len(v.Findings) > 0 {
		t := model.Table{Columns: []model.Column{{Title: "about", Priority: 9}, {Title: "what is wrong, and the fix", Priority: 9}}}
		for _, f := range v.Findings[:min(Limit, len(v.Findings))] {
			t.Rows = append(t.Rows, []string{f.Entity, withFix(f)})
		}
		out = append(out, model.Heading{Text: "Findings"}, t)
		if len(v.Findings) > Limit {
			out = append(out, model.Note{Text: fmt.Sprintf("%d of %d findings shown. Fix these and run it again to see the rest.", Limit, len(v.Findings))})
		}
	}
	if len(v.Notices) > 0 {
		items := make([]string, len(v.Notices))
		for i, n := range v.Notices {
			items[i] = n.Entity + ": " + n.Msg
		}
		out = append(out, model.Heading{Text: "Notices"}, model.List{Items: items})
	}
	out = append(out, model.Note{Text: heldLine(v.Held)})
	if len(v.Findings) == 0 && v.Rules == 0 {
		out = append(out, model.Note{Text: "The rulebook holds no rule yet. Write one with `" + next + "`, or bring some in with `gragas import`."})
	}
	return out
}

// heldLine is the one line on the migrated rules the prose lint holds, said
// when there are none too: each stays held until its statement
// is reworded, so the count only falls.
func heldLine(n int) string {
	return model.Plural(n, "migrated rule") + " held from the prose lint until reworded; the count only falls."
}

// withFix is a finding's sentence and, when one is offered, the fix beside it.
func withFix(f model.Finding) string {
	if f.Fix == "" {
		return f.Msg
	}
	return f.Msg + " Fix: " + f.Fix
}
