package link_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

func bnd(key string, v any) model.Binding {
	return model.Binding{Tool: "t", Target: "config", Key: key, Value: v, Rung: 1}
}

func layer(name string, rules ...model.Rule) model.ChainLayer {
	return model.ChainLayer{Name: name, Book: model.Book{Identity: model.RulebookIdentity{Name: name}, Rules: rules}}
}

// TestResolveGivesEachEntryItsLayerAndWhatItDid: a parent's entry is set by the
// parent, an overlay's tightening is changed by the overlay, an exclusion
// stays listed with its decision, and an own rule is set by the book.
func TestResolveGivesEachEntryItsLayerAndWhatItDid(t *testing.T) {
	t.Parallel()
	parent := layer("base",
		model.Rule{ID: "a.b.1", Bindings: []model.Binding{bnd("k1", 1)}},
		model.Rule{ID: "a.b.2", Bindings: []model.Binding{bnd("k2", 1)}},
		model.Rule{ID: "a.b.3", Bindings: []model.Binding{bnd("k3", 1)}})
	tight := bnd("k1", 2)
	tight.Tightens = "a.b.1"
	book := layer("over", model.Rule{ID: "o.b.1", Bindings: []model.Binding{tight, bnd("own", 1)}}).Book
	book.Exclusions = []model.Exclusion{{Overlay: "over", Rule: "a.b.2", Number: 1, Decision: "drop-two"}}
	got, err := link.Resolve(book, []model.ChainLayer{parent})
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		rule, layer, how, decision string
	}
	var rows []row
	for _, e := range got.Entries {
		rows = append(rows, row{string(e.Rule), e.Layer, string(e.How), e.Decision})
	}
	want := []row{
		{"o.b.1", "over", "changed", ""},
		{"a.b.2", "base", "excluded", "drop-two"},
		{"a.b.3", "base", "set", ""},
		{"o.b.1", "over", "set", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %v, want %v", i, rows[i], want[i])
		}
	}
	if strings.Join(got.Layers, ",") != "base,over" {
		t.Errorf("layers = %v", got.Layers)
	}
}

// TestResolveOfOneLayerListsThatLayer: no parents, the book's own entries.
func TestResolveOfOneLayerListsThatLayer(t *testing.T) {
	t.Parallel()
	book := layer("solo", model.Rule{ID: "a.b.1", Bindings: []model.Binding{bnd("k", 1)}}).Book
	got, err := link.Resolve(book, nil)
	if err != nil || len(got.Entries) != 1 || got.Entries[0].How != model.ResolvedSet {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

// TestAnExclusionOfARuleNoLayerHoldsNamesTheLayer: the plant for the refusal.
func TestAnExclusionOfARuleNoLayerHoldsNamesTheLayer(t *testing.T) {
	t.Parallel()
	book := layer("over").Book
	book.Exclusions = []model.Exclusion{{Overlay: "over", Rule: "x.y.9", Number: 3, Decision: "d"}}
	_, err := link.Resolve(book, []model.ChainLayer{layer("base")})
	if err == nil || !strings.Contains(err.Error(), "layer over") || !strings.Contains(err.Error(), "x.y.9") {
		t.Errorf("err = %v", err)
	}
}

// TestALiftedExclusionDropsNothing.
func TestALiftedExclusionDropsNothing(t *testing.T) {
	t.Parallel()
	book := layer("over").Book
	book.Exclusions = []model.Exclusion{{Overlay: "over", Rule: "a.b.1", Number: 1, Decision: "d", Lifted: &model.Lifted{Decision: "e"}}}
	got, err := link.Resolve(book, []model.ChainLayer{layer("base", model.Rule{ID: "a.b.1", Bindings: []model.Binding{bnd("k", 1)}})})
	if err != nil || got.Entries[0].How != model.ResolvedSet {
		t.Errorf("got %+v, err %v", got, err)
	}
}
