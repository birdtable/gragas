package fs_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/birdtable/go-run/runlog"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// TestARunIsRecordedWithItsChangesWayBackAndEnding: the run log holds a run
// as one record, its command line, change and ending, and under its own
// fields the build and each way back, printed for a person, in order.
func TestARunIsRecordedWithItsChangesWayBackAndEnding(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "runs.jsonl")
	r := hostfs.Runs{}.Begin(log, []string{"gragas", "gen", "--apply"}, "v1.2.3")
	if r.ID() == "" {
		t.Fatal("a run has no id")
	}
	r.Change("write rulebook/rules.json")
	r.WayBack(legacy.Undo{Lines: []string{"git restore rulebook/rules.json"}, Why: "tracked"})
	r.WayBack(legacy.Undo{Why: "the folder was new"})
	r.Finish(0, "done", "wrote 1 generated file")
	if err := r.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	runs, err := runlog.Log{Path: log}.Read(t.Context())
	if err != nil || len(runs) != 1 {
		t.Fatalf("the log holds %v, %v, want one run", runs, err)
	}
	got := runs[0]
	var build string
	if ok, err := got.Field("build", &build); !ok || err != nil || build != "v1.2.3" {
		t.Errorf("build = %q, %v, %v, want v1.2.3", build, ok, err)
	}
	var ways []wayBack
	if ok, err := got.Field("undo", &ways); !ok || err != nil || len(ways) != 2 ||
		ways[0].How != "printed" || !slices.Equal(ways[0].Lines, []string{"git restore rulebook/rules.json"}) || ways[0].Why != "tracked" ||
		ways[1].How != "printed" || len(ways[1].Lines) != 0 || ways[1].Why != "the folder was new" {
		t.Errorf("undo = %+v, %v, %v", ways, ok, err)
	}
	if got.ID != r.ID() || got.Outcome != "done" || got.Summary != "wrote 1 generated file" ||
		!slices.Equal(got.Changes, []string{"write rulebook/rules.json"}) || !slices.Equal(got.Argv, []string{"gragas", "gen", "--apply"}) {
		t.Errorf("record = %+v", got)
	}
}

// wayBack is the shape of the record's field undo, as a reader outside
// gragas would decode it.
type wayBack struct {
	How   string   `json:"how"`
	Lines []string `json:"lines"`
	Why   string   `json:"why"`
}

// TestALineGoRunV014WroteIsReadAsARunWithItsWayBack: a log that holds a line
// gragas wrote at go-run v0.1.4 (version 3, its way back at the top of the
// line, its build under extra) is read with its id, command line, changes,
// ending and build, and a run begun on that log is still recorded after it.
// Since go-run v0.5.2 a field at a line's top level that the
// record has no name for is read as the caller's own, so the old way back is
// gragas's field undo, read in the shape it was written in, the shape gragas
// writes today; nothing in gragas reads the log back. Risk: an old line that
// stops the read of every run after it, or a way back read in a shape it was
// not written in.
func TestALineGoRunV014WroteIsReadAsARunWithItsWayBack(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "runs.jsonl")
	old := `{"version":3,"id":"0123456789abcdef","argv":["gragas","gen","--apply"],"started":"2026-11-01T10:00:00Z","finished":"2026-11-01T10:00:01Z","code":0,"outcome":"done","summary":"wrote 1 generated file","changes":["write rulebook/rules.json"],"undo":[{"how":"printed","lines":["git restore rulebook/rules.json"],"why":"tracked"}],"extra":{"build":"v0.9.0"}}` + "\n"
	if err := os.WriteFile(log, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	r := hostfs.Runs{}.Begin(log, []string{"gragas", "gen", "--apply"}, "v1.2.3")
	r.Finish(0, "done", "nothing to write")
	if err := r.Err(); err != nil {
		t.Fatalf("a run on a log with a v0.1.4 line was not recorded: %v", err)
	}
	skipped := 0
	runs, err := runlog.Log{Path: log, Skipped: &skipped}.Read(t.Context())
	if err != nil || skipped != 0 || len(runs) != 2 {
		t.Fatalf("the log read as %+v, %v, %d skipped; want two runs and none skipped", runs, err, skipped)
	}
	got := runs[0]
	var build string
	if ok, err := got.Field("build", &build); !ok || err != nil || build != "v0.9.0" {
		t.Errorf("build = %q, %v, %v, want v0.9.0", build, ok, err)
	}
	if got.ID != "0123456789abcdef" || got.Outcome != "done" || got.Summary != "wrote 1 generated file" || got.Code != 0 ||
		!slices.Equal(got.Changes, []string{"write rulebook/rules.json"}) || !slices.Equal(got.Argv, []string{"gragas", "gen", "--apply"}) {
		t.Errorf("the v0.1.4 run = %+v", got)
	}
	var ways []wayBack
	if ok, err := got.Field("undo", &ways); !ok || err != nil || len(ways) != 1 ||
		ways[0].How != "printed" || !slices.Equal(ways[0].Lines, []string{"git restore rulebook/rules.json"}) || ways[0].Why != "tracked" {
		t.Errorf("the v0.1.4 run's way back = %+v, %v, %v; want its one printed line", ways, ok, err)
	}
	if runs[1].ID != r.ID() || runs[1].Summary != "nothing to write" {
		t.Errorf("the new run = %+v", runs[1])
	}
}

// TestARecordThatCouldNotOpenSaysSoAndRecordsNothing: a log whose folder is a
// file opens no record; the run has no id, its changes go nowhere, and Err
// says why, never stopping the run.
func TestARecordThatCouldNotOpenSaysSoAndRecordsNothing(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := hostfs.Runs{}.Begin(filepath.Join(file, "runs.jsonl"), []string{"gragas"}, "v1.2.3")
	r.Change("write x")
	r.WayBack(legacy.Undo{Lines: []string{"git restore x"}})
	r.Finish(1, "failed", "x")
	if r.ID() != "" || r.Err() == nil {
		t.Errorf("ID = %q, Err = %v, want no id and the failure", r.ID(), r.Err())
	}
}
