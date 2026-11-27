package apply_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TestTwoDataFilesAndARecordAreWrittenDataFirstRecordLast: the calls of one
// run, in order, from the lock to its release. Each file's old content is
// kept and journalled before it is replaced, the intent is the first record,
// every digest is read before the first write, the record of the yes is the
// last file, and every file is read again for the goal before the close. Risk: a write path that reorders these, which is the
// crash-safety it exists for.
func TestTwoDataFilesAndARecordAreWrittenDataFirstRecordLast(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}, edit{path: "rules/b.yaml", data: "status: trial\n"})
	tp.yes, tp.inputs = true, []string{"identity.yaml"}

	o := f.run(tp.spec())

	wantDone(t, o)
	want := []string{
		"take root",
		"journal intent",
		"read identity.yaml", "read rules/a.yaml", "read rules/b.yaml", "read history.jsonl",
		"keep rules/a.yaml", "journal copy", "journal step", "replace rules/a.yaml", "read rules/a.yaml",
		"keep rules/b.yaml", "journal copy", "journal step", "replace rules/b.yaml", "read rules/b.yaml",
		"keep history.jsonl", "journal copy", "journal step", "append history.jsonl", "read history.jsonl",
		"read rules/a.yaml", "read rules/b.yaml", "read history.jsonl",
		"journal close",
		"release root",
	}
	if !slices.Equal(f.spy.log, want) {
		t.Errorf("calls:\n got %q\nwant %q", f.spy.log, want)
	}
	if got := f.content("rules/a.yaml") + f.content("rules/b.yaml"); got != "status: retired\nstatus: trial\n" {
		t.Errorf("data files hold %q", got)
	}
	if !slices.Equal(o.Changes, []string{"rules/a.yaml", "rules/b.yaml", "history.jsonl"}) {
		t.Errorf("changes %q", o.Changes)
	}
}

