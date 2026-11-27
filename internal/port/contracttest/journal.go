package contracttest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// JournalSetup is the Journal under test and two roots it keeps a journal
// of, both opened. Pass moves on the clock the Journal stamps a record and
// sweeps by, by d; the Journal keeps a closed run thirty days.
type JournalSetup struct {
	J           port.Journal
	Root, Other model.Root
	Pass        func(d time.Duration)
	// Bytes is the journal file of Root as it is now, the bytes a caller
	// past the port reads of it and hands to RecordsOf; none before the
	// first record.
	Bytes func() []byte
}

// JournalMaker makes the setup for one subtest.
type JournalMaker func(t *testing.T) JournalSetup

// Journal runs the Journal suite against the port build answers.
func Journal(t *testing.T, build JournalMaker) {
	t.Helper()
	t.Run("a root with no journal has no records", func(t *testing.T) { journalEmpty(t, build(t)) })
	t.Run("records come back in order with every fixed field", func(t *testing.T) { journalOrder(t, build(t)) })
	t.Run("a record with no detail has none back", func(t *testing.T) { journalNoDetail(t, build(t)) })
	t.Run("two roots keep two journals", func(t *testing.T) { journalSeparate(t, build(t)) })
	t.Run("runs come back in the order begun, each run's records in the order written", func(t *testing.T) { journalRunOrder(t, build(t)) })
	t.Run("an entry of a run not yet begun is torn until its intent or close begins the run", func(t *testing.T) { journalNeverBegun(t, build(t)) })
	t.Run("the records of the bytes of a run not yet begun are its Records, torn too", func(t *testing.T) { journalNeverBegunBytes(t, build(t)) })
	t.Run("a sweep that removes a run drops the entries of a run not yet begun", func(t *testing.T) { journalNeverBegunSwept(t, build(t)) })
	t.Run("a cancelled read is refused, and a later record goes in with the cancel taken off", func(t *testing.T) { journalCancelled(t, build(t)) })
	t.Run("the records of a journal's bytes are its Records, line for line", func(t *testing.T) { journalBytesAgree(t, build(t)) })
	journalRefuses(t, build)
	t.Run("a copy is kept by run and path", func(t *testing.T) { copyKeptByRunAndPath(t, build(t)) })
	t.Run("a copy nobody kept is not there", func(t *testing.T) { copyMissing(t, build(t)) })
	t.Run("each copy is at a path of its own outside the root", func(t *testing.T) { copyAt(t, build(t)) })
	t.Run("an empty copy is an empty slice and not an error", func(t *testing.T) { copyEmpty(t, build(t)) })
	t.Run("a copy is the bytes given, not the slice", func(t *testing.T) { copyIsolated(t, build(t)) })
	t.Run("a second copy for the same run and path is refused and the first stays", func(t *testing.T) { copyTwice(t, build(t)) })
	t.Run("a copy under a run that is no name is refused", func(t *testing.T) { copyBadRun(t, build(t)) })
	t.Run("a sweep removes only the runs closed past the keep, never an open one", func(t *testing.T) { sweepOld(t, build(t)) })
	t.Run("a sweep of a root with no journal removes nothing", func(t *testing.T) { sweepEmpty(t, build(t)) })
	t.Run("a cancelled sweep is refused and removes nothing", func(t *testing.T) { sweepCancelled(t, build(t)) })
}

// fixed is a record with every fixed field and a detail, of kind.
func fixed(run, kind string) model.Record {
	return model.Record{
		Run: run, Kind: kind, Command: "gragas gen --apply", Plan: "sha256:aa", Engine: "v0.1.0",
		Detail: []byte(`{"argv":["gragas","gen"],"paths":["rules/a.md"]}`),
	}
}

func journalEmpty(t *testing.T, s JournalSetup) {
	t.Helper()
	got, torn, err := s.J.Records(t.Context(), s.Root)
	if err != nil || len(got) != 0 || torn != 0 {
		t.Errorf("Records of a root with no journal = %+v, %d, %v, want none, 0 and no error", got, torn, err)
	}
}

