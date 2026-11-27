package coreapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/shell"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/canon"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The project's folder name is something the journal's answers must never
// carry: the journal speaks of a root by its digest.
const workDir = "/work/the-workDir-project"

const rootDigest = "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f"

func journalDigestOf(s string) string { return string(apply.DigestBytes([]byte(s))) }

func object(o canon.Object) []byte {
	b, err := canon.Encode(o, canon.Document)
	if err != nil {
		panic(err)
	}
	return b
}

// journalWorld is a root holding files and a journal of the records a test
// writes by hand, as the write path wrote them.
type journalWorld struct {
	t     *testing.T
	w     *porttest.World
	root  model.Root
	ports port.Ports
}

func newJournalWorld(t *testing.T, files map[string]string) *journalWorld {
	t.Helper()
	w := porttest.NewWorld(t)
	w.Instant(time.Unix(now, 0))
	w.SeedTerminal(porttest.Term{})
	root := w.Seed(workDir, files)
	return &journalWorld{t: t, w: w, root: root, ports: w.Ports()}
}

func (j *journalWorld) record(run, kind string, at int64, detail []byte) {
	j.t.Helper()
	err := j.ports.Journal.Append(j.t.Context(), j.root, model.Record{Run: run, Kind: kind, At: at, Command: "gragas test", Plan: "sha256:plan-" + run, Root: rootDigest, Engine: "t", Detail: detail})
	if err != nil {
		j.t.Fatal(err)
	}
}

// copied journals the copy of path before a run replaces it.
func (j *journalWorld) copied(run, path, before string, existed bool, at int64) {
	j.t.Helper()
	d := ""
	var old []byte
	if existed {
		d, old = journalDigestOf(before), []byte(before)
	}
	if err := j.ports.Journal.Keep(j.root, run, path, old); err != nil {
		j.t.Fatal(err)
	}
	// A copy of a run its intent has not begun is torn on the host journal,
	// so the run's intent is written first, as the write path writes it.
	recs, _, err := j.ports.Journal.Records(j.t.Context(), j.root)
	if err != nil {
		j.t.Fatal(err)
	}
	if !slices.ContainsFunc(recs, func(r model.Record) bool { return r.Run == run }) {
		j.record(run, model.KindIntent, at, nil)
	}
	j.record(run, model.KindCopy, at, object(canon.Object{"path": canon.String(path), "existed": canon.Bool(existed), "before": canon.String(d)}))
}

// closed journals the close of a run that wrote after to path.
func (j *journalWorld) closed(run, outcome, path, before, after string, at int64) {
	j.t.Helper()
	files := canon.Array{}
	if path != "" {
		b := ""
		if before != "" {
			b = journalDigestOf(before)
		}
		files = canon.Array{canon.Object{"path": canon.String(path), "before": canon.String(b), "after": canon.String(journalDigestOf(after))}}
	}
	j.record(run, model.KindClose, at, object(canon.Object{"outcome": canon.String(outcome), "file": canon.String(""), "files": files}))
}

func (j *journalWorld) file(p string) string {
	j.t.Helper()
	s, _ := j.w.File(j.root, p)
	return s
}

// A run r1 replaced a.txt "old" with "new" and closed; the file holds "new".
func oneClosedRun(t *testing.T) *journalWorld {
	t.Helper()
	j := newJournalWorld(t, map[string]string{"a.txt": "new"})
	j.record("r1", model.KindIntent, now-300, nil)
	j.copied("r1", "a.txt", "old", true, now-299)
	j.closed("r1", "done", "a.txt", "old", "new", now-290)
	return j
}

func restoreAsk(run string) coreapi.RestoreAsk {
	return coreapi.RestoreAsk{Run: run, StateDir: "/state"}
}

// keptAt is where the journal port says the copy of p under run lies.
func (j *journalWorld) keptAt(run, p string) string {
	j.t.Helper()
	at, err := j.ports.Journal.KeptAt(j.root, run, p)
	if err != nil {
		j.t.Fatal(err)
	}
	return at
}

// refuseWriting is a Writer that fails the test on any call.
type refuseWriting struct {
	t *testing.T
	port.Writer
}

func (r refuseWriting) Replace(model.Root, string, []byte) error {
	r.t.Error("Writer.Replace was called")
	return errors.New("no writes")
}
func (r refuseWriting) Append(model.Root, string, []byte) error {
	r.t.Error("Writer.Append was called")
	return errors.New("no writes")
}
func (r refuseWriting) Remove(model.Root, string) error {
	r.t.Error("Writer.Remove was called")
	return errors.New("no writes")
}

