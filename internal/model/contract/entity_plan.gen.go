// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-plan.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Plan is Plan (entity, spec 16.1 and 15.3): one shape for every changing command.
type Plan struct {
	// Display only.
	// Since contract 1.0.
	Digest Digest `json:"digest"`
	// Display only.
	// Since contract 1.0.
	Command Text `json:"command"`
	// Display only.
	// Since contract 1.0.
	Changes []PlanChanges `json:"changes"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	ConsumerActions []Text `json:"consumer_actions,omitempty"`
	// Display only.
	// Since contract 1.0.
	Writes []PlanWrites `json:"writes"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Rekeyed []PlanRekeyed `json:"rekeyed,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Renderings []PlanRenderings `json:"renderings,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	CheckerCode []PlanCheckerCode `json:"checker_code,omitempty"`
	// Display only.
	// Since contract 1.0.
	Question Text `json:"question"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Plan.
func (v Plan) ToCanon() canon.Value {
	o := canon.Object{}
	o["digest"] = v.Digest.ToCanon()
	o["command"] = v.Command.ToCanon()
	o["changes"] = listOf(v.Changes, PlanChanges.ToCanon)
	keep(o, "consumer_actions", listOf(v.ConsumerActions, Text.ToCanon), len(v.ConsumerActions) > 0)
	o["writes"] = listOf(v.Writes, PlanWrites.ToCanon)
	keep(o, "rekeyed", listOf(v.Rekeyed, PlanRekeyed.ToCanon), len(v.Rekeyed) > 0)
	keep(o, "renderings", listOf(v.Renderings, PlanRenderings.ToCanon), len(v.Renderings) > 0)
	keep(o, "checker_code", listOf(v.CheckerCode, PlanCheckerCode.ToCanon), len(v.CheckerCode) > 0)
	o["question"] = v.Question.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Plan has no
// field for, under its exact name, is refused.
func (v *Plan) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Plan) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "digest", "command", "changes", "consumer_actions", "writes", "rekeyed", "renderings", "checker_code", "question")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "digest", &v.Digest, readVia[Digest]),
		field(o, at, "command", &v.Command, readVia[Text]),
		field(o, at, "changes", &v.Changes, listRead(readVia[PlanChanges])),
		field(o, at, "consumer_actions", &v.ConsumerActions, listRead(readVia[Text])),
		field(o, at, "writes", &v.Writes, listRead(readVia[PlanWrites])),
		field(o, at, "rekeyed", &v.Rekeyed, listRead(readVia[PlanRekeyed])),
		field(o, at, "renderings", &v.Renderings, listRead(readVia[PlanRenderings])),
		field(o, at, "checker_code", &v.CheckerCode, listRead(readVia[PlanCheckerCode])),
		field(o, at, "question", &v.Question, readVia[Text]),
	)
}

// PlanChanges is has the shape the schema gives it.
type PlanChanges struct {
	// Display only.
	// Since contract 1.0.
	Entity Text `json:"entity"`
	// null means not applicable: the entry is not about a rule, or the plan creates the rule and it had no standing before.
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	StandingBefore *Standing `json:"standing_before,omitempty"`
	// null means not applicable: the entry is not about a rule.
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	StandingAfter *Standing `json:"standing_after,omitempty"`
	// Display only.
	// Since contract 1.0.
	What Text `json:"what"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote PlanChanges.
func (v PlanChanges) ToCanon() canon.Value {
	o := canon.Object{}
	o["entity"] = v.Entity.ToCanon()
	keepPtr(o, "standing_before", v.StandingBefore, Standing.ToCanon)
	keepPtr(o, "standing_after", v.StandingAfter, Standing.ToCanon)
	o["what"] = v.What.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member PlanChanges has no
// field for, under its exact name, is refused.
func (v *PlanChanges) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanChanges) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "entity", "standing_before", "standing_after", "what")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "entity", &v.Entity, readVia[Text]),
		field(o, at, "standing_before", &v.StandingBefore, ptrRead(readVia[Standing])),
		field(o, at, "standing_after", &v.StandingAfter, ptrRead(readVia[Standing])),
		field(o, at, "what", &v.What, readVia[Text]),
	)
}