func journalOrder(t *testing.T, s JournalSetup) {
	t.Helper()
	kinds := []string{model.KindIntent, model.KindCopy, model.KindStep, model.KindStart, model.KindClose}
	for _, k := range kinds {
		if err := s.J.Append(t.Context(), s.Root, fixed("run-1", k)); err != nil {
			t.Fatalf("Append(%s) = %v", k, err)
		}
	}
	got, torn, err := s.J.Records(t.Context(), s.Root)
	if err != nil || torn != 0 || len(got) != len(kinds) {
		t.Fatalf("Records = %d records, %d torn, %v, want %d, 0 and no error", len(got), torn, err, len(kinds))
	}
	for i, r := range got {
		want := fixed("run-1", kinds[i])
		if r.Version != model.RecordVersion || r.Run != want.Run || r.Kind != want.Kind || r.Command != want.Command ||
			r.Plan != want.Plan || r.Engine != want.Engine || string(r.Detail) != string(want.Detail) {
			t.Errorf("record %d = %+v, want the fields of %+v at version %d", i, r, want, model.RecordVersion)
		}
	}
}

// journalRunOrder writes two runs' records interleaved: Records gives the
// run begun first whole, then the other, each in the order written.
func journalRunOrder(t *testing.T, s JournalSetup) {
	t.Helper()
	for _, r := range []model.Record{
		{Run: "run-a", Kind: model.KindIntent}, {Run: "run-b", Kind: model.KindIntent},
		{Run: "run-a", Kind: model.KindStep}, {Run: "run-b", Kind: model.KindClose},
		{Run: "run-a", Kind: model.KindClose},
	} {
		if err := s.J.Append(t.Context(), s.Root, r); err != nil {
			t.Fatalf("Append(%s, %s) = %v", r.Run, r.Kind, err)
		}
	}
	got, torn, err := s.J.Records(t.Context(), s.Root)
	if err != nil || torn != 0 {
		t.Fatalf("Records = %d torn, %v", torn, err)
	}
	var seen []string
	for _, r := range got {
		seen = append(seen, r.Run+" "+r.Kind)
	}
	want := []string{"run-a intent", "run-a step", "run-a close", "run-b intent", "run-b close"}
	if !slices.Equal(seen, want) {
		t.Errorf("Records = %v, want %v", seen, want)
	}
}

