package coreapi_test

import (
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// checkersWorld is a rulebook below the folder book/ with the files given
// beside its identity, structure and rules.
func checkersWorld(t *testing.T, identity string, files map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/checker.schema.json")
	all := map[string]string{"book/rulebook.yaml": identity, "book/structure.yaml": bookStructure, "book/language/go/rules/errors.yaml": bookRules}
	for k, v := range files {
		all["book/"+k] = v
	}
	return w, w.Seed("repo", all)
}

// TestAnImportsCheckerCodeIsDeclaredByTheImporterOrAFinding (S3): with
// the import handed in, as a fetch will hand it, an artifact of the
// import the importer does not declare by reference is a finding, at the
// rulebook's checkers folder; the clean control, which declares every one,
// has none and lists the declaration; and with nothing fetched, as on a live
// run today, the import's half is not checked.
// Risk: an import that brings checker code silently.
func TestAnImportsCheckerCodeIsDeclaredByTheImporterOrAFinding(t *testing.T) {
	t.Parallel()
	identity := bookIdentity + "imports:\n  - rulebook: shared-go\n    version: 2.0.0\n    parts: [ui]" + "\n"
	w, root := checkersWorld(t, identity, map[string]string{
		"checkers/shared-analyzer/checker.yaml": "format: 3\nname: shared-analyzer\ntool: lint-a\npublisher: sam\nfrom: shared-go\n",
	})
	imported := func(code ...model.CheckerArtifact) format.Composed {
		return format.Composed{Imports: []format.Foreign{{Rulebook: "shared-go", Version: "2.0.0", Parts: []string{"ui"}, CheckerCode: code}}}
	}
	analyzer := model.CheckerArtifact{Name: "shared-analyzer", Publisher: "sam", Tool: "lint-a"}
	r, err := coreapi.LoadBookComposed(t.Context(), w.Ports(), root, imported(analyzer))
	if err != nil || len(r.Findings) != 0 || len(r.Checkers) != 1 || r.Checkers[0].From != "shared-go" {
		t.Fatalf("the clean control: %+v, %+v, %v", r.Findings, r.Checkers, err)
	}
	r, err = coreapi.LoadBookComposed(t.Context(), w.Ports(), root, imported(analyzer, model.CheckerArtifact{Name: "shared-policy", Publisher: "sam", Tool: "lint-b"}))
	if err != nil || len(r.Findings) != 1 || r.Findings[0].File != "book/checkers" ||
		!strings.Contains(r.Findings[0].Msg, "the import shared-go brings the checker code shared-policy by sam, and this rulebook does not declare it") {
		t.Errorf("an import's undeclared artifact: %+v, %v", r.Findings, err)
	}
	if r, err := coreapi.LoadBook(t.Context(), w.Ports(), root); err != nil || len(r.Findings) != 0 {
		t.Errorf("with nothing fetched: %+v, %v", r.Findings, err)
	}
}

// TestAnOverlayDeclaringItsParentsArtifactAsItsOwnIsAFinding (S23): an overlay's own declaration by the name of an artifact the
// handed-in parent carries is a finding; the same overlay with the parent
// carrying another artifact has none.
// Risk: an overlay that republishes its parent's checker code as its own.
func TestAnOverlayDeclaringItsParentsArtifactAsItsOwnIsAFinding(t *testing.T) {
	t.Parallel()
	overlay := strings.Replace(bookIdentity, "kind: base", "kind: overlay", 1) + "extends:\n  rulebook: base\n  version: 2.1.0\n"
	w, root := checkersWorld(t, overlay, map[string]string{
		"checkers/wrapcheck-plugin/checker.yaml":   "format: 3\nname: wrapcheck-plugin\ntool: lint-a\npublisher: team-a\ntests: testdata\n",
		"checkers/wrapcheck-plugin/testdata/a.txt": "a case\n",
	})
	parent := func(name string) format.Composed {
		return format.Composed{Parent: &format.Foreign{Rulebook: "base", Version: "2.1.0", Parts: []string{"go"},
			CheckerCode: []model.CheckerArtifact{{Name: name, Publisher: "pat", Tool: "lint-a"}}}}
	}
	r, err := coreapi.LoadBookComposed(t.Context(), w.Ports(), root, parent("wrapcheck-plugin"))
	var mine []model.Finding
	for _, f := range r.Findings {
		if strings.HasPrefix(f.File, "book/checkers/") {
			mine = append(mine, f)
		}
	}
	if err != nil || len(mine) != 1 || mine[0].File != "book/checkers/wrapcheck-plugin/checker.yaml" ||
		!strings.Contains(mine[0].Msg, "wrapcheck-plugin is the parent base's checker code") {
		t.Errorf("an overlay declaring its parent's artifact: %+v, %v", r.Findings, err)
	}
	if r, err := coreapi.LoadBookComposed(t.Context(), w.Ports(), root, parent("other-plugin")); err != nil || len(r.Checkers) != 1 {
		t.Fatalf("the control: %+v, %v", r, err)
	} else {
		for _, f := range r.Findings {
			if strings.HasPrefix(f.File, "book/checkers/") {
				t.Errorf("the control has a checker finding: %+v", f)
			}
		}
	}
}
