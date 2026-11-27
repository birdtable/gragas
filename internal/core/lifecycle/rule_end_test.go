package lifecycle_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func newWhy() model.Why {
	return model.Why{New: &model.NewDecision{Title: "Retire it", Text: "It no longer holds."}}
}

func retire() model.RetireRule {
	return model.RetireRule{ID: "go.style.1", Why: newWhy(), Files: files()}
}

func refusal(t *testing.T, err error, hint, msg string) {
	t.Helper()
	var r *lifecycle.Refusal
	if !errors.As(err, &r) || !strings.Contains(r.Msg, msg) || !strings.Contains(r.Hint, hint) {
		t.Fatalf("err = %v, want a refusal saying %q with the hint %q", err, msg, hint)
	}
}

func TestRetireSetsTheStatusKeepsTheRuleAndRecordsTheDecision(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanRuleRetire(book(), retire(), moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "rule:go.style.1", "status"); c.Before != "active" || c.After != "retired" {
		t.Errorf("change = %+v", c)
	}
	changeFor(t, p, "decision:decide-retire-go-style-1", "")
	want := lifecycle.Record{Kind: "retire", Entity: "rule:go.style.1", Decision: "decide-retire-go-style-1",
		Before: &model.Lifecycle{Status: model.Active}, After: model.Lifecycle{Status: model.Retired}}
	if p.Record.Kind != want.Kind || p.Record.Decision != want.Decision || p.Record.After != want.After || *p.Record.Before != *want.Before {
		t.Errorf("record = %+v, want %+v", p.Record, want)
	}
	for _, c := range p.Delta.Changes {
		if c.Field == "" && strings.HasPrefix(c.Entity, "rule:") {
			t.Errorf("a retirement adds or removes a rule: %+v", c)
		}
	}
}

func TestRetireNamesTheSuccessorWhenGiven(t *testing.T) {
	t.Parallel()
	r := retire()
	r.Successor = "go.errors.1"
	p, err := lifecycle.PlanRuleRetire(book(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "rule:go.style.1", "replaced_by"); c.After != "go.errors.1" {
		t.Errorf("change = %+v", c)
	}
	if p.Record.After.ReplacedBy != "go.errors.1" {
		t.Errorf("record = %+v", p.Record)
	}
}

func TestRetireTwiceIsNothingToDo(t *testing.T) {
	t.Parallel()
	r := retire()
	r.ID = "go.errors.2"
	p, err := lifecycle.PlanRuleRetire(book(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Delta.Changes) != 0 || p.Record.Kind != "retire" || p.Record.Entity != "rule:go.errors.2" {
		t.Errorf("plan = %+v", p)
	}
}

func TestRetireRefusesWhatItCannotPlan(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		edit      func(*model.RetireRule)
		hint, msg string
	}{
		{"no rule", func(r *model.RetireRule) { r.ID = "go.style.9" }, "gragas rule list", "not in the book"},
		{"an old id", func(r *model.RetireRule) { r.ID = "go.errors.4" }, "go.errors.1", "old id"},
		{"no decision", func(r *model.RetireRule) { r.Why = model.Why{} }, "--decision", "decision"},
		{"two decisions", func(r *model.RetireRule) { r.Why.Cite = "taken" }, "one", "both"},
		{"a decision not in the book", func(r *model.RetireRule) { r.Why = model.Why{Cite: "nope"} }, "decision", "nope"},
		{"itself as the successor", func(r *model.RetireRule) { r.Successor = "go.style.1" }, "another", "itself"},
		{"a retired successor", func(r *model.RetireRule) { r.Successor = "go.errors.2" }, "active", "retired"},
		{"an unknown successor", func(r *model.RetireRule) { r.Successor = "go.style.9" }, "gragas rule list", "not in the book"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := retire()
			tc.edit(&r)
			_, err := lifecycle.PlanRuleRetire(book(), r, moment())
			refusal(t, err, tc.hint, tc.msg)
		})
	}
}