// journalCancelled: with the context cancelled, Records is refused and a
// later record of a run that is there is refused and leaves the journal as
// it was. The same record with the cancel taken off (context.WithoutCancel,
// as a rollback or a close passes it) goes in.
func journalCancelled(t *testing.T, s JournalSetup) {
	t.Helper()
	if err := s.J.Append(t.Context(), s.Root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, _, err := s.J.Records(ctx, s.Root); err == nil {
		t.Errorf("Records with the context cancelled = %+v, want an error", got)
	}
	if err := s.J.Append(ctx, s.Root, model.Record{Run: "run-1", Kind: model.KindClose}); err == nil {
		t.Error("Append of a later record with the context cancelled = nil, want an error")
	}
	if got, _, err := s.J.Records(t.Context(), s.Root); err != nil || len(got) != 1 {
		t.Fatalf("Records after the refused record = %+v, %v, want the intent alone", got, err)
	}
	if err := s.J.Append(context.WithoutCancel(ctx), s.Root, model.Record{Run: "run-1", Kind: model.KindClose}); err != nil {
		t.Fatalf("Append with the cancel taken off = %v", err)
	}
	got, _, err := s.J.Records(t.Context(), s.Root)
	if err != nil || len(got) != 2 || got[1].Kind != model.KindClose {
		t.Errorf("Records = %+v, %v, want the intent and the close", got, err)
	}
}

// journalBytesAgree: RecordsOf over the bytes of Root's
// journal file answers what Records answers of Root, record for record and
// field for field, and the same torn count, at each point of two runs
// written interleaved with every fixed field; no bytes are no records.
func journalBytesAgree(t *testing.T, s JournalSetup) {
	t.Helper()
	if got, torn, err := s.J.RecordsOf(t.Context(), s.Bytes()); err != nil || len(got) != 0 || torn != 0 {
		t.Fatalf("RecordsOf of no journal = %+v, %d, %v, want none, 0 and no error", got, torn, err)
	}
	for i, r := range []model.Record{
		fixed("run-a", model.KindIntent), {Run: "run-b", Kind: model.KindIntent},
		fixed("run-a", model.KindCopy), fixed("run-b", model.KindStep),
		fixed("run-a", model.KindClose), {Run: "run-b", Kind: model.KindClose},
	} {
		if err := s.J.Append(t.Context(), s.Root, r); err != nil {
			t.Fatalf("Append(%s, %s) = %v", r.Run, r.Kind, err)
		}
		want, wantTorn, err := s.J.Records(t.Context(), s.Root)
		if err != nil {
			t.Fatal(err)
		}
		got, torn, err := s.J.RecordsOf(t.Context(), s.Bytes())
		if err != nil || torn != wantTorn || len(got) != len(want) {
			t.Fatalf("after record %d, RecordsOf = %d records, %d torn, %v; Records = %d, %d", i+1, len(got), torn, err, len(want), wantTorn)
		}
		for n := range want {
			if a, b := shownRecord(got[n]), shownRecord(want[n]); a != b {
				t.Errorf("after record %d, record %d of the bytes is %s, and of Records %s", i+1, n+1, a, b)
			}
		}
	}
}

// shownRecord is every field of r, its detail as text.
func shownRecord(r model.Record) string {
	d := r.Detail
	r.Detail = nil
	return fmt.Sprintf("%+v detail %q", r, d)
}

func journalNoDetail(t *testing.T, s JournalSetup) {
	t.Helper()
	if err := s.J.Append(t.Context(), s.Root, model.Record{Run: "run-1", Kind: model.KindClose}); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.J.Records(t.Context(), s.Root)
	if err != nil || len(got) != 1 || len(got[0].Detail) != 0 {
		t.Errorf("Records = %+v, %v, want one record with no detail", got, err)
	}
}

func journalSeparate(t *testing.T, s JournalSetup) {
	t.Helper()
	if err := s.J.Append(t.Context(), s.Root, fixed("run-1", model.KindIntent)); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.J.Records(t.Context(), s.Other)
	if err != nil || len(got) != 0 {
		t.Errorf("Records of the other root = %+v, %v, want none", got, err)
	}
}

func journalRefuses(t *testing.T, build JournalMaker) {
	t.Helper()
	bad := map[string]model.Record{
		"no run":              fixed("", model.KindIntent),
		"a run with a slash":  fixed("a/b", model.KindIntent),
		"a run that climbs":   fixed("..", model.KindIntent),
		"a run with a space":  fixed("a b", model.KindIntent),
		"a run of 65 letters": fixed(strings.Repeat("a", 65), model.KindIntent),
		"an unknown kind":     fixed("run-1", "novel"),
		"no kind":             fixed("run-1", ""),
		"a half detail":       {Run: "run-1", Kind: model.KindStep, Detail: []byte(`{"a":`)},
	}
	if len(bad) == 0 {
		t.Fatal("no refused record")
	}
	for name, r := range bad {
		t.Run("a record with "+name+" is refused and writes nothing", func(t *testing.T) {
			s := build(t)
			if err := s.J.Append(t.Context(), s.Root, r); err == nil {
				t.Errorf("Append(%+v) was taken", r)
			}
			if got, _, err := s.J.Records(t.Context(), s.Root); err != nil || len(got) != 0 {
				t.Errorf("Records after the refusal = %+v, %v, want none", got, err)
			}
		})
	}
}

// copies are three copies of two files under two runs.
func copies() []struct{ run, path, old string } {
	return []struct{ run, path, old string }{
		{"run-1", "rules/a.md", "one"}, {"run-1", "rules/b.md", "two"}, {"run-2", "rules/a.md", "three"},
	}
}

func copyKeptByRunAndPath(t *testing.T, s JournalSetup) {
	t.Helper()
	for _, c := range copies() {
		if err := s.J.Keep(s.Root, c.run, c.path, []byte(c.old)); err != nil {
			t.Fatalf("Keep(%s, %s) = %v", c.run, c.path, err)
		}
	}
	for _, c := range copies() {
		if got, err := s.J.Kept(s.Root, c.run, c.path); err != nil || string(got) != c.old {
			t.Errorf("Kept(%s, %s) = %q, %v, want %q", c.run, c.path, got, err, c.old)
		}
	}
}

func copyMissing(t *testing.T, s JournalSetup) {
	t.Helper()
	if err := s.J.Keep(s.Root, "run-1", "a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		root      model.Root
		run, path string
	}{{s.Root, "run-2", "a"}, {s.Root, "run-1", "b"}, {s.Other, "run-1", "a"}} {
		if _, err := s.J.Kept(c.root, c.run, c.path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Kept(%s, %s, %s) = %v, want fs.ErrNotExist", c.root, c.run, c.path, err)
		}
		if at, err := s.J.KeptAt(c.root, c.run, c.path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("KeptAt(%s, %s, %s) = %q, %v, want fs.ErrNotExist", c.root, c.run, c.path, at, err)
		}
	}
}

// copyAt: where each copy lies is an absolute, clean path outside both
// roots, the same when asked again, and another for another run or path.
func copyAt(t *testing.T, s JournalSetup) {
	t.Helper()
	seen := map[string]string{}
	for _, c := range copies() {
		if err := s.J.Keep(s.Root, c.run, c.path, []byte(c.old)); err != nil {
			t.Fatalf("Keep(%s, %s) = %v", c.run, c.path, err)
		}
	}
	for _, c := range copies() {
		at, err := s.J.KeptAt(s.Root, c.run, c.path)
		if err != nil || !filepath.IsAbs(at) || filepath.Clean(at) != at {
			t.Errorf("KeptAt(%s, %s) = %q, %v, want an absolute, clean path", c.run, c.path, at, err)
			continue
		}
		// A copy no relative path from the root reaches, as on another
		// volume, is outside it.
		for _, r := range []model.Root{s.Root, s.Other} {
			if in, err := filepath.Rel(string(r), at); err == nil && !strings.HasPrefix(in, "..") {
				t.Errorf("KeptAt(%s, %s) = %s, inside the root %s", c.run, c.path, at, r)
			}
		}
		if again, err := s.J.KeptAt(s.Root, c.run, c.path); err != nil || again != at {
			t.Errorf("KeptAt(%s, %s) = %s, then %s, %v", c.run, c.path, at, again, err)
		}
		if other, dup := seen[at]; dup {
			t.Errorf("KeptAt(%s, %s) = %s, as for %s", c.run, c.path, at, other)
		}
		seen[at] = c.run + " " + c.path
	}
}

func copyEmpty(t *testing.T, s JournalSetup) {
	t.Helper()
	if err := s.J.Keep(s.Root, "run-1", "a", nil); err != nil {
		t.Fatal(err)
	}
	if got, err := s.J.Kept(s.Root, "run-1", "a"); err != nil || len(got) != 0 {
		t.Errorf("Kept of an empty copy = %q, %v, want an empty slice", got, err)
	}
}

func copyIsolated(t *testing.T, s JournalSetup) {
	t.Helper()
	old := []byte("before")
	if err := s.J.Keep(s.Root, "run-1", "a", old); err != nil {
		t.Fatal(err)
	}
	copy(old, "AFTER!")
	got, err := s.J.Kept(s.Root, "run-1", "a")
	if err != nil || !slices.Equal(got, []byte("before")) {
		t.Fatalf("Kept = %q, %v, want the bytes as they were kept", got, err)
	}
	got[0] = 'X'
	if again, err := s.J.Kept(s.Root, "run-1", "a"); err != nil || string(again) != "before" {
		t.Errorf("Kept after the caller changed its answer = %q, %v, want before", again, err)
	}
}

func copyBadRun(t *testing.T, s JournalSetup) {
	t.Helper()
	runs := []string{"", "a/b", "..", strings.Repeat("a", 65)}
	if len(runs) == 0 {
		t.Fatal("no run to refuse")
	}
	for _, run := range runs {
		if err := s.J.Keep(s.Root, run, "a", []byte("x")); err == nil {
			t.Errorf("Keep under the run %q was taken", run)
		}
		if at, err := s.J.KeptAt(s.Root, run, "a"); err == nil || errors.Is(err, fs.ErrNotExist) {
			t.Errorf("KeptAt under the run %q = %q, %v, want it refused", run, at, err)
		}
	}
}

func copyTwice(t *testing.T, s JournalSetup) {
	t.Helper()
	if err := s.J.Keep(s.Root, "run-1", "rules/a.md", []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := s.J.Keep(s.Root, "run-1", "rules/a.md", []byte("replaced")); err == nil {
		t.Error("a second Keep for the same run and path was taken")
	}
	if got, err := s.J.Kept(s.Root, "run-1", "rules/a.md"); err != nil || string(got) != "original" {
		t.Errorf("Kept after the refused second copy = %q, %v, want the first bytes", got, err)
	}
	if err := s.J.Keep(s.Root, "run-2", "rules/a.md", []byte("other run")); err != nil {
		t.Errorf("a copy of the same path under another run = %v, want it kept", err)
	}
}

// ran writes the records of run, of kinds, to root, and keeps one copy under
// it holding the run's name.
func ran(t *testing.T, s JournalSetup, root model.Root, run string, kinds ...string) {
	t.Helper()
	for _, k := range kinds {
		if err := s.J.Append(t.Context(), root, model.Record{Run: run, Kind: k}); err != nil {
			t.Fatalf("Append(%s, %s) = %v", run, k, err)
		}
	}
	if err := s.J.Keep(root, run, "rules/a.md", []byte(run)); err != nil {
		t.Fatalf("Keep(%s) = %v", run, err)
	}
}

// runsIn is each run of root's records once, in the order first written.
func runsIn(t *testing.T, s JournalSetup, root model.Root) []string {
	t.Helper()
	recs, torn, err := s.J.Records(t.Context(), root)
	if err != nil || torn != 0 {
		t.Fatalf("Records = %d torn, %v", torn, err)
	}
	var out []string
	for _, r := range recs {
		if !slices.Contains(out, r.Run) {
			out = append(out, r.Run)
		}
	}
	return out
}

// sweepOld: a run opened 31 days ago and never closed is the control: it
// stays with its copy, as the run closed a moment ago does. Only the run
// closed 31 days ago goes, records and copy, and the other root's run closed
// as long ago stays, since the sweep was of one root.
func sweepOld(t *testing.T, s JournalSetup) {
	t.Helper()
	ran(t, s, s.Root, "open-long-ago", model.KindIntent, model.KindCopy)
	ran(t, s, s.Root, "closed-long-ago", model.KindIntent, model.KindClose)
	ran(t, s, s.Other, "other-root", model.KindIntent, model.KindClose)
	s.Pass(31 * 24 * time.Hour)
	ran(t, s, s.Root, "closed-lately", model.KindIntent, model.KindClose)
	if n, err := s.J.Sweep(t.Context(), s.Root); err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v, want 1 run swept", n, err)
	}
	if got := runsIn(t, s, s.Root); !slices.Equal(got, []string{"open-long-ago", "closed-lately"}) {
		t.Errorf("runs after the sweep = %v, want the open run and the run closed lately", got)
	}
	if _, err := s.J.Kept(s.Root, "closed-long-ago", "rules/a.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the swept run's copy = %v, want fs.ErrNotExist", err)
	}
	for _, c := range []struct {
		root model.Root
		run  string
	}{{s.Root, "open-long-ago"}, {s.Root, "closed-lately"}, {s.Other, "other-root"}} {
		if got, err := s.J.Kept(c.root, c.run, "rules/a.md"); err != nil || string(got) != c.run {
			t.Errorf("Kept(%s) after the sweep = %q, %v, want the copy still there", c.run, got, err)
		}
	}
	if got := runsIn(t, s, s.Other); !slices.Equal(got, []string{"other-root"}) {
		t.Errorf("the other root's runs after a sweep of the first = %v, want its run", got)
	}
	if n, err := s.J.Sweep(t.Context(), s.Root); err != nil || n != 0 {
		t.Errorf("a second Sweep = %d, %v, want nothing left to sweep", n, err)
	}
}

