package fs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/birdtable/go-files/confine"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/go-files/lock"
	"github.com/birdtable/go-run/runlog"
	testenv "github.com/birdtable/go-testkit"
	"github.com/birdtable/gragas/internal/digest"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// journalOf is a journal over a state folder at dir, stamped at instant, with
// the Writer over the Tree it opens roots in.
func journalOf(t *testing.T, dir string, instant time.Time, age time.Duration) (hostfs.Journal, *hostfs.Tree) {
	t.Helper()
	tree := newTree(t)
	return hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: at(instant), Dir: dir, Age: age}, tree
}

// openRoot makes and opens a folder to keep a journal of.
func openRoot(t *testing.T, tree *hostfs.Tree, name string) model.Root {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	root, err := tree.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// journalFile is the one journal file in the state folder dir.
func journalFile(t *testing.T, dir string) string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.journal"))
	if err != nil || len(names) != 1 {
		t.Fatalf("the state folder holds %v, %v, want the one journal", names, err)
	}
	return names[0]
}

// runLine is a run of a journal as go-run's runlog writes it: the run's
// record line under its id, then an entry line of the run for each of
// entries, each one JSON value, its records in order. A test plants with it
// what another engine would have written.
func runLine(run string, entries ...string) string {
	out := `{"id":"` + run + `","argv":null,"started":"2026-11-08T06:00:00Z","code":0,"changes":[]}` + "\n"
	for _, e := range entries {
		out += entryLine(run, e)
	}
	return out
}

// entryLine is one entry line of run, as runlog's Entry writes it, its
// value the record e.
func entryLine(run, e string) string {
	return `{"entry":"` + run + `","time":"2026-11-08T06:00:00Z","value":` + e + `}` + "\n"
}

// journalBytes is the one journal file in the state folder dir, or nothing
// before there is one.
func journalBytes(t *testing.T, dir string) []byte {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.journal"))
	if err != nil || len(names) > 1 {
		t.Fatalf("the state folder holds %v, %v, want at most the one journal", names, err)
	}
	if len(names) == 0 {
		return nil
	}
	data, err := os.ReadFile(names[0])
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestTheJournalAdapterPassesTheJournalSuite(t *testing.T) {
	t.Parallel()
	contracttest.Journal(t, func(t *testing.T) contracttest.JournalSetup {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "state")
		j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
		clock := &moving{at: time.Time(lockInstant())}
		j.Clock = clock
		root := openRoot(t, tree, "a")
		other := openRoot(t, tree, "b")
		// Bytes reads the one journal in the state folder: the row that
		// calls it writes Root's alone.
		return contracttest.JournalSetup{J: j, Root: root, Other: other, Pass: clock.pass,
			Bytes: func() []byte { return journalBytes(t, dir) }}
	})
}

// moving is a clock a test moves on.
type moving struct{ at time.Time }

func (m *moving) Now() time.Time       { return m.at }
func (m *moving) pass(d time.Duration) { m.at = m.at.Add(d) }

// failing is a Writer that writes nothing and says so.
type failing struct{}

var errDisk = errors.New("the disk is full")

func (failing) Replace(model.Root, string, []byte) error { return errDisk }
func (failing) Append(model.Root, string, []byte) error  { return errDisk }
func (failing) Remove(model.Root, string) error          { return errDisk }

// TestAJournalThatCannotBeWrittenStopsTheEffect: a record runlog cannot
// write, or a copy the Writer cannot, is an error the caller sees, so it makes
// no effect. runlog writes the journal itself, through the held state folder,
// so its write is failed by a folder where the journal is: runlog refuses
// anything there but a plain file. The control is the same journal before.
func TestAJournalThatCannotBeWrittenStopsTheEffect(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	intent := model.Record{Run: "run-1", Kind: model.KindIntent}
	if err := j.Append(t.Context(), root, intent); err != nil {
		t.Fatalf("a journal that can be written = %v", err)
	}
	name := journalFile(t, dir)
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(name, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), root, model.Record{Run: "run-2", Kind: model.KindIntent}); err == nil {
		t.Error("Append over a folder where the journal is = nil, want the write's error")
	}
	j.Writer = failing{}
	if err := j.Keep(root, "run-1", "a", []byte("old")); !errors.Is(err, errDisk) {
		t.Errorf("Keep over a failing Writer = %v, want the disk's error", err)
	}
}

