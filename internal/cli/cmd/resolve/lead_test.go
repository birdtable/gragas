package resolve

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestTheLeadIsATableOfEntryRuleLayerAndHow: a
// tool, a target and a key share the entry cell, and the how cell is the word
// and its decision. Term escapes every field alike, so no column carries a flag.
func TestTheLeadIsATableOfEntryRuleLayerAndHow(t *testing.T) {
	t.Parallel()
	r := model.Resolved{Layers: []string{"root"}, Entries: []model.ResolvedEntry{{Rule: "go.errors.1", Layer: "root", Decision: "D-1", Binding: model.Binding{Tool: "fake-tool", Key: "errcheck"}}}}
	for _, b := range lead(r) {
		if tbl, ok := b.(model.Table); ok {
			var titles []string
			for _, c := range tbl.Columns {
				titles = append(titles, c.Title)
			}
			if want := []string{"entry", "rule", "layer", "how"}; !slices.Equal(titles, want) {
				t.Errorf("columns %q, want %q", titles, want)
			}
			return
		}
	}
	t.Fatal("no table")
}
