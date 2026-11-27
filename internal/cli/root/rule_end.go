package root

import (
	"context"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// decision is the four flags that name the decision a change stands on: one
// already in the book, or a new one taken with the change.
type decision struct{ cite, slug, title, text string }

// flags adds the four flags to c; they say the same on every command.
func (d *decision) flags(c *cobra.Command) {
	f := c.Flags()
	f.StringVar(&d.cite, "decision", "", "The slug of a decision the book holds")
	f.StringVar(&d.slug, "decision-slug", "", "The slug of the new decision")
	f.StringVar(&d.title, "decision-title", "", "The title of the decision")
	f.StringVar(&d.text, "decision-text", "", "What was decided and why")
}

// questions are asked when no decision is cited.
func (d *decision) questions() []prompted {
	none := func() bool { return d.cite != "" }
	return []prompted{
		{flag: "--decision-title", ask: "What is the decision called?", to: &d.title, skip: none},
		{flag: "--decision-text", ask: "What did you decide, and why?", to: &d.text, skip: none},
	}
}

// why is the decision as the request carries it.
func (d *decision) why() model.Why {
	if d.cite != "" {
		return model.Why{Cite: d.cite}
	}
	if d.title == "" && d.text == "" && d.slug == "" {
		return model.Why{}
	}
	return model.Why{New: &model.NewDecision{Slug: d.slug, Title: d.title, Text: d.text}}
}

func (s *session) ruleRetire(ctx context.Context) *cobra.Command {
	var successor string
	var d decision
	c := &cobra.Command{
		Use:   "retire <id>",
		Short: "Retire a rule and keep it in its file",
		Long: `Retire a rule on a decision. The rule stays in its file with
the status retired, and the id is never given again. Name the
rule that takes its place with --successor, if there is one.
A rule that is retired already is left as it is.

  gragas rule retire <id> --decision-title "..." --decision-text "..."
  gragas rule retire <id> --decision <slug> --successor <id> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		if code, ok := s.asked(ctx, c, d.questions()); !ok {
			return exitErr(code)
		}
		req := model.RetireRule{ID: args[0], Successor: model.RuleID(successor), Why: d.why()}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleRetire(ctx, w, ask, req)
		}))
	}
	c.Flags().StringVar(&successor, "successor", "", "The rule that takes its place")
	d.flags(c)
	c.Flags().BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}

func (s *session) ruleRestore(ctx context.Context) *cobra.Command {
	var reviewBy string
	var d decision
	c := &cobra.Command{
		Use:   "restore <id>",
		Short: "Return a retired rule as active",
		Long: `Return a retired rule as active, on a decision, with the
date it is next confirmed. It skips the trial: it was active
before. The history keeps the retirement and the restore both.
A rule that is not retired is refused, naming its status.

  gragas rule restore <id> --review-by 2027-01-31 --decision <slug>
  gragas rule restore <id> --review-by 2027-01-31 \
    --decision-title "..." --decision-text "..." --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		qs := append([]prompted{{flag: "--review-by", ask: "When is it next confirmed, as YYYY-MM-DD?", to: &reviewBy}}, d.questions()...)
		if code, ok := s.asked(ctx, c, qs); !ok {
			return exitErr(code)
		}
		req := model.RestoreRule{ID: args[0], ReviewBy: reviewBy, Why: d.why()}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleRestore(ctx, w, ask, req)
		}))
	}
	c.Flags().StringVar(&reviewBy, "review-by", "", "The date it is next confirmed, YYYY-MM-DD")
	d.flags(c)
	c.Flags().BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}

func (s *session) ruleReplace(ctx context.Context) *cobra.Command {
	var statement, strength, definition, owner, reviewBy string
	var d decision
	c := &cobra.Command{
		Use:   "replace <id>",
		Short: "Replace a rule with one that says something else",
		Long: `Replace a rule on a decision: a new rule in the same concern,
as a draft, and the old rule retired with a pointer to it. A
change of what a rule means is a replace, never a reword of
its words. Projects that bind the old rule are told to move.

  gragas rule replace <id> --statement "..." --strength must \
    --decision-title "..." --decision-text "..."
  gragas rule replace <id> --statement "..." --strength must \
    --decision <slug> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		qs := append([]prompted{
			{flag: "--statement", ask: "What is the new rule, in one sentence?", to: &statement},
			{flag: "--strength", ask: "How firmly does it bind (must, should or may)?", to: &strength},
		}, d.questions()...)
		if code, ok := s.asked(ctx, c, qs); !ok {
			return exitErr(code)
		}
		succ := model.NewRule{Statement: statement, Strength: model.Strength(strength), Definition: definition, Owner: owner, ReviewBy: reviewBy}
		req := model.ReplaceRule{ID: args[0], Successor: succ, Why: d.why()}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleReplace(ctx, w, ask, req)
		}))
	}
	f := c.Flags()
	f.StringVar(&statement, "statement", "", "The rule in one sentence")
	f.StringVar(&strength, "strength", "", "How firmly it binds: must, should or may")
	f.StringVar(&definition, "definition", "", "What the rule means")
	f.StringVar(&owner, "rule-owner", "", "Who owns the rule")
	f.StringVar(&reviewBy, "review-by", "", "The date it is next confirmed, YYYY-MM-DD")
	d.flags(c)
	f.BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}

// ruleOnDecision is a rule command that takes an id and a decision: it asks
// for the decision, makes its request from the id and the why, and writes with
// --apply.
func (s *session) ruleOnDecision(ctx context.Context, c *cobra.Command, run func(port.Ports, model.ChangeAsk, string, model.Why) (*coreapi.Change, *model.Refusal, error)) *cobra.Command {
	var d decision
	c.Args = cobra.ExactArgs(1)
	c.Annotations = map[string]string{writes: "with --apply"}
	c.RunE = func(_ *cobra.Command, args []string) error {
		if code, ok := s.asked(ctx, c, d.questions()); !ok {
			return exitErr(code)
		}
		why := d.why()
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return run(w, ask, args[0], why)
		}))
	}
	d.flags(c)
	c.Flags().BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}
