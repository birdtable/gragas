// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rule-map.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// RuleMap is Rule map toward one mapped rulebook (entity, spec 16.1).
type RuleMap struct {
	// Display only.
	// Since contract 1.0.
	Rulebook Text `json:"rulebook"`
	// Display only.
	// Since contract 1.0.
	Rows []RuleMapRows `json:"rows"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RuleMap.
func (v RuleMap) ToCanon() canon.Value {
	o := canon.Object{}
	o["rulebook"] = v.Rulebook.ToCanon()
	o["rows"] = listOf(v.Rows, RuleMapRows.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RuleMap has no
// field for, under its exact name, is refused.
func (v *RuleMap) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RuleMap) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rulebook", "rows")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rulebook", &v.Rulebook, readVia[Text]),
		field(o, at, "rows", &v.Rows, listRead(readVia[RuleMapRows])),
	)
}

// RuleMapRows is has the shape the schema gives it.
type RuleMapRows struct {
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Left *ID `json:"left,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Arrived *ID `json:"arrived,omitempty"`
	// Display only.
	// Since contract 1.0.
	Mark Mark `json:"mark"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Reason *Reason `json:"reason,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RuleMapRows.
func (v RuleMapRows) ToCanon() canon.Value {
	o := canon.Object{}
	keepPtr(o, "left", v.Left, ID.ToCanon)
	keepPtr(o, "arrived", v.Arrived, ID.ToCanon)
	o["mark"] = v.Mark.ToCanon()
	keepPtr(o, "reason", v.Reason, Reason.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RuleMapRows has no
// field for, under its exact name, is refused.
func (v *RuleMapRows) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RuleMapRows) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "left", "arrived", "mark", "reason")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "left", &v.Left, ptrRead(readVia[ID])),
		field(o, at, "arrived", &v.Arrived, ptrRead(readVia[ID])),
		field(o, at, "mark", &v.Mark, readVia[Mark]),
		field(o, at, "reason", &v.Reason, ptrRead(readVia[Reason])),
	)
}
