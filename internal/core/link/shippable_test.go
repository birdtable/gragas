package link_test

import (
	"reflect"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// TestShippableSettlesAKeyAsGenerateDoes: one key bound by a trial rule and an active rule, at one
// target, is one entry when the two state it alike (both rules, in canon order,
// the entry of the first, the active one) and the first alone when they state
// it otherwise, the second named in the notes as Generate names it; a key at
// two targets is an entry at each; entries are in canon order of key. Risk: a
// plugin given one key twice, free to write the looser statement.
func TestShippableSettlesAKeyAsGenerateDoes(t *testing.T) {
	t.Parallel()
	at := func(key string, target model.TargetType, sev model.Severity) model.Binding {
		return model.Binding{Tool: "lint-a", Target: target, Key: key, Severity: sev, Rung: 1}
	}
	book := model.Book{Rules: []model.Rule{
		{ID: "go.b.2", Status: model.Trial, Bindings: []model.Binding{at("alike", model.TargetSource, model.SeverityError), at("differs", model.TargetSource, model.SeverityWarning), at("zed", model.TargetSource, model.SeverityError)}},
		{ID: "go.b.1", Status: model.Active, Bindings: []model.Binding{at("differs", model.TargetSource, model.SeverityError), at("alike", model.TargetSource, model.SeverityError), at("alike", model.TargetConfig, model.SeverityError)}},
	}}
	got, notes := link.Shippable(book, model.Chain{},
		func(tool string) bool { return tool == "lint-a" },
		func(string) ([]model.TargetType, bool) {
			return []model.TargetType{model.TargetSource, model.TargetConfig}, true
		})
	type row struct {
		key    string
		target model.TargetType
		rules  []model.RuleID
		trial  bool
	}
	var rows []row
	for _, e := range got["lint-a"] {
		rows = append(rows, row{e.Binding.Key, e.Binding.Target, e.Rules, e.Trial})
	}
	want := []row{
		{"alike", model.TargetConfig, []model.RuleID{"go.b.1"}, false},
		{"alike", model.TargetSource, []model.RuleID{"go.b.1", "go.b.2"}, false},
		{"differs", model.TargetSource, []model.RuleID{"go.b.1"}, false},
		{"zed", model.TargetSource, []model.RuleID{"go.b.2"}, true},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("entries = %+v, want %+v", rows, want)
	}
	if want := []string{"go.b.2: lint-a source differs is not written: go.b.1 already sets the entry otherwise"}; !reflect.DeepEqual(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
}

// TestShippableIsTheBindingsGenerateShips: an
// active and a trial rule ship, a proposed rule, a binding at the last rung and
// a binding of a tool not asked for do not and are not named, and a binding at
// a target the tool does not serve, or that an open conflict blocks, is named
// in the notes and not shipped. Risk: a plugin's preset that ships what an
// adapter's would not, or the reverse.
func TestShippableIsTheBindingsGenerateShips(t *testing.T) {
	t.Parallel()
	at := func(key string, target model.TargetType, rung model.Rung) model.Binding {
		return model.Binding{Tool: "lint-a", Target: target, Key: key, Severity: model.SeverityError, Rung: rung}
	}
	blocked := at("blocked", model.TargetSource, 1)
	book := model.Book{Rules: []model.Rule{
		{ID: "go.a.1", Status: model.Active, Bindings: []model.Binding{at("on", model.TargetSource, 1), at("elsewhere", model.TargetConfig, 1), at("last", model.TargetSource, model.MaxRung)}},
		{ID: "go.a.2", Status: model.Trial, Bindings: []model.Binding{at("tried", model.TargetSource, 2), {Tool: "lint-b", Target: model.TargetSource, Key: "unasked", Rung: 1}}},
		{ID: "go.a.3", Status: model.Proposed, Bindings: []model.Binding{at("proposed", model.TargetSource, 1)}},
		{ID: "go.a.4", Status: model.Active, Bindings: []model.Binding{blocked}},
	}}
	chain := model.Chain{Conflicts: []model.Conflict{{Sides: []model.ConflictSide{{Ref: blocked.Ref("go.a.4")}}}}}
	got, notes := link.Shippable(book, chain,
		func(tool string) bool { return tool == "lint-a" },
		func(string) ([]model.TargetType, bool) { return []model.TargetType{model.TargetSource}, true })
	var keys []string
	var trial []bool
	for _, e := range got["lint-a"] {
		keys, trial = append(keys, e.Binding.Key), append(trial, e.Trial)
	}
	if want := []string{"on", "tried"}; !reflect.DeepEqual(keys, want) || !reflect.DeepEqual(trial, []bool{false, true}) {
		t.Errorf("shipped %v (trial %v), want %v, the active one then the trial one", keys, trial, want)
	}
	if _, ok := got["lint-b"]; ok {
		t.Errorf("a tool not asked for shipped: %v", got["lint-b"])
	}
	want := []string{
		"go.a.1: lint-a config elsewhere is not shipped: the tool does not cover that target",
		"go.a.4: lint-a source blocked is not shipped: it is in an open entry conflict",
	}
	if !reflect.DeepEqual(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
}