// PlanWrites is has the shape the schema gives it.
type PlanWrites struct {
	// Display only.
	// Since contract 1.0.
	Path Text `json:"path"`
	// Display only.
	// Since contract 1.0.
	What Text `json:"what"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote PlanWrites.
func (v PlanWrites) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = v.Path.ToCanon()
	o["what"] = v.What.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member PlanWrites has no
// field for, under its exact name, is refused.
func (v *PlanWrites) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanWrites) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "path", "what")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "path", &v.Path, readVia[Text]),
		field(o, at, "what", &v.What, readVia[Text]),
	)
}

// PlanRekeyedEntity is has the shape the schema gives it.
type PlanRekeyedEntity string

// The values of PlanRekeyedEntity.
const (
	PlanRekeyedEntityWaiver   PlanRekeyedEntity = "waiver"
	PlanRekeyedEntityBaseline PlanRekeyedEntity = "baseline"
)

// ToCanon is v as canon's value tree, as encoding/json wrote PlanRekeyedEntity.
func (v PlanRekeyedEntity) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member PlanRekeyedEntity has no
// field for, under its exact name, is refused.
func (v *PlanRekeyedEntity) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanRekeyedEntity) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = PlanRekeyedEntity(x)
	return err
}

// PlanRekeyedState is has the shape the schema gives it.
type PlanRekeyedState string

// The values of PlanRekeyedState.
const (
	PlanRekeyedStateReKeyed  PlanRekeyedState = "re-keyed"
	PlanRekeyedStateOrphaned PlanRekeyedState = "orphaned"
)

// ToCanon is v as canon's value tree, as encoding/json wrote PlanRekeyedState.
func (v PlanRekeyedState) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member PlanRekeyedState has no
// field for, under its exact name, is refused.
func (v *PlanRekeyedState) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanRekeyedState) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = PlanRekeyedState(x)
	return err
}

// PlanRekeyed is has the shape the schema gives it.
type PlanRekeyed struct {
	// Display only.
	// Since contract 1.0.
	Entity PlanRekeyedEntity `json:"entity"`
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	State PlanRekeyedState `json:"state"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote PlanRekeyed.
func (v PlanRekeyed) ToCanon() canon.Value {
	o := canon.Object{}
	o["entity"] = v.Entity.ToCanon()
	o["slug"] = v.Slug.ToCanon()
	o["state"] = v.State.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member PlanRekeyed has no
// field for, under its exact name, is refused.
func (v *PlanRekeyed) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanRekeyed) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "entity", "slug", "state")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "entity", &v.Entity, readVia[PlanRekeyedEntity]),
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "state", &v.State, readVia[PlanRekeyedState]),
	)
}

// PlanRenderingsWhat is has the shape the schema gives it.
type PlanRenderingsWhat string

// The values of PlanRenderingsWhat.
const (
	PlanRenderingsWhatAgentFile  PlanRenderingsWhat = "agent file"
	PlanRenderingsWhatReviewFile PlanRenderingsWhat = "review file"
	PlanRenderingsWhatPresetCopy PlanRenderingsWhat = "preset copy"
	PlanRenderingsWhatPage       PlanRenderingsWhat = "page"
)

// ToCanon is v as canon's value tree, as encoding/json wrote PlanRenderingsWhat.
func (v PlanRenderingsWhat) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member PlanRenderingsWhat has no
// field for, under its exact name, is refused.
func (v *PlanRenderingsWhat) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanRenderingsWhat) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = PlanRenderingsWhat(x)
	return err
}

// PlanRenderings is has the shape the schema gives it.
type PlanRenderings struct {
	// Display only.
	// Since contract 1.0.
	Path Text `json:"path"`
	// Display only.
	// Since contract 1.0.
	What PlanRenderingsWhat `json:"what"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote PlanRenderings.
func (v PlanRenderings) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = v.Path.ToCanon()
	o["what"] = v.What.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member PlanRenderings has no
// field for, under its exact name, is refused.
func (v *PlanRenderings) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanRenderings) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "path", "what")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "path", &v.Path, readVia[Text]),
		field(o, at, "what", &v.What, readVia[PlanRenderingsWhat]),
	)
}

// PlanCheckerCode is has the shape the schema gives it.
type PlanCheckerCode struct {
	// Display only.
	// Since contract 1.0.
	Name Slug `json:"name"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Diff *URL `json:"diff,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote PlanCheckerCode.
func (v PlanCheckerCode) ToCanon() canon.Value {
	o := canon.Object{}
	o["name"] = v.Name.ToCanon()
	keepPtr(o, "diff", v.Diff, URL.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member PlanCheckerCode has no
// field for, under its exact name, is refused.
func (v *PlanCheckerCode) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *PlanCheckerCode) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "name", "diff")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "name", &v.Name, readVia[Slug]),
		field(o, at, "diff", &v.Diff, ptrRead(readVia[URL])),
	)
}
