package apply

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/birdtable/gragas/internal/apply/jwrite"
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// journal appends one record of this run, before the effect it announces,
// under ctx, which no cancel reaches (the run's jctx).
func (r *run) journal(ctx context.Context, kind string, detail canon.Value) error {
	return r.journalAs(ctx, Interrupted{Run: r.s.Run, Command: r.s.Command, Plan: string(r.plan.Digest)}, kind, detail)
}

// journalAs appends one record of the run o, with this engine's version: a
// roll-back writes its steps and its close under the run it undoes, so that
// run's record is what closes.
func (r *run) journalAs(ctx context.Context, o Interrupted, kind string, detail canon.Value) error {
	return r.journalTo(ctx, jwrite.Run{ID: o.Run, Command: o.Command, Plan: o.Plan, Engine: r.s.Engine}, kind, detail)
}

// journalTo appends one record of run, its detail in canonical form.
func (r *run) journalTo(ctx context.Context, run jwrite.Run, kind string, detail canon.Value) error {
	b, err := canon.Encode(detail, canon.Canonical)
	if err != nil {
		return fmt.Errorf("the %s record has no canonical form: %w", kind, err)
	}
	rec := model.Record{
		Run: run.ID, Kind: kind, Command: run.Command, Plan: run.Plan,
		Engine: run.Engine, Detail: b,
	}
	if err := r.p.Journal.Append(ctx, r.root, rec); err != nil {
		return fmt.Errorf("the journal could not be written: %w", err)
	}
	return nil
}

// A written file, for the close record.
type written struct {
	path          string
	before, after model.Digest
}

// write makes each file's effect in order, the record of the yes last
// (architecture 5, steps 6 to 8), then holds the tree to the goal. Any
// failure leaves the journal open, for apply to roll back.
func (r *run) write() Outcome {
	var done []written
	for _, t := range r.files {
		if err := r.ctx.Err(); err != nil {
			return r.fail(FailCancelled, err)
		}
		w, kind, err := r.one(t)
		if errors.Is(err, jwrite.ErrNotOwner) && len(done) == 0 {
			// The root is another account's, so the first replace wrote
			// nothing, and nothing before it was written: the journal closes
			// as refused, as a check that refuses does.
			if cerr := r.close(closeValue("refused", t.Path, nil)); cerr != nil {
				return r.fail(FailJournal, cerr)
			}
			return r.notMine(err)
		}
		if err != nil {
			return r.fail(kind, err)
		}
		done = append(done, w)
		r.out.Changes = append(r.out.Changes, t.Path)
	}
	if err := r.holds(); err != nil {
		return r.fail(FailGoal, err)
	}
	if err := r.close(closeValue("done", "", done)); err != nil {
		return r.fail(FailJournal, err)
	}
	r.out.Word = model.Done
	return r.out
}

// one writes a file through the journalled write, which names none of the
// plan, the canonical form or the ports: this binds them to it.
func (r *run) one(t model.Target) (written, model.Kind, error) {
	old := r.seen[t.Path] // read and compared by the check, under this lock
	run := jwrite.Run{ID: r.s.Run, Command: r.s.Command, Plan: string(r.plan.Digest), Engine: r.s.Engine}
	w, stage, err := jwrite.One(seamJournal{r, run}, seamFiles{r}, run,
		jwrite.File{Path: t.Path, Data: t.Data, Append: t.Append, Delete: t.Delete},
		jwrite.Seen{Data: old.data, Exists: old.exists, Digest: string(old.digest)})
	if err != nil {
		return written{}, failKind(stage), err
	}
	return written{path: w.Path, before: model.Digest(w.Before), after: model.Digest(w.After)}, "", nil
}

// failKind is the kind of failure a stage of the journalled write ends a run on.
func failKind(s jwrite.Stage) model.Kind {
	switch s {
	case jwrite.StageJournal:
		return FailJournal
	case jwrite.StageWrite:
		return FailWrite
	case jwrite.StageRead:
		return FailRead
	case jwrite.StageVerify:
		return FailVerify
	}
	return FailWiring
}

// seamJournal is the run's journal as the journalled write sees it.
type seamJournal struct {
	r   *run
	run jwrite.Run
}

func (j seamJournal) Append(rec jwrite.Record) error {
	return j.r.journalTo(j.r.jctx, jwrite.Run{ID: rec.Run, Command: rec.Command, Plan: rec.Plan, Engine: rec.Engine}, rec.Kind, toCanon(rec.Detail))
}

func (j seamJournal) Keep(run, path string, old []byte) error {
	return j.r.p.Journal.Keep(j.r.root, run, path, old)
}

func (j seamJournal) Kept(run, path string) ([]byte, error) {
	return j.r.p.Journal.Kept(j.r.root, run, path)
}

func (j seamJournal) KeptAt(run, path string) (string, error) {
	return j.r.p.Journal.KeptAt(j.r.root, run, path)
}

// Decode is a record's detail in the journalled write's plain types: the
// canonical form decoded, and each value but a string, a bool, a list, an
// object or null kept as the canonical value it is.
func (seamJournal) Decode(detail []byte) (any, error) {
	v, err := canon.Decode(detail)
	if err != nil {
		return nil, err
	}
	return fromCanon(v), nil
}

// fromCanon is a canonical value in the journalled write's plain types.
func fromCanon(v canon.Value) any {
	switch v := v.(type) {
	case canon.String:
		return string(v)
	case canon.Bool:
		return bool(v)
	case canon.Null:
		return nil
	case canon.Array:
		a := make([]any, 0, len(v))
		for _, e := range v {
			a = append(a, fromCanon(e))
		}
		return a
	case canon.Object:
		m := make(map[string]any, len(v))
		for k, e := range v {
			m[k] = fromCanon(e)
		}
		return m
	}
	return v // a number: no roll back reads one, and none passes for a string
}