// TestTheJournalKeepsEachOldFileAndClosesWithItsDigests: the copy of what was
// replaced is the old bytes, the record of the yes carries the digest of the
// plan the run applied, and the close record is the last. Risk: a copy of the
// new bytes, or a yes with another plan's digest (cli.commands.25).
func TestTheJournalKeepsEachOldFileAndClosesWithItsDigests(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"})
	tp.yes = true

	o := f.run(tp.spec())

	wantDone(t, o)
	for p, want := range map[string]string{"rules/a.yaml": "status: active\n", "history.jsonl": `{"n":1}` + "\n"} {
		got, err := f.inner().Journal.Kept(f.root, runID, p)
		if err != nil || string(got) != want {
			t.Errorf("kept %s = %q, %v; want %q", p, got, err, want)
		}
	}
	if o.Plan.Digest == "" || o.Plan.Yes == nil || o.Plan.Yes.Plan != o.Plan.Digest {
		t.Fatalf("the sealed plan %+v does not carry its digest in its yes", o.Plan)
	}
	lines := strings.Split(strings.TrimSuffix(f.content("history.jsonl"), "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], string(o.Plan.Digest)) {
		t.Errorf("history.jsonl = %q, want the old line then a yes naming %s", lines, o.Plan.Digest)
	}
	recs := f.records()
	if got, want := kinds(recs), []string{"intent", "copy", "step", "copy", "step", "close"}; !slices.Equal(got, want) {
		t.Fatalf("record kinds %q, want %q", got, want)
	}
	for _, r := range recs {
		if r.Run != runID || r.Plan != string(o.Plan.Digest) || r.Command != "gragas test --apply" || r.Engine != "v0.0.0-test" {
			t.Errorf("record %+v does not carry the fixed fields", r)
		}
	}
	closing := string(recs[len(recs)-1].Detail)
	for _, want := range []string{`"outcome":"done"`, `"path":"rules/a.yaml"`, `"path":"history.jsonl"`, string(apply.DigestBytes([]byte("status: retired\n")))} {
		if !strings.Contains(closing, want) {
			t.Errorf("close %s lacks %s", closing, want)
		}
	}
	if !strings.Contains(string(recs[0].Detail), "7374617475733a20726574697265640a") { // "status: retired\n" in hex
		t.Errorf("the intent %s does not hold the whole plan, new bytes included", recs[0].Detail)
	}
}

// TestATreeThatMovedBetweenPlanAndLockIsRefusedBeforeTheFirstWrite: the plan
// read a file, the tree changed, and the run ends refused naming that file
// with nothing written. A target that moved is refused the same way. Risk:
// writing a result made from stale inputs (cli.commands.22, A44).
func TestATreeThatMovedBetweenPlanAndLockIsRefusedBeforeTheFirstWrite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		input bool
		moved string
	}{
		{"an input the plan only read", true, "identity.yaml"},
		{"a target the plan writes", false, "rules/a.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}, edit{path: "rules/b.yaml", data: "status: trial\n"})
			tp.inputs = []string{"identity.yaml"}
			tp.moved = func() {
				cur, _ := f.w.File(f.root, tc.moved)
				f.w.Seed(string(f.root), map[string]string{
					"identity.yaml": "name: demo\n", "rules/a.yaml": "status: active\n", "rules/b.yaml": "status: active\n",
					tc.moved: cur + "# edited by hand\n",
				})
			}

			o := f.run(tp.spec())

			if o.Word != model.Refused || o.Refusal == nil || !strings.Contains(o.Refusal.Observed(), tc.moved) {
				t.Fatalf("ended %q %+v, want refused naming %s", o.Word, o.Refusal, tc.moved)
			}
			if w := f.w.Writes; len(w) != 0 {
				t.Errorf("wrote %v before the refusal", w)
			}
			if got, want := kinds(f.records()), []string{"intent", "close"}; !slices.Equal(got, want) {
				t.Errorf("records %q, want %q: the intent and a close that says refused", got, want)
			}
			if c := string(f.records()[1].Detail); !strings.Contains(c, `"outcome":"refused"`) || !strings.Contains(c, tc.moved) {
				t.Errorf("close %s", c)
			}
			if tail := f.spy.log[len(f.spy.log)-1]; tail != "release root" {
				t.Errorf("last call %q: the lock was not put back", tail)
			}
		})
	}
}

// TestAHeldLockEndsBusyWithTheHoldersTime: another process holds the root
// lock since a time; the run ends busy, says when, and calls nothing else.
// Risk: a second writer on one root (cli.commands.3, store.consistency.1).
func TestAHeldLockEndsBusyWithTheHoldersTime(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.w.HoldLock(f.root, model.RootLock, 1794132000)
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"})

	o := f.run(tp.spec())

	if o.Word != model.Busy || o.Refusal == nil || !strings.Contains(o.Refusal.Observed(), "1794132000") {
		t.Fatalf("ended %q %+v, want busy naming the holder's time", o.Word, o.Refusal)
	}
	if o.Refusal.Hint() == "" {
		t.Error("a busy refusal carries the line to type")
	}
	if len(f.w.Writes) != 0 || len(f.records()) != 0 {
		t.Errorf("a busy run wrote %v and journalled %d records", f.w.Writes, len(f.records()))
	}
	if !slices.Equal(f.spy.log, []string{"take root"}) {
		t.Errorf("calls %q, want the lock alone", f.spy.log)
	}
}

