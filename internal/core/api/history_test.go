package coreapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// renumbered is the rules file of a rulebook whose one rule was renumbered:
// go.errors.2 is its id and go.errors.1 an alias it keeps forever (S62).
const renumbered = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.2
    aliases: [go.errors.1]
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
`

// historyLine is the canonical line of an entry, and its digest.
func historyLine(t *testing.T, number int, second int, entity, previous string) (string, string) {
	t.Helper()
	v := canon.Object{
		"format": canon.Int(3), "number": canon.Int(number), "kind": canon.String("promote"),
		"entity": canon.String(entity), "after": canon.Object{"status": canon.String("active")},
		"who": canon.String("ada"), "when": canon.String(fmt.Sprintf("2026-01-01T00:00:%02dZ", second)),
		"plan": canon.String(fmt.Sprintf("sha256:%064x", second)),
	}
	if previous != "" {
		v["previous"] = canon.String(previous)
	}
	b, err := canon.Encode(v, canon.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	d, err := canon.DigestOf(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), d.String()
}

// threeEntries is a chain of three: one under the old id, one under the new
// id and one about another rule.
func threeEntries(t *testing.T) string {
	t.Helper()
	l1, d1 := historyLine(t, 1, 1, "rule:go.errors.1", "")
	l2, d2 := historyLine(t, 2, 2, "rule:go.errors.2", d1)
	l3, _ := historyLine(t, 3, 3, "rule:go.other.1", d2)
	return l1 + "\n" + l2 + "\n" + l3 + "\n"
}

func historyWorld(t *testing.T, rules, history string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	files := map[string]string{"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure, "book/language/go/rules/errors.yaml": rules}
	if history != "" {
		files["book/history.jsonl"] = history
	}
	return w, w.Seed("repo", files)
}

func entities(l coreapi.HistoryListing) []string {
	var out []string
	for _, e := range l.Entries {
		out = append(out, fmt.Sprintf("%d %s", e.Number, e.Entity))
	}
	return out
}

func TestHistoryOfARulebookWithNoFileIsEmptyAndSaysWhy(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, renumbered, "")
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 0 || got.Total != 0 || got.Next != 1 || got.Source != "the working tree" || len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "book/history.jsonl is not there") {
		t.Fatalf("listing %+v", got)
	}
}

func TestHistoryListsTheWorkingTreeInOrder(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, renumbered, threeEntries(t))
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1 rule:go.errors.1", "2 rule:go.errors.2", "3 rule:go.other.1"}
	if !slices.Equal(entities(got), want) || got.Total != 3 || got.Next != 4 || got.Schema != coreapi.HistorySchema {
		t.Fatalf("listing %v total %d next %d", entities(got), got.Total, got.Next)
	}
}

func TestHistoryByEntityKeepsOnlyThatEntity(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, renumbered, threeEntries(t))
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root, Entity: "rule:go.other.1"})
	if err != nil || !slices.Equal(entities(got), []string{"3 rule:go.other.1"}) || got.Total != 3 {
		t.Fatalf("%v total %d, %v", entities(got), got.Total, err)
	}
}

// TestHistoryByARuleResolvesItsAliases: the entry written before the rule was
// renumbered and the one written after both belong to it, whichever of its
// two ids is asked for.
func TestHistoryByARuleResolvesItsAliases(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, renumbered, threeEntries(t))
	for _, id := range []string{"go.errors.2", "go.errors.1"} {
		got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root, Rule: id})
		want := []string{"1 rule:go.errors.1", "2 rule:go.errors.2"}
		if err != nil || !slices.Equal(entities(got), want) {
			t.Errorf("rule %s: %v, %v; want %v", id, entities(got), err, want)
		}
		if len(got.Notes) != 0 {
			t.Errorf("rule %s: notes %q", id, got.Notes)
		}
	}
}

func TestHistoryByAnIdTheBookDoesNotHoldMatchesTheNameAsWrittenAndSaysSo(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, renumbered, threeEntries(t))
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root, Rule: "go.other.1"})
	if err != nil || !slices.Equal(entities(got), []string{"3 rule:go.other.1"}) {
		t.Fatalf("%v, %v", entities(got), err)
	}
	if len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "go.other.1 is not a rule of this rulebook") {
		t.Fatalf("notes %q", got.Notes)
	}
}

func TestHistoryAtARevReadsThroughTheHistoryPortNotTheTree(t *testing.T) {
	t.Parallel()
	l1, _ := historyLine(t, 1, 1, "rule:go.errors.2", "")
	w, root := historyWorld(t, renumbered, threeEntries(t))
	w.SeedRepo(root, porttest.Repo{Tags: map[string]porttest.Rev{"v1": {Commit: "c1", Files: map[string]string{"book/history.jsonl": l1 + "\n"}}}})
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root, Rev: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(entities(got), []string{"1 rule:go.errors.2"}) || got.Source != "v1" {
		t.Fatalf("%v from %q", entities(got), got.Source)
	}
}

func TestHistoryAtARevThatLacksTheFileIsEmptyAndAtOneThatDoesNotExistIsAnError(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, renumbered, threeEntries(t))
	w.SeedRepo(root, porttest.Repo{Tags: map[string]porttest.Rev{"v0": {Commit: "c0", Files: map[string]string{"x": "y"}}}})
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root, Rev: "v0"})
	if err != nil || len(got.Entries) != 0 || len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "not there at v0") {
		t.Fatalf("%+v, %v", got, err)
	}
	if _, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root, Rev: "nope"}); err == nil {
		t.Fatal("a rev that does not exist is an error")
	}
}

func TestHistoryCarriesWhatTheReaderSkippedAndFound(t *testing.T) {
	t.Parallel()
	l1, d1 := historyLine(t, 1, 1, "rule:go.errors.2", "")
	l3, _ := historyLine(t, 3, 3, "rule:go.errors.2", d1)
	w, root := historyWorld(t, renumbered, l1+"\n{\"number\": 2, \"ki\n"+l3+"\n")
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || len(got.Skipped) != 1 || got.Skipped[0].Line != 2 {
		t.Fatalf("entries %d skipped %+v", len(got.Entries), got.Skipped)
	}
	var kinds []string
	for _, f := range got.Findings {
		kinds = append(kinds, fmt.Sprintf("%s %d", f.Kind, f.Number))
	}
	if !slices.Equal(kinds, []string{"gap 2"}) {
		t.Fatalf("findings %q", kinds)
	}
	if got.Next != 4 {
		t.Fatalf("next %d, want 4", got.Next)
	}
}

// TestHistoryJSONHasTheSameFieldsOnEveryRow: a decision that was not taken is
// null, not left out (cli.commands.24, cli.output.9).
func TestHistoryJSONHasTheSameFieldsOnEveryRow(t *testing.T) {
	t.Parallel()
	line, _ := historyLine(t, 1, 1, "rule:go.errors.2", "")
	w, root := historyWorld(t, renumbered, line+"\n")
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Schema  int              `json:"schema"`
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	row := doc.Entries[0]
	for _, k := range []string{"number", "kind", "entity", "who", "when", "plan", "decision", "previous", "before", "after"} {
		if _, ok := row[k]; !ok {
			t.Errorf("the row has no %q: %s", k, b)
		}
	}
	if row["decision"] != nil || row["before"] != nil || row["previous"] != nil || doc.Schema != 1 {
		t.Errorf("row %v: want decision, before and previous null", row)
	}
}

// TestHistoryByAnIdWhenTheBookHasFindingsSaysItMayBeOneTheBookCannotRead: a
// rule with a finding is not read, so its alias does not resolve, and the
// listing says why it matched by name.
func TestHistoryByAnIdWhenTheBookHasFindingsSaysItMayBeOneTheBookCannotRead(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, renumbered, threeEntries(t))
	doc, err := ordered.ParseYAML([]byte(renumbered))
	if err != nil {
		t.Fatal(err)
	}
	w.Plant("v3/rules.schema.json", []byte(ordered.JSON(doc, -1)), model.Problem{Line: 1, Msg: "planted"})
	got, err := coreapi.History(context.Background(), w.Ports(), coreapi.HistoryAsk{Root: root, Rule: "go.errors.1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "The rulebook has 4 findings, and a rule with one is not read") {
		t.Fatalf("notes %q", got.Notes)
	}
}