// TestAJournalThatIsNotAPlainFileIsNotRead (gragas#2441, go-run v0.9.1): runlog
// reads a log only when it is a plain file, as it writes one, so Records over a
// folder where the journal is, or a link to another file that holds a run, is an
// error that names the journal and reads nothing: runlog.ErrNotPlain. The
// control is the same journal before, which reads.
func TestAJournalThatIsNotAPlainFileIsNotRead(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	if recs, _, err := j.Records(t.Context(), root); err != nil || len(recs) != 1 {
		t.Fatalf("Records of a plain journal = %d records, %v, want its one record", len(recs), err)
	}
	name := journalFile(t, dir)
	plain, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(name, 0o700); err != nil {
		t.Fatal(err)
	}
	if recs, _, err := j.Records(t.Context(), root); err == nil || len(recs) != 0 || !errors.Is(err, runlog.ErrNotPlain) {
		t.Errorf("Records over a folder where the journal is = %d records, %v, want runlog.ErrNotPlain", len(recs), err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	// a system that refuses a link skips this row alone, as testenv.Link says
	t.Run("a link to a journal", func(t *testing.T) {
		testenv.Link(t, target, name)
		if recs, _, err := j.Records(t.Context(), root); err == nil || len(recs) != 0 || !errors.Is(err, runlog.ErrNotPlain) {
			t.Errorf("Records over a link to a journal = %d records, %v, want runlog.ErrNotPlain", len(recs), err)
		}
	})
}

// TestAJournalWithNoStateFolderOrAForeignRootKeepsNothing: the wiring that
// left the state folder out, or a root the Tree never opened, is an error and
// no file anywhere.
func TestAJournalWithNoStateFolderOrAForeignRootKeepsNothing(t *testing.T) {
	t.Parallel()
	j, tree := journalOf(t, "", time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	rec := model.Record{Run: "run-1", Kind: model.KindIntent}
	if err := j.Append(t.Context(), root, rec); err == nil {
		t.Error("a journal with no state folder took a record")
	}
	j.Dir = filepath.Join(t.TempDir(), "state")
	if err := j.Append(t.Context(), model.Root(filepath.Join(t.TempDir(), "elsewhere")), rec); err == nil {
		t.Error("a journal took a record of a root its Tree never opened")
	}
	if _, err := os.Stat(j.Dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the refused records made the state folder: %v", err)
	}
}

// TestATornLastLineIsSkippedCountedAndReported: a run killed in the middle of
// a line leaves half of it; the reader skips it, counts it, and the next
// record is whole on a line of its own.
func TestATornLastLineIsSkippedCountedAndReported(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	for _, k := range []string{model.KindIntent, model.KindStep} {
		if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: k}); err != nil {
			t.Fatal(err)
		}
	}
	name := journalFile(t, dir)
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, append(data, `{"v":1,"run":"run-1","ki`...), 0o600); err != nil {
		t.Fatal(err)
	}
	got, torn, err := j.Records(t.Context(), root)
	if err != nil || torn != 1 || len(got) != 2 {
		t.Fatalf("Records = %d records, %d torn, %v, want 2, 1 and no error", len(got), torn, err)
	}
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindClose}); err != nil {
		t.Fatal(err)
	}
	got, torn, err = j.Records(t.Context(), root)
	if err != nil || torn != 1 || len(got) != 3 || got[2].Kind != model.KindClose {
		t.Errorf("Records after the next record = %d records, %d torn, %v, want 3, 1 and the close last", len(got), torn, err)
	}
}

// TestARunThatWritesItsIntentBeforeItsStartsLeavesNoTornRecord: a dry-run
// import over a plugin is a run of its own, intent first, then each start, then
// the close; read back, none of its records is torn.
func TestARunThatWritesItsIntentBeforeItsStartsLeavesNoTornRecord(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	for _, k := range []string{model.KindIntent, model.KindStart, model.KindStart, model.KindClose} {
		if err := j.Append(t.Context(), root, model.Record{Run: "read-1-1", Kind: k}); err != nil {
			t.Fatal(err)
		}
	}
	got, torn, err := j.Records(t.Context(), root)
	if err != nil || torn != 0 || len(got) != 4 || got[0].Kind != model.KindIntent || got[3].Kind != model.KindClose {
		t.Errorf("Records = %d records, %d torn, %v, want 4, 0, no error, the intent first and the close last", len(got), torn, err)
	}
}