// seamFiles is the run's root as the journalled write sees it.
type seamFiles struct{ r *run }

func (f seamFiles) Replace(path string, data []byte) error {
	return notOwner(f.r.p.Writer.Replace(f.r.root, path, data))
}

func (f seamFiles) Remove(path string) error {
	return notOwner(f.r.p.Writer.Remove(f.r.root, path))
}

func (f seamFiles) AppendLine(path string, line []byte) error {
	return notOwner(f.r.p.Writer.Append(f.r.root, path, line))
}

func (f seamFiles) Read(path string) ([]byte, error) {
	b, err := f.r.p.Tree.Read(f.r.root, path)
	if errors.Is(err, fs.ErrNotExist) {
		return b, absentError{err}
	}
	return b, err
}

// absentError is an error of a read that keeps its words and also matches
// jwrite.ErrAbsent.
type absentError struct{ err error }

func (e absentError) Error() string        { return e.err.Error() }
func (e absentError) Unwrap() error        { return e.err }
func (e absentError) Is(target error) bool { return target == jwrite.ErrAbsent }

// toCanon is a detail of the journalled write as a canonical value.
func toCanon(v any) canon.Value {
	switch v := v.(type) {
	case string:
		return canon.String(v)
	case bool:
		return canon.Bool(v)
	case []any:
		a := canon.Array{}
		for _, e := range v {
			a = append(a, toCanon(e))
		}
		return a
	case map[string]any:
		return toObject(v)
	}
	return canon.Null{} // a detail holds nothing else: the seam's test says so
}

// toObject is a detail of the journalled write as a canonical object.
func toObject(m map[string]any) canon.Object {
	o := canon.Object{}
	for k, e := range m {
		o[k] = toCanon(e)
	}
	return o
}

// expected is what the file holds once t is written to old.
func expected(old []byte, t model.Target) []byte {
	return jwrite.Expected(old, jwrite.File{Path: t.Path, Data: t.Data, Append: t.Append, Delete: t.Delete})
}

// closeValue is the detail of a close record: how the run ended, the file it
// refused on when it did, and each written file's digest before and after
// (cli.commands.25).
func closeValue(outcome, file string, files []written) canon.Object {
	ws := []jwrite.Written{}
	for _, w := range files {
		ws = append(ws, jwrite.Written{Path: w.path, Before: string(w.before), After: string(w.after)})
	}
	return toObject(jwrite.CloseDetail(outcome, file, ws))
}

// close journals this run's close and clears its open mark: the record is
// closed once the line is written (go.concurrency.5).
func (r *run) close(detail canon.Object) error {
	if err := r.journal(r.jctx, model.KindClose, detail); err != nil {
		return err
	}
	r.open = false
	return nil
}

// refuseAfterIntent ends the run "refused", having journalled the intent:
// the journal is closed as refused, with no file written, so no open record
// is left for a run that changed nothing.
func (r *run) refuseAfterIntent(observed, file string) Outcome {
	if err := r.close(closeValue("refused", file, nil)); err != nil {
		return r.fail(FailJournal, err)
	}
	return r.refuse(observed, nil, r.s.Command)
}

// refuse ends the run "refused": the tree is as it was.
func (r *run) refuse(observed string, valid []string, hint string) Outcome {
	return r.refusal(model.Refused, observed, valid, hint)
}

// notMine ends the run "refused" on a folder another account owns, the root
// or the state folder, which the lock and the replace write in by its path:
// err says which folder and whose it is.
func (r *run) notMine(err error) Outcome {
	return r.refuse(err.Error(), nil, "type "+r.s.Command+" again as the account that owns that folder")
}

// busy ends the run "busy", with what the lock said of its holder.
func (r *run) busy(held error) Outcome {
	return r.refusal(model.Busy, held.Error(), nil, "wait for the other run to end, then type "+r.s.Command+" again")
}

func (r *run) refusal(w model.Word, observed string, valid []string, hint string) Outcome {
	rf, err := model.NewRefusal(w, observed, valid, hintOr(hint))
	if err != nil {
		return r.fail(FailWiring, err)
	}
	r.out.Word, r.out.Refusal = w, &rf
	return r.out
}

// hintOr is hint, or a line to type when the command gave none: the
// constructor of a refusal wants one (cli.errors.2).
func hintOr(hint string) string {
	if hint == "" {
		return "gragas help"
	}
	return hint
}

// fail ends the run "failed", with its kind and the run id.
func (r *run) fail(kind model.Kind, cause error) Outcome {
	if kind == "" {
		kind = FailWiring
	}
	id := r.s.Run
	if id == "" {
		id = "unknown"
	}
	f, err := model.NewFailure(kind, id)
	if err != nil {
		cause = errors.Join(cause, err)
	}
	r.out.Word, r.out.Failure, r.out.Cause = model.Failed, &f, cause
	return r.out
}

// ownerError is an error of a write that keeps its words and also matches
// jwrite.ErrNotOwner.
type ownerError struct{ err error }

func (e ownerError) Error() string        { return e.err.Error() }
func (e ownerError) Unwrap() error        { return e.err }
func (e ownerError) Is(target error) bool { return target == jwrite.ErrNotOwner }

// notOwner marks an error of the port that means another account owns the
// folder, for the journalled write, which knows no port.
func notOwner(err error) error {
	if errors.Is(err, port.ErrNotOwner) {
		return ownerError{err}
	}
	return err
}