func TestACitedDecisionIsNotWrittenAgain(t *testing.T) {
	t.Parallel()
	r := retire()
	r.Why = model.Why{Cite: "taken"}
	p, err := lifecycle.PlanRuleRetire(book(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Delta.Changes {
		if strings.HasPrefix(c.Entity, "decision:") {
			t.Errorf("a cited decision is written: %+v", c)
		}
	}
	if p.Record.Decision != "taken" {
		t.Errorf("decision = %q", p.Record.Decision)
	}
}

func restore() model.RestoreRule {
	return model.RestoreRule{ID: "go.errors.2", ReviewBy: "2027-01-31", Why: newWhy(), Files: files()}
}

func TestRestoreReturnsARetiredRuleAsActiveWithANewReviewDate(t *testing.T) {
	t.Parallel()
	b := book()
	b.Rules[1].ReplacedBy = "go.errors.1"
	p, err := lifecycle.PlanRuleRestore(b, restore(), moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "rule:go.errors.2", "status"); c.Before != "retired" || c.After != "active" {
		t.Errorf("status = %+v", c)
	}
	if c := changeFor(t, p, "rule:go.errors.2", "review_by"); c.After != "2027-01-31" {
		t.Errorf("review_by = %+v", c)
	}
	if c := changeFor(t, p, "rule:go.errors.2", "replaced_by"); c.Before != "go.errors.1" || c.After != "" {
		t.Errorf("replaced_by = %+v, want it taken away", c)
	}
	if p.Record.Kind != "restore" || p.Record.After.Status != model.Active || p.Record.Before.Status != model.Retired {
		t.Errorf("record = %+v", p.Record)
	}
}

func TestRestoreRefusesAnActiveRuleNamingTheField(t *testing.T) {
	t.Parallel()
	r := restore()
	r.ID = "go.style.1"
	_, err := lifecycle.PlanRuleRestore(book(), r, moment())
	refusal(t, err, "gragas rule edit", "status")
}

func TestRestoreNeedsAReviewDateAndADecision(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(*model.RestoreRule){
		"no date":     func(r *model.RestoreRule) { r.ReviewBy = "" },
		"a bad date":  func(r *model.RestoreRule) { r.ReviewBy = "soon" },
		"no decision": func(r *model.RestoreRule) { r.Why = model.Why{} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := restore()
			edit(&r)
			if _, err := lifecycle.PlanRuleRestore(book(), r, moment()); err == nil {
				t.Error("planned")
			}
		})
	}
}

func replace() model.ReplaceRule {
	s := newReq()
	s.Statement = "Wrap an error and name the call."
	s.Files = model.PlanFiles{}
	return model.ReplaceRule{ID: "go.errors.1", Successor: s, Why: newWhy(), Files: files()}
}

func TestReplaceAddsTheSuccessorAndRetiresTheOldRuleWithAPointerToIt(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanRuleReplace(book(), replace(), moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "rule:go.errors.5", ""); !strings.Contains(c.After, "status: proposed") {
		t.Errorf("successor = %+v", c)
	}
	changeFor(t, p, "rule:go.errors.1", "status")
	if c := changeFor(t, p, "rule:go.errors.1", "replaced_by"); c.After != "go.errors.5" {
		t.Errorf("replaced_by = %+v", c)
	}
	if p.Record.Kind != "replace" || p.Record.Entity != "rule:go.errors.1" || p.Record.After.ReplacedBy != "go.errors.5" {
		t.Errorf("record = %+v", p.Record)
	}
	var consumers string
	for _, s := range p.Sections {
		if s.Title == "Consumer actions" {
			consumers = strings.Join(s.Lines, "\n")
		}
	}
	if !strings.Contains(consumers, "go.errors.1") || !strings.Contains(consumers, "go.errors.5") {
		t.Errorf("consumer actions = %q, want the old and the new id", consumers)
	}
}

func TestReplaceRefusesARetiredRuleAndAMissingDecision(t *testing.T) {
	t.Parallel()
	r := replace()
	r.ID = "go.errors.2"
	_, err := lifecycle.PlanRuleReplace(book(), r, moment())
	refusal(t, err, "restore", "retired")
	r = replace()
	r.Why = model.Why{}
	if _, err := lifecycle.PlanRuleReplace(book(), r, moment()); err == nil {
		t.Error("planned without a decision")
	}
}

func TestReplaceTwiceSaysTheSuccessorIsThereAndWritesNothing(t *testing.T) {
	t.Parallel()
	first, err := lifecycle.PlanRuleReplace(book(), replace(), moment())
	if err != nil {
		t.Fatal(err)
	}
	_ = first
	b := book()
	b.Rules[0].Status = model.Retired
	b.Rules[0].ReplacedBy = "go.errors.5"
	b.Rules = append(b.Rules, model.Rule{ID: "go.errors.5", Statement: "Wrap an error and name the call.", Status: model.Proposed})
	r := replace()
	p, err := lifecycle.PlanRuleReplace(b, r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Delta.Changes) != 0 || p.Record.Entity != "rule:go.errors.1" {
		t.Errorf("plan = %+v", p)
	}
}

func TestADecisionTheChangeWouldNameTwiceIsRefusedNotWrittenTwice(t *testing.T) {
	t.Parallel()
	b := book()
	b.Decisions = append(b.Decisions, model.Decision{Slug: "decide-retire-go-style-1", Title: "t", Text: "x", Who: "w", Date: "2026-01-01"})
	_, err := lifecycle.PlanRuleRetire(b, retire(), moment())
	refusal(t, err, "--decision-slug", "taken")
}