// TestASecondRunWithTheSameInputsIsDoneNothingToDo: a plan whose goal already
// holds is the empty delta. The table is the model's idempotent shape: retire
// a retired rule, waive a waived path, bind an existing binding. Risk: a
// second run that writes again or journals (cli.commands.2, cli.commands.6).
func TestASecondRunWithTheSameInputsIsDoneNothingToDo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit edit
	}{
		{"retire a retired rule", edit{path: "rules/a.yaml", data: "status: retired\n"}},
		{"waive a waived path", edit{path: "state/waive.yaml", data: "waivers: [src/gen]\n"}},
		{"bind an existing binding", edit{path: "rules/b.yaml", data: "bindings: [lint.x]\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tp := f.plan(tc.edit)

			first := f.run(tp.spec())
			wantDone(t, first)
			if first.Nothing || len(first.Changes) != 1 {
				t.Fatalf("the first run %+v, want one change", first)
			}
			writes, records, calls := len(f.w.Writes), len(f.records()), len(f.spy.log)

			again := tp.spec()
			again.Run = "run-2"
			second := f.run(again)

			wantDone(t, second)
			if !second.Nothing || len(second.Changes) != 0 {
				t.Errorf("the second run %+v, want done, nothing to do", second)
			}
			if len(f.w.Writes) != writes || len(f.records()) != records {
				t.Errorf("the second run wrote %d times and journalled %d records", len(f.w.Writes)-writes, len(f.records())-records)
			}
			if got, want := f.spy.log[calls:], []string{"take root", "release root"}; !slices.Equal(got, want) {
				t.Errorf("the second run called %q, want only the lock", got)
			}
		})
	}
}

// TestAConfirmThatDoesNotNameTheThingIsRefusedBeforeTheLock: a destructive
// command takes the name it removes. A wrong, an empty and a case-folded name
// each end refused with nothing taken; the right one runs. Risk: a withdraw
// or an archive run by a flag that was not read (cli.commands.3).
func TestAConfirmThatDoesNotNameTheThingIsRefusedBeforeTheLock(t *testing.T) {
	t.Parallel()
	for _, given := range []string{"", "other", "V1.0.0", "v1.0.0 "} {
		f := newFixture(t)
		s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
		s.Name, s.Confirmed = "v1.0.0", given

		o := f.run(s)

		if o.Word != model.Refused || o.Refusal == nil || !slices.Equal(o.Refusal.Valid(), []string{"v1.0.0"}) ||
			!strings.Contains(o.Refusal.Hint(), "--confirm v1.0.0") {
			t.Errorf("confirm %q ended %q %+v, want refused with the name and the line to type", given, o.Word, o.Refusal)
		}
		if len(f.spy.log) != 0 || len(f.w.Writes) != 0 {
			t.Errorf("confirm %q: called %q and wrote %v", given, f.spy.log, f.w.Writes)
		}
	}
	f := newFixture(t)
	s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
	s.Name, s.Confirmed = "v1.0.0", "v1.0.0"
	wantDone(t, f.run(s))
}

// TestAPathOutsideTheScopeOrNotCleanIsRefusedBeforeTheIntent: every target is
// vetted before anything is journalled. Risk: a plan that writes where the
// command did not say, or into the repository's own folder, which runs hooks
// (cli.commands.4).
func TestAPathOutsideTheScopeOrNotCleanIsRefusedBeforeTheIntent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, path string }{
		{"outside the scope", "identity.yaml"},
		{"climbing", "../rules/a.yaml"},
		{"climbing inside", "rules/../rules/a.yaml"},
		{"absolute", "/work/scratch/hosts"},
		{"a backslash", backslashed(t)["file"]},
		{"the repository's folder", ".git/hooks/pre-push"},
		{"the repository's folder, folded", ".GIT/config"},
		{"the repository's folder, padded", ".git /config"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := f.plan().spec()
			if tc.name != "outside the scope" {
				s.Scope = []string{"*", "*/*", "*/*/*", ".git/*", ".GIT/*", ".git /*", "../rules/*", "/work/scratch/*", "rules/../rules/*", backslashed(t)["scope"]}
			}
			s.Render = func(model.Delta) ([]model.Target, error) {
				return []model.Target{{Path: tc.path, Data: []byte("x")}}, nil
			}
			s.Plan = func(context.Context) (model.Plan, error) {
				return model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "file:" + tc.path}}}}, nil
			}

			o := f.run(s)

			if o.Word != model.Refused {
				t.Fatalf("ended %q, want refused", o.Word)
			}
			if len(f.w.Writes) != 0 || len(f.records()) != 0 {
				t.Errorf("wrote %v and journalled %d records", f.w.Writes, len(f.records()))
			}
		})
	}
	f := newFixture(t)
	s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
	s.Scope = nil
	if o := f.run(s); o.Word != model.Refused {
		t.Errorf("a command with no scope ended %q, want refused", o.Word)
	}
}