// readOnlyJournal is a Journal that fails the test on a write or a sweep.
type readOnlyJournal struct {
	t *testing.T
	port.Journal
}

func (r readOnlyJournal) Append(context.Context, model.Root, model.Record) error {
	r.t.Error("Journal.Append was called")
	return errors.New("no writes")
}
func (r readOnlyJournal) Keep(model.Root, string, string, []byte) error {
	r.t.Error("Journal.Keep was called")
	return errors.New("no writes")
}
func (r readOnlyJournal) Sweep(context.Context, model.Root) (int, error) {
	r.t.Error("Journal.Sweep was called")
	return 0, errors.New("no writes")
}

// noSteps is a Steps that fails the test on any call: a journal answer is
// built from the record, never from a tool a step runs.
type noSteps struct {
	t *testing.T
	port.Steps
}

func (n noSteps) Bind(context.Context, plugin.Scope) (port.StepRunner, error) {
	n.t.Error("Steps.Bind was called")
	return nil, errors.New("no tool")
}

func readOnly(t *testing.T, p port.Ports) port.Ports {
	t.Helper()
	p.Writer = refuseWriting{t, p.Writer}
	p.Journal = readOnlyJournal{t, p.Journal}
	p.Steps = noSteps{t, p.Steps}
	return p
}

// TestListBuildsEachRunFromItsRecord: a closed run, a rolled-back one and an
// open one are listed with their command, plan digest, state and files, and
// the answer holds the root's digest and not its folder. Risk: a state read
// from anything but the record, or a project's name in the answer.
func TestListBuildsEachRunFromItsRecord(t *testing.T) {
	t.Parallel()
	j := oneClosedRun(t)
	j.record("r2", model.KindIntent, now-200, nil)
	j.copied("r2", "b.txt", "", false, now-199)
	j.record("r2", model.KindClose, now-190, object(canon.Object{"outcome": canon.String("rolled back"), "file": canon.String(""), "files": canon.Array{}}))
	j.record("r3", model.KindIntent, now-100, nil)
	j.copied("r3", "a.txt", "new", true, now-99)
	got, err := coreapi.JournalList(t.Context(), readOnly(t, j.ports), j.root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != coreapi.JournalSchema || got.Root != rootDigest || len(got.Runs) != 3 {
		t.Fatalf("schema %d root %q runs %d, want the schema, the root digest and 3 runs", got.Schema, got.Root, len(got.Runs))
	}
	want := []struct{ run, state string }{{"r1", "done"}, {"r2", "rolled back"}, {"r3", "open"}}
	for i, w := range want {
		r := got.Runs[i]
		if r.Run != w.run || r.State != w.state || r.Command != "gragas test" || r.Plan != "sha256:plan-"+w.run {
			t.Errorf("run %d is %+v, want %s %s with its command and plan digest", i, r, w.run, w.state)
		}
	}
	if !slices.Equal(got.Runs[0].Files, []string{"a.txt"}) || !slices.Equal(got.Runs[1].Files, []string{"b.txt"}) {
		t.Errorf("files are %v and %v, want the paths of each run's copy records", got.Runs[0].Files, got.Runs[1].Files)
	}
	if strings.Contains(textOf(got), "the-workDir-project") {
		t.Errorf("the listing names the project: %v", got)
	}
}

func textOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestListOfARootWithNoJournalIsEmptyNotAnError.
func TestListOfARootWithNoJournalIsEmptyNotAnError(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, nil)
	got, err := coreapi.JournalList(t.Context(), j.ports, j.root)
	if err != nil || len(got.Runs) != 0 || got.Runs == nil {
		t.Fatalf("got %+v, %v, want an empty list that is not null", got, err)
	}
}

// TestRestoreCleanControlPrintsTheLinesAndWritesNothing: a file the run
// changed and nobody touched since gets a line that puts its copy back, and a
// file the run made gets a line that removes it. No Writer, Journal write or
// Tool call is made. This is the control of the moved-file refusal.
func TestRestoreCleanControlPrintsTheLinesAndWritesNothing(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"a.txt": "new", "b.txt": "made"})
	j.record("r1", model.KindIntent, now-300, nil)
	j.copied("r1", "a.txt", "old", true, now-299)
	j.copied("r1", "b.txt", "", false, now-298)
	j.record("r1", model.KindClose, now-290, object(canon.Object{"outcome": canon.String("done"), "file": canon.String(""), "files": canon.Array{
		canon.Object{"path": canon.String("a.txt"), "before": canon.String(journalDigestOf("old")), "after": canon.String(journalDigestOf("new"))},
		canon.Object{"path": canon.String("b.txt"), "before": canon.String(""), "after": canon.String(journalDigestOf("made"))},
	}}))
	got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
	if err != nil || len(got.Moved) != 0 {
		t.Fatalf("restore: %v, moved %v, want none", err, got.Moved)
	}
	want := []string{quoted(t, "cp", "--", j.keptAt("r1", "a.txt"), "a.txt"), quoted(t, "rm", "--", "b.txt")}
	slices.Sort(got.Lines)
	slices.Sort(want)
	if !slices.Equal(got.Lines, want) {
		t.Errorf("lines are %q, want %q", got.Lines, want)
	}
	if j.file("a.txt") != "new" || j.file("b.txt") != "made" {
		t.Error("restore changed the tree")
	}
}

