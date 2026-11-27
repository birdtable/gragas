// Package cache is the cache command: list says what gragas keeps and how old
// it is, and clear removes the closed runs of this folder's journal past the
// keep age, with the scratch folders a stopped run left and the folders the
// tools keep their caches in. It lives in a package of its own below internal/cli/cmd; the root
// joins it from the generated join_gen.go. clear writes, so it takes --apply
// and its dry run ends on that line.
package cache

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 540

// Row is the one declaration of cache (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "cache", Then: []string{"list", "clear"}, Order: Order, Spec: true,
		Context: kit.Anywhere, Ends: kit.EndsListing, State: kit.Built,
		Summary: "List what gragas keeps in its cache or clear it",
		Examples: [2]kit.Example{
			{Line: "gragas cache list", Does: "list it"},
			{Line: "gragas cache clear --apply", Does: "clear it"},
		},
		Writes:   []string{"the closed runs of the journal, the scratch folders a stopped run left and the tools' cache folders (clear)"},
		Exits:    []model.Word{model.Done, model.Refused},
		JSONKind: "cache",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "cache",
		Short: "List or clear what gragas keeps in its cache",
		Long: `What gragas keeps on this machine, outside any project:
the copies of what a run replaced and the runs they belong
to, and the caches that later commands fill. Nothing in it
carries a project's name.

  gragas cache list
  gragas cache clear --apply`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	c.AddCommand(list(s), clear(s))
	return c
}

func list(s kit.Session) *cobra.Command {
	c := &cobra.Command{Use: "list", Short: "List what is kept, how big and how old"}
	c.Long = `One row for each kind of thing gragas keeps: how many
files, how big, how old the oldest closed run is and how
many runs are still open. A figure that cannot be told is
null and says why. It reads and changes nothing.

  gragas cache list
  gragas cache list --json`
	c.Args = cobra.NoArgs
	c.RunE = func(c *cobra.Command, _ []string) error { return s.Exit(runList(c, s)) }
	return c
}

func clear(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "clear",
		Short: "Remove the closed runs kept longer than the keep age, the scratch folders a stopped run left and the tools' caches",
		Long: `Removes, from the journal of this folder, every closed run
older than the keep setting, with the copies it kept. A run
that never closed is never removed, whatever its age: a
roll-back may need it. It also removes the scratch folders a
stopped run left, when no run holds them, and every folder a
tool keeps its caches in from one step to the next, of every
project of this account, which the next step fills again; a
step that runs meanwhile, in any project, may fail. Other
folders' journals are cleared when you run it there.
Without --apply it lists what it would remove and writes
nothing. A second run removes nothing.

  gragas cache clear
  gragas cache clear --apply`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{kit.Writes: "with --apply"},
		RunE:        func(c *cobra.Command, _ []string) error { return s.Exit(runClear(c, s)) },
	}
	c.Flags().BoolVar(s.ApplyFlag(), "apply", false, "Write the files")
	return c
}

func runList(c *cobra.Command, s kit.Session) int {
	got, err := s.CacheList(c.Context())
	said, isSaid := errors.AsType[kit.Said](err)
	refused, isRefused := errors.AsType[kit.Refusing](err)
	switch {
	case isRefused:
		return s.End(kit.Ending{Word: model.Refused, What: refused.What, Next: refused.Next})
	case isSaid:
		return s.End(kit.Ending{Word: model.Failed, What: said.What})
	case err != nil:
		return s.End(kit.Ending{Word: model.Failed, What: "the cache could not be listed: " + err.Error()})
	}
	if s.JSON() {
		s.Document(got)
		return exit.Done
	}
	table := model.Table{Columns: []model.Column{{Title: "Kind", Priority: 9}, {Title: "Files", Priority: 5}, {Title: "Size", Priority: 7},
		{Title: "Oldest closed", Priority: 3}, {Title: "Open", Priority: 1}}}
	var notes []model.Block
	for _, r := range got.Kinds {
		table.Rows = append(table.Rows, []string{r.Kind, strconv.Itoa(r.Files), size(r.Size), day(r.Oldest), count(r.Open)})
		for _, why := range []string{r.SizeNote, r.OldestNote} {
			if why != "" {
				notes = append(notes, model.Note{Text: r.Kind + ": " + why})
			}
		}
	}
	lead := append([]model.Block{model.Heading{Text: "gragas cache list"}, table}, notes...)
	return s.End(kit.Ending{Word: model.Done, What: fmt.Sprintf("%d kinds listed", len(got.Kinds)), Lead: lead})
}

