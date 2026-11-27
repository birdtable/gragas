// Package journal is the journal command: the runs that wrote on this folder,
// from the record alone (cli.commands.24). list and restore read; restore
// prints the lines that put a run's files back and runs none of them
// (cli.commands.25), so it has no --apply. recover is the one that writes: it
// rolls an open run back, with the write path's own roll-back, and never
// resumes it (cli.commands.7). It writes only with --apply. It lives in a
// package of its own below internal/cli/cmd; the root joins it from the
// generated join_gen.go.
package journal

import (
	"context"
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
const Order = 530

const recoverLine = "gragas journal recover --apply"

// Row is the one declaration of journal (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "journal", Then: []string{"list", "recover", "restore"}, Order: Order, Spec: true,
		Context: kit.Anywhere, Ends: kit.EndsListing, State: kit.Built,
		Summary: "List the runs that wrote, roll an interrupted one back or print how to restore",
		Examples: [2]kit.Example{
			{Line: "gragas journal list", Does: "list the runs"},
			{Line: "gragas journal recover --apply", Does: "roll an interrupted run back"},
		},
		Writes:   []string{"the files an open run changed, and its record (recover)"},
		Exits:    []model.Word{model.Done, model.Refused},
		JSONKind: "journal",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "journal",
		Short: "List, recover or restore runs that wrote here",
		Long: `Every run that wrote on this folder keeps a record: what it
meant to do, the copy of each file it replaced and how it
ended. These commands read that record; nothing here asks a
tool. The folder is named by a digest, never by its path.

  gragas journal list
  gragas journal recover --apply
  gragas journal restore <run>`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	c.AddCommand(list(s), recoverCmd(s), restore(s))
	return c
}

func list(s kit.Session) *cobra.Command {
	return &cobra.Command{
		RunE:  func(c *cobra.Command, _ []string) error { return s.Exit(runList(c.Context(), s)) },
		Use:   "list",
		Args:  cobra.NoArgs,
		Short: "List the runs that wrote here",
		Long: `One row for each run: its id, the command, how it ended
(open for a run with no close), the digest of its plan and
how many files it kept a copy of. It reads and changes
nothing.

  gragas journal list
  gragas journal list --json`,
	}
}

func recoverCmd(s kit.Session) *cobra.Command {
	c := &cobra.Command{Use: "recover", Short: "Roll back the runs that were interrupted"}
	c.Long = `A run that was killed after its first write has a record
with no close. recover puts every file of each such run back
from the copies its record kept, under the folder's lock, and
closes the record as rolled back. It never finishes a run:
plan it again. Without --apply it lists the runs it would
roll back and writes nothing.

  gragas journal recover
  gragas journal recover --apply`
	c.Args = cobra.NoArgs
	c.Annotations = map[string]string{kit.Writes: "with --apply"}
	c.RunE = func(c *cobra.Command, _ []string) error { return s.Exit(runRecover(c, s)) }
	c.Flags().BoolVar(s.ApplyFlag(), "apply", false, "Write the files")
	return c
}

