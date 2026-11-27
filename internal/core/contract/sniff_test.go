package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/contract"
)

// TestSniffKindTellsEachGoldenFromItsOneField: every document of the fixture is
// the kind its file is named for, and a document of no kind, of two kinds or
// that is not an object is none. Risk: a kind guessed wrong sends a document
// to the wrong schema.
func TestSniffKindTellsEachGoldenFromItsOneField(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]contract.Kind{
		"bundle.json": contract.Bundle, "report-clean.json": contract.Report, "report-weakened.json": contract.Report,
		"changelog.json": contract.Changelog, "sarif.json": contract.SARIF, "plan.json": contract.Plan, "proposal.json": contract.Proposal,
		"grid.json": contract.Grid,
	} {
		doc, err := os.ReadFile(filepath.Join("..", "..", "..", "contract", "fixture", name))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := contract.SniffKind(doc); !ok || got != want {
			t.Errorf("%s: kind %q (%v), want %q", name, got, ok, want)
		}
	}
	for _, doc := range []string{`{}`, `{"rules":[],"runs":[]}`, `[]`, `not json`, ``, `{"rules":` + strings.Repeat("[", 3)} {
		if got, ok := contract.SniffKind([]byte(doc)); ok {
			t.Errorf("%q was read as %q", doc, got)
		}
	}
}

// TestParseKindKnowsTheSixAndNothingElse.
func TestParseKindKnowsTheSixAndNothingElse(t *testing.T) {
	t.Parallel()
	for _, k := range contract.Kinds() {
		if got, ok := contract.ParseKind(string(k)); !ok || got != k {
			t.Errorf("%s: %q %v", k, got, ok)
		}
	}
	for _, name := range []string{"", "Bundle", "rules", "bundle "} {
		if _, ok := contract.ParseKind(name); ok {
			t.Errorf("%q is no kind", name)
		}
	}
}
