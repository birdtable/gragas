// Package run is the seam a plan command ends through (S43, A9; architecture 5,
// step 6b of #444): the one form of the dry run, the question at a terminal and
// the apply, written over a Host the root implements. It holds no command and
// no cobra: the root says whether the command takes the flag and whether it was
// given, so a command package never reaches the root to run a plan.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// ApplyFlag is the one yes (A42): the flag that applies a plan, and the
// answer to the form's question when nobody is there to give it.
const ApplyFlag = "--apply"

// A Form is the one question a plan command asks at a terminal, with the
// flag that answers it in a script (cli.environment.14). Build it with
// NewForm.
type Form struct {
	question, flag string
}

// NewForm is the form of the command at path that asks question and is
// answered by flag. A flag the command does not take (takes false) is
// refused: a form never names an answer its command lacks.
func NewForm(takes bool, path, question, flag string) (Form, error) {
	name, ok := strings.CutPrefix(flag, "--")
	if !ok || name == "" || !takes {
		return Form{}, fmt.Errorf("the question %q names %s, which %s does not take", question, flag, path)
	}
	return Form{question: question, flag: flag}, nil
}

// Prompt is the question as it is asked, with its flag beside it.
func (f Form) Prompt() string { return f.question + " (" + f.flag + " answers it in a script)" }

// Flag is the flag that answers the question.
func (f Form) Flag() string { return f.flag }

// Changer is what the form runs: core/api's Change.
type Changer interface {
	Preview(ctx context.Context) (coreapi.Outcome, bool)
	Apply(ctx context.Context, agreed model.Digest) coreapi.Outcome
}

// A Closer is a Changer whose applied run says more than the files it wrote:
// what it left, added to the closing words, and the next line to run. ""
// says nothing more; plugin upgrade is one.
type Closer interface {
	Closing() (what, next string)
}

// presetsUnwritten is what a change that binds a tool no plugin writes a
// preset for says after it applies.
type presetsUnwritten interface{ PresetsUnwritten() []string }

// An Ending is how a run ends: its closing word, what happened and the next
// line to run.
type Ending struct {
	Word       model.Word
	What, Next string
}

// A Host is what the root gives a plan: the screen, the diagnostics, the
// terminal's question and the record of what the run wrote.
type Host interface {
	// Say writes a line before the closing screen; it writes nothing under --json.
	Say(line string)
	// Diag writes a labelled line to the diagnostics.
	Diag(label, msg string)
	// Die ends a run that cannot go on, and returns its exit code.
	Die(msg string) int
	// End draws the lead blocks and the closing word, and returns the exit code.
	End(e Ending, lead ...model.Block) int
	// Ask puts the question to the person and returns the answer.
	Ask(ctx context.Context, prompt string) (string, error)
	// SetChanges replaces the lines the answer says the run changed.
	SetChanges(lines []string)
	// AddChanges appends to them.
	AddChanges(lines []string)
	// MarkApplied says the run applied.
	MarkApplied()
}

// A Plan is one plan command's run of the form.
type Plan struct {
	// Path is the command's path, for a refusal that names it.
	Path string
	// Takes is whether the command takes --apply, and Applying whether it was given.
	Takes, Applying bool
	Change          Changer
	// Next is the command line with --apply, the line a declined run ends on.
	Next string
	// Ask is whether a person can answer, so not under --json, in CI or off a
	// terminal.
	Ask bool
}

// Run runs p's change through the form and returns the exit code:
//
//   - with --apply the change is applied, the lock taken before the plan is
//     made, so plan and apply see one tree (store.consistency.1);
//   - without it the plan is shown. When nobody can answer, nothing is asked
//     (cli.environment.8): the plan prints, then the one footer naming the
//     flag, then "declined", exit 6 (cli.look.6);
//   - at a terminal the form asks once. A yes applies the plan shown and no
//     other: the plan is made again under the lock, and one that moved while
//     the person read is shown again and asked of again. Anything else, or
//     the end of the input, is "declined".
//
// No lock is held while a person reads: the change takes it inside Apply and
// puts it back before it answers.
func Run(ctx context.Context, h Host, p Plan) int {
	f, err := NewForm(p.Takes, p.Path, "apply?", ApplyFlag)
	if err != nil {
		return h.Die(err.Error())
	}
	if w, ok := p.Change.(interface{ OnWaiting(func()) }); ok {
		w.OnWaiting(func() { h.Diag("waiting", "another gragas run holds the lock on this folder; waiting for it to end") })
	}
	if p.Applying {
		return endApplied(h, p.Change, p.Change.Apply(ctx, ""))
	}
	out, ok := p.Change.Preview(ctx)
	if !ok {
		return endOutcome(h, out)
	}
	if !p.Ask {
		h.SetChanges(wouldWrite(out.Plan))
		return h.End(Ending{Word: model.Declined, What: "nothing was changed", Next: p.Next},
			append(planBlocks(out.Plan), model.DryRunFooter{Flag: f.flag})...)
	}
	for {
		show(h, planBlocks(out.Plan))
		answer, err := h.Ask(ctx, f.Prompt())
		if err != nil && !errors.Is(err, io.EOF) {
			return h.End(Ending{Word: model.Failed, What: "the answer could not be read: " + err.Error()})
		}
		if !yes(answer) || err != nil {
			h.SetChanges(wouldWrite(out.Plan))
			return h.End(Ending{Word: model.Declined, What: "nothing was changed", Next: p.Next}, model.DryRunFooter{Flag: f.flag})
		}
		out = p.Change.Apply(ctx, out.Plan.Digest)
		if !out.Moved {
			return endApplied(h, p.Change, out)
		}
		h.Say("The plan changed while you read it, and nothing was written. This is the plan now:")
	}
}

