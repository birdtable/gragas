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

const promoteRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
    pairs:
      - number: 1
        target: source
  - id: go.errors.2
    statement: Log an error once.
    strength: may
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
  - id: go.errors.3
    statement: Return an error.
    strength: must
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
`

const promoteSources = "format: 3\nsources:\n  - slug: effective-go\n    kind: guide\n    title: Effective Go\n    locator: https://go.dev/doc/effective_go\n"

func promoteWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": promoteRules, "book/sources.yaml": promoteSources,
	})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

// TestAMustGoesThroughTrialAndANoPairIsRefused: the journey of rule promote at
// the core. A must with its source and pair goes to trial; a second promote
// before a release is refused; with one release gone by, inside the window, it
// goes active; each is one history line. A must with no pair is refused
// naming the example pair, and a may with its source goes straight to active.
// Risk: trial skipped or never left, or a rule promoted without its pair.
func TestAMustGoesThroughTrialAndANoPairIsRefused(t *testing.T) {
	t.Parallel()
	w, root := promoteWorld(t)
	ctx := context.Background()
	step := func(run string, req model.PromoteRule) (*coreapi.Change, *model.Refusal, error) {
		ask := changeAsk(root, "gragas rule promote "+req.ID)
		ask.Run = run
		return coreapi.RulePromote(ctx, w.Ports(), ask, req)
	}
	c, ref, err := step("run-1", model.PromoteRule{ID: "go.errors.1", Why: why()})
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("promote to trial: %+v", out)
	}
	if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); !strings.Contains(rules, "status: trial") {
		t.Errorf("the must is not in trial:\n%s", rules)
	}
	if _, ref, err = step("run-2", model.PromoteRule{ID: "go.errors.1", Why: model.Why{Cite: "decide-it"}}); err != nil || ref == nil || !strings.Contains(ref.Observed(), "no release has gone by") {
		t.Fatalf("a trial before a release: ref = %v, err = %v, want a refusal", ref, err)
	}
	c, ref, err = step("run-3", model.PromoteRule{ID: "go.errors.1", Why: model.Why{Cite: "decide-it"}, Releases: 1})
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("promote to active: %+v", out)
	}
	if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); !strings.Contains(rules, "status: active") {
		t.Errorf("the must is not active:\n%s", rules)
	}
	if _, ref, err = step("run-4", model.PromoteRule{ID: "go.errors.3", Why: model.Why{Cite: "decide-it"}}); err != nil || ref == nil || !strings.Contains(ref.Observed(), "example pair") {
		t.Fatalf("a must with no pair: ref = %v, err = %v, want a refusal naming the example pair", ref, err)
	}
	c, ref, err = step("run-5", model.PromoteRule{ID: "go.errors.2", Why: model.Why{Cite: "decide-it"}})
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("promote a may: %+v", out)
	}
	if h := read(t, w, root, "book/history.jsonl"); rows(h) != 3 || strings.Count(h, `"kind":"promote"`) != 3 {
		t.Errorf("history keeps %d lines, want three promotions:\n%s", rows(h), h)
	}
	c, ref, err = step("run-6", model.PromoteRule{ID: "go.errors.1", Why: model.Why{Cite: "decide-it"}, Releases: 1})
	if out := applied(t, c, ref, err); !out.Nothing || out.Word != model.Done {
		t.Fatalf("promote an active rule: %+v, want done with nothing to do", out)
	}
}

// TestConfirmMovesReviewByRecordsTheReviewerAndASecondRunHasNothingToDo:
// the cadence is the identity's review_days (30), from the day of the plan.
func TestConfirmMovesReviewByRecordsTheReviewerAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	w, root := promoteWorld(t)
	ctx := context.Background()
	c, ref, err := coreapi.RulePromote(ctx, w.Ports(), changeAsk(root, "gragas rule promote go.errors.2"), model.PromoteRule{ID: "go.errors.2", Why: why()})
	if out := applied(t, c, ref, err); out.Word != model.Done {
		t.Fatalf("promote: %+v", out)
	}
	ask := changeAsk(root, "gragas rule confirm go.errors.2")
	ask.Run = "run-2"
	c, ref, err = coreapi.RuleConfirm(ctx, w.Ports(), ask, model.ConfirmRule{ID: "go.errors.2"})
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("confirm: %+v", out)
	}
	if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); !strings.Contains(rules, "review_by: \"2026-12-08\"") && !strings.Contains(rules, "review_by: 2026-12-08") {
		t.Errorf("review_by is not 30 days on:\n%s", rules)
	}
	if h := read(t, w, root, "book/history.jsonl"); !strings.Contains(h, `"kind":"confirm"`) || !strings.Contains(h, `"who":"`+testWho+`"`) {
		t.Errorf("the history lacks the confirm or its reviewer:\n%s", h)
	}
	ask.Run = "run-3"
	c, ref, err = coreapi.RuleConfirm(ctx, w.Ports(), ask, model.ConfirmRule{ID: "go.errors.2"})
	if out := applied(t, c, ref, err); !out.Nothing || out.Word != model.Done {
		t.Fatalf("confirm twice: %+v", out)
	}
	_, ref, err = coreapi.RuleConfirm(ctx, w.Ports(), runTwo(changeAsk(root, "gragas rule confirm go.errors.1")), model.ConfirmRule{ID: "go.errors.1"})
	if err != nil || ref == nil || !strings.Contains(ref.Observed(), "only an active rule is confirmed") {
		t.Errorf("confirm of a proposed rule: ref = %v, err = %v", ref, err)
	}
}
