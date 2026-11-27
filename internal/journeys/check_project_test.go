package journeys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// projectBook is a format 3 book of one part, go, with one active rule that
// binds the task "gate" at rung 0.
func projectBook() map[string]string {
	return map[string]string{
		"rulebook.yaml":                 "format: 3\nname: example\nversion: 1.0.0\nkind: base\nlicences:\n  rules: CC-BY-4.0\nlanguage: en\nowners: [team-a]\ngovernance:\n  contribution: open\n  decision_required: false\n  trial_releases: 1\n  review_days: 30\n",
		"structure.yaml":                "format: 3\nlayers:\n  - slug: language\n    name: Language\n    parts:\n      - slug: go\n        name: Go\n        concerns:\n          - slug: errors\n            name: Errors\nkinds: []\n",
		"sources.yaml":                  "format: 3\nsources:\n  - slug: effective-go\n    kind: guide\n    title: Effective Go\n    locator: https://go.dev/doc/effective_go\n    attribution: The Go authors\n    verified: \"2026-11-01\"\n",
		"language/go/rules/errors.yaml": "format: 3\npart: go\nconcern: errors\nrules:\n  - id: go.errors.1\n    statement: Hold the gate task.\n    strength: must\n    status: active\n    origin: authored\n    sources:\n      - {ref: effective-go, backs: rule}\n    bindings:\n      - {tool: task, target: config, key: gate, severity: error, rung: 0, run_point: push}\n",
	}
}

// projectRepo is a project that pins the book beside it: a repository root,
// the pin, and a Taskfile that holds the task named by tasks.
func projectRepo(t *testing.T, tasks string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".git":             "gitdir: elsewhere\n",
		".gragas/pin.yaml": "rulebook: example\nsource: book\nparts: [go]\nformat: 1\n",
		"Taskfile.yml":     "version: '3'\ntasks:\n  " + tasks + ": {cmds: [echo]}\n",
	}
	for name, body := range projectBook() {
		files["book/"+name] = body
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestCheckBelowAPinJudgesTheProjectAgainstTheBook: the built binary, run in
// a project that pins a format 3 book, ends 1 with --fail-on naming the kind
// when the Taskfile lacks the task the rule binds, says so in the finding's
// words, and ends done when the task is there.
// Risk: a check that never reaches the book, or that passes an entry absent.
func TestCheckBelowAPinJudgesTheProjectAgainstTheBook(t *testing.T) {
	t.Parallel()
	env := ruleEnv(t)
	absent := run(t, projectRepo(t, "lint"), env, "check", "--fail-on", "tool-config")
	if absent.code != 1 || !strings.Contains(absent.stdout, "go.errors.1") || !strings.Contains(absent.stdout, "findings") {
		t.Errorf("a Taskfile without the task: exit %d, want 1 naming the rule and the word findings\n%s%s", absent.code, absent.stdout, absent.stderr)
	}
	held := run(t, projectRepo(t, "gate"), env, "check", "--fail-on", "tool-config")
	if held.code != 0 || strings.Contains(held.stdout, "go.errors.1 (task") || !strings.Contains(held.stdout, "done") {
		t.Errorf("a Taskfile with the task: exit %d, want done and no finding for the rule\n%s%s", held.code, held.stdout, held.stderr)
	}
}

// TestTheProjectCheckScreensAreGoldensAtThreeWidths: the built binary below a
// pin ends on two screens a person reads, a finding under --fail-on and a
// clean run, each drawn at 80, 60 and 40 columns against a golden written only
// with -update by a person who reads the diff (cli.look.6). Risk: a closing
// sentence or a finding that wraps badly or drifts unseen.
func TestTheProjectCheckScreensAreGoldensAtThreeWidths(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct{ name, task string }{
			{"project-findings", "lint"},
			{"project-done", "gate"},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				t.Parallel()
				env := ruleEnv(t)
				env["COLUMNS"] = cols
				args := []string{"check", "--fail-on", "tool-config"}
				dir := projectRepo(t, tc.task)
				r := run(t, dir, env, args...)
				got := fmt.Sprintf("$ gragas %s\n--- stdout\n%s--- stderr\n%s", strings.Join(args, " "), r.stdout, r.stderr)
				uitest.Golden(t, tc.name+"-"+cols, scrub(got, dir))
			})
		}
	}
}
