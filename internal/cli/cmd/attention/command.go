// Package attention is the attention command: it lists what needs the
// maintainer of a rulebook, grouped by what is missing, read from the book and
// the day the clock gives. It lives in a package of its own below
// internal/cli/cmd; the root joins it from the generated join_gen.go. It reads
// nothing itself: it asks the session, which asks the core, and draws what
// comes back.
package attention

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 130

// OverrideLine is the line the listing says about the override rate until
// batches are given (the batches are M9's).
const OverrideLine = "override rate: no batches given"

// Row is the one declaration of attention (cli.commands.20, .26).
func Row() kit.Command {
	c := kit.Listing("attention", Order, "List what needs the owner's attention first", [2]kit.Example{
		{Line: "gragas attention", Does: "list it"},
		{Line: "gragas attention --json", Does: "as one JSON document"},
	})
	c.Context = kit.InRulebook
	return c
}

// long is the help text of the command.
const long = `Lists the rules that need you, grouped by what is missing: a
review-by that is past, no source, no example pair, no decision
where the governance asks for one, no owner. The days are judged
against today's date, read when the command runs; nothing is
kept between runs. A retired rule asks for nothing. When every
rule is proposed it lists the whole book, and says so first.
It reads and changes nothing.

  gragas attention
  gragas attention --json`

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "attention",
		Short: Row().Summary,
		Long:  long,
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			code := run(c, s)
			return s.Exit(code)
		},
	}
}

func run(c *cobra.Command, s kit.Session) int {
	l, err := s.Attention(c.Context())
	switch {
	case errors.Is(err, coreapi.ErrNoBook):
		return s.End(kit.Ending{Word: model.NotReady, What: "this folder holds no rulebook, so there is nothing to attend to. Run it in a rulebook's repository", Next: "gragas help attention"})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas version"})
	case err != nil:
		return s.End(kit.Ending{Word: model.Failed, What: "the attention list could not be made: " + err.Error()})
	}
	return kit.EndListing(s, coreapi.AttentionDoc(l), said(l), lead(l))
}

// said is the sentence beside the closing word.
func said(l model.AttentionListing) string {
	rules := map[model.RuleID]bool{}
	for _, i := range l.Items {
		if i.Kind != model.AttentionComplete {
			rules[i.Rule] = true
		}
	}
	return fmt.Sprintf("%s need attention, %s", model.Plural(len(rules), "rule"), model.Plural(len(l.Items), "item"))
}

// lead is the listing above the closing word: the one line a book of proposed
// rules opens with, a table a kind, the findings that kept rules from being
// read and the override line last.
func lead(l model.AttentionListing) []model.Block {
	var out []model.Block
	if l.AllProposed {
		out = append(out, model.Note{Text: "every rule is proposed, so the whole book is listed, grouped by what is missing"})
	}
	for _, k := range model.AttentionKinds() {
		t := model.Table{Columns: []model.Column{{Title: "rule", Priority: 9}, {Title: "what is wrong", Priority: 7}, {Title: "fix", Priority: 8}}}
		for _, i := range l.Items {
			if i.Kind == k {
				t.Rows = append(t.Rows, []string{string(i.Rule), i.Why, i.Fix})
			}
		}
		if len(t.Rows) > 0 {
			out = append(out, model.Heading{Text: fmt.Sprintf("%s (%d)", k, len(t.Rows))}, t)
		}
	}
	if len(l.Items) == 0 {
		out = append(out, model.Note{Text: "nothing needs attention"})
	}
	for _, f := range l.Findings {
		out = append(out, model.Note{Text: fmt.Sprintf("%s: %s", f.Entity, f.Msg)})
	}
	return append(out, model.Note{Text: OverrideLine})
}