// TestALineFromANewerVersionIsRefusedWithBothVersions: a journal a newer engine
// wrote is not read half; the error names the record's place in its run's
// list, its version and the newest this engine reads. The newer engine's
// entry line is the run's last, so runlog reads the run as it: the record of
// this engine's version first, then one of version 9. The control is the run
// before that line, of the version this engine reads. RecordsOf over the same
// bytes answers the same.
func TestALineFromANewerVersionIsRefusedWithBothVersions(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	name := journalFile(t, dir)
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if got, _, err := j.Records(t.Context(), root); err != nil || len(got) != 1 {
		t.Fatalf("control: Records = %+v, %v, want the one record", got, err)
	}
	planted := append(data, entryLine("run-1", `{"v":9,"kind":"step","at":1,"root":"x"}`)...)
	if err := os.WriteFile(name, planted, 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, err := j.Records(t.Context(), root)
	ne, ok := errors.AsType[*model.NewerRecordError](err)
	if !ok || ne.Run != "run-1" || ne.Entry != 2 || ne.Found != 9 || ne.Reads != model.RecordVersion || got != nil {
		t.Fatalf("Records = %+v, %v, want a NewerRecordError of record 2 of run-1, version 9, reading 1, and no records", got, err)
	}
	if msg := err.Error(); !strings.Contains(msg, "version 9") || !strings.Contains(msg, "version 1") {
		t.Errorf("the error reads %q, want both versions", msg)
	}
	got, torn, err := j.RecordsOf(t.Context(), planted)
	if of, ok := errors.AsType[*model.NewerRecordError](err); !ok || *of != *ne || got != nil || torn != 0 {
		t.Errorf("RecordsOf = %+v, %d, %v, want the NewerRecordError Records answers", got, torn, err)
	}
}

// TestALineOfAnOlderFormIsRefusedByName: a line of release-1's own form, one
// record a line under its run with no runlog id, is refused by Records and by
// Sweep with an OlderError naming the line, the run and the journal's path,
// never read as torn; Sweep leaves the journal as it was. The control is the
// same journal before the line, which Records reads and Sweep passes.
func TestALineOfAnOlderFormIsRefusedByName(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	name := journalFile(t, dir)
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if got, torn, err := j.Records(t.Context(), root); err != nil || len(got) != 1 || torn != 0 {
		t.Fatalf("control: Records = %+v, %d, %v, want the one record", got, torn, err)
	}
	if _, err := j.Sweep(t.Context(), root); err != nil {
		t.Fatalf("control: Sweep = %v", err)
	}
	planted := append(data, `{"v":1,"run":"run-0","kind":"intent","at":1,"root":"x"}`+"\n"...)
	if err := os.WriteFile(name, planted, 0o600); err != nil {
		t.Fatal(err)
	}
	// The journal names its path by the state folder resolved, as KeptAt does.
	real, err := confine.Resolve(name)
	if err != nil {
		t.Fatal(err)
	}
	// The planted line is the file's third: the run's entry line, then its
	// record line, then the older one.
	got, _, err := j.Records(t.Context(), root)
	var oe *model.OlderJournalError
	if !errors.As(err, &oe) || oe.Line != 3 || oe.Run != "run-0" || oe.Path != real || got != nil {
		t.Fatalf("Records = %+v, %v, want an OlderJournalError of line 3, run run-0, at %s, and no records", got, err, real)
	}
	if msg := err.Error(); !strings.Contains(msg, "older release") || !strings.Contains(msg, real) {
		t.Errorf("the error reads %q, want the older release and the journal's path", msg)
	}
	// RecordsOf reads bytes and knows no path, so its error names none.
	if got, torn, err := j.RecordsOf(t.Context(), planted); !errors.As(err, &oe) || *oe != (model.OlderJournalError{Line: 3, Run: "run-0"}) || got != nil || torn != 0 {
		t.Errorf("RecordsOf = %+v, %d, %v, want the OlderJournalError of line 3 and run run-0 with no path", got, torn, err)
	}
	if _, err := j.Sweep(t.Context(), root); !errors.As(err, &oe) {
		t.Errorf("Sweep = %v, want the OlderJournalError", err)
	}
	// A record appended to it would go with the journal when the fix
	// removes it, so Append refuses too: a new run's first record, and a
	// later one of a run already there.
	for _, r := range []model.Record{{Run: "run-2", Kind: model.KindIntent}, {Run: "run-1", Kind: model.KindClose}} {
		if err := j.Append(t.Context(), root, r); !errors.As(err, &oe) || oe.Line != 3 || oe.Path != real {
			t.Errorf("Append(%s %s) = %v, want the OlderJournalError of line 3 at %s", r.Run, r.Kind, err, real)
		}
	}
	if after, err := os.ReadFile(name); err != nil || string(after) != string(planted) {
		t.Errorf("the journal after the refused sweep and appends is %q, %v, want it as planted", after, err)
	}
}

// TestACurrentLineThatAlsoCarriesTheOlderFieldsIsARecord: a line of
// runlog's form that has an id is a record, even when it also carries the
// v, run and kind an older line has, beside a torn line that makes the
// look run. Only a line with no id is of the older form.
func TestACurrentLineThatAlsoCarriesTheOlderFieldsIsARecord(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	name := journalFile(t, dir)
	line := `{"id":"run-2","v":1,"run":"run-2","kind":"intent","argv":null,"started":"2026-11-08T06:00:00Z","code":0,"changes":[]}` + "\n" +
		entryLine("run-2", `{"v":1,"kind":"intent","at":1,"root":"x"}`)
	data := []byte(line + "{\n")
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, torn, err := j.Records(t.Context(), root)
	if err != nil || torn != 1 || len(got) != 1 || got[0].Run != "run-2" {
		t.Fatalf("Records = %+v, %d, %v, want the record of run-2 and the torn line counted", got, torn, err)
	}
	if got, torn, err := j.RecordsOf(t.Context(), data); err != nil || torn != 1 || len(got) != 1 || got[0].Run != "run-2" {
		t.Errorf("RecordsOf = %+v, %d, %v, want what Records answers", got, torn, err)
	}
}

// TestARecordOverTheLongestLineIsRefusedAndTheJournalIsAsItWas:
// the bound on a line is runlog's MaxLine, and the adapter
// builds none of its own. A record whose entry line would be over it is
// refused with runlog's ErrLineTooLong, of a run's first record and of a
// later one, and the journal is byte for byte as it was. The control is a
// record well under the bound, which goes in.
func TestARecordOverTheLongestLineIsRefusedAndTheJournalIsAsItWas(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindIntent, Detail: []byte(`{"pad":"x"}`)}); err != nil {
		t.Fatalf("control: a short record = %v", err)
	}
	before := journalBytes(t, dir)
	long := []byte(`{"pad":"` + strings.Repeat("x", runlog.DefaultMaxLine) + `"}`)
	for _, r := range []model.Record{{Run: "run-2", Kind: model.KindIntent}, {Run: "run-1", Kind: model.KindStep}, {Run: "run-1", Kind: model.KindClose}} {
		r.Detail = long
		if err := j.Append(t.Context(), root, r); !errors.Is(err, runlog.ErrLineTooLong) {
			t.Errorf("Append(%s %s) over the longest line = %v, want runlog's ErrLineTooLong", r.Run, r.Kind, err)
		}
	}
	if after := journalBytes(t, dir); !bytes.Equal(after, before) {
		t.Errorf("the journal after the refused records is %d bytes, want the %d it was", len(after), len(before))
	}
	if got, torn, err := j.Records(t.Context(), root); err != nil || torn != 0 || len(got) != 1 {
		t.Errorf("Records after the refused records = %d records, %d torn, %v, want the control alone", len(got), torn, err)
	}
}