func sweepEmpty(t *testing.T, s JournalSetup) {
	t.Helper()
	if n, err := s.J.Sweep(t.Context(), s.Root); err != nil || n != 0 {
		t.Errorf("Sweep of a root with no journal = %d, %v, want 0 and no error", n, err)
	}
}

// sweepCancelled: a run closed past the keep, swept with the context
// cancelled, is refused and stays, records and copy.
func sweepCancelled(t *testing.T, s JournalSetup) {
	t.Helper()
	ran(t, s, s.Root, "closed-long-ago", model.KindIntent, model.KindClose)
	s.Pass(31 * 24 * time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if n, err := s.J.Sweep(ctx, s.Root); err == nil {
		t.Errorf("Sweep with the context cancelled = %d, nil, want an error", n)
	}
	if got := runsIn(t, s, s.Root); !slices.Equal(got, []string{"closed-long-ago"}) {
		t.Errorf("runs after the refused sweep = %v, want the run still there", got)
	}
	if _, err := s.J.Kept(s.Root, "closed-long-ago", "rules/a.md"); err != nil {
		t.Errorf("the run's copy after the refused sweep = %v, want it still there", err)
	}
}

// journalNeverBegun writes entries of a run that has no intent and no close
// yet: a copy, a step or a start belongs to a run its intent or close begins,
// so before that Records answers none of them and counts each
// as torn, and once the run is begun they read back in the order written,
// ahead of the record that began it. A run begun by an intent is unaffected.
func journalNeverBegun(t *testing.T, s JournalSetup) {
	t.Helper()
	for _, k := range []string{model.KindStart, model.KindStep} {
		if err := s.J.Append(t.Context(), s.Root, fixed("run-n", k)); err != nil {
			t.Fatalf("Append(%s) = %v", k, err)
		}
	}
	got, torn, err := s.J.Records(t.Context(), s.Root)
	if err != nil || len(got) != 0 || torn != 2 {
		t.Fatalf("Records of two entries of a run never begun = %d records, %d torn, %v, want 0, 2 and no error", len(got), torn, err)
	}
	if err := s.J.Append(t.Context(), s.Root, fixed("run-n", model.KindClose)); err != nil {
		t.Fatalf("Append(close) = %v", err)
	}
	got, torn, err = s.J.Records(t.Context(), s.Root)
	if err != nil || torn != 0 || len(got) != 3 {
		t.Fatalf("Records once the run is closed = %d records, %d torn, %v, want 3, 0 and no error", len(got), torn, err)
	}
	for i, k := range []string{model.KindStart, model.KindStep, model.KindClose} {
		if got[i].Kind != k {
			t.Errorf("record %d is a %s, want a %s: the entries read back in the order written, then the close", i, got[i].Kind, k)
		}
	}
}

// journalNeverBegunBytes: the records of the journal's bytes are its Records
// for a run never begun too, before its close and after it.
func journalNeverBegunBytes(t *testing.T, s JournalSetup) {
	t.Helper()
	agree := func(when string) {
		t.Helper()
		want, wantTorn, err := s.J.Records(t.Context(), s.Root)
		if err != nil {
			t.Fatalf("Records %s: %v", when, err)
		}
		got, torn, err := s.J.RecordsOf(t.Context(), s.Bytes())
		if err != nil || torn != wantTorn || len(got) != len(want) {
			t.Fatalf("RecordsOf %s = %d records, %d torn, %v, want the Records' %d and %d", when, len(got), torn, err, len(want), wantTorn)
		}
		for i := range got {
			if got[i].Run != want[i].Run || got[i].Kind != want[i].Kind {
				t.Errorf("RecordsOf %s: record %d is %s %s, want %s %s", when, i, got[i].Run, got[i].Kind, want[i].Run, want[i].Kind)
			}
		}
	}
	for _, k := range []string{model.KindStart, model.KindStep} {
		if err := s.J.Append(t.Context(), s.Root, fixed("run-n", k)); err != nil {
			t.Fatalf("Append(%s) = %v", k, err)
		}
	}
	agree("before the run is begun")
	if err := s.J.Append(t.Context(), s.Root, fixed("run-n", model.KindClose)); err != nil {
		t.Fatalf("Append(close) = %v", err)
	}
	agree("after the close")
}

// journalNeverBegunSwept: a sweep that removes a run rewrites the journal
// from the runs begun, so the entries of a run never begun are dropped with
// it, and the run's close, written after, reads alone.
func journalNeverBegunSwept(t *testing.T, s JournalSetup) {
	t.Helper()
	if err := s.J.Append(t.Context(), s.Root, fixed("run-n", model.KindStart)); err != nil {
		t.Fatalf("Append(start) = %v", err)
	}
	ran(t, s, s.Root, "closed-long-ago", model.KindIntent, model.KindClose)
	s.Pass(31 * 24 * time.Hour)
	if n, err := s.J.Sweep(t.Context(), s.Root); err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v, want 1 run swept", n, err)
	}
	if err := s.J.Append(t.Context(), s.Root, fixed("run-n", model.KindClose)); err != nil {
		t.Fatalf("Append(close) = %v", err)
	}
	got, torn, err := s.J.Records(t.Context(), s.Root)
	if err != nil || torn != 0 || len(got) != 1 || got[0].Kind != model.KindClose {
		t.Fatalf("Records after the sweep and the close = %d records, %d torn, %v, want the close alone", len(got), torn, err)
	}
}
