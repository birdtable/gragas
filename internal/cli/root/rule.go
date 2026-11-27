package root

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ruleCmd is `gragas rule`, the family that changes a rule's life. Each word
// plans the change, shows it and writes it only with --apply (cli.commands.2),
// with a line for the history.
func (s *session) ruleCmd(ctx context.Context) *cobra.Command {
	rule := &cobra.Command{
		Use:   "rule",
		Short: "Add or change a rule, with its history",
		Long: `Every change to a rule goes through one of these words.
Each shows a plan and writes only with --apply: the rule's
file, the sources and decisions it names, and one line of
history.jsonl. The words and prose of a rule are a file edit:
rule new seeds them once and never rewrites them.

  gragas rule new
  gragas rule edit <id> --rule-owner <name> --apply`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	rule.AddCommand(s.ruleNew(ctx), s.ruleEdit(ctx), s.ruleReplace(ctx), s.ruleRetire(ctx), s.ruleRestore(ctx), s.ruleConfirm(ctx))
	rule.AddCommand(s.ruleDecided(ctx)...)
	rule.AddCommand(s.ruleExclusions(ctx)...)
	return rule
}

func (s *session) ruleNew(ctx context.Context) *cobra.Command {
	var concern, statement, strength, source, decisionText string
	var paths, parts, kinds, cites []string
	var definition, owner, reviewBy, title string
	var own bool
	c := &cobra.Command{
		Use:   "new",
		Short: "Add a draft rule to a concern",
		Long: `Add a rule to a concern as a draft: its statement, its
strength, where it applies, why, and a source for it or the
owner's own decision. At a terminal the missing answers are
asked, each with the flag that gives it in a script. The rule
is proposed; a later command moves it on. A second identical
run finds the rule and ends "nothing to do".

  gragas rule new
  gragas rule new --concern go.style --statement "..." --strength must \
    --own --decision-text "..." --apply`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		qs := []prompted{
			{flag: "--concern", ask: "Which concern, as part.concern?", to: &concern},
			{flag: "--statement", ask: "What is the rule, in one sentence?", to: &statement},
			{flag: "--strength", ask: "How firmly does it bind (must, should or may)?", to: &strength},
			{flag: "--source", ask: "Which source slug backs it, or own for your own decision?", to: &source, skip: func() bool { return own || len(cites) > 0 }},
			{flag: "--decision-title", ask: "What is the decision called?", to: &title, skip: func() bool { return !own && !strings.EqualFold(source, "own") }},
			{flag: "--decision-text", ask: "What did you decide, and why?", to: &decisionText, skip: func() bool { return !own && !strings.EqualFold(source, "own") }},
		}
		if code, ok := s.asked(ctx, c, qs); !ok {
			return exitErr(code)
		}
		if strings.EqualFold(source, "own") {
			own = true
		} else if source != "" {
			cites = append(cites, source)
		}
		part, conc, _ := strings.Cut(concern, ".")
		req := model.NewRule{Part: part, Concern: conc, Statement: statement, Strength: model.Strength(strength),
			Definition: definition, Owner: owner, ReviewBy: reviewBy, Cite: cites, Own: own}
		if len(paths)+len(parts)+len(kinds) > 0 {
			req.Scope = &model.RuleScope{Parts: parts, Paths: paths, Kinds: kinds}
		}
		if own {
			req.Decision = &model.NewDecision{Title: title, Text: decisionText}
		}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleNew(ctx, w, ask, req)
		}))
	}
	f := c.Flags()
	f.StringVar(&concern, "concern", "", "The concern, as part.concern")
	f.StringVar(&statement, "statement", "", "The rule in one sentence")
	f.StringVar(&strength, "strength", "", "How firmly it binds: must, should or may")
	f.StringVar(&definition, "definition", "", "What the rule means")
	f.StringSliceVar(&paths, "scope-path", nil, "A glob the rule applies to; repeatable")
	f.StringSliceVar(&parts, "scope-part", nil, "A part the rule applies to; repeatable")
	f.StringSliceVar(&kinds, "scope-kind", nil, "A kind the rule applies to; repeatable")
	f.StringVar(&source, "source", "", "A source slug that backs it, or own")
	f.StringSliceVar(&cites, "cite", nil, "A source slug that backs it; repeatable")
	f.BoolVar(&own, "own", false, "The owner's own decision is the source")
	f.StringVar(&title, "decision-title", "", "The title of the decision")
	f.StringVar(&decisionText, "decision-text", "", "What was decided and why")
	f.StringVar(&owner, "rule-owner", "", "Who owns the rule")
	f.StringVar(&reviewBy, "review-by", "", "The date it is next confirmed, YYYY-MM-DD")
	f.BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}

