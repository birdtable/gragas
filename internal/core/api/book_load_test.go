package coreapi_test

import (
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const bookIdentity = `format: 3
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

const bookStructure = `format: 3
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

const bookRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
`

func bookWorld(t *testing.T, identity string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	return w, w.Seed("repo", map[string]string{"book/rulebook.yaml": identity, "book/structure.yaml": bookStructure, "book/language/go/rules/errors.yaml": bookRules})
}

func TestLoadBookReadsTheRulebookBelowASubfolder(t *testing.T) {
	t.Parallel()
	w, root := bookWorld(t, bookIdentity)
	r, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil || len(r.Findings) != 0 {
		t.Fatalf("LoadBook = %+v, %v", r.Findings, err)
	}
	if r.Dir != "book" || r.Book.Identity.Name != "example" || r.Book.Structure.Layers[0].Parts[0].Slug != "go" {
		t.Errorf("report = %+v", r)
	}
}

func TestLoadBookEndsNotReadyOnANewerFormatNamingBothVersions(t *testing.T) {
	t.Parallel()
	w, root := bookWorld(t, strings.Replace(bookIdentity, "format: 3", "format: 4", 1))
	_, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if !errors.Is(err, coreapi.ErrNotReady) {
		t.Fatalf("err = %v; want ErrNotReady", err)
	}
	for _, want := range []string{"format 4", "format 3", "book/rulebook.yaml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not name %q", err, want)
		}
	}
}

func TestLoadBookRefusesTwoRulebooks(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"a/rulebook.yaml": bookIdentity, "b/rulebook.yaml": bookIdentity})
	if _, err := coreapi.LoadBook(t.Context(), w.Ports(), root); !errors.Is(err, coreapi.ErrTwoBooks) {
		t.Fatalf("err = %v; want ErrTwoBooks", err)
	}
}

func TestLoadBookListsFindingsWithFileAndLine(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	root := w.Seed("repo", map[string]string{"rulebook.yaml": "format: 3\nname: a\nname: b\n", "structure.yaml": bookStructure, "language/go/rules/errors.yaml": bookRules})
	r, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil || len(r.Findings) != 1 || r.Findings[0].Entity != "rulebook.yaml:3" {
		t.Fatalf("LoadBook = %+v, %v", r.Findings, err)
	}
}

func TestLoadBookHandsOutOneResolutionThatKnowsAliases(t *testing.T) {
	t.Parallel()
	w, root := bookWorld(t, bookIdentity)
	r, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil || r.Resolve == nil {
		t.Fatalf("LoadBook = %v, %v", r.Resolve == nil, err)
	}
	if got, ok := r.Resolve("go.errors.1"); !ok || got.Statement != "Wrap an error." {
		t.Errorf("Resolve(go.errors.1) = %+v, %v", got, ok)
	}
	if _, ok := r.Resolve("go.errors.2"); ok {
		t.Error("an unknown id resolved")
	}
}

func TestLoadBookNamedReadsTheListFromOutsideTheRepository(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	rules := strings.Replace(bookRules, "Wrap an error.", "Ask alex before you wrap an error.", 1)
	root := w.Seed("repo", map[string]string{"rulebook.yaml": bookIdentity, "structure.yaml": bookStructure, "language/go/rules/errors.yaml": rules})
	w.SeedOutside("/work/scratch/names.txt", "# stand-in list\n\\balex\\b\n")
	dir := "/work/scratch"
	r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, dir+"/names.txt")
	if err != nil || len(r.Findings) != 1 || len(r.NamesFound()) != 1 || r.Findings[0].Entity != "language/go/rules/errors.yaml:6" || r.Findings[0].Fix == "" || len(r.Notices) != 0 || r.NamesOff != "" {
		t.Fatalf("report = %+v, %v", r, err)
	}
}

func TestLoadBookNamedWithoutAListIsANoticeAndABadPatternIsNotReady(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	root := w.Seed("repo", map[string]string{"rulebook.yaml": bookIdentity, "structure.yaml": bookStructure, "language/go/rules/errors.yaml": bookRules})
	w.SeedOutside("/work/scratch/bad.txt", "fine\n(unclosed\n")
	for _, file := range []string{"", "/work/scratch" + "/missing.txt"} {
		r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, file)
		if err != nil || len(r.Notices) != 1 || !strings.Contains(r.Notices[0].Msg, "names check did not run") {
			t.Errorf("%q: notices = %+v, %v", file, r.Notices, err)
		}
	}
	_, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, "/work/scratch"+"/bad.txt")
	if !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("err = %v; want ErrNotReady naming line 2", err)
	}
}

func TestAStaleTranslationIsANoticeInTheReportAndNeverAFinding(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/translations.schema.json")
	stale := "format: 3\ntranslations:\n  - entity: rule:go.errors.1\n    field: statement\n    language: fr\n    text: x\n    hash: sha256:" + strings.Repeat("0", 64) + "\n"
	root := w.Seed("repo", map[string]string{"rulebook.yaml": bookIdentity, "structure.yaml": bookStructure, "language/go/rules/errors.yaml": bookRules, "translations/fr/errors.yaml": stale})
	r, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil || len(r.Findings) != 0 {
		t.Fatalf("report = %+v, %v", r.Findings, err)
	}
	found := false
	for _, n := range r.Notices {
		found = found || (n.Entity == "translations/fr/errors.yaml:3" && strings.Contains(n.Msg, "is stale"))
	}
	if !found {
		t.Errorf("notices = %+v", r.Notices)
	}
}

// TestLoadBookOfAnOverlayNotesItsParentNotFetched: nothing fetches a parent
// yet, so an overlay's report holds the notice naming the pin and the checks
// skipped, and no finding: the closing word is the one its files make. A
// tightening of the book's own rule is still a finding. Planted failure:
// the contracts not checked by LoadBook.
func TestLoadBookOfAnOverlayNotesItsParentNotFetched(t *testing.T) {
	t.Parallel()
	overlay := strings.Replace(bookIdentity, "kind: base", "kind: overlay", 1) + "extends:\n  rulebook: base\n  version: 2.1.0\n"
	w, root := bookWorld(t, overlay)
	r, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil || len(r.Findings) != 0 {
		t.Fatalf("LoadBook = %+v, %v", r.Findings, err)
	}
	var parent []model.Notice
	for _, n := range r.Notices {
		if n.Entity == "book/rulebook.yaml" {
			parent = append(parent, n)
		}
	}
	if len(parent) != 1 || !strings.HasPrefix(parent[0].Msg, "parent not fetched: base 2.1.0, so the overlay contract against it was not checked") {
		t.Errorf("Notices = %+v; want one, the parent not fetched", r.Notices)
	}
	w2 := porttest.NewWorld(t)
	w2.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	root = w2.Seed("repo", map[string]string{"book/rulebook.yaml": overlay, "book/structure.yaml": bookStructure, "book/language/go/rules/errors.yaml": bookRules + "    bindings:\n      - tool: golangci-lint\n        target: config\n        key: wrapcheck\n        severity: error\n        rung: 1\n        run_point: push\n        tightens: go.errors.1\n"})
	r, err = coreapi.LoadBook(t.Context(), w2.Ports(), root)
	if err != nil || len(r.Findings) != 1 || !strings.Contains(r.Findings[0].Msg, "this rulebook's own rule") {
		t.Errorf("LoadBook of a tightening of its own rule = %+v, %v; want one finding", r.Findings, err)
	}
}
