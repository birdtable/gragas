package apply_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
)

// yesPlan is a plan with a yes over the fixture: a rule retired, a line
// appended, and identity.yaml read as an input, so a change to it moves the
// plan without changing what it writes.
func (f *fixture) yesPlan() testPlan {
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}, edit{path: "state/waive.yaml", data: "waivers: [src/gen]\n"})
	tp.inputs, tp.yes = []string{"identity.yaml"}, true
	return tp
}

// preview is the dry run of s over the fixture.
func (f *fixture) preview(s apply.Spec) (apply.Outcome, bool) {
	f.t.Helper()
	if s.Applies == "" {
		s.Applies = "gragas test --apply"
	}
	return apply.Preview(f.t.Context(), f.p, f.root, s)
}

// TestThePreviewAndTheAppliedPlanAreOnePlan: the dry run and the run under
// the lock make the plan with the one seal, so over one tree they answer one
// plan, field for field, with one digest, though the clock moved between
// them: no reading of it but the plan function's own is in the plan. The
// dry run takes no lock, journals nothing and writes nothing. Risk: a dry
// run whose plan is not the plan applied (cli.commands.17, cli.commands.1).
func TestThePreviewAndTheAppliedPlanAreOnePlan(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.yesPlan()
	before := f.snapshot(fixturePaths()...)

	shown, ok := f.preview(tp.spec())
	if !ok || shown.Word != "" || shown.Plan.Digest == "" {
		t.Fatalf("the preview ended %q (refusal %v, failure %v, cause %v), want a plan waiting for its yes", shown.Word, shown.Refusal, shown.Failure, shown.Cause)
	}
	for _, call := range f.spy.log {
		if !strings.HasPrefix(call, "read ") {
			t.Errorf("the preview called %q; a dry run only reads", call)
		}
	}
	if after := f.snapshot(fixturePaths()...); !reflect.DeepEqual(after, before) || len(f.w.Writes) != 0 || len(f.records()) != 0 {
		t.Errorf("the preview wrote %d files and %d records", len(f.w.Writes), len(f.records()))
	}

	f.w.Instant(instant().Add(90 * 60e9))
	again, ok := f.preview(tp.spec())
	if !ok || again.Plan.Digest != shown.Plan.Digest {
		t.Errorf("a second preview over the same tree has digest %s, want %s", again.Plan.Digest, shown.Plan.Digest)
	}

	s := tp.spec()
	s.Agreed = shown.Plan.Digest
	applied := f.run(s)
	wantDone(t, applied)
	if !reflect.DeepEqual(applied.Plan, shown.Plan) {
		t.Errorf("the applied plan\n %+v\nwant the plan shown\n %+v", applied.Plan, shown.Plan)
	}
}

