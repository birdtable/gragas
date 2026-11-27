// Package apply is the write path (architecture 5, A35): the one place a
// plan becomes changes to a root's files, for every command that writes
// (arch.ports.14). It is pure, over the ports, and takes the plan function
// and the render function as arguments, so it decides nothing of rules or
// bytes.
//
// Run goes in a fixed order and never reorders it:
//
//  1. refuse a --confirm name that does not match, before anything is taken;
//  2. take the root lock, non-blocking: held is "busy" with the holder's time;
//  3. roll back every open record of the root from the journal's copies, or
//     hand it to the completion when that finishes it, and say so (5.2);
//  4. plan and render under the lock, so plan and apply see one tree, then
//     check the paths, set the goal, seal the digest and make the record of
//     the yes;
//  5. an empty plan ends "done, nothing to do" and writes nothing, and a
//     plan whose digest is not the one a person said yes to at a terminal
//     ends "refused" with the new plan, which the form shows again (A44);
//  6. journal the intent, the whole plan, before the first effect;
//  7. read every input and every target again and compare with the plan: any
//     difference ends "refused", naming the file, before the first write;
//  8. for each file, data files first and the record of the yes last: keep
//     its old content in the journal, journal the step, write it, read it
//     back and compare;
//  9. read every file of the goal again from the tree and hold it to the
//     goal: each data file's digest after, and the record of the yes as the
//     last line of its file;
//  10. close the journal with each file's digest before and after, and
//     release the lock on every path.
//
// A run that ends after its intent without its close is rolled back at once
// from the journal's copies, the same roll-back an interrupted run gets from
// the next command (A37), and ends "failed" with its run id. Nothing resumes
// a run: a run is undone whole and planned again. Ready is the check a
// reading command makes first: an open record ends it "not ready". Preview
// is the dry run: the same plan, from the same seal, with no lock and no
// write; the form shows it and Run makes it again under the lock.
//
// The residual: the root lock is advisory. A process that does not take it can
// change a file between the check (step 6) and the write (step 7), and the
// read back after the write is the only thing that sees it. The check narrows
// the window and proves what the plan was made from; it does not close it
// (A35).
package apply

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The kinds of failure a run ends on, the "kind" of the JSON document.
const (
	// FailPlan is a plan or render function that failed, or a plan that has no
	// canonical form.
	FailPlan model.Kind = "plan"
	// FailLock is a lock that could not be taken or put back, other than held.
	FailLock model.Kind = "lock"
	// FailJournal is a journal that could not be written. No effect is made
	// after it.
	FailJournal model.Kind = "journal"
	// FailRead is a file that could not be read.
	FailRead model.Kind = "read"
	// FailWrite is a file that could not be written.
	FailWrite model.Kind = "write"
	// FailVerify is a file that does not hold what was just written.
	FailVerify model.Kind = "verify"
	// FailCancelled is a run its context stopped.
	FailCancelled model.Kind = "cancelled"
	// FailGoal is a plan whose goal does not hold once its files are written
	// and read again from the tree.
	FailGoal model.Kind = "goal"
	// FailRecover is an open record that could not be rolled back or
	// completed. The record stays open, for the next command to try again.
	FailRecover model.Kind = "recover"
	// FailWiring is a call that gave Run something it cannot run.
	FailWiring model.Kind = "wiring"
)

// A NotReadyError is a plan error a command chooses to end its run on the
// closing word "not ready": the command's registry row declares it. Run maps
// it with errors.As and never by what an error says; every other plan error
// ends on failed, kind plan.
type NotReadyError struct {
	// Reason is what was seen, as the closing line says it.
	Reason string
	// Hint is the line to type; empty, it is the command again.
	Hint string
}

func (e *NotReadyError) Error() string { return "not ready: " + e.Reason }

// planFailed ends the run on what a plan or render function returned: not
// ready for a NotReadyError, failed (plan) for any other error, with msg
// saying what was being done.
func (r *run) planFailed(err error, msg string) Outcome {
	if nr, ok := errors.AsType[*NotReadyError](err); ok {
		hint := nr.Hint
		if hint == "" {
			hint = r.s.Command
		}
		return r.refusal(model.NotReady, nr.Reason, nil, hint)
	}
	return r.fail(FailPlan, fmt.Errorf("%s: %w", msg, err))
}

// PlanFunc is the context's plan function: it reads the tree, decides, and
// answers the plan without its files and digest: the command, the delta, the
// sections, the input set, the goal and, for a plan that ends in a yes, who
// and when. A clock reading is taken here, once, and carried unchanged.
type PlanFunc func(ctx context.Context) (model.Plan, error)