func (s *session) ruleEdit(ctx context.Context) *cobra.Command {
	var owner, reviewBy, zeroBy string
	var parts, paths, kinds, relates, conflicts, depends, covers, cites, uncites []string
	c := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change a rule's owner, dates, scope, links, sources",
		Long: `Change the lifecycle fields of a rule: its owner, its
review and zero dates, what its scope covers, the rules it
relates to and the sources it cites. The statement and the
reasoning are prose: edit the file. A hand edit of a field
this command does not touch is left as it is. An empty value
takes a field away. A rule past proposed keeps one source, so
its last one is not uncited; a source no rule cites any more
stays in the book.

  gragas rule edit <id> --rule-owner <name>
  gragas rule edit <id> --uncite <source slug> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{writes: "with --apply"},
	}
	c.RunE = func(_ *cobra.Command, args []string) error {
		req := model.EditRule{ID: args[0], AddParts: parts, AddPaths: paths, AddKinds: kinds,
			Relates: ids(relates), Conflicts: ids(conflicts), Depends: ids(depends), Covers: ids(covers), Cite: cites, Uncite: uncites}
		for _, o := range []struct {
			flag string
			v    *string
			to   **string
		}{{"rule-owner", &owner, &req.Owner}, {"review-by", &reviewBy, &req.ReviewBy}, {"zero-by", &zeroBy, &req.ZeroBy}} {
			if c.Flags().Changed(o.flag) {
				*o.to = o.v
			}
		}
		return exitErr(s.ruleChange(ctx, c, args, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleEdit(ctx, w, ask, req)
		}))
	}
	f := c.Flags()
	f.StringVar(&owner, "rule-owner", "", "Who owns the rule")
	f.StringVar(&reviewBy, "review-by", "", "The date it is next confirmed, YYYY-MM-DD")
	f.StringVar(&zeroBy, "zero-by", "", "The date its findings must reach zero, YYYY-MM-DD")
	f.StringSliceVar(&parts, "scope-part", nil, "A part the rule applies to; repeatable")
	f.StringSliceVar(&paths, "scope-path", nil, "A glob the rule applies to; repeatable")
	f.StringSliceVar(&kinds, "scope-kind", nil, "A kind the rule applies to; repeatable")
	f.StringSliceVar(&relates, "relates", nil, "Add a rule it relates to; repeatable")
	f.StringSliceVar(&conflicts, "conflicts", nil, "Add a rule it conflicts with; repeatable")
	f.StringSliceVar(&depends, "depends", nil, "Add a rule it depends on; repeatable")
	f.StringSliceVar(&covers, "covers", nil, "Add a rule it covers; repeatable")
	f.StringSliceVar(&cites, "cite", nil, "A source slug that backs it; repeatable")
	f.StringSliceVar(&uncites, "uncite", nil, "A source slug it stops citing; repeatable")
	f.BoolVar(&s.applied, "apply", false, "Write the files")
	return c
}

func ids(in []string) []model.RuleID {
	var out []model.RuleID
	for _, s := range in {
		out = append(out, model.RuleID(s))
	}
	return out
}

// ruleChange makes the change of a rule command and runs it through the form.
func (s *session) ruleChange(ctx context.Context, c *cobra.Command, args []string, mk func(port.Ports, model.ChangeAsk) (*coreapi.Change, *model.Refusal, error)) int {
	return s.changeWith(ctx, c, args, mk, s.canAsk())
}

// changeWith is ruleChange, and asks at a terminal only when asking is true: a
// command whose plan is not the plan it applies (plugin record, whose apply
// runs a tool) never asks, and is applied by --apply alone.
func (s *session) changeWith(ctx context.Context, c *cobra.Command, args []string, mk func(port.Ports, model.ChangeAsk) (*coreapi.Change, *model.Refusal, error), asking bool) int {
	root, ok := s.folder()
	if !ok {
		return exit.Failed
	}
	argv, end := ruleArgv(c, args)
	record, line, err := s.planLinesAt(argv, end)
	if err != nil {
		return s.end(ending{word: model.Usage, what: err.Error(), next: helpLine(c)})
	}
	ask := model.ChangeAsk{Root: root, Command: record, Applies: line, Who: c.CommandPath(),
		Run: s.rec.id(), Engine: s.build.String(), NamesFile: s.p.Config.NamesFile}
	if ask.Run == "" {
		// a plan that is not applied has no run record; the label stands for it
		ask.Run = "dry-run"
	}
	h, err := s.p.History(ask.Run)
	switch {
	case errors.Is(err, ErrNoGit):
		return s.noGit()
	case err != nil:
		return s.end(ending{word: model.Failed, what: "git could not be set up: " + err.Error()})
	}
	w := s.p.World
	w.History = h
	ch, ref, err := mk(w, ask)
	next, byPlugin := coreapi.PluginNext(err)
	switch {
	case byPlugin:
		// a value no installed plugin ranks, or a plugin not approved
		return s.end(ending{word: model.NotReady, what: err.Error(), next: next})
	case ref != nil:
		return s.end(ending{word: ref.Word(), what: ref.Observed(), next: ref.Hint()})
	case errors.Is(err, coreapi.ErrNotReady):
		// validate names what is not ready: a file the engine cannot read, or a
		// parent not fetched at its pin
		return s.end(ending{word: model.NotReady, what: coreapi.Reason(err), next: "gragas validate"})
	case errors.Is(err, coreapi.ErrNothingToImport):
		return s.end(ending{word: model.Done, what: "nothing to import"})
	case err != nil:
		return s.end(ending{word: model.Failed, what: "the plan could not be made: " + err.Error()})
	}
	return s.plan(ctx, planRun{cmd: c, change: ch, next: line, ask: asking})
}

// noGit ends a run that needs the history of the repository when the settings
// name no git: programs.git is where the path to it comes from.
func (s *session) noGit() int {
	return s.end(ending{word: model.NotReady, what: "this needs git to name who made the change, and the settings name none: set programs.git to git's absolute path (docs/settings.md)", next: "gragas help rule"})
}