// TestARecordCarriesTheSameFixedFieldsWhoeverDidTheWork: a tool's record
// becomes one entry line of its run, written by runlog's Entry before the
// run's record line, whose value holds it with the fixed fields and nothing
// else: the digest of the root, the instant, and the tool's detail passed
// through whole.
func TestARecordCarriesTheSameFixedFieldsWhoeverDidTheWork(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	instant := time.Time(lockInstant())
	j, tree := journalOf(t, dir, instant, 0)
	root := openRoot(t, tree, "a")
	detail := `{"argv":["gragas","gen"],"digests":{"rules/a.md":"sha256:bb"}}`
	rec := model.Record{Run: "run-1", Kind: model.KindIntent, Command: "gragas gen --apply", Plan: "sha256:aa", Engine: "v0.1.0", Detail: []byte(" " + detail + "\n")}
	if err := j.Append(t.Context(), root, rec); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(journalFile(t, dir))
	lines := strings.SplitAfter(string(data), "\n")
	if err != nil || len(lines) != 3 || lines[2] != "" {
		t.Fatalf("the journal holds %q, %v, want two lines each ending in a newline", data, err)
	}
	var entry struct {
		Run   string                     `json:"entry"`
		Value map[string]json.RawMessage `json:"value"`
	}
	var run struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(lines[0]), &entry) != nil || json.Unmarshal([]byte(lines[1]), &run) != nil || entry.Run != "run-1" || run.ID != "run-1" {
		t.Fatalf("the journal is %s, want run-1's entry line, then its record line", data)
	}
	fields := entry.Value
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"at", "command", "detail", "engine", "kind", "plan", "root", "v"}; !slices.Equal(keys, want) {
		t.Errorf("the line has the fields %v, want %v", keys, want)
	}
	if string(fields["v"]) != "1" || string(fields["root"]) != `"`+digest.Hex([]byte(root))+`"` || string(fields["at"]) != "1794117600" || string(fields["detail"]) != detail {
		t.Errorf("the line is %s, want version 1, the root's digest, the instant and the detail whole", data)
	}
}

