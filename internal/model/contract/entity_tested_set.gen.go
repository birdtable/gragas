// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-tested-set.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// TestedSet is Tested set of a release (entity, spec 16.1).
type TestedSet struct {
	// Display only.
	// Since contract 1.0.
	Tools []TestedSetTools `json:"tools"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Plugins []TestedSetPlugins `json:"plugins,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Kits []TestedSetKits `json:"kits,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Adapters []TestedSetAdapters `json:"adapters,omitempty"`
	// Display only.
	// Since contract 1.0.
	MinimumEngine Semver `json:"minimum_engine"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote TestedSet.
func (v TestedSet) ToCanon() canon.Value {
	o := canon.Object{}
	o["tools"] = listOf(v.Tools, TestedSetTools.ToCanon)
	keep(o, "plugins", listOf(v.Plugins, TestedSetPlugins.ToCanon), len(v.Plugins) > 0)
	keep(o, "kits", listOf(v.Kits, TestedSetKits.ToCanon), len(v.Kits) > 0)
	keep(o, "adapters", listOf(v.Adapters, TestedSetAdapters.ToCanon), len(v.Adapters) > 0)
	o["minimum_engine"] = v.MinimumEngine.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member TestedSet has no
// field for, under its exact name, is refused.
func (v *TestedSet) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *TestedSet) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "tools", "plugins", "kits", "adapters", "minimum_engine")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "tools", &v.Tools, listRead(readVia[TestedSetTools])),
		field(o, at, "plugins", &v.Plugins, listRead(readVia[TestedSetPlugins])),
		field(o, at, "kits", &v.Kits, listRead(readVia[TestedSetKits])),
		field(o, at, "adapters", &v.Adapters, listRead(readVia[TestedSetAdapters])),
		field(o, at, "minimum_engine", &v.MinimumEngine, readVia[Semver]),
	)
}

// TestedSetTools is has the shape the schema gives it.
type TestedSetTools struct {
	// Display only.
	// Since contract 1.0.
	Tool Text `json:"tool"`
	// Display only.
	// Since contract 1.0.
	Version Text `json:"version"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote TestedSetTools.
func (v TestedSetTools) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool"] = v.Tool.ToCanon()
	o["version"] = v.Version.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member TestedSetTools has no
// field for, under its exact name, is refused.
func (v *TestedSetTools) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *TestedSetTools) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "tool", "version")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "tool", &v.Tool, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Text]),
	)
}

// TestedSetPlugins is has the shape the schema gives it.
type TestedSetPlugins struct {
	// Display only.
	// Since contract 1.0.
	Plugin Text `json:"plugin"`
	// Display only.
	// Since contract 1.0.
	Version Text `json:"version"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote TestedSetPlugins.
func (v TestedSetPlugins) ToCanon() canon.Value {
	o := canon.Object{}
	o["plugin"] = v.Plugin.ToCanon()
	o["version"] = v.Version.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member TestedSetPlugins has no
// field for, under its exact name, is refused.
func (v *TestedSetPlugins) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *TestedSetPlugins) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "plugin", "version")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "plugin", &v.Plugin, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Text]),
	)
}

// TestedSetKits is has the shape the schema gives it.
type TestedSetKits struct {
	// Display only.
	// Since contract 1.0.
	Kit Text `json:"kit"`
	// Display only.
	// Since contract 1.0.
	Version Text `json:"version"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote TestedSetKits.
func (v TestedSetKits) ToCanon() canon.Value {
	o := canon.Object{}
	o["kit"] = v.Kit.ToCanon()
	o["version"] = v.Version.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member TestedSetKits has no
// field for, under its exact name, is refused.
func (v *TestedSetKits) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *TestedSetKits) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "kit", "version")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "kit", &v.Kit, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Text]),
	)
}

// TestedSetAdapters is has the shape the schema gives it.
type TestedSetAdapters struct {
	// Display only.
	// Since contract 1.0.
	Adapter Text `json:"adapter"`
	// Display only.
	// Since contract 1.0.
	Version Text `json:"version"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote TestedSetAdapters.
func (v TestedSetAdapters) ToCanon() canon.Value {
	o := canon.Object{}
	o["adapter"] = v.Adapter.ToCanon()
	o["version"] = v.Version.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member TestedSetAdapters has no
// field for, under its exact name, is refused.
func (v *TestedSetAdapters) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *TestedSetAdapters) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "adapter", "version")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "adapter", &v.Adapter, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Text]),
	)
}
