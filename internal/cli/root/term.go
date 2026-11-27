package root

import (
	"context"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// termCmd is `gragas term`, the family that defines, changes and retires a
// rulebook's terms. Each word plans the change, shows it and writes it only
// with --apply (cli.commands.1), with a line for the history. A term's
// definition is prose: edit terms.yaml.
func (s *session) termCmd(ctx context.Context) *cobra.Command {
	term := &cobra.Command{
		Use:   "term",
		Short: "Add, change or retire a term, with its history",
		Long: `A term is a word the rulebook defines once, so a statement
can write {slug} and stay neutral. A term may stand for a
literal, such as a header name or a package, which the names
check then lets a statement use. Each word shows a plan and
writes only with --apply: terms.yaml and one line of
history.jsonl. A definition is prose: edit the file.

  gragas term new
  gragas term retire <slug> --apply`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	term.AddCommand(s.termNew(ctx), s.termEdit(ctx), s.termRetire(ctx))
	return term
}

func (s *session) termNew(ctx context.Context) *cobra.Command {
	var slug, definition, literal string
	c := &cobra.Command{
		Use:   "new",
		Short: "Define a term",
		Long: `Define a term: its slug, its meaning in one line, and the
literal it stands for, if there is one. At a terminal the
missing answers are asked, each with the flag that gives it
in a script. A term that is defined already, with the same
words, ends "nothing to do".

  gragas term new
  gragas term new --slug header --literal X-Request-Id \
    --term-definition "The name a request carries." --apply`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		qs := []prompted{
			{flag: "--slug", ask: "What is the term, as lower-case words joined by hyphens?", to: &slug},
			{flag: "--term-definition", ask: "What does it mean, in one line?", to: &definition},
		}
		if code, ok := s.asked(ctx, c, qs); !ok {
			return exitErr(code)
		}
		req := model.NewTerm{Slug: slug, Definition: definition, Literal: literal}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.TermNew(ctx, w, ask, req)
		}))
	}
	f := c.Flags()
	f.StringVar(&slug, "slug", "", "The term, as lower-case words joined by hyphens")
	f.StringVar(&definition, "term-definition", "", "What the term means, in one line")
	f.StringVar(&literal, "literal", "", "The header name, package or other literal it stands for")
	f.BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}

func (s *session) termEdit(ctx context.Context) *cobra.Command {
	var literal string
	c := &cobra.Command{
		Use:   "edit <slug>",
		Short: "Change a term's literal",
		Long: `Change the lifecycle field of a term: the literal it stands
for. An empty value takes the literal away. The definition is
prose: edit terms.yaml. A literal the term has already ends
"nothing to do".

  gragas term edit <slug> --literal X-Request-Id
  gragas term edit <slug> --literal "" --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		req := model.EditTerm{Slug: args[0]}
		if c.Flags().Changed("literal") {
			req.Literal = &literal
		}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.TermEdit(ctx, w, ask, req)
		}))
	}
	c.Flags().StringVar(&literal, "literal", "", "The header name, package or other literal it stands for")
	c.Flags().BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}

func (s *session) termRetire(ctx context.Context) *cobra.Command {
	var d decision
	c := &cobra.Command{
		Use:   "retire <slug>",
		Short: "Retire a term no rule uses",
		Long: `Retire a term on a decision. It stays in terms.yaml with the
status retired, and no statement may use it. A term a rule
still uses is refused, and the refusal names the rules. With no
decision given, it asks for one at a terminal; elsewhere the
retirement is refused. A term that is retired already ends
"nothing to do".

  gragas term retire <slug> --decision-title "..." --decision-text "..."
  gragas term retire <slug> --decision <slug> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		// At a terminal the decision is asked for, as rule retire asks it.
		// Where no one can answer, nothing is asked, and the core refuses a
		// retirement with no decision as it always has (refused, 5), rather
		// than the usage ending asked gives where it cannot ask.
		if s.canAsk() {
			if code, ok := s.asked(ctx, c, d.questions()); !ok {
				return exitErr(code)
			}
		}
		req := model.RetireTerm{Slug: args[0], Why: d.why()}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.TermRetire(ctx, w, ask, req)
		}))
	}
	d.flags(c)
	c.Flags().BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}
