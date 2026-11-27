package coreapi

import (
	"github.com/birdtable/gragas/internal/core/generate"
	"github.com/birdtable/gragas/internal/core/rulebook"
	"github.com/birdtable/gragas/internal/ordered"
)

// book is the rulebook domain's book as generate reads it, through the
// interface generate declares: the one place the two domains meet
// (arch.ports.8), so neither imports the other.
type book struct{ b *rulebook.Book }

func (x book) Root() string { return rulebook.Dir }

func (x book) Parts() []generate.Part {
	out := make([]generate.Part, len(x.b.Parts.List))
	for i, p := range x.b.Parts.List {
		out[i] = generate.Part{Short: p.Short, Layer: p.Layer, Name: p.Name, Description: p.Description,
			LayerName: x.b.Parts.LayerName(p.Layer), BuildsOn: p.BuildsOn, Data: p.Data}
	}
	return out
}

func (x book) Concerns(part string) []generate.Concern {
	var out []generate.Concern
	for _, f := range x.b.Files {
		if f.Part == part {
			out = append(out, generate.Concern{Name: f.Concern, Order: f.Order(), Doc: f.Doc})
		}
	}
	return out
}

func (x book) Tools(part string) []*ordered.Map { return x.b.Tools[part] }

func (x book) Registry() generate.Registry {
	return generate.Registry{Kinds: x.b.Kinds, Sources: x.b.Sources}
}

func (x book) HasGuide(part string) bool { return x.b.Guides[part] }

func (x book) Rules() []generate.Rule {
	ids := x.b.IDs()
	out := make([]generate.Rule, len(ids))
	for i, id := range ids {
		r := x.b.Rules[id]
		out[i] = generate.Rule{ID: id, Part: r.Part, Concern: r.Concern, Data: r.Data}
	}
	return out
}

func (x book) Standing(id string) string { return x.b.Standing(id) }

func (x book) Check(check *ordered.Map) generate.CheckState {
	return generate.CheckState{Resolves: x.b.Resolves(check), Holds: x.b.Holds(check)}
}

func (x book) LockIDs() []string { return x.b.LockIDs() }
