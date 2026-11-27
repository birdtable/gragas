package contract

import (
	"bytes"
	"errors"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// fixtureGrid is the fixture's grid document: all four results and the three
// run levels.
func fixtureGrid(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile(path.Join(fixtureDir, "grid.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestTheFixtureGridReadsToItsCellsAndEncodesToItsOwnBytes is the round-trip
// golden: read, then encode, gives the bytes read, and the cells hold all four
// results and every run level.
func TestTheFixtureGridReadsToItsCellsAndEncodesToItsOwnBytes(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	want := fixtureGrid(t)
	grid, err := ReadGrid(want, s)
	if err != nil {
		t.Fatal(err)
	}
	results, levels := map[model.Truth]bool{}, map[model.RunLevel]bool{}
	for _, c := range grid.Cells {
		results[c.Result], levels[c.Level] = true, true
	}
	if len(results) != 4 || len(levels) != len(model.RunLevels()) {
		t.Errorf("the fixture grid holds results %v and levels %v, want all four results and every level", results, levels)
	}
	got, err := EncodeGrid(grid)
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("EncodeGrid(ReadGrid(fixture)) = %v\n%s\nwant\n%s", err, got, want)
	}
}

// TestAGridHoldsNoTimeAndNoPath is S57, held on the fixture's bytes.
func TestAGridHoldsNoTimeAndNoPath(t *testing.T) {
	t.Parallel()
	b := bytes.ToLower(fixtureGrid(t))
	for _, banned := range []string{"time", "date", "path", "host", "/users", "\\"} {
		if bytes.Contains(b, []byte(banned)) {
			t.Errorf("the fixture grid holds %q", banned)
		}
	}
}

// draw is a seeded linear congruential generator: the same draws every run,
// and no random source for a lint to take for a secret.
type draw struct{ state uint64 }

// IntN is a number from 0 up to, not including, n.
func (d *draw) IntN(n int) int {
	d.state = d.state*6364136223846793005 + 1442695040888963407
	return int((d.state >> 33) % uint64(n))
}

// TestEncodeThenReadOfAnyGridIsTheIdentity is the property: for grids drawn at
// random, ReadGrid(EncodeGrid(cells)) is cells, and the bytes are stable.
func TestEncodeThenReadOfAnyGridIsTheIdentity(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	rng := &draw{state: 57}
	pick := func(from ...string) string { return from[rng.IntN(len(from))] }
	for i := range 300 {
		var cells []model.Cell
		for range rng.IntN(8) {
			c := model.Cell{
				Binding:     pick("a.b.1/t/x", "a.b.2/t/y", "é ü   \"q\" \\", "x#0123"),
				ToolVersion: pick("1.0.0", "none", "2.1.0-rc.1"),
				Result:      model.Truths()[rng.IntN(len(model.Truths()))],
				Rung:        rng.IntN(7),
				Level:       model.RunLevels()[rng.IntN(len(model.RunLevels()))],
				Adapter:     pick("", "1.0.0"),
				Inputs:      model.Digest(pick("", "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("0", 64))),
				Tool:        pick("", "lint", "ast-grep"),
			}
			if c.Result == model.TruthReviewed {
				c.Rung = 6
			}
			if c.Rung == 6 && c.Result != model.TruthNotRun {
				c.Result = model.TruthReviewed
			}
			if c.Result == model.TruthNotRun {
				c.Reason = model.Reasons()[rng.IntN(len(model.Reasons()))]
			}
			cells = append(cells, c)
		}
		grid := model.ProofGrid{Cells: cells}
		if rng.IntN(2) == 0 {
			grid.TestedSet = &model.Tested{MinEngine: pick("1.0.0", "2.3.4"), Tools: []model.Version{{Name: "lint", Version: "1.2.3"}}}
			if rng.IntN(2) == 0 {
				grid.TestedSet.Plugins = []model.Version{{Name: "p", Version: "1"}}
				grid.TestedSet.Kits = []model.Version{{Name: "k", Version: "2"}}
				grid.TestedSet.Adapters = []model.Version{{Name: "lint", Version: "1.0.0"}}
			}
		}
		first, err := EncodeGrid(grid)
		if err != nil {
			t.Fatalf("draw %d: EncodeGrid: %v", i, err)
		}
		got, err := ReadGrid(first, s)
		if err != nil {
			t.Fatalf("draw %d: ReadGrid of its own encoding: %v\n%s", i, err, first)
		}
		if len(cells) == 0 {
			got.Cells = nil
		}
		if !reflect.DeepEqual(got, grid) {
			t.Fatalf("draw %d: read %+v, want %+v", i, got, grid)
		}
		again, err := EncodeGrid(got)
		if err != nil || !bytes.Equal(again, first) {
			t.Fatalf("draw %d: the bytes are not stable: %v", i, err)
		}
	}
}

// TestAGridInANewerContractEndsNotReadyNamingBothVersions: a newer major and a
// newer minor of this major.
func TestAGridInANewerContractEndsNotReadyNamingBothVersions(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	for _, found := range []string{"2.0", "1.9"} {
		_, err := ReadGrid([]byte(`{"contract": "`+found+`", "cells": [], "later": 1}`), s)
		var nr *NotReady
		if !errors.As(err, &nr) {
			t.Fatalf("ReadGrid at %s = %v, want not ready", found, err)
		}
		if nr.Kind != Grid || nr.Found.String() != found || nr.Reads != Current() ||
			!strings.Contains(err.Error(), "contract "+found) || !strings.Contains(err.Error(), "contract "+Current().String()) {
			t.Errorf("not ready at %s: %+v, %q", found, nr, err)
		}
	}
}

// TestEncodeGridOfNoCellsIsTheEmptyGrid: the empty grid is a document, with an
// empty cells list.
func TestEncodeGridOfNoCellsIsTheEmptyGrid(t *testing.T) {
	t.Parallel()
	got, err := EncodeGrid(model.ProofGrid{})
	if err != nil || !bytes.Equal(got, golden(t, Grid)) {
		t.Errorf("EncodeGrid(model.ProofGrid{}) = %v\n%s", err, got)
	}
}

// FuzzReadGrid: whatever bytes come in, ReadGrid is a grid, a not-ready or a
// refusal, and a grid it accepts encodes to bytes it reads back to the same
// cells.
func FuzzReadGrid(f *testing.F) {
	files := schemaFiles(f)
	f.Add(fixtureGrid(f))
	f.Add(golden(f, Grid))
	f.Add([]byte(`{"contract": "1.0", "cells": [{"binding": "a", "tool_version": "1", "result": "pass", "Result": "fail"}]}`))
	f.Add([]byte(`{"contract": "1.0", "cells": [{"binding": "a", "tool_version": "1", "result": "reviewed", "rung": 6, "run_level": "pair"}]}`))
	f.Add([]byte(`{"contract": "1.0", "cells": [null]}`))
	f.Add([]byte(`{"contract": "2.0", "cells": 3}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, s := world(t, files)
		first, err := ReadGrid(data, s)
		if err != nil {
			var nr *NotReady
			var r *Refused
			if !errors.As(err, &nr) && !errors.As(err, &r) {
				t.Fatalf("ReadGrid failed: %v", err)
			}
			return
		}
		b1, err := EncodeGrid(first)
		if err != nil {
			t.Fatalf("EncodeGrid of an accepted grid: %v", err)
		}
		second, err := ReadGrid(b1, s)
		if err != nil {
			t.Fatalf("ReadGrid of its own encoding: %v\n%s", err, b1)
		}
		if len(first.Cells) == 0 && len(second.Cells) == 0 {
			first.Cells, second.Cells = nil, nil
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("the grid changed on the round trip:\n%+v\n%+v", first, second)
		}
	})
}
