// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-preset.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Preset is Preset by name and digest (entity, spec 16.1).
type Preset struct {
	// Display only.
	// Since contract 1.0.
	Tool Text `json:"tool"`
	// Display only.
	// Since contract 1.0.
	Name Slug `json:"name"`
	// Display only.
	// Since contract 1.0.
	Release Semver `json:"release"`
	// Display only.
	// Since contract 1.0.
	Digest Digest `json:"digest"`
	// Display only.
	// Since contract 1.0.
	// Absent means 0.
	// Constraints: at least 0.
	Entries int `json:"entries,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Extends []PresetExtends `json:"extends,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Licence TextOrEmpty `json:"licence,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Preset.
func (v Preset) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool"] = v.Tool.ToCanon()
	o["name"] = v.Name.ToCanon()
	o["release"] = v.Release.ToCanon()
	o["digest"] = v.Digest.ToCanon()
	keep(o, "entries", wholeOf(v.Entries), v.Entries != 0)
	keep(o, "extends", listOf(v.Extends, PresetExtends.ToCanon), len(v.Extends) > 0)
	keep(o, "licence", v.Licence.ToCanon(), v.Licence != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Preset has no
// field for, under its exact name, is refused.
func (v *Preset) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Preset) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "tool", "name", "release", "digest", "entries", "extends", "licence")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "tool", &v.Tool, readVia[Text]),
		field(o, at, "name", &v.Name, readVia[Slug]),
		field(o, at, "release", &v.Release, readVia[Semver]),
		field(o, at, "digest", &v.Digest, readVia[Digest]),
		field(o, at, "entries", &v.Entries, readWhole),
		field(o, at, "extends", &v.Extends, listRead(readVia[PresetExtends])),
		field(o, at, "licence", &v.Licence, readVia[TextOrEmpty]),
	)
}

// PresetExtends is has the shape the schema gives it.
type PresetExtends struct {
	// Display only.
	// Since contract 1.0.
	Rulebook Text `json:"rulebook"`
	// Display only.
	// Since contract 1.0.
	Version Semver `json:"version"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote PresetExtends.
func (v PresetExtends) ToCanon() canon.Value {
	o := canon.Object{}
	o["rulebook"] = v.Rulebook.ToCanon()
	o["version"] = v.Version.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member PresetExtends has no
// field for, under its exact name, is refused.
func (v *PresetExtends) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PresetExtends) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rulebook", "version")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rulebook", &v.Rulebook, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Semver]),
	)
}
