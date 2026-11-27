// Package guard is the guard family: guard edit judges one edit an agent's
// hook proposes in a rulebook's repository, and refuses it when it changes a
// lifecycle field by hand (S59, A29). guard staged, the gate's form, is not
// built yet. The root joins it from the generated join_gen.go.
package guard

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 80

// Row is the one declaration of guard (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "guard", Then: []string{"edit", "staged"},
		Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsListing, State: kit.Built,
		Summary: "Refuse a hand edit of a lifecycle field",
		Examples: [2]kit.Example{
			{Line: "gragas guard edit", Does: "judge an edit on standard input"},
			{Line: "gragas guard staged", Does: "judge the staged change"},
		},
		Exits:    []model.Word{model.Done, model.Refused},
		JSONKind: "guard",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	guard := &cobra.Command{
		Use:   "guard",
		Short: "Refuse a hand edit of a lifecycle field",
		Long: `The guard holds a rulebook's lifecycle fields to their
commands: a status, an id, a scope or a binding changes only
through the command that writes its history entry. The hook
init prints calls guard edit before an agent edits a file,
and the git hook calls guard staged before a commit.

  gragas guard edit
  gragas guard staged`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	guard.AddCommand(judge(s, "edit", "Judge the edit an agent's hook gives", faultHint, Edit, `Reads one proposed edit on standard input, as an agent's
hook gives it (Write, Edit or MultiEdit as JSON), makes it in
memory and compares the rulebook before and after. An edit of
prose, or of a file that holds no lifecycle field, ends done.
A change of a lifecycle field ends refused, exit 5, naming the
field and the command that makes the change. An edit it cannot
read, place or judge is refused too. Each refusal is kept in
the journal. It runs only in a rulebook's repository.

  gragas guard edit < edit.json
  gragas guard edit --json < edit.json`))
	var mergeHook bool
	staged := judge(s, "staged", "Judge the staged change before a commit", stagedFaultHint, func(ctx context.Context, s kit.Session) int {
		return Staged(ctx, s, mergeHook)
	}, `Reads HEAD and the index from git, never the working folder,
and replays each line the staged change adds to history.jsonl:
its command is planned again over the tree before it, as the
line's who at the line's when, and the plan's digest must be
the line's. The files the commands write must be staged as
they write them, and nothing else may change a lifecycle field.
A field changed with no line, a line with no change, a forged
digest, or an earlier line rewritten or removed ends refused,
exit 5, and so does a change it cannot read or replay. During
a merge a line the other side's history holds is that side's.
git runs the pre-merge-commit hook before it records the merge,
so with --merge-hook a merge that changes no lifecycle field and
no history line ends done, and any other ends refused: git keeps
the merge, and git commit runs the pre-commit hook, which judges
it. Each refusal is kept in the journal. It runs only in a
rulebook's repository.

  gragas guard staged
  gragas guard staged --merge-hook
  gragas guard staged --json`)
	staged.Flags().BoolVar(&mergeHook, "merge-hook", false, "Run as git's pre-merge-commit hook, before git records the merge")
	guard.AddCommand(staged)
	return guard
}

// judge is one guard's command: it takes no argument, runs run over the
// session, and fails closed, a crash ending refused with fault as the line to
// type (kit.FailsClosed).
func judge(s kit.Session, use, short, fault string, run func(context.Context, kit.Session) int, long string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{kit.FailsClosed: fault},
		RunE:        func(c *cobra.Command, _ []string) error { return s.Exit(run(c.Context(), s)) },
	}
}

// stagedFaultHint is the line to type when guard staged itself fails.
const stagedFaultHint = "report the fault with the staged change that made it, and make the change through its command"

// Staged runs guard staged over the session and answers its exit code. A
// panic is the root's to recover, and ends refused (kit.FailsClosed), so a
// guard that crashes does not pass the commit.
func Staged(ctx context.Context, s kit.Session, mergeHook bool) int {
	g, ref, err := s.GuardStaged(ctx, mergeHook)
	switch {
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.End(kit.Ending{Word: model.NotReady, What: "not a rulebook: " + err.Error() + ". guard staged runs in a rulebook's repository", Next: "gragas init"})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas validate"})
	case err != nil:
		return refuseAs(s, coreapi.GuardStagedCommand, g, "the staged change could not be judged, so it is refused: "+err.Error(), "gragas validate")
	case ref != nil:
		return refuseAs(s, coreapi.GuardStagedCommand, g, ref.Observed(), ref.Hint())
	}
	if s.JSON() {
		s.Document(coreapi.GuardDocAs(coreapi.GuardStagedCommand, g, "", ""))
	}
	return s.End(kit.Ending{Word: model.Done, What: "every lifecycle field staged is recorded in " + g.Path})
}

// faultHint is the line to type when the guard itself fails: the root's one
// recover ends a panic of guard edit refused with it (kit.FailsClosed).
const faultHint = "report the fault with the edit that made it, and make the change through its command"

// Edit runs guard edit over the session and answers its exit code. A fault of
// its own, a panic, is the root's to recover: the command carries
// kit.FailsClosed, so it ends refused, never done, and a guard that crashes
// does not pass the edit.
func Edit(ctx context.Context, s kit.Session) int {
	g, ref, err := s.GuardEdit(ctx)
	switch {
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.End(kit.Ending{Word: model.NotReady, What: "not a rulebook: " + err.Error() + ". guard edit runs in a rulebook's repository", Next: "gragas init"})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas validate"})
	case err != nil:
		return refuse(s, g, "the edit could not be judged, so it is refused: "+err.Error(), "gragas validate")
	case ref != nil:
		return refuse(s, g, ref.Observed(), ref.Hint())
	}
	if s.JSON() {
		s.Document(coreapi.GuardDoc(g, "", ""))
	}
	return s.End(kit.Ending{Word: model.Done, What: allowed(g)})
}

// allowed is the sentence beside done.
func allowed(g model.Guard) string {
	return g.Path + " changes no lifecycle field"
}

// refuse ends the run refused: the fields as a table above the word, the
// document under --json, and the sentence and the line to type on standard
// error too, which the hook hands to the agent.
func refuse(s kit.Session, g model.Guard, why, next string) int {
	return refuseAs(s, coreapi.GuardCommand, g, why, next)
}

// refuseAs is refuse for the guard command named.
func refuseAs(s kit.Session, command string, g model.Guard, why, next string) int {
	s.Aside("refused: " + why)
	s.Aside("instead: " + next)
	if s.JSON() {
		s.Document(coreapi.GuardDocAs(command, g, why, next))
		return s.End(kit.Ending{Word: model.Refused, What: why, Next: next})
	}
	return s.End(kit.Ending{Word: model.Refused, What: why, Next: next, Lead: lead(command, g)})
}

// lead is the table of the fields an edit, or a staged change, changes with
// no history line, one row each.
func lead(command string, g model.Guard) []model.Block {
	if len(g.Changes) == 0 {
		return nil
	}
	t := model.Table{Columns: []model.Column{{Title: "about", Priority: 9}, {Title: "field", Priority: 9}, {Title: "before", Priority: 5},
		{Title: "after", Priority: 5}, {Title: "the command", Priority: 7}}}
	for _, c := range g.Changes {
		t.Rows = append(t.Rows, []string{c.Entity, c.Field, orNone(c.Before), orNone(c.After), orNone(c.Command)})
	}
	title := "Lifecycle fields this edit changes by hand"
	if command == coreapi.GuardStagedCommand {
		title = "Lifecycle fields the staged change does not record"
	}
	return []model.Block{model.Heading{Text: title}, t}
}

func orNone(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
