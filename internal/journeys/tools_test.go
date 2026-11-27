package journeys

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/birdtable/go-console/uitest"
)

// boundRules is one part with two rules that bind to three tools, none of
// which an installed plugin serves: two hold the same entry of the first.
const boundRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
    bindings:
      - {tool: golangci-lint, target: source, key: errorlint, severity: error, rung: 1, run_point: edit}
      - {tool: eslint, target: source, key: no-throw-literal, severity: error, rung: 1, run_point: edit}
  - id: go.errors.2
    statement: Name the cause.
    strength: should
    status: proposed
    origin: authored
    bindings:
      - {tool: golangci-lint, target: source, key: errorlint, severity: error, rung: 1, run_point: edit}
      - {tool: golangci-lint, target: source, key: nilerr, severity: warning, rung: 1, run_point: commit}
      - {tool: stylelint, target: source, key: color-no-invalid-hex, severity: error, rung: 1, run_point: edit}
`

// boundBook is a rulebook whose tested set names two tools: one a rule binds to,
// and one that none does.
func boundBook() map[string]string {
	return map[string]string{
		"rulebook.yaml":                 vIdentity + "tested:\n  tools:\n    - {name: golangci-lint, version: 2.15.0}\n    - {name: tsc, version: 6.0.3}\n  min_engine: 0.1.0\n",
		"language/go/rules/errors.yaml": boundRules,
	}
}

func toolsRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"tools"}, args...)...)
}

// TestTheToolsGoldens: tools on a rulebook that binds rules to three tools no
// plugin serves, which lists them and ends not ready naming plugin add, on
// one with no binding, on a folder with no rulebook and on a
// rulebook with a finding, each at 80 columns against its golden (cli.look.8).
// Risk: a listing that changes shape unseen, that lists a tool entry under the
// wrong rule, or that ends done while a bound tool is read by no plugin.
func TestTheToolsGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files map[string]string // nil: a folder with no rulebook
		code  int
		wants string
	}{
		{"tools-bound", boundBook(), 4, "gragas plugin add <folder>"},
		{"tools-no-binding", map[string]string{}, 0, "no bindings: 0 rules held"},
		{"tools-no-rulebook", nil, 4, "gragas init"},
		{"tools-findings", map[string]string{"rulebook.yaml": "format: 3\nname: a\nname: b\n"}, 4, "gragas validate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.files != nil {
				dir = rulebook(t, tc.files)
			}
			r := toolsRun(t, dir, "80")
			if r.code != tc.code || !strings.Contains(r.stdout+r.stderr, tc.wants) {
				t.Errorf("exit %d, want %d naming %q\n%s%s", r.code, tc.code, tc.wants, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name+"-80", transcript([]string{"tools"}, r))
		})
	}
}

// TestTheToolsListingFitsTheScreenAt60And40Columns: no line is wider
// than the screen, and the same tools come in the same order.
func TestTheToolsListingFitsTheScreenAt60And40Columns(t *testing.T) {
	t.Parallel()
	var tools []string
	for _, cols := range []string{"80", "60", "40"} {
		r := toolsRun(t, rulebook(t, boundBook()), cols)
		if r.code != 4 {
			t.Fatalf("exit %d at %s columns\n%s%s", r.code, cols, r.stdout, r.stderr)
		}
		width, err := strconv.Atoi(cols)
		if err != nil {
			t.Fatal(err)
		}
		var seen []string
		for line := range strings.SplitSeq(r.stdout, "\n") {
			if utf8.RuneCountInString(line) > width {
				t.Errorf("a line of %d columns at %s: %q", utf8.RuneCountInString(line), cols, line)
			}
			for _, tool := range []string{"eslint", "golangci-lint", "stylelint", "tsc"} {
				if strings.HasPrefix(line, tool+", ") { // a heading, never the closing lines, which are indented
					seen = append(seen, tool)
				}
			}
		}
		if tools == nil {
			tools = seen
		}
		if len(seen) < 4 || strings.Join(seen, ",") != strings.Join(tools, ",") {
			t.Errorf("at %s columns the tools read %v, want the same in the same order as at 80", cols, seen)
		}
	}
}

// TestToolsJSONCarriesTheContractVersionAndEveryEntryWithItsRules: one document
// on stdout, the counts, and the entry both rules hold under both (cli.output.1);
// with no plugin installed, no tool names one, the word is not ready and the
// three bound tools are unserved.
func TestToolsJSONCarriesTheContractVersionAndEveryEntryWithItsRules(t *testing.T) {
	t.Parallel()
	type doc struct {
		Schema      int      `json:"schema"`
		Contract    string   `json:"contract"`
		Command     string   `json:"command"`
		Word        string   `json:"word"`
		Unserved    []string `json:"unserved"`
		Rules       int      `json:"rules"`
		Entries     int      `json:"entries"`
		Unexplained []string `json:"unexplained"`
		Tools       []struct {
			Tool    string                          `json:"tool"`
			Tested  string                          `json:"tested"`
			Plugin  *struct{ Name, Version string } `json:"plugin"`
			Entries []struct {
				Key   string   `json:"key"`
				Rules []string `json:"rules"`
			} `json:"entries"`
		} `json:"tools"`
	}
	r := toolsRun(t, rulebook(t, boundBook()), "80", "--json")
	var d doc
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
	}
	if r.code != 4 || d.Contract != "1.3" || d.Schema != 2 || d.Command != "tools" || d.Word != "not ready" || strings.Join(d.Unserved, ",") != "eslint,golangci-lint,stylelint" || d.Rules != 2 || d.Entries != 4 ||
		d.Unexplained == nil || len(d.Unexplained) != 0 || len(d.Tools) != 4 {
		t.Fatalf("exit %d, doc %+v", r.code, d)
	}
	g := d.Tools[1]
	if g.Tool != "golangci-lint" || g.Tested != "2.15.0" || g.Plugin != nil || len(g.Entries) != 2 || g.Entries[0].Key != "errorlint" ||
		strings.Join(g.Entries[0].Rules, ",") != "go.errors.1,go.errors.2" {
		t.Errorf("golangci-lint = %+v", g)
	}
	uitest.Golden(t, "json-tools-bound", r.stdout)
}

// TestToolsReadsAndWritesNothing: the folder is the same after a run.
func TestToolsReadsAndWritesNothing(t *testing.T) {
	t.Parallel()
	dir := rulebook(t, boundBook())
	before := snapshot(t, dir)
	toolsRun(t, dir, "80")
	if after := snapshot(t, dir); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("tools changed the folder:\n%v\n%v", before, after)
	}
}

// standinRule is one more rule of the go part's errors, after promoteRules'
// three, bound to the standin tool, which the standin plugin serves.
const standinRule = `  - id: go.errors.4
    statement: Say no BAD.
    strength: may
    status: proposed
    origin: authored
    bindings:
      - {tool: standin, target: source, key: bad, severity: error, rung: 1, run_point: edit}