// RenderFunc makes the data files of a delta, in the order they are written.
type RenderFunc func(d model.Delta) ([]model.Target, error)

// RecordFunc makes the target that holds the yes, with the plan digest set in
// y. It is called once, after the digest is known.
type RecordFunc func(y model.Yes) (model.Target, error)

// A Spec is one run of the write path.
type Spec struct {
	// Command is the command as a person types it, for the journal and the
	// hint of a refusal.
	Command string
	// Engine is the engine's version, for the journal.
	Engine string
	// Run is the run id the journal files this run under. The caller made it.
	Run string
	// Scope are the path.Match patterns, relative to the root, that the
	// command writes inside (cli.commands.4). A target outside every pattern
	// is refused. No pattern is a refusal of every write.
	Scope []string
	// Name is the thing a destructive command removes: the release, the
	// rulebook, the target release, the folder. Empty for a command that is
	// reversed by a history entry and takes no confirm.
	Name string
	// Confirmed is what --confirm gave. A Name needs the same text here.
	Confirmed string
	// Plan, Render and Record are the context's functions. Record is nil for
	// a plan with no yes.
	Plan   PlanFunc
	Render RenderFunc
	Record RecordFunc
	// Agreed is the digest of the plan a person was shown and said yes to at
	// a terminal (A44). The plan is made again under the lock, and one with
	// another digest is not applied: the run ends "refused" with Moved set
	// and the new plan, which the form shows and asks of again. Empty under
	// --apply, where the plan is made under the lock and nothing was shown.
	Agreed model.Digest
	// Applies is the command line that applies this plan, with --apply: the
	// line a dry run over an open record ends on, since that run rolls it
	// back (A37). Preview needs it; Run does not read it.
	Applies string
	// TakeRoot, when set, takes the root lock in place of Take, and may wait
	// for another run to put it back (gragas#1690): the time it waits is the
	// caller's, as this package keeps none. It answers Take's errors, so a
	// lock still held ends "busy" with the same words either way. Nil takes
	// the lock at once.
	TakeRoot func(ctx context.Context, l port.Lock, root model.Root) (model.Hold, error)
}

// takeRoot takes the root lock as the spec says.
func (r *run) takeRoot(ctx context.Context) (model.Hold, error) {
	if r.s.TakeRoot != nil {
		return r.s.TakeRoot(ctx, r.p.Lock, r.root)
	}
	return r.p.Lock.Take(ctx, r.root, model.RootLock)
}

// An Outcome is how a run ended.
type Outcome struct {
	// Word is the closing word: done, busy, refused or failed, or not ready
	// from Ready and Preview. It is empty when Preview answers a plan that
	// waits for its yes.
	Word model.Word
	// Run is the run id.
	Run string
	// Nothing says a done run found nothing to change.
	Nothing bool
	// Changes are the paths written, in the order they were written. When
	// RolledBack is set they were put back as they were.
	Changes []string
	// Refusal is set with busy and refused.
	Refusal *model.Refusal
	// Failure is set with failed; Cause is the error behind it, for the log.
	Failure *model.Failure
	Cause   error
	// Plan is the sealed plan, zero when the run ended before it was made.
	Plan model.Plan
	// RolledBack says the run ended after its intent without its close and
	// was rolled back from the journal, every file as it was before.
	RolledBack bool
	// Recovered are the open records of the root this run found under the
	// lock and settled before it planned, oldest first: what it says it did.
	Recovered []Recovery
	// Moved says the plan made under the lock is not the plan of Agreed: the
	// tree changed between the show and the yes. Nothing was written, Plan is
	// the new plan, and the word is refused.
	Moved bool
}

// Run applies the plan s makes to root, in the order the package comment
// gives. It takes the root lock, non-blocking, and puts it back on every path
// (go.concurrency.3).
func Run(ctx context.Context, p port.Ports, root model.Root, s Spec) (out Outcome) {
	r := &run{ctx: ctx, jctx: context.WithoutCancel(ctx), p: p, root: root, s: s, out: Outcome{Run: s.Run}}
	if s.Plan == nil || s.Render == nil || s.Run == "" {
		return r.fail(FailWiring, errors.New("a run needs a run id, a plan function and a render function"))
	}
	if s.Name != "" && s.Confirmed != s.Name {
		return r.refuse(fmt.Sprintf("this removes %q and --confirm did not name it", s.Name), []string{s.Name},
			s.Command+" --confirm "+ShellWord(s.Name))
	}
	hold, err := r.takeRoot(ctx)
	if errors.Is(err, port.ErrHeld) {
		return r.busy(err)
	}
	if errors.Is(err, port.ErrNotOwner) {
		return r.notMine(err)
	}
	if err != nil {
		return r.fail(FailLock, err)
	}
	defer func() {
		if err := p.Lock.Release(hold); err != nil && out.Word != model.Failed {
			out = r.fail(FailLock, fmt.Errorf("the lock could not be put back: %w", err))
		}
	}()
	return r.apply()
}

