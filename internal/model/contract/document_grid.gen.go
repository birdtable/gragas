// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-grid.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// GridDocument is The proof grid, binding by tool version (document, spec S57).
type GridDocument struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	Cells []ProofResult `json:"cells"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	TestedSet *TestedSet `json:"tested_set,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote GridDocument.
func (v GridDocument) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["cells"] = listOf(v.Cells, ProofResult.ToCanon)
	keepPtr(o, "tested_set", v.TestedSet, TestedSet.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member GridDocument has no
// field for, under its exact name, is refused.
func (v *GridDocument) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *GridDocument) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "cells", "tested_set")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "cells", &v.Cells, listRead(readVia[ProofResult])),
		field(o, at, "tested_set", &v.TestedSet, ptrRead(readVia[TestedSet])),
	)
}
