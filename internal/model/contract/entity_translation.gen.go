// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-translation.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Translation is Translation of one field of one entity (entity, spec 16.1).
type Translation struct {
	// Display only.
	// Since contract 1.0.
	Entity Text `json:"entity"`
	// Display only.
	// Since contract 1.0.
	Field Slug `json:"field"`
	// Display only.
	// Since contract 1.0.
	Language Language `json:"language"`
	// Display only.
	// Since contract 1.0.
	Text Text `json:"text"`
	// Display only.
	// Since contract 1.0.
	AuthoritativeHash Digest `json:"authoritative_hash"`
	// Display only.
	// Since contract 1.0.
	Stale bool `json:"stale"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Translation.
func (v Translation) ToCanon() canon.Value {
	o := canon.Object{}
	o["entity"] = v.Entity.ToCanon()
	o["field"] = v.Field.ToCanon()
	o["language"] = v.Language.ToCanon()
	o["text"] = v.Text.ToCanon()
	o["authoritative_hash"] = v.AuthoritativeHash.ToCanon()
	o["stale"] = truthOf(v.Stale)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Translation has no
// field for, under its exact name, is refused.
func (v *Translation) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Translation) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "entity", "field", "language", "text", "authoritative_hash", "stale")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "entity", &v.Entity, readVia[Text]),
		field(o, at, "field", &v.Field, readVia[Slug]),
		field(o, at, "language", &v.Language, readVia[Language]),
		field(o, at, "text", &v.Text, readVia[Text]),
		field(o, at, "authoritative_hash", &v.AuthoritativeHash, readVia[Digest]),
		field(o, at, "stale", &v.Stale, readTruth),
	)
}
