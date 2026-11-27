package coreapi_test

import (
	"os"
	"path"
	"testing"

	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const gridSchemas = "../../host/assets/contract/schema/v1"

// gridWorld is a world whose rulebook folder holds files, with the contract's
// schemas for the grid seeded, and the contract's grid reader over it.
func gridWorld(t *testing.T, files map[string]string) (proof.Decode, *porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	assets := map[string]string{}
	entries, err := os.ReadDir(gridSchemas)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(path.Join(gridSchemas, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		assets[contract.SchemaDir+"/v1/"+e.Name()] = string(b)
	}
	w.SeedSchemas("v1/document-grid.schema.json")
	w.SeedAssets(assets)
	p := w.Ports()
	s := contract.Schemas{Check: p.Schema, Files: p.Assets}
	root := w.Seed("repo", files)
	return func(data []byte) (model.ProofGrid, error) { return contract.ReadGrid(data, s) }, w, root
}

// TestTheProofGridFileDecidesTheStandingOfARule: the grid read from proof/grid.json
// is what link.Derive reads. A passing cell makes the rule enforced, a failing
// cell marks it unproven, and no grid file leaves it planned.
func TestTheProofGridFileDecidesTheStandingOfARule(t *testing.T) {
	t.Parallel()
	b := model.Binding{Tool: "lint", Key: "k", Rung: 3, Pair: 1}
	rule := model.Rule{ID: "a.b.1", Status: model.Active, Strength: model.Must, Bindings: []model.Binding{b}, Pairs: []model.Pair{{Number: 1}}}
	book := model.Book{Rules: []model.Rule{rule}}
	text := proof.BindingText(b.Ref(rule.ID))
	for name, tc := range map[string]struct {
		result model.Truth
		file   bool
		review bool
		want   string
	}{
		"no file":                 {file: false, want: "planned, unproven"},
		"pass":                    {model.TruthPass, true, false, "enforced"},
		"fail":                    {model.TruthFail, true, false, "planned, unproven"},
		"a rung-6 review besides": {model.TruthPass, true, true, "planned, unproven"},
	} {
		files := map[string]string{"rulebook.yaml": "x"}
		decode, w, root := gridWorld(t, files)
		if tc.file {
			cell := model.Cell{Binding: text, ToolVersion: "1.0.0", Result: tc.result, Rung: 3, Level: model.RunPair}
			grid := []model.Cell{cell}
			if tc.review {
				grid = append(grid, model.Cell{Binding: text, ToolVersion: "2.0.0", Result: model.TruthReviewed, Rung: 6, Level: model.RunPair})
			}
			data, err := contract.EncodeGrid(model.ProofGrid{Cells: grid})
			if err != nil {
				t.Fatal(err)
			}
			root = w.Seed("repo2", map[string]string{"rulebook.yaml": "x", proof.Path: string(data)})
		}
		grid, err := proof.Read(w.Ports().Tree, root, decode)
		if err != nil {
			t.Fatalf("%s: Read: %v", name, err)
		}
		cells := grid.Cells
		held, err := link.Derive(book, proof.Truths(book, cells), model.Chain{}) // a book with one rule holds no conflict
		if err != nil || len(held) != 1 || held[0].String() != tc.want {
			t.Errorf("%s: Derive = %v, %v, want %q", name, held, err, tc.want)
		}
	}
}
