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

const decisionRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: may
    status: proposed
    origin: authored
    reasoning:
      decision: wrap
  - id: go.errors.2
    statement: Log an error once.
    strength: may
    status: proposed
    origin: authored
`

const decisionLayout = `format: 3
part: go
concern: layout
rules:
  - id: go.layout.1
    statement: Keep a package small.
    strength: may
    status: proposed
    origin: authored
    reasoning:
      decision: wrap
`

const decisionList = `format: 3
decisions:
  - slug: wrap
    title: Wrap
    text: Wrap errors.
    rejected:
      - bare
    who: Ada
    date: "2026-01-01"
`

func decisionWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure, "book/sources.yaml": promoteSources,
		"book/language/go/rules/errors.yaml": decisionRules, "book/language/go/rules/layout.yaml": decisionLayout,
		"book/decisions.yaml": decisionList,
	})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

func supersedeReq() model.SupersedeDecision {
	return model.SupersedeDecision{Old: "wrap", Slug: "wrap-with-context", Title: "Wrap with context",
		Text: "Wrap an error with what was being done.", Rejected: []string{"the bare cause: it loses where it failed"}}
}

// TestSupersedeOfADecisionTwoRulesStandOnWritesOnePlan: the old record is
// marked and stays, the new one names it, the two rules, in two files, stand
// on the new one and the history holds a line for each, all under one plan
// digest; a second run has nothing to do.
// Planted failure: the old record is not marked, a rule is left on the old
// decision, or a line of the history carries another digest.
func TestSupersedeOfADecisionTwoRulesStandOnWritesOnePlan(t *testing.T) {
	t.Parallel()
	w, root := decisionWorld(t)
	ctx := context.Background()
	c, ref, err := coreapi.DecisionSupersede(ctx, w.Ports(), changeAsk(root, "gragas decision supersede wrap"), supersedeReq())
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	decisions := read(t, w, root, "book/decisions.yaml")
	for _, want := range []string{"superseded_by: wrap-with-context", "  - slug: wrap-with-context\n", "supersedes: wrap\n", "    cites:\n      rules:\n        - go.errors.1\n        - go.layout.1\n"} {
		if !strings.Contains(decisions, want) {
			t.Errorf("decisions.yaml lacks %q:\n%s", want, decisions)
		}
	}
	for _, f := range []string{"book/language/go/rules/errors.yaml", "book/language/go/rules/layout.yaml"} {
		got := read(t, w, root, f)
		if !strings.Contains(got, "decision: wrap-with-context") || strings.Contains(got, "decision: wrap\n") {
			t.Errorf("%s:\n%s", f, got)
		}
	}
	lines := strings.Split(strings.TrimSuffix(read(t, w, root, "book/history.jsonl"), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("history has %d lines, want 4:\n%v", len(lines), lines)
	}
	digest := func(l string) string {
		_, after, _ := strings.Cut(l, `"plan":"`)
		d, _, _ := strings.Cut(after, `"`)
		return d
	}
	for i, want := range []string{`"entity":"decision:wrap"`, `"entity":"decision:wrap-with-context"`, `"entity":"rule:go.errors.1"`, `"entity":"rule:go.layout.1"`} {
		if !strings.Contains(lines[i], want) || digest(lines[i]) == "" || digest(lines[i]) != digest(lines[0]) {
			t.Errorf("line %d = %s, want %s and the digest of line 0", i, lines[i], want)
		}
	}
	again, ref, err := coreapi.DecisionSupersede(ctx, w.Ports(), runTwo(changeAsk(root, "gragas decision supersede wrap")), supersedeReq())
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if second := again.Apply(ctx, ""); second.Word != model.Done || !second.Nothing {
		t.Errorf("a second run ended %q nothing=%v (%v)", second.Word, second.Nothing, second.Cause)
	}
}

// TestSupersedeOfASupersededDecisionIsRefusedNamingTheHead.
func TestSupersedeOfASupersededDecisionIsRefusedNamingTheHead(t *testing.T) {
	t.Parallel()
	w, root := decisionWorld(t)
	ctx := context.Background()
	c, ref, err := coreapi.DecisionSupersede(ctx, w.Ports(), changeAsk(root, "gragas decision supersede wrap"), supersedeReq())
	if out := applied(t, c, ref, err); out.Word != model.Done {
		t.Fatalf("the first supersede ended %q", out.Word)
	}
	before := read(t, w, root, "book/decisions.yaml")
	other := supersedeReq()
	other.Slug = "wrap-again"
	_, ref, err = coreapi.DecisionSupersede(ctx, w.Ports(), runTwo(changeAsk(root, "gragas decision supersede wrap")), other)
	if err != nil || ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), "the head of its chain is wrap-with-context") {
		t.Fatalf("ref = %+v, err = %v", ref, err)
	}
	if read(t, w, root, "book/decisions.yaml") != before {
		t.Error("a refused supersede wrote")
	}
}

// TestADecisionIsRecordedWithItsRejectedOptionAndTwiceHasNothingToDo.
func TestADecisionIsRecordedWithItsRejectedOptionAndTwiceHasNothingToDo(t *testing.T) {
	t.Parallel()
	w, root := decisionWorld(t)
	ctx := context.Background()
	req := model.NewDecisionRecord{Slug: "small", Title: "Small", Text: "Keep it small.", Rejected: []string{"big: slow"},
		Sources: []string{"effective-go"}, Rules: []model.RuleID{"go.layout.1"}}
	c, ref, err := coreapi.DecisionNew(ctx, w.Ports(), changeAsk(root, "gragas decision new"), req)
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply: %+v", out)
	}
	got := read(t, w, root, "book/decisions.yaml")
	if !strings.HasPrefix(got, decisionList) || !strings.Contains(got, "  - slug: small\n    title: Small\n    text: Keep it small.\n    rejected:\n      - \"big: slow\"\n    cites:\n      sources:\n        - effective-go\n      rules:\n        - go.layout.1\n") {
		t.Errorf("decisions.yaml:\n%s", got)
	}
	if hist := read(t, w, root, "book/history.jsonl"); strings.Count(hist, "\n") != 1 || !strings.Contains(hist, `"entity":"decision:small"`) {
		t.Errorf("history:\n%s", hist)
	}
	again, ref, err := coreapi.DecisionNew(ctx, w.Ports(), runTwo(changeAsk(root, "gragas decision new")), req)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if second := again.Apply(ctx, ""); second.Word != model.Done || !second.Nothing {
		t.Errorf("a second run ended %q nothing=%v (%v)", second.Word, second.Nothing, second.Cause)
	}
}
