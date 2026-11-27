// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-history-entry.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// HistoryEntry is History entry (entity, spec 16.1).
type HistoryEntry struct {
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 1.
	Number int `json:"number"`
	// Display only.
	// Since contract 1.0.
	Kind Slug `json:"kind"`
	// Display only.
	// Since contract 1.0.
	Entity Text `json:"entity"`
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Before canon.Object `json:"before,omitempty"`
	// Display only.
	// Since contract 1.0.
	After canon.Object `json:"after"`
	// Display only.
	// Since contract 1.0.
	Who Text `json:"who"`
	// Display only.
	// Since contract 1.0.
	When Timestamp `json:"when"`
	// Display only.
	// Since contract 1.0.
	Plan Digest `json:"plan"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Command *Text `json:"command,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Decision *Slug `json:"decision,omitempty"`
	// the status this step moved a rule to; it equals after.status where the step records one, and is absent or null on a step that moves no rule's status. after stays an untyped record, the entity's own, so this field is the one a reader can name.
	// Display only.
	// Since contract 1.3.
	// Absent means null.
	Status *Status `json:"status,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote HistoryEntry.
func (v HistoryEntry) ToCanon() canon.Value {
	o := canon.Object{}
	o["number"] = wholeOf(v.Number)
	o["kind"] = v.Kind.ToCanon()
	o["entity"] = v.Entity.ToCanon()
	keep(o, "before", objectOf(v.Before), len(v.Before) > 0)
	o["after"] = objectOf(v.After)
	o["who"] = v.Who.ToCanon()
	o["when"] = v.When.ToCanon()
	o["plan"] = v.Plan.ToCanon()
	keepPtr(o, "command", v.Command, Text.ToCanon)
	keepPtr(o, "decision", v.Decision, Slug.ToCanon)
	keepPtr(o, "status", v.Status, Status.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member HistoryEntry has no
// field for, under its exact name, is refused.
func (v *HistoryEntry) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *HistoryEntry) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "number", "kind", "entity", "before", "after", "who", "when", "plan", "command", "decision", "status")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "number", &v.Number, readWhole),
		field(o, at, "kind", &v.Kind, readVia[Slug]),
		field(o, at, "entity", &v.Entity, readVia[Text]),
		field(o, at, "before", &v.Before, readObject),
		field(o, at, "after", &v.After, readObject),
		field(o, at, "who", &v.Who, readVia[Text]),
		field(o, at, "when", &v.When, readVia[Timestamp]),
		field(o, at, "plan", &v.Plan, readVia[Digest]),
		field(o, at, "command", &v.Command, ptrRead(readVia[Text])),
		field(o, at, "decision", &v.Decision, ptrRead(readVia[Slug])),
		field(o, at, "status", &v.Status, ptrRead(readVia[Status])),
	)
}
