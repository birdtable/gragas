// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-baseline.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Baseline is Baseline as a report counts it (entity, spec 16.7).
type Baseline struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Rule ID `json:"rule"`
	// Display only.
	// Since contract 1.0.
	Tool Text `json:"tool"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 0.
	Count int `json:"count"`
	// Display only.
	// Since contract 1.0.
	Owner Text `json:"owner"`
	// Display only.
	// Since contract 1.0.
	ZeroBy Date `json:"zero_by"`
	// Display only.
	// Since contract 1.0.
	// Absent means 0.
	// Constraints: at least 0.
	AgeDays int `json:"age_days,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Baseline.
func (v Baseline) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["rule"] = v.Rule.ToCanon()
	o["tool"] = v.Tool.ToCanon()
	o["count"] = wholeOf(v.Count)
	o["owner"] = v.Owner.ToCanon()
	o["zero_by"] = v.ZeroBy.ToCanon()
	keep(o, "age_days", wholeOf(v.AgeDays), v.AgeDays != 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Baseline has no
// field for, under its exact name, is refused.
func (v *Baseline) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Baseline) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "rule", "tool", "count", "owner", "zero_by", "age_days")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "rule", &v.Rule, readVia[ID]),
		field(o, at, "tool", &v.Tool, readVia[Text]),
		field(o, at, "count", &v.Count, readWhole),
		field(o, at, "owner", &v.Owner, readVia[Text]),
		field(o, at, "zero_by", &v.ZeroBy, readVia[Date]),
		field(o, at, "age_days", &v.AgeDays, readWhole),
	)
}
