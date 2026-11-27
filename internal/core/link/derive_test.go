package link_test

import (
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// bind is a binding of the given rung that names pair n; each rung and pair
// is its own binding.
func bind(rung model.Rung, pair int) model.Binding {
	return model.Binding{Tool: "lint", Key: fmt.Sprintf("k%d.%d", rung, pair), Rung: rung, Pair: pair}
}

func rule(id string, st model.Status, s model.Strength, bs ...model.Binding) model.Rule {
	r := model.Rule{ID: model.RuleID(id), Status: st, Strength: s, Bindings: bs}
	r.Pairs = []model.Pair{{Number: 1}}
	return r
}

// cells is the grid saying t of the bindings of r at the given places.
func cells(r model.Rule, t model.Truth, at ...int) model.Grid {
	g := model.Grid{}
	for _, i := range at {
		g[r.Bindings[i].Ref(r.ID)] = t
	}
	return g
}

// TestDeriveSaysWhatSpec17PrintsForEachCase: every row of spec 17.2 and the
// examples of 17.3, one case each. pass and fail are the places of the
// bindings the grid says pass and fail; the others are not run.
func TestDeriveSaysWhatSpec17PrintsForEachCase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		r          model.Rule
		pass, fail []int
		want       string
	}{
		{"proposed", rule("a.b.1", model.Proposed, model.Must, bind(1, 0)), nil, nil, "proposed"},
		{"retired", rule("a.b.1", model.Retired, model.Must, bind(1, 0)), []int{0}, nil, "retired"},
		{"enforced", rule("a.b.1", model.Active, model.Must, bind(1, 0)), []int{0}, nil, "enforced"},
		{"enforced at rung 0", rule("a.b.1", model.Active, model.Should, bind(0, 0)), []int{0}, nil, "enforced"},
		{"enforced at rung 5", rule("a.b.1", model.Active, model.May, bind(5, 0)), []int{0}, nil, "enforced"},
		{"enforced, trial", rule("a.b.1", model.Trial, model.Must, bind(2, 0)), []int{0}, nil, "enforced, trial"},
		{"reviewed", rule("a.b.1", model.Active, model.Must, bind(6, 1)), nil, nil, "reviewed"},
		{"reviewed, trial", rule("a.b.1", model.Trial, model.Must, bind(6, 1)), nil, nil, "reviewed, trial"},
		{"enforced beats reviewed", rule("a.b.1", model.Active, model.Must, bind(6, 1), bind(1, 0)), []int{1}, nil, "enforced"},
		{"planned, no binding", rule("a.b.1", model.Active, model.Must), nil, nil, "planned"},
		{"planned, should", rule("a.b.1", model.Active, model.Should), nil, nil, "planned"},
		{"planned, trial", rule("a.b.1", model.Trial, model.Must), nil, nil, "planned, trial"},
		{"planned, unproven: not run", rule("a.b.1", model.Active, model.Must, bind(1, 0)), nil, nil, "planned, unproven"},
		{"planned, unproven: failed", rule("a.b.1", model.Active, model.Must, bind(1, 0)), nil, []int{0}, "planned, unproven"},
		{"planned, unproven: no pair at rung 6", rule("a.b.1", model.Active, model.Must, bind(6, 9)), nil, nil, "planned, unproven"},
		{"planned, trial, unproven", rule("a.b.1", model.Trial, model.Must, bind(1, 0)), nil, nil, "planned, trial, unproven"},
		{"advisory", rule("a.b.1", model.Active, model.May), nil, nil, "advisory"},
		{"advisory, unproven", rule("a.b.1", model.Active, model.May, bind(1, 0)), nil, []int{0}, "advisory, unproven"},
		{"enforced with one failing binding", rule("a.b.1", model.Active, model.Must, bind(1, 0), bind(2, 0)), []int{1}, []int{0}, "enforced, unproven"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			grid := merge(cells(c.r, model.TruthPass, c.pass...), cells(c.r, model.TruthFail, c.fail...))
			got, err := link.Derive(model.Book{Rules: []model.Rule{c.r}}, grid, model.Chain{})
			if err != nil || len(got) != 1 {
				t.Fatalf("Derive = %v, %v", got, err)
			}
			if got[0].String() != c.want {
				t.Errorf("got %q, want %q", got[0], c.want)
			}
		})
	}
}

