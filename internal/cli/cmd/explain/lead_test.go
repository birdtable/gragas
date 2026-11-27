package explain

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestTheLeadIsATableWithTheEntryColumn: a tool, a
// target and a key share the entry cell, as the first column of the table.
// Term escapes every field alike, so the column carries no flag.
func TestTheLeadIsATableWithTheEntryColumn(t *testing.T) {
	t.Parallel()
	e := model.Explained{Rules: []model.ExplainedRule{{Rule: "go.errors.1", Bindings: []model.ExplainedBinding{{Tool: "fake-tool", Key: "errcheck"}}}}}
	for _, b := range lead(e) {
		if tbl, ok := b.(model.Table); ok {
			if tbl.Columns[0].Title != "entry" {
				t.Errorf("first column %+v is not the entry column", tbl.Columns[0])
			}
			return
		}
	}
	t.Fatal("no table")
}
