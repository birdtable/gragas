package lifecycle_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func decisionFiles() model.PlanFiles {
	f := files()
	f.RuleFiles = f.Rules + "\nrulebook/language/go/rules/style.yaml\nrulebook/language/go/rules/other.yaml"
	f.RuleSites = "go.errors.1 " + f.Rules + "\ngo.style.1 rulebook/language/go/rules/style.yaml\ngo.errors.2 rulebook/language/go/rules/other.yaml"
	return f
}

// decisionBook is the book of the rule tests with two rules standing on the
// decision "wrap" and one on no decision.
func decisionBook() model.Book {
	b := book()
	b.Decisions = append(b.Decisions, model.Decision{Slug: "wrap", Title: "Wrap", Text: "Wrap errors.", Rejected: []string{"bare"}, Who: "w", Date: "2026-01-01"})
	b.Rules[0].Reasoning = &model.Reasoning{Decision: "wrap"}
	b.Rules[2].Reasoning = &model.Reasoning{Decision: "wrap"}
	b.Rules[1].Reasoning = &model.Reasoning{Decision: "taken"}
	return b
}

func supersede() model.SupersedeDecision {
	return model.SupersedeDecision{Old: "wrap", Slug: "wrap-with-context", Title: "Wrap with context", Text: "Wrap an error with what was being done.",
		Rejected: []string{"wrap with the bare cause: loses where it failed"}, Files: decisionFiles()}
}

// TestSupersedeMarksTheOldMovesBothRulesAndRecordsEachOfThem: one plan holds
// the new record naming the old, the old marked, the two rules on the new one
// and a history line for the old, the new and each rule.
// Planted failure: the old record is not marked, or a rule is left on it.
func TestSupersedeMarksTheOldMovesBothRulesAndRecordsEachOfThem(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanDecisionSupersede(decisionBook(), supersede(), moment())
	if err != nil {
		t.Fatal(err)
	}
	n := changeFor(t, p, "decision:wrap-with-context", "")
	for _, want := range []string{"supersedes: wrap\n", "cites:\n  rules:\n    - go.errors.1\n    - go.style.1\n", "who: \"Ada <ada@example.org>\"", "date: \"2026-11-08\"", "rejected:\n"} {
		if !strings.Contains(n.After, want) {
			t.Errorf("the new entry lacks %q:\n%s", want, n.After)
		}
	}
	if c := changeFor(t, p, "decision:wrap", "superseded_by"); c.Before != "" || c.After != "wrap-with-context" {
		t.Errorf("the old record: %+v", c)
	}
	for _, id := range []string{"go.errors.1", "go.style.1"} {
		if c := changeFor(t, p, "rule:"+id, "reasoning.decision"); c.Before != "wrap" || c.After != "wrap-with-context" {
			t.Errorf("rule %s: %+v", id, c)
		}
	}
	if len(p.Delta.Changes) != 4 {
		t.Errorf("changes = %+v", p.Delta.Changes)
	}
	var entities []string
	for _, r := range append([]lifecycle.Record{p.Record}, p.Also...) {
		entities = append(entities, r.Kind+" "+r.Entity)
	}
	want := []string{"supersede decision:wrap", "new decision:wrap-with-context", "decision-superseded rule:go.errors.1", "decision-superseded rule:go.style.1"}
	if !slices.Equal(entities, want) {
		t.Errorf("history lines = %v, want %v", entities, want)
	}
	if p.Record.After.Literal != "superseded by wrap-with-context" || p.Record.Before.Literal != "head" {
		t.Errorf("record = %+v", p.Record)
	}
	for _, line := range []string{"wrap is marked superseded by wrap-with-context", "2 rule(s) move from wrap to wrap-with-context: go.errors.1, go.style.1"} {
		if !strings.Contains(screenOf(p), line) {
			t.Errorf("the plan screen lacks %q:\n%s", line, screenOf(p))
		}
	}
}

// TestSupersedeOfASupersededDecisionIsRefusedNamingTheHeadOfTheChain.
// Planted failure: the refusal names the decision asked, not the head.
func TestSupersedeOfASupersededDecisionIsRefusedNamingTheHeadOfTheChain(t *testing.T) {
	t.Parallel()
	b := decisionBook()
	b.Decisions[1].SupersededBy = "two"
	b.Decisions = append(b.Decisions,
		model.Decision{Slug: "two", Title: "Two", Text: "x", Who: "w", Date: "2026-02-01", Supersedes: "wrap", SupersededBy: "three"},
		model.Decision{Slug: "three", Title: "Three", Text: "x", Who: "w", Date: "2026-03-01", Supersedes: "two"})
	_, err := lifecycle.PlanDecisionSupersede(b, supersede(), moment())
	refusal(t, err, "gragas decision supersede three", "the head of its chain is three")
}

func TestSupersedeTwiceIsNothingToDo(t *testing.T) {
	t.Parallel()
	b := decisionBook()
	p, err := lifecycle.PlanDecisionSupersede(b, supersede(), moment())
	if err != nil {
		t.Fatal(err)
	}
	// the book after the plan: the old marked, the new written, the rules moved
	b.Decisions[1].SupersededBy = "wrap-with-context"
	r := supersede()
	b.Decisions = append(b.Decisions, model.Decision{Slug: r.Slug, Title: r.Title, Text: r.Text, Rejected: r.Rejected, Who: "w", Date: "d", Supersedes: "wrap"})
	again, err := lifecycle.PlanDecisionSupersede(b, r, moment())
	if err != nil || len(again.Delta.Changes) != 0 || !strings.Contains(screenOf(again), "nothing") || len(p.Delta.Changes) == 0 {
		t.Fatalf("again = %+v, %v", again, err)
	}
}

