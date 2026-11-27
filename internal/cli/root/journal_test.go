package root

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

const clock = 1_800_000_000

func dig(s string) string {
	return digest.Prefixed([]byte(s))
}

// journalProcess is a run of args over a working folder holding files and a
// journal written by hand.
type journalProcess struct {
	t    *testing.T
	p    Process
	sc   screens
	root model.Root
}

func newJournal(t *testing.T, files map[string]string, args ...string) *journalProcess {
	t.Helper()
	p, _ := process(t, world(t), args...)
	p.Config.Settings = config.Defaults()
	p.Config.StateDir = fakeState()
	p, sc := attach(t, p)
	sc.out.Instant(time.Unix(clock, 0))
	return &journalProcess{t: t, p: p, sc: sc, root: sc.out.Seed(".", files)}
}

func (j *journalProcess) rec(run, kind string, at int64, detail []byte) {
	j.t.Helper()
	err := j.p.World.Journal.Append(j.t.Context(), j.root, model.Record{Run: run, Kind: kind, At: at, Command: "gragas gen --apply", Plan: dig("plan " + run), Root: "ab12cd34ef56ab12", Detail: detail})
	if err != nil {
		j.t.Fatal(err)
	}
}

func (j *journalProcess) copied(run, path, before string, existed bool, at int64) {
	j.t.Helper()
	d, old := "", []byte(nil)
	if existed {
		d, old = dig(before), []byte(before)
	}
	if err := j.p.World.Journal.Keep(j.root, run, path, old); err != nil {
		j.t.Fatal(err)
	}
	j.rec(run, model.KindCopy, at, []byte(fmt.Sprintf(`{"path":%q,"existed":%t,"before":%q}`, path, existed, d)))
}

func (j *journalProcess) closed(run, outcome, path, before, after string, at int64) {
	j.t.Helper()
	files := ""
	if path != "" {
		files = fmt.Sprintf(`{"path":%q,"before":%q,"after":%q}`, path, dig(before), dig(after))
	}
	j.rec(run, model.KindClose, at, []byte(fmt.Sprintf(`{"outcome":%q,"file":"","files":[%s]}`, outcome, files)))
}

// oneRun is r1, closed, that replaced a.txt "old" with "new".
func (j *journalProcess) oneRun() {
	j.rec("r1", model.KindIntent, clock-300, nil)
	j.copied("r1", "a.txt", "old", true, clock-299)
	j.closed("r1", "done", "a.txt", "old", "new", clock-290)
}

func (j *journalProcess) run() int { return Execute(j.t.Context(), j.p) }

// told is everything the run said, for a failure message: the closing word,
// standard output, then standard error, each named, so a run on another
// system that says nothing on one still shows what it said on the other.
func (j *journalProcess) told() string {
	return fmt.Sprintf("closing %q\nstdout:\n%s\nstderr:\n%s", j.sc.closing(), j.sc.output(), strings.Join(j.sc.diag.Said, "\n"))
}

func (j *journalProcess) file(p string) string {
	s, _ := j.sc.out.File(j.root, p)
	return s
}

// onlyReads is a Writer that fails the test on any call.
type onlyReads struct {
	t *testing.T
	port.Writer
}

func (o onlyReads) Replace(model.Root, string, []byte) error {
	o.t.Error("Writer.Replace was called")
	return errors.New("no")
}
func (o onlyReads) Append(model.Root, string, []byte) error {
	o.t.Error("Writer.Append was called")
	return errors.New("no")
}
func (o onlyReads) Remove(model.Root, string) error {
	o.t.Error("Writer.Remove was called")
	return errors.New("no")
}

