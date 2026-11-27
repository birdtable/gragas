// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rendering-manifest.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// RenderingManifest is Rendering manifest of a release and target (entity, spec 16.1).
type RenderingManifest struct {
	// Display only.
	// Since contract 1.0.
	Release Semver `json:"release"`
	// Display only.
	// Since contract 1.0.
	Target Text `json:"target"`
	// Display only.
	// Since contract 1.0.
	Renderings []RenderingManifestRenderings `json:"renderings"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RenderingManifest.
func (v RenderingManifest) ToCanon() canon.Value {
	o := canon.Object{}
	o["release"] = v.Release.ToCanon()
	o["target"] = v.Target.ToCanon()
	o["renderings"] = listOf(v.Renderings, RenderingManifestRenderings.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RenderingManifest has no
// field for, under its exact name, is refused.
func (v *RenderingManifest) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RenderingManifest) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "release", "target", "renderings")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "release", &v.Release, readVia[Semver]),
		field(o, at, "target", &v.Target, readVia[Text]),
		field(o, at, "renderings", &v.Renderings, listRead(readVia[RenderingManifestRenderings])),
	)
}

// RenderingManifestRenderingsRules is has the shape the schema gives it.
type RenderingManifestRenderingsRules struct {
	// Display only.
	// Since contract 1.1.
	Rule ID `json:"rule"`
	// Display only.
	// Since contract 1.1.
	Standing Standing `json:"standing"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RenderingManifestRenderingsRules.
func (v RenderingManifestRenderingsRules) ToCanon() canon.Value {
	o := canon.Object{}
	o["rule"] = v.Rule.ToCanon()
	o["standing"] = v.Standing.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RenderingManifestRenderingsRules has no
// field for, under its exact name, is refused.
func (v *RenderingManifestRenderingsRules) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RenderingManifestRenderingsRules) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rule", "standing")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rule", &v.Rule, readVia[ID]),
		field(o, at, "standing", &v.Standing, readVia[Standing]),
	)
}

// RenderingManifestRenderingsOmissions is has the shape the schema gives it.
type RenderingManifestRenderingsOmissions struct {
	// Display only.
	// Since contract 1.0.
	What Text `json:"what"`
	// Display only.
	// Since contract 1.0.
	Who Text `json:"who"`
	// Display only.
	// Since contract 1.0.
	When Timestamp `json:"when"`
	// Display only.
	// Since contract 1.0.
	Plan Digest `json:"plan"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RenderingManifestRenderingsOmissions.
func (v RenderingManifestRenderingsOmissions) ToCanon() canon.Value {
	o := canon.Object{}
	o["what"] = v.What.ToCanon()
	o["who"] = v.Who.ToCanon()
	o["when"] = v.When.ToCanon()
	o["plan"] = v.Plan.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RenderingManifestRenderingsOmissions has no
// field for, under its exact name, is refused.
func (v *RenderingManifestRenderingsOmissions) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RenderingManifestRenderingsOmissions) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "what", "who", "when", "plan")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "what", &v.What, readVia[Text]),
		field(o, at, "who", &v.Who, readVia[Text]),
		field(o, at, "when", &v.When, readVia[Timestamp]),
		field(o, at, "plan", &v.Plan, readVia[Digest]),
	)
}

// RenderingManifestRenderings is has the shape the schema gives it.
type RenderingManifestRenderings struct {
	// Display only.
	// Since contract 1.0.
	Path Text `json:"path"`
	// Display only.
	// Since contract 1.0.
	SourceHash Digest `json:"source_hash"`
	// Display only.
	// Since contract 1.0.
	Language Language `json:"language"`
	// the standing each rule had when the rendering was written, to compare with its standing now; one row per rule.
	// Display only.
	// Since contract 1.1.
	// Absent means [].
	Rules []RenderingManifestRenderingsRules `json:"rules,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Omissions []RenderingManifestRenderingsOmissions `json:"omissions,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RenderingManifestRenderings.
func (v RenderingManifestRenderings) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = v.Path.ToCanon()
	o["source_hash"] = v.SourceHash.ToCanon()
	o["language"] = v.Language.ToCanon()
	keep(o, "rules", listOf(v.Rules, RenderingManifestRenderingsRules.ToCanon), len(v.Rules) > 0)
	keep(o, "omissions", listOf(v.Omissions, RenderingManifestRenderingsOmissions.ToCanon), len(v.Omissions) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RenderingManifestRenderings has no
// field for, under its exact name, is refused.
func (v *RenderingManifestRenderings) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RenderingManifestRenderings) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "path", "source_hash", "language", "rules", "omissions")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "path", &v.Path, readVia[Text]),
		field(o, at, "source_hash", &v.SourceHash, readVia[Digest]),
		field(o, at, "language", &v.Language, readVia[Language]),
		field(o, at, "rules", &v.Rules, listRead(readVia[RenderingManifestRenderingsRules])),
		field(o, at, "omissions", &v.Omissions, listRead(readVia[RenderingManifestRenderingsOmissions])),
	)
}
