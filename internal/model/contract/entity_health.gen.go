// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-health.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Health is The seven health numbers of a release (entity, spec 16.1).
type Health struct {
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 7 items, at most 7 items.
	Numbers []HealthNumbers `json:"numbers"`
	// an overlay carries two sets: its own, and the one resolved over its parent.
	// Display only.
	// Since contract 1.0.
	// Absent means false.
	Resolved bool `json:"resolved,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Health.
func (v Health) ToCanon() canon.Value {
	o := canon.Object{}
	o["numbers"] = listOf(v.Numbers, HealthNumbers.ToCanon)
	keep(o, "resolved", truthOf(v.Resolved), v.Resolved)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Health has no
// field for, under its exact name, is refused.
func (v *Health) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Health) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "numbers", "resolved")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "numbers", &v.Numbers, listRead(readVia[HealthNumbers])),
		field(o, at, "resolved", &v.Resolved, readTruth),
	)
}

// HealthNumbers is has the shape the schema gives it.
type HealthNumbers struct {
	// Display only.
	// Since contract 1.0.
	Name Slug `json:"name"`
	// Display only.
	// Since contract 1.0.
	Value float64 `json:"value"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote HealthNumbers.
func (v HealthNumbers) ToCanon() canon.Value {
	o := canon.Object{}
	o["name"] = v.Name.ToCanon()
	o["value"] = numberOf(v.Value)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member HealthNumbers has no
// field for, under its exact name, is refused.
func (v *HealthNumbers) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *HealthNumbers) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "name", "value")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "name", &v.Name, readVia[Slug]),
		field(o, at, "value", &v.Value, readNumber),
	)
}
