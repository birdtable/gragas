package contract

import (
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	mc "github.com/birdtable/gragas/internal/model/contract"
)

// ReadGrid reads data, a proof grid from outside (supply.scans.5, S57), into
// its cells, in the order the document holds them, and its tested set. It reads in the order
// ReadBundle does, the size cap, canon's decode, the contract version and its
// schema, then the decode into the model type, and ends the same ways: a grid
// in a newer major, or in a newer minor of this major, is *NotReady naming both
// versions, and anything else wrong is *Refused. The grid holds no time and no
// host name or path (S57); this reads none. Whether the cells are never blank
// and one for each key is the proof context's to hold, not this reader's.
func ReadGrid(data []byte, s Schemas) (model.ProofGrid, error) {
	var doc mc.GridDocument
	obj, ver, findings, err := read(Grid, data, s)
	cur := Current()
	switch {
	case err != nil:
		return model.ProofGrid{}, err
	case ver.Major == cur.Major && ver.Minor > cur.Minor:
		return model.ProofGrid{}, &NotReady{Kind: Grid, Found: ver, Reads: cur}
	case len(findings) > 0:
		return model.ProofGrid{}, &Refused{Kind: Grid, Why: "it fails its contract", Findings: findings}
	}
	if err := strict(Grid, obj, &doc); err != nil {
		return model.ProofGrid{}, err
	}
	rows := doc.Cells
	cells := make([]model.Cell, 0, len(rows))
	for _, c := range rows {
		cells = append(cells, cellOf(c))
	}
	return model.ProofGrid{Cells: cells, TestedSet: testedOf(doc.TestedSet)}, nil
}

// orElse is *p, or d for an absent field: the schema's default, applied.
func orElse[T any](p *T, d T) T {
	if p == nil {
		return d
	}
	return *p
}

// EncodeGrid is the canonical bytes of a proof grid, its cells in the
// order given and its tested set where it has one: Encode of a grid document, so the same cells are always the same
// bytes. A cell is written whole, its level always and its reason only when it
// has one; nothing in it is a time or a path.
func EncodeGrid(g model.ProofGrid) ([]byte, error) {
	doc := mc.GridDocument{Cells: make([]mc.ProofResult, 0, len(g.Cells)), TestedSet: testedSetOf(g.TestedSet)}
	for _, c := range g.Cells {
		doc.Cells = append(doc.Cells, resultOf(c))
	}
	out, err := Encode(Grid, doc)
	if err != nil {
		return nil, fmt.Errorf("encode the proof grid: %w", err)
	}
	return out, nil
}

// cellOf is the model's cell for a generated proof result, the schema's
// defaults applied: a result with no level ran at the fixture level.
func cellOf(r mc.ProofResult) model.Cell {
	c := model.Cell{
		Binding:     string(r.Binding),
		ToolVersion: string(r.ToolVersion),
		Result:      model.Truth(r.Result),
		Rung:        int(r.Rung),
		Level:       model.RunLevel(orElse(r.RunLevel, mc.ProofResultRunLevel(model.RunFixture))),
		Adapter:     string(r.AdapterVersion),
		Reason:      model.Reason(orElse(r.Reason, "")),
		Inputs:      model.Digest(r.Inputs),
		Tool:        string(r.Tool),
	}
	return c
}

// resultOf is the generated proof result for a cell.
func resultOf(c model.Cell) mc.ProofResult {
	level := mc.ProofResultRunLevel(c.Level)
	r := mc.ProofResult{
		Binding:        mc.Text(c.Binding),
		ToolVersion:    mc.Text(c.ToolVersion),
		Result:         mc.Result(c.Result),
		Rung:           mc.Rung(c.Rung),
		RunLevel:       &level,
		AdapterVersion: mc.TextOrEmpty(c.Adapter),
		Inputs:         mc.DigestOrEmpty(c.Inputs),
		Tool:           mc.TextOrEmpty(c.Tool),
	}
	if c.Reason != "" {
		why := mc.ProofResultReason(c.Reason)
		r.Reason = &why
	}
	return r
}

// testedOf is the model's tested set for a generated one, or nil for none.
func testedOf(t *mc.TestedSet) *model.Tested {
	if t == nil {
		return nil
	}
	return &model.Tested{
		MinEngine: string(t.MinimumEngine),
		Tools: mapped(t.Tools, func(v mc.TestedSetTools) model.Version {
			return model.Version{Name: string(v.Tool), Version: string(v.Version)}
		}),
		Plugins: mapped(t.Plugins, func(v mc.TestedSetPlugins) model.Version {
			return model.Version{Name: string(v.Plugin), Version: string(v.Version)}
		}),
		Kits: mapped(t.Kits, func(v mc.TestedSetKits) model.Version {
			return model.Version{Name: string(v.Kit), Version: string(v.Version)}
		}),
		Adapters: mapped(t.Adapters, func(v mc.TestedSetAdapters) model.Version {
			return model.Version{Name: string(v.Adapter), Version: string(v.Version)}
		}),
	}
}

// mapped is f of each of in, in order, and nil for none: a copy of a list the
// tested set only displays, so no decision reads it.
func mapped[S, T any](in []S, f func(S) T) []T {
	var out []T
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

// testedSetOf is the generated tested set for a model one, or nil for none.
// Its tools list is always written, as the schema requires it, empty or not.
func testedSetOf(t *model.Tested) *mc.TestedSet {
	if t == nil {
		return nil
	}
	out := &mc.TestedSet{MinimumEngine: mc.Semver(t.MinEngine), Tools: []mc.TestedSetTools{}}
	for _, v := range t.Tools {
		out.Tools = append(out.Tools, mc.TestedSetTools{Tool: mc.Text(v.Name), Version: mc.Text(v.Version)})
	}
	for _, v := range t.Plugins {
		out.Plugins = append(out.Plugins, mc.TestedSetPlugins{Plugin: mc.Text(v.Name), Version: mc.Text(v.Version)})
	}
	for _, v := range t.Kits {
		out.Kits = append(out.Kits, mc.TestedSetKits{Kit: mc.Text(v.Name), Version: mc.Text(v.Version)})
	}
	for _, v := range t.Adapters {
		out.Adapters = append(out.Adapters, mc.TestedSetAdapters{Adapter: mc.Text(v.Name), Version: mc.Text(v.Version)})
	}
	return out
}
