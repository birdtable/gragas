// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-plan.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// PlanDocument is A plan as a document (spec 15.3).
type PlanDocument struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	Plan Plan `json:"plan"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote PlanDocument.
func (v PlanDocument) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["plan"] = v.Plan.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member PlanDocument has no
// field for, under its exact name, is refused.
func (v *PlanDocument) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanDocument) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "plan")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "plan", &v.Plan, readVia[Plan]),
	)
}
