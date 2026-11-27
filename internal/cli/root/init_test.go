package root

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/port"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// initProcess is a run of init over a working folder holding files, with the
// schemas compiled and a git identity configured.
func initProcess(t *testing.T, files map[string]string, args ...string) *journalProcess {
	t.Helper()
	j := newJournal(t, files, append([]string{"init"}, args...)...)
	j.sc.out.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/history.schema.json")
	j.sc.out.SeedRepo(j.root, porttest.Repo{Who: &model.Identity{Name: "A Person", Email: "a@example.test"}})
	j.p.History = func(string) (port.History, error) { return j.p.World.History, nil }
	return j
}

func (j *journalProcess) has(p string) bool {
	_, ok := j.sc.out.File(j.root, p)
	return ok
}

// TestInitOffATerminalWithoutKindIsUsage: nobody can answer the first
// question, so the run ends usage and names the flag that answers it.
func TestInitOffATerminalWithoutKindIsUsage(t *testing.T) {
	t.Parallel()
	j := initProcess(t, map[string]string{"a.txt": "x"})
	if code := j.run(); code != exit.Usage || j.sc.closing() != model.Usage {
		t.Fatalf("exit %d, word %q; want usage", code, j.sc.closing())
	}
	said := strings.Join(j.sc.out.Said, "\n") + sayOf(j)
	for _, want := range []string{"init needs to know what this folder is", "gragas init --kind rulebook"} {
		if !strings.Contains(said, want) {
			t.Errorf("the usage lacks %q:\n%s", want, said)
		}
	}
}

// TestInitOfARulebookOffATerminalPrintsThePlanAndDeclines: the dry run writes
// nothing, prints the guard lines and ends declined, exit 6.
func TestInitOfARulebookOffATerminalPrintsThePlanAndDeclines(t *testing.T) {
	t.Parallel()
	j := initProcess(t, map[string]string{"a.txt": "x"}, "--kind", "rulebook", "--name", "demo", "--owner", "team-a")
	if code := j.run(); code != exit.Declined || j.sc.closing() != model.Declined {
		t.Fatalf("exit %d, word %q; want declined", code, j.sc.closing())
	}
	if j.has("rulebook.yaml") || j.has("history.jsonl") {
		t.Fatal("the dry run wrote a file")
	}
	said := strings.Join(j.sc.out.Said, "\n") + sayOf(j)
	for _, want := range []string{"gragas guard staged || exit 2", "gragas guard edit", "pre-merge-commit  gragas guard staged --merge-hook || exit 2", ".gragas/state.yaml", "--apply"} {
		if !strings.Contains(said, want) {
			t.Errorf("the transcript lacks %q:\n%s", want, said)
		}
	}
}

// TestInitApplyWritesTheRulebook: identity, structure and the first history
// line are in the folder, and the run ends done.
func TestInitApplyWritesTheRulebook(t *testing.T) {
	t.Parallel()
	j := initProcess(t, map[string]string{"a.txt": "x"}, "--kind", "rulebook", "--name", "demo", "--owner", "team-a", "--apply")
	if code := j.run(); code != exit.Done {
		t.Fatalf("exit %d, word %q; want done", code, j.sc.closing())
	}
	for _, p := range []string{"rulebook.yaml", "structure.yaml", "history.jsonl"} {
		if !j.has(p) {
			t.Errorf("%s was not written", p)
		}
	}
	if h, _ := j.sc.out.File(j.root, "history.jsonl"); !strings.Contains(h, `"number":1`) || strings.Contains(h, "previous") {
		t.Errorf("history.jsonl = %q", h)
	}
}

// TestInitInARulebookOrAProjectIsNotReady: the folder is already set up.
func TestInitInARulebookOrAProjectIsNotReady(t *testing.T) {
	t.Parallel()
	for file, name := range map[string]string{"rulebook.yaml": "rulebook", ".gragas/pin.yaml": "pinned"} {
		j := initProcess(t, map[string]string{file: "x"}, "--kind", "rulebook", "--name", "demo", "--owner", "team-a")
		if code := j.run(); code != exit.NotReady || j.sc.closing() != model.NotReady {
			t.Errorf("%s: exit %d, word %q; want not ready", file, code, j.sc.closing())
		}
		if !strings.Contains(strings.Join(j.sc.out.Said, " ")+sayOf(j), name) {
			t.Errorf("%s: the ending does not say %q", file, name)
		}
	}
}

// TestInitOfAProjectIsRefusedForNow: the project form is M6's.
func TestInitOfAProjectIsRefusedForNow(t *testing.T) {
	t.Parallel()
	j := initProcess(t, map[string]string{"a.txt": "x"}, "--kind", "project")
	if code := j.run(); code != exit.Refused {
		t.Fatalf("exit %d, want refused", code)
	}
}

func sayOf(j *journalProcess) string {
	var b strings.Builder
	for _, s := range j.sc.out.Shown {
		b.WriteString(said(s))
	}
	return b.String()
}

// TestInitWithoutAGitInTheSettingsIsNotReady: the settings name no git, so
// nobody can say who made the rulebook; the sentence names the key, and nothing
// is written.
func TestInitWithoutAGitInTheSettingsIsNotReady(t *testing.T) {
	t.Parallel()
	j := initProcess(t, map[string]string{"a.txt": "x"}, "--kind", "rulebook", "--name", "demo", "--owner", "team-a", "--apply")
	j.p.History = func(string) (port.History, error) { return nil, ErrNoGit }
	if code := j.run(); code != exit.NotReady || j.has("rulebook.yaml") {
		t.Fatalf("exit %d; want not ready and no file", code)
	}
	if got := sayOf(j); !strings.Contains(got, "programs.git") {
		t.Errorf("the ending does not name programs.git: %q", got)
	}
}