// run is one Run's state.
type run struct {
	ctx context.Context
	// jctx is the context of the run's journal reads and writes: ctx, never
	// cancelled, since a run cancelled halfway still reads its records,
	// rolls back and closes its record (go.concurrency.4). The
	// run itself stops at its own checks of ctx, before the next effect.
	jctx context.Context
	p    port.Ports
	root model.Root
	s    Spec
	plan model.Plan
	out  Outcome
	// seen is what the check read: each path's content and digest. A written
	// file is copied from here, so the copy is the bytes whose digest was
	// compared.
	seen map[string]snapshot
	// files are the targets in write order, the record last.
	files []model.Target
	// open says this run journalled its intent and not yet its close; it is
	// cleared when the close is written (go.concurrency.5).
	open bool
}

// apply is Run's body once the lock is held.
func (r *run) apply() Outcome {
	if out, ok := r.recoverOpen(); !ok {
		return out
	}
	if out, ok := r.seal(); !ok {
		return out
	}
	if empty(r.plan) {
		r.out.Word, r.out.Nothing = model.Done, true
		return r.out
	}
	if r.s.Agreed != "" && r.plan.Digest != r.s.Agreed {
		return r.moved()
	}
	if err := r.journal(r.jctx, model.KindIntent, planValue(r.plan, true)); err != nil {
		return r.fail(FailJournal, err)
	}
	r.open = true
	out := r.change()
	if r.open {
		return r.undo()
	}
	return out
}

// empty is a plan with nothing to change: no delta and no file.
func empty(p model.Plan) bool { return len(p.Delta.Changes) == 0 && len(p.Files) == 0 }

// moved ends a run whose plan, made again under the lock, is not the plan a
// person said yes to (A44): the tree changed while they read. Nothing is
// written; the refusal is journalled under the run id as a close holding
// both digests, and the new plan goes back for the form to show and ask of.
func (r *run) moved() Outcome {
	detail := canon.Object{
		"outcome": canon.String("moved"),
		"agreed":  canon.String(string(r.s.Agreed)),
		"planned": canon.String(string(r.plan.Digest)),
	}
	if err := r.journal(r.jctx, model.KindClose, detail); err != nil {
		return r.fail(FailJournal, err)
	}
	r.out.Moved = true
	return r.refuse("the plan changed after it was shown: the tree moved between the plan and the yes, and nothing was written",
		nil, r.s.Command)
}

// Preview is the dry run, and the plan a terminal shows before it asks
// (architecture 5, steps 1 and 2). The open-record check comes first, as for
// every reading command (A37), then the one seal Run plans with, from the
// same functions, so over one tree the plan shown and the plan applied are
// one plan with one digest (cli.commands.17). It takes no lock, journals no
// intent and writes nothing.
//
// ok is true with the sealed plan in out.Plan, waiting for its yes. It is
// false when the run ends on out: not ready or busy over an open record,
// refused, failed, or done with nothing to do.
func Preview(ctx context.Context, p port.Ports, root model.Root, s Spec) (out Outcome, ok bool) {
	r := &run{ctx: ctx, jctx: context.WithoutCancel(ctx), p: p, root: root, s: s, out: Outcome{Run: s.Run}}
	if s.Plan == nil || s.Render == nil || s.Run == "" || s.Applies == "" {
		return r.fail(FailWiring, errors.New("a preview needs a run id, a plan function, a render function and the line that applies it")), false
	}
	if out, ok := Ready(ctx, p, root, Check{Run: s.Run, Command: s.Command, Recover: s.Applies}); !ok {
		return out, false
	}
	if out, ok := r.seal(); !ok {
		return out, false
	}
	if empty(r.plan) {
		r.out.Word, r.out.Nothing = model.Done, true
		return r.out, false
	}
	return r.out, true
}

// change checks the tree and writes the plan, once the intent is journalled.
func (r *run) change() Outcome {
	if out, ok := r.check(); !ok {
		return out
	}
	return r.write()
}

