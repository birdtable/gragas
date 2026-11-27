// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-proposal.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// ProposalDocument is A proposal as a document (spec 16.1).
type ProposalDocument struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	Proposal Proposal `json:"proposal"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ProposalDocument.
func (v ProposalDocument) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["proposal"] = v.Proposal.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ProposalDocument has no
// field for, under its exact name, is refused.
func (v *ProposalDocument) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProposalDocument) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "proposal")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "proposal", &v.Proposal, readVia[Proposal]),
	)
}
