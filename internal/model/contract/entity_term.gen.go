// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-term.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Term is Term (entity, spec 16.1).
type Term struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Definition Text `json:"definition"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Literal TextOrEmpty `json:"literal,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Translations []Translation `json:"translations,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Term.
func (v Term) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["definition"] = v.Definition.ToCanon()
	keep(o, "literal", v.Literal.ToCanon(), v.Literal != "")
	keep(o, "translations", listOf(v.Translations, Translation.ToCanon), len(v.Translations) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Term has no
// field for, under its exact name, is refused.
func (v *Term) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Term) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "definition", "literal", "translations")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "definition", &v.Definition, readVia[Text]),
		field(o, at, "literal", &v.Literal, readVia[TextOrEmpty]),
		field(o, at, "translations", &v.Translations, listRead(readVia[Translation])),
	)
}
