// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-release.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Release is Release (entity, spec 16.1).
type Release struct {
	// Display only.
	// Since contract 1.0.
	Version Semver `json:"version"`
	// Display only.
	// Since contract 1.0.
	Bump ReleaseBump `json:"bump"`
	// Display only.
	// Since contract 1.0.
	Date Date `json:"date"`
	// one note on the release, plain words, display only (S75).
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	ReleaseNote TextOrEmpty `json:"release_note,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Assets []ReleaseAssets `json:"assets,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	CheckerCode []ReleaseCheckerCode `json:"checker_code,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	TestedSet *TestedSet `json:"tested_set,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Health *Health `json:"health,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 1.
	Format int `json:"format"`
	// Decides: which contract version's schema reads the release's documents, and whether they are read or end not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Withdrawn *ReleaseWithdrawn `json:"withdrawn,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Release.
func (v Release) ToCanon() canon.Value {
	o := canon.Object{}
	o["version"] = v.Version.ToCanon()
	o["bump"] = v.Bump.ToCanon()
	o["date"] = v.Date.ToCanon()
	keep(o, "release_note", v.ReleaseNote.ToCanon(), v.ReleaseNote != "")
	keep(o, "assets", listOf(v.Assets, ReleaseAssets.ToCanon), len(v.Assets) > 0)
	keep(o, "checker_code", listOf(v.CheckerCode, ReleaseCheckerCode.ToCanon), len(v.CheckerCode) > 0)
	keepPtr(o, "tested_set", v.TestedSet, TestedSet.ToCanon)
	keepPtr(o, "health", v.Health, Health.ToCanon)
	o["format"] = wholeOf(v.Format)
	o["contract"] = v.Contract.ToCanon()
	keepPtr(o, "withdrawn", v.Withdrawn, ReleaseWithdrawn.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Release has no
// field for, under its exact name, is refused.
func (v *Release) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Release) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "version", "bump", "date", "release_note", "assets", "checker_code", "tested_set", "health", "format", "contract", "withdrawn")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "version", &v.Version, readVia[Semver]),
		field(o, at, "bump", &v.Bump, readVia[ReleaseBump]),
		field(o, at, "date", &v.Date, readVia[Date]),
		field(o, at, "release_note", &v.ReleaseNote, readVia[TextOrEmpty]),
		field(o, at, "assets", &v.Assets, listRead(readVia[ReleaseAssets])),
		field(o, at, "checker_code", &v.CheckerCode, listRead(readVia[ReleaseCheckerCode])),
		field(o, at, "tested_set", &v.TestedSet, ptrRead(readVia[TestedSet])),
		field(o, at, "health", &v.Health, ptrRead(readVia[Health])),
		field(o, at, "format", &v.Format, readWhole),
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "withdrawn", &v.Withdrawn, ptrRead(readVia[ReleaseWithdrawn])),
	)
}

// ReleaseBump is has the shape the schema gives it.
type ReleaseBump string

// The values of ReleaseBump.
const (
	ReleaseBumpMajor ReleaseBump = "major"
	ReleaseBumpMinor ReleaseBump = "minor"
	ReleaseBumpPatch ReleaseBump = "patch"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReleaseBump.
func (v ReleaseBump) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReleaseBump has no
// field for, under its exact name, is refused.
func (v *ReleaseBump) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReleaseBump) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReleaseBump(x)
	return err
}

// ReleaseAssets is has the shape the schema gives it.
type ReleaseAssets struct {
	// Display only.
	// Since contract 1.0.
	Path Text `json:"path"`
	// Decides: whether a fetched asset is the one the release lists, so whether it is accepted.
	// Since contract 1.0.
	Digest Digest `json:"digest"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReleaseAssets.
func (v ReleaseAssets) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = v.Path.ToCanon()
	o["digest"] = v.Digest.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReleaseAssets has no
// field for, under its exact name, is refused.
func (v *ReleaseAssets) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReleaseAssets) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "path", "digest")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "path", &v.Path, readVia[Text]),
		field(o, at, "digest", &v.Digest, readVia[Digest]),
	)
}

// ReleaseCheckerCode is has the shape the schema gives it.
type ReleaseCheckerCode struct {
	// Display only.
	// Since contract 1.0.
	Name Slug `json:"name"`
	// Display only.
	// Since contract 1.0.
	Publisher Text `json:"publisher"`
	// Decides: whether fetched checker code is the code the release lists, so whether it is accepted.
	// Since contract 1.0.
	Digest Digest `json:"digest"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReleaseCheckerCode.
func (v ReleaseCheckerCode) ToCanon() canon.Value {
	o := canon.Object{}
	o["name"] = v.Name.ToCanon()
	o["publisher"] = v.Publisher.ToCanon()
	o["digest"] = v.Digest.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReleaseCheckerCode has no
// field for, under its exact name, is refused.
func (v *ReleaseCheckerCode) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReleaseCheckerCode) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "name", "publisher", "digest")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "name", &v.Name, readVia[Slug]),
		field(o, at, "publisher", &v.Publisher, readVia[Text]),
		field(o, at, "digest", &v.Digest, readVia[Digest]),
	)
}

// ReleaseWithdrawn is has the shape the schema gives it.
type ReleaseWithdrawn struct {
	// Display only.
	// Since contract 1.0.
	Date Date `json:"date"`
	// Display only.
	// Since contract 1.0.
	Reason Reason `json:"reason"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReleaseWithdrawn.
func (v ReleaseWithdrawn) ToCanon() canon.Value {
	o := canon.Object{}
	o["date"] = v.Date.ToCanon()
	o["reason"] = v.Reason.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReleaseWithdrawn has no
// field for, under its exact name, is refused.
func (v *ReleaseWithdrawn) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReleaseWithdrawn) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "date", "reason")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "date", &v.Date, readVia[Date]),
		field(o, at, "reason", &v.Reason, readVia[Reason]),
	)
}
