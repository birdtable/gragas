// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-finding.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Finding is Finding or notice (entity, spec 16.1).
type Finding struct {
	// Display only.
	// Since contract 1.0.
	Kind Slug `json:"kind"`
	// Display only.
	// Since contract 1.0.
	Entity Text `json:"entity"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Location TextOrEmpty `json:"location,omitempty"`
	// Display only.
	// Since contract 1.0.
	Message Text `json:"message"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Fix TextOrEmpty `json:"fix,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Rule *ID `json:"rule,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means false.
	Refusal bool `json:"refusal,omitempty"`
	// a notice never changes the closing word.
	// Display only.
	// Since contract 1.0.
	// Absent means false.
	Notice bool `json:"notice,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Finding.
func (v Finding) ToCanon() canon.Value {
	o := canon.Object{}
	o["kind"] = v.Kind.ToCanon()
	o["entity"] = v.Entity.ToCanon()
	keep(o, "location", v.Location.ToCanon(), v.Location != "")
	o["message"] = v.Message.ToCanon()
	keep(o, "fix", v.Fix.ToCanon(), v.Fix != "")
	keepPtr(o, "rule", v.Rule, ID.ToCanon)
	keep(o, "refusal", truthOf(v.Refusal), v.Refusal)
	keep(o, "notice", truthOf(v.Notice), v.Notice)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Finding has no
// field for, under its exact name, is refused.
func (v *Finding) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Finding) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "kind", "entity", "location", "message", "fix", "rule", "refusal", "notice")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "kind", &v.Kind, readVia[Slug]),
		field(o, at, "entity", &v.Entity, readVia[Text]),
		field(o, at, "location", &v.Location, readVia[TextOrEmpty]),
		field(o, at, "message", &v.Message, readVia[Text]),
		field(o, at, "fix", &v.Fix, readVia[TextOrEmpty]),
		field(o, at, "rule", &v.Rule, ptrRead(readVia[ID])),
		field(o, at, "refusal", &v.Refusal, readTruth),
		field(o, at, "notice", &v.Notice, readTruth),
	)
}