// TestRestoreFilesSayAsDataWhatTheirLinesSay: each file of a restore carries
// its op, the kept copy it is written from and the path it is written back to,
// the words of its line, so a reader of --json never parses a shell line.
// Risk: a reader that splits the line to find the copy reads a quoted path
// wrong.
func TestRestoreFilesSayAsDataWhatTheirLinesSay(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"a b.txt": "new", "c.txt": "made"})
	j.record("r1", model.KindIntent, now-300, nil)
	j.copied("r1", "a b.txt", "old", true, now-299)
	j.copied("r1", "c.txt", "", false, now-298)
	j.record("r1", model.KindClose, now-290, object(canon.Object{"outcome": canon.String("done"), "file": canon.String(""), "files": canon.Array{
		canon.Object{"path": canon.String("a b.txt"), "before": canon.String(journalDigestOf("old")), "after": canon.String(journalDigestOf("new"))},
		canon.Object{"path": canon.String("c.txt"), "before": canon.String(""), "after": canon.String(journalDigestOf("made"))},
	}}))
	got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	want := []coreapi.RestoreFile{
		{Path: "c.txt", Op: "remove", From: "", Digest: ""},
		{Path: "a b.txt", Op: "restore", From: j.keptAt("r1", "a b.txt"), Digest: journalDigestOf("old")},
	}
	if !slices.Equal(got.Files, want) {
		t.Fatalf("files are %+v, want %+v", got.Files, want)
	}
	for i, f := range got.Files {
		argv := []string{"rm", "--", f.Path}
		if f.Op == "restore" {
			argv = []string{"cp", "--", f.From, f.Path}
		}
		if line := quoted(t, argv...); got.Lines[i] != line {
			t.Errorf("file %d is %+v, but its line is %q, not %q", i, f, got.Lines[i], line)
		}
	}
}

// TestRestoreInAShellGivenNoRmLineSaysEachStepAsDataOnly (go-console#218):
// where the line maker gives no line for rm or cp (model.ErrOtherProgram, as
// in PowerShell and cmd), the restore is not refused: Files says each step as
// it does in POSIX, and Lines is empty, since Lines are lines to type and a
// reader of --json must never run words. Risk: an rm line pasted where rm is
// an alias or no command, or prose in Lines that a program runs.
func TestRestoreInAShellGivenNoRmLineSaysEachStepAsDataOnly(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"a b.txt": "new", "c.txt": "made"})
	j.record("r1", model.KindIntent, now-300, nil)
	j.copied("r1", "a b.txt", "old", true, now-299)
	j.copied("r1", "c.txt", "", false, now-298)
	j.record("r1", model.KindClose, now-290, object(canon.Object{"outcome": canon.String("done"), "file": canon.String(""), "files": canon.Array{
		canon.Object{"path": canon.String("a b.txt"), "before": canon.String(journalDigestOf("old")), "after": canon.String(journalDigestOf("new"))},
		canon.Object{"path": canon.String("c.txt"), "before": canon.String(""), "after": canon.String(journalDigestOf("made"))},
	}}))
	noOther := func([]string) (string, error) { return "", model.ErrOtherProgram }
	got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), noOther)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	want := []coreapi.RestoreFile{
		{Path: "c.txt", Op: "remove", From: "", Digest: ""},
		{Path: "a b.txt", Op: "restore", From: j.keptAt("r1", "a b.txt"), Digest: journalDigestOf("old")},
	}
	if !slices.Equal(got.Files, want) || got.Lines == nil || len(got.Lines) != 0 {
		t.Errorf("files are %+v and lines %q, want %+v and no line", got.Files, got.Lines, want)
	}
}

