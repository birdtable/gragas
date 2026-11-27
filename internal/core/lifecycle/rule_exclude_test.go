package lifecycle_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// overlay is book() as an overlay of parent 1.0.0 with three records: an open
// exclusion of std.errors.2, a lifted one of std.errors.3, and another
// overlay's of std.errors.4. Its own go.errors.9 covers std.errors.2.
func overlay() model.Book {
	b := book()
	b.Identity = model.RulebookIdentity{Name: "house", Kind: model.KindOverlay, Extends: &model.Extends{Rulebook: "parent", Version: "1.0.0"}}
	b.Exclusions = []model.Exclusion{
		{Overlay: "house", Rule: "std.errors.2", Number: 1, Decision: "taken", Who: "w", When: "2026-10-01T09:00:00Z"},
		{Overlay: "house", Rule: "std.errors.3", Number: 1, Decision: "taken", Who: "w", When: "2026-10-01T09:00:00Z",
			Lifted: &model.Lifted{Decision: "taken", Who: "w", When: "2026-10-02T09:00:00Z"}},
		{Overlay: "elsewhere", Rule: "std.errors.4", Number: 7, Decision: "taken", Who: "w", When: "2026-10-01T09:00:00Z"},
	}
	b.Rules = append(b.Rules, model.Rule{ID: "go.errors.9", Statement: "d", Strength: model.Must, Status: model.Active, Origin: model.Authored,
		Relations: &model.Relations{Covers: []model.RuleID{"std.errors.2"}}})
	return b
}

func parent() lifecycle.Parent {
	return lifecycle.Parent{Rulebook: "parent", Version: "1.0.0", Rules: map[model.RuleID]bool{
		"std.errors.1": false, "std.errors.2": false, "std.errors.3": false, "std.errors.4": false, "std.errors.5": true,
	}}
}

func now() lifecycle.Moment {
	m := moment()
	m.When = "2026-11-08T10:00:00Z"
	return m
}

func exclusionFiles() model.PlanFiles {
	f := files()
	f.Rules, f.Exclusions = "", "rulebook/exclusions.yaml"
	return f
}

func exclude(id string) model.ExcludeRule {
	return model.ExcludeRule{ID: id, Why: newWhy(), Files: exclusionFiles()}
}

func include(id string) model.IncludeRule {
	return model.IncludeRule{ID: id, Why: newWhy(), Files: exclusionFiles()}
}

// refusedAs holds err to a refusal ending on w whose message says msg.
func refusedAs(t *testing.T, err error, w model.Word, msg string) {
	t.Helper()
	var r *lifecycle.Refusal
	if !errors.As(err, &r) || r.Word != w || !strings.Contains(r.Msg, msg) || r.Hint == "" {
		t.Fatalf("err = %#v, want a refusal ending %q saying %q, with a hint", err, w, msg)
	}
}

// TestExcludeAddsARecordWithTheNextNumberAndItsDecision: the record names the
// overlay, the parent's rule, the number after the highest the overlay used
// for it, lifted records counted, the decision, who and the instant; the
// history line is a new record, active. Risk: a number reused after a lift,
// which two records would then share.
func TestExcludeAddsARecordWithTheNextNumberAndItsDecision(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanRuleExclude(overlay(), parent(), exclude("std.errors.3"), now())
	if err != nil {
		t.Fatal(err)
	}
	c := changeFor(t, p, "exclusion:house/std.errors.3/2", "")
	for _, want := range []string{"overlay: house\n", "rule: std.errors.3\n", "number: 2\n", "decision: decide-exclude-std-errors-3\n", `who: "Ada <ada@example.org>"`, `when: "2026-11-08T10:00:00Z"`} {
		if !strings.Contains(c.After, want) || c.Before != "" {
			t.Errorf("the record lacks %q:\n%s", want, c.After)
		}
	}
	changeFor(t, p, "decision:decide-exclude-std-errors-3", "")
	want := lifecycle.Record{Kind: "exclude", Entity: "exclusion:house/std.errors.3/2", After: model.Lifecycle{Status: model.Active}, Decision: "decide-exclude-std-errors-3"}
	if p.Record.Before != nil || p.Record.Kind != want.Kind || p.Record.Entity != want.Entity || p.Record.After != want.After || p.Record.Decision != want.Decision {
		t.Errorf("record = %+v, want %+v", p.Record, want)
	}
	if fresh, err := lifecycle.PlanRuleExclude(overlay(), parent(), exclude("std.errors.4"), now()); err != nil || len(fresh.Delta.Changes) != 2 {
		t.Fatalf("exclude std.errors.4 = %+v, %v", fresh, err)
	} else {
		changeFor(t, fresh, "exclusion:house/std.errors.4/1", "")
	}
}

// TestExcludingAnExcludedRulePlansNothing: the goal holds, so the second run
// ends done, nothing to do, whatever decision it names (cli.commands.2).
func TestExcludingAnExcludedRulePlansNothing(t *testing.T) {
	t.Parallel()
	r := exclude("std.errors.2")
	r.Why = model.Why{}
	p, err := lifecycle.PlanRuleExclude(overlay(), parent(), r, now())
	if err != nil || len(p.Delta.Changes) != 0 || !strings.Contains(p.Sections[0].Lines[0], "excluded already") {
		t.Fatalf("plan = %+v, %v; want nothing to do", p, err)
	}
	if p.Record.Entity != "exclusion:house/std.errors.2/1" || p.Record.After.Status != model.Active {
		t.Errorf("record = %+v", p.Record)
	}
}

