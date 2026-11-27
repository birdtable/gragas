package link_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

func explainBook() model.Book {
	return model.Book{
		Identity: model.RulebookIdentity{DocsURL: "https://docs.example/rules/", ExplorerURL: ""},
		Rules: []model.Rule{
			{ID: "go.errors.2", Status: model.Active, Aliases: []model.RuleID{"go.errors.7"}, Bindings: []model.Binding{
				{Tool: "lint-a", Target: model.TargetSource, Key: "wrap", Severity: model.SeverityError, Rung: 1, RunPoint: "edit"},
				{Tool: "lint-a", Target: model.TargetSource, Key: "review", Severity: model.SeverityError, Rung: model.MaxRung, RunPoint: "review", Pair: 1},
			}},
			{ID: "go.errors.3", Status: model.Active},
		},
	}
}

// TestExplainRuleReadsAnAliasAsItsRuleAndLinksOnlyWhereTheIdentityGivesABase:
// the alias answers the rule's current id and says so, a rung-6 binding with a
// pair reads reviewed and a lower one not run, the page is the base and the id
// with no doubled slash, and no explorer link is made from no base (S72).
func TestExplainRuleReadsAnAliasAsItsRuleAndLinksOnlyWhereTheIdentityGivesABase(t *testing.T) {
	t.Parallel()
	b := explainBook()
	e, ok := link.ExplainRule(b, nil, nil, "go.errors.7")
	if !ok || e.Alias != "go.errors.2" || len(e.Rules) != 1 || e.Rules[0].Rule != "go.errors.2" {
		t.Fatalf("explained %+v, %v", e, ok)
	}
	r := e.Rules[0]
	if r.Page != "https://docs.example/rules/go.errors.2" || r.Explorer != "" {
		t.Errorf("links %q, %q", r.Page, r.Explorer)
	}
	if len(r.Bindings) != 2 || r.Bindings[0].Proof != model.TruthNotRun || r.Bindings[1].Proof != model.TruthReviewed {
		t.Errorf("bindings %+v", r.Bindings)
	}
	if _, ok := link.ExplainRule(b, nil, nil, "go.errors.99"); ok {
		t.Error("an id no rule holds was explained")
	}
}

// TestExplainEntryNamesTheRulesAndNoRuleIsUnmapped.
func TestExplainEntryNamesTheRulesAndNoRuleIsUnmapped(t *testing.T) {
	t.Parallel()
	b := explainBook()
	if e := link.ExplainEntry(b, nil, nil, "lint-a", "wrap"); e.Unmapped || len(e.Rules) != 1 {
		t.Errorf("held entry: %+v", e)
	}
	e := link.ExplainEntry(b, nil, nil, "lint-a", "other")
	if !e.Unmapped || e.Tool != "lint-a" || e.Rules == nil || len(e.Rules) != 0 {
		t.Errorf("unmapped entry: %+v", e)
	}
}