// TestRestoreSaysNoLineWhenOnePathHoldsANewline: r1 changed a.txt and made
// a file whose name holds a newline. No pasted line carries that name, so the
// restore is refused whole, names the file, and says no line, not even
// a.txt's. Risk: a restore that prints a.txt's line alone reads as the whole
// way back, or a line a newline splits in two.
func TestRestoreSaysNoLineWhenOnePathHoldsANewline(t *testing.T) {
	t.Parallel()
	odd := "b\nc.txt"
	j := newJournalWorld(t, map[string]string{"a.txt": "new", odd: "made"})
	j.record("r1", model.KindIntent, now-300, nil)
	j.copied("r1", "a.txt", "old", true, now-299)
	j.copied("r1", odd, "", false, now-298)
	j.record("r1", model.KindClose, now-290, object(canon.Object{"outcome": canon.String("done"), "file": canon.String(""), "files": canon.Array{
		canon.Object{"path": canon.String("a.txt"), "before": canon.String(journalDigestOf("old")), "after": canon.String(journalDigestOf("new"))},
		canon.Object{"path": canon.String(odd), "before": canon.String(""), "after": canon.String(journalDigestOf("made"))},
	}}))
	got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
	if !errors.Is(err, coreapi.ErrNoLine) || !errors.Is(err, shell.ErrNewline) || !strings.Contains(err.Error(), strconv.Quote(odd)) || strings.Contains(err.Error(), odd) {
		t.Fatalf("restore: %v, want ErrNoLine naming %q", err, odd)
	}
	if len(got.Lines) != 0 || len(got.Files) != 0 {
		t.Errorf("a refused restore said lines %q and files %v", got.Lines, got.Files)
	}
}

// TestRestoreRefusesAFileWhoseDigestMovedAndNamesTheLaterRun: r1 changed a.txt,
// then r2 changed it again. r1's restore would undo r2's work, so it is
// refused and names r2, and prints no line. Risk: a restore that overwrites
// a later run's change.
func TestRestoreRefusesAFileWhoseDigestMovedAndNamesTheLaterRun(t *testing.T) {
	t.Parallel()
	j := oneClosedRun(t)
	j.record("r2", model.KindIntent, now-200, nil)
	j.copied("r2", "a.txt", "new", true, now-199)
	j.closed("r2", "done", "a.txt", "new", "newer", now-190)
	j.w.Seed(workDir, map[string]string{"a.txt": "newer"})
	got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Moved) != 1 || got.Moved[0].Path != "a.txt" || !slices.Equal(got.Moved[0].By, []string{"r2"}) {
		t.Fatalf("moved is %+v, want a.txt changed by r2", got.Moved)
	}
	if len(got.Lines) != 0 {
		t.Errorf("a refused restore printed %q", got.Lines)
	}
}

// TestRestoreRefusesAFileAPersonChangedAndSaysNoRunDid: the digest moved and
// no later run copied the file, so the change was made outside gragas.
func TestRestoreRefusesAFileAPersonChangedAndSaysNoRunDid(t *testing.T) {
	t.Parallel()
	j := oneClosedRun(t)
	j.w.Seed(workDir, map[string]string{"a.txt": "by hand"})
	got, err := coreapi.JournalRestore(t.Context(), j.ports, j.root, restoreAsk("r1"), posix)
	if err != nil || len(got.Moved) != 1 || len(got.Moved[0].By) != 0 {
		t.Fatalf("got %+v, %v, want one moved file with no run named", got.Moved, err)
	}
}

// TestRestoreOfAnOpenRunPointsAtRecover: an open run has no digest after, so
// a restore cannot tell whether the file moved; it says to recover.
func TestRestoreOfAnOpenRunPointsAtRecover(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"a.txt": "half"})
	j.record("r1", model.KindIntent, now-100, nil)
	j.copied("r1", "a.txt", "old", true, now-99)
	if _, err := coreapi.JournalRestore(t.Context(), j.ports, j.root, restoreAsk("r1"), posix); !errors.Is(err, coreapi.ErrRunOpen) {
		t.Errorf("restore of an open run: %v, want ErrRunOpen", err)
	}
	if _, err := coreapi.JournalRestore(t.Context(), j.ports, j.root, restoreAsk("nope"), posix); !errors.Is(err, coreapi.ErrNoRun) {
		t.Errorf("restore of a run that is not there: %v, want ErrNoRun", err)
	}
}