`

// TestToolsNamesThePluginThatServesEachToolAndAnUnapprovedOneIsNotReady
// : in a rulebook whose one bound tool the standin plugin serves,
// added and approved, tools names it by name and version and ends done; with
// the plugin added and not approved, the listing ends not ready naming the
// approval, for a plugin that does not open says no tool. Each at 80 columns
// against its golden. Risk: a listing that names an adapter the engine
// embeds in place of the plugin that reads the tool, or that reads an
// unapproved plugin's manifest.
func TestToolsNamesThePluginThatServesEachToolAndAnUnapprovedOneIsNotReady(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		opts  recordOpts
		code  int
		wants string
	}{
		{"tools-served", recordOpts{}, 0, "standin, not tested, plugin standin 1.0.0"},
		{"tools-unapproved", recordOpts{unapproved: true}, 4, "gragas plugin approve standin@1.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, env := recordProject(t, tc.opts)
			put(t, filepath.Join(dir, "language", "go", "rules"), map[string]string{"errors.yaml": promoteRules + standinRule})
			env["COLUMNS"] = "80"
			r := run(t, dir, env, "tools")
			if r.code != tc.code || !strings.Contains(r.stdout+r.stderr, tc.wants) {
				t.Errorf("exit %d, want %d naming %q\n%s%s", r.code, tc.code, tc.wants, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name+"-80", recordMasked([]string{"tools"}, r))
		})
	}
}
