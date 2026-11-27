package journeys

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// coverRules is a part with two active rules: one binds golangci-lint's errorlint,
// the other adopts an upstream preset whole at rung 0, and a third has no binding.
const coverRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: active
    origin: authored
    bindings:
      - {tool: golangci-lint, target: config, key: errorlint, severity: error, rung: 1, run_point: edit}
  - id: go.errors.2
    statement: Use the recommended set.
    strength: should
    status: active
    origin: authored
    bindings:
      - {tool: golangci-lint, target: config, key: recommended, severity: error, rung: 0, run_point: edit}
  - id: go.errors.3
    statement: Name the cause.
    strength: should
    status: active
    origin: authored
`

// coverUpstream is the flattened set of golangci-lint: the recommended preset
// held three entries at the pin, and the upstream holds a fourth now.
const coverUpstream = `{"version":1,"tool":"golangci-lint","sets":[{"preset":"recommended","entries":["errcheck","govet","staticcheck"]}],"observed":["errcheck","errorlint","govet","staticcheck","unused"]}`

func coverBook(upstream string) map[string]string {
	files := map[string]string{"language/go/rules/errors.yaml": coverRules}
	if upstream != "" {
		files["upstream/golangci-lint.flat.json"] = upstream
	}
	return files
}

func coverRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"coverage"}, args...)...)
}

// TestTheCoverageGoldens: coverage both ways on a rulebook that binds one entry
// and adopts an upstream set whole (with one entry added after the pin), on the
// same with no upstream file, on a rulebook with no binding (the line that offers
// bind), on a folder with no rulebook and on a file in a newer version, at 80
// columns against its golden (cli.look.8). Risk: an adopted set counted entry by
// entry, an entry added after the pin missing from unmapped, or the empty state
// without its next command.
func TestTheCoverageGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files map[string]string // nil: a folder with no rulebook
		code  int
		wants string
	}{
		{"coverage-set-and-added-entry", coverBook(coverUpstream), 0, "1 unmapped entry"},
		{"coverage-no-upstream-file", coverBook(""), 0, "0 rules held, 2 entries explained"},
		{"coverage-no-binding", map[string]string{}, 0, "gragas bind"},
		{"coverage-no-rulebook", nil, 4, "gragas init"},
		{"coverage-newer-upstream", coverBook(`{"version":2,"tool":"golangci-lint","sets":[],"observed":[]}`), 4, "version 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.files != nil {
				dir = rulebook(t, tc.files)
			}
			r := coverRun(t, dir, "80")
			if r.code != tc.code || !strings.Contains(r.stdout+r.stderr, tc.wants) {
				t.Errorf("exit %d, want %d naming %q\n%s%s", r.code, tc.code, tc.wants, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name+"-80", transcript([]string{"coverage"}, r))
		})
	}
}

// TestTheAdoptedSetIsOneUnitInTheListingAndInTheJSON: the recommended set is one
// entry explained, with its three members beneath it, and the entry added
// after the pin is the one unmapped (--json,
// cli.output.1).
func TestTheAdoptedSetIsOneUnitInTheListingAndInTheJSON(t *testing.T) {
	t.Parallel()
	var d struct {
		Schema           int    `json:"schema"`
		Contract         string `json:"contract"`
		Command          string `json:"command"`
		Word             string `json:"word"`
		RulesHeld        int    `json:"rules_held"`
		EntriesExplained int    `json:"entries_explained"`
		Coverage         struct {
			RulesToBindings []string `json:"rules_to_bindings"`
			BindingsToRules []string `json:"bindings_to_rules"`
		} `json:"coverage"`
		Entries []struct {
			Set     string   `json:"set"`
			Members []string `json:"members"`
		} `json:"entries"`
		Unmapped []struct{ Tool, Entry string } `json:"unmapped"`
	}
	r := coverRun(t, rulebook(t, coverBook(coverUpstream)), "80", "--json")
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
	}
	if r.code != 0 || d.Contract != "1.3" || d.Schema != 1 || d.Command != "coverage" || d.Word != "done" || d.EntriesExplained != 2 ||
		len(d.Coverage.BindingsToRules) != 2 || len(d.Unmapped) != 1 || d.Unmapped[0].Entry != "unused" {
		t.Fatalf("exit %d, doc %+v", r.code, d)
	}
	sets := 0
	for _, e := range d.Entries {
		if e.Set == "recommended" && len(e.Members) == 3 {
			sets++
		}
	}
	if sets != 1 {
		t.Errorf("the recommended set is not one unit with three members: %+v", d.Entries)
	}
	uitest.Golden(t, "json-coverage-set-and-added-entry", r.stdout)
}

// TestCoverageReadsAndWritesNothing: the folder is the same after a run.
func TestCoverageReadsAndWritesNothing(t *testing.T) {
	t.Parallel()
	dir := rulebook(t, coverBook(coverUpstream))
	before := snapshot(t, dir)
	coverRun(t, dir, "80")
	if after := snapshot(t, dir); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("coverage changed the folder:\n%v\n%v", before, after)
	}
}

// TestOnlyTheFileAtTheContractPathIsReadForATool: a second flat.json for the
// same tool in a folder below upstream/ neither replaces the direct file nor is
// read at all, so the adopted set stays one unit and the added entry stays
// unmapped. Risk: the last file listed winning, with no word.
func TestOnlyTheFileAtTheContractPathIsReadForATool(t *testing.T) {
	t.Parallel()
	files := coverBook(coverUpstream)
	files["upstream/z/golangci-lint.flat.json"] = `{"version":1,"tool":"golangci-lint","sets":[],"observed":[]}`
	r := coverRun(t, rulebook(t, files), "80")
	if r.code != 0 || !strings.Contains(r.stdout, "1 unmapped entry") {
		t.Errorf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestARefusedUpstreamFileEndsRefusedWithTheLineToType: a file the reader
// refuses (a member missing) ends refused, exit 5, with the line to type, in
// the listing and under --json, and draws no listing. Risk: a refusal that
// the session cannot close, ending exit 1 with no word.
func TestARefusedUpstreamFileEndsRefusedWithTheLineToType(t *testing.T) {
	t.Parallel()
	dir := rulebook(t, coverBook(`{"version":1,"tool":"golangci-lint"}`))
	for _, args := range [][]string{nil, {"--json"}} {
		r := coverRun(t, dir, "80", args...)
		out := r.stdout + r.stderr
		if r.code != 5 || !strings.Contains(out, "refused") || !strings.Contains(out, "gragas coverage") || strings.Contains(out, "could not say how it ended") {
			t.Errorf("%v: exit %d, want 5 refused\n%s", args, r.code, out)
		}
	}
}
