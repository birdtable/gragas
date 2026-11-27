package coreapi_test

import (
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// generatedWorld seeds the example book with the rules.json and ids.lock
// given, "" for none.
func generatedWorld(t *testing.T, name, rules, rulesJSON, lock string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	files := map[string]string{"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure, "book/language/go/rules/errors.yaml": rules}
	if rulesJSON != "" {
		files["book/rules.json"] = rulesJSON
	}
	if lock != "" {
		files["book/ids.lock"] = lock
	}
	return w, w.Seed(name, files)
}

// TestStaleGeneratedMakesTheFilesFromTheSourcesAndComparesThem: the files the
// sources make are current; a missing one, one that is not what they make and
// one left from before a rule was edited are each refused, and a refusal
// carries the bytes that replace the file.
func TestStaleGeneratedMakesTheFilesFromTheSourcesAndComparesThem(t *testing.T) {
	t.Parallel()
	w, root := generatedWorld(t, "none", bookRules, "", "")
	made, err := coreapi.StaleGenerated(t.Context(), w.Ports(), root)
	if err != nil || len(made) != 2 || made[0].Path != "book/rules.json" || made[1].Path != "book/ids.lock" {
		t.Fatalf("missing generated files: %+v, %v", made, err)
	}
	want, lock := string(made[0].Data), string(made[1].Data)
	for _, has := range []string{`"schema": 3`, `"id": "go.errors.1"`, `"statement": "Wrap an error."`, `"standing": "proposed"`} {
		if !strings.Contains(want, has) {
			t.Errorf("rules.json lacks %s:\n%s", has, want)
		}
	}
	if !strings.HasSuffix(lock, "\ngo.errors.1\n") {
		t.Errorf("ids.lock lacks the book's id:\n%s", lock)
	}
	for name, tc := range map[string]struct {
		rules, json, lock string
		stale             []string
	}{
		"current":                  {bookRules, want, lock, nil},
		"planted, not what it was": {bookRules, "{}\n", lock, []string{"book/rules.json"}},
		"left from before an edit": {strings.Replace(bookRules, "Wrap an error.", "Wrap every error.", 1), want, lock, []string{"book/rules.json"}},
		"a lock with no id":        {bookRules, want, "# nothing\n", []string{"book/ids.lock"}},
	} {
		w, root := generatedWorld(t, name, tc.rules, tc.json, tc.lock)
		got, err := coreapi.StaleGenerated(t.Context(), w.Ports(), root)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var paths []string
		for _, o := range got {
			paths = append(paths, o.Path)
		}
		if strings.Join(paths, " ") != strings.Join(tc.stale, " ") {
			t.Errorf("%s: stale %v, want %v", name, paths, tc.stale)
		}
	}
}

// TestTheIDsLockKeepsEveryIDEverIssued: an id the old lock holds stays when no
// rules file holds it any more, and the ids are sorted, each once.
func TestTheIDsLockKeepsEveryIDEverIssued(t *testing.T) {
	t.Parallel()
	w, root := generatedWorld(t, "kept", bookRules, "", "# old\ngo.errors.9\ngo.errors.1\n")
	made, err := coreapi.StaleGenerated(t.Context(), w.Ports(), root)
	if err != nil || len(made) != 2 {
		t.Fatalf("%+v, %v", made, err)
	}
	if got := string(made[1].Data); !strings.HasSuffix(got, "\ngo.errors.1\ngo.errors.9\n") || strings.Count(got, "go.errors.1\n") != 1 {
		t.Errorf("ids.lock:\n%s", got)
	}
}

// TestStaleGeneratedRefusesABookWithFindings: nothing is made from a book that
// does not load clean.
func TestStaleGeneratedRefusesABookWithFindings(t *testing.T) {
	t.Parallel()
	w, root := generatedWorld(t, "bad", bookRules+strings.TrimPrefix(bookRules, "format: 3\npart: go\nconcern: errors\nrules:\n"), "", "")
	if got, err := coreapi.StaleGenerated(t.Context(), w.Ports(), root); err == nil {
		t.Errorf("a book with a finding gave %+v and no error", got)
	}
}
