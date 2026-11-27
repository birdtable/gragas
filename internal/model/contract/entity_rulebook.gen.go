// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rulebook.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Rulebook is Rulebook: the identity a document carries (entity, spec 16.1).
type Rulebook struct {
	// Display only.
	// Since contract 1.0.
	Name Slug `json:"name"`
	// Display only.
	// Since contract 1.0.
	Version Semver `json:"version"`
	// Display only.
	// Since contract 1.0.
	Kind RulebookKind `json:"kind"`
	// Display only.
	// Since contract 1.0.
	Licences RulebookLicences `json:"licences"`
	// Display only.
	// Since contract 1.0.
	Language Language `json:"language"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	DocsURL *URL `json:"docs_url,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	ExplorerURL *URL `json:"explorer_url,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Extends *RulebookExtends `json:"extends,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Imports []RulebookImports `json:"imports,omitempty"`
	// Display only.
	// Since contract 1.0.
	Owners []Text `json:"owners"`
	// Display only.
	// Since contract 1.0.
	Governance RulebookGovernance `json:"governance"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 1.
	Format int `json:"format"`
	// the name of the rulebook that replaces this one, which an archived rulebook names (S28, S38); absent or null when it names none. The release's rule map toward it, where one is published, is the bundle's rule_map, whose rulebook is this name.
	// Display only.
	// Since contract 1.3.
	// Absent means null.
	Successor *Slug `json:"successor,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Rulebook.
func (v Rulebook) ToCanon() canon.Value {
	o := canon.Object{}
	o["name"] = v.Name.ToCanon()
	o["version"] = v.Version.ToCanon()
	o["kind"] = v.Kind.ToCanon()
	o["licences"] = v.Licences.ToCanon()
	o["language"] = v.Language.ToCanon()
	keepPtr(o, "docs_url", v.DocsURL, URL.ToCanon)
	keepPtr(o, "explorer_url", v.ExplorerURL, URL.ToCanon)
	keepPtr(o, "extends", v.Extends, RulebookExtends.ToCanon)
	keep(o, "imports", listOf(v.Imports, RulebookImports.ToCanon), len(v.Imports) > 0)
	o["owners"] = listOf(v.Owners, Text.ToCanon)
	o["governance"] = v.Governance.ToCanon()
	o["format"] = wholeOf(v.Format)
	keepPtr(o, "successor", v.Successor, Slug.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Rulebook has no
// field for, under its exact name, is refused.
func (v *Rulebook) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Rulebook) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "name", "version", "kind", "licences", "language", "docs_url", "explorer_url", "extends", "imports", "owners", "governance", "format", "successor")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "name", &v.Name, readVia[Slug]),
		field(o, at, "version", &v.Version, readVia[Semver]),
		field(o, at, "kind", &v.Kind, readVia[RulebookKind]),
		field(o, at, "licences", &v.Licences, readVia[RulebookLicences]),
		field(o, at, "language", &v.Language, readVia[Language]),
		field(o, at, "docs_url", &v.DocsURL, ptrRead(readVia[URL])),
		field(o, at, "explorer_url", &v.ExplorerURL, ptrRead(readVia[URL])),
		field(o, at, "extends", &v.Extends, ptrRead(readVia[RulebookExtends])),
		field(o, at, "imports", &v.Imports, listRead(readVia[RulebookImports])),
		field(o, at, "owners", &v.Owners, listRead(readVia[Text])),
		field(o, at, "governance", &v.Governance, readVia[RulebookGovernance]),
		field(o, at, "format", &v.Format, readWhole),
		field(o, at, "successor", &v.Successor, ptrRead(readVia[Slug])),
	)
}

// RulebookKind is has the shape the schema gives it.
type RulebookKind string

// The values of RulebookKind.
const (
	RulebookKindBase      RulebookKind = "base"
	RulebookKindOverlay   RulebookKind = "overlay"
	RulebookKindCompanion RulebookKind = "companion"
	RulebookKindFork      RulebookKind = "fork"
	RulebookKindArchived  RulebookKind = "archived"
)

// ToCanon is v as canon's value tree, as encoding/json wrote RulebookKind.
func (v RulebookKind) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member RulebookKind has no
// field for, under its exact name, is refused.
func (v *RulebookKind) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RulebookKind) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = RulebookKind(x)
	return err
}

// RulebookLicences is has the shape the schema gives it.
type RulebookLicences struct {
	// Display only.
	// Since contract 1.0.
	Rules Text `json:"rules"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	CheckerCode TextOrEmpty `json:"checker_code,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Presets TextOrEmpty `json:"presets,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RulebookLicences.
func (v RulebookLicences) ToCanon() canon.Value {
	o := canon.Object{}
	o["rules"] = v.Rules.ToCanon()
	keep(o, "checker_code", v.CheckerCode.ToCanon(), v.CheckerCode != "")
	keep(o, "presets", v.Presets.ToCanon(), v.Presets != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RulebookLicences has no
// field for, under its exact name, is refused.
func (v *RulebookLicences) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RulebookLicences) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rules", "checker_code", "presets")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rules", &v.Rules, readVia[Text]),
		field(o, at, "checker_code", &v.CheckerCode, readVia[TextOrEmpty]),
		field(o, at, "presets", &v.Presets, readVia[TextOrEmpty]),
	)
}