// TestAPlanThatWritesAFileTwiceOrAddsALineWithANewlineIsRefused: Keep takes a
// path once per run, and an appended line is one line. Risk: a second copy
// that stores replaced bytes as the original, or two lines under one record.
func TestAPlanThatWritesAFileTwiceOrAddsALineWithANewlineIsRefused(t *testing.T) {
	t.Parallel()
	for name, targets := range map[string][]model.Target{
		"twice":   {{Path: "rules/a.yaml", Before: apply.DigestBytes([]byte("status: active\n")), Data: []byte("a")}, {Path: "rules/a.yaml", Before: apply.DigestBytes([]byte("status: active\n")), Data: []byte("b")}},
		"newline": {{Path: "history.jsonl", Before: apply.DigestBytes([]byte("{\"n\":1}\n")), Data: []byte("one\ntwo"), Append: true}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := f.plan().spec()
			s.Render = func(model.Delta) ([]model.Target, error) { return targets, nil }
			s.Plan = func(context.Context) (model.Plan, error) {
				return model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "x"}}}}, nil
			}
			if o := f.run(s); o.Word != model.Refused || len(f.w.Writes) != 0 {
				t.Errorf("ended %q after %v", o.Word, f.w.Writes)
			}
		})
	}
}

// TestALockIsPutBackOnEveryPath: after done, nothing to do, refused, busy
// excepted, a failed plan, a failed write and a cancelled run, the next Take
// of the root lock succeeds. Risk: a lock left held, which blocks every later
// writer (go.concurrency.3).
func TestALockIsPutBackOnEveryPath(t *testing.T) {
	t.Parallel()
	edits := []edit{{path: "rules/a.yaml", data: "status: retired\n"}}
	for name, arrange := range map[string]func(f *fixture, s *apply.Spec) context.Context{
		"done":    func(*fixture, *apply.Spec) context.Context { return t.Context() },
		"nothing": func(f *fixture, s *apply.Spec) context.Context { *s = f.plan().spec(); return t.Context() },
		"plan failed": func(_ *fixture, s *apply.Spec) context.Context {
			s.Plan = func(context.Context) (model.Plan, error) { return model.Plan{}, errBoom }
			return t.Context()
		},
		"write failed": func(f *fixture, _ *apply.Spec) context.Context {
			f.w.WriteFails("rules/a.yaml", errBoom)
			return t.Context()
		},
		"cancelled": func(*fixture, *apply.Spec) context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx
		},
		"moved": func(f *fixture, s *apply.Spec) context.Context {
			tp := f.plan(edits...)
			tp.moved = func() { f.w.Seed(string(f.root), map[string]string{"rules/a.yaml": "other\n"}) }
			*s = tp.spec()
			return t.Context()
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := f.plan(edits...).spec()
			ctx := arrange(f, &s)
			apply.Run(ctx, f.p, f.root, s)
			h, err := f.inner().Lock.Take(t.Context(), f.root, model.RootLock)
			if err != nil {
				t.Fatalf("the lock is still held: %v", err)
			}
			_ = f.inner().Lock.Release(h)
		})
	}
}

// TestAFailedReleaseOfTheLockEndsFailed: a lock that cannot be put back is not
// hidden behind done.
func TestAFailedReleaseOfTheLockEndsFailed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.p.Lock = badRelease{f.p.Lock}
	o := f.run(f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec())
	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailLock || o.Failure.Run() != runID {
		t.Errorf("ended %q %+v, want failed, kind lock, the run id", o.Word, o.Failure)
	}
}

type badRelease struct{ port.Lock }

func (badRelease) Release(model.Hold) error { return errBoom }

