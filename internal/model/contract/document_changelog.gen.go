// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-changelog.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// ChangelogDocument is The changelog across releases (document).
type ChangelogDocument struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	Rulebook Slug `json:"rulebook"`
	// Display only.
	// Since contract 1.0.
	Releases []ChangelogDocumentReleases `json:"releases"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ChangelogDocument.
func (v ChangelogDocument) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["rulebook"] = v.Rulebook.ToCanon()
	o["releases"] = listOf(v.Releases, ChangelogDocumentReleases.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ChangelogDocument has no
// field for, under its exact name, is refused.
func (v *ChangelogDocument) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ChangelogDocument) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "rulebook", "releases")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "rulebook", &v.Rulebook, readVia[Slug]),
		field(o, at, "releases", &v.Releases, listRead(readVia[ChangelogDocumentReleases])),
	)
}

// ChangelogDocumentReleases is has the shape the schema gives it.
type ChangelogDocumentReleases struct {
	// Display only.
	// Since contract 1.0.
	Version Semver `json:"version"`
	// Display only.
	// Since contract 1.0.
	Date Date `json:"date"`
	// the release's bump kind, the same enum as release.bump; null when the engine has none for it.
	// Display only.
	// Since contract 1.1.
	// Absent means null.
	Bump *Bump `json:"bump,omitempty"`
	// one note on the release (S75), display only.
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	ReleaseNote TextOrEmpty `json:"release_note,omitempty"`
	// Display only.
	// Since contract 1.0.
	Entries []ChangelogEntry `json:"entries"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ChangelogDocumentReleases.
func (v ChangelogDocumentReleases) ToCanon() canon.Value {
	o := canon.Object{}
	o["version"] = v.Version.ToCanon()
	o["date"] = v.Date.ToCanon()
	keepPtr(o, "bump", v.Bump, Bump.ToCanon)
	keep(o, "release_note", v.ReleaseNote.ToCanon(), v.ReleaseNote != "")
	o["entries"] = listOf(v.Entries, ChangelogEntry.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ChangelogDocumentReleases has no
// field for, under its exact name, is refused.
func (v *ChangelogDocumentReleases) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ChangelogDocumentReleases) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "version", "date", "bump", "release_note", "entries")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "version", &v.Version, readVia[Semver]),
		field(o, at, "date", &v.Date, readVia[Date]),
		field(o, at, "bump", &v.Bump, ptrRead(readVia[Bump])),
		field(o, at, "release_note", &v.ReleaseNote, readVia[TextOrEmpty]),
		field(o, at, "entries", &v.Entries, listRead(readVia[ChangelogEntry])),
	)
}
