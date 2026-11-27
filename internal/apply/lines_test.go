package apply_test

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// historyLines are n lines of the shape of a history record, numbered after
// the line the fixture's history holds: the append of a plan that records
// several entries.
func historyLines(n int, y model.Yes) []string {
	var out []string
	for i := range n {
		out = append(out, fmt.Sprintf(`{"after":{"status":"active"},"entity":"rule:go.errors.%d","format":3,"kind":"reference","number":%d,"plan":%q,"when":"2026-11-08T10:00:00Z","who":%q}`,
			i+1, i+2, y.Plan, y.Who))
	}
	return out
}

// severalLines is a plan that edits a file and records lines, as one append.
func severalLines(f *fixture, lines func(model.Yes) []string) apply.Spec {
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"})
	tp.yes = true
	s := tp.spec()
	s.Record = func(y model.Yes) (model.Target, error) {
		return model.Target{Path: "history.jsonl", Before: tp.digest("history.jsonl"), Data: []byte(strings.Join(lines(y), "\n")), Append: true}, nil
	}
	return s
}

func appendsTo(f *fixture, p string) int {
	n := 0
	for _, l := range f.spy.log {
		if l == "append "+p {
			n++
		}
	}
	return n
}

// TestAPlanThatRecordsSeveralEntriesAppendsThemInOneWriteAndTheGoalChecksEach:
// three history lines are one append; the run ends done, the history holds the
// line it had and the three after it, each whole, and the goal verified them.
// Planted failure: the write is split into one append for each line.
func TestAPlanThatRecordsSeveralEntriesAppendsThemInOneWriteAndTheGoalChecksEach(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var want []string
	o := f.run(severalLines(f, func(y model.Yes) []string { want = historyLines(3, y); return want }))
	wantDone(t, o)
	if got := f.content("history.jsonl"); got != `{"n":1}`+"\n"+strings.Join(want, "\n")+"\n" {
		t.Errorf("history.jsonl = %q", got)
	}
	if n := appendsTo(f, "history.jsonl"); n != 1 {
		t.Errorf("the history was appended to %d times, want one write for the plan's three lines", n)
	}
}

// TestALineThatIsNotAHistoryRecordAmongSeveralIsRefusedAndNothingIsWritten:
// the wider append is held to the record's own shape, so a forged line in the
// middle of it, or a line that lacks a key of a record, ends refused and the
// tree is as it was.
// Planted failure: the lines are not checked, so the forged one is appended.
func TestALineThatIsNotAHistoryRecordAmongSeveralIsRefusedAndNothingIsWritten(t *testing.T) {
	t.Parallel()
	for name, forge := range map[string]func([]string) []string{
		"not JSON":          func(l []string) []string { l[1] = "an entry nobody made"; return l },
		"no plan":           func(l []string) []string { l[1] = strings.Replace(l[1], `"plan":`, `"planned":`, 1); return l },
		"not an object":     func(l []string) []string { l[1] = `["a","list"]`; return l },
		"an empty line":     func(l []string) []string { l[1] = ""; return l },
		"a carriage return": func(l []string) []string { l[1] += "\r"; return l },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			before := f.snapshot(fixturePaths()...)
			o := f.run(severalLines(f, func(y model.Yes) []string { return forge(historyLines(3, y)) }))
			if o.Word != model.Refused {
				t.Fatalf("ended %q (cause %v), want refused", o.Word, o.Cause)
			}
			if after := f.snapshot(fixturePaths()...); !maps.Equal(after, before) {
				t.Errorf("the tree changed:\n%q\nwant\n%q", after, before)
			}
		})
	}
}

// landedThenFails is a Writer whose append lands every line it is given and then
// reports a failure, as a write does when the run is cut just after it: the plan's
// lines are all in the file or none of them are, and the journal's roll-back, which
// knows the file by its digest before and after, takes them all out.
type landedThenFails struct{ port.Writer }

func (w landedThenFails) Append(r model.Root, p string, lines []byte) error {
	if err := w.Writer.Append(r, p, lines); err != nil {
		return err
	}
	return errBoom
}

