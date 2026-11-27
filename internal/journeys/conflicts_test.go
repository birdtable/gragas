package journeys

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/birdtable/go-console/uitest"
)

// conflictRules is one part with four trial rules on golangci-lint: the first
// two set the entry errorlint to different severities, and the last two hold
// the entry nilerr at the same value, the many-to-many case.
const conflictRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: trial
    origin: authored
    bindings:
      - {tool: golangci-lint, target: source, key: errorlint, severity: error, rung: 1, run_point: edit}
  - id: go.errors.2
    statement: Wrap it quietly.
    strength: should
    status: trial
    origin: authored
    bindings:
      - {tool: golangci-lint, target: source, key: errorlint, severity: warning, rung: 1, run_point: edit}
  - id: go.errors.3
    statement: Check a nil error.
    strength: should
    status: trial
    origin: authored
    bindings:
      - {tool: golangci-lint, target: source, key: nilerr, severity: warning, rung: 1, run_point: commit}
  - id: go.errors.4
    statement: Give a reason when you return a nil error.
    strength: may
    status: trial
    origin: authored
    bindings:
      - {tool: golangci-lint, target: source, key: nilerr, severity: warning, rung: 1, run_point: commit}
`

func conflictBook(rules string) map[string]string {
	return map[string]string{
		"rulebook.yaml":                 vIdentity + "tested:\n  tools:\n    - {name: golangci-lint, version: 2.15.0}\n  min_engine: 0.1.0\n",
		"language/go/rules/errors.yaml": rules,
	}
}

// agreeing is the same book with the second rule's severity made the first's,
// which leaves no conflict.
func agreeing() string {
	return strings.Replace(conflictRules, "severity: warning, rung: 1, run_point: edit", "severity: error, rung: 1, run_point: edit", 1)
}

func conflictsRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"conflicts"}, args...)...)
}

// TestTheConflictsGoldens: conflicts on a rulebook with one entry two rules set
// differently and one they share, on one where they agree, on a folder with no
// rulebook and on a rulebook with a finding, each at 80 columns against its
// golden (cli.look.8): findings and exit 1 with a conflict open, done and exit
// 0 with none. Risk: a listing that blames the shared entry, hides the real
// conflict or picks a winner.
func TestTheConflictsGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files map[string]string // nil: a folder with no rulebook
		code  int
		wants string
	}{
		{"conflicts-open", conflictBook(conflictRules), 1, "go.errors.1"},
		{"conflicts-none", conflictBook(agreeing()), 0, "no conflicts"},
		{"conflicts-no-rulebook", nil, 4, "gragas init"},
		{"conflicts-findings", map[string]string{"rulebook.yaml": "format: 3\nname: a\nname: b\n"}, 4, "gragas validate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.files != nil {
				dir = rulebook(t, tc.files)
			}
			r := conflictsRun(t, dir, "80")
			if r.code != tc.code || !strings.Contains(r.stdout+r.stderr, tc.wants) {
				t.Errorf("exit %d, want %d naming %q\n%s%s", r.code, tc.code, tc.wants, r.stdout, r.stderr)
			}
			if tc.name == "conflicts-open" && strings.Contains(r.stdout, "nilerr") {
				t.Errorf("the entry two rules share at one value is listed as a conflict:\n%s", r.stdout)
			}
			uitest.Golden(t, tc.name+"-80", transcript([]string{"conflicts"}, r))
		})
	}
}

// TestTheConflictsListingFitsTheScreenAt60And40Columns: no line is
// wider than the screen, and the same rules are named.
func TestTheConflictsListingFitsTheScreenAt60And40Columns(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		r := conflictsRun(t, rulebook(t, conflictBook(conflictRules)), cols)
		if r.code != 1 {
			t.Fatalf("exit %d at %s columns\n%s%s", r.code, cols, r.stdout, r.stderr)
		}
		width, err := strconv.Atoi(cols)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(r.stdout, "\n") {
			if utf8.RuneCountInString(line) > width {
				t.Errorf("a line of %d columns at %s: %q", utf8.RuneCountInString(line), cols, line)
			}
		}
		for _, id := range []string{"go.errors.1", "go.errors.2"} {
			if !strings.Contains(r.stdout, id) {
				t.Errorf("at %s columns the listing does not name %s:\n%s", cols, id, r.stdout)
			}
		}
	}
}

// TestConflictsJSONCarriesTheOpenConflictWithItsSidesAndTheResolutions: one
// document on stdout with the word findings, and for the conflict the entry,
// what differs and each rule with its severity (cli.output.1).
func TestConflictsJSONCarriesTheOpenConflictWithItsSidesAndTheResolutions(t *testing.T) {
	t.Parallel()
	type doc struct {
		Schema    int    `json:"schema"`
		Contract  string `json:"contract"`
		Command   string `json:"command"`
		Word      string `json:"word"`
		Open      int    `json:"open"`
		Resolved  int    `json:"resolved"`
		Conflicts []struct {
			Entry   struct{ Tool, Target, Key string } `json:"entry"`
			Differs []string                           `json:"differs"`
			Sides   []struct {
				Rule     string `json:"rule"`
				Severity string `json:"severity"`
			} `json:"sides"`
		} `json:"conflicts"`
		Resolutions []string `json:"resolutions"`
	}
	r := conflictsRun(t, rulebook(t, conflictBook(conflictRules)), "80", "--json")
	var d doc
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
	}
	if r.code != 1 || d.Contract != "1.3" || d.Schema != 1 || d.Command != "conflicts" || d.Word != "findings" || d.Open != 1 || d.Resolved != 0 ||
		len(d.Conflicts) != 1 || len(d.Resolutions) != 3 {
		t.Fatalf("exit %d, doc %+v", r.code, d)
	}
	c := d.Conflicts[0]
	if c.Entry.Tool != "golangci-lint" || c.Entry.Key != "errorlint" || strings.Join(c.Differs, ",") != "severity" || len(c.Sides) != 2 ||
		c.Sides[0].Rule != "go.errors.1" || c.Sides[0].Severity != "error" || c.Sides[1].Rule != "go.errors.2" || c.Sides[1].Severity != "warning" {
		t.Errorf("conflict = %+v", c)
	}
	uitest.Golden(t, "json-conflicts-open", r.stdout)
	if r := conflictsRun(t, rulebook(t, conflictBook(agreeing())), "80", "--json"); r.code != 0 || !strings.Contains(r.stdout, `"word": "done"`) {
		t.Errorf("no conflict, --json: exit %d\n%s", r.code, r.stdout)
	}
}

// TestConflictsSARIFIsTheDocumentAloneOnStdout: --sarif answers with one SARIF
// document on stdout, a result for the open conflict, held to its golden; with
// none open the document has no result (cli.output.2).
func TestConflictsSARIFIsTheDocumentAloneOnStdout(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		rules string
		code  int
		word  string
	}{
		{"sarif-conflicts-open", conflictRules, 1, "findings"},
		{"sarif-conflicts-none", agreeing(), 0, "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := conflictsRun(t, rulebook(t, conflictBook(tc.rules)), "80", "--sarif")
			if r.code != tc.code || !strings.HasPrefix(r.stdout, "{\n") || !strings.Contains(r.stderr, tc.word) {
				t.Errorf("exit %d, want %d, with SARIF on stdout and %q on stderr\n--- stdout\n%s--- stderr\n%s", r.code, tc.code, tc.word, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name, r.stdout)
		})
	}
}

// TestConflictsReadsAndWritesNothing: the folder is the same after a run.
func TestConflictsReadsAndWritesNothing(t *testing.T) {
	t.Parallel()
	dir := rulebook(t, conflictBook(conflictRules))
	before := snapshot(t, dir)
	conflictsRun(t, dir, "80")
	if after := snapshot(t, dir); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("conflicts changed the folder:\n%v\n%v", before, after)
	}
}
