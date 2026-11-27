// Package history is the history command: it lists the entries of a rulebook's
// history.jsonl, all of them or a rule's. It lives in a package of its own
// below internal/cli/cmd; the root joins it from the generated join_gen.go. It
// reads nothing itself: it asks the session, which asks the core, and draws
// what comes back.
package history

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 140

// Row is the one declaration of history (cli.commands.20, .26).
func Row() kit.Command {
	c := kit.Listing("history", Order, "List the history of a rule or of the rulebook", [2]kit.Example{
		{Line: "gragas history", Does: "list the entries"},
		{Line: "gragas history <id>", Does: "list the entries for one rule"},
	})
	c.Context = kit.InRulebook
	return c
}

// long is the help text of the command.
const long = `Every change to a rule's life, one row each, in the order
the history keeps: its number, what was done, to which rule,
by whom, when, the plan that was accepted and the decision
taken. Give a rule's id, or an id it had before it was
renumbered, to list only that rule's rows.

A line that cannot be read is skipped and counted, and a
link that does not hold is listed under the rows. It reads
and changes nothing.

  gragas history
  gragas history <id>
  gragas history --at v1.2.0
  gragas history --json`

// New is the cobra command, drawn through the session of the run. It always
// ends done: what is wrong with the chain is listed beside the entries, and
// the rulebook's gate is what refuses it (cli.commands.9).
func New(s kit.Session) *cobra.Command {
	var entity, at string
	c := &cobra.Command{
		Use:   "history [id]",
		Short: Row().Summary,
		Long:  long,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ask := model.HistoryAsk{Rev: at, Entity: entity}
			if len(args) == 1 {
				ask.Rule = args[0]
			}
			return s.Exit(run(c, s, ask))
		},
	}
	c.Flags().StringVar(&entity, "entity", "", "List only the rows about this entity, as the history names it")
	c.Flags().StringVar(&at, "at", "", "Read the history as it is at this tag, commit or branch")
	return c
}

func run(c *cobra.Command, s kit.Session, ask model.HistoryAsk) int {
	got, err := s.History(c.Context(), ask)
	said, isSaid := errors.AsType[kit.Said](err)
	switch {
	case isSaid:
		return s.End(kit.Ending{Word: model.Failed, What: said.What})
	case errors.Is(err, coreapi.ErrNoBook):
		return s.End(kit.Ending{Word: model.NotReady, What: "this folder holds no rulebook, so it has no history. Run it in a rulebook's repository", Next: "gragas help history"})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error()})
	case err != nil:
		return s.End(kit.Ending{Word: model.Failed, What: "the history could not be read: " + err.Error()})
	}
	if s.JSON() {
		s.Document(got)
		return exit.Done
	}
	lead := []model.Block{model.Heading{Text: "gragas history"}}
	if len(got.Entries) > 0 {
		table := model.Table{Columns: []model.Column{{Title: "No", Priority: 9}, {Title: "Entity", Priority: 8}, {Title: "Kind", Priority: 7},
			{Title: "When", Priority: 5}, {Title: "Who", Priority: 4}, {Title: "Decision", Priority: 3}, {Title: "Plan", Priority: 2}}}
		for _, e := range got.Entries {
			decision := "-"
			if e.Decision != nil {
				decision = *e.Decision
			}
			table.Rows = append(table.Rows, []string{strconv.Itoa(e.Number), e.Entity, e.Kind, e.When, e.Who, decision, shortDigest(e.Plan)})
		}
		lead = append(lead, table)
	}
	for _, n := range got.Notes {
		lead = append(lead, model.Note{Text: n})
	}
	for _, f := range got.Findings {
		lead = append(lead, model.Note{Text: fmt.Sprintf("%s: line %d: %s", f.Kind, f.Line, f.Msg)})
	}
	for _, k := range got.Skipped {
		lead = append(lead, model.Note{Text: fmt.Sprintf("skipped line %d: %s", k.Line, k.Why)})
	}
	return s.End(kit.Ending{Word: model.Done, What: summary(got), Lead: lead})
}

// shortDigest is a digest as far as a person tells two apart: the algorithm
// and twelve digits. --json carries the whole digest.
func shortDigest(d string) string {
	algo, hex, ok := strings.Cut(d, ":")
	if !ok || len(hex) <= 12 {
		return d
	}
	return algo + ":" + hex[:12]
}

func summary(l model.HistoryListing) string {
	s := plural(len(l.Entries), "entry", "entries") + " listed"
	if len(l.Entries) != l.Total {
		s += " of " + strconv.Itoa(l.Total)
	}
	if n := len(l.Skipped); n > 0 {
		s += ", " + plural(n, "line", "lines") + " skipped"
	}
	if n := len(l.Findings); n > 0 {
		s += ", " + plural(n, "problem", "problems") + " in the chain"
	}
	return s
}

// plural is n in digits and the noun that goes with it.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