// yes is an answer that applies: y or yes, in any case.
func yes(answer string) bool {
	a := strings.ToLower(strings.TrimSpace(answer))
	return a == "y" || a == "yes"
}

// show says the blocks of a plan as lines, before the question: a screen
// ends on its closing word, and the plan at a terminal ends on the question.
func show(h Host, blocks []model.Block) {
	for _, b := range blocks {
		switch x := b.(type) {
		case model.Heading:
			h.Say(x.Text)
		case model.List:
			for _, it := range x.Items {
				h.Say("  " + it)
			}
		}
	}
}

// planBlocks is the plan as a person reads it (15.3): each section under
// its title, then what is written where, file by file, the record last.
func planBlocks(p model.Plan) []model.Block {
	var out []model.Block
	for _, sec := range p.Sections {
		out = append(out, model.Heading{Text: sec.Title}, model.List{Items: sec.Lines})
	}
	var files []string
	for _, t := range append(p.Files, recordOf(p)...) {
		files = append(files, verbOf(t)+" "+t.Path)
	}
	if slices.ContainsFunc(p.Sections, func(s model.Section) bool { return strings.EqualFold(s.Title, "what is written where") }) {
		// the plan already says it, file by file, in its own words
		return out
	}
	return append(out, model.Heading{Text: "what is written where"}, model.List{Items: files})
}

// wouldWrite is what the plan would write, a line a file, for the answer.
func wouldWrite(p model.Plan) []string {
	var out []string
	for _, t := range append(p.Files, recordOf(p)...) {
		out = append(out, "would "+verbOf(t)+" "+t.Path)
	}
	return out
}

func recordOf(p model.Plan) []model.Target {
	if p.Record == nil {
		return nil
	}
	return []model.Target{*p.Record}
}

// verbOf says what a target does to its file.
func verbOf(t model.Target) string {
	switch {
	case t.Delete:
		return "remove"
	case t.Append:
		return "append to"
	case t.Before == "":
		return "make"
	default:
		return "replace"
	}
}

// endApplied ends a run that applied, or tried to. A file the plan removed is
// recorded as removed, not written; a preset no plugin writes ends it not
// ready, and otherwise a Closer's words follow the count.
func endApplied(h Host, ch Changer, out coreapi.Outcome) int {
	if out.Word == model.Done && !out.Nothing {
		h.MarkApplied()
		removed := map[string]bool{}
		for _, t := range out.Plan.Files {
			if t.Delete {
				removed[t.Path] = true
			}
		}
		var wrote []string
		gone := 0
		for _, p := range out.Changes {
			if removed[p] {
				wrote = append(wrote, "removed "+p)
				gone++
				continue
			}
			wrote = append(wrote, "wrote "+p)
		}
		h.AddChanges(wrote)
		// a removal is counted apart from the writes: "3 files written, 2
		// removed", never "5 files written"
		n := len(out.Changes) - gone
		what := fmt.Sprintf("%d %s written", n, fileWord(n))
		if gone > 0 {
			what += fmt.Sprintf(", %d removed", gone)
		}
		return h.End(appliedEnding(ch, what+", run "+out.Run))
	}
	return endOutcome(h, out)
}

// appliedEnding is how a run that applied ends, what being the files it
// wrote and its run: not ready when the change binds a tool no installed
// plugin writes a preset for, and otherwise done, with a Closer's words after
// the count.
func appliedEnding(ch Changer, what string) Ending {
	if u, ok := ch.(presetsUnwritten); ok {
		if tools := u.PresetsUnwritten(); len(tools) > 0 {
			// the record is written and the preset is not: not ready, as
			// preset --check ends on it
			return Ending{Word: model.NotReady, What: what + "; no installed plugin writes a preset for " + strings.Join(tools, ", "), Next: "gragas plugin add <folder>"}
		}
	}
	e := Ending{Word: model.Done, What: what}
	if cl, ok := ch.(Closer); ok {
		if said, next := cl.Closing(); said != "" {
			e.What, e.Next = e.What+"; "+said, next
		}
	}
	return e
}

// endOutcome ends a run on an outcome that wrote nothing it kept: nothing to do,
// a refusal, or a failure, which names the run and says what was put back.
func endOutcome(h Host, out coreapi.Outcome) int {
	switch {
	case out.Word == model.Done:
		return h.End(Ending{Word: model.Done, What: "nothing to do"})
	case out.Refusal != nil:
		return h.End(Ending{Word: out.Refusal.Word(), What: out.Refusal.Observed(), Next: out.Refusal.Hint()})
	}
	what := "the run failed"
	if out.Failure != nil {
		what = fmt.Sprintf("run %s failed (%s)", out.Failure.Run(), out.Failure.Kind())
		if out.RolledBack {
			what += ", and every file was put back as it was"
		}
	}
	if out.Cause != nil {
		h.Diag("error: ", out.Cause.Error())
	}
	return h.End(Ending{Word: model.Failed, What: what})
}

// fileWord is "file" or "files" for n.
func fileWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}
