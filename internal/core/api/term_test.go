package coreapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const termsFile = "format: 3\nterms:\n  - slug: wrap\n    definition: Add what was being done to an error.\n  - slug: spare\n    definition: Nothing uses it.\n    literal: spare-part\n"

const termRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error with {wrap}.
    strength: must
    status: proposed
    origin: authored
`

func termWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(append(allSchemas(), "v3/terms.schema.json")...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": termRules, "book/terms.yaml": termsFile,
	})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

// TestTermNewWritesTheTermAndOneHistoryLineAndASecondRunHasNothingToDo: the
// dry run writes nothing, the apply writes terms.yaml and one line of history
// that names the term and its literal, and the same run again is done with
// nothing to do (cli.commands.2).
// Planted failure: the dry run writes the term, or the second run appends a
// history line.
func TestTermNewWritesTheTermAndOneHistoryLineAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	w, root := termWorld(t)
	req := model.NewTerm{Slug: "header", Definition: "The name a request carries.", Literal: "X-Request-Id"}
	c, ref, err := coreapi.TermNew(context.Background(), w.Ports(), changeAsk(root, "gragas term new"), req)
	if err != nil || ref != nil {
		t.Fatalf("ref = %v, err = %v", ref, err)
	}
	shown, ok := c.Preview(context.Background())
	if !ok || shown.Plan.Digest == "" {
		t.Fatalf("preview ok=%v word=%q", ok, shown.Word)
	}
	if strings.Contains(read(t, w, root, "book/terms.yaml"), "header") {
		t.Fatal("the dry run wrote the term")
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	terms := read(t, w, root, "book/terms.yaml")
	if !strings.HasSuffix(terms, "  - slug: header\n    definition: The name a request carries.\n    literal: X-Request-Id\n") || !strings.HasPrefix(terms, termsFile) {
		t.Errorf("terms file:\n%s", terms)
	}
	hist := read(t, w, root, "book/history.jsonl")
	if strings.Count(hist, "\n") != 1 || !strings.Contains(hist, `"entity":"term:header"`) || !strings.Contains(hist, `"literal":"X-Request-Id"`) || !strings.Contains(hist, `"kind":"new"`) {
		t.Errorf("history:\n%s", hist)
	}
	again, ref, err := coreapi.TermNew(context.Background(), w.Ports(), runTwo(changeAsk(root, "gragas term new")), req)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if second := again.Apply(context.Background(), ""); second.Word != model.Done || !second.Nothing {
		t.Errorf("a second identical run ended %q nothing=%v (%v); want done, nothing to do", second.Word, second.Nothing, second.Cause)
	}
	if read(t, w, root, "book/history.jsonl") != hist {
		t.Error("the second run wrote history")
	}
}

func TestTermEditSetsTheLiteralFromWhatTheFileHoldsAndRecordsBothValues(t *testing.T) {
	t.Parallel()
	w, root := termWorld(t)
	next := "spare-tyre"
	c, ref, err := coreapi.TermEdit(context.Background(), w.Ports(), changeAsk(root, "gragas term edit spare"), model.EditTerm{Slug: "spare", Literal: &next})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if got := read(t, w, root, "book/terms.yaml"); got != strings.Replace(termsFile, "spare-part", "spare-tyre", 1) {
		t.Errorf("terms file:\n%s", got)
	}
	hist := read(t, w, root, "book/history.jsonl")
	if !strings.Contains(hist, `"before":{"literal":"spare-part","status":"active"}`) || !strings.Contains(hist, `"after":{"literal":"spare-tyre","status":"active"}`) {
		t.Errorf("history:\n%s", hist)
	}
}

// TestTermRetireOfATermInUseIsRefusedNamingTheRulesAndAnUnusedOneIsRetired:
// the refusal is a refusal with the rules in it and writes nothing; the
// unused term stays in the file with its status, and the history says so.
// Planted failure: the used term is retired, or the retired term is removed
// from the file.
func TestTermRetireOfATermInUseIsRefusedNamingTheRulesAndAnUnusedOneIsRetired(t *testing.T) {
	t.Parallel()
	w, root := termWorld(t)
	_, ref, err := coreapi.TermRetire(context.Background(), w.Ports(), changeAsk(root, "gragas term retire wrap"), model.RetireTerm{Slug: "wrap", Why: retireWhy()})
	if err != nil || ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), "go.errors.1") || !strings.Contains(ref.Hint(), "gragas rule replace") {
		t.Fatalf("ref = %+v, err = %v", ref, err)
	}
	if read(t, w, root, "book/terms.yaml") != termsFile || read(t, w, root, "book/history.jsonl") != "" {
		t.Fatal("a refused retirement wrote")
	}
	c, ref, err := coreapi.TermRetire(context.Background(), w.Ports(), changeAsk(root, "gragas term retire spare"), model.RetireTerm{Slug: "spare", Why: retireWhy()})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if got := read(t, w, root, "book/terms.yaml"); got != termsFile+"    status: retired\n" {
		t.Errorf("terms file:\n%s", got)
	}
	hist := read(t, w, root, "book/history.jsonl")
	if !strings.Contains(hist, `"kind":"retire"`) || !strings.Contains(hist, `"after":{"literal":"spare-part","status":"retired"}`) {
		t.Errorf("history:\n%s", hist)
	}
}

func retireWhy() model.Why {
	return model.Why{New: &model.NewDecision{Title: "Nothing uses it", Text: "No rule says it any more."}}
}

// TestTermRetireStandsOnADecisionTheHistoryNamesAndRefusesWithout: the
// decision is written to decisions.yaml and named by the history line; a
// retirement with none is a refusal that writes nothing.
// Planted failure: the retirement is planned with no decision.
func TestTermRetireStandsOnADecisionTheHistoryNamesAndRefusesWithout(t *testing.T) {
	t.Parallel()
	w, root := termWorld(t)
	_, ref, err := coreapi.TermRetire(context.Background(), w.Ports(), changeAsk(root, "gragas term retire spare"), model.RetireTerm{Slug: "spare"})
	if err != nil || ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), "stands on a decision") || !strings.Contains(ref.Hint(), "--decision-title") {
		t.Fatalf("ref = %+v, err = %v", ref, err)
	}
	if read(t, w, root, "book/terms.yaml") != termsFile || read(t, w, root, "book/history.jsonl") != "" {
		t.Fatal("a refused retirement wrote")
	}
	c, ref, err := coreapi.TermRetire(context.Background(), w.Ports(), changeAsk(root, "gragas term retire spare"), model.RetireTerm{Slug: "spare", Why: retireWhy()})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if dec := read(t, w, root, "book/decisions.yaml"); !strings.Contains(dec, "slug: decide-retire-spare") || !strings.Contains(dec, "title: Nothing uses it") {
		t.Errorf("decisions:\n%s", dec)
	}
	if !strings.Contains(read(t, w, root, "book/history.jsonl"), `"decision":"decide-retire-spare"`) {
		t.Error("the history does not name the decision")
	}
}