// TestOnlyAnOverlayExcludesOrIncludes: a base, and an overlay that names no
// parent, are refused by both, before the parent is looked at.
func TestOnlyAnOverlayExcludesOrIncludes(t *testing.T) {
	t.Parallel()
	base := overlay()
	base.Identity.Kind = model.KindBase
	orphan := overlay()
	orphan.Identity.Extends = nil
	for name, b := range map[string]model.Book{"a base": base, "an overlay with no parent": orphan} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := lifecycle.PlanRuleExclude(b, lifecycle.Parent{}, exclude("std.errors.1"), now())
			refusedAs(t, err, "", "only an overlay can exclude")
			_, err = lifecycle.PlanRuleInclude(b, lifecycle.Parent{}, include("std.errors.2"), now())
			refusedAs(t, err, "", "only an overlay can include")
		})
	}
}

// TestARuleTheParentDoesNotHoldIsRefused: the parent's ids decide, and an id
// it does not hold, the overlay's own among them, is refused naming the pin.
func TestARuleTheParentDoesNotHoldIsRefused(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"std.errors.9", "go.errors.1"} {
		_, err := lifecycle.PlanRuleExclude(overlay(), parent(), exclude(id), now())
		refusedAs(t, err, "", "not in the parent parent 1.0.0")
		_, err = lifecycle.PlanRuleInclude(overlay(), parent(), include(id), now())
		refusedAs(t, err, "", "not in the parent parent 1.0.0")
	}
}

// TestExcludeStandsOnADecision: none given, or one the book lacks, is refused.
func TestExcludeStandsOnADecision(t *testing.T) {
	t.Parallel()
	r := exclude("std.errors.1")
	r.Why = model.Why{}
	_, err := lifecycle.PlanRuleExclude(overlay(), parent(), r, now())
	refusedAs(t, err, "", "exclude stands on a decision")
	r.Why = model.Why{Cite: "no-such"}
	_, err = lifecycle.PlanRuleExclude(overlay(), parent(), r, now())
	refusedAs(t, err, "", "not in the book")
}

// TestIncludeLiftsTheExclusionAndKeepsTheRecord: the record gains lifted, by
// decision, who and when, and nothing of it is removed; the history line
// takes the record from active to retired; the plan names the overlay's own
// rule that covers the parent's. Risk: an include that deletes the record.
func TestIncludeLiftsTheExclusionAndKeepsTheRecord(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanRuleInclude(overlay(), parent(), include("std.errors.2"), now())
	if err != nil {
		t.Fatal(err)
	}
	entity := "exclusion:house/std.errors.2/1"
	for field, want := range map[string]string{"lifted.decision": "decide-include-std-errors-2", "lifted.who": "Ada <ada@example.org>", "lifted.when": "2026-11-08T10:00:00Z"} {
		if c := changeFor(t, p, entity, field); c.Before != "" || c.After != want {
			t.Errorf("%s = %+v, want %q", field, c, want)
		}
	}
	for _, c := range p.Delta.Changes {
		if strings.HasPrefix(c.Entity, "exclusion:") && (c.Field == "" || c.After == "") {
			t.Errorf("an include adds or takes away a record: %+v", c)
		}
	}
	if p.Record.Kind != "include" || p.Record.Entity != entity || p.Record.Before == nil || p.Record.Before.Status != model.Active || p.Record.After.Status != model.Retired {
		t.Errorf("record = %+v", p.Record)
	}
	if !slices.ContainsFunc(p.Sections[0].Lines, func(l string) bool { return strings.Contains(l, "go.errors.9 of house cover std.errors.2") }) {
		t.Errorf("the plan does not name the covering rule: %q", p.Sections[0].Lines)
	}
}

// TestIncludeOfARuleNotExcludedEndsUsage: never excluded, or lifted already,
// is usage (S4).
func TestIncludeOfARuleNotExcludedEndsUsage(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"std.errors.1", "std.errors.3", "std.errors.4"} {
		_, err := lifecycle.PlanRuleInclude(overlay(), parent(), include(id), now())
		refusedAs(t, err, model.Usage, "is not excluded by house")
	}
}

// TestIncludeOfARuleTheParentRetiredEndsNotReady: it would bind nothing until
// the pin moves to a release that holds it (S4).
func TestIncludeOfARuleTheParentRetiredEndsNotReady(t *testing.T) {
	t.Parallel()
	b := overlay()
	b.Exclusions = append(b.Exclusions, model.Exclusion{Overlay: "house", Rule: "std.errors.5", Number: 1, Decision: "taken", Who: "w", When: "2026-10-01T09:00:00Z"})
	_, err := lifecycle.PlanRuleInclude(b, parent(), include("std.errors.5"), now())
	refusedAs(t, err, model.NotReady, "retired in parent 1.0.0")
}

// TestIncludeRefusesTwoOpenExclusionsThatShareANumber: the record to mark
// cannot be told apart; validate lists the pair.
func TestIncludeRefusesTwoOpenExclusionsThatShareANumber(t *testing.T) {
	t.Parallel()
	b := overlay()
	b.Exclusions = append(b.Exclusions, b.Exclusions[0])
	_, err := lifecycle.PlanRuleInclude(b, parent(), include("std.errors.2"), now())
	refusedAs(t, err, "", "share the number 1")
}

// TestIncludeLiftsEveryOpenExclusionOfTheRule: two branches that each
// excluded the rule under different numbers both end lifted.
func TestIncludeLiftsEveryOpenExclusionOfTheRule(t *testing.T) {
	t.Parallel()
	b := overlay()
	second := b.Exclusions[0]
	second.Number = 2
	b.Exclusions = append(b.Exclusions, second)
	p, err := lifecycle.PlanRuleInclude(b, parent(), include("std.errors.2"), now())
	if err != nil {
		t.Fatal(err)
	}
	changeFor(t, p, "exclusion:house/std.errors.2/1", "lifted.decision")
	changeFor(t, p, "exclusion:house/std.errors.2/2", "lifted.decision")
}