// size is a byte count in the unit a person reads, or "unknown" for null.
func size(n *int64) string {
	switch {
	case n == nil:
		return "unknown"
	case *n < 1024:
		return strconv.FormatInt(*n, 10) + " B"
	case *n < 1024*1024:
		return strconv.FormatInt(*n/1024, 10) + " KiB"
	}
	return strconv.FormatInt(*n/(1024*1024), 10) + " MiB"
}

// day is a Unix time as its UTC date, or "none" for null. A date holds a golden
// still where an age would move with the clock; --json carries the age.
func day(at *int64) string {
	if at == nil {
		return "none"
	}
	return time.Unix(*at, 0).UTC().Format(time.DateOnly)
}

func count(n *int) string {
	if n == nil {
		return "-"
	}
	return strconv.Itoa(*n)
}

const clearLine = "gragas cache clear --apply"

// runClear removes the closed runs past the keep age from this folder's
// journal. Without --apply it reads and lists; with it, the session takes the
// folder's lock and calls the journal's own sweep, under the run record the
// root's pre-run hook took.
func runClear(c *cobra.Command, s kit.Session) int {
	plan, err := s.CacheClearPlan(c.Context())
	said, isSaid := errors.AsType[kit.Said](err)
	switch {
	case isSaid:
		return s.End(kit.Ending{Word: model.Failed, What: said.What})
	case err != nil:
		return s.End(kit.Ending{Word: model.Failed, What: err.Error()})
	}
	kept := fmt.Sprintf("%s kept, %s open and never cleared", model.Plural(plan.Keep, "closed run"), model.Plural(plan.Open, "run"))
	if !s.Applying() {
		s.Data(plan)
		if plan.Remove == 0 && plan.Scratch == 0 && plan.Tools == 0 {
			return s.End(kit.Ending{Word: model.Done, What: "nothing to clear: " + kept})
		}
		items := make([]string, 0, plan.Remove+2)
		for _, r := range plan.Runs {
			items = append(items, s.Change("remove", "run "+r))
		}
		items = append(items, folderItems(s, plan.Scratch, plan.Tools)...)
		return s.End(kit.Ending{Word: model.Done, What: "nothing was removed yet, the lines above are the plan: " + kept, Next: clearLine,
			Lead: []model.Block{model.List{Items: items}, model.DryRunFooter{Flag: "--apply"}}})
	}
	got, err := s.CacheClear(c.Context())
	if err != nil {
		return s.End(kit.Ending{Word: model.Failed, What: s.WithRun(err.Error())})
	}
	if got.Word == model.Busy {
		s.Data(got)
		return s.End(kit.Ending{Word: model.Busy, What: got.Observed, Next: clearLine})
	}
	if s.JSON() {
		s.Document(got)
		return exit.Done
	}
	items := make([]string, 0, len(plan.Runs))
	for _, r := range plan.Runs[:min(got.Removed, len(plan.Runs))] {
		items = append(items, s.Change("remove", "run "+r))
	}
	items = append(items, folderItems(s, got.Scratch, got.Tools)...)
	var lead []model.Block
	if len(items) > 0 {
		lead = append(lead, model.List{Items: items})
	}
	return s.End(kit.Ending{Word: model.Done, What: removedText(got), Lead: lead})
}

// folderItems are the plan's lines of the folders a clear removes besides the
// runs: the scratch folders and the tools' cache folders, each when there are
// any.
func folderItems(s kit.Session, scratch, tools int) []string {
	var items []string
	if scratch > 0 {
		items = append(items, s.Change("remove", model.Plural(scratch, "scratch folder")+" a stopped run left"))
	}
	if tools > 0 {
		items = append(items, s.Change("remove", model.Plural(tools, "tools' cache folder")+" of every project of this account"))
	}
	return items
}

// removedText says what a clear removed: the runs of the journal, and the
// scratch folders and the tools' cache folders when there were any.
func removedText(got model.Cleared) string {
	text := model.Plural(got.Removed, "run") + " removed from the journal"
	var also []string
	if got.Scratch > 0 {
		also = append(also, model.Plural(got.Scratch, "scratch folder"))
	}
	if got.Tools > 0 {
		also = append(also, model.Plural(got.Tools, "tools' cache folder"))
	}
	if len(also) > 0 {
		text += " and " + strings.Join(also, " and ") + " removed"
	}
	return text
}
