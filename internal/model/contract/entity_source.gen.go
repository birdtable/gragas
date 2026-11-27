// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-source.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Source is Source (entity, spec 16.1).
type Source struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Kind SourceKind `json:"kind"`
	// Display only.
	// Since contract 1.0.
	Title Text `json:"title"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Locator TextOrEmpty `json:"locator,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Attribution TextOrEmpty `json:"attribution,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Source.
func (v Source) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["kind"] = v.Kind.ToCanon()
	o["title"] = v.Title.ToCanon()
	keep(o, "locator", v.Locator.ToCanon(), v.Locator != "")
	keep(o, "attribution", v.Attribution.ToCanon(), v.Attribution != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Source has no
// field for, under its exact name, is refused.
func (v *Source) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Source) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "kind", "title", "locator", "attribution")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "kind", &v.Kind, readVia[SourceKind]),
		field(o, at, "title", &v.Title, readVia[Text]),
		field(o, at, "locator", &v.Locator, readVia[TextOrEmpty]),
		field(o, at, "attribution", &v.Attribution, readVia[TextOrEmpty]),
	)
}

// SourceKind is has the shape the schema gives it.
type SourceKind string

// The values of SourceKind.
const (
	SourceKindStandard       SourceKind = "standard"
	SourceKindGuide          SourceKind = "guide"
	SourceKindStudy          SourceKind = "study"
	SourceKindIncident       SourceKind = "incident"
	SourceKindDecisionRecord SourceKind = "decision record"
	SourceKindVendorDocument SourceKind = "vendor document"
	SourceKindOwnDecision    SourceKind = "own decision"
)

// ToCanon is v as canon's value tree, as encoding/json wrote SourceKind.
func (v SourceKind) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member SourceKind has no
// field for, under its exact name, is refused.
func (v *SourceKind) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SourceKind) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = SourceKind(x)
	return err
}
