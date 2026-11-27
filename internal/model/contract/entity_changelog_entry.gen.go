// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-changelog-entry.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// ChangelogEntry is Changelog entry (entity, spec 16.1).
type ChangelogEntry struct {
	// Display only.
	// Since contract 1.0.
	Release Semver `json:"release"`
	// Display only.
	// Since contract 1.0.
	Entity Text `json:"entity"`
	// Display only.
	// Since contract 1.0.
	Change ChangelogEntryChange `json:"change"`
	// the bump kind of the release the entry shipped in; null when the engine has none for it.
	// Display only.
	// Since contract 1.1.
	// Absent means null.
	Bump *Bump `json:"bump,omitempty"`
	// the date of the release the entry shipped in; null when the engine has none for it.
	// Display only.
	// Since contract 1.1.
	// Absent means null.
	Date *Date `json:"date,omitempty"`
	// written by a person, display only (S75).
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Sentence TextOrEmpty `json:"sentence,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	ConsumerAction TextOrEmpty `json:"consumer_action,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Remedy TextOrEmpty `json:"remedy,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Diff *URL `json:"diff,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ChangelogEntry.
func (v ChangelogEntry) ToCanon() canon.Value {
	o := canon.Object{}
	o["release"] = v.Release.ToCanon()
	o["entity"] = v.Entity.ToCanon()
	o["change"] = v.Change.ToCanon()
	keepPtr(o, "bump", v.Bump, Bump.ToCanon)
	keepPtr(o, "date", v.Date, Date.ToCanon)
	keep(o, "sentence", v.Sentence.ToCanon(), v.Sentence != "")
	keep(o, "consumer_action", v.ConsumerAction.ToCanon(), v.ConsumerAction != "")
	keep(o, "remedy", v.Remedy.ToCanon(), v.Remedy != "")
	keepPtr(o, "diff", v.Diff, URL.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ChangelogEntry has no
// field for, under its exact name, is refused.
func (v *ChangelogEntry) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ChangelogEntry) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "release", "entity", "change", "bump", "date", "sentence", "consumer_action", "remedy", "diff")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "release", &v.Release, readVia[Semver]),
		field(o, at, "entity", &v.Entity, readVia[Text]),
		field(o, at, "change", &v.Change, readVia[ChangelogEntryChange]),
		field(o, at, "bump", &v.Bump, ptrRead(readVia[Bump])),
		field(o, at, "date", &v.Date, ptrRead(readVia[Date])),
		field(o, at, "sentence", &v.Sentence, readVia[TextOrEmpty]),
		field(o, at, "consumer_action", &v.ConsumerAction, readVia[TextOrEmpty]),
		field(o, at, "remedy", &v.Remedy, readVia[TextOrEmpty]),
		field(o, at, "diff", &v.Diff, ptrRead(readVia[URL])),
	)
}

// ChangelogEntryChange is has the shape the schema gives it.
type ChangelogEntryChange string

// The values of ChangelogEntryChange.
const (
	ChangelogEntryChangeAdded     ChangelogEntryChange = "added"
	ChangelogEntryChangeChanged   ChangelogEntryChange = "changed"
	ChangelogEntryChangeWeakened  ChangelogEntryChange = "weakened"
	ChangelogEntryChangeTightened ChangelogEntryChange = "tightened"
	ChangelogEntryChangeRetired   ChangelogEntryChange = "retired"
	ChangelogEntryChangeRestored  ChangelogEntryChange = "restored"
	ChangelogEntryChangeReplaced  ChangelogEntryChange = "replaced"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ChangelogEntryChange.
func (v ChangelogEntryChange) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ChangelogEntryChange has no
// field for, under its exact name, is refused.
func (v *ChangelogEntryChange) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ChangelogEntryChange) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ChangelogEntryChange(x)
	return err
}
