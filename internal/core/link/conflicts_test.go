package link_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// cfEntry is a binding of a rule on the entry key of the tool lint, at rung 1.
func cfEntry(key string, value any, sev model.Severity) model.Binding {
	return model.Binding{Tool: "lint", Target: "code", Key: key, Value: value, Severity: sev, Rung: 1}
}

func cfRule(id string, bs ...model.Binding) model.Rule {
	return model.Rule{ID: model.RuleID(id), Status: model.Active, Strength: model.Must, Bindings: bs}
}

func cfIDs(c model.Conflict) []string {
	var out []string
	for _, s := range c.Sides {
		out = append(out, string(s.Rule))
	}
	return out
}

// TestConflictsReportsTwoConflictingBindingsAndNotAManyToManyPair is the
// conflict check's promise: one fixture with two rules that set an entry to different
// values and a pair of rules that hold another entry at the same value. Only
// the first is reported. The plant is the same fixture with the pair's values
// made different, which reports it too.
func TestConflictsReportsTwoConflictingBindingsAndNotAManyToManyPair(t *testing.T) {
	t.Parallel()
	fixture := func(pairValue any) model.Book {
		return model.Book{Rules: []model.Rule{
			cfRule("a.b.1", cfEntry("no-x", true, model.SeverityError)),
			cfRule("a.b.2", cfEntry("no-x", false, model.SeverityError)),
			cfRule("a.b.3", cfEntry("shared", 3, model.SeverityWarning)),
			cfRule("a.b.4", cfEntry("shared", pairValue, model.SeverityWarning)),
		}}
	}
	got := link.Conflicts(fixture(3), nil)
	if len(got) != 1 || got[0].Entry.Key != "no-x" || !slices.Equal(cfIDs(got[0]), []string{"a.b.1", "a.b.2"}) {
		t.Fatalf("conflicts %+v, want only no-x between a.b.1 and a.b.2", got)
	}
	if !slices.Equal(got[0].Differs, []string{model.DiffValue}) || !got[0].Open() {
		t.Errorf("the conflict differs in %v and open is %v, want the value and open", got[0].Differs, got[0].Open())
	}
	plant := link.Conflicts(fixture(4), nil)
	if len(plant) != 2 || plant[1].Entry.Key != "shared" {
		t.Errorf("the plant (the pair at different values) reported %+v, want it too", plant)
	}
}

// TestConflictsNamesWhatTheBindingsDifferIn: value, options and severity each,
// and all three; the same value, options and severity is none.
func TestConflictsNamesWhatTheBindingsDifferIn(t *testing.T) {
	t.Parallel()
	with := func(b model.Binding, o map[string]any) model.Binding { b.Options = o; return b }
	cases := []struct {
		name string
		a, b model.Binding
		want []string
	}{
		{"value", cfEntry("k", 1, model.SeverityError), cfEntry("k", 2, model.SeverityError), []string{"value"}},
		{"options", with(cfEntry("k", 1, model.SeverityError), map[string]any{"n": 1}), with(cfEntry("k", 1, model.SeverityError), map[string]any{"n": 2}), []string{"options"}},
		{"severity", cfEntry("k", 1, model.SeverityError), cfEntry("k", 1, model.SeverityWarning), []string{"severity"}},
		{"all three", with(cfEntry("k", 1, model.SeverityError), map[string]any{"n": 1}), cfEntry("k", 2, model.SeverityInfo), []string{"value", "options", "severity"}},
		{"none", with(cfEntry("k", 1, model.SeverityError), map[string]any{"n": 1}), with(cfEntry("k", 1, model.SeverityError), map[string]any{"n": 1}), nil},
		{"an absent option is an empty one", cfEntry("k", 1, model.SeverityError), with(cfEntry("k", 1, model.SeverityError), map[string]any{}), nil},
	}
	for _, c := range cases {
		got := link.Conflicts(model.Book{Rules: []model.Rule{cfRule("a.b.1", c.a), cfRule("a.b.2", c.b)}}, nil)
		switch {
		case c.want == nil && len(got) != 0:
			t.Errorf("%s: reported %+v, want no conflict", c.name, got)
		case c.want != nil && (len(got) != 1 || !slices.Equal(got[0].Differs, c.want)):
			t.Errorf("%s: reported %+v, want one differing in %v", c.name, got, c.want)
		}
	}
}

