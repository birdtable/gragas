package link_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// scaleForTighten is a plugin's rank whose values run off, low, high, and
// whose key "inverted" runs the other way.
func scaleForTighten() link.Rank {
	return scale{
		off:   []any{false, "off"},
		order: []any{"low", "high"},
		keys:  map[string]scale{"inverted": {off: []any{"none"}, order: []any{"high", "low"}}},
	}.rank()
}

func borrowed() model.Binding {
	return model.Binding{
		Tool: "tool-a", Target: model.TargetType("linter"), Key: "entry", Value: "low",
		Options: map[string]any{"a": 1, "b": []any{"x"}}, Severity: model.SeverityWarning,
		Rung: 2, RunPoint: model.RunPoint("commit"),
	}
}

// TestTightenRefusesEachWayOfWeakeningAndNamesTheField: the table of S34's
// guard, one row for each refusal the guard promises, with the field each
// names. Risk: a weakening let through as a tightening, or a refusal that
// does not say which field to fix.
func TestTightenRefusesEachWayOfWeakeningAndNamesTheField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		edit  func(*model.Binding)
		field string
		says  string
	}{
		{"weaken the value", func(b *model.Binding) { b.Value = "off" }, link.FieldValue, "weaker"},
		{"weaken the value to another off word", func(b *model.Binding) { b.Value = false }, link.FieldValue, "weaker"},
		{"weaken the severity", func(b *model.Binding) { b.Severity = model.SeverityInfo }, link.FieldSeverity, "weaker"},
		{"move to another key", func(b *model.Binding) { b.Key = "other" }, link.FieldEntry, "move"},
		{"move to another tool", func(b *model.Binding) { b.Tool = "tool-b" }, link.FieldEntry, "move"},
		{"move to another target", func(b *model.Binding) { b.Target = "formatter" }, link.FieldEntry, "move"},
		{"change the rung", func(b *model.Binding) { b.Rung = 4 }, link.FieldRung, "rung"},
		{"change the run point", func(b *model.Binding) { b.RunPoint = "push" }, link.FieldRunPoint, "run point"},
		{"an option it does not hold", func(b *model.Binding) { b.Options = map[string]any{"a": 1, "c": true} }, link.FieldOptions, "cannot be compared"},
		{"an option at another value", func(b *model.Binding) { b.Options = map[string]any{"a": 2} }, link.FieldOptions, "cannot be compared"},
		{"an option dropped (planted: was a tightening)", func(b *model.Binding) { b.Options = map[string]any{"b": []any{"x"}} }, link.FieldOptions, "cannot be compared"},
		{"every option dropped", func(b *model.Binding) { b.Options = nil }, link.FieldOptions, "cannot be compared"},
		{"a stricter value with an option dropped", func(b *model.Binding) { b.Value = "high"; b.Options = nil }, link.FieldOptions, "cannot be compared"},
		{"a value the plugin does not rank", func(b *model.Binding) { b.Value = "medium" }, link.FieldValue, "cannot be compared"},
		{"a severity outside the set", func(b *model.Binding) { b.Severity = "loud" }, link.FieldSeverity, "cannot be compared"},
		{"another remedy", func(b *model.Binding) { b.Remedy = "ask someone" }, link.FieldRemedy, "cannot be compared"},
		{"another pair", func(b *model.Binding) { b.Pair = 2 }, link.FieldPair, "cannot be compared"},
		{"another checker", func(b *model.Binding) { b.Checker = "own" }, link.FieldChecker, "cannot be compared"},
		{"tighter value, weaker severity", func(b *model.Binding) { b.Value = "high"; b.Severity = model.SeverityInfo }, link.FieldSeverity, "weaker"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			next := borrowed()
			tc.edit(&next)
			same, a := link.Tighten(scaleForTighten(), borrowed(), next)
			if same || a == nil {
				t.Fatalf("same %v, against %+v: want a refusal of %s", same, a, tc.field)
			}
			if a.Field != tc.field || !strings.Contains(a.Msg, tc.says) {
				t.Errorf("against %+v, want the field %s and a message saying %q", a, tc.field, tc.says)
			}
		})
	}
}

// TestTightenLetsEachTighteningThrough: a stricter value, a value up from
// off, a higher severity, and the key's own scale where it
// runs the other way. Risk: a guard so strict an overlay cannot tighten.
func TestTightenLetsEachTighteningThrough(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		was  func(*model.Binding)
		edit func(*model.Binding)
	}{
		{"a stricter value", nil, func(b *model.Binding) { b.Value = "high" }},
		{"on from off", func(b *model.Binding) { b.Value = false }, func(b *model.Binding) { b.Value = "low" }},
		{"a higher severity", nil, func(b *model.Binding) { b.Severity = model.SeverityError }},
		{"the key's own scale", func(b *model.Binding) { b.Key, b.Value = "inverted", "high" }, func(b *model.Binding) { b.Value = "low" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			was := borrowed()
			if tc.was != nil {
				tc.was(&was)
			}
			next := was
			tc.edit(&next)
			if same, a := link.Tighten(scaleForTighten(), was, next); same || a != nil {
				t.Errorf("same %v, against %+v: want a tightening", same, a)
			}
		})
	}
	if same, a := link.Tighten(scaleForTighten(), borrowed(), borrowed()); !same || a != nil {
		t.Errorf("the borrowed binding itself: same %v, against %+v", same, a)
	}
	was := borrowed()
	was.Key, was.Value = "inverted", "high"
	next := was
	next.Value = "none"
	if _, a := link.Tighten(scaleForTighten(), was, next); a == nil || a.Field != link.FieldValue {
		t.Errorf("off by the key's own scale: against %+v, want the value refused", a)
	}
}

// TestTightenIsAnOrder (tests.generated.2): over generated pairs of
// bindings at one entry, a binding is the same as itself, and two bindings
// that each tighten the other are the same binding; so no chain of
// tightenings comes back round to a weaker binding. Risk: two rebinds, each
// let through, that together weaken.
func TestTightenIsAnOrder(t *testing.T) {
	t.Parallel()
	sc := scaleForTighten()
	values := []any{nil, false, "off", "low", "high", "medium", 3}
	sevs := []model.Severity{model.SeverityError, model.SeverityWarning, model.SeverityInfo}
	gen := func(g *rng) model.Binding {
		b := borrowed()
		b.Key = []string{"entry", "inverted"}[g.n(2)]
		b.Value = values[g.n(len(values))]
		b.Severity = sevs[g.n(len(sevs))]
		b.Options = map[string]any{}
		for i := range g.n(3) {
			b.Options[fmt.Sprintf("o%d", i+g.n(2))] = g.n(2)
		}
		return b
	}
	for seed := 1; seed <= 2000; seed++ {
		g := rng(seed * 2654435761)
		a := gen(&g)
		b := gen(&g)
		b.Key = a.Key
		if same, against := link.Tighten(sc, a, a); !same || against != nil {
			t.Fatalf("seed %d: %+v against itself: same %v, %+v", seed, a, same, against)
		}
		sab, ab := link.Tighten(sc, a, b)
		sba, ba := link.Tighten(sc, b, a)
		if sab != sba {
			t.Fatalf("seed %d: same one way and not the other\n%+v\n%+v", seed, a, b)
		}
		if ab == nil && ba == nil && !sab {
			t.Fatalf("seed %d: each tightens the other and they are not the same\n%+v\n%+v", seed, a, b)
		}
	}
}
