package journeys

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
	"github.com/birdtable/go-console/uitest"
)

// newFolder is an empty git repository whose configured identity is a fixed
// person, which is where init takes who it is from (S67).
func newFolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ctx := t.Context()
	for _, c := range []*exec.Cmd{
		exec.CommandContext(ctx, "git", "init", "-q"),
		exec.CommandContext(ctx, "git", "config", "user.name", "A Person"),
		exec.CommandContext(ctx, "git", "config", "user.email", "a@example.test"),
	} {
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", c.Args, err, out)
		}
	}
	return dir
}

func initRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	git := journeyGit(t)
	settings, _ := settingsFile(t, "version: 1\nprograms:\n  git: "+git+"\n")
	env["GRAGAS_CONFIG"] = settings
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"init"}, args...)...)
}

// runID is a run id as the screen prints it: go-run's sixteen hex digits, or
// a prefix of eight or more, that differ each run.
var runID = regexp.MustCompile(`run\s+[0-9a-f]{8,16}\b`)

// runMasked is s with its run id replaced, so a golden does not move with it.
func runMasked(s string) string { return runID.ReplaceAllString(s, "run <id>") }

func initFlags() []string {
	return []string{"--kind", "rulebook", "--name", "demo", "--owner", "team-a"}
}

// TestInitThenValidateEndsDoneWithZeroRules: in an empty folder init --apply
// writes identity, structure and a first history line, validate reads them
// back and ends done with no rule, and a second init ends not ready, already a
// rulebook (spec S67; the zero-rule rulebook the format now allows).
func TestInitThenValidateEndsDoneWithZeroRules(t *testing.T) {
	t.Parallel()
	dir := newFolder(t)
	r := initRun(t, dir, "80", append(initFlags(), "--apply")...)
	if r.code != 0 {
		t.Fatalf("init --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	v := validateRun(t, dir, "80")
	if v.code != 0 || !strings.Contains(v.stdout, "0 rules") {
		t.Fatalf("validate: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	again := initRun(t, dir, "80", initFlags()...)
	if again.code != 4 || !strings.Contains(again.stdout, "already a rulebook") {
		t.Fatalf("second init: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
}

// TestInitGoldens: init off a terminal without --apply (the plan, then
// declined, exit 6), with --apply, and without --kind (usage, exit 2), at 80,
// 60 and 40 columns, and validate over the rulebook init made. Risk: a plan
// screen that moves unseen, a guard line that drops out of the transcript.
func TestInitGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"init-declined", initFlags(), 6},
		{"init-done", append(append([]string{}, initFlags()...), "--apply"), 0},
		{"init-usage", nil, 2},
	}
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range cases {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				r := initRun(t, newFolder(t), cols, tc.args...)
				if r.code != tc.code {
					t.Errorf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
				}
				uitest.Golden(t, tc.name+"-"+cols, runMasked(transcript(append([]string{"init"}, tc.args...), r)))
			})
		}
		t.Run("validate-empty-"+cols, func(t *testing.T) {
			dir := newFolder(t)
			if r := initRun(t, dir, cols, append(initFlags(), "--apply")...); r.code != 0 {
				t.Fatalf("init: exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			r := validateRun(t, dir, cols)
			uitest.Golden(t, "validate-empty-"+cols, transcript([]string{"validate"}, r))
		})
	}
}

// TestInitJSONCarriesTheGuardLines: the three guard lines, the gate's rule and
// the ignore line are in the document of the dry run and of the apply, and
// the validate of the result says zero rules in JSON.
func TestInitJSONCarriesTheGuardLines(t *testing.T) {
	t.Parallel()
	dir := newFolder(t)
	r := initRun(t, dir, "80", append(initFlags(), "--json")...)
	if r.code != 6 {
		t.Fatalf("exit %d, want 6\n%s%s", r.code, r.stdout, r.stderr)
	}
	var d struct {
		Data struct {
			Guard  []string `json:"guard"`
			Gate   string   `json:"gate"`
			Ignore string   `json:"ignore"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, r.stdout)
	}
	if len(d.Data.Guard) != 3 || d.Data.Guard[0] != "gragas guard staged || exit 2" || d.Data.Guard[1] != "gragas guard edit || exit 2" || d.Data.Guard[2] != "gragas guard staged --merge-hook || exit 2" || d.Data.Ignore != ".gragas/state.yaml" {
		t.Errorf("data = %+v", d.Data)
	}
	uitest.Golden(t, "json-init-declined", r.stdout)
	if initRun(t, dir, "80", append(initFlags(), "--apply")...).code != 0 {
		t.Fatal("apply failed")
	}
	uitest.Golden(t, "json-validate-empty", validateRun(t, dir, "80", "--json").stdout)
}

// moments are the parts of a history line that differ between two runs: when
// it was made and the run that made it.
var moments = regexp.MustCompile(`"(at|run|when|plan)":\s*("[^"]*"|[0-9]+)`)

// writtenBy is every file init wrote in dir, as name and body, the moments of
// a history line masked.
func writtenBy(t *testing.T, dir string) string {
	t.Helper()
	var out strings.Builder
	for _, name := range []string{"rulebook.yaml", "structure.yaml", "history.jsonl"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("init wrote no %s: %v", name, err)
		}
		fmt.Fprintf(&out, "--- %s\n%s", name, moments.ReplaceAll(b, []byte(`"$1":0`)))
	}
	return out.String()
}

// TestInitTheApplyLineItPrintsWritesThePlanItWasMadeFor: for each flag of init
// that shapes the rulebook, a dry run ends on an apply line, and that line, run
// as printed in a second folder, writes the same files as the typed command
// with --apply does in a first. Risk: an apply line built from the name alone,
// so following it writes a different rulebook than the one the plan showed.
func TestInitTheApplyLineItPrintsWritesThePlanItWasMadeFor(t *testing.T) {
	t.Parallel()
	overlay := []string{"--rulebook-kind", "overlay", "--parent", "base-rules", "--parent-version", "2.0.0", "--parent-hash", hashA}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"only the owner", nil},
		{"version", []string{"--version", "2.3.4"}},
		{"language", []string{"--language", "de"}},
		{"licence", []string{"--licence", "CC0-1.0"}},
		{"presets licence", []string{"--presets-licence", "Apache-2.0"}},
		{"docs url", []string{"--docs-url", "https://example.test/docs"}},
		{"layer", []string{"--layer", "language"}},
		{"part", []string{"--part", "go"}},
		{"owners, one with a space", []string{"--owner", "A B", "--owner", "it's me"}},
		{"parent, version and hash", overlay},
		{"two imports", []string{"--import", "other@1.0.0=go=" + hashB, "--import", "third@2.1.0=ts,go=" + hashA}},
		{"every one together", append([]string{"--version", "2.3.4", "--language", "de", "--licence", "CC0-1.0", "--presets-licence", "Apache-2.0", "--docs-url", "https://example.test/docs", "--layer", "language", "--part", "go", "--owner", "A B"}, overlay...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--kind", "rulebook", "--name", "demo"}, tc.args...)
			if !slices.Contains(args, "--owner") {
				// git is never asked for an owner
				args = append(args, "--owner", "team-a")
			}
			typed := newFolder(t)
			if r := initRun(t, typed, "80", append(slices.Clone(args), "--apply")...); r.code != 0 {
				t.Fatalf("typed --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			dir := newFolder(t)
			plan := initRun(t, dir, "80", append(slices.Clone(args), "--json")...)
			var doc struct {
				Next string `json:"next"`
			}
			if err := json.Unmarshal([]byte(plan.stdout), &doc); err != nil || doc.Next == "" {
				t.Fatalf("no next line in the plan: %v\n%s%s", err, plan.stdout, plan.stderr)
			}
			words := shellWords(doc.Next)
			if len(words) < 3 || words[0] != "gragas" || words[1] != "init" || words[len(words)-1] != "--apply" {
				t.Fatalf("the next line is %q", doc.Next)
			}
			if again, err := shell.Quote(shell.POSIX, words); err != nil || again != doc.Next {
				t.Fatalf("the next line %q is not the quoter's line for the words it reads back as: %q, %v", doc.Next, again, err)
			}
			if r := initRun(t, dir, "80", words[2:]...); r.code != 0 {
				t.Fatalf("the printed line %q: exit %d\n%s%s", doc.Next, r.code, r.stdout, r.stderr)
			}
			if got, want := writtenBy(t, dir), writtenBy(t, typed); got != want {
				t.Errorf("the printed line %q wrote another rulebook\n--- printed\n%s\n--- typed\n%s", doc.Next, got, want)
			}
		})
	}
}

// The two content hashes the overlay and import journeys name.
const (
	hashA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// TestInitAnOverlayNeedsTheHashOfItsParentsRelease (S67): an overlay
// with its parent's hash writes it into identity with the imports' hashes; an
// overlay with none, a hash that is no sha256 digest and an import with a bad
// one each end on a usage line that names the flag, and write nothing.
// Planted failure: a form that takes any text as the hash.
func TestInitAnOverlayNeedsTheHashOfItsParentsRelease(t *testing.T) {
	t.Parallel()
	overlay := append(initFlags(), "--rulebook-kind", "overlay", "--parent", "base", "--parent-version", "1.0.0")
	t.Run("with its hashes", func(t *testing.T) {
		t.Parallel()
		dir := newFolder(t)
		args := append(slices.Clone(overlay), "--parent-hash", hashA, "--import", "other@2.0.0=go,ts="+hashB, "--apply")
		if r := initRun(t, dir, "80", args...); r.code != 0 {
			t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
		b, err := os.ReadFile(filepath.Join(dir, "rulebook.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`content_hash: "` + hashA + `"`, `content_hash: "` + hashB + `"`, `rulebook: "other"`} {
			if !strings.Contains(string(b), want) {
				t.Errorf("rulebook.yaml lacks %s:\n%s", want, b)
			}
		}
	})
	for _, tc := range []struct {
		name, flag string
		extra      []string
	}{
		{"no hash", "--parent-hash", nil},
		{"a malformed hash", "--parent-hash", []string{"--parent-hash", "sha256:xyz"}},
		{"an import with a bad hash", "--import", []string{"--parent-hash", hashA, "--import", "other@2.0.0=go=nothex"}},
		{"an import with no hash", "--import", []string{"--parent-hash", hashA, "--import", "other@2.0.0=go="}},
		{"an import that is no pin", "--import", []string{"--parent-hash", hashA, "--import", "other"}},
		{"a parent version that is two numbers", "--parent-version", []string{"--parent-hash", hashA, "--parent-version", "1.0"}},
		{"an import version that is two numbers", "--import", []string{"--parent-hash", hashA, "--import", "other@1.0=go=" + hashB}},
		{"an import part in capitals", "--import", []string{"--parent-hash", hashA, "--import", "other@1.0.0=Go=" + hashB}},
		{"an import part named twice", "--import", []string{"--parent-hash", hashA, "--import", "other@1.0.0=go,go=" + hashB}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := newFolder(t)
			r := initRun(t, dir, "80", append(slices.Clone(overlay), append(slices.Clone(tc.extra), "--apply")...)...)
			if r.code == 0 || !strings.Contains(r.stdout+r.stderr, tc.flag) {
				t.Fatalf("exit %d, want a usage line naming %s\n%s%s", r.code, tc.flag, r.stdout, r.stderr)
			}
			if _, err := os.Stat(filepath.Join(dir, "rulebook.yaml")); err == nil {
				t.Error("a refused init wrote rulebook.yaml")
			}
		})
	}
}