// TestAStatusAndStrengthThatCannotBeHeldAreRefusedWhateverTheGridSays: a trial
// on a may (with no binding, and with a passing one), a status that is none of
// the four and a strength that is none of the three have no row, with or
// without proof; the others still do.
func TestAStatusAndStrengthThatCannotBeHeldAreRefusedWhateverTheGridSays(t *testing.T) {
	t.Parallel()
	proven := rule("a.b.1", model.Trial, model.May, bind(1, 0))
	book := model.Book{Rules: []model.Rule{
		rule("a.b.1", model.Trial, model.May),
		proven,
		rule("a.b.2", model.Active, model.Must),
		rule("a.b.3", "typed", model.Must),
		rule("a.b.4", model.Active, "maybe"),
	}}
	for name, grid := range map[string]model.Grid{"empty": nil, "passing": cells(proven, model.TruthPass, 0)} {
		got, err := link.Derive(book, grid, model.Chain{})
		if err == nil {
			t.Fatalf("%s grid: no error", name)
		}
		for _, id := range []string{"a.b.1", "a.b.3", "a.b.4"} {
			if !strings.Contains(err.Error(), id) {
				t.Errorf("%s grid: %s is not named in %v", name, id, err)
			}
		}
		if len(got) != 1 || got[0].Rule != "a.b.2" {
			t.Errorf("%s grid: rows = %v, want only a.b.2", name, got)
		}
	}
}

// TestAPassRecordedForOneBindingDoesNotCarryToAnotherAfterAnUnbindOrAReorder:
// a cell names the binding it proved by what decides it, so a binding that
// moves into its place reads as not run, and a binding that changed loses
// its pass.
func TestAPassRecordedForOneBindingDoesNotCarryToAnotherAfterAnUnbindOrAReorder(t *testing.T) {
	t.Parallel()
	proven := model.Binding{Tool: "lint", Key: "proven-entry", Rung: 1}
	never := model.Binding{Tool: "other", Key: "never-proven", Rung: 1}
	before := rule("a.b.1", model.Active, model.Must, proven, never)
	grid := cells(before, model.TruthPass, 0)
	held := func(r model.Rule) string {
		got, err := link.Derive(model.Book{Rules: []model.Rule{r}}, grid, model.Chain{})
		if err != nil || len(got) != 1 {
			t.Fatalf("Derive = %v, %v", got, err)
		}
		return got[0].String()
	}
	if got := held(before); got != "enforced, unproven" {
		t.Fatalf("before: %q", got)
	}
	if got := held(rule("a.b.1", model.Active, model.Must, never)); got != "planned, unproven" {
		t.Errorf("after unbinding the proven one, the grid kept: %q, want planned, unproven", got)
	}
	if got := held(rule("a.b.1", model.Active, model.Must, never, proven)); got != "enforced, unproven" {
		t.Errorf("after a reorder: %q, want enforced, unproven", got)
	}
	changed := proven
	changed.Value = "stricter"
	if got := held(rule("a.b.1", model.Active, model.Must, changed)); got != "planned, unproven" {
		t.Errorf("after the binding changed: %q, want planned, unproven", got)
	}
	remedied := proven
	remedied.Remedy = "fix it" // display only: the proof stands
	if got := held(rule("a.b.1", model.Active, model.Must, remedied)); got != "enforced" {
		t.Errorf("after only the remedy changed: %q, want enforced", got)
	}
}

// gen is a deterministic generator (splitmix64) for the property tests: the
// same seed makes the same cases, so a failure names the case that fails.
type gen struct{ state uint64 }

func (g *gen) intn(n int) int {
	g.state += 0x9e3779b97f4a7c15
	z := g.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return int((z ^ (z >> 31)) % uint64(n))
}

// randomRule builds one rule from the generator.
func randomRule(rng *gen, id string) (model.Rule, model.Grid) {
	r := model.Rule{
		ID:       model.RuleID(id),
		Status:   model.Statuses()[rng.intn(4)],
		Strength: model.Strengths()[rng.intn(3)],
		Pairs:    []model.Pair{{Number: 1}},
	}
	g := model.Grid{}
	for range rng.intn(4) {
		r.Bindings = append(r.Bindings, bind(model.Rung(rng.intn(7)), rng.intn(3)))
		g[r.Bindings[len(r.Bindings)-1].Ref(r.ID)] = model.Truths()[rng.intn(4)]
	}
	return r, g
}

func merge(a, b model.Grid) model.Grid {
	out := model.Grid{}
	maps.Copy(out, a)
	maps.Copy(out, b)
	return out
}