// TestAPlanThatMovedAfterTheYesIsNeverApplied: a yes given to a plan, then an
// input changed before the run takes the lock. The run makes the plan again
// under the lock, finds another digest and ends refused with Moved and the
// new plan, having written nothing: the refusal is journalled under its run
// id as a close with both digests, and no record is left open. The new plan,
// agreed to in its turn under a new run id, is applied. Risk: the plan a
// person did not see applied on their yes (A44, cli.commands.22).
func TestAPlanThatMovedAfterTheYesIsNeverApplied(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.yesPlan()
	shown, ok := f.preview(tp.spec())
	if !ok {
		t.Fatalf("the preview ended %q, cause %v", shown.Word, shown.Cause)
	}
	f.change("identity.yaml", "name: renamed\n")
	before := f.snapshot(fixturePaths()...)

	s := tp.spec()
	s.Agreed = shown.Plan.Digest
	moved := f.run(s)

	if moved.Word != model.Refused || !moved.Moved || moved.Refusal == nil {
		t.Fatalf("ended %q, moved %v (refusal %v, failure %v, cause %v), want refused and moved", moved.Word, moved.Moved, moved.Refusal, moved.Failure, moved.Cause)
	}
	if moved.Plan.Digest == "" || moved.Plan.Digest == shown.Plan.Digest {
		t.Errorf("the plan answered has digest %q, want the new plan's, not %s", moved.Plan.Digest, shown.Plan.Digest)
	}
	if after := f.snapshot(fixturePaths()...); !reflect.DeepEqual(after, before) || len(f.w.Writes) != 1 {
		t.Errorf("the moved run wrote %d files", len(f.w.Writes)-1)
	}
	if got := kinds(f.records()); !slices.Equal(got, []string{model.KindClose}) {
		t.Errorf("the journal holds %q, want the refusal's close alone", got)
	}
	c := f.closeOf(runID)
	if c["outcome"] != canon.String("moved") || c["agreed"] != canon.String(string(shown.Plan.Digest)) ||
		c["planned"] != canon.String(string(moved.Plan.Digest)) {
		t.Errorf("the close says %v, want moved with both digests", c)
	}
	if open := f.open(); len(open) != 0 {
		t.Errorf("open records %q after a refusal", open)
	}
	if !strings.HasPrefix(f.spy.log[0], "take ") {
		t.Errorf("the run began with %q, want the lock before the plan", f.spy.log[0])
	}

	t.Run("the new plan, agreed to, is applied", func(t *testing.T) {
		t.Parallel()
		s := tp.spec()
		s.Run, s.Agreed = "run-2", moved.Plan.Digest
		wantDone(t, f.run(s))
		if got := f.content("rules/a.yaml"); got != "status: retired\n" {
			t.Errorf("rules/a.yaml holds %q", got)
		}
	})
}

// TestARunIdTheJournalHoldsIsNeverUsedAgain: a run id names one run. A
// second run under the id of one that closed is refused before it plans,
// since its records, filed after that close, would never read as open.
// Risk: a crash of the second run that the next command cannot see
// (A37, store.consistency.4).
func TestARunIdTheJournalHoldsIsNeverUsedAgain(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.yesPlan()
	shown, _ := f.preview(tp.spec())
	f.change("identity.yaml", "name: renamed\n")
	s := tp.spec()
	s.Agreed = shown.Plan.Digest
	if o := f.run(s); !o.Moved {
		t.Fatalf("the first run ended %q, want moved", o.Word)
	}
	records := len(f.records())

	s.Agreed = ""
	o := f.run(s)

	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailWiring {
		t.Fatalf("ended %q %+v, want failed, wiring", o.Word, o.Failure)
	}
	if len(f.w.Writes) != 1 || len(f.records()) != records {
		t.Errorf("the run wrote %d files and %d records", len(f.w.Writes)-1, len(f.records())-records)
	}
}

// TestUnderApplyTheLockComesBeforeThePlanAndAnotherRunIsBusy: with --apply
// one process plans under the root lock, so while its plan function runs a
// second run, a competing process, ends busy without planning, and the
// first goes on to done. Risk: two runs planning over one tree, each
// applying a plan the other's write made stale (store.consistency.1, A44).
func TestUnderApplyTheLockComesBeforeThePlanAndAnotherRunIsBusy(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.yesPlan()
	var competitor apply.Outcome
	planned := 0
	tp.moved = func() {
		if planned++; planned > 1 {
			t.Error("the competing run planned")
			return
		}
		other := f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"})
		s := other.spec()
		s.Run = "run-2"
		other.moved = func() { planned++ }
		s.Plan = other.plan
		competitor = f.run(s)
	}

	o := f.run(tp.spec())

	wantDone(t, o)
	if competitor.Word != model.Busy || competitor.Refusal == nil {
		t.Errorf("the competing run ended %q (refusal %v, cause %v), want busy", competitor.Word, competitor.Refusal, competitor.Cause)
	}
	if got := f.content("rules/b.yaml"); got != "status: active\n" {
		t.Errorf("rules/b.yaml holds %q: the busy run wrote", got)
	}
	first := slices.IndexFunc(f.spy.log, func(c string) bool { return strings.HasPrefix(c, "read ") })
	if first < 0 || !slices.Contains(f.spy.log[:first], "take root") {
		t.Errorf("calls %q, want the lock taken before the plan read the tree", f.spy.log)
	}
}