// TestTheStateFolderNamesNoProject: only digests name a file in the state
// folder, the journal, its folder of copies, the run's folder and each copy.
func TestTheStateFolderNamesNoProject(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "orch-project")
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindCopy}); err != nil {
		t.Fatal(err)
	}
	if err := j.Keep(root, "run-1", "rules/orch.md", []byte("old")); err != nil {
		t.Fatal(err)
	}
	digest := regexp.MustCompile(`^[0-9a-f]{64}(\.journal|\.copies)?$`)
	seen := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir || d.Name() == "run-1" {
			return err
		}
		seen++
		if !digest.MatchString(d.Name()) {
			t.Errorf("%s in the state folder is not named for a digest", p)
		}
		return nil
	})
	if err != nil || seen != 3 {
		t.Errorf("the state folder holds %d names, %v, want the journal, the copies folder and one copy", seen, err)
	}
}

// TestARunIdIsRandomAndHex: two ids differ and each is thirty-two hex digits,
// a name the journal takes.
func TestARunIdIsRandomAndHex(t *testing.T) {
	t.Parallel()
	a, errA := hostfs.NewRunID()
	b, errB := hostfs.NewRunID()
	if errA != nil || errB != nil || a == b || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(a) {
		t.Errorf("run ids = %q, %q, %v, %v, want two different 32-digit hex ids", a, b, errA, errB)
	}
	j, tree := journalOf(t, filepath.Join(t.TempDir(), "state"), time.Time(lockInstant()), 0)
	if err := j.Append(t.Context(), openRoot(t, tree, "a"), model.Record{Run: a, Kind: model.KindIntent}); err != nil {
		t.Errorf("the journal refused a run id it was made for: %v", err)
	}
}

// importsMathRand is whether the Go source src imports math/rand or its v2.
func importsMathRand(t *testing.T, name, src string) bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		if p := strings.Trim(imp.Path.Value, `"`); p == "math/rand" || p == "math/rand/v2" {
			return true
		}
	}
	return false
}

// TestNoFileOfThisPackageImportsMathRand: a run id and a temporary file's name
// come from the cryptographic generator (supply.scans.6), so no non-test file
// of the package imports the other. The planted sources show the check sees
// both spellings and lets crypto/rand by.
func TestNoFileOfThisPackageImportsMathRand(t *testing.T) {
	t.Parallel()
	for src, want := range map[string]bool{
		"package fs\nimport \"math/rand\"\n":      true,
		"package fs\nimport r \"math/rand/v2\"\n": true,
		"package fs\nimport \"crypto/rand\"\n":    false,
	} {
		if got := importsMathRand(t, "planted.go", src); got != want {
			t.Errorf("importsMathRand(%q) = %v, want %v", src, got, want)
		}
	}
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		if importsMathRand(t, name, string(src)) {
			t.Errorf("%s imports math/rand", name)
		}
	}
	if checked == 0 {
		t.Fatal("no file of the package was checked")
	}
}

// swept is a journal with three runs on a day: a run closed long ago, a run
// closed lately, and a run opened long ago and never closed, each with a copy.
// It answers the state folder, the root and the journal as of later.
func swept(t *testing.T, age, later time.Duration) (dir string, root model.Root, j hostfs.Journal) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "state")
	then := time.Time(lockInstant())
	old, tree := journalOf(t, dir, then, age)
	root = openRoot(t, tree, "a")
	lately, _ := journalOf(t, dir, then.Add(29*24*time.Hour), age)
	lately.Tree, lately.Writer = tree, hostfs.Writer{Tree: tree}
	for _, c := range []struct {
		j    hostfs.Journal
		run  string
		kind []string
	}{
		{old, "closed-long-ago", []string{model.KindIntent, model.KindClose}},
		{old, "never-closed", []string{model.KindIntent}},
		{lately, "closed-lately", []string{model.KindIntent, model.KindClose}},
	} {
		for _, k := range c.kind {
			if err := c.j.Append(t.Context(), root, model.Record{Run: c.run, Kind: k}); err != nil {
				t.Fatal(err)
			}
		}
		if err := c.j.Keep(root, c.run, "a", []byte(c.run)); err != nil {
			t.Fatal(err)
		}
	}
	j, _ = journalOf(t, dir, then.Add(later), age)
	j.Tree, j.Writer = tree, hostfs.Writer{Tree: tree}
	return dir, root, j
}