// TestStandingIsAFunctionOfTheRulesOwnStatusBindingsAndCells (property): for
// generated rules, a rule's row is the same alone, among others, and in any
// order, and retiring it changes that row and no other.
func TestStandingIsAFunctionOfTheRulesOwnStatusBindingsAndCells(t *testing.T) {
	t.Parallel()
	rng := &gen{state: 185}
	for n := range 300 {
		var rules []model.Rule
		grid := model.Grid{}
		for i := range 1 + rng.intn(6) {
			r, g := randomRule(rng, fmt.Sprintf("p.c.%d", i+1))
			rules = append(rules, r)
			grid = merge(grid, g)
		}
		all, _ := link.Derive(model.Book{Rules: rules}, grid, model.Chain{})
		byRule := map[model.RuleID]string{}
		for _, h := range all {
			byRule[h.Rule] = h.String()
		}
		for _, r := range rules {
			alone, _ := link.Derive(model.Book{Rules: []model.Rule{r}}, grid, model.Chain{})
			if (len(alone) == 1) != (byRule[r.ID] != "") || (len(alone) == 1 && alone[0].String() != byRule[r.ID]) {
				t.Fatalf("case %d: %s alone %v, among others %q", n, r.ID, alone, byRule[r.ID])
			}
		}
		// Retire one: only its row moves.
		k := rng.intn(len(rules))
		retired := append([]model.Rule(nil), rules...)
		retired[k].Status = model.Retired
		after, _ := link.Derive(model.Book{Rules: retired}, grid, model.Chain{}) // a generated invalid rule has no row
		for _, h := range after {
			if h.Rule == rules[k].ID {
				if h.String() != "retired" {
					t.Fatalf("case %d: retired rule stands %q", n, h)
				}
			} else if want, ok := byRule[h.Rule]; ok && h.String() != want {
				t.Fatalf("case %d: retiring %s moved %s from %q to %q", n, rules[k].ID, h.Rule, want, h)
			}
		}
	}
}

// TestUnprovenIsNeverEnforced (property): a rule whose grid names none of its
// bindings, with no rung-6 binding whose pair exists, is never enforced or
// reviewed, and a grid kept from other bindings proves none of them.
func TestUnprovenIsNeverEnforced(t *testing.T) {
	t.Parallel()
	rng := &gen{state: 1850}
	for n := range 300 {
		r, _ := randomRule(rng, "p.c.1")
		_, stale := randomRule(rng, "p.c.1") // a grid of other bindings, all of them possibly pass
		for i := range r.Bindings {
			if r.Bindings[i].Rung == model.MaxRung {
				r.Bindings[i].Pair = 7 // no such pair
			}
			r.Bindings[i].Tool = "never-run" // none of the stale grid's bindings
		}
		got, _ := link.Derive(model.Book{Rules: []model.Rule{r}}, stale, model.Chain{})
		if len(got) == 1 && (got[0].Standing == model.StandingEnforced || got[0].Standing == model.StandingReviewed) {
			t.Fatalf("case %d: %v is %s with nothing proven", n, r.Bindings, got[0])
		}
	}
}

// TestTheFixtureBookStandsAsItsGoldenCounts: the standings of a fixture
// rulebook that has every kind of rule, counted. testdata/derive.golden is
// written by a person who reads it and never by this test.
func TestTheFixtureBookStandsAsItsGoldenCounts(t *testing.T) {
	t.Parallel()
	rules := []model.Rule{
		rule("f.a.1", model.Proposed, model.Must),
		rule("f.a.2", model.Active, model.Must, bind(1, 0)),
		rule("f.a.3", model.Trial, model.Must, bind(2, 0)),
		rule("f.a.4", model.Active, model.Must, bind(6, 1)),
		rule("f.a.5", model.Active, model.Must),
		rule("f.a.6", model.Active, model.Should, bind(1, 0)),
		rule("f.a.7", model.Active, model.May),
		rule("f.a.8", model.Retired, model.Must),
	}
	grid := merge(merge(cells(rules[1], model.TruthPass, 0), cells(rules[2], model.TruthPass, 0)), cells(rules[5], model.TruthFail, 0))
	rows, err := link.Derive(model.Book{Rules: rules}, grid, model.Chain{})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[model.Standing]int{}
	marks := map[model.Mark]int{}
	for _, h := range rows {
		counts[h.Standing]++
		for _, m := range h.Marks {
			marks[m]++
		}
	}
	var got strings.Builder
	for _, s := range model.Standings() {
		fmt.Fprintf(&got, "%-9s %d\n", s, counts[s])
	}
	for _, m := range model.Marks() {
		fmt.Fprintf(&got, "mark %-8s %d\n", m, marks[m])
	}
	b, err := os.ReadFile("testdata/derive.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != string(b) {
		t.Errorf("the counts differ from testdata/derive.golden; got:\n%s", got.String())
	}
}
