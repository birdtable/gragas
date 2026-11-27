// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-exclusion.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Exclusion is Exclusion of a parent rule by an overlay (entity, spec 16.1).
type Exclusion struct {
	// Display only.
	// Since contract 1.0.
	Overlay Slug `json:"overlay"`
	// Display only.
	// Since contract 1.0.
	Rule ID `json:"rule"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 1.
	Number int `json:"number"`
	// Display only.
	// Since contract 1.0.
	Decision Slug `json:"decision"`
	// Display only.
	// Since contract 1.0.
	Who Text `json:"who"`
	// Display only.
	// Since contract 1.0.
	When Timestamp `json:"when"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Lifted *ExclusionLifted `json:"lifted,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Exclusion.
func (v Exclusion) ToCanon() canon.Value {
	o := canon.Object{}
	o["overlay"] = v.Overlay.ToCanon()
	o["rule"] = v.Rule.ToCanon()
	o["number"] = wholeOf(v.Number)
	o["decision"] = v.Decision.ToCanon()
	o["who"] = v.Who.ToCanon()
	o["when"] = v.When.ToCanon()
	keepPtr(o, "lifted", v.Lifted, ExclusionLifted.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Exclusion has no
// field for, under its exact name, is refused.
func (v *Exclusion) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Exclusion) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "overlay", "rule", "number", "decision", "who", "when", "lifted")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "overlay", &v.Overlay, readVia[Slug]),
		field(o, at, "rule", &v.Rule, readVia[ID]),
		field(o, at, "number", &v.Number, readWhole),
		field(o, at, "decision", &v.Decision, readVia[Slug]),
		field(o, at, "who", &v.Who, readVia[Text]),
		field(o, at, "when", &v.When, readVia[Timestamp]),
		field(o, at, "lifted", &v.Lifted, ptrRead(readVia[ExclusionLifted])),
	)
}

// ExclusionLifted is has the shape the schema gives it.
type ExclusionLifted struct {
	// Display only.
	// Since contract 1.0.
	Decision Slug `json:"decision"`
	// Display only.
	// Since contract 1.0.
	Who Text `json:"who"`
	// Display only.
	// Since contract 1.0.
	When Timestamp `json:"when"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ExclusionLifted.
func (v ExclusionLifted) ToCanon() canon.Value {
	o := canon.Object{}
	o["decision"] = v.Decision.ToCanon()
	o["who"] = v.Who.ToCanon()
	o["when"] = v.When.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ExclusionLifted has no
// field for, under its exact name, is refused.
func (v *ExclusionLifted) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ExclusionLifted) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "decision", "who", "when")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "decision", &v.Decision, readVia[Slug]),
		field(o, at, "who", &v.Who, readVia[Text]),
		field(o, at, "when", &v.When, readVia[Timestamp]),
	)
}
