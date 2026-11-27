package link_test

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

func covBind(tool, key string, rung model.Rung) model.Binding {
	return model.Binding{Tool: tool, Target: model.TargetConfig, Key: key, Rung: rung}
}

func covRule(id string, bs ...model.Binding) model.Rule {
	return model.Rule{ID: model.RuleID(id), Status: model.Active, Strength: model.Must, Bindings: bs, Pairs: []model.Pair{{Number: 1}}}
}

func covered(t *testing.T, rules []model.Rule, g model.Grid, up map[string]model.Upstream) model.Coverage {
	t.Helper()
	book := model.Book{Rules: rules}
	held, err := link.Derive(book, g, model.Chain{})
	if err != nil {
		t.Fatal(err)
	}
	return link.Coverage(book, held, up)
}

// TestAnUnboundRulebookHoldsNothingAndExplainsNothing: no binding is zero rules
// held, zero entries explained, and every rule is listed as planned.
func TestAnUnboundRulebookHoldsNothingAndExplainsNothing(t *testing.T) {
	t.Parallel()
	c := covered(t, []model.Rule{covRule("go.errors.1"), covRule("go.errors.2")}, nil, nil)
	if c.RulesHeld() != 0 || len(c.Entries) != 0 || len(c.Unmapped) != 0 || len(c.Rules) != 2 || false {
		t.Fatalf("got %+v", c)
	}
	for _, r := range c.Rules {
		if r.Standing != model.StandingPlanned || r.Held {
			t.Fatalf("%s is %s held=%v", r.Rule, r.Standing, r.Held)
		}
	}
}

// TestABoundRuleIsHeldOnlyOnceProvenAndItsEntryIsExplained: a binding that has
// not run explains its entry and holds nothing.
func TestABoundRuleIsHeldOnlyOnceProvenAndItsEntryIsExplained(t *testing.T) {
	t.Parallel()
	r := covRule("go.errors.1", covBind("lint", "errcheck", 1))
	for truth, held := range map[model.Truth]int{model.TruthNotRun: 0, model.TruthFail: 0, model.TruthPass: 1} {
		c := covered(t, []model.Rule{r}, cells(r, truth, 0), nil)
		if c.RulesHeld() != held || len(c.Entries) != 1 || c.Entries[0].Entry != "config: errcheck" || !slices.Equal(c.Entries[0].Rules, []model.RuleID{"go.errors.1"}) {
			t.Fatalf("%s: got %+v", truth, c)
		}
	}
}

func upstream() map[string]model.Upstream {
	return map[string]model.Upstream{"lint": {Tool: "lint",
		Sets:     []model.UpstreamSet{{Preset: "recommended", Entries: []string{"a", "b", "c"}}},
		Observed: []string{"a", "b", "c", "d"}}}
}

// TestAnAdoptedSetIsOneCoveredUnitAndAnEntryAddedAfterThePinIsUnmapped is the
// plant's test: a set adopted whole at rung 0 counts as one entry, naming its
// members, and the entry the upstream holds now that the set did not at the pin
// is the one unmapped. A count by entry (three explained, none unmapped) turns
// it red.
func TestAnAdoptedSetIsOneCoveredUnitAndAnEntryAddedAfterThePinIsUnmapped(t *testing.T) {
	t.Parallel()
	r := covRule("go.style.1", covBind("lint", "recommended", 0))
	c := covered(t, []model.Rule{r}, cells(r, model.TruthPass, 0), upstream())
	if len(c.Entries) != 1 || c.Entries[0].Set != "recommended" || !slices.Equal(c.Entries[0].Members, []string{"a", "b", "c"}) {
		t.Fatalf("the set is not one unit: %+v", c.Entries)
	}
	if want := []model.CoverUnmapped{{Tool: "lint", Entry: "d"}}; !slices.Equal(c.Unmapped, want) {
		t.Fatalf("unmapped %+v, want %+v", c.Unmapped, want)
	}
	if c.RulesHeld() != 1 {
		t.Fatalf("held %d", c.RulesHeld())
	}
}

// TestASetNoBindingAdoptsExplainsNothingAndAToolWithNoFileCountsZero: the
// same set at rung 1 is no adoption, so its entries are unmapped; a tool with
// no upstream file has no unmapped entry.
func TestASetNoBindingAdoptsExplainsNothingAndAToolWithNoFileCountsZero(t *testing.T) {
	t.Parallel()
	r := covRule("go.style.1", covBind("lint", "recommended", 1), covBind("other", "x", 1))
	c := covered(t, []model.Rule{r}, nil, upstream())
	if len(c.Unmapped) != 4 || c.Entries[0].Set != "" {
		t.Fatalf("got %+v", c)
	}
	if c = covered(t, []model.Rule{r}, nil, nil); len(c.Unmapped) != 0 {
		t.Fatalf("no file counts zero: %+v", c.Unmapped)
	}
}

// TestAddingABindingNeverLowersRulesHeld: for random books and grids, a binding
// added to any rule leaves the count of held rules where it was or higher.
func TestAddingABindingNeverLowersRulesHeld(t *testing.T) {
	t.Parallel()
	seed := uint64(1)
	rnd := func(n int) int { // a fixed sequence, so a failure is the same on every run
		seed = seed*6364136223846793005 + 1442695040888963407
		return int(seed>>33) % n
	}
	for range 300 {
		rules := make([]model.Rule, 1+rnd(4))
		g := model.Grid{}
		for i := range rules {
			rules[i] = covRule("a.b."+string(rune('1'+i)), covBind("lint", "k"+string(rune('a'+rnd(5))), model.Rung(rnd(7))))
			rules[i].Bindings[0].Pair = rnd(2)
			g[rules[i].Bindings[0].Ref(rules[i].ID)] = []model.Truth{model.TruthPass, model.TruthFail, model.TruthNotRun}[rnd(3)]
		}
		before := covered(t, rules, g, nil).RulesHeld()
		i := rnd(len(rules))
		more := covBind("lint", "extra"+string(rune('a'+rnd(5))), model.Rung(rnd(7)))
		rules[i].Bindings = append(slices.Clone(rules[i].Bindings), more)
		if rnd(2) == 0 {
			g[more.Ref(rules[i].ID)] = model.TruthPass
		}
		if after := covered(t, rules, g, nil).RulesHeld(); after < before {
			t.Fatalf("held fell from %d to %d adding %+v to %s", before, after, more, rules[i].ID)
		}
	}
}
