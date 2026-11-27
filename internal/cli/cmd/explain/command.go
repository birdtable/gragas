// Package explain is the explain command: in a rulebook it says what holds a
// rule, by the id or an alias it had, or which rules hold a tool entry. It
// reads nothing itself: it asks the session, which asks the core, and draws
// what comes back. The forms for a project, a path and a diagnostic are
// M6-10's.
package explain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 230

// Row is the one declaration of explain (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "explain", Order: Order, Spec: true, Context: kit.InBoth, Ends: kit.EndsListing, State: kit.Built,
		Summary: "Explain a rule or a tool entry by what holds it",
		Examples: [2]kit.Example{
			{Line: "gragas explain <id>", Does: "say what holds a rule"},
			{Line: "gragas explain <tool> <key>", Does: "say which rules hold it"},
		},
		Exits:    []model.Word{model.Done},
		JSONKind: "explain",
	}
}

// long is the help text of the command.
const long = `Says what holds a rule: its standing and marks, each binding with
the tool entry, severity, rung and run point, what the proof says
of it and the preset file it is written to. The rule is asked for
by its id or by an alias it had before it was renumbered. With a
tool and an entry key it says which rules hold that entry, or that
no rule explains it. The page and explorer links are printed where
the rulebook's identity gives a base address, and never guessed. It
reads and changes nothing.

  gragas explain <id>
  gragas explain <tool> <key>`

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use: "explain <id> | <tool> <key>", Short: Row().Summary, Long: long, Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error { return s.Exit(run(c, s, args)) },
	}
}

func run(c *cobra.Command, s kit.Session, args []string) int {
	var e model.Explained
	var err error
	if len(args) == 1 {
		e, err = s.ExplainRule(c.Context(), args[0])
	} else {
		e, err = s.ExplainEntry(c.Context(), args[0], args[1])
	}
	if err != nil {
		return refused(s, err)
	}
	what := said(e)
	if s.JSON() {
		s.Document(coreapi.ExplainDocument(e))
		return s.End(kit.Ending{Word: model.Done, What: what}) // draws nothing under --json
	}
	return s.End(kit.Ending{Word: model.Done, What: what, Lead: lead(e)})
}

// refused ends the run on a rule that is not there, or a folder that cannot be
// read.
func refused(s kit.Session, err error) int {
	var unknown *coreapi.UnknownRule
	var findings *coreapi.FindingsError
	switch {
	case errors.As(err, &unknown):
		return s.End(kit.Ending{Word: model.Usage, What: err.Error(), Next: "gragas coverage"})
	case coreapi.IsOpenRecord(err):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: coreapi.RecoverLine})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas version"})
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error() + ". explain reads a rulebook", Next: "gragas init"})
	case errors.As(err, &findings):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: "gragas validate"})
	}
	return s.End(kit.Ending{Word: model.Failed, What: "the explanation could not be made: " + err.Error()})
}

// said is the sentence beside the closing word.
func said(e model.Explained) string {
	switch {
	case e.Unmapped:
		return fmt.Sprintf("unmapped: no rule explains this entry (%s)", e.Tool)
	case e.Alias != "":
		return fmt.Sprintf("%s is now %s", e.Asked, e.Alias)
	case e.Tool != "":
		return fmt.Sprintf("%s %s is held by %s", e.Tool, e.Entry, model.Plural(len(e.Rules), "rule"))
	}
	r := e.Rules[0]
	return fmt.Sprintf("%s: %s", r.Rule, standing(r))
}

func standing(r model.ExplainedRule) string {
	h := model.Held{Rule: r.Rule, Standing: r.Standing, Marks: r.Marks}
	return h.String()
}

// lead is the explanation above the closing word: for each rule its standing
// and a table of its bindings, then the links.
func lead(e model.Explained) []model.Block {
	var out []model.Block
	if e.Unmapped {
		return []model.Block{model.Note{Text: fmt.Sprintf("no rule of this rulebook binds %s %s", e.Tool, e.Entry)}}
	}
	for _, r := range e.Rules {
		out = append(out, model.Heading{Text: fmt.Sprintf("%s: %s (%s)", r.Rule, standing(r), r.Status)})
		if len(r.Bindings) == 0 {
			out = append(out, model.Note{Text: "bound to no tool entry"})
		} else {
			t := model.Table{Columns: []model.Column{{Title: "entry", Priority: 9}, {Title: "severity", Priority: 7}, {Title: "rung", Priority: 5}, {Title: "proof", Priority: 8}, {Title: "preset", Priority: 6}}}
			for _, b := range r.Bindings {
				t.Rows = append(t.Rows, []string{fmt.Sprintf("%s %s: %s", b.Tool, b.Target, b.Key), string(b.Severity), fmt.Sprint(int(b.Rung)), string(b.Proof), orNone(b.Preset)})
			}
			out = append(out, t)
		}
		var links []string
		if r.Page != "" {
			links = append(links, "page "+r.Page)
		}
		if r.Explorer != "" {
			links = append(links, "explorer "+r.Explorer)
		}
		if len(links) > 0 {
			out = append(out, model.Note{Text: strings.Join(links, ", ")})
		}
	}
	return out
}

func orNone(s string) string {
	if s == "" {
		return "not shipped"
	}
	return s
}