// TestRecoverRollsAnOpenRunBackAndNeverResumesIt: the intent names a.txt and
// b.txt; only a.txt was written. Recover puts a.txt back, b.txt is never made,
// and the run closes "rolled back". Risk: a recover that finishes the run.
func TestRecoverRollsAnOpenRunBackAndNeverResumesIt(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"a.txt": "half"})
	j.record("r1", model.KindIntent, now-100, object(canon.Object{"files": canon.Array{canon.String("a.txt"), canon.String("b.txt")}}))
	j.copied("r1", "a.txt", "old", true, now-99)
	j.record("r1", model.KindStep, now-98, object(canon.Object{"path": canon.String("a.txt"), "op": canon.String("write"), "after": canon.String(journalDigestOf("half"))}))
	got, err := coreapi.JournalRecover(t.Context(), readOnlySteps(t, j.ports), j.root, coreapi.RecoverAsk{Run: "rec1", Engine: "t"})
	if err != nil || got.Word != model.Done {
		t.Fatalf("recover: %v, word %q, want done", err, got.Word)
	}
	if j.file("a.txt") != "old" {
		t.Errorf("a.txt holds %q, want the copy", j.file("a.txt"))
	}
	if _, made := j.w.File(j.root, "b.txt"); made {
		t.Error("recover made b.txt: it resumed the run")
	}
	if len(got.Recovered) != 1 || got.Recovered[0].Run != "r1" || got.Recovered[0].Completed {
		t.Errorf("recovered %+v, want r1 rolled back", got.Recovered)
	}
	list, _ := coreapi.JournalList(t.Context(), j.ports, j.root)
	if list.Runs[0].State != "rolled back" {
		t.Errorf("r1 is %q after, want rolled back", list.Runs[0].State)
	}
}

func readOnlySteps(t *testing.T, p port.Ports) port.Ports {
	t.Helper()
	p.Steps = noSteps{t, p.Steps}
	return p
}

// TestRecoverWithNothingOpenIsDoneAndWritesNothing.
func TestRecoverWithNothingOpenIsDoneAndWritesNothing(t *testing.T) {
	t.Parallel()
	j := oneClosedRun(t)
	p := j.ports
	p.Writer = refuseWriting{t, p.Writer}
	got, err := coreapi.JournalRecover(t.Context(), p, j.root, coreapi.RecoverAsk{Run: "rec1", Engine: "t"})
	if err != nil || got.Word != model.Done || len(got.Recovered) != 0 {
		t.Fatalf("got %+v, %v, want done with nothing recovered", got, err)
	}
}

// TestRecoverOfARootAnotherRunHoldsIsBusy: the root lock is held, so recover
// ends busy and changes nothing.
func TestRecoverOfARootAnotherRunHoldsIsBusy(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"a.txt": "half"})
	j.record("r1", model.KindIntent, now-100, nil)
	j.copied("r1", "a.txt", "old", true, now-99)
	j.w.HoldLock(j.root, model.RootLock, now-5)
	got, err := coreapi.JournalRecover(t.Context(), j.ports, j.root, coreapi.RecoverAsk{Run: "rec1", Engine: "t"})
	if err != nil || got.Word != model.Busy || j.file("a.txt") != "half" {
		t.Fatalf("got %+v, %v, file %q, want busy and the file as it was", got, err, j.file("a.txt"))
	}
}

// openRuns are runs a clear must keep: one old open run, and one old closed.
func clearWorld(t *testing.T) *journalWorld {
	t.Helper()
	j := newJournalWorld(t, map[string]string{"a.txt": "half"})
	old := int64(now - 100*86400)
	j.record("open1", model.KindIntent, old, nil)
	j.copied("open1", "a.txt", "old", true, old+1)
	j.record("shut1", model.KindIntent, old, nil)
	j.copied("shut1", "a.txt", "older", true, old+1)
	j.closed("shut1", "done", "a.txt", "older", "old", old+2)
	j.record("fresh", model.KindIntent, now-60, nil)
	j.closed("fresh", "done", "", "", "", now-50)
	return j
}