// TestAnInterruptedAppendOfSeveralLinesLeavesNoneOfThemAndACompleteOneAllOfThem:
// the run is cut just after the append landed; it ends failed and rolled back, and
// the history is the bytes it was, none of the plan's lines, so the chain is what it
// was. The control, uncut, holds all three, written by one append. One write has two
// states for the roll-back to know, none and all.
// Planted failure: the plan's lines are appended one by one, which the count of
// appends shows, since a cut between two of them leaves some and not the rest.
func TestAnInterruptedAppendOfSeveralLinesLeavesNoneOfThemAndACompleteOneAllOfThem(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	before := f.snapshot(fixturePaths()...)
	f.p.Writer = landedThenFails{Writer: f.p.Writer}
	o := f.run(severalLines(f, func(y model.Yes) []string { return historyLines(3, y) }))
	if o.Word != model.Failed || !o.RolledBack {
		t.Fatalf("ended %q (cause %v) rolled back %v, want failed and rolled back", o.Word, o.Cause, o.RolledBack)
	}
	if after := f.snapshot(fixturePaths()...); !maps.Equal(after, before) {
		t.Errorf("the tree after a cut\n %q\nwant it as before\n %q", after, before)
	}
	t.Run("the control, uncut, holds every line from one append", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		wantDone(t, f.run(severalLines(f, func(y model.Yes) []string { return historyLines(3, y) })))
		if n := strings.Count(f.content("history.jsonl"), "\n"); n != 4 {
			t.Errorf("history.jsonl holds %d lines, want the one it had and the three", n)
		}
		if n := appendsTo(f, "history.jsonl"); n != 1 {
			t.Errorf("the history was appended to %d times, want one write for the three lines", n)
		}
	})
}

// forgingOnTheSecondCall is a plan whose record function answers a clean
// record while the plan digest is still empty and forged lines once the digest
// is in: the record that is written is the second answer, so the check of the
// lines must read it, not only the first.
func forgingOnTheSecondCall(f *fixture, clean, forged func(model.Yes) []string) apply.Spec {
	tp := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"})
	tp.yes = true
	s := tp.spec()
	s.Record = func(y model.Yes) (model.Target, error) {
		lines := clean(y)
		if y.Plan != "" {
			lines = forged(y)
		}
		return model.Target{Path: "history.jsonl", Before: tp.digest("history.jsonl"), Data: []byte(strings.Join(lines, "\n")), Append: true}, nil
	}
	return s
}

// TestALineForgedByTheSecondCallOfTheRecordFunctionIsRefusedLikeOneOfTheFirst:
// the lines that are written are the second call's, made with the plan digest set,
// and they are held to the same checks as the first: an extra line that is not a
// record, a carriage return at the end of the second of several lines, a single
// line that ends in one. Each ends refused and writes nothing.
// Planted failure: the check reads only the record of the first call, so each of
// the three ends done.
func TestALineForgedByTheSecondCallOfTheRecordFunctionIsRefusedLikeOneOfTheFirst(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ clean, forged func(model.Yes) []string }{
		"an extra line": {
			func(y model.Yes) []string { return historyLines(2, y) },
			func(y model.Yes) []string { return append(historyLines(2, y), "an entry nobody made") },
		},
		"a carriage return at the end of line 2": {
			func(y model.Yes) []string { return historyLines(3, y) },
			func(y model.Yes) []string { l := historyLines(3, y); l[1] += "\r"; return l },
		},
		"a single line that ends in a carriage return": {
			func(y model.Yes) []string { return historyLines(1, y) },
			func(y model.Yes) []string { return []string{historyLines(1, y)[0] + "\r"} },
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			before := f.snapshot(fixturePaths()...)
			o := f.run(forgingOnTheSecondCall(f, tc.clean, tc.forged))
			if o.Word != model.Refused {
				t.Fatalf("ended %q (cause %v), want refused", o.Word, o.Cause)
			}
			if after := f.snapshot(fixturePaths()...); !maps.Equal(after, before) {
				t.Errorf("the tree changed:\n%q\nwant\n%q", after, before)
			}
		})
	}
}
