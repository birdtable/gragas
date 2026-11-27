package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// These tests read a rulebook from a real folder with the real Tree and the
// schemas main embeds, which no package under internal/core may import: each
// kind of finding the loader promises has a planted file here, and a clean
// control (tests.generated.5's listing).

const realIdentity = `format: 3
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

const realStructure = `format: 3
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

const realRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error with what was being done.
    strength: must
    status: active
    review_by: "2027-01-31"
    origin: authored
`

// planted maps the rule file's text to the finding it must end with.
func TestEachFindingKindHasAPlantedFileThatEndsWithFileAndLine(t *testing.T) {
	t.Parallel()
	const rel = "language/go/rules/errors.yaml"
	cases := []struct {
		name, text, want string
	}{
		{"an unknown field", strings.Replace(realRules, "    origin: authored\n", "    origin: authored\n    colour: red\n", 1), rel + ":5:"},
		{"a missing required field", strings.Replace(realRules, "    statement: Wrap an error with what was being done.\n", "", 1), rel + ":5:"},
		{"a wrong enum", strings.Replace(realRules, "strength: must", "strength: shall", 1), rel + ":7:"},
		{"a key written twice", strings.Replace(realRules, "    origin: authored\n", "    origin: authored\n    origin: migrated\n", 1), rel + ":11:"},
		{"a bad date", strings.Replace(realRules, `"2027-01-31"`, `"2027-1-31"`, 1), rel + ":9:"},
		{"a date left unquoted", strings.Replace(realRules, `"2027-01-31"`, `2027-01-31`, 1), rel + ":9:"},
	}
	// The subtests run in parallel, after this function returns: the count is
	// read when they have all ended.
	var ran atomic.Int32
	t.Cleanup(func() {
		if n := int(ran.Load()); n != len(cases) {
			t.Errorf("ran %d of %d planted files", n, len(cases))
		}
	})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := loadFrom(t, c.text)
			if len(r.Findings) == 0 || !strings.HasPrefix(r.Findings[0].Entity+":", c.want) {
				t.Fatalf("findings = %+v; want the first at %s", r.Findings, c.want)
			}
			ran.Add(1)
		})
	}
}

func TestACleanControlLoadsItsRule(t *testing.T) {
	t.Parallel()
	r := loadFrom(t, realRules)
	if len(r.Findings) != 0 || len(r.Book.Rules) != 1 || r.Book.Rules[0].ID != "go.errors.1" {
		t.Fatalf("report = %+v", r)
	}
}

// opened is the root of dir in p's tree. The tree lets go of it when the test
// ends, before the folder is removed: Windows refuses to remove one held open.
func opened(t *testing.T, p port.Ports, dir string) model.Root {
	t.Helper()
	t.Cleanup(func() {
		if err := p.Tree.(*hostfs.Tree).Close(); err != nil {
			t.Errorf("let go of the folders: %v", err)
		}
	})
	root, err := p.Tree.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func loadFrom(t *testing.T, rules string) coreapi.BookReport {
	t.Helper()
	dir := t.TempDir()
	for p, c := range map[string]string{
		"rulebook.yaml": realIdentity, "structure.yaml": realStructure, "language/go/rules/errors.yaml": rules,
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := world(nil, config.Config{})
	root := opened(t, p, dir)
	r, err := coreapi.LoadBook(t.Context(), p, root)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAGuideBlockIsCheckedAgainstTheRealBlockSchema(t *testing.T) {
	t.Parallel()
	block := func(extra string) string {
		return "## Errors\n\n```gragas\nanchor: errors\nrules:\n  - id: go.errors.2\n    statement: Name the failed step.\n    strength: should\n    status: proposed\n    origin: authored\n" + extra + "```\n"
	}
	cases := []struct{ name, guide, want string }{
		{"clean", block(""), ""},
		{"unknown field in a rule", block("    colour: red\n"), "language/go/guide.md:"},
		{"no rules", "```gragas\nanchor: errors\nrules: []\n```\n", "language/go/guide.md:"},
		{"format in a block", "```gragas\nformat: 3\nanchor: errors\nrules: []\n```\n", "language/go/guide.md:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for p, text := range map[string]string{
				"rulebook.yaml": realIdentity, "structure.yaml": realStructure,
				"language/go/rules/errors.yaml": realRules, "language/go/guide.md": c.guide,
			} {
				full := filepath.Join(dir, filepath.FromSlash(p))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p := world(nil, config.Config{})
			root := opened(t, p, dir)
			r, err := coreapi.LoadBook(t.Context(), p, root)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case c.want == "" && (len(r.Findings) != 0 || len(r.Book.Rules) != 2):
				t.Fatalf("clean guide: %+v, %d rules", r.Findings, len(r.Book.Rules))
			case c.want != "" && (len(r.Findings) == 0 || !strings.HasPrefix(r.Findings[0].Entity, c.want)):
				t.Fatalf("findings = %+v; want the first in %s", r.Findings, c.want)
			}
		})
	}
}

func TestAnIDWithABadShapeIsAFindingFromTheRealSchema(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"Go.errors.1", "go.errors.0", "go.errors", "go.errors.01x"} {
		text := strings.Replace(realRules, "id: go.errors.1", "id: "+id, 1)
		r := loadFrom(t, text)
		if len(r.Findings) == 0 || !strings.HasPrefix(r.Findings[0].Entity, "language/go/rules/errors.yaml:") {
			t.Errorf("%s: findings = %+v", id, r.Findings)
		}
	}
}

func TestTheNamesListIsReadFromAFolderOutsideTheRepositoryByTheRealTree(t *testing.T) {
	t.Parallel()
	repo, private := t.TempDir(), t.TempDir()
	write := func(dir, name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(repo, "rulebook.yaml", realIdentity)
	write(repo, "structure.yaml", realStructure)
	write(repo, "language/go/rules/errors.yaml", strings.Replace(realRules, "statement: ", "statement: Ask alex, then ", 1))
	write(private, "names.txt", "\\balex\\b\n")
	p := world(nil, config.Config{})
	root := opened(t, p, repo)
	r, err := coreapi.LoadBookNamed(t.Context(), p, root, filepath.Join(private, "names.txt"))
	if err != nil || len(r.Findings) != 1 || len(r.NamesFound()) != 1 || r.Findings[0].Line == 0 || r.NamesOff != "" {
		t.Fatalf("report = %+v, %v", r.Findings, err)
	}
}

func TestATranslationWithAWrongHashFormIsAFindingWithItsLineFromTheRealSchema(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for p, text := range map[string]string{
		"rulebook.yaml": realIdentity, "structure.yaml": realStructure, "language/go/rules/errors.yaml": realRules,
		"translations/fr/errors.yaml": "format: 3\ntranslations:\n  - entity: rule:go.errors.1\n    field: statement\n    language: fr\n    text: x\n    hash: SHA256:ABC\n",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := world(nil, config.Config{})
	root := opened(t, p, dir)
	r, err := coreapi.LoadBook(t.Context(), p, root)
	if err != nil || len(r.Findings) == 0 || !strings.HasPrefix(r.Findings[0].Entity, "translations/fr/errors.yaml:") {
		t.Fatalf("report = %+v, %v", r.Findings, err)
	}
}