// TestJournalListEndsDoneOnEmptyAndOnRuns: list over a folder with no journal
// and over one with a closed and an open run each end done on exit 0, and the
// rows carry the run, its state and its command.
func TestJournalListEndsDoneOnEmptyAndOnRuns(t *testing.T) {
	t.Parallel()
	e := newJournal(t, nil, "journal", "list")
	if code := e.run(); code != exit.Done || e.sc.closing() != model.Done || !strings.Contains(e.sc.output(), "no run has written") {
		t.Fatalf("empty: exit %d, said:\n%s", code, e.sc.output())
	}
	j := newJournal(t, map[string]string{"a.txt": "new"}, "journal", "list")
	j.oneRun()
	j.rec("r2", model.KindIntent, clock-100, nil)
	if code := j.run(); code != exit.Done {
		t.Fatalf("exit %d, said:\n%s", code, j.sc.output())
	}
	out := j.sc.output()
	if !strings.Contains(out, "r1 done gragas gen --apply 1") || !strings.Contains(out, "r2 open") || !strings.Contains(out, "2 runs listed") {
		t.Errorf("the listing is:\n%s", out)
	}
}

// TestRestoreHasNoApplyAndCallsNoWriterMethod: restore takes no --apply (the
// flag is a usage error), and a run of it over a Writer that fails on any call
// ends done. Risk: a restore that writes (cli.commands.25).
func TestRestoreHasNoApplyAndCallsNoWriterMethod(t *testing.T) {
	t.Parallel()
	if c, _, err := tree(t).Find([]string{"journal", "restore"}); err != nil || c.Flags().Lookup("apply") != nil {
		t.Fatalf("restore: %v, apply flag %v, want a command with no --apply", err, c.Flags().Lookup("apply"))
	}
	bad := newJournal(t, map[string]string{"a.txt": "new"}, "journal", "restore", "r1", "--apply")
	bad.oneRun()
	if code := bad.run(); code != exit.Usage {
		t.Errorf("restore --apply: exit %d, want usage %d", code, exit.Usage)
	}
	j := newJournal(t, map[string]string{"a.txt": "new"}, "journal", "restore", "r1")
	j.oneRun()
	j.p.World.Writer = onlyReads{t, j.p.World.Writer}
	kept, err := j.p.World.Journal.KeptAt(j.root, "r1", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if code := j.run(); code != exit.Done || !strings.Contains(j.sc.output(), "cp -- ") || !strings.Contains(j.sc.output(), kept) || !strings.Contains(j.sc.output(), " a.txt") {
		t.Errorf("exit %d, said:\n%s\nwant a cp line for a.txt, then %s (state %s)", code, j.told(), kept, fakeState())
	}
}

// TestRestoreOfAMovedFileEndsRefusedOnExit5: a.txt changed again after r1, by
// r2, so r1's restore is refused (exit 5, the word refused), names r2 and
// gives the line to type; the clean control above ends done.
func TestRestoreOfAMovedFileEndsRefusedOnExit5(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{"a.txt": "newer"}, "journal", "restore", "r1")
	j.oneRun()
	j.rec("r2", model.KindIntent, clock-200, nil)
	j.copied("r2", "a.txt", "new", true, clock-199)
	j.closed("r2", "done", "a.txt", "new", "newer", clock-190)
	code := j.run()
	out := j.sc.output()
	if code != exit.Refused || j.sc.closing() != model.Refused || !strings.Contains(out, "run r2 changed it since") || !strings.Contains(out, "gragas journal restore r2") {
		t.Errorf("exit %d, closing %q, said:\n%s", code, j.sc.closing(), out)
	}
	if strings.Contains(out, "cp ") {
		t.Errorf("a refused restore printed a line:\n%s", out)
	}
}

// TestRestoreOfAnOpenOrUnknownRunIsRefusedWithTheLineToType.
func TestRestoreOfAnOpenOrUnknownRunIsRefusedWithTheLineToType(t *testing.T) {
	t.Parallel()
	for run, want := range map[string]string{"r9": "gragas journal list", "r2": "gragas journal recover --apply"} {
		j := newJournal(t, map[string]string{"a.txt": "half"}, "journal", "restore", run)
		j.rec("r2", model.KindIntent, clock-100, nil)
		if code := j.run(); code != exit.Refused || !strings.Contains(j.sc.output(), want) {
			t.Errorf("restore %s: exit %d, said:\n%s\nwant refused with %q", run, code, j.sc.output(), want)
		}
	}
}