// TestConflictsComparesValuesThroughThePluginsRank: two spellings the plugin
// ranks as the same value are one value, and a number is its value however it
// is written; without the rank the same two bindings conflict. The plant is a
// nil rank.
func TestConflictsComparesValuesThroughThePluginsRank(t *testing.T) {
	t.Parallel()
	scale := scale{off: []any{false, "off"}, order: []any{"warn", "error"}}.every()
	off := model.Book{Rules: []model.Rule{
		cfRule("a.b.1", cfEntry("k", false, model.SeverityError)),
		cfRule("a.b.2", cfEntry("k", "off", model.SeverityError)),
	}}
	if got := link.Conflicts(off, scale); len(got) != 0 {
		t.Errorf("false and off, both off to the plugin, reported %+v, want none", got)
	}
	if got := link.Conflicts(off, nil); len(got) != 1 {
		t.Errorf("the plant (no rank) reported %+v, want the two spellings in conflict", got)
	}
	num := model.Book{Rules: []model.Rule{
		cfRule("a.b.1", cfEntry("k", 1, model.SeverityError)),
		cfRule("a.b.2", cfEntry("k", 1.0, model.SeverityError)),
	}}
	if got := link.Conflicts(num, nil); len(got) != 0 {
		t.Errorf("1 and 1.0 reported %+v, want none", got)
	}
	on := model.Book{Rules: []model.Rule{
		cfRule("a.b.1", cfEntry("k", false, model.SeverityError)),
		cfRule("a.b.2", cfEntry("k", "warn", model.SeverityError)),
	}}
	if got := link.Conflicts(on, scale); len(got) != 1 {
		t.Errorf("off against warn reported %+v, want a conflict", got)
	}
}

// TestConflictsReadsOnlyTheBindingsThatHoldAnEntry: a proposed or a retired
// rule holds none, and a rung-6 binding names a question, not an entry. The
// plant of each is the same pair with the rule active, or at rung 1.
func TestConflictsReadsOnlyTheBindingsThatHoldAnEntry(t *testing.T) {
	t.Parallel()
	other := cfRule("a.b.2", cfEntry("k", 2, model.SeverityError))
	for _, st := range []model.Status{model.Proposed, model.Retired} {
		r := cfRule("a.b.1", cfEntry("k", 1, model.SeverityError))
		r.Status = st
		if got := link.Conflicts(model.Book{Rules: []model.Rule{r, other}}, nil); len(got) != 0 {
			t.Errorf("a %s rule's binding conflicted: %+v", st, got)
		}
	}
	trial := cfRule("a.b.1", cfEntry("k", 1, model.SeverityError))
	trial.Status = model.Trial
	if got := link.Conflicts(model.Book{Rules: []model.Rule{trial, other}}, nil); len(got) != 1 {
		t.Errorf("the plant (a trial rule) reported %+v, want the conflict", got)
	}
	q := cfEntry("k", 1, model.SeverityError)
	q.Rung = model.MaxRung
	if got := link.Conflicts(model.Book{Rules: []model.Rule{cfRule("a.b.1", q), other}}, nil); len(got) != 0 {
		t.Errorf("a rung-6 binding conflicted: %+v", got)
	}
}

