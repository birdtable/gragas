package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-files/lock"
	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"
)

// Each journey says what it exercises and the risk a cheaper test cannot
// see (tests.journeys.1, tests.journeys.4): the in-process tests never run
// the shipped binary, its flags, its environment or its exit status.

// TestEveryExitCodeIsTheOneTheTableNames: go-run/exit's codes, each reached
// the way a person reaches it. Risk: a code that drifts breaks every script
// that branches on it, and only the process's real status shows it.
func TestEveryExitCodeIsTheOneTheTableNames(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	genRepo := genFixture(t, true)
	stale := genFixture(t, true)
	if err := os.WriteFile(filepath.Join(stale, "rulebook/rules.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unknownKey, _ := settingsFile(t, "version: 1\ncolr: never\n")
	newer, _ := settingsFile(t, "version: 2\n")
	// The settings file's reader refuses each of these two before a byte is
	// parsed: one larger than the most it reads (host/trust's MaxOwned,
	// 1 MiB), and one every other account may write. That one is made so by
	// go-files' trusttest.Loosen when its row starts, and is put back when the
	// row ends: a mode bit on Unix, an access list entry on Windows. A volume
	// that keeps no access list skips that row alone.
	tooLarge, _ := settingsFile(t, "version: 1\n#"+strings.Repeat("x", 1<<20)+"\n")
	shared, _ := settingsFile(t, "version: 1\n")
	plants := map[string]func(t *testing.T){
		"a settings file others may write": func(t *testing.T) {
			t.Helper()
			trusttest.Loosen(t, shared, trusttest.OthersWrite)
		},
	}
	// Four rows end on exit 5, so a planted row also names its reason, in
	// go-files' words: "can be written by" a mode bit, or an access list
	// entry that "lets" an account "change it". The output is read with its
	// lines joined, since a refusal wraps at the terminal's width.
	reasons := map[string]*regexp.Regexp{
		"a settings file others may write": regexp.MustCompile(`can be written by|lets \S+ change it`),
		"a names file with a bad pattern":  regexp.MustCompile(`line 1 of the names file is not a pattern that compiles`),
	}
	twice, _ := settingsFile(t, "version: 1\ncolour: never\ncolour: always\n")
	good, _ := settingsFile(t, "version: 1\n")
	badNames := filepath.Join(t.TempDir(), "names")
	if err := os.WriteFile(badNames, []byte("(unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		dir  string
		env  map[string]string
		args []string
		code int
		// usage is a command line that was not understood: stdout stays empty.
		usage bool
	}{
		{"check passes", repo, nil, []string{"check"}, 0, false},
		{"gen with nothing stale", genRepo, nil, []string{"gen", "--check"}, 0, false},
		{"gen --check with a stale file", stale, nil, []string{"gen", "--check"}, 1, false},
		{"init in a rulebook is not ready", repo, nil, []string{"init"}, 4, false},
		{"a mistyped command", repo, nil, []string{"chek"}, 2, true},
		{"a mistyped flag", repo, nil, []string{"gen", "--aply"}, 2, true},
		{"an argument nobody takes", repo, nil, []string{"check", "extra"}, 2, true},
		{"--apply with --check", repo, nil, []string{"gen", "--apply", "--check"}, 2, false},
		{"not a rulebook repository", t.TempDir(), nil, []string{"check"}, 4, false},
		{"a names file with a bad pattern", repo, map[string]string{"GRAGAS_NAMES_FILE": badNames}, []string{"check"}, 4, false},
		{"a settings file with an unknown key", repo, map[string]string{"GRAGAS_CONFIG": unknownKey}, []string{"check"}, 5, false},
		{"a settings file larger than gragas reads", repo, map[string]string{"GRAGAS_CONFIG": tooLarge}, []string{"check"}, 5, false},
		{"a settings file others may write", repo, map[string]string{"GRAGAS_CONFIG": shared}, []string{"check"}, 5, false},
		{"a settings file newer than this gragas", repo, map[string]string{"GRAGAS_CONFIG": newer}, []string{"check"}, 4, false},
		{"GRAGAS_CONFIG naming no file", repo, map[string]string{"GRAGAS_CONFIG": filepath.Join(t.TempDir(), "none.yaml")}, []string{"check"}, 4, false},
		{"a settings file with a key written twice", repo, map[string]string{"GRAGAS_CONFIG": twice}, []string{"check"}, 5, false},
		{"--config naming a bad file", repo, nil, []string{"check", "--config", unknownKey}, 5, false},
		{"--config over a bad GRAGAS_CONFIG", repo, map[string]string{"GRAGAS_CONFIG": unknownKey}, []string{"check", "--config", good}, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if plant, ok := plants[tc.name]; ok {
				plant(t)
			}
			r := run(t, tc.dir, tc.env, tc.args...)
			if r.code != tc.code {
				t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, tc.code, r.stdout, r.stderr)
			}
			if want, ok := reasons[tc.name]; ok && !want.MatchString(strings.Join(strings.Fields(r.stdout+r.stderr), " ")) {
				t.Errorf("the refusal does not say %q\nstdout:\n%s\nstderr:\n%s", want, r.stdout, r.stderr)
			}
			if tc.usage && r.stdout != "" {
				t.Errorf("a usage error printed on stdout:\n%s", r.stdout)
			}
		})
	}
}

// TestARunThatFindsTheLockHeldIsBusy: exit 3, and nothing written. Risk: two
// gen --apply runs at once would interleave their writes.
func TestARunThatFindsTheLockHeldIsBusy(t *testing.T) {
	t.Parallel()
	repo := genFixture(t, true)
	if err := os.WriteFile(filepath.Join(repo, "rulebook/rules.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// go-files' lock.Take makes no folder; gragas makes its state folder
	// before it takes the lock, and so does this test.
	if err := os.MkdirAll(stateDir(t), 0o700); err != nil {
		t.Fatal(err)
	}
	// The holder writes its note, as gragas does, beside the lock.
	held := filepath.Join(stateDir(t), "gragas.lock")
	l, err := lock.Take(held, held+".holder")
	if err == nil {
		err = l.Note(nil)
	}
	if err != nil {
		t.Fatalf("take the lock: %v", err)
	}
	defer func() { _ = l.Release() }()
	r := run(t, repo, nil, "gen", "--apply")
	if r.code != 3 || !strings.Contains(r.stdout, "another gragas run is writing") {
		t.Fatalf("exit %d, want 3 with the refusal\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got, _ := os.ReadFile(filepath.Join(repo, "rulebook/rules.json")); string(got) != "{}\n" {
		t.Error("a busy run wrote rules.json")
	}
}

// TestGenWritesOnlyWithApplyRecordsTheRunAndIsSafeTwice: the dry run writes
// nothing and ends with the apply line; --apply writes, under a run record
// with an id; a second --apply has nothing to do. Risk: a dry run that
// writes, or a write that leaves no record.
func TestGenWritesOnlyWithApplyRecordsTheRunAndIsSafeTwice(t *testing.T) {
	t.Parallel()
	repo := genFixture(t, true)
	target := filepath.Join(repo, "rulebook/rules.json")
	want, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dry := run(t, repo, nil, "gen")
	if dry.code != 6 || !strings.Contains(dry.stdout, "would write") ||
		!strings.HasSuffix(strings.TrimSpace(dry.stdout), "gragas gen --apply") {
		t.Fatalf("the dry run said:\n%s%s", dry.stdout, dry.stderr)
	}
	if got, _ := os.ReadFile(target); string(got) != "edited by hand\n" {
		t.Fatal("the dry run wrote the file")
	}
	applied := run(t, repo, nil, "gen", "--apply")
	if applied.code != 0 || !strings.Contains(applied.stdout, "done       wrote 1 generated file") {
		t.Fatalf("--apply said:\n%s%s", applied.stdout, applied.stderr)
	}
	if got, _ := os.ReadFile(target); string(got) != string(want) {
		t.Error("--apply did not write the generated file")
	}
	records := readRecords(t)
	last := records[len(records)-1]
	if last.ID == "" || last.Code != 0 || last.Ending != "done" || len(last.Changes) != 1 || last.Changes[0] != "write rulebook/rules.json" ||
		len(last.Extra.Undo) != 1 || last.Extra.Undo[0].How != "printed" || len(last.Extra.Undo[0].Lines) != 1 || last.Extra.Undo[0].Lines[0] != "git restore --source=HEAD -- rulebook/rules.json" {
		t.Errorf("the run record is %+v", last)
	}
	again := run(t, repo, nil, "gen", "--apply")
	if again.code != 0 || !strings.Contains(again.stdout, "the generated files are current") {
		t.Errorf("the second --apply said:\n%s%s", again.stdout, again.stderr)
	}
}

// TestAFailedWriteCarriesTheRunID: a person can quote the run. Risk: an
// error with no id cannot be matched to its record.
func TestAFailedWriteCarriesTheRunID(t *testing.T) {
	t.Parallel()
	repo := genFixture(t, true)
	dir := filepath.Join(repo, "rulebook")
	// A link to nowhere where the generated file goes: the write cannot go
	// through it, with no mode bits involved. (A folder there is read as a
	// layer, and the book is refused before any write.)
	if err := os.Remove(filepath.Join(dir, "rules.json")); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, "nowhere/rules.json", filepath.Join(dir, "rules.json"))
	r := run(t, repo, nil, "gen", "--apply")
	records := readRecords(t)
	id := records[len(records)-1].ID
	if r.code != 1 || id == "" || !strings.Contains(r.stdout, "(run "+id+")") {
		t.Fatalf("exit %d, the last record %q; the run said:\n%s%s", r.code, id, r.stdout, r.stderr)
	}
	// A failed replace leaves what stands at the target: the planted link is still a link.
	if _, err := os.Readlink(filepath.Join(dir, "rules.json")); err != nil {
		t.Errorf("the failed write took what stood at the target: %v", err)
	}
}

// record is the part of a go-run/runlog run record these journeys read.
type record struct {
	ID      string   `json:"id"`
	Code    int      `json:"code"`
	Ending  string   `json:"outcome"` // the closing word; go-run's runlog calls it outcome
	Changes []string `json:"changes"`
	// Extra is the fields gragas sets on its own, under the line's extra.
	Extra struct {
		// Undo is each way back the run recorded, in order: gragas's own
		// field since go-run v0.5.0 keeps none.
		Undo []struct {
			How   string   `json:"how"`
			Lines []string `json:"lines"`
		} `json:"undo"`
	} `json:"extra"`
}

// readRecords is every run in the log, each as its last line says.
func readRecords(t *testing.T) []record {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir(t), "runs.jsonl"))
	if err != nil {
		t.Fatalf("no run record: %v", err)
	}
	byID := map[string]int{}
	var out []record
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("a record line is not JSON: %v", err)
		}
		if i, seen := byID[r.ID]; seen {
			out[i] = r
			continue
		}
		byID[r.ID] = len(out)
		out = append(out, r)
	}
	if len(out) == 0 {
		t.Fatal("the run log is empty")
	}
	return out
}