func (j *journalProcess) openRun() {
	j.rec("r1", model.KindIntent, clock-100, []byte(`{"files":["a.txt","b.txt"]}`))
	j.copied("r1", "a.txt", "old", true, clock-99)
	// The roll-back puts a file back only if it holds its before or what a step
	// of the run left, so the run's one step is the "half" the tests plant.
	j.rec("r1", model.KindStep, clock-98, []byte(fmt.Sprintf(`{"path":"a.txt","op":"write","after":%q}`, dig("half"))))
}

// TestRecoverDryRunWritesNothingAndEndsOnTheApplyLine: without --apply the
// open run is listed, the file is untouched, no Writer method is called and
// the screen ends on the apply line.
func TestRecoverDryRunWritesNothingAndEndsOnTheApplyLine(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{"a.txt": "half"}, "journal", "recover")
	j.openRun()
	j.p.World.Writer = onlyReads{t, j.p.World.Writer}
	code := j.run()
	out := j.sc.output()
	if code != exit.Done || !strings.Contains(out, "roll back  run r1") || !strings.Contains(out, "Add --apply") || !strings.Contains(out, "gragas journal recover --apply") {
		t.Errorf("exit %d, said:\n%s", code, out)
	}
	if j.file("a.txt") != "half" {
		t.Error("the dry run changed a.txt")
	}
}

// TestRecoverApplyRollsBackNeverResumesAndRecordsTheRun: --apply puts a.txt
// back, makes no b.txt, ends done and opens a run record.
func TestRecoverApplyRollsBackNeverResumesAndRecordsTheRun(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{"a.txt": "half"}, "journal", "recover", "--apply")
	j.openRun()
	code := j.run()
	if code != exit.Done || j.sc.closing() != model.Done || !strings.Contains(j.sc.output(), "1 run rolled back") {
		t.Fatalf("exit %d, said:\n%s", code, j.sc.output())
	}
	if _, made := j.sc.out.File(j.root, "b.txt"); j.file("a.txt") != "old" || made {
		t.Errorf("a.txt %q, b.txt made %v, want the copy and no b.txt", j.file("a.txt"), made)
	}
}

// TestJournalAnswersCarryNoFolderAndTheContractField: --json documents hold
// the schema number and the root digest and none of the state folder's or
// the working folder's path.
func TestJournalAnswersCarryNoFolderAndTheContractField(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"journal", "list", "--json"}, {"journal", "restore", "r1", "--json"}} {
		j := newJournal(t, map[string]string{"a.txt": "new"}, args...)
		j.oneRun()
		var out strings.Builder
		j.p.Stdout = &out
		if code := j.run(); code != exit.Done {
			t.Fatalf("%v: exit %d", args, code)
		}
		doc := out.String()
		if !strings.Contains(doc, `"schema": 1`) || !strings.Contains(doc, "ab12cd34ef56ab12") || strings.Contains(doc, "/work") {
			t.Errorf("%v: document is\n%s", args, doc)
		}
	}
}

// clearOld is a journal with a closed run past the keep age.
func (j *journalProcess) clearOld() {
	old := int64(clock - 90*86400)
	j.rec("old1", model.KindIntent, old, nil)
	j.closed("old1", "done", "a.txt", "x", "y", old+1)
	j.rec("open1", model.KindIntent, old, nil)
}