// runsOf is the runs of the records, in order, without repeats.
func runsOf(recs []model.Record) []string {
	var out []string
	for _, r := range recs {
		if !slices.Contains(out, r.Run) {
			out = append(out, r.Run)
		}
	}
	return out
}

// TestSweepRemovesOnlyClosedRunsOlderThanTheAge: with the default thirty days,
// thirty-one days on, the run closed on day 0 and its copy go; the run closed
// on day 29 and the run never closed stay, with their copies.
func TestSweepRemovesOnlyClosedRunsOlderThanTheAge(t *testing.T) {
	t.Parallel()
	_, root, j := swept(t, 0, 31*24*time.Hour)
	n, err := j.Sweep(t.Context(), root)
	if err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v, want 1 run swept", n, err)
	}
	recs, torn, err := j.Records(t.Context(), root)
	if got := runsOf(recs); err != nil || torn != 0 || !slices.Equal(got, []string{"never-closed", "closed-lately"}) {
		t.Errorf("Records after the sweep = runs %v, %d torn, %v, want never-closed and closed-lately", got, torn, err)
	}
	if _, err := j.Kept(root, "closed-long-ago", "a"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the swept run's copy = %v, want it gone", err)
	}
	for _, run := range []string{"never-closed", "closed-lately"} {
		if got, err := j.Kept(root, run, "a"); err != nil || string(got) != run {
			t.Errorf("Kept(%s) = %q, %v, want the copy still there", run, got, err)
		}
	}
	if n, err := j.Sweep(t.Context(), root); err != nil || n != 0 {
		t.Errorf("a second Sweep = %d, %v, want nothing left to sweep", n, err)
	}
}

// TestSweepKeepsWhatIsYoungerThanTheSettingsAge: the age is the one the
// settings file names: at an hour, the run closed on day 29 goes too, and at a
// year, none does. A run never closed stays at any age.
func TestSweepKeepsWhatIsYoungerThanTheSettingsAge(t *testing.T) {
	t.Parallel()
	_, root, j := swept(t, time.Hour, 31*24*time.Hour)
	if n, err := j.Sweep(t.Context(), root); err != nil || n != 2 {
		t.Errorf("Sweep at an age of an hour = %d, %v, want both closed runs", n, err)
	}
	if recs, _, _ := j.Records(t.Context(), root); !slices.Equal(runsOf(recs), []string{"never-closed"}) {
		t.Errorf("runs left = %v, want only the run never closed", runsOf(recs))
	}
	_, root, j = swept(t, 365*24*time.Hour, 31*24*time.Hour)
	if n, err := j.Sweep(t.Context(), root); err != nil || n != 0 {
		t.Errorf("Sweep at an age of a year = %d, %v, want nothing", n, err)
	}
}

// TestSweepRefusesAJournalItCannotReadWhole: a line of a newer version stops
// the sweep before it removes anything, and the journal is as it was.
func TestSweepRefusesAJournalItCannotReadWhole(t *testing.T) {
	t.Parallel()
	dir, root, j := swept(t, 0, 31*24*time.Hour)
	name := journalFile(t, dir)
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, runLine("newer", `{"v":9,"kind":"step","at":1,"root":"x"}`)...)
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var ne *model.NewerRecordError
	if n, err := j.Sweep(t.Context(), root); !errors.As(err, &ne) || n != 0 {
		t.Errorf("Sweep = %d, %v, want a NewerRecordError and nothing swept", n, err)
	}
	if after, err := os.ReadFile(name); err != nil || string(after) != string(data) {
		t.Errorf("the journal changed under a refused sweep: %v", err)
	}
	if got, err := j.Kept(root, "closed-long-ago", "a"); err != nil || string(got) != "closed-long-ago" {
		t.Errorf("the copy of the run the refused sweep would have removed = %q, %v", got, err)
	}
}