func runsOf(t *testing.T, j *journalWorld) []string {
	t.Helper()
	l, err := coreapi.JournalList(t.Context(), j.ports, j.root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range l.Runs {
		out = append(out, r.Run)
	}
	return out
}

// TestClearRemovesOldClosedRunsAndKeepsEveryOpenOne: the closed run past the
// keep age goes with its copies; the open run, older still, and the fresh
// closed run stay. Risk: clear removing evidence a roll-back needs.
func TestClearRemovesOldClosedRunsAndKeepsEveryOpenOne(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Word != model.Done || got.Removed != 1 {
		t.Fatalf("got %+v, %v, want done with 1 run removed", got, err)
	}
	if r := runsOf(t, j); !slices.Equal(r, []string{"open1", "fresh"}) {
		t.Errorf("runs left are %v, want open1 and fresh", r)
	}
	if _, err := j.ports.Journal.Kept(j.root, "open1", "a.txt"); err != nil {
		t.Errorf("the open run's copy is gone: %v", err)
	}
	if _, err := j.ports.Journal.Kept(j.root, "shut1", "a.txt"); err == nil {
		t.Error("the old closed run's copy is still there")
	}
}

// TestClearTwiceChangesNothingTheSecondTime.
func TestClearTwiceChangesNothingTheSecondTime(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	if _, err := coreapi.CacheClear(t.Context(), j.ports, j.root); err != nil {
		t.Fatal(err)
	}
	before := runsOf(t, j)
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Word != model.Done || got.Removed != 0 || !slices.Equal(runsOf(t, j), before) {
		t.Fatalf("second clear: %+v, %v, runs %v, want none removed and the runs as they were", got, err, runsOf(t, j))
	}
}

// TestClearPlanWritesNothingAndCountsWhatClearWouldRemove: the dry run's
// numbers come from reading alone.
func TestClearPlanWritesNothingAndCountsWhatClearWouldRemove(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	p := j.ports
	p.Writer = refuseWriting{t, p.Writer}
	p.Journal = readOnlyJournal{t, p.Journal}
	got, err := coreapi.CacheClearPlan(t.Context(), p, j.root, 30*86400)
	if err != nil || got.Remove != 1 || got.Open != 1 || got.Keep != 1 {
		t.Fatalf("plan %+v, %v, want remove 1, keep 1, open 1", got, err)
	}
	if !slices.Equal(got.Runs, []string{"shut1"}) {
		t.Errorf("plan names %v, want shut1", got.Runs)
	}
}

// TestClearOfARootAnotherRunHoldsIsBusyAndRemovesNothing.
func TestClearOfARootAnotherRunHoldsIsBusyAndRemovesNothing(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	j.w.HoldLock(j.root, model.RootLock, now-5)
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Word != model.Busy || got.Removed != 0 || len(runsOf(t, j)) != 3 {
		t.Fatalf("got %+v, %v, runs %v, want busy and nothing removed", got, err, runsOf(t, j))
	}
}

// TestRestoreQuotesAPathWithASpace: a line is pasted into a shell, so a path
// with a space or a quote is one word.
func TestRestoreQuotesAPathWithASpace(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"my file.txt": "new", "it's.txt": "made"})
	j.record("r1", model.KindIntent, now-300, nil)
	j.copied("r1", "my file.txt", "old", true, now-299)
	j.copied("r1", "it's.txt", "", false, now-298)
	j.record("r1", model.KindClose, now-290, object(canon.Object{"outcome": canon.String("done"), "file": canon.String(""), "files": canon.Array{
		canon.Object{"path": canon.String("my file.txt"), "before": canon.String(journalDigestOf("old")), "after": canon.String(journalDigestOf("new"))},
		canon.Object{"path": canon.String("it's.txt"), "before": canon.String(""), "after": canon.String(journalDigestOf("made"))},
	}}))
	got, err := coreapi.JournalRestore(t.Context(), j.ports, j.root, restoreAsk("r1"), posix)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got.Lines, "\n")
	if !strings.Contains(joined, "'my file.txt'") || !strings.Contains(joined, `rm -- 'it'\''s.txt'`) {
		t.Errorf("lines are\n%s\nwant each odd path as one quoted word", joined)
	}
}

// TestRestoreRefusesACopyThatIsMissingOrNoLongerHoldsWhatTheRecordSays: the
// file still holds what the run left, but the copy kept for it is gone, or
// its bytes are not the digest the copy record names. The printed cp would
// put the wrong bytes back, so the restore is refused, names the file, the
// copy and both digests, and prints no line. Risk: a restore that copies
// from a damaged copy and ends done.
func TestRestoreRefusesACopyThatIsMissingOrNoLongerHoldsWhatTheRecordSays(t *testing.T) {
	t.Parallel()
	for name, keep := range map[string]bool{"missing": false, "replaced": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			j := newJournalWorld(t, map[string]string{"a.txt": "new"})
			j.record("r1", model.KindIntent, now-300, nil)
			if keep {
				if err := j.ports.Journal.Keep(j.root, "r1", "a.txt", []byte("other bytes")); err != nil {
					t.Fatal(err)
				}
			}
			j.record("r1", model.KindCopy, now-299, object(canon.Object{"path": canon.String("a.txt"), "existed": canon.Bool(true), "before": canon.String(journalDigestOf("old"))}))
			j.closed("r1", "done", "a.txt", "old", "new", now-290)
			got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
			if err != nil {
				t.Fatal(err)
			}
			// a copy nobody kept has no place the journal can name
			wantCopy := ""
			if keep {
				wantCopy = j.keptAt("r1", "a.txt")
			}
			if len(got.Damaged) != 1 || got.Damaged[0].Path != "a.txt" || got.Damaged[0].Expected != journalDigestOf("old") ||
				got.Damaged[0].Copy != wantCopy {
				t.Fatalf("damaged is %+v, want a.txt with its copy and the expected digest", got.Damaged)
			}
			if keep && got.Damaged[0].Got != journalDigestOf("other bytes") {
				t.Errorf("got digest %q, want the digest of what the copy holds", got.Damaged[0].Got)
			}
			if len(got.Lines) != 0 || len(got.Files) != 0 {
				t.Errorf("a refused restore printed %q", got.Lines)
			}
		})
	}
}

