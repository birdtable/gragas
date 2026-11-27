package root

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// whoOf is the who of the one line of a history file, or fails the test.
func whoOf(t *testing.T, history string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(history), "\n")
	if len(lines) != 1 {
		t.Fatalf("history.jsonl holds %d lines, want 1:\n%s", len(lines), history)
	}
	_, after, ok := strings.Cut(lines[0], `"who":"`)
	if !ok {
		t.Fatalf("the line names no who: %s", lines[0])
	}
	who, _, _ := strings.Cut(after, `"`)
	return who
}

// TestInitRecordsItsOwnCommandPathAsWhoAndAsksGitForNoOne:
// the line init writes names the command's path, with a repository that can
// name nobody. Planted break: init handing in no who (the run ends on a
// refusal), its leaf name (init) or git's identity.
func TestInitRecordsItsOwnCommandPathAsWhoAndAsksGitForNoOne(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{"a.txt": "x"}, "init", "--kind", "rulebook", "--name", "demo", "--owner", "team-a", "--apply")
	j.sc.out.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/history.schema.json")
	j.sc.out.SeedRepo(j.root, porttest.Repo{})
	j.p.History = func(string) (port.History, error) { return j.p.World.History, nil }
	if code := j.run(); code != exit.Done {
		t.Fatalf("exit %d, word %q, said %q; want done", code, j.sc.closing(), sayOf(j))
	}
	h, _ := j.sc.out.File(j.root, "history.jsonl")
	if who := whoOf(t, h); who != "gragas init" {
		t.Errorf("init records %q as who, want %q", who, "gragas init")
	}
}

// TestMigrateRecordsItsOwnCommandPathAsWhoAndAsksGitForNoOne: the same for
// migrate, over a v2 folder. Planted break: migrate reading History.Who again
// (the repository names nobody, so the run is refused), handing in no who
// (the run ends on a refusal), or its leaf name.
func TestMigrateRecordsItsOwnCommandPathAsWhoAndAsksGitForNoOne(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{
		"parts.yaml":                  "layers:\n  - key: base\n    name: Base\nparts:\n  - short: arch\n    layer: base\n    name: Architecture\n    owner: operator\n",
		"sources.yaml":                "sources:\n  one-book:\n    kind: book\n    title: A Book\n    locator: isbn:9780321293534\n",
		"products.yaml":               "kinds:\n  - key: cli\n    name: Command-line program\n    description: A program run from a terminal.\n",
		"base/arch/rules/layers.yaml": "part: arch\nconcern: layers\norder: 1\ntitle: Layers\nrules:\n  - id: arch.layers.1\n    rule: Keep packages free of cycles.\n    strength: Hard\n    status: active\n    sources:\n      - ref: one-book\n        where: chapter 2\n        backs: rule\n",
	}, "migrate", "--name", "demo", "--owner", "team-a", "--apply")
	j.sc.out.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/rule.schema.json", "v3/block.schema.json", "v3/sources.schema.json", "v3/decisions.schema.json", "v3/history.schema.json", "v3/common.schema.json")
	j.sc.out.SeedRepo(j.root, porttest.Repo{})
	j.p.History = func(string) (port.History, error) { return j.p.World.History, nil }
	if code := j.run(); code != exit.Done {
		t.Fatalf("exit %d, word %q, said %q; want done", code, j.sc.closing(), sayOf(j)+strings.Join(j.sc.out.Said, "\n"))
	}
	h, _ := j.sc.out.File(j.root, "history.jsonl")
	if who := whoOf(t, h); who != "gragas migrate" {
		t.Errorf("migrate records %q as who, want %q", who, "gragas migrate")
	}
}

// TestARuleWordRecordsItsOwnCommandPathAsWhoAndAsksGitForNoOne: the rule
// family hands in its command's path, rule promote, and not the leaf's name,
// and the plan reads no identity. Planted break: the leaf's name (promote), no
// who, or git's identity.
func TestARuleWordRecordsItsOwnCommandPathAsWhoAndAsksGitForNoOne(t *testing.T) {
	t.Parallel()
	j := newJournal(t, map[string]string{
		"rulebook.yaml":                 "format: 3\nname: example\nversion: 1.0.0\nkind: base\nlicences:\n  rules: CC-BY-4.0\nlanguage: en\nowners: [team-a]\ngovernance:\n  contribution: open\n  decision_required: false\n  trial_releases: 1\n  review_days: 30\n",
		"structure.yaml":                "format: 3\nlayers:\n  - slug: language\n    name: Language\n    parts:\n      - slug: go\n        name: Go\n        concerns:\n          - slug: errors\n            name: Errors\nkinds: []\n",
		"language/go/rules/errors.yaml": "format: 3\npart: go\nconcern: errors\nrules:\n  - id: go.errors.2\n    statement: Log an error once.\n    strength: may\n    status: proposed\n    origin: authored\n    sources:\n      - ref: effective-go\n        backs: rule\n",
		"sources.yaml":                  "format: 3\nsources:\n  - slug: effective-go\n    kind: guide\n    title: Effective Go\n    locator: https://go.dev/doc/effective_go\n",
	}, "rule", "promote", "go.errors.2", "--decision-title", "Decide it", "--decision-text", "Because.", "--apply")
	j.sc.out.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/rule.schema.json", "v3/block.schema.json", "v3/sources.schema.json", "v3/decisions.schema.json", "v3/history.schema.json", "v3/common.schema.json")
	j.sc.out.SeedRepo(j.root, porttest.Repo{})
	j.p.History = func(string) (port.History, error) { return j.p.World.History, nil }
	if code := j.run(); code != exit.Done {
		t.Fatalf("exit %d, word %q, said %q; want done", code, j.sc.closing(), sayOf(j)+strings.Join(j.sc.out.Said, "\n"))
	}
	h, _ := j.sc.out.File(j.root, "history.jsonl")
	if who := whoOf(t, h); who != "gragas rule promote" {
		t.Errorf("rule promote records %q as who, want %q", who, "gragas rule promote")
	}
}