// TestAJournalThatCannotBeWrittenStopsTheEffect: a failing Append of any kind,
// and a failing Keep, end the run failed before the first write, or before the
// write that record announces. Risk: an effect with no record of it
// (store.consistency.4).
func TestAJournalThatCannotBeWrittenStopsTheEffect(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		failKind   string
		failKeep   bool
		wantWrites int
	}{
		{"the intent", "intent", false, 0},
		{"the copy record", "copy", false, 0},
		{"the step record", "step", false, 0},
		{"the keep", "", true, 0},
		// The write, then its roll-back, whose close fails too: the tree is
		// put back and the record stays open for the next command.
		{"the close", "close", false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.p.Journal = flakyJournal{Journal: f.p.Journal, kind: tc.failKind, keep: tc.failKeep}

			o := f.run(f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec())

			if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailJournal || o.Failure.Run() != runID {
				t.Fatalf("ended %q %+v, want failed, kind journal, with the run id", o.Word, o.Failure)
			}
			if len(f.w.Writes) != tc.wantWrites {
				t.Errorf("wrote %d files, want %d", len(f.w.Writes), tc.wantWrites)
			}
		})
	}
}

type flakyJournal struct {
	port.Journal
	kind string
	keep bool
}

func (j flakyJournal) Append(ctx context.Context, r model.Root, rec model.Record) error {
	if rec.Kind == j.kind {
		return errBoom
	}
	return j.Journal.Append(ctx, r, rec)
}

func (j flakyJournal) Keep(r model.Root, run, p string, old []byte) error {
	if j.keep {
		return errBoom
	}
	return j.Journal.Keep(r, run, p, old)
}

// TestAFailedWriteEndsFailedWithTheRunIdAndRollsBack: the second of three
// writes fails. The run is failed with its id and rolled back: the first file
// is put back from its copy under a step of its own, the second, never
// written, is left as it is, the third was never touched, and the record
// closes rolled back (cli.commands.7).
func TestAFailedWriteEndsFailedWithTheRunIdAndRollsBack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.w.WriteFails("rules/b.yaml", errBoom)
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}, edit{path: "rules/b.yaml", data: "status: trial\n"})
	tp.yes = true

	o := f.run(tp.spec())

	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailWrite || o.Failure.Run() != runID || !errors.Is(o.Cause, errBoom) {
		t.Fatalf("ended %q %+v cause %v, want failed, kind write, the run id", o.Word, o.Failure, o.Cause)
	}
	if got, want := kinds(f.records()), []string{"intent", "copy", "step", "copy", "step", "step", "close"}; !slices.Equal(got, want) {
		t.Errorf("records %q, want %q: a restore of a, then the close", got, want)
	}
	if !o.RolledBack {
		t.Error("the run does not say it was rolled back")
	}
	if f.content("rules/a.yaml") != "status: active\n" || f.content("rules/b.yaml") != "status: active\n" || f.content("history.jsonl") != "{\"n\":1}\n" {
		t.Errorf("tree: a=%q b=%q history=%q", f.content("rules/a.yaml"), f.content("rules/b.yaml"), f.content("history.jsonl"))
	}
	if b, err := f.inner().Journal.Kept(f.root, runID, "rules/b.yaml"); err != nil || string(b) != "status: active\n" {
		t.Errorf("the copy of b = %q, %v", b, err)
	}
}

// TestAFileThatDoesNotHoldWhatWasWrittenEndsFailed: the read back after a
// write is compared with the bytes written (cli.commands.22), and a file
// that reads as bytes the run never wrote is not rolled back over: the run
// ends failed, kind verify, with its record open, naming the file. Risk: a
// write that returned nil and left something else, and a roll-back over
// bytes that are not the run's.
func TestAFileThatDoesNotHoldWhatWasWrittenEndsFailed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.p.Tree = lyingTree{Tree: f.p.Tree, path: "rules/a.yaml"}
	o := f.run(f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec())
	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailVerify {
		t.Errorf("ended %q %+v, want failed, kind verify", o.Word, o.Failure)
	}
	if got, want := kinds(f.records()), []string{"intent", "copy", "step"}; !slices.Equal(got, want) {
		t.Errorf("records %q, want %q: no restore and no close", got, want)
	}
	if o.RolledBack || o.Cause == nil || !strings.Contains(o.Cause.Error(), "rules/a.yaml changed since run "+runID) {
		t.Errorf("rolled back %v, cause %v; want the record open, naming a", o.RolledBack, o.Cause)
	}
	if open := f.open(); !slices.Equal(open, []string{runID}) {
		t.Errorf("open records %v, want %s", open, runID)
	}
}

