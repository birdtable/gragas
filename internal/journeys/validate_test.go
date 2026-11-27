package journeys

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// A rulebook of the format as a person writes one: an identity, a structure
// of one part and one concern, and a rules file with one rule.
const (
	vIdentity = `format: 3
name: example
version: 1.0.0
kind: base
licences:
  rules: CC-BY-4.0
language: en
owners: [team-a]
governance:
  contribution: open
  decision_required: false
  trial_releases: 1
  review_days: 30
`
	vStructure = `format: 3
layers:
  - slug: language
    name: Language
    parts:
      - slug: go
        name: Go
        concerns:
          - slug: errors
            name: Errors
kinds: []
`
	vRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
`
)

// gone, as a file's body, leaves that file out of the rulebook.
const gone = "-"

// rulebook is a folder holding a valid rulebook of the format, with the
// private names list unset so the run is the same on every machine.
func rulebook(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"rulebook.yaml": vIdentity, "structure.yaml": vStructure, "language/go/rules/errors.yaml": vRules,
		// the settings input the golangci-lint adapter names, which a rulebook that binds the tool holds
		"presets/golangci-lint/settings.yml": "version: \"2\"\nlinters:\n  enable:\n    # generated:linters:begin\n    # generated:linters:end\n"}
	maps.Copy(all, files)
	for name, body := range all {
		if body == gone {
			delete(all, name)
		}
	}
	put(t, dir, all)
	return dir
}

func validateRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"validate"}, args...)...)
}

func transcript(args []string, r result) string {
	return fmt.Sprintf("$ gragas %s\n--- exit %d\n--- stdout\n%s--- stderr\n%s", strings.Join(args, " "), r.code, r.stdout, r.stderr)
}

// manyFindings is a rulebook of 20 concerns whose rules each break the format
// once, 10 to a file (the loader stops a file at ten): 200 findings.
func manyFindings() map[string]string {
	var st strings.Builder
	st.WriteString("format: 3\nlayers:\n  - slug: language\n    name: Language\n    parts:\n      - slug: go\n        name: Go\n        concerns:\n")
	st.WriteString("          - slug: errors\n            name: Errors\n")
	files := map[string]string{}
	for c := 1; c <= 20; c++ {
		fmt.Fprintf(&st, "          - slug: c%d\n            name: Concern %d\n", c, c)
		var b strings.Builder
		fmt.Fprintf(&b, "format: 3\npart: go\nconcern: c%d\nrules:\n", c)
		for i := 1; i <= 10; i++ {
			fmt.Fprintf(&b, "  - id: go.c%d.%d\n    statement: Wrap an error.\n    strength: must\n    status: nonsense\n    origin: authored\n", c, i)
		}
		files[fmt.Sprintf("language/go/rules/c%d.yaml", c)] = b.String()
	}
	st.WriteString("kinds: []\n")
	files["structure.yaml"] = st.String()
	return files
}

// TestTheValidateGoldens: validate on a good rulebook, on a planted bad copy
// (a key written twice), on a newer format and on 200
// findings, as a pipe at 80, 60 and 40 columns, each against its golden
// (cli.look.8). Risk: a listing that changes shape unseen, or that cuts a
// list without saying so.
func TestTheValidateGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files map[string]string
		code  int
	}{
		{"validate-done", nil, 0},
		{"validate-findings", map[string]string{"rulebook.yaml": "format: 3\nname: a\nname: b\n"}, 1},
		{"validate-not-ready", map[string]string{"rulebook.yaml": strings.Replace(vIdentity, "format: 3", "format: 4", 1)}, 4},
		{"validate-200", manyFindings(), 1},
	}
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range cases {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				r := validateRun(t, rulebook(t, tc.files), cols)
				if r.code != tc.code {
					t.Errorf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
				}
				uitest.Golden(t, tc.name+"-"+cols, transcript([]string{"validate"}, r))
			})
		}
	}
}

// TestValidateJSONCarriesTheSameWordsAndTheContractVersion: one document on
// stdout for a good rulebook, a bad one and 200 findings, with the count, the
// shown number and the contract version (cli.output.1).
func TestValidateJSONCarriesTheSameWordsAndTheContractVersion(t *testing.T) {
	t.Parallel()
	type doc struct {
		Schema   int    `json:"schema"`
		Contract string `json:"contract"`
		Word     string `json:"word"`
		Rules    int    `json:"rules"`
		Total    int    `json:"total"`
		Shown    int    `json:"shown"`
		Findings []struct {
			Entity string `json:"entity"`
		} `json:"findings"`
		Notices []struct {
			Msg string `json:"msg"`
		} `json:"notices"`
	}
	for _, tc := range []struct {
		name         string
		files        map[string]string
		code         int
		word         string
		total, shown int
	}{
		{"json-validate-done", nil, 0, "done", 0, 0},
		{"json-validate-findings", map[string]string{"rulebook.yaml": "format: 3\nname: a\nname: b\n"}, 1, "findings", 1, 1},
		{"json-validate-200", manyFindings(), 1, "findings", 200, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validateRun(t, rulebook(t, tc.files), "80", "--json")
			var d doc
			if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
				t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
			}
			if r.code != tc.code || d.Word != tc.word || d.Total != tc.total || d.Shown != tc.shown || len(d.Findings) != tc.shown || d.Contract != "1.3" || d.Schema != 1 {
				t.Errorf("exit %d, doc %+v", r.code, d)
			}
			if len(d.Notices) == 0 {
				t.Error("the names check did not run, and no notice said so")
			}
			uitest.Golden(t, tc.name, r.stdout)
		})
	}
}

// TestValidateOverARootWithAnOpenRunEndsNotReadyNamingJournalRecover: the
// binary, a journal with a run that never closed, and a rulebook that is
// otherwise valid. Planted failure: validate not asking the journal.
func TestValidateOverARootWithAnOpenRunEndsNotReadyNamingJournalRecover(t *testing.T) {
	t.Parallel()
	w := newJournalWorld(t)
	w.repo = rulebook(t, nil)
	real, err := filepath.EvalSymlinks(w.repo)
	if err != nil {
		t.Fatal(err)
	}
	w.root = sum(filepath.ToSlash(real))
	w.line("r1", "intent", 1779000200, "")
	w.copy("r1", "rulebook.yaml", "x", true, 1779000201)
	w.write(nil)
	for _, cols := range []string{"80", "60", "40"} {
		t.Run(cols, func(t *testing.T) {
			env := map[string]string{"COLUMNS": cols, "GRAGAS_NAMES_FILE": ""}
			maps.Copy(env, w.env)
			r := run(t, w.repo, env, "validate")
			if r.code != 4 || !strings.Contains(r.stdout+r.stderr, "gragas journal recover --apply") {
				t.Errorf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			uitest.Golden(t, "validate-open-run-"+cols, w.scrubbed(transcript([]string{"validate"}, r)))
		})
	}
}

// TestValidateReadsAndWritesNothing: the folder is the same after a run, a
// good one and a bad one.
func TestValidateReadsAndWritesNothing(t *testing.T) {
	t.Parallel()
	for _, files := range []map[string]string{nil, {"rulebook.yaml": "format: 3\nname: a\nname: b\n"}} {
		dir := rulebook(t, files)
		before := snapshot(t, dir)
		validateRun(t, dir, "80")
		if after := snapshot(t, dir); strings.Join(before, "\n") != strings.Join(after, "\n") {
			t.Errorf("validate changed the folder:\n%v\n%v", before, after)
		}
	}
}

// TestValidateSARIFIsTheDocumentAloneOnStdout: --sarif on a good rulebook and
// on a planted bad copy answers with one SARIF document on stdout, held to
// its golden, which cmd/gragas checks against the contract's real SARIF
// schema; the closing word goes to stderr. A rulebook that cannot be read,
// and --json beside --sarif, leave stdout empty (cli.output.2). Risk: a
// SARIF consumer reading a screen, or half a document, as SARIF.
func TestValidateSARIFIsTheDocumentAloneOnStdout(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		files map[string]string
		code  int
		word  string
	}{
		{"sarif-validate-done", nil, 0, "done"},
		{"sarif-validate-findings", map[string]string{"rulebook.yaml": "format: 3\nname: a\nname: b\n"}, 1, "findings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validateRun(t, rulebook(t, tc.files), "80", "--sarif")
			if r.code != tc.code || !strings.HasPrefix(r.stdout, "{\n") || !strings.Contains(r.stderr, tc.word) {
				t.Errorf("exit %d, want %d, with SARIF on stdout and %q on stderr\n--- stdout\n%s--- stderr\n%s", r.code, tc.code, tc.word, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name, r.stdout)
		})
	}
	for _, tc := range []struct {
		name string
		args []string
		code int
		says string
	}{
		{"a newer format", []string{"--sarif"}, 4, "not ready"},
		{"--json beside --sarif", []string{"--sarif", "--json"}, 2, "--json and --sarif cannot be given together"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validateRun(t, rulebook(t, map[string]string{"rulebook.yaml": strings.Replace(vIdentity, "format: 3", "format: 4", 1)}), "80", tc.args...)
			if r.code != tc.code || r.stdout != "" || !strings.Contains(r.stderr, tc.says) {
				t.Errorf("exit %d, want %d, stdout empty and %q on stderr\n--- stdout\n%s--- stderr\n%s", r.code, tc.code, tc.says, r.stdout, r.stderr)
			}
		})
	}
}
