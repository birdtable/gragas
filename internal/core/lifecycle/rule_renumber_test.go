package lifecycle_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func renumber(id string) model.RenumberRule {
	return model.RenumberRule{ID: id, Why: model.Why{New: &model.NewDecision{Title: "Renumber it", Text: "Two branches took the id."}}, Files: files()}
}

func TestRenumberTakesTheNextNumberAboveEveryIDAndAliasAndKeepsTheOldIDAsAnAlias(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanRuleRenumber(book(), renumber("go.errors.1"), moment())
	if err != nil {
		t.Fatal(err)
	}
	// go.errors.1 has the alias go.errors.4 and go.errors.2 is retired: 5 is the first never held.
	if c := changeFor(t, p, "rule:go.errors.1", "id"); c.Before != "go.errors.1" || c.After != "go.errors.5" {
		t.Errorf("id change = %+v", c)
	}
	if c := changeFor(t, p, "rule:go.errors.1", "aliases[]"); c.Before != "" || c.After != "go.errors.1" {
		t.Errorf("alias change = %+v", c)
	}
	changeFor(t, p, "decision:decide-renumber-go-errors-1", "")
	if p.Record.Kind != "renumber" || p.Record.Entity != "rule:go.errors.5" || p.Record.Decision != "decide-renumber-go-errors-1" {
		t.Errorf("record = %+v", p.Record)
	}
	if p.Record.Before == nil || *p.Record.Before != p.Record.After || p.Record.After.Status != model.Active {
		t.Errorf("a renumber changes no lifecycle field: before %+v, after %+v", p.Record.Before, p.Record.After)
	}
	// the alias is written before the id changes: the delta names the rule by the id it has until the last change
	var fields []string
	for _, c := range p.Delta.Changes {
		if c.Entity == "rule:go.errors.1" {
			fields = append(fields, c.Field)
		}
	}
	if strings.Join(fields, ",") != "aliases[],id" {
		t.Errorf("changes of the rule in order = %v", fields)
	}
}

func TestRenumberOfTwoRulesThatTookOneIDRenumbersTheLaterOne(t *testing.T) {
	t.Parallel()
	b := book()
	b.Clashed = []model.Rule{{ID: "go.errors.1", Statement: "later", Strength: model.Must, Status: model.Proposed, Origin: model.Authored}}
	p, err := lifecycle.PlanRuleRenumber(b, renumber("go.errors.1"), moment())
	if err != nil {
		t.Fatal(err)
	}
	if p.Record.Before == nil || p.Record.Before.Status != model.Proposed {
		t.Errorf("the plan read the rule that kept the id, not the later one: %+v", p.Record.Before)
	}
	if !strings.Contains(fmt.Sprint(p.Sections), "the later of two rules that took the id") {
		t.Errorf("the plan does not say it renumbers the later rule: %+v", p.Sections)
	}
}

func TestRenumberRefusesAnIDTheRuleNoLongerHasNamingTheOneItHas(t *testing.T) {
	t.Parallel()
	_, err := lifecycle.PlanRuleRenumber(book(), renumber("go.errors.4"), moment())
	refusal(t, err, "go.errors.1", "go.errors.4")
}

func TestRenumberStandsOnADecisionLikeEveryChangeOfALifeCycle(t *testing.T) {
	t.Parallel()
	r := renumber("go.errors.1")
	r.Why = model.Why{}
	_, err := lifecycle.PlanRuleRenumber(book(), r, moment())
	refusal(t, err, "--decision", "renumber stands on a decision")
}

// TestRenumberNeverReusesAnIDAndEveryOldIDResolvesToOneRule: over 300 renumbers
// of the rules of a concern, taken in turn, no id given was ever held, in an id
// or an alias, by any rule before; and every id and alias a rule has held
// names that rule and no other. Risk: a reused id points a link, a waiver or a
// rule map at another rule than it was written for (S62).
func TestRenumberNeverReusesAnIDAndEveryOldIDResolvesToOneRule(t *testing.T) {
	t.Parallel()
	b := book()
	held := map[model.RuleID]model.RuleID{} // every id and alias ever held, to the rule that holds it now
	for _, r := range b.Rules {
		for _, id := range append([]model.RuleID{r.ID}, r.Aliases...) {
			held[id] = r.ID
		}
	}
	for i := range 300 {
		at := i % len(b.Rules)
		old := b.Rules[at].ID
		p, err := lifecycle.PlanRuleRenumber(b, renumber(string(old)), moment())
		if err != nil {
			t.Fatalf("renumber %d of %s: %v", i, old, err)
		}
		next := model.RuleID(changeFor(t, p, "rule:"+string(old), "id").After)
		if _, taken := held[next]; taken {
			t.Fatalf("renumber %d gave %s, which a rule held before", i, next)
		}
		if p.Record.Entity != "rule:"+string(next) {
			t.Fatalf("record entity %q, want the new id %s", p.Record.Entity, next)
		}
		// make the change the plan names, as the format context writes it
		b.Rules[at].Aliases = append(b.Rules[at].Aliases, old)
		b.Rules[at].ID = next
		held[next] = next
		for _, id := range b.Rules[at].Aliases {
			held[id] = next
		}
	}
	table := map[model.RuleID]model.RuleID{}
	for _, r := range b.Rules {
		for _, id := range append([]model.RuleID{r.ID}, r.Aliases...) {
			if other, dup := table[id]; dup {
				t.Fatalf("%s names %s and %s", id, other, r.ID)
			}
			table[id] = r.ID
		}
	}
	if len(table) != len(held) {
		t.Errorf("%d ids and aliases name a rule now, and %d were held: one was lost", len(table), len(held))
	}
}

// TestARenumberWritesNoPlaceOfARuleInItsPlan: the delta, the record and what is
// shown hold no "#". A clashed rule is found by where it stands in its file,
// which only Render reads (the format context), and no id of a plan carries it.
func TestARenumberWritesNoPlaceOfARuleInItsPlan(t *testing.T) {
	t.Parallel()
	b := book()
	b.Clashed = []model.Rule{{ID: "go.errors.1", Statement: "later", Strength: model.Must, Status: model.Proposed, Origin: model.Authored}}
	p, err := lifecycle.PlanRuleRenumber(b, renumber("go.errors.1"), moment())
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprintf("%+v", p); strings.Contains(s, "#") {
		t.Errorf("the plan holds a #:\n%s", s)
	}
}
