// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-structure.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Structure is Layers, parts, concerns and kinds (entity, spec 16.1).
type Structure struct {
	// Display only.
	// Since contract 1.0.
	Layers []StructureLayers `json:"layers"`
	// Display only.
	// Since contract 1.0.
	Parts []StructureParts `json:"parts"`
	// Display only.
	// Since contract 1.0.
	Kinds []StructureKinds `json:"kinds"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Structure.
func (v Structure) ToCanon() canon.Value {
	o := canon.Object{}
	o["layers"] = listOf(v.Layers, StructureLayers.ToCanon)
	o["parts"] = listOf(v.Parts, StructureParts.ToCanon)
	o["kinds"] = listOf(v.Kinds, StructureKinds.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Structure has no
// field for, under its exact name, is refused.
func (v *Structure) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Structure) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "layers", "parts", "kinds")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "layers", &v.Layers, listRead(readVia[StructureLayers])),
		field(o, at, "parts", &v.Parts, listRead(readVia[StructureParts])),
		field(o, at, "kinds", &v.Kinds, listRead(readVia[StructureKinds])),
	)
}

// StructureLayers is has the shape the schema gives it.
type StructureLayers struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Name Text `json:"name"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 0.
	Order int `json:"order"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote StructureLayers.
func (v StructureLayers) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["name"] = v.Name.ToCanon()
	o["order"] = wholeOf(v.Order)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member StructureLayers has no
// field for, under its exact name, is refused.
func (v *StructureLayers) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *StructureLayers) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "name", "order")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "name", &v.Name, readVia[Text]),
		field(o, at, "order", &v.Order, readWhole),
	)
}

// StructurePartsConcerns is has the shape the schema gives it.
type StructurePartsConcerns struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Name Text `json:"name"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 0.
	Order int `json:"order"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote StructurePartsConcerns.
func (v StructurePartsConcerns) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["name"] = v.Name.ToCanon()
	o["order"] = wholeOf(v.Order)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member StructurePartsConcerns has no
// field for, under its exact name, is refused.
func (v *StructurePartsConcerns) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *StructurePartsConcerns) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "name", "order")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "name", &v.Name, readVia[Text]),
		field(o, at, "order", &v.Order, readWhole),
	)
}

// StructureParts is has the shape the schema gives it.
type StructureParts struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Name Text `json:"name"`
	// Display only.
	// Since contract 1.0.
	Layer Slug `json:"layer"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 0.
	Order int `json:"order"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Guide TextOrEmpty `json:"guide,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Owners []Text `json:"owners,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Aliases []Slug `json:"aliases,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Concerns []StructurePartsConcerns `json:"concerns,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote StructureParts.
func (v StructureParts) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["name"] = v.Name.ToCanon()
	o["layer"] = v.Layer.ToCanon()
	o["order"] = wholeOf(v.Order)
	keep(o, "guide", v.Guide.ToCanon(), v.Guide != "")
	keep(o, "owners", listOf(v.Owners, Text.ToCanon), len(v.Owners) > 0)
	keep(o, "aliases", listOf(v.Aliases, Slug.ToCanon), len(v.Aliases) > 0)
	keep(o, "concerns", listOf(v.Concerns, StructurePartsConcerns.ToCanon), len(v.Concerns) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member StructureParts has no
// field for, under its exact name, is refused.
func (v *StructureParts) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *StructureParts) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "name", "layer", "order", "guide", "owners", "aliases", "concerns")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "name", &v.Name, readVia[Text]),
		field(o, at, "layer", &v.Layer, readVia[Slug]),
		field(o, at, "order", &v.Order, readWhole),
		field(o, at, "guide", &v.Guide, readVia[TextOrEmpty]),
		field(o, at, "owners", &v.Owners, listRead(readVia[Text])),
		field(o, at, "aliases", &v.Aliases, listRead(readVia[Slug])),
		field(o, at, "concerns", &v.Concerns, listRead(readVia[StructurePartsConcerns])),
	)
}

// StructureKinds is has the shape the schema gives it.
type StructureKinds struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Name Text `json:"name"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Description TextOrEmpty `json:"description,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote StructureKinds.
func (v StructureKinds) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["name"] = v.Name.ToCanon()
	keep(o, "description", v.Description.ToCanon(), v.Description != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member StructureKinds has no
// field for, under its exact name, is refused.
func (v *StructureKinds) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *StructureKinds) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "name", "description")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "name", &v.Name, readVia[Text]),
		field(o, at, "description", &v.Description, readVia[TextOrEmpty]),
	)
}
