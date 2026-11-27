package coreapi_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// conflictRules is a rules file of four trial rules on lint-a: go.errors.1 and
// .2 set the entry wrap to different severities, and .3 and .4 hold the entry
// cause at the same value.
const conflictRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: trial
    origin: authored
    bindings:
      - {tool: lint-a, target: source, key: wrap, severity: error, rung: 1, run_point: edit}
  - id: go.errors.2
    statement: Wrap it quietly.
    strength: should
    status: trial
    origin: authored
    bindings:
      - {tool: lint-a, target: source, key: wrap, severity: warning, rung: 1, run_point: edit}
  - id: go.errors.3
    statement: Name the cause.
    strength: should
    status: trial
    origin: authored
    bindings:
      - {tool: lint-a, target: config, key: cause, severity: warning, rung: 1, run_point: commit}
  - id: go.errors.4
    statement: Say why.
    strength: may
    status: trial
    origin: authored
    bindings:
      - {tool: lint-a, target: config, key: cause, severity: warning, rung: 1, run_point: commit}
`

func conflictBook() map[string]string {
	files := goodBook()
	files["language/go/rules/errors.yaml"] = conflictRules
	return files
}

// TestListConflictsListsTheEntryTwoRulesSetDifferentlyAndNotTheOneTheyShare:
// wrap differs in severity between go.errors.1 and .2; cause is held by two
// rules at the same value and is no conflict. The word is findings while one is
// open. The plant is the same book with the second rule's severity made the
// first's, which leaves nothing and ends done.
func TestListConflictsListsTheEntryTwoRulesSetDifferentlyAndNotTheOneTheyShare(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, conflictBook())
	l, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
	if err != nil || len(l.Conflicts) != 1 {
		t.Fatalf("ListConflicts = %+v, %v; want the one conflict", l, err)
	}
	c := l.Conflicts[0]
	if c.Entry.Key != "wrap" || !slices.Equal(c.Differs, []string{model.DiffSeverity}) || !c.Open() || l.Word() != model.Findings {
		t.Errorf("conflict = %+v, word %s; want wrap, differing in severity, open, findings", c, l.Word())
	}
	d := coreapi.ConflictsDocument(l)
	if d.Command != "conflicts" || d.Word != "findings" || d.Open != 1 || d.Resolved != 0 || len(d.Resolutions) != 3 || d.Contract != model.ContractVersion {
		t.Errorf("document = %+v", d)
	}
	files := conflictBook()
	files["language/go/rules/errors.yaml"] = strings.Replace(conflictRules, "severity: warning, rung: 1, run_point: edit", "severity: error, rung: 1, run_point: edit", 1)
	w, root = listWorld(t, files)
	l, err = coreapi.ListConflicts(context.Background(), w.Ports(), root)
	if err != nil || len(l.Conflicts) != 0 || l.Word() != model.Done {
		t.Fatalf("the plant: %+v, %v; want no conflict and done", l, err)
	}
	if d := coreapi.ConflictsDocument(l); d.Word != "done" || d.Conflicts == nil || len(d.Resolutions) != 0 || d.Resolutions == nil {
		t.Errorf("the empty document = %+v; want done, empty lists and no resolution said", d)
	}
}

// TestConflictsSARIFHasAResultForEachOpenConflictAndNoneForAResolvedOne.
func TestConflictsSARIFHasAResultForEachOpenConflictAndNoneForAResolvedOne(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, conflictBook())
	l, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := coreapi.ConflictsSARIF(l, "v1.2.3")
	if err != nil || strings.Count(string(doc), `"ruleId"`) != 1 || !strings.Contains(string(doc), model.ConflictsCheck) ||
		!strings.Contains(string(doc), "go.errors.1 and go.errors.2") {
		t.Fatalf("SARIF = %s, %v; want one result naming both rules", doc, err)
	}
	l.Conflicts[0].Winner = "go.errors.1"
	doc, err = coreapi.ConflictsSARIF(l, "v1.2.3")
	if err != nil || strings.Count(string(doc), `"ruleId"`) != 0 {
		t.Errorf("a resolved conflict SARIF = %s, %v; want no result", doc, err)
	}
}

// TestListConflictsRefusesAFolderWithNoRulebookAndOneWithFindings: as
// ListTools does.
func TestListConflictsRefusesAFolderWithNoRulebookAndOneWithFindings(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, map[string]string{"README.md": "x"})
	if _, err := coreapi.ListConflicts(context.Background(), w.Ports(), root); !errors.Is(err, coreapi.ErrNoBook) {
		t.Errorf("no rulebook: %v; want ErrNoBook", err)
	}
	files := conflictBook()
	files["rulebook.yaml"] = "format: 3\nname: a\nname: b\n"
	w, root = listWorld(t, files)
	var f *coreapi.FindingsError
	if _, err := coreapi.ListConflicts(context.Background(), w.Ports(), root); !errors.As(err, &f) || f.Count < 1 {
		t.Errorf("a finding: %v; want a FindingsError", err)
	}
}
