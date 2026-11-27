// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-proposal.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Proposal is Proposal (entity, spec 16.1).
type Proposal struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Target Text `json:"target"`
	// Display only.
	// Since contract 1.0.
	Change ProposalChange `json:"change"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Evidence []ProposalEvidence `json:"evidence,omitempty"`
	// Display only.
	// Since contract 1.0.
	State ProposalState `json:"state"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Link *URL `json:"link,omitempty"`
	// Display only.
	// Since contract 1.0.
	Opened Date `json:"opened"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Proposal.
func (v Proposal) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["target"] = v.Target.ToCanon()
	o["change"] = v.Change.ToCanon()
	keep(o, "evidence", listOf(v.Evidence, ProposalEvidence.ToCanon), len(v.Evidence) > 0)
	o["state"] = v.State.ToCanon()
	keepPtr(o, "link", v.Link, URL.ToCanon)
	o["opened"] = v.Opened.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Proposal has no
// field for, under its exact name, is refused.
func (v *Proposal) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Proposal) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "target", "change", "evidence", "state", "link", "opened")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "target", &v.Target, readVia[Text]),
		field(o, at, "change", &v.Change, readVia[ProposalChange]),
		field(o, at, "evidence", &v.Evidence, listRead(readVia[ProposalEvidence])),
		field(o, at, "state", &v.State, readVia[ProposalState]),
		field(o, at, "link", &v.Link, ptrRead(readVia[URL])),
		field(o, at, "opened", &v.Opened, readVia[Date]),
	)
}

// ProposalChange is has the shape the schema gives it.
type ProposalChange struct {
	// Display only.
	// Since contract 1.0.
	Anatomy Text `json:"anatomy"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Pair TextOrEmpty `json:"pair,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Binding TextOrEmpty `json:"binding,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Sources []Slug `json:"sources,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ProposalChange.
func (v ProposalChange) ToCanon() canon.Value {
	o := canon.Object{}
	o["anatomy"] = v.Anatomy.ToCanon()
	keep(o, "pair", v.Pair.ToCanon(), v.Pair != "")
	keep(o, "binding", v.Binding.ToCanon(), v.Binding != "")
	keep(o, "sources", listOf(v.Sources, Slug.ToCanon), len(v.Sources) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ProposalChange has no
// field for, under its exact name, is refused.
func (v *ProposalChange) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProposalChange) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "anatomy", "pair", "binding", "sources")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "anatomy", &v.Anatomy, readVia[Text]),
		field(o, at, "pair", &v.Pair, readVia[TextOrEmpty]),
		field(o, at, "binding", &v.Binding, readVia[TextOrEmpty]),
		field(o, at, "sources", &v.Sources, listRead(readVia[Slug])),
	)
}

// ProposalEvidenceKind is has the shape the schema gives it.
type ProposalEvidenceKind string

// The values of ProposalEvidenceKind.
const (
	ProposalEvidenceKindWaiverPattern   ProposalEvidenceKind = "waiver-pattern"
	ProposalEvidenceKindIncidentExample ProposalEvidenceKind = "incident-example"
	ProposalEvidenceKindAgentMiss       ProposalEvidenceKind = "agent-miss"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ProposalEvidenceKind.
func (v ProposalEvidenceKind) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ProposalEvidenceKind has no
// field for, under its exact name, is refused.
func (v *ProposalEvidenceKind) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProposalEvidenceKind) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ProposalEvidenceKind(x)
	return err
}

// ProposalEvidence is has the shape the schema gives it.
type ProposalEvidence struct {
	// Display only.
	// Since contract 1.0.
	Kind ProposalEvidenceKind `json:"kind"`
	// Display only.
	// Since contract 1.0.
	Text Text `json:"text"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ProposalEvidence.
func (v ProposalEvidence) ToCanon() canon.Value {
	o := canon.Object{}
	o["kind"] = v.Kind.ToCanon()
	o["text"] = v.Text.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ProposalEvidence has no
// field for, under its exact name, is refused.
func (v *ProposalEvidence) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProposalEvidence) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "kind", "text")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "kind", &v.Kind, readVia[ProposalEvidenceKind]),
		field(o, at, "text", &v.Text, readVia[Text]),
	)
}

// ProposalState is has the shape the schema gives it.
type ProposalState string

// The values of ProposalState.
const (
	ProposalStateOpen      ProposalState = "open"
	ProposalStateAccepted  ProposalState = "accepted"
	ProposalStateDeclined  ProposalState = "declined"
	ProposalStateWithdrawn ProposalState = "withdrawn"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ProposalState.
func (v ProposalState) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ProposalState has no
// field for, under its exact name, is refused.
func (v *ProposalState) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProposalState) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ProposalState(x)
	return err
}