// seal plans, renders, checks the paths, sets the goal and the digest. The
// goal is the write path's to set, as the digest is: a goal the plan
// function answered is replaced. ok is false with the outcome when the run
// ends here.
func (r *run) seal() (Outcome, bool) {
	if err := r.ctx.Err(); err != nil {
		return r.fail(FailCancelled, err), false
	}
	plan, err := r.s.Plan(r.ctx)
	if err != nil {
		return r.planFailed(err, "the plan could not be made"), false
	}
	plan.Command, plan.Record, plan.Digest, plan.Goal = r.s.Command, nil, "", model.Goal{}
	if plan.Files, err = r.s.Render(plan.Delta); err != nil {
		return r.planFailed(err, "the plan could not be rendered"), false
	}
	if (plan.Yes == nil) != (r.s.Record == nil) {
		return r.fail(FailWiring, errors.New("a plan has a yes exactly when a record function is given")), false
	}
	if plan.Yes != nil {
		y := *plan.Yes
		y.Plan = ""
		plan.Yes = &y
		// The record's path and mode are fixed before the digest; its bytes
		// follow the digest.
		if err := r.record(&plan); err != nil {
			return r.fail(FailPlan, err), false
		}
	}
	if out, ok := r.vet(plan); !ok {
		return out, false
	}
	goal, out, ok := r.goal(plan.Files)
	if !ok {
		return out, false
	}
	plan.Goal = goal
	if plan.Digest, err = Digest(plan); err != nil {
		return r.fail(FailPlan, err), false
	}
	if plan.Yes != nil {
		first := *plan.Record
		plan.Yes.Plan = plan.Digest
		if err := r.record(&plan); err != nil {
			return r.fail(FailPlan, err), false
		}
		if got := *plan.Record; got.Path != first.Path || got.Before != first.Before || got.Append != first.Append {
			return r.fail(FailWiring, errors.New("the record function moved the record between its two calls")), false
		}
		// The lines that are written are this second answer's, made with the plan
		// digest set, so they are held to the checks the first answer was held to.
		if out, ok := r.linesFit(*plan.Record); !ok {
			return out, false
		}
	}
	r.plan, r.out.Plan = plan, plan
	r.files = append(slices.Clone(plan.Files), recordTargets(plan)...)
	return Outcome{}, true
}

// linesFit refuses a record whose lines badLines objects to, naming the file.
func (r *run) linesFit(t model.Target) (Outcome, bool) {
	if t.Append {
		if why := badLines(t.Data); why != "" {
			return r.refuse(fmt.Sprintf("%s: the lines to add %s", t.Path, why), nil, r.s.Command), false
		}
	}
	return Outcome{}, true
}

// record asks the record function for the target that holds p's yes.
func (r *run) record(p *model.Plan) error {
	t, err := r.s.Record(*p.Yes)
	if err != nil {
		return fmt.Errorf("the record of the yes could not be made: %w", err)
	}
	p.Record = &t
	return nil
}

func recordTargets(p model.Plan) []model.Target {
	if p.Record == nil {
		return nil
	}
	return []model.Target{*p.Record}
}

// vet refuses a plan whose paths are not safe, in scope or distinct, before
// the plan is journalled. Every refusal names the path.
func (r *run) vet(p model.Plan) (Outcome, bool) {
	targets := append(append([]model.Target(nil), p.Files...), recordTargets(p)...)
	seen := map[string]bool{}
	for _, t := range targets {
		if why := r.unsafePath(t.Path); why != "" {
			return r.refuse(fmt.Sprintf("%s: %s", t.Path, why), nil, r.s.Command), false
		}
		if !inScope(r.s.Scope, t.Path) {
			return r.refuse(fmt.Sprintf("%s is outside the files this command writes", t.Path), r.s.Scope, r.s.Command), false
		}
		if seen[t.Path] {
			return r.refuse(fmt.Sprintf("%s is written twice by one plan", t.Path), nil, r.s.Command), false
		}
		seen[t.Path] = true
		if t.Delete && (t.Append || len(t.Data) > 0 || t.Before == "") {
			return r.refuse(fmt.Sprintf("%s: a delete holds no data, adds no line and names the digest of the file it takes away", t.Path), nil, r.s.Command), false
		}
		if t.Append {
			if why := badLines(t.Data); why != "" {
				return r.refuse(fmt.Sprintf("%s: the lines to add %s", t.Path, why), nil, r.s.Command), false
			}
		}
	}
	for _, in := range p.Inputs {
		if why := r.unsafePath(in.Path); why != "" {
			return r.refuse(fmt.Sprintf("%s: %s", in.Path, why), nil, r.s.Command), false
		}
	}
	return Outcome{}, true
}

