package coreapi_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// clashRules is the concern file of a book two branches each added a rule to,
// and took the same id for: go.errors.1 is written twice.
const clashRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
  - id: go.errors.2
    statement: Name an error.
    strength: should
    status: proposed
    origin: authored
  - id: go.errors.1
    statement: Return an error.
    strength: must
    status: proposed
    origin: authored
`

func renumberReq(id string) model.RenumberRule {
	return model.RenumberRule{ID: id, Why: model.Why{New: &model.NewDecision{Title: "Two branches took one id", Text: "Give the later rule a number of its own."}}}
}

func clashWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": clashRules, "book/language/go/guide.md": ruleGuide,
	})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

// TestRenumberOfAClashedIdGivesTheLaterRuleANewIdAndKeepsTheOldAsAnAlias: of
// two rules that took go.errors.1 the later becomes go.errors.3, the next
// number the concern has not held, with go.errors.1 as its alias; the rule that
// kept the id, and the one between, are as they were; one history line names
// the new id. Risk: the rule that kept the id is the one renumbered, and every
// link and waiver that named it breaks.
func TestRenumberOfAClashedIdGivesTheLaterRuleANewIdAndKeepsTheOldAsAnAlias(t *testing.T) {
	t.Parallel()
	w, root := clashWorld(t)
	c, ref, err := coreapi.RuleRenumber(context.Background(), w.Ports(), changeAsk(root, "gragas rule renumber go.errors.1"), renumberReq("go.errors.1"))
	if err != nil || ref != nil {
		t.Fatalf("ref = %v, err = %v", ref, err)
	}
	shown, ok := c.Preview(context.Background())
	if !ok {
		t.Fatalf("preview: %+v", shown)
	}
	if got := read(t, w, root, "book/language/go/rules/errors.yaml"); got != clashRules {
		t.Fatalf("the dry run wrote the rules:\n%s", got)
	}
	out := c.Apply(context.Background(), "")
	if out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	got := read(t, w, root, "book/language/go/rules/errors.yaml")
	first, _, _ := strings.Cut(got, "  - id: go.errors.2")
	if first != strings.Split(clashRules, "  - id: go.errors.2")[0] {
		t.Errorf("the rule that kept the id changed:\n%s", first)
	}
	if !strings.Contains(got, "  - id: go.errors.3\n    statement: Return an error.\n") || strings.Contains(got, "aliases") || strings.Count(got, "go.errors.1") != 1 {
		t.Errorf("the later rule is not go.errors.3, with no alias of the id the other rule kept:\n%s", got)
	}
	hist := read(t, w, root, "book/history.jsonl")
	if strings.Count(hist, "\n") != 1 || !strings.Contains(hist, `"kind":"renumber"`) || !strings.Contains(hist, `"entity":"rule:go.errors.3"`) {
		t.Errorf("history:\n%s", hist)
	}
	// The place of the later rule is read from the load and written nowhere:
	// not in a file, in the history, in the plan shown or in a message.
	shownJSON, _ := json.Marshal(shown)
	outJSON, _ := json.Marshal(out)
	for name, s := range map[string]string{
		"rules": got, "history": hist, "decisions": read(t, w, root, "book/decisions.yaml"),
		"plan shown": string(shownJSON), "outcome": string(outJSON),
	} {
		if strings.Contains(s, "#") {
			t.Errorf("%s holds a #:\n%s", name, s)
		}
	}
}

// TestASecondRenumberKeepsBothOldIds: go.errors.1 becomes go.errors.2 and then
// go.errors.3, and the rule lists both ids it had. An alias is kept for good.
func TestASecondRenumberKeepsBothOldIds(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	for i, id := range []string{"go.errors.1", "go.errors.2"} {
		ask := changeAsk(root, "gragas rule renumber "+id)
		ask.Run = "run-" + string(rune('a'+i))
		c, ref, err := coreapi.RuleRenumber(context.Background(), w.Ports(), ask, renumberReq(id))
		if err != nil || ref != nil {
			t.Fatalf("renumber %s: ref = %v, err = %v", id, ref, err)
		}
		if out := c.Apply(context.Background(), ""); out.Word != model.Done || out.Nothing {
			t.Fatalf("renumber %s ended %q (%v)", id, out.Word, out.Cause)
		}
	}
	got := read(t, w, root, "book/language/go/rules/errors.yaml")
	if !strings.Contains(got, "  - id: go.errors.3\n") || strings.Contains(got, "id: go.errors.1\n") || strings.Contains(got, "id: go.errors.2\n") {
		t.Errorf("the rule is not go.errors.3:\n%s", got)
	}
	if !strings.Contains(got, "go.errors.1") || !strings.Contains(got, "go.errors.2") || !strings.Contains(got, "aliases") {
		t.Errorf("the rule lost an old id:\n%s", got)
	}
	if hist := read(t, w, root, "book/history.jsonl"); strings.Count(hist, `"kind":"renumber"`) != 2 {
		t.Errorf("history:\n%s", hist)
	}
}

// TestARenumberOfAnIdTheRuleNoLongerHasIsRefusedNamingTheNewOne: the second run
// of a renumber finds go.errors.1 is go.errors.2 now, and writes nothing.
func TestARenumberOfAnIdTheRuleNoLongerHasIsRefusedNamingTheNewOne(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	c, _, err := coreapi.RuleRenumber(context.Background(), w.Ports(), changeAsk(root, "gragas rule renumber go.errors.1"), renumberReq("go.errors.1"))
	if err != nil {
		t.Fatal(err)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("first run ended %q", out.Word)
	}
	before := read(t, w, root, "book/language/go/rules/errors.yaml")
	_, ref, err := coreapi.RuleRenumber(context.Background(), w.Ports(), runTwo(changeAsk(root, "gragas rule renumber go.errors.1")), renumberReq("go.errors.1"))
	if err != nil || ref == nil || !strings.Contains(ref.Hint()+ref.Observed(), "go.errors.2") {
		t.Fatalf("ref = %+v, err = %v; want a refusal naming go.errors.2", ref, err)
	}
	if read(t, w, root, "book/language/go/rules/errors.yaml") != before {
		t.Error("the refused run wrote")
	}
}
