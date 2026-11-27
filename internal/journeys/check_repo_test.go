package journeys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"

	"github.com/birdtable/go-files/filestest"
)

// The three repository checks below a pin: the names, the
// tracked files and the counts, as the project check lists them.

// repoBook is projectBook whose go rules bind the five keys the three checks
// judge under, each an active rule at rung 0, and no task.
func repoBook() map[string]string {
	b := projectBook()
	rules := "format: 3\npart: go\nconcern: errors\nrules:\n"
	for i, key := range []string{"names", "link", "fold", "debt", "directive"} {
		rules += fmt.Sprintf("  - id: go.errors.%d\n    statement: Hold %s.\n    strength: must\n    status: active\n    origin: authored\n"+
			"    sources:\n      - {ref: effective-go, backs: rule}\n    bindings:\n"+
			"      - {tool: gragas, target: config, key: %s, severity: error, rung: 0, run_point: push}\n", i+1, key, key)
	}
	b["language/go/rules/errors.yaml"] = rules
	return b
}

// repoProject is a project in a git repository of its own that pins repoBook
// beside it and holds the tier contract (the core's clean Taskfile and hook),
// so the only findings of a run are the three checks'. Its one commit holds
// a.go and notes.md and is the branch the environment names as the base ref;
// the names list of the environment holds the one name zorbquux.
func repoProject(t *testing.T) (dir string, env map[string]string) {
	t.Helper()
	dir, env = t.TempDir(), ruleEnv(t)
	copyTree(t, filepath.Join(moduleRoot(), "internal/core/api/testdata/tiers/clean"), dir)
	files := map[string]string{
		".gragas/pin.yaml": "rulebook: example\nsource: book\nparts: [go]\nformat: 1\n",
		"a.go":             "package p\n",
		"notes.md":         "Notes of the project.\n",
	}
	for name, body := range repoBook() {
		files["book/"+name] = body
	}
	put(t, dir, files)
	list := filepath.Join(filestest.Private(t), "list.txt")
	if err := os.WriteFile(list, []byte("zorbquux\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["GRAGAS_NAMES_FILE"], env["GRAGAS_BASE_REF"] = list, gateBase
	gitIn(t, dir, env, "init", "-q")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "branch", gateBase)
	return dir, env
}

// repoCases is, for each of the three checks, the kind --fail-on names and
// the one change past the base that the check finds: a name of the list in a
// tracked text, a symbolic link in the index, and a directive the merge base
// does not have.
func repoCases() []struct {
	kind  string
	plant func(t *testing.T, dir string, env map[string]string)
} {
	return []struct {
		kind  string
		plant func(t *testing.T, dir string, env map[string]string)
	}{
		{"names", func(t *testing.T, dir string, env map[string]string) {
			t.Helper()
			put(t, dir, map[string]string{"notes.md": "Notes of the project.\nAsk zorbquux about the rest.\n"})
			commitAll(t, dir, env)
		}},
		{"tracked-files", func(t *testing.T, dir string, env map[string]string) {
			t.Helper()
			// The link is an entry of the index alone, its text the blob of
			// notes.md's name: no link is made in the work tree, which not
			// every system lets a test make.
			put(t, dir, map[string]string{"target.txt": "notes.md"})
			id := strings.TrimSpace(gitIn(t, dir, env, "hash-object", "-w", "target.txt"))
			if err := os.Remove(filepath.Join(dir, "target.txt")); err != nil {
				t.Fatal(err)
			}
			gitIn(t, dir, env, "update-index", "--add", "--cacheinfo", "120000,"+id+",notes.lnk")
		}},
		{"counts", func(t *testing.T, dir string, env map[string]string) {
			t.Helper()
			put(t, dir, map[string]string{"a.go": "package p\n\n//nolint:errcheck\nvar x = 1\n"})
			commitAll(t, dir, env)
		}},
	}
}

// TestCheckBelowAPinListsTheThreeRepositoryChecks: the built binary, in a
// project whose pinned part binds the names, the tracked files and the counts,
// lists a finding of each under its kind, ends done with the findings listed,
// and ends 1 when --fail-on names the kind; --json holds the same findings as
// entries of the one findings list. A project with none of the three wrong
// ends done under each --fail-on, with no finding.
// Risk: a check wired to nothing, which passes a name, a link or a directive.
func TestCheckBelowAPinListsTheThreeRepositoryChecks(t *testing.T) {
	t.Parallel()
	clean, env := repoProject(t)
	for _, c := range repoCases() {
		if r := run(t, clean, env, "check", "--fail-on", c.kind); r.code != 0 || !strings.Contains(r.stdout, "0 findings") {
			t.Errorf("the control, --fail-on %s: exit %d, want done with no finding\n%s%s", c.kind, r.code, r.stdout, r.stderr)
		}
		dir, env := repoProject(t)
		c.plant(t, dir, env)
		if r := run(t, dir, env, "check"); r.code != 0 || !strings.Contains(r.stdout, c.kind) || !strings.Contains(r.stdout, "1 finding") {
			t.Errorf("%s with no --fail-on: exit %d, want done with the finding listed\n%s%s", c.kind, r.code, r.stdout, r.stderr)
		}
		r := run(t, dir, env, "check", "--fail-on", c.kind)
		if r.code != 1 || !strings.Contains(r.stdout, c.kind) || strings.Contains(r.stdout+r.stderr, "zorbquux") {
			t.Errorf("--fail-on %s: exit %d, want 1 with the finding listed and no name said\n%s%s", c.kind, r.code, r.stdout, r.stderr)
		}
		doc := run(t, dir, env, "check", "--json")
		if doc.code != 0 || !strings.Contains(doc.stdout, `"kind": "`+c.kind+`"`) || strings.Contains(doc.stdout+doc.stderr, "zorbquux") {
			t.Errorf("%s --json: exit %d, want a done document with a finding of the kind and no name\n%s%s", c.kind, doc.code, doc.stdout, doc.stderr)
		}
	}
}

// TestTheRepositoryCheckScreensAreGoldens: each of the three checks below a
// pin, with its finding under --fail-on at 80, 60 and 40 columns and without
// one at 80: twelve screens, each against a golden written only with -update
// by a person who reads the diff (cli.look.6). Risk: a finding whose place or
// rule wraps badly or drifts unseen, or a name printed by the check for names.
func TestTheRepositoryCheckScreensAreGoldens(t *testing.T) {
	t.Parallel()
	for _, c := range repoCases() {
		for _, s := range []struct {
			name, cols string
			found      bool
		}{{"finding-80", "80", true}, {"finding-60", "60", true}, {"finding-40", "40", true}, {"done-80", "80", false}} {
			t.Run(c.kind+"-"+s.name, func(t *testing.T) {
				t.Parallel()
				dir, env := repoProject(t)
				if s.found {
					c.plant(t, dir, env)
				}
				env["COLUMNS"] = s.cols
				args := []string{"check", "--fail-on", c.kind}
				r := run(t, dir, env, args...)
				got := fmt.Sprintf("$ gragas %s\n--- stdout\n%s--- stderr\n%s", strings.Join(args, " "), r.stdout, r.stderr)
				uitest.Golden(t, "project-"+c.kind+"-"+s.name, scrub(got, dir))
			})
		}
	}
}