// unsafePath says why a path is not one a plan may name, or "". The plan's own
// rule is the shape: a path is slash-separated, relative, clean and inside the
// root. What a disk does with each name in it is the port's answer, not a table
// of this package's: a name no system holds, an end Windows strips, a code
// point a Mac disk ignores, and the repository's own folder in any case, .git
// and its short name, which no scope may reach.
func (r *run) unsafePath(p string) string { return UnsafePath(r.p.FileNames, p) }

// UnsafePath is unsafePath for a caller that holds no run: a path from a record
// the journal holds is judged by the same rule before anything reads or writes
// it.
func UnsafePath(names port.FileNames, p string) string {
	switch {
	case p == "":
		return "a file is named by a path"
	case path.IsAbs(p) || path.Clean(p) != p || p == "." || p == ".." || strings.HasPrefix(p, "../"):
		return "a path is clean, relative and stays inside the root"
	}
	for seg := range strings.SplitSeq(p, "/") {
		if _, err := names.Portable(seg); err != nil {
			return err.Error()
		}
	}
	return ""
}

// ShellWord is s as one word of a POSIX shell: as it stands when it holds only
// characters a shell reads as themselves, else in single quotes with each
// single quote closed, escaped and reopened. The hint of a refusal is a line a
// person pastes, and a name is data (a release, a folder).
func ShellWord(s string) string {
	safe := s != ""
	for _, r := range s {
		if !plainRune(r) {
			safe = false
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// plainRune is a character a POSIX shell reads as itself in a word.
func plainRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_@%+=:,./-", r)
}

func inScope(scope []string, p string) bool {
	for _, pat := range scope {
		if ok, err := path.Match(pat, p); err == nil && ok {
			return true
		}
	}
	return false
}

// A snapshot is one file as the check read it.
type snapshot struct {
	data   []byte
	digest model.Digest
	exists bool
}

// read reads p once, under the lock, and remembers it.
func (r *run) read(p string) (snapshot, error) {
	if s, ok := r.seen[p]; ok {
		return s, nil
	}
	data, err := r.p.Tree.Read(r.root, p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s := snapshot{}
		r.remember(p, s)
		return s, nil
	case err != nil:
		return snapshot{}, err
	}
	s := snapshot{data: data, digest: DigestBytes(data), exists: true}
	r.remember(p, s)
	return s, nil
}

func (r *run) remember(p string, s snapshot) {
	if r.seen == nil {
		r.seen = map[string]snapshot{}
	}
	r.seen[p] = s
}

// check reads every input and every target again and compares: a plan made
// from a tree that moved is refused, naming the file, before any write
// (cli.commands.22). The journal is closed as refused, so no record stays
// open for a run that wrote nothing.
func (r *run) check() (Outcome, bool) {
	type want struct {
		path   string
		digest model.Digest
	}
	var wants []want
	for _, in := range r.plan.Inputs {
		wants = append(wants, want{in.Path, in.Digest})
	}
	for _, t := range r.files {
		wants = append(wants, want{t.Path, t.Before})
	}
	for _, w := range wants {
		got, err := r.read(w.path)
		if errors.Is(err, port.ErrOutside) {
			return r.refuseAfterIntent(fmt.Sprintf("%s leaves the root: %v", w.path, err), w.path), false
		}
		if err != nil {
			return r.fail(FailRead, fmt.Errorf("%s could not be read: %w", w.path, err)), false
		}
		if got.digest != w.digest {
			return r.refuseAfterIntent(fmt.Sprintf("%s changed since the plan was made", w.path), w.path), false
		}
	}
	return Outcome{}, true
}

// badLines says what is wrong with the lines a plan appends, or "" when they
// are fit: one line holds no newline, nor a carriage return; several, joined
// by a newline, hold none empty, and each is held to the history record's own
// shape, a JSON object with the keys of a record, so a wider append cannot
// carry a line that is not one.
func badLines(data []byte) string {
	// the keys every line of history.jsonl carries, the schema's required ones
	historyKeys := []string{"format", "number", "kind", "entity", "after", "who", "when", "plan"}
	if strings.ContainsRune(string(data), '\r') {
		return "hold a carriage return"
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 1 {
		return ""
	}
	for i, l := range lines {
		v, err := canon.Decode([]byte(l))
		o, ok := v.(canon.Object)
		if err != nil || !ok {
			return fmt.Sprintf("hold a line (%d of %d) that is not a history record", i+1, len(lines))
		}
		for _, k := range historyKeys {
			if _, has := o[k]; !has {
				return fmt.Sprintf("hold a line (%d of %d) that is not a history record: it has no %s", i+1, len(lines), k)
			}
		}
	}
	return ""
}