// TestRestoreWithOneDamagedCopyPrintsNoLineForTheSoundOne: a run of two copied
// files, a.txt whose kept copy is damaged and b.txt whose copy is sound, gives
// no file and no line, since a half restore would put b.txt back and leave
// a.txt. Risk: the sound file's line printed beside a refusal.
func TestRestoreWithOneDamagedCopyPrintsNoLineForTheSoundOne(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, map[string]string{"a.txt": "new", "b.txt": "newb"})
	j.record("r1", model.KindIntent, now-300, nil)
	if err := j.ports.Journal.Keep(j.root, "r1", "a.txt", []byte("other bytes")); err != nil {
		t.Fatal(err)
	}
	j.record("r1", model.KindCopy, now-299, object(canon.Object{"path": canon.String("a.txt"), "existed": canon.Bool(true), "before": canon.String(journalDigestOf("old"))}))
	j.copied("r1", "b.txt", "oldb", true, now-298)
	j.record("r1", model.KindClose, now-290, object(canon.Object{"outcome": canon.String("done"), "file": canon.String(""), "files": canon.Array{
		canon.Object{"path": canon.String("a.txt"), "before": canon.String(journalDigestOf("old")), "after": canon.String(journalDigestOf("new"))},
		canon.Object{"path": canon.String("b.txt"), "before": canon.String(journalDigestOf("oldb")), "after": canon.String(journalDigestOf("newb"))},
	}}))
	got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Damaged) != 1 || got.Damaged[0].Path != "a.txt" {
		t.Fatalf("damaged is %+v, want a.txt alone", got.Damaged)
	}
	if len(got.Lines) != 0 || len(got.Files) != 0 {
		t.Errorf("a refused restore carries files %+v and lines %q, want none", got.Files, got.Lines)
	}
}

// TestClearPlanCountsTheScratchFoldersTheClearRemoves: the plan counts the
// folders a stopped run left through the Sandbox, the count its Sweep removes
// by, and the clear then removes as many. Risk: a dry run that says "nothing
// to clear" over folders a clear removes, or counts what it will not.
func TestClearPlanCountsTheScratchFoldersTheClearRemoves(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	none, err := coreapi.CacheClearPlan(t.Context(), j.ports, j.root, 30*86400)
	if err != nil || none.Scratch != 0 {
		t.Errorf("plan over a machine with no scratch folder %+v, %v, want 0", none, err)
	}
	for range 2 {
		if _, err := j.ports.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "src/a.go", Data: []byte("x")}}}); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := coreapi.CacheClearPlan(t.Context(), j.ports, j.root, 30*86400)
	if err != nil || plan.Scratch != 2 {
		t.Fatalf("plan %+v, %v, want 2 scratch folders", plan, err)
	}
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Scratch != plan.Scratch || got.Removed != plan.Remove {
		t.Errorf("clear %+v, %v, want what the plan %+v said", got, err, plan)
	}
}

// TestClearWhileARunOfAnotherRootHoldsTheScratchLockIsBusy: the scratch folders
// of every root are made in one folder, so a run of another root that holds
// the scratch lock holds them from this root's clear too. Risk: a clear in one
// folder that removes the folder a run in another is reading.
func TestClearWhileARunOfAnotherRootHoldsTheScratchLockIsBusy(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	other := j.w.Seed("/work/other", map[string]string{"x.txt": "x"})
	sc, err := j.ports.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	if err != nil {
		t.Fatal(err)
	}
	j.w.HoldLock(other, model.ScratchLock, now-5)
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Word != model.Busy || got.Removed != 0 || got.Scratch != 0 || len(runsOf(t, j)) != 3 {
		t.Fatalf("got %+v, %v, runs %v, want busy and nothing removed", got, err, runsOf(t, j))
	}
	if _, there := j.w.Scratch(sc); !there {
		t.Error("the folder a run of another root holds was removed")
	}
}

