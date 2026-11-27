package coverage

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestTheLeadIsTwoTablesOfRulesAndEntries: a rule's
// entries, an entry's tool and unit, and the rule ids that hold it are several
// outside values in one cell. Term escapes every field alike, so no column
// carries a flag.
func TestTheLeadIsTwoTablesOfRulesAndEntries(t *testing.T) {
	t.Parallel()
	c := model.Coverage{
		Rules:   []model.CoverRule{{Rule: "go.errors.1", Entries: []string{"fake-tool a: b", "fake-tool c: d"}}},
		Entries: []model.CoverEntry{{Tool: "fake-tool", Entry: "errcheck", Rules: []model.RuleID{"go.errors.1", "go.errors.2"}}},
	}
	var titles [][]string
	for _, b := range lead(c) {
		if tbl, ok := b.(model.Table); ok {
			var row []string
			for _, col := range tbl.Columns {
				row = append(row, col.Title)
			}
			titles = append(titles, row)
		}
	}
	want := [][]string{{"rule", "standing", "entries"}, {"entry", "held by"}}
	if !slices.EqualFunc(titles, want, slices.Equal[[]string]) {
		t.Errorf("tables with columns %q, want %q", titles, want)
	}
}