func restore(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "restore <run>",
		Short: "Print the lines that put a run's files back",
		Long: `Prints one line for each file the run changed: a cp of the
copy it kept, or an rm for a file it made. It runs none of
them, and it takes no --apply: you read the lines and paste
them. For PowerShell or cmd it prints no line: it says each
step in words, as rm and cp are not commands there. If a file
no longer holds what the run left, it is refused and names
the runs that changed it since.

  gragas journal restore <run>
  gragas journal restore <run> --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error { return s.Exit(runRestore(c.Context(), s, args[0])) },
	}
}

// failed ends a run on an error: the sentence the session worded, or the
// core's own.
func failed(s kit.Session, err error) int {
	if said, ok := errors.AsType[kit.Said](err); ok {
		return s.End(kit.Ending{Word: model.Failed, What: said.What})
	}
	return s.End(kit.Ending{Word: model.Failed, What: err.Error()})
}

// short is the first characters of a digest, for a table.
func short(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		d = d[:12]
	}
	return d
}

func runTable(runs []model.JournalRun) model.Table {
	t := model.Table{Columns: []model.Column{{Title: "Run", Priority: 9}, {Title: "State", Priority: 8}, {Title: "Command", Priority: 6},
		{Title: "Files", Priority: 4}, {Title: "Plan", Priority: 2}}}
	for _, r := range runs {
		t.Rows = append(t.Rows, []string{r.Run, r.State, r.Command, strconv.Itoa(len(r.Files)), short(r.Plan)})
	}
	return t
}

func runList(ctx context.Context, s kit.Session) int {
	got, err := s.JournalList(ctx)
	if err != nil {
		return failed(s, err)
	}
	if s.JSON() {
		s.Document(got)
		return exit.Done
	}
	if len(got.Runs) == 0 {
		return s.End(kit.Ending{Word: model.Done, What: "no run has written on this folder", Lead: []model.Block{model.Heading{Text: "gragas journal list"}}})
	}
	lead := []model.Block{model.Heading{Text: "gragas journal list"}, runTable(got.Runs)}
	if got.Torn > 0 {
		lead = append(lead, model.Note{Text: fmt.Sprintf("%s skipped as torn", model.Plural(got.Torn, "line"))})
	}
	return s.End(kit.Ending{Word: model.Done, What: model.Plural(len(got.Runs), "run") + " listed, folder " + short(got.Root), Lead: lead})
}

func runRestore(ctx context.Context, s kit.Session, run string) int {
	got, err := s.JournalRestore(ctx, run)
	switch {
	case errors.Is(err, coreapi.ErrNoRun):
		return s.End(kit.Ending{Word: model.Refused, What: "the journal holds no run " + run, Next: "gragas journal list"})
	case errors.Is(err, coreapi.ErrRunOpen):
		return s.End(kit.Ending{Word: model.Refused, What: "run " + run + " has no close, so it is still open and its files may be half written", Next: recoverLine})
	case errors.Is(err, coreapi.ErrNoLine):
		// no line for any file: a part of the way back would read as all of it
		return s.End(kit.Ending{Word: model.Refused, What: "run " + run + " is not restored: " + err.Error(), Next: "gragas help journal restore"})
	case err != nil:
		return failed(s, err)
	case len(got.Moved) > 0:
		return moved(s, run, got)
	case len(got.Damaged) > 0:
		return damaged(s, run, got)
	}
	if s.JSON() {
		s.Document(got)
		return exit.Done
	}
	// A line is said whole, apart from the screen: a line to paste is never
	// wrapped at the width.
	for _, l := range got.Lines {
		s.Say(l)
	}
	what := "run " + run + " kept nothing to put back"
	switch n := len(got.Lines); {
	case n > 0:
		what = model.Plural(n, "line") + " put run " + run + " back; none was run"
	case len(got.Files) > 0:
		for _, f := range got.Files {
			s.Say(stepInWords(f))
		}
		what = model.Plural(len(got.Files), "step") + " put run " + run + " back, in the folder " + short(got.Root) + "; none was run"
	}
	return s.End(kit.Ending{Word: model.Done, What: what})
}

// stepInWords is f's step said in words, each path as %q, for a shell gragas
// writes no rm or cp line for: the person runs it with the
// shell's own commands.
func stepInWords(f model.RestoreFile) string {
	if f.Op == "remove" {
		return fmt.Sprintf("remove %q", f.Path)
	}
	return fmt.Sprintf("copy %q over %q", f.From, f.Path)
}

// moved ends a restore on a file that does not hold what the run left.
func moved(s kit.Session, run string, got model.Restoration) int {
	s.Data(got)
	m := got.Moved[0]
	by, next := "it was changed outside gragas", "gragas journal list"
	if len(m.By) > 0 {
		by, next = "run "+strings.Join(m.By, " and run ")+" changed it since", "gragas journal restore "+m.By[len(m.By)-1]
	}
	what := fmt.Sprintf("%s moved after run %s: %s, and putting its copy back would undo that", m.Path, run, by)
	if n := len(got.Moved); n > 1 {
		what += fmt.Sprintf(" (and %s more)", model.Plural(n-1, "file"))
	}
	return s.End(kit.Ending{Word: model.Refused, What: what, Next: next})
}

// damaged ends a restore whose kept copy is missing or is not the bytes the
// record names: putting it back would write the wrong bytes.
func damaged(s kit.Session, run string, got model.Restoration) int {
	s.Data(got)
	d := got.Damaged[0]
	what := damagedSaid(run, d)
	if n := len(got.Damaged); n > 1 {
		what += fmt.Sprintf(" (and %s more)", model.Plural(n-1, "file"))
	}
	return s.End(kit.Ending{Word: model.Refused, What: what, Next: "gragas journal list"})
}

// damagedSaid is the words for one damaged copy. A copy nobody kept has no
// path, and then the copy is not named: "it could not be read" says it is gone.
func damagedSaid(run string, d model.Damaged) string {
	have := "it could not be read"
	if d.Got != "" {
		have = "it holds " + d.Got
	}
	what := fmt.Sprintf("the copy of %s that run %s kept is damaged: %s, and the record names %s", d.Path, run, have, d.Expected)
	if d.Copy != "" {
		what += fmt.Sprintf(" (copy %s)", d.Copy)
	}
	return what
}

func runRecover(c *cobra.Command, s kit.Session) int {
	if !s.Applying() {
		return recoverPlan(c.Context(), s)
	}
	got, err := s.JournalRecover(c.Context())
	if err != nil {
		if said, ok := errors.AsType[kit.Said](err); ok {
			return s.End(kit.Ending{Word: model.Failed, What: s.WithRun(said.What)})
		}
		return s.End(kit.Ending{Word: model.Failed, What: s.WithRun(err.Error())})
	}
	if s.JSON() && got.Word == model.Done {
		s.Document(got)
		return exit.Done
	}
	s.Data(got)
	switch got.Word {
	case model.Busy:
		return s.End(kit.Ending{Word: model.Busy, What: got.Observed, Next: recoverLine})
	case model.Failed:
		return s.End(kit.Ending{Word: model.Failed, What: s.WithRun(got.Why)})
	default:
	}
	if len(got.Recovered) == 0 {
		return s.End(kit.Ending{Word: model.Done, What: "no run was open, and nothing was changed"})
	}
	items := make([]string, 0, len(got.Recovered))
	for _, r := range got.Recovered {
		items = append(items, s.Change("roll back", "run "+r.Run+" ("+r.Command+")"))
	}
	return s.End(kit.Ending{Word: model.Done, What: model.Plural(len(got.Recovered), "run") + " rolled back", Lead: []model.Block{model.List{Items: items}}})
}

// recoverPlan lists the open runs recover would roll back, and writes nothing.
func recoverPlan(ctx context.Context, s kit.Session) int {
	got, err := s.JournalList(ctx)
	if err != nil {
		return failed(s, err)
	}
	var open []model.JournalRun
	for _, r := range got.Runs {
		if r.State == "open" {
			open = append(open, r)
		}
	}
	got.Runs = open
	s.Data(got)
	if len(open) == 0 {
		return s.End(kit.Ending{Word: model.Done, What: "no run is open, so there is nothing to roll back"})
	}
	items := make([]string, 0, len(open))
	for _, r := range open {
		items = append(items, s.Change("roll back", "run "+r.Run+" ("+r.Command+")"))
	}
	return s.End(kit.Ending{Word: model.Done, What: "nothing was rolled back yet, the lines above are the plan", Next: recoverLine,
		Lead: []model.Block{model.List{Items: items}, model.DryRunFooter{Flag: "--apply"}}})
}