// FuzzTheJournalReader: any bytes are read by RecordsOf without a panic into
// records whose run and kind a record can have, a torn count no larger than
// the lines, and at most the one error a newer version or an older form is;
// the same bytes read the same twice.
func FuzzTheJournalReader(f *testing.F) {
	for _, seed := range []string{
		"",
		"\n\n",
		runLine("run-1", `{"v":1,"kind":"intent","at":1,"root":"x"}`),
		runLine("run-1", `{"v":1,"kind":"intent","at":1,"root":"x"}`) + `{"entry":"run-1","ti`,
		entryLine("run-1", `{"v":1,"kind":"intent","at":1,"root":"x"}`),
		entryLine("run-1", `{"v":1,"kind":"step","at":1,"root":"x"}`) + runLine("run-1", `{"v":1,"kind":"close","at":2,"root":"x"}`),
		runLine("run-1", `{"v":1,"kind":"intent","at":1,"root":"x"}`) + `{"entry":"run-1","time":null,"value":{}}` + "\n",
		runLine("run-1", `{"v":1,"kind":"intent","at":1,"root":"x"}`, `{"v":1,"kind":"close","at":2,"root":"x"}`),
		runLine("run-1", `{"v":1,"kind":"intent","at":1,"root":"x"}`) + runLine("run-2", `{"v":1,"kind":"step","at":1,"root":"x"}`),
		runLine("run-1", `{"v":2,"kind":"intent","at":1,"root":"x"}`),
		runLine("../x", `{"v":1,"kind":"intent"}`),
		runLine("run-1", `{"v":1,"kind":"novel"}`),
		runLine("run-1", `{"v":"1"}`, `[1]`, `null`),
		runLine("run-1", `{"v":0,"kind":"close"}`),
		`{"id":"run-1","extra":{"entries":"x"}}` + "\n",
		`{"id":"run-1"}` + "\n",
		`{"v":1,"run":"run-1","kind":"intent","at":1,"root":"x"}` + "\n",
		runLine("run-1", `{"v":1,"kind":"intent","at":1,"root":"x"}`) + `{"v":1,"run":"run-0","kind":"close","at":1}` + "\n",
		`{"id":null,"v":1,"run":"run-1","kind":"intent"}` + "\n",
		"[1,2]\nnull\n\"x\"\n",
		"\x00\xff\xfe{\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		recs, torn, err := hostfs.Journal{}.RecordsOf(t.Context(), data)
		var ne *model.NewerRecordError
		var oe *model.OlderJournalError
		switch {
		case err == nil:
		case recs != nil || torn != 0:
			t.Fatalf("RecordsOf = %+v, %d, %v, want an error alone", recs, torn, err)
		case errors.As(err, &oe):
			if oe.Line < 1 || oe.Run == "" || strings.ContainsAny(oe.Run, "/. ") || oe.Path != "" {
				t.Fatalf("the OlderJournalError %+v names no line or no run, or a path", oe)
			}
		case !errors.As(err, &ne) || ne.Found <= ne.Reads:
			t.Fatalf("RecordsOf = %v, want a NewerRecordError, an OlderJournalError or records", err)
		}
		// Each record is an entry line and each torn one a line or an
		// entry line, so together they are no more than the lines.
		if lines := len(bytes.FieldsFunc(data, func(r rune) bool { return r == '\n' })); len(recs)+torn > lines {
			t.Fatalf("%d records and %d torn from %d lines", len(recs), torn, lines)
		}
		for _, r := range recs {
			if r.Version < 1 || r.Version > model.RecordVersion || r.Run == "" || strings.ContainsAny(r.Run, "/. ") || !slices.Contains([]string{model.KindIntent, model.KindCopy, model.KindStep, model.KindClose}, r.Kind) {
				t.Fatalf("the record %+v is no record", r)
			}
		}
		again, torn2, err2 := hostfs.Journal{}.RecordsOf(t.Context(), data)
		if torn2 != torn || len(again) != len(recs) || (err2 == nil) != (err == nil) {
			t.Fatalf("the same bytes read twice differ: %d/%d records, %d/%d torn", len(recs), len(again), torn, torn2)
		}
	})
}

var _ port.Journal = hostfs.Journal{}