// TestClearRemovesTheScratchFoldersNoRunHolds: a folder Prepare made and no run
// released goes with the old closed runs, and the answer counts it. Risk:
// the cache growing by a folder a stopped run left, for good.
func TestClearRemovesTheScratchFoldersNoRunHolds(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	sc, err := j.ports.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "src/a.go", Data: []byte("x")}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Word != model.Done || got.Removed != 1 || got.Scratch != 1 {
		t.Fatalf("got %+v, %v, want done with 1 run and 1 scratch folder removed", got, err)
	}
	if _, there := j.w.Scratch(sc); there {
		t.Error("the scratch folder a stopped run left is still there")
	}
	if again, err := coreapi.CacheClear(t.Context(), j.ports, j.root); err != nil || again.Scratch != 0 {
		t.Errorf("a second clear = %+v, %v, want no scratch folder removed", again, err)
	}
}

// TestClearRemovesTheToolsCacheFoldersThePlanCounts: the plan counts the
// folders a grant's caches are kept in through the Steps, and the clear
// removes as many under its locks and says so; a busy clear removes none.
// Risk: a dry run that says "nothing to clear" over cache folders a clear
// removes, or a busy clear that removes them all the same.
func TestClearRemovesTheToolsCacheFoldersThePlanCounts(t *testing.T) {
	t.Parallel()
	held := clearWorld(t)
	held.w.KeepCaches(3)
	held.w.HoldLock(held.root, model.ScratchLock, now-5)
	if busy, err := coreapi.CacheClear(t.Context(), held.ports, held.root); err != nil || busy.Word != model.Busy || busy.Tools != 0 {
		t.Fatalf("a busy clear = %+v, %v, want busy and no cache folder removed", busy, err)
	}
	if left, err := held.ports.Steps.CachesLeft(); err != nil || left != 3 {
		t.Errorf("after a busy clear %d cache folders are left, %v, want all 3", left, err)
	}
	j := clearWorld(t)
	j.w.KeepCaches(3)
	plan, err := coreapi.CacheClearPlan(t.Context(), j.ports, j.root, 30*86400)
	if err != nil || plan.Tools != 3 {
		t.Fatalf("plan %+v, %v, want 3 cache folders", plan, err)
	}
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Word != model.Done || got.Tools != plan.Tools {
		t.Errorf("clear %+v, %v, want done with what the plan %+v said", got, err, plan)
	}
	if again, err := coreapi.CacheClearPlan(t.Context(), j.ports, j.root, 30*86400); err != nil || again.Tools != 0 {
		t.Errorf("a plan after the clear = %+v, %v, want no cache folder left", again, err)
	}
}

// TestClearWhileARunHoldsTheScratchLockIsBusyAndRemovesNothing: a proof run
// holds the scratch lock while its folders are in use, so a clear that cannot
// take it ends busy, removes no run and no folder, and gives the root lock
// back. Risk: a clear that removes the folder a running tool reads.
func TestClearWhileARunHoldsTheScratchLockIsBusyAndRemovesNothing(t *testing.T) {
	t.Parallel()
	j := clearWorld(t)
	sc, err := j.ports.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	if err != nil {
		t.Fatal(err)
	}
	j.w.HoldLock(j.root, model.ScratchLock, now-5)
	got, err := coreapi.CacheClear(t.Context(), j.ports, j.root)
	if err != nil || got.Word != model.Busy || got.Removed != 0 || got.Scratch != 0 || len(runsOf(t, j)) != 3 {
		t.Fatalf("got %+v, %v, runs %v, want busy and nothing removed", got, err, runsOf(t, j))
	}
	if _, there := j.w.Scratch(sc); !there {
		t.Error("a folder a run holds was removed")
	}
	h, err := j.ports.Lock.Take(t.Context(), j.root, model.RootLock)
	if err != nil {
		t.Fatalf("the root lock is still held after a busy clear: %v", err)
	}
	if err := j.ports.Lock.Release(h); err != nil {
		t.Error(err)
	}
}

// posix is the Liner the command tree hands a restore: argv as one POSIX
// line, through go-console's shell.Quote.
func posix(argv []string) (string, error) { return shell.Quote(shell.POSIX, argv) }

// quoted is argv as the one POSIX line a restore prints for it.
func quoted(t *testing.T, argv ...string) string {
	t.Helper()
	line, err := shell.Quote(shell.POSIX, argv)
	if err != nil {
		t.Fatal(err)
	}
	return line
}
