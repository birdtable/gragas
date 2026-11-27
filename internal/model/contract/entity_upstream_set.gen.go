// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-upstream-set.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// UpstreamSet is The flattened upstream set of one tool (entity).
type UpstreamSet struct {
	// Decides: whether the file is read or ends not ready on a version newer than the engine reads.
	// Since contract 1.0.
	// Constraints: at least 1.
	Version int `json:"version"`
	// Decides: which tool's bindings the sets are counted under.
	// Since contract 1.0.
	Tool Slug `json:"tool"`
	// the upstream presets, each with the entry keys it held at the pin.
	// Decides: which upstream presets a rung-0 binding can adopt whole, and the entries each explains.
	// Since contract 1.0.
	Sets []UpstreamSetSets `json:"sets"`
	// every entry key the upstream config holds now.
	// Decides: which entries coverage counts, and which of them nothing explains.
	// Since contract 1.0.
	Observed []Text `json:"observed"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote UpstreamSet.
func (v UpstreamSet) ToCanon() canon.Value {
	o := canon.Object{}
	o["version"] = wholeOf(v.Version)
	o["tool"] = v.Tool.ToCanon()
	o["sets"] = listOf(v.Sets, UpstreamSetSets.ToCanon)
	o["observed"] = listOf(v.Observed, Text.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member UpstreamSet has no
// field for, under its exact name, is refused.
func (v *UpstreamSet) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *UpstreamSet) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "version", "tool", "sets", "observed")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "version", &v.Version, readWhole),
		field(o, at, "tool", &v.Tool, readVia[Slug]),
		field(o, at, "sets", &v.Sets, listRead(readVia[UpstreamSetSets])),
		field(o, at, "observed", &v.Observed, listRead(readVia[Text])),
	)
}

// UpstreamSetSets is has the shape the schema gives it.
type UpstreamSetSets struct {
	// Decides: which rung-0 binding adopts the set, by its key.
	// Since contract 1.0.
	Preset Text `json:"preset"`
	// Decides: the entries the set explains.
	// Since contract 1.0.
	Entries []Text `json:"entries"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote UpstreamSetSets.
func (v UpstreamSetSets) ToCanon() canon.Value {
	o := canon.Object{}
	o["preset"] = v.Preset.ToCanon()
	o["entries"] = listOf(v.Entries, Text.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member UpstreamSetSets has no
// field for, under its exact name, is refused.
func (v *UpstreamSetSets) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *UpstreamSetSets) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "preset", "entries")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "preset", &v.Preset, readVia[Text]),
		field(o, at, "entries", &v.Entries, listRead(readVia[Text])),
	)
}
