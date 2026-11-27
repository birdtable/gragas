package apply_test

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// errNotMine is what the host's Lock.Take and Writer.Replace answer over a
// folder another account owns, with made-up folder and accounts.
var errNotMine = fmt.Errorf("the root /made/up is owned by account 7, and this run is account 0: %w", port.ErrNotOwner)

// notMineLock is a lock kept in a state folder another account owns.
type notMineLock struct{ port.Lock }

func (notMineLock) Wait(context.Context, model.Root, model.Lock) (model.Hold, error) {
	return model.Hold{}, errNotMine
}

func (notMineLock) Take(context.Context, model.Root, model.Lock) (model.Hold, error) {
	return model.Hold{}, errNotMine
}

// TestAFolderAnotherAccountOwnsEndsRefused: the lock or the replace answers
// ErrNotOwner, and the command ends "refused", the closing word a person
// reads, never "failed": the observed line is the host's sentence, saying
// whose folder it is, the hint says to type the command again as that
// account, and no file of the root is written. Each place the error can
// come from: the run's lock, its first replace, the reading check's lock
// over an open record, and the replace of a roll-back of an interrupted run.
// Risk: a refusal for safety read as the engine breaking.
func TestAFolderAnotherAccountOwnsEndsRefused(t *testing.T) {
	t.Parallel()
	edits := []edit{{path: "rules/a.yaml", data: "status: retired\n"}}
	for name, tc := range map[string]struct {
		arrange func(f *fixture)
		run     func(f *fixture) apply.Outcome
		// records are the kinds of the records the refusal leaves after
		// the ones before it: a roll-back journals its step before it
		// writes. outcome is what this run's close says, "" for none.
		records []string
		outcome string
	}{
		"the run's lock": {
			arrange: func(f *fixture) { f.p.Lock = notMineLock{f.p.Lock} },
			run:     func(f *fixture) apply.Outcome { return f.run(f.plan(edits...).spec()) },
		},
		"the first replace": {
			arrange: func(f *fixture) { f.w.WriteFails("rules/a.yaml", errNotMine) },
			run:     func(f *fixture) apply.Outcome { return f.run(f.plan(edits...).spec()) },
			records: []string{"intent", "copy", "step", "close"},
			outcome: "refused",
		},
		"the check's lock over an open record": {
			arrange: func(f *fixture) {
				f.interrupt("run-0", 1, f.threeFiles().spec())
				f.p.Lock = notMineLock{f.p.Lock}
			},
			run: func(f *fixture) apply.Outcome {
				o, _ := apply.Ready(t.Context(), f.p, f.root, apply.Check{Run: "run-9", Command: "gragas status", Recover: recoverLine})
				return o
			},
		},
		"the roll-back's replace": {
			arrange: func(f *fixture) {
				f.interrupt("run-0", 1, f.threeFiles().spec())
				f.w.WriteFails("", errNotMine)
			},
			run:     func(f *fixture) apply.Outcome { return f.run(f.plan(edits...).spec()) },
			records: []string{"step"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tc.arrange(f)
			before, n, writes, open := f.snapshot(fixturePaths()...), len(f.records()), len(f.w.Writes), f.open()

			o := tc.run(f)

			if o.Word != model.Refused || o.Refusal == nil || o.Failure != nil {
				t.Fatalf("ended %q %+v %+v, want refused", o.Word, o.Refusal, o.Failure)
			}
			if obs := o.Refusal.Observed(); !strings.Contains(obs, "owned by account 7, and this run is account 0") {
				t.Errorf("observed %q, want the host's sentence", obs)
			}
			if hint := o.Refusal.Hint(); !strings.HasSuffix(hint, " again as the account that owns that folder") {
				t.Errorf("hint %q, want the command again as the folder's owner", hint)
			}
			if !maps.Equal(f.snapshot(fixturePaths()...), before) || len(f.w.Writes) != writes {
				t.Errorf("the refusal wrote %v", f.w.Writes[writes:])
			}
			if got := f.open(); !slices.Equal(got, open) {
				t.Errorf("open records %v, want %v as they were", got, open)
			}
			recs := f.records()[n:]
			if got := kinds(recs); !slices.Equal(got, tc.records) {
				t.Errorf("records %q, want %q", got, tc.records)
			}
			if tc.outcome != "" {
				if c := f.closeOf(runID); c["outcome"] != canon.String(tc.outcome) {
					t.Errorf("the close says %v, want %q", c, tc.outcome)
				}
			}
		})
	}
}

// TestARootAnotherAccountOwnsAfterAWriteFailsAndRollsBack: the refusal holds
// only while nothing is written. A second replace that answers ErrNotOwner,
// after the first file was written, is no refusal: the tree is not as it was,
// so the run fails and rolls back, as any failed write does. Risk: "refused",
// which says the tree is as it was, over a half-written tree.
func TestARootAnotherAccountOwnsAfterAWriteFailsAndRollsBack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.w.WriteFails("rules/b.yaml", errNotMine)
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}, edit{path: "rules/b.yaml", data: "status: trial\n"})

	o := f.run(tp.spec())

	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailWrite || !o.RolledBack {
		t.Fatalf("ended %q %+v, rolled back %v, want failed, kind write, rolled back", o.Word, o.Failure, o.RolledBack)
	}
	if a := f.content("rules/a.yaml"); a != "status: active\n" {
		t.Errorf("rules/a.yaml holds %q, want it put back", a)
	}
}
