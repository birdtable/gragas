package term

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestTheNarrowCheckMeasuresTablesAsTableDoes: the gap between columns and the
// least a last column is given, as the journeys' narrow-width check reads them
// from its testdata to bound a dropped column, are Table's own. Only
// those two are held here; the drop orders in the same file are each command's,
// and the narrow check's own runs hold them. Risk: Table's measures move, the
// file does not, and the check lets a column go that still fits.
func TestTheNarrowCheckMeasuresTablesAsTableDoes(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", "journeys", "testdata", "table-measures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Gap     *int            `json:"gap"`
		MinLast *int            `json:"minLast"`
		Orders  json.RawMessage `json:"orders"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		t.Fatalf("the journeys' table-measures.json: %v", err)
	}
	if m.Gap == nil || *m.Gap != gap {
		t.Errorf("the journeys' table-measures.json gives the gap as %s; Table's is %d", shown(m.Gap), gap)
	}
	if m.MinLast == nil || *m.MinLast != minLast {
		t.Errorf("the journeys' table-measures.json gives the last column's least as %s; Table's is %d", shown(m.MinLast), minLast)
	}
}

// shown is a measure read from the file, or that it is missing.
func shown(n *int) string {
	if n == nil {
		return "missing"
	}
	return strconv.Itoa(*n)
}
