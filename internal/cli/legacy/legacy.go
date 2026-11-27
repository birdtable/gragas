// Package legacy is the old check and gen, mounted behind the new root with
// their flags, annotations, output and exit codes as they were. It is deleted
// at M6, when the commands of the contexts replace them (M0a-20, A41).
//
// The commands draw and end through the Host the root hands them, so the
// closing words, the exit codes, --json and the run record are the root's;
// this package holds only what check and gen decide and say.
package legacy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	oldport "github.com/birdtable/gragas/internal/port/legacy"
)

// Writes is the annotation a command that can write carries: the run that
// writes is the one given --apply (cli.commands.1). The root reads it.
const Writes = "writes"

// maxShown is how many findings one run prints.
const maxShown = 80

// Ending is how a run ends: the word, the sentence beside it and the line to
// type. Said is the line to type when the sentence already says it, in
// backticks: it is the answer's next, and it is not drawn again under the word.
type Ending struct {
	Word             model.Word
	What, Next, Said string
}

// Host is what the root gives the old commands: the screen they end on, the
// lines they say, and the record of the run around them.
type Host interface {
	// Say writes one line of the answer at once; Warn reports something true
	// that did not stop the run.
	Say(line string)
	Warn(format string, a ...any)
	// End draws lead and the closing word, and returns the exit code.
	End(e Ending, lead ...model.Block) int
	// Data sets the answer's data under --json.
	Data(v any)
	// Applied tells the run it was given --apply.
	Applied(bool)
	// Change records and words one line of what was, or would be, written.
	Change(verb, path string) string
	// WayBack records how to undo what the run wrote.
	WayBack(u oldport.Undo)
	// WithRun adds the run's id to what a failed run says.
	WithRun(what string) string
	// Line is argv as the one line a person pastes into the shell the run
	// writes its lines for, or an error that names a word no such line can
	// carry: the command tree makes it, so these
	// commands quote nothing themselves.
	Line(argv []string) (string, error)
	// Exit is the error a command returns to end with code.
	Exit(code int) error
}

// Deps are what the old commands read from the world.
type Deps struct {
	Ports     oldport.Ports
	NamesFile string
	// Today is the date, YYYY-MM-DD, that check judges an exemption's
	// expiry on, asked when check runs; nil gives no date, which is a
	// finding of the file.
	Today func() string
	// Readers are the tool configs check reads.
	Readers coreapi.Readers
	// Tiers is the table of the tier contract; check reads the repository's
	// Taskfile against it.
	Tiers coreapi.TierTable
	// Audit makes the ports the check reads a tool's config through its
	// installed plugin with, for the project at a folder. Nil
	// reads by the adapter alone.
	Audit func(dir string) coreapi.AuditPlugins
}

type commands struct {
	h       Host
	d       Deps
	applied bool
	// ex is what check found of the project's exemptions file.
	ex coreapi.Exemptions
	// cf is what check found of the project's own tool configs.
	cf coreapi.Conformance
}

// Check is the check command over the host and the world it is given. It takes
// the context of the run it is executed with.
func Check(h Host, d Deps) *cobra.Command { return (&commands{h: h, d: d}).checkCmd() }

// Gen is the gen command over the host and the world it is given.
func Gen(ctx context.Context, h Host, d Deps) *cobra.Command {
	return (&commands{h: h, d: d}).genCmd(ctx)
}

// dirFlag is the one flag every command that reads a repository takes.
func dirFlag(c *cobra.Command, dir *string) {
	c.Flags().StringVarP(dir, "dir", "C", ".", "The repository holding rulebook/")
}