// TestAChildsTighterBindingWinsAcrossAChainAndALooserOneConflicts: a binding
// that tightens the parent's rule wins where it is a stricter value by the
// plugin's rank or a stricter severity, with the same options (S70); where
// it is looser, or its options differ, the two are in conflict. The plant is
// the same child, looser, and one with an option dropped (gragas#1554).
func TestAChildsTighterBindingWinsAcrossAChainAndALooserOneConflicts(t *testing.T) {
	t.Parallel()
	scale := scale{off: []any{false}, order: []any{"warn", "error"}}.every()
	build := func(parent, child model.Binding) model.Book {
		child.Tightens = "p.a.1"
		return model.Book{Rules: []model.Rule{cfRule("p.a.1", parent), cfRule("c.a.1", child)}}
	}
	parent := cfEntry("k", "warn", model.SeverityWarning)
	if got := link.Conflicts(build(parent, cfEntry("k", "error", model.SeverityError)), scale); len(got) != 0 {
		t.Errorf("a tighter value and severity reported %+v, want none", got)
	}
	if got := link.Conflicts(build(parent, cfEntry("k", "warn", model.SeverityError)), scale); len(got) != 0 {
		t.Errorf("a stricter severity alone reported %+v, want none", got)
	}
	if got := link.Conflicts(build(cfEntry("k", "error", model.SeverityError), cfEntry("k", "warn", model.SeverityError)), scale); len(got) != 1 {
		t.Errorf("a looser value reported %+v, want the conflict", got)
	}
	if got := link.Conflicts(build(parent, cfEntry("k", "error", model.SeverityInfo)), scale); len(got) != 1 {
		t.Errorf("a stricter value with a looser severity reported %+v, want the conflict", got)
	}
	if got := link.Conflicts(build(parent, cfEntry("k", false, model.SeverityError)), scale); len(got) != 1 {
		t.Errorf("an off value against warn reported %+v, want the conflict", got)
	}
	// Planted (gragas#1554): dropping an option is no tightening, as a
	// stricter severity with fewer options was once let through.
	held := cfEntry("k", "warn", model.SeverityWarning)
	held.Options = map[string]any{"checkLoops": true}
	dropped := cfEntry("k", "warn", model.SeverityError)
	if got := link.Conflicts(build(held, dropped), scale); len(got) != 1 {
		t.Errorf("a stricter severity with an option dropped reported %+v, want the conflict", got)
	}
	kept := dropped
	kept.Options = map[string]any{"checkLoops": true}
	if got := link.Conflicts(build(held, kept), scale); len(got) != 0 {
		t.Errorf("a stricter severity with the same options reported %+v, want none", got)
	}
}

// TestADecisionThatNamesAWinnerResolvesAConflictOnlyWithTheRelation: a
// conflicts relation between two of the rules and a decision, not superseded,
// that cites one rule's binding on the entry names the winner; without the
// relation, with the decision superseded or with two winners named, the
// conflict stays open.
func TestADecisionThatNamesAWinnerResolvesAConflictOnlyWithTheRelation(t *testing.T) {
	t.Parallel()
	build := func(related bool, d model.Decision) model.Book {
		a := cfRule("a.b.1", cfEntry("k", 1, model.SeverityError))
		b := cfRule("a.b.2", cfEntry("k", 2, model.SeverityError))
		if related {
			b.Relations = &model.Relations{Conflicts: []model.RuleID{"a.b.1"}}
		}
		return model.Book{Rules: []model.Rule{a, b}, Decisions: []model.Decision{d}}
	}
	cite := func(bindings ...string) model.Decision {
		return model.Decision{Slug: "d", Cites: &model.Cites{Bindings: bindings}}
	}
	got := link.Conflicts(build(true, cite("a.b.2 lint k")), nil)
	if len(got) != 1 || got[0].Winner != "a.b.2" || got[0].Open() {
		t.Fatalf("a related pair and a decision naming a.b.2: %+v, want it the winner", got)
	}
	for name, book := range map[string]model.Book{
		"no relation":      build(false, cite("a.b.2 lint k")),
		"no decision":      build(true, model.Decision{Slug: "d"}),
		"superseded":       build(true, model.Decision{Slug: "d", SupersededBy: "e", Cites: &model.Cites{Bindings: []string{"a.b.2 lint k"}}}),
		"two winners":      build(true, cite("a.b.1 lint k", "a.b.2 lint k")),
		"another entry":    build(true, cite("a.b.2 lint other")),
		"a rule outside":   build(true, cite("x.y.1 lint k")),
		"a malformed cite": build(true, cite("a.b.2 lint")),
	} {
		got := link.Conflicts(book, nil)
		if len(got) != 1 || !got[0].Open() {
			t.Errorf("%s: %+v, want the conflict open", name, got)
		}
	}
}