// TestAPreviewOverAnOpenRecordEndsNotReadyOnTheLineThatApplies: a dry run is
// a reading command, so an open record ends it not ready before it plans,
// and the line it ends on is the apply, which rolls the record back. The
// control is a preview over a closed journal, which plans. Risk: a plan
// shown from half-written files (A37).
func TestAPreviewOverAnOpenRecordEndsNotReadyOnTheLineThatApplies(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.interrupt("run-0", 1, f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec())
	s := f.yesPlan().spec()
	s.Applies = "gragas test now --apply"

	o, ok := f.preview(s)

	if ok || o.Word != model.NotReady || o.Refusal == nil || o.Refusal.Hint() != "gragas test now --apply" {
		t.Fatalf("ended %q ok %v (refusal %+v, cause %v), want not ready on the apply line", o.Word, ok, o.Refusal, o.Cause)
	}
	if o.Plan.Digest != "" {
		t.Errorf("a plan was made over the open record: %s", o.Plan.Digest)
	}

	t.Run("the control, with no open record, plans", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		if o, ok := f.preview(f.yesPlan().spec()); !ok || o.Plan.Digest == "" {
			t.Errorf("ended %q ok %v, want a plan", o.Word, ok)
		}
	})
}

// TestAPreviewWithNothingToChangeIsDoneAndOneWithoutItsWiringFails: the
// two ends of a preview that is no plan to ask of.
func TestAPreviewWithNothingToChangeIsDoneAndOneWithoutItsWiringFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	o, ok := f.preview(f.plan(edit{path: "rules/a.yaml", data: "status: active\n"}).spec())
	if ok || o.Word != model.Done || !o.Nothing {
		t.Errorf("ended %q ok %v nothing %v, want done, nothing to do", o.Word, ok, o.Nothing)
	}
	s := f.yesPlan().spec()
	o, ok = apply.Preview(t.Context(), f.p, f.root, s)
	if ok || o.Word != model.Failed || o.Failure.Kind() != apply.FailWiring {
		t.Errorf("a preview with no apply line ended %q ok %v, want failed, wiring", o.Word, ok)
	}
}

// change is another process replacing the file at p with data.
func (f *fixture) change(p, data string) {
	f.t.Helper()
	if err := f.inner().Writer.Replace(f.root, p, []byte(data)); err != nil {
		f.t.Fatal(err)
	}
}

// remove is another process taking away the file at p.
func (f *fixture) remove(p string) {
	f.t.Helper()
	if err := f.inner().Writer.Remove(f.root, p); err != nil {
		f.t.Fatal(err)
	}
}

// TestAYesTwoSecondsAfterThePreviewOnAnUnchangedTreeIsApplied: the time of the
// yes is not part of the plan digest, so a person who reads for a while and then
// says yes gets the plan they read written, not "the plan changed". Risk: a
// digest that moves with the clock, so every terminal yes is
// refused as moved.
func TestAYesTwoSecondsAfterThePreviewOnAnUnchangedTreeIsApplied(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.yesPlan()
	shown, ok := f.preview(tp.spec())
	if !ok {
		t.Fatalf("the preview ended %q, cause %v", shown.Word, shown.Cause)
	}
	later := f.yesPlan()
	later.at = func() time.Time { return instant().Add(2 * time.Second) }
	s := later.spec()
	s.Agreed = shown.Plan.Digest
	o := f.run(s)
	if o.Moved {
		t.Fatalf("a yes two seconds later moved the plan: %q, %v", o.Word, o.Cause)
	}
	wantDone(t, o)
}
