// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-waiver.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Waiver is Waiver as a report counts it (entity, spec 16.6): never the path.
type Waiver struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Rule ID `json:"rule"`
	// Display only.
	// Since contract 1.0.
	Kind WaiverKind `json:"kind"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 0.
	AgeDays int `json:"age_days"`
	// Display only.
	// Since contract 1.0.
	Expires Date `json:"expires"`
	// Display only.
	// Since contract 1.0.
	OwnerPresent bool `json:"owner_present"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Waiver.
func (v Waiver) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["rule"] = v.Rule.ToCanon()
	o["kind"] = v.Kind.ToCanon()
	o["age_days"] = wholeOf(v.AgeDays)
	o["expires"] = v.Expires.ToCanon()
	o["owner_present"] = truthOf(v.OwnerPresent)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Waiver has no
// field for, under its exact name, is refused.
func (v *Waiver) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Waiver) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "rule", "kind", "age_days", "expires", "owner_present")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "rule", &v.Rule, readVia[ID]),
		field(o, at, "kind", &v.Kind, readVia[WaiverKind]),
		field(o, at, "age_days", &v.AgeDays, readWhole),
		field(o, at, "expires", &v.Expires, readVia[Date]),
		field(o, at, "owner_present", &v.OwnerPresent, readTruth),
	)
}

// WaiverKind is has the shape the schema gives it.
type WaiverKind string

// The values of WaiverKind.
const (
	WaiverKindPath WaiverKind = "path"
	WaiverKindKind WaiverKind = "kind"
	WaiverKindRule WaiverKind = "rule"
)

// ToCanon is v as canon's value tree, as encoding/json wrote WaiverKind.
func (v WaiverKind) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member WaiverKind has no
// field for, under its exact name, is refused.
func (v *WaiverKind) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *WaiverKind) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = WaiverKind(x)
	return err
}