// lyingTree reads path as something else once it has been written.
type lyingTree struct {
	port.Tree
	path string
}

func (l lyingTree) Read(r model.Root, p string) ([]byte, error) {
	b, err := l.Tree.Read(r, p)
	if p == l.path && strings.Contains(string(b), "retired") {
		return []byte("something else"), err
	}
	return b, err
}

// TestACancelledRunWritesNothingMore: a context that ends before the first
// write ends the run failed, kind cancelled, and no file is touched.
func TestACancelledRunWritesNothingMore(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}, edit{path: "rules/b.yaml", data: "status: trial\n"}).spec()
	plan := s.Plan
	s.Plan = func(c context.Context) (model.Plan, error) { p, err := plan(c); cancel(); return p, err }

	o := apply.Run(ctx, f.p, f.root, s)

	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailCancelled && o.Failure.Kind() != apply.FailPlan {
		t.Fatalf("ended %q %+v", o.Word, o.Failure)
	}
	if len(f.w.Writes) != 0 {
		t.Errorf("wrote %v", f.w.Writes)
	}
}

// TestAppendEndsATornLastLineAndTheRecordHoldsTheYes: a history file whose
// last line has no newline gets the line on a line of its own, and the read
// back accepts it.
func TestAppendEndsATornLastLineAndTheRecordHoldsTheYes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.w.Seed(string(f.root), map[string]string{"rules/a.yaml": "status: active\n", "history.jsonl": `{"n":1}`})
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"})
	tp.yes = true

	o := f.run(tp.spec())

	wantDone(t, o)
	lines := strings.Split(strings.TrimSuffix(f.content("history.jsonl"), "\n"), "\n")
	if len(lines) != 2 || lines[0] != `{"n":1}` {
		t.Errorf("history.jsonl = %q", lines)
	}
}

// TestAPlanOrRenderThatFailsEndsFailedBeforeAnyRecord.
func TestAPlanOrRenderThatFailsEndsFailedBeforeAnyRecord(t *testing.T) {
	t.Parallel()
	for name, arrange := range map[string]func(*apply.Spec){
		"plan": func(s *apply.Spec) {
			s.Plan = func(context.Context) (model.Plan, error) { return model.Plan{}, errBoom }
		},
		"render": func(s *apply.Spec) { s.Render = func(model.Delta) ([]model.Target, error) { return nil, errBoom } },
		"record": func(s *apply.Spec) {
			s.Record = func(model.Yes) (model.Target, error) { return model.Target{}, errBoom }
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"})
			tp.yes = true
			s := tp.spec()
			arrange(&s)
			o := f.run(s)
			if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailPlan || !errors.Is(o.Cause, errBoom) {
				t.Errorf("ended %q %+v cause %v, want failed, kind plan", o.Word, o.Failure, o.Cause)
			}
			if len(f.records()) != 0 || len(f.w.Writes) != 0 {
				t.Errorf("journalled %d records and wrote %v", len(f.records()), f.w.Writes)
			}
		})
	}
}