// RulebookExtends is has the shape the schema gives it.
type RulebookExtends struct {
	// Display only.
	// Since contract 1.0.
	Rulebook Text `json:"rulebook"`
	// Display only.
	// Since contract 1.0.
	Version Semver `json:"version"`
	// Decides: the content hash the parent's release must have, so whether the fetched parent is accepted.
	// Since contract 1.0.
	// Absent means "".
	ContentHash DigestOrEmpty `json:"content_hash,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RulebookExtends.
func (v RulebookExtends) ToCanon() canon.Value {
	o := canon.Object{}
	o["rulebook"] = v.Rulebook.ToCanon()
	o["version"] = v.Version.ToCanon()
	keep(o, "content_hash", v.ContentHash.ToCanon(), v.ContentHash != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RulebookExtends has no
// field for, under its exact name, is refused.
func (v *RulebookExtends) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RulebookExtends) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rulebook", "version", "content_hash")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rulebook", &v.Rulebook, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Semver]),
		field(o, at, "content_hash", &v.ContentHash, readVia[DigestOrEmpty]),
	)
}

// RulebookImports is has the shape the schema gives it.
type RulebookImports struct {
	// Display only.
	// Since contract 1.0.
	Rulebook Text `json:"rulebook"`
	// Display only.
	// Since contract 1.0.
	Version Semver `json:"version"`
	// Display only.
	// Since contract 1.0.
	Parts []Slug `json:"parts"`
	// Decides: the content hash the import's release must have, so whether the fetched import is accepted.
	// Since contract 1.0.
	// Absent means "".
	ContentHash DigestOrEmpty `json:"content_hash,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RulebookImports.
func (v RulebookImports) ToCanon() canon.Value {
	o := canon.Object{}
	o["rulebook"] = v.Rulebook.ToCanon()
	o["version"] = v.Version.ToCanon()
	o["parts"] = listOf(v.Parts, Slug.ToCanon)
	keep(o, "content_hash", v.ContentHash.ToCanon(), v.ContentHash != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RulebookImports has no
// field for, under its exact name, is refused.
func (v *RulebookImports) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RulebookImports) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rulebook", "version", "parts", "content_hash")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rulebook", &v.Rulebook, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Semver]),
		field(o, at, "parts", &v.Parts, listRead(readVia[Slug])),
		field(o, at, "content_hash", &v.ContentHash, readVia[DigestOrEmpty]),
	)
}

// RulebookGovernanceContribution is has the shape the schema gives it.
type RulebookGovernanceContribution string

// The values of RulebookGovernanceContribution.
const (
	RulebookGovernanceContributionOpen      RulebookGovernanceContribution = "open"
	RulebookGovernanceContributionProposals RulebookGovernanceContribution = "proposals"
	RulebookGovernanceContributionClosed    RulebookGovernanceContribution = "closed"
)

// ToCanon is v as canon's value tree, as encoding/json wrote RulebookGovernanceContribution.
func (v RulebookGovernanceContribution) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member RulebookGovernanceContribution has no
// field for, under its exact name, is refused.
func (v *RulebookGovernanceContribution) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RulebookGovernanceContribution) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = RulebookGovernanceContribution(x)
	return err
}

// RulebookGovernance is has the shape the schema gives it.
type RulebookGovernance struct {
	// Display only.
	// Since contract 1.0.
	Contribution RulebookGovernanceContribution `json:"contribution"`
	// Display only.
	// Since contract 1.0.
	DecisionRequired bool `json:"decision_required"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 0.
	TrialReleases int `json:"trial_releases"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 1.
	ReviewDays int `json:"review_days"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RulebookGovernance.
func (v RulebookGovernance) ToCanon() canon.Value {
	o := canon.Object{}
	o["contribution"] = v.Contribution.ToCanon()
	o["decision_required"] = truthOf(v.DecisionRequired)
	o["trial_releases"] = wholeOf(v.TrialReleases)
	o["review_days"] = wholeOf(v.ReviewDays)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RulebookGovernance has no
// field for, under its exact name, is refused.
func (v *RulebookGovernance) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RulebookGovernance) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contribution", "decision_required", "trial_releases", "review_days")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contribution", &v.Contribution, readVia[RulebookGovernanceContribution]),
		field(o, at, "decision_required", &v.DecisionRequired, readTruth),
		field(o, at, "trial_releases", &v.TrialReleases, readWhole),
		field(o, at, "review_days", &v.ReviewDays, readWhole),
	)
}