func TestSupersedeRefusesAnUnknownOldAWordlessNewAndATakenSlug(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		edit       func(*model.SupersedeDecision)
		hint, said string
	}{
		"unknown":             {func(r *model.SupersedeDecision) { r.Old = "nope" }, "decisions.yaml", "is not in the book"},
		"no reason":           {func(r *model.SupersedeDecision) { r.Rejected = nil }, "--rejected", "no rejected option"},
		"no title":            {func(r *model.SupersedeDecision) { r.Title = " " }, "title and a text", "no title"},
		"bad slug":            {func(r *model.SupersedeDecision) { r.Slug = "Not A Slug" }, "hyphens", "is not a slug"},
		"multi-line title":    {func(r *model.SupersedeDecision) { r.Title = "one\ntwo" }, "title and a text", "more than one line"},
		"multi-line text":     {func(r *model.SupersedeDecision) { r.Text = "one\ntwo" }, "title and a text", "more than one line"},
		"multi-line rejected": {func(r *model.SupersedeDecision) { r.Rejected = []string{"a\nb"} }, "on one line", "more than one line"},
		"taken":               {func(r *model.SupersedeDecision) { r.Slug = "taken" }, "--decision-slug", "in the book already"},
		"itself":              {func(r *model.SupersedeDecision) { r.Slug = "wrap" }, "--decision-slug", "in the book already"},
	} {
		r := supersede()
		c.edit(&r)
		_, err := lifecycle.PlanDecisionSupersede(decisionBook(), r, moment())
		t.Run(name, func(t *testing.T) { t.Parallel(); refusal(t, err, c.hint, c.said) })
	}
}

func TestANewDecisionIsAddedWithItsCitesAndTwiceIsNothingToDo(t *testing.T) {
	t.Parallel()
	b := decisionBook()
	req := model.NewDecisionRecord{Slug: "small", Title: "Small", Text: "Keep it small.", Rejected: []string{"big: slow"},
		Sources: []string{"effective-go"}, Rules: []model.RuleID{"go.style.1"}, Bindings: []string{"go.style.1 golangci-lint revive"}, Files: decisionFiles()}
	p, err := lifecycle.PlanDecisionNew(b, req, moment())
	if err != nil {
		t.Fatal(err)
	}
	c := changeFor(t, p, "decision:small", "")
	for _, want := range []string{"slug: small\n", "cites:\n  sources:\n    - effective-go\n  rules:\n    - go.style.1\n  bindings:\n    - go.style.1 golangci-lint revive\n"} {
		if !strings.Contains(c.After, want) {
			t.Errorf("the entry lacks %q:\n%s", want, c.After)
		}
	}
	if p.Record.Kind != "new" || p.Record.Entity != "decision:small" || p.Record.Before != nil {
		t.Errorf("record = %+v", p.Record)
	}
	b.Decisions = append(b.Decisions, model.Decision{Slug: "small", Title: "Small", Text: "Keep it small.", Rejected: []string{"big: slow"},
		Cites: &model.Cites{Sources: []string{"effective-go"}, Rules: []model.RuleID{"go.style.1"}, Bindings: []string{"go.style.1 golangci-lint revive"}}})
	again, err := lifecycle.PlanDecisionNew(b, req, moment())
	if err != nil || len(again.Delta.Changes) != 0 {
		t.Fatalf("again = %+v, %v", again, err)
	}
	req.Text = "Other words."
	_, err = lifecycle.PlanDecisionNew(b, req, moment())
	refusal(t, err, "gragas decision supersede small", "in the book already, with other words")
}

func TestANewDecisionRefusesWhatItCitesThatIsNotThere(t *testing.T) {
	t.Parallel()
	base := model.NewDecisionRecord{Slug: "small", Title: "Small", Text: "Keep it small.", Rejected: []string{"big"}, Files: decisionFiles()}
	for name, c := range map[string]struct {
		edit       func(*model.NewDecisionRecord)
		hint, said string
	}{
		"source":  {func(r *model.NewDecisionRecord) { r.Sources = []string{"nope"} }, "gragas source new", "source nope"},
		"rule":    {func(r *model.NewDecisionRecord) { r.Rules = []model.RuleID{"go.nope.1"} }, "rule list", "rule go.nope.1"},
		"binding": {func(r *model.NewDecisionRecord) { r.Bindings = []string{"only two"} }, "<rule id> <tool> <key>", "is not a binding"},
	} {
		r := base
		c.edit(&r)
		_, err := lifecycle.PlanDecisionNew(decisionBook(), r, moment())
		t.Run(name, func(t *testing.T) { t.Parallel(); refusal(t, err, c.hint, c.said) })
	}
}

// TestSupersedeListsOnlyTheFilesOfTheRulesThatMove: the plan names the rules
// files of the two rules that move to the new decision, and not the file of a
// rule that stands on another decision.
// Planted failure: the plan lists every file that holds a rule on any decision.
func TestSupersedeListsOnlyTheFilesOfTheRulesThatMove(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanDecisionSupersede(decisionBook(), supersede(), moment())
	if err != nil {
		t.Fatal(err)
	}
	s := screenOf(p)
	for _, want := range []string{"rulebook/language/go/rules/errors.yaml, rulebook/language/go/rules/style.yaml: sets the decision of 2 rule(s)"} {
		if !strings.Contains(s, want) {
			t.Errorf("the plan lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "other.yaml") {
		t.Errorf("the plan names the file of a rule that does not move:\n%s", s)
	}
}
