package coreapi_test

import (
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// generatedWorld seeds the example book with the rules.json given, "" for none.
func generatedWorld(t *testing.T, name, rules, rulesJSON string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	files := map[string]string{"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure, "book/language/go/rules/errors.yaml": rules}
	if rulesJSON != "" {
		files["book/rules.json"] = rulesJSON
	}
	return w, w.Seed(name, files)
}

// TestStaleRulesJSONMakesTheFileFromTheSourcesAndComparesIt: the file the
// sources make is current; one that is missing, one that is not what they make
// and one left from before a rule was edited are each refused, and a refusal
// carries the bytes that replace the file (gragas#2549).
func TestStaleRulesJSONMakesTheFileFromTheSourcesAndComparesIt(t *testing.T) {
	t.Parallel()
	w, root := generatedWorld(t, "none", bookRules, "")
	made, err := coreapi.StaleRulesJSON(t.Context(), w.Ports(), root)
	if err != nil || len(made) != 1 || made[0].Path != "book/rules.json" {
		t.Fatalf("a missing rules.json: %+v, %v", made, err)
	}
	want := string(made[0].Data)
	for _, has := range []string{`"schema": 3`, `"id": "go.errors.1"`, `"statement": "Wrap an error."`, `"standing": "proposed"`} {
		if !strings.Contains(want, has) {
			t.Errorf("rules.json lacks %s:\n%s", has, want)
		}
	}
	for name, tc := range map[string]struct {
		rules, json string
		stale       bool
	}{
		"current":                  {bookRules, want, false},
		"planted, not what it was": {bookRules, "{}\n", true},
		"left from before an edit": {strings.Replace(bookRules, "Wrap an error.", "Wrap every error.", 1), want, true},
	} {
		w, root := generatedWorld(t, name, tc.rules, tc.json)
		got, err := coreapi.StaleRulesJSON(t.Context(), w.Ports(), root)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if (len(got) == 1) != tc.stale || len(got) > 1 {
			t.Errorf("%s: stale = %d files, want stale %v", name, len(got), tc.stale)
		}
	}
}

// TestStaleRulesJSONRefusesABookWithFindings: nothing is made from a book that
// does not load clean.
func TestStaleRulesJSONRefusesABookWithFindings(t *testing.T) {
	t.Parallel()
	w, root := generatedWorld(t, "bad", bookRules+strings.TrimPrefix(bookRules, "format: 3\npart: go\nconcern: errors\nrules:\n"), "")
	if got, err := coreapi.StaleRulesJSON(t.Context(), w.Ports(), root); err == nil {
		t.Errorf("a book with a finding gave %+v and no error", got)
	}
}
