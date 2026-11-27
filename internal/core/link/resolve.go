package link

import (
	"errors"
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// Resolve is the effective rulebook of a chain with provenance (S3, S8, A18):
// the layers are given as values, the root first, and the book is the last
// layer. It reads no file, calls no port and fetches nothing.
//
// A layer sets the entries of its own rules. An overlay's binding that
// tightens a parent's rule (Binding.Tightens) replaces the parent's entry of
// the same tool, target and key, and the entry reads "changed" by that layer;
// where the parent holds no such entry the binding is added as changed. A
// layer's record in bindings/ of a rule it borrowed, from an earlier layer or
// an import, does the same for its subject (S34, S70); a record that cannot be
// read, or of a rule the layer did not borrow, is an error naming the layer.
// An overlay's standing exclusion of a parent's rule marks each of its entries
// "excluded" with the decision, and they stay in the listing. An exclusion of
// a rule no earlier layer holds is an error naming the layer, and the entries
// beside it are for the report only.
func Resolve(book model.Book, parents []model.ChainLayer) (model.Resolved, error) {
	layers := append(slices.Clone(parents), model.ChainLayer{Name: book.Identity.Name, Book: book})
	out := model.Resolved{Entries: []model.ResolvedEntry{}}
	var refused []error
	held := map[model.RuleID]bool{} // the rules of the layers before this one
	for _, l := range layers {
		out.Layers = append(out.Layers, l.Name)
		setEntries(&out, l)
		refused = append(refused, records(&out, l, held)...)
		for _, e := range l.Book.Exclusions {
			if e.Lifted != nil || e.Overlay != l.Name {
				continue
			}
			// A rule with no binding has no entry to mark, but an earlier layer
			// still holds it.
			if !exclude(&out, e, l.Name) && !held[e.Rule] {
				refused = append(refused, fmt.Errorf("layer %s: its exclusion %d names rule %s, which no earlier layer holds", l.Name, e.Number, e.Rule))
			}
		}
		for _, r := range l.Book.Rules {
			held[r.ID] = true
		}
	}
	return out, errors.Join(refused...)
}

// setEntries adds the entries of the layer's own rules: each binding sets its
// entry, and one that tightens a parent's replaces it.
func setEntries(out *model.Resolved, l model.ChainLayer) {
	for _, r := range l.Book.Rules {
		for _, b := range r.Bindings {
			if b.Tightens != "" && change(out, b, l.Name, r.ID) {
				continue
			}
			how := model.ResolvedSet
			if b.Tightens != "" {
				how = model.ResolvedChanged
			}
			out.Entries = append(out.Entries, model.ResolvedEntry{Rule: r.ID, Binding: b, Layer: l.Name, How: how})
		}
	}
}

// change replaces the entry the binding tightens and says whether it found
// one.
func change(out *model.Resolved, b model.Binding, layer string, by model.RuleID) bool {
	for i, e := range out.Entries {
		if e.Rule == b.Tightens && sameKey(e.Binding, b) && e.How != model.ResolvedExcluded {
			out.Entries[i] = model.ResolvedEntry{Rule: by, Binding: b, Layer: layer, How: model.ResolvedChanged}
			return true
		}
	}
	return false
}

// exclude marks every entry of the rule and says whether the chain held it.
func exclude(out *model.Resolved, x model.Exclusion, layer string) bool {
	found := false
	for i, e := range out.Entries {
		if e.Rule == x.Rule && e.Layer != layer {
			out.Entries[i].How, out.Entries[i].Decision, found = model.ResolvedExcluded, x.Decision, true
		}
	}
	return found
}

// sameKey says whether two bindings name one tool entry.
func sameKey(a, b model.Binding) bool {
	return a.Tool == b.Tool && a.Target == b.Target && a.Key == b.Key
}