// TestACallThatGivesNoRunIdOrNoFunctionsIsFailedWiring.
func TestACallThatGivesNoRunIdOrNoFunctionsIsFailedWiring(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for name, mutate := range map[string]func(*apply.Spec){
		"no run":    func(s *apply.Spec) { s.Run = "" },
		"no plan":   func(s *apply.Spec) { s.Plan = nil },
		"no render": func(s *apply.Spec) { s.Render = nil },
		"a yes without a record function": func(s *apply.Spec) {
			plan := s.Plan
			s.Plan = func(c context.Context) (model.Plan, error) {
				p, err := plan(c)
				p.Yes = &model.Yes{Who: "x"}
				return p, err
			}
		},
		"a record function without a yes": func(s *apply.Spec) {
			s.Record = func(model.Yes) (model.Target, error) { return model.Target{}, nil }
		},
	} {
		s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
		mutate(&s)
		o := f.run(s)
		if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailWiring || o.Failure.Run() == "" {
			t.Errorf("%s: ended %q %+v, want failed, kind wiring, a run id", name, o.Word, o.Failure)
		}
	}
	if len(f.w.Writes) != 0 {
		t.Errorf("wrote %v", f.w.Writes)
	}
}

// TestARecordThatMovesBetweenItsTwoCallsIsFailedWiring: the record's path is
// fixed before the digest, so a function that answers another path the second
// time is refused rather than written.
func TestARecordThatMovesBetweenItsTwoCallsIsFailedWiring(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"})
	tp.yes = true
	s := tp.spec()
	calls := 0
	s.Record = func(y model.Yes) (model.Target, error) {
		calls++
		return model.Target{Path: map[int]string{1: "history.jsonl", 2: "rules/b.yaml"}[calls], Data: []byte("x"), Append: true}, nil
	}
	if o := f.run(s); o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailWiring || len(f.w.Writes) != 0 {
		t.Errorf("ended %q %+v after %v", o.Word, o.Failure, f.w.Writes)
	}
}

// TestADeleteKeepsTheOldFileTakesItAwayThroughTheWriterAndHoldsItsGoal: a plan
// that drops a file keeps its bytes in the journal, journals a step of op
// "delete", removes it through the Writer, and ends done with the file not
// there; the same words again have nothing to do. A file that changed
// between the plan and the lock is refused with nothing removed.
// Risk: a delete that keeps no copy, one that leaves the file, or one that
// removes a file the plan never read.
func TestADeleteKeepsTheOldFileTakesItAwayThroughTheWriterAndHoldsItsGoal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tp := f.plan(edit{path: "rules/b.yaml", drop: true})

	o := f.run(tp.spec())

	wantDone(t, o)
	if got := f.content("rules/b.yaml"); got != "<absent>" {
		t.Errorf("rules/b.yaml holds %q after its delete", got)
	}
	if !slices.Contains(f.spy.log, "remove rules/b.yaml") || slices.Contains(f.spy.log, "replace rules/b.yaml") {
		t.Errorf("calls %q: the file was not taken away through Remove", f.spy.log)
	}
	if kept, err := f.inner().Journal.Kept(f.root, runID, "rules/b.yaml"); err != nil || string(kept) != "status: active\n" {
		t.Errorf("kept %q, %v; want the old bytes", kept, err)
	}
	if recs := f.records(); !strings.Contains(string(recs[2].Detail), `"op":"delete"`) {
		t.Errorf("step record %s does not say delete", recs[2].Detail)
	}
	next := f.plan(edit{path: "rules/b.yaml", drop: true}).spec()
	next.Run = "run-2"
	if again := f.run(next); again.Word != model.Done || !again.Nothing || len(again.Changes) != 0 {
		t.Errorf("a second delete: %q %+v, want done, nothing to do", again.Word, again)
	}

	moved := newFixture(t)
	mp := moved.plan(edit{path: "rules/b.yaml", drop: true})
	mp.moved = func() {
		moved.w.Seed(string(moved.root), map[string]string{"rules/a.yaml": "status: active\n", "rules/b.yaml": "status: active\n# edited by hand\n"})
	}
	if r := moved.run(mp.spec()); r.Word != model.Refused || moved.content("rules/b.yaml") == "<absent>" {
		t.Errorf("a file edited after the plan: %q, file %q, want refused and kept", r.Word, moved.content("rules/b.yaml"))
	}
}