// TestDeriveBlocksTheBindingsOfAnOpenConflict: both rules of an open conflict
// read "planned, blocked"; a rule with another proven binding is "enforced,
// blocked"; once a decision resolves the conflict nothing is blocked. The
// plant is the chain left empty.
func TestDeriveBlocksTheBindingsOfAnOpenConflict(t *testing.T) {
	t.Parallel()
	a := cfRule("a.b.1", cfEntry("k", 1, model.SeverityError))
	b := cfRule("a.b.2", cfEntry("k", 2, model.SeverityError), cfEntry("other", 1, model.SeverityError))
	book := model.Book{Rules: []model.Rule{a, b}}
	grid := model.Grid{}
	for _, r := range book.Rules {
		for _, bd := range r.Bindings {
			grid[bd.Ref(r.ID)] = model.TruthPass
		}
	}
	chain := model.Chain{Conflicts: link.Conflicts(book, nil)}
	rows, err := link.Derive(book, grid, chain)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0].String(); got != "planned, blocked" {
		t.Errorf("the rule whose only binding is in the conflict reads %q, want planned, blocked", got)
	}
	if got := rows[1].String(); got != "enforced, blocked" {
		t.Errorf("the rule with another proven binding reads %q, want enforced, blocked", got)
	}
	clean, _ := link.Derive(book, grid, model.Chain{})
	if got := clean[0].String(); got != "enforced" || strings.Contains(clean[1].String(), "blocked") {
		t.Errorf("the plant (an empty chain) reads %q and %q, want nothing blocked", got, clean[1].String())
	}
	resolved := chain
	resolved.Conflicts = []model.Conflict{chain.Conflicts[0]}
	resolved.Conflicts[0].Winner = "a.b.2"
	if rows, _ := link.Derive(book, grid, resolved); rows[0].Has(model.MarkBlocked) {
		t.Errorf("a resolved conflict still blocks: %s", rows[0])
	}
}

// TestConflictsDoNotDependOnTheOrderOfTheRules: the conflicts found are the
// same set whichever order the rules are in.
func TestConflictsDoNotDependOnTheOrderOfTheRules(t *testing.T) {
	t.Parallel()
	rules := []model.Rule{
		cfRule("a.b.1", cfEntry("k", 1, model.SeverityError)),
		cfRule("a.b.2", cfEntry("k", 2, model.SeverityError)),
		cfRule("a.b.3", cfEntry("j", 1, model.SeverityError)),
		cfRule("a.b.4", cfEntry("j", 1, model.SeverityWarning)),
		cfRule("a.b.5", cfEntry("m", 1, model.SeverityError)),
	}
	keys := func(rs []model.Rule) []string {
		var out []string
		for _, c := range link.Conflicts(model.Book{Rules: rs}, nil) {
			out = append(out, c.Entry.Key+":"+strings.Join(c.Differs, ","))
		}
		slices.Sort(out)
		return out
	}
	want := keys(rules)
	for i := range rules {
		rot := append(slices.Clone(rules[i:]), rules[:i]...)
		if got := keys(rot); !slices.Equal(got, want) {
			t.Errorf("rotated by %d: %v, want %v", i, got, want)
		}
	}
}
