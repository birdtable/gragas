package coreapi

import (
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	mc "github.com/birdtable/gragas/internal/model/contract"
)

// parentLayer is the parent's bundle as a layer of a chain: its name, and the
// rules it holds with the bindings it released, as values (A10). Each binding
// is copied whole by bindingOf, and no decision is made on one here: the
// contract marks a binding's fields deciding for what an overlay's binding is
// held to (S70), and link's Tighten is what compares them. A rule's strength
// is display only, copied for the standing a plan prints and never compared.
func parentLayer(name string, doc *mc.BundleDocument) model.ChainLayer {
	rules := make([]model.Rule, 0, len(doc.Rules))
	at := map[model.RuleID]int{}
	for _, r := range doc.Rules {
		at[model.RuleID(r.ID)] = len(rules)
		rules = append(rules, model.Rule{ID: model.RuleID(r.ID), Status: model.Status(r.Status), Strength: model.Strength(r.Strength)})
	}
	attach(rules, at, doc.Bindings)
	return model.ChainLayer{Name: name, Book: model.Book{Identity: model.RulebookIdentity{Name: name}, Rules: rules}}
}

// attach puts each released binding on the rule it binds, at its place in rules.
func attach(rules []model.Rule, at map[model.RuleID]int, released []mc.Binding) {
	for _, b := range released {
		id, bound := bindingOf(b)
		if i, ok := at[id]; ok {
			rules[i].Bindings = append(rules[i].Bindings, bound)
		}
	}
}

// bindingOf is a released binding as a model binding, and the rule it binds.
func bindingOf(b mc.Binding) (model.RuleID, model.Binding) {
	return model.RuleID(b.Rule), model.Binding{
		Tool: string(b.Tool), Target: model.TargetType(b.TargetType), Key: string(b.Key), Value: plain(b.Value),
		Options: options(b.Options), Severity: model.Severity(b.Severity), Rung: model.Rung(b.Rung),
		RunPoint: model.RunPoint(b.RunPoint), Remedy: string(b.Remedy), Pair: number(b.Pair), PairPath: string(b.PairPath),
		Tightens: tightens(b.Tightens),
	}
}

func number(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func tightens(id *mc.ID) model.RuleID {
	if id == nil {
		return ""
	}
	return model.RuleID(*id)
}

func options(o canon.Object) map[string]any {
	m, ok := plain(o).(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	return m
}

// plain is a canon value as the plain Go value a binding holds: nil for none.
func plain(v canon.Value) any {
	switch x := v.(type) {
	case canon.Bool:
		return bool(x)
	case canon.Int:
		return int(x)
	case canon.Float:
		return float64(x)
	case canon.String:
		return string(x)
	case canon.Array:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plain(e)
		}
		return out
	case canon.Object:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = plain(e)
		}
		return out
	}
	return nil
}