// TestARefusedSecondCopyNamesTheRunAndThePath: the error of a second Keep says
// which run and which path already have a copy, so a person reading it knows
// what a retry would have overwritten.
func TestARefusedSecondCopyNamesTheRunAndThePath(t *testing.T) {
	t.Parallel()
	j, tree := journalOf(t, filepath.Join(t.TempDir(), "state"), time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	if err := j.Keep(root, "run-1", "rules/a.md", []byte("original")); err != nil {
		t.Fatal(err)
	}
	err := j.Keep(root, "run-1", "rules/a.md", []byte("replaced"))
	if err == nil || !strings.Contains(err.Error(), "run-1") || !strings.Contains(err.Error(), "rules/a.md") {
		t.Errorf("a second Keep = %v, want an error that names the run and the path", err)
	}
}

// TestWhereACopyLiesIsTheFileKeptReads (arch.ports.19): the path KeptAt
// shows a person is the one file in the state folder holding the bytes
// Kept answers, so a person who copies from it puts back what the roll-back
// would. Planted failure: the copies folder answered for the copy. Risk: a
// message that sends a person to the wrong bytes.
func TestWhereACopyLiesIsTheFileKeptReads(t *testing.T) {
	t.Parallel()
	state := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, state, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	for _, c := range []struct{ run, path, old string }{{"run-1", "rules/a.md", "one\n"}, {"run-1", "rules/b.md", ""}, {"run-2", "rules/a.md", "three\n"}} {
		if err := j.Keep(root, c.run, c.path, []byte(c.old)); err != nil {
			t.Fatal(err)
		}
		at, err := j.KeptAt(root, c.run, c.path)
		if err != nil {
			t.Fatal(err)
		}
		real, err := confine.Resolve(state)
		if err != nil {
			t.Fatal(err)
		}
		if in, err := filepath.Rel(real, at); err != nil || strings.HasPrefix(in, "..") {
			t.Errorf("KeptAt(%s, %s) = %s, outside the state folder %s", c.run, c.path, at, state)
		}
		data, err := os.ReadFile(at)
		if err != nil || string(data) != c.old {
			t.Errorf("the file at KeptAt(%s, %s), %s, holds %q, %v, want %q", c.run, c.path, at, data, err, c.old)
		}
	}
}

// TestAJournalHeldByAnotherIsAWriteThatFails: another descriptor holds the journal's
// lock, and an Append under a ctx no interrupt ends, as its callers hand it,
// answers that the record could not be written within the journal's hold,
// for a record of a run already there and for the intent of a new one; the
// journal is as it was, so no effect follows. Planted: Append with no
// deadline of its own (both still waiting, red here).
func TestAJournalHeldByAnotherIsAWriteThatFails(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	j, tree := journalOf(t, dir, time.Time(lockInstant()), 0)
	root := openRoot(t, tree, "a")
	if err := j.Append(t.Context(), root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatalf("a journal that can be written = %v", err)
	}
	name := journalFile(t, dir)
	before := journalBytes(t, dir)
	held, err := lock.Take(name, "")
	if err != nil {
		t.Fatalf("holding the journal = %v", err)
	}
	t.Cleanup(func() {
		if err := held.Release(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.WithoutCancel(t.Context())
	records := []model.Record{{Run: "run-1", Kind: model.KindCopy}, {Run: "run-2", Kind: model.KindIntent}}
	errs := make(chan error, len(records))
	for _, r := range records {
		go func() { errs <- j.Append(ctx, root, r) }()
	}
	wait := hostfs.JournalHold + 10*time.Second
	bound := time.After(wait)
	for range records {
		select {
		case err := <-errs:
			if err == nil || !strings.Contains(err.Error(), "could not be written") {
				t.Errorf("Append to a journal another holds = %v, want that the record could not be written", err)
			}
		case <-bound:
			t.Fatalf("Append to a journal another holds is still waiting after %s", wait)
		}
	}
	if got := journalBytes(t, dir); !bytes.Equal(got, before) {
		t.Errorf("the journal changed while another held it:\n%s\nwant\n%s", got, before)
	}
}

// TestTwoAppendsAtOnceKeepEachRecordWhole: two writers appending at once, as
// two refused guard edits do, each with records long enough to need more than
// one buffer, leave every record whole and on its own line: one write per
// record, through the flag Writer.Append alone names. Risk: two records interleave, and the
// journal of a refusal reads as torn or as another record.
func TestTwoAppendsAtOnceKeepEachRecordWhole(t *testing.T) {
	t.Parallel()
	j, tree := journalOf(t, t.TempDir(), time.Unix(1700000000, 0), time.Hour)
	root := openRoot(t, tree, "book")
	const each = 100
	long := strings.Repeat("x", 64<<10)
	var wg sync.WaitGroup
	for w := range 2 {
		wg.Go(func() {
			for i := range each {
				detail := fmt.Sprintf(`{"outcome":"refused","writer":%d,"n":%d,"pad":%q}`, w, i, long)
				if err := j.Append(t.Context(), root, model.Record{Run: fmt.Sprintf("guard-%d-%d", w, i), Kind: model.KindClose,
					Command: "gragas guard edit", Detail: []byte(detail)}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	records, torn, err := j.Records(t.Context(), root)
	if err != nil || torn != 0 || len(records) != 2*each {
		t.Fatalf("Records = %d records, %d torn, %v; want %d whole", len(records), torn, err, 2*each)
	}
	for _, r := range records {
		var d struct {
			Pad string `json:"pad"`
		}
		if err := json.Unmarshal(r.Detail, &d); err != nil || d.Pad != long {
			t.Fatalf("record %s is not whole: %v", r.Run, err)
		}
	}
}
