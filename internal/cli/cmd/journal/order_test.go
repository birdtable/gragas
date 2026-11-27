package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// dropOrder is the order term.Look.Table leaves columns out in when the width
// is short, read from the priorities the columns carry: the lowest first, the
// later of equals first. The first column is in no order, since the last one
// standing wraps and is never left out.
func dropOrder(cols []string, priority []int) []string {
	idx := make([]int, 0, len(cols)-1)
	for i := 1; i < len(cols); i++ {
		idx = append(idx, i)
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if priority[idx[a]] != priority[idx[b]] {
			return priority[idx[a]] < priority[idx[b]]
		}
		return idx[a] > idx[b]
	})
	out := make([]string, len(idx))
	for i, c := range idx {
		out[i] = cols[c]
	}
	return out
}

// TestTheJournalListDropsColumnsInTheOrderTheMeasuresFileLists: the drop order
// the journeys' testdata/table-measures.json lists for the journal list's
// table, tail included, is the one the priorities journal.go gives its
// columns make. The narrow runs exercise only the prefix, so without this a
// swap of two priorities in runTable moves a column out of the order and
// nothing reads it. Risk: journal.go's priorities move, the file does not, and
// the narrow check bounds a column that no longer goes when it says it does.
func TestTheJournalListDropsColumnsInTheOrderTheMeasuresFileLists(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "journeys", "testdata", "table-measures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Orders []struct {
			Titles []string `json:"titles"`
			Order  []string `json:"order"`
		} `json:"orders"`
	}
	// The other fields of the file are term's, held in internal/host/term.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("the journeys' table-measures.json: %v", err)
	}
	if err := json.Unmarshal(raw["orders"], &m.Orders); err != nil {
		t.Fatalf("the journeys' table-measures.json: orders: %v", err)
	}

	table := runTable(nil)
	titles := make([]string, len(table.Columns))
	priority := make([]int, len(table.Columns))
	for i, c := range table.Columns {
		titles[i], priority[i] = c.Title, c.Priority
	}
	// The order leaves the first column out because it is the last to go: no
	// other column may have a higher priority.
	for i := 1; i < len(priority); i++ {
		if priority[0] < priority[i] {
			t.Errorf("journal.go gives column %q priority %d, below column %q's %d; the listed orders leave the first column out as the last to go", titles[0], priority[0], titles[i], priority[i])
		}
	}
	held := 0
	for _, o := range m.Orders {
		if !slices.Equal(o.Titles, titles) {
			continue // another command's table, held where it is built
		}
		held++
		if want := dropOrder(titles, priority); !slices.Equal(o.Order, want) {
			t.Errorf("table-measures.json lists the journal list's drop order as %v; journal.go's priorities %v make it %v", o.Order, priority, want)
		}
	}
	if held != 1 {
		t.Errorf("table-measures.json lists %d orders for the journal list's columns %v; want one", held, titles)
	}
}