func (c *commands) checkCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the rulebook tree",
		Long: `Reads every file under rulebook/ and checks it: each
against its JSON Schema, then across files (ids used
once and kept in ids.lock, a retired rule's
replacement, each source, kind, facet, binding and
tool a rule names, folders that are parts), then each
count against its baseline in baseline.json, which may
only fall, then, when gate/exemptions.yaml is there,
each exception row in it (an expired row, a row for a
rule that is not active, an id listed twice and an
exit file that is not there fail), and, when
` + config.NamesFileVar + ` is set, for names public text
must not hold. It writes nothing.

  gragas check
  gragas check -C ../gragas
  gragas check --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return c.h.Exit(c.check(cmd.Context(), dir)) },
	}
	dirFlag(cmd, &dir)
	return cmd
}

func (c *commands) genCmd(ctx context.Context) *cobra.Command {
	var (
		dir          string
		apply, check bool
	)
	cmd := &cobra.Command{
		Use:   "gen",
		Short: "Regenerate the generated files",
		Long: `Makes the generated files from the rulebook's YAML
and compares them with the tree. A dry run first: it
says which files it would write. --apply writes them,
each whole, under the lock, and records the run.
--check writes nothing and fails when a file would
change, for the checks before a push.

  gragas gen
  gragas gen --apply
  gragas gen --check`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{Writes: "with --apply"},
		RunE: func(*cobra.Command, []string) error {
			if apply && check {
				return c.h.Exit(c.h.End(Ending{Word: model.Usage, What: "--apply and --check do not go together", Next: "gragas gen --check"}))
			}
			c.applied = apply
			c.h.Applied(apply)
			return c.h.Exit(c.gen(ctx, dir, check))
		},
	}
	dirFlag(cmd, &dir)
	cmd.Flags().BoolVar(&apply, "apply", false, "Write the files")
	cmd.Flags().BoolVar(&check, "check", false, "Fail when a file would change")
	return cmd
}

// load reads the rulebook in dir and says what went wrong when it could not.
// verb is the command, check or gen, and again the line that runs it again,
// as the session made it: a line to paste, or the command in words where the
// shell is given none, so nothing is read back out of it.
// lead is what the screen of this run starts with; the ending is nil when the
// rulebook loaded.
func (c *commands) load(ctx context.Context, dir, verb, again string, exemptions bool) (l coreapi.Loaded, lead []model.Block, e *Ending) {
	// Said at once, not drawn with the answer: a run that takes a while shows
	// what it is reading while it reads.
	c.h.Say("reading rulebook/ in " + dir)
	if c.d.NamesFile == "" {
		c.h.Warn("the names check is skipped: %s is not set", config.NamesFileVar)
	}
	l, err := coreapi.Load(ctx, c.d.Ports, dir, c.d.NamesFile)
	switch {
	case errors.Is(err, context.Canceled):
		return l, lead, &Ending{Word: model.Failed, What: c.h.WithRun("stopped by a signal before the rulebook was read; nothing was written. Run it again: `" + again + "`"), Said: again}
	case errors.Is(err, coreapi.ErrNoParts):
		return l, lead, &Ending{Word: model.NotReady, What: dir + " is not a rulebook repository: it has no rulebook/parts.yaml",
			Next: "gragas " + verb + " -C <the gragas repository>"}
	case errors.Is(err, coreapi.ErrNames):
		return l, lead, &Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "fix the file, or unset " + config.NamesFileVar}
	case err != nil:
		return l, lead, &Ending{Word: model.Failed, What: c.h.WithRun(err.Error())}
	}
	if l.Notice != "" {
		// A list that holds no name checks nothing: said as the skipped
		// check is, never a clean pass in silence.
		c.h.Warn("%s", l.Notice)
	}
	if exemptions {
		// A project's exceptions are checked with its rulebook, and listed
		// with its findings (gates.exemptions.4).
		c.ex = coreapi.CheckExemptions(l, c.d.Today)
		l.Findings = append(slices.Clip(l.Findings), c.ex.Findings...)
		// The tool configs the project owns are held to the bindings of the
		// parts it takes. A tool that could not be asked is
		// said on stderr: that is no finding and no pass. A tool this
		// system cannot run as held (Halt) ends the run red in the tool's
		// own words, after the findings are listed.
		l, lead = c.conformance(ctx, l, lead, dir)
	}
	stop := cmp.Or(c.haltEnding(), c.notCheckedEnding())
	if n := len(l.Findings); n > 0 {
		items := make([]string, 0, min(n, maxShown)+1)
		for i, f := range l.Findings {
			if i == maxShown {
				break
			}
			items = append(items, "error "+f)
		}
		lead = append(lead, model.List{Items: items})
		if n > maxShown {
			lead = append(lead, model.Note{Text: fmt.Sprintf("... and %s more", model.Count(n-maxShown))})
		}
		c.h.Data(c.findingsData(l))
		if stop != nil {
			return l, lead, stop // not ready whatever else was found, as the new check ends
		}
		where := "the rulebook"
		if len(c.ex.Findings) > 0 {
			where = "the rulebook and " + c.ex.File
		}
		if len(c.cf.Findings) > 0 {
			where += " and the tool configs"
		}
		return l, lead, &Ending{Word: model.Findings, What: model.Plural(n, "problem") + " in " + where + ": fix them in the YAML named above, then run `" + again + "`", Next: again}
	}
	return l, lead, stop
}

// notCheckedEnding is the ending of a check that could not judge a tool: not
// ready, in the words of the row (which tool, and how to install its plugin),
// with the Next line the new check gives, so a run never passes on a tool it
// did not check. It is nil when every tool was judged.
func (c *commands) notCheckedEnding() *Ending {
	var said []string
	for _, n := range c.cf.NotChecked {
		if !n.Partial { // a Partial row ends nothing, in both checks
			said = append(said, n.String())
		}
	}
	if len(said) == 0 {
		return nil
	}
	return &Ending{Word: model.NotReady, What: model.Plural(len(said), "tool row") + " not checked: " + strings.Join(said, "; "), Next: "gragas tools"}
}

// haltEnding is the ending of a check whose tool configs met a tool this
// system cannot run as held (coreapi.Conformance.Halt): the tool's own
// refusal, its word, what it saw and its fix, so the run never passes on
// rules it did not check. It is nil when there was none.
func (c *commands) haltEnding() *Ending {
	if c.cf.Halt == nil {
		return nil
	}
	if r, ok := coreapi.AsRefusal(c.cf.Halt); ok {
		return &Ending{Word: r.Word(), What: r.Observed(), Next: r.Hint()}
	}
	return &Ending{Word: model.Failed, What: c.h.WithRun(c.cf.Halt.Error())}
}

// findingsData is the findings as the JSON answer's data, with the
// exemptions' count when the project has the file.
func (c *commands) findingsData(l coreapi.Loaded) map[string]any {
	return c.withExemptions(map[string]any{"findings": slices.Clone(l.Findings)})
}

// withExemptions adds to data the count of the project's exemption rows, the
// soonest expiry still ahead ("" when none is) and the findings of the file
// alone, when it has the file. The object is display only: gragas-dev's
// gate holds the file itself with project.CheckExemptions and reads no
// answer of this check (docs/checks.md).
func (c *commands) withExemptions(data map[string]any) map[string]any {
	if c.ex.Present {
		data["exemptions"] = map[string]any{"rows": c.ex.Rows, "next_expiry": c.ex.NextExpiry, "findings": append([]string{}, c.ex.Findings...)}
	}
	if c.cf.Present {
		data["conformance"] = map[string]any{"parts": c.cf.Parts, "findings": c.cf.Findings, "exempt": c.cf.Exempt, "not_checked": c.cf.NotChecked}
	}
	return data
}

// report is the per-part counts as a table.
func report(counts []coreapi.Count) model.Table {
	rows := make([][]string, len(counts))
	for i, c := range counts {
		rows[i] = []string{c.Name, model.Count(c.Entries), model.Count(c.Enforced), model.Count(c.Tools)}
	}
	return model.Table{Columns: []model.Column{
		{Title: "part", Priority: 4}, {Title: "entries", Priority: 3}, {Title: "enforced", Priority: 2}, {Title: "tools", Priority: 1},
	}, Rows: rows}
}

func (c *commands) check(ctx context.Context, dir string) int {
	again, err := c.h.Line(append([]string{"gragas", "check"}, dirWords(dir)...))
	if err != nil {
		return c.h.End(Ending{Word: model.Usage, What: err.Error()})
	}
	l, lead, e := c.load(ctx, dir, "check", again, true)
	if e = c.release(l, e); e != nil {
		return c.h.End(*e, lead...)
	}
	counts := coreapi.Counts(l)
	c.h.Data(c.withExemptions(map[string]any{"entries": l.Entries, "files": l.Files, "parts": counts}))
	return c.h.End(Ending{Word: model.Done, What: fmt.Sprintf("%s entries in %s files, and no problem", model.Count(l.Entries), model.Count(l.Files))},
		append(lead, report(counts))...)
}

// genLines is gen's apply line and the line that runs this gen again: the
// apply line once applied, else the same words, so the same answer. --check
// is one of the words, never joined to a line made, which may be words.
func (c *commands) genLines(dir string, check bool) (apply, again string, err error) {
	if apply, err = c.h.Line(append([]string{"gragas", "gen", "--apply"}, dirWords(dir)...)); err != nil || c.applied {
		return apply, apply, err
	}
	words := append([]string{"gragas", "gen"}, dirWords(dir)...)
	if check {
		words = append(words, "--check")
	}
	again, err = c.h.Line(words)
	return apply, again, err
}

func (c *commands) gen(ctx context.Context, dir string, check bool) int {
	apply, again, err := c.genLines(dir, check)
	if err != nil {
		return c.h.End(Ending{Word: model.Usage, What: err.Error()})
	}
	l, lead, e := c.load(ctx, dir, "gen", again, false)
	var stale []coreapi.Output
	if e == nil {
		stale, err = coreapi.Stale(l)
	}
	if e = c.release(l, e); e != nil {
		return c.h.End(*e, lead...)
	}
	if err != nil {
		return c.h.End(Ending{Word: model.Failed, What: c.h.WithRun(err.Error())}, lead...)
	}
	paths := make([]string, len(stale))
	for i, o := range stale {
		paths[i] = o.Path
	}
	c.h.Data(map[string]any{"stale": paths})
	if len(stale) == 0 {
		return c.h.End(Ending{Word: model.Done, What: "the generated files are current"}, lead...)
	}
	if !c.applied {
		items := make([]string, len(paths))
		for i, p := range paths {
			items[i] = c.h.Change("write", p)
		}
		lead = append(lead, model.List{Items: items})
		if check {
			return c.h.End(Ending{Word: model.Findings, What: model.Plural(len(stale), "generated file") + " differ from their sources", Next: apply}, lead...)
		}
		flag := strings.TrimPrefix(apply, "gragas gen ")
		return c.h.End(Ending{Word: model.Declined, What: "nothing was written yet, the lines above are the plan", Next: apply},
			append(lead, model.DryRunFooter{Flag: flag})...)
	}
	var items []string
	err = coreapi.Apply(ctx, c.d.Ports.Writer, dir, stale, func(p string) { items = append(items, c.h.Change("write", p)) })
	if len(items) > 0 {
		lead = append(lead, model.List{Items: items})
	}
	switch {
	case errors.Is(err, oldport.ErrNotOwner) && len(items) == 0:
		// The repository is another account's, so the first write wrote
		// nothing, and nothing before it was written.
		return c.h.End(Ending{Word: model.Refused, What: c.h.WithRun(err.Error()), Next: "run it again as the account that owns that folder: " + apply}, lead...)
	case errors.Is(err, context.Canceled):
		return c.h.End(Ending{Word: model.Failed, What: c.h.WithRun("stopped by a signal; the files written above are whole, the rest are not written. Run it again: `" + apply + "`"), Said: apply}, lead...)
	case err != nil:
		return c.h.End(Ending{Word: model.Failed, What: c.h.WithRun(err.Error() + ". Run it again: `" + apply + "`"), Said: apply}, lead...)
	}
	if u, err := genWayBack(c.h, dir, paths); err != nil {
		c.h.Warn("no line to put the files back is printed: %v", err)
	} else {
		c.h.WayBack(u)
	}
	return c.h.End(Ending{Word: model.Done, What: "wrote " + model.Plural(len(stale), "generated file")}, lead...)
}

// genWayBack is how gen's written files are put back: git restore from HEAD,
// as the line h makes, or, where h gives it no line (model.ErrInWords: a
// program other than gragas in PowerShell and cmd, or a shell with no safe
// quoting), the same step in words with each path as %q.
// The run wrote either way, so it still ends done. Any other error is a value
// no line can carry.
func genWayBack(h Host, dir string, paths []string) (oldport.Undo, error) {
	const why = "the generated files are tracked: git puts back the ones before"
	back, err := h.Line(slices.Concat([]string{"git"}, dirWords(dir), []string{"restore", "--source=HEAD", "--"}, paths))
	switch {
	case errors.Is(err, model.ErrInWords):
		quoted := make([]string, len(paths))
		for i, p := range paths {
			quoted[i] = strconv.Quote(p)
		}
		in := ""
		if dir != "." && dir != "" {
			in = " in the repository at " + strconv.Quote(dir)
		}
		return oldport.Undo{Why: why + ": restore " + strings.Join(quoted, ", ") + " from HEAD with git" + in}, nil
	case err != nil:
		return oldport.Undo{}, err
	}
	return oldport.Undo{Lines: []string{back}, Why: why}, nil
}

// release lets go of the repository l was read from, once this run reads it
// no more and before the run ends: Windows refuses to remove a folder that is
// held open. It answers the ending the run comes to: e, or, when
// e is nil and the repository could not be let go of, failed. A run that
// ends on e already ends on it.
func (c *commands) release(l coreapi.Loaded, e *Ending) *Ending {
	err := l.Close()
	if e != nil || err == nil {
		return e
	}
	return &Ending{Word: model.Failed, What: c.h.WithRun("the repository could not be let go of: " + err.Error())}
}

// dirWords is -C dir, or nothing for the current folder.
func dirWords(dir string) []string {
	if dir == "." || dir == "" {
		return nil
	}
	return []string{"-C", dir}
}

// conformance holds the project's tool configs to the bindings of its parts:
// the findings join the loaded ones, a tool that could not be asked is said on
// stderr, and an exempt rule is a note of the screen's lead.
func (c *commands) conformance(ctx context.Context, l coreapi.Loaded, lead []model.Block, dir string) (coreapi.Loaded, []model.Block) {
	if c.d.Readers.Refused != "" {
		c.h.Warn("the tool configs were not checked: %s", c.d.Readers.Refused)
		return l, lead
	}
	if c.d.Tiers.Refused != "" {
		c.h.Warn("the tier contract was not checked: %s", c.d.Tiers.Refused)
	}
	var plugins coreapi.AuditPlugins
	if c.d.Audit != nil {
		plugins = c.d.Audit(dir)
	}
	c.cf = coreapi.CheckConformance(ctx, l, coreapi.Tools{Readers: c.d.Readers.Rows, Plugins: plugins, TOML: c.d.Ports.TOML.Read, Tiers: c.d.Tiers}, c.d.Today)
	for _, f := range c.cf.Findings {
		l.Findings = append(l.Findings, f.String())
	}
	for _, n := range c.cf.NotChecked {
		if n.Partial {
			// said, as the new check lists the row: one part of a tool was not
			// read and the rest was, so no tool is left unjudged
			c.h.Warn("%s", n.String())
		}
	}
	for _, e := range c.cf.Exempt {
		lead = append(lead, model.Note{Text: e.String()})
	}
	return l, lead
}