// TestCacheClearDryRunWritesNothingAndEndsOnTheApplyLine.
func TestCacheClearDryRunWritesNothingAndEndsOnTheApplyLine(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{"a.txt": "x"}, "cache", "clear")
	j.clearOld()
	j.p.World.Writer = onlyReads{t, j.p.World.Writer}
	code := j.run()
	out := j.sc.output()
	if code != exit.Done || !strings.Contains(out, "remove     run old1") || !strings.Contains(out, "gragas cache clear --apply") || !strings.Contains(out, "Add --apply") {
		t.Errorf("exit %d, said:\n%s", code, out)
	}
	if recs, _, _ := j.p.World.Journal.Records(t.Context(), j.root); len(recs) != 3 {
		t.Errorf("the dry run left %d records, want the 3 it found", len(recs))
	}
}

// TestCacheClearApplyKeepsOpenRunsAndASecondRunChangesNothing, and every file
// it keeps or locks is a row of the persisted table.
func TestCacheClearApplyKeepsOpenRunsAndASecondRunChangesNothing(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "cache", "clear", "--apply")
	p.Config.Settings, p.Config.StateDir = config.Defaults(), fakeState()
	p, sc := attach(t, p)
	sc.out.Instant(time.Unix(clock, 0))
	j := &journalProcess{t: t, p: p, sc: sc, root: sc.out.Seed(".", map[string]string{"a.txt": "x"})}
	j.clearOld()
	if code := j.run(); code != exit.Done || !strings.Contains(sc.output(), "1 run removed") {
		t.Fatalf("exit %d, said:\n%s", code, sc.output())
	}
	recs, _, _ := p.World.Journal.Records(t.Context(), j.root)
	for _, r := range recs {
		if r.Run == "old1" {
			t.Errorf("old1 is still in the journal: %+v", r)
		}
	}
	if len(recs) != 1 || recs[0].Run != "open1" {
		t.Errorf("records left %d, want the open run's intent alone", len(recs))
	}
	if missing, checked := unheld(config.Persisted(), p.Config, f); len(missing) != 0 || checked < 2 {
		t.Errorf("not in the table: %v (checked %d)", missing, checked)
	}
	for _, name := range []string{"0123abcd.journal", "0123abcd.copies/r1/ff00"} {
		if _, ok := config.Holds(config.Persisted(), config.InState, name); !ok {
			t.Errorf("the table does not hold %s, which a sweep removes or rewrites", name)
		}
	}
	sc.out.Shown = nil
	if code := j.run(); code != exit.Done || !strings.Contains(sc.output(), "0 runs removed") {
		t.Errorf("second clear: exit %d, said:\n%s", code, sc.output())
	}
	if after, _, _ := p.World.Journal.Records(t.Context(), j.root); len(after) != 1 {
		t.Errorf("the second clear left %d records, want 1", len(after))
	}
}

// TestRestoreOfADamagedCopyEndsRefusedOnExit5: the copy r1 kept of a.txt holds
// other bytes than its record names, so the restore is refused (exit 5, the
// word refused), names the file and the copy, and prints no cp line; the clean
// control above ends done. Risk: a damaged restore that falls through to done.
func TestRestoreOfADamagedCopyEndsRefusedOnExit5(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{"a.txt": "new"}, "journal", "restore", "r1")
	j.rec("r1", model.KindIntent, clock-300, nil)
	if err := j.p.World.Journal.Keep(j.root, "r1", "a.txt", []byte("other bytes")); err != nil {
		t.Fatal(err)
	}
	j.rec("r1", model.KindCopy, clock-299, []byte(fmt.Sprintf(`{"path":"a.txt","existed":true,"before":%q}`, dig("old"))))
	j.closed("r1", "done", "a.txt", "old", "new", clock-290)
	code := j.run()
	out := j.sc.output()
	if code != exit.Refused || j.sc.closing() != model.Refused || !strings.Contains(out, "a.txt") || !strings.Contains(out, ".copies/r1/") {
		t.Errorf("exit %d, closing %q, said:\n%s", code, j.sc.closing(), out)
	}
	if strings.Contains(out, "cp ") || strings.Contains(out, "rm ") {
		t.Errorf("a refused restore printed a line:\n%s", out)
	}
}
