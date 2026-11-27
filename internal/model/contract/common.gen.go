// Written by gragas-generate from internal/host/assets/contract/schema/v1/common.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Contract is the contract version a document is written in: the major is this folder's, the minor is the document's; a reader that finds a newer major stops and names both versions.
// Constraints: matches ^1\.(0|[1-9][0-9]*)$.
type Contract string

// ToCanon is v as canon's value tree, as encoding/json wrote Contract.
func (v Contract) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Contract has no
// field for, under its exact name, is refused.
func (v *Contract) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Contract) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Contract(x)
	return err
}

// Version is has the shape the schema gives it.
// Constraints: matches ^[0-9]+\.[0-9]+(\.[0-9]+)?$.
type Version string

// ToCanon is v as canon's value tree, as encoding/json wrote Version.
func (v Version) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Version has no
// field for, under its exact name, is refused.
func (v *Version) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Version) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Version(x)
	return err
}

// Versions is the three versions every report and bundle carries.
type Versions struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 1.
	Format int `json:"format"`
	// Display only.
	// Since contract 1.0.
	Engine Semver `json:"engine"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Versions.
func (v Versions) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["format"] = wholeOf(v.Format)
	o["engine"] = v.Engine.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Versions has no
// field for, under its exact name, is refused.
func (v *Versions) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Versions) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "format", "engine")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "format", &v.Format, readWhole),
		field(o, at, "engine", &v.Engine, readVia[Semver]),
	)
}

// ID is has the shape the schema gives it.
// Constraints: matches ^[a-z]+\.[a-z0-9]+(-[a-z0-9]+)*\.[1-9][0-9]*$.
type ID string

// ToCanon is v as canon's value tree, as encoding/json wrote ID.
func (v ID) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ID has no
// field for, under its exact name, is refused.
func (v *ID) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ID) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ID(x)
	return err
}

// Slug is has the shape the schema gives it.
// Constraints: matches ^[a-z0-9]+(-[a-z0-9]+)*$.
type Slug string

// ToCanon is v as canon's value tree, as encoding/json wrote Slug.
func (v Slug) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Slug has no
// field for, under its exact name, is refused.
func (v *Slug) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Slug) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Slug(x)
	return err
}

// Text is has the shape the schema gives it.
// Constraints: at least 1 characters.
type Text string

// ToCanon is v as canon's value tree, as encoding/json wrote Text.
func (v Text) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Text has no
// field for, under its exact name, is refused.
func (v *Text) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Text) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Text(x)
	return err
}

// Date is has the shape the schema gives it.
// Constraints: matches ^[0-9]{4}-[0-9]{2}-[0-9]{2}$.
type Date string

// ToCanon is v as canon's value tree, as encoding/json wrote Date.
func (v Date) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Date has no
// field for, under its exact name, is refused.
func (v *Date) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Date) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Date(x)
	return err
}

// Timestamp is UTC, to the second.
// Constraints: matches ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$.
type Timestamp string

// ToCanon is v as canon's value tree, as encoding/json wrote Timestamp.
func (v Timestamp) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Timestamp has no
// field for, under its exact name, is refused.
func (v *Timestamp) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Timestamp) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Timestamp(x)
	return err
}

// Semver is has the shape the schema gives it.
// Constraints: matches ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$.
type Semver string

// ToCanon is v as canon's value tree, as encoding/json wrote Semver.
func (v Semver) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Semver has no
// field for, under its exact name, is refused.
func (v *Semver) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Semver) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Semver(x)
	return err
}

// Digest is has the shape the schema gives it.
// Constraints: matches ^sha256:[0-9a-f]{64}$.
type Digest string

// ToCanon is v as canon's value tree, as encoding/json wrote Digest.
func (v Digest) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Digest has no
// field for, under its exact name, is refused.
func (v *Digest) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Digest) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Digest(x)
	return err
}

// Language is has the shape the schema gives it.
// Constraints: matches ^[a-z]{2,3}(-[A-Za-z0-9]+)*$.
type Language string

// ToCanon is v as canon's value tree, as encoding/json wrote Language.
func (v Language) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Language has no
// field for, under its exact name, is refused.
func (v *Language) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Language) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Language(x)
	return err
}

// URL is has the shape the schema gives it.
// Constraints: matches ^https://[^\s]+$.
type URL string

// ToCanon is v as canon's value tree, as encoding/json wrote URL.
func (v URL) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member URL has no
// field for, under its exact name, is refused.
func (v *URL) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *URL) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = URL(x)
	return err
}

// Reason is plain words, display only.
// Constraints: at least 1 characters.
type Reason string

// ToCanon is v as canon's value tree, as encoding/json wrote Reason.
func (v Reason) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Reason has no
// field for, under its exact name, is refused.
func (v *Reason) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Reason) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Reason(x)
	return err
}

// Strength is has the shape the schema gives it.
type Strength string

// The values of Strength.
const (
	StrengthMust   Strength = "must"
	StrengthShould Strength = "should"
	StrengthMay    Strength = "may"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Strength.
func (v Strength) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Strength has no
// field for, under its exact name, is refused.
func (v *Strength) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Strength) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Strength(x)
	return err
}

// Bump is has the shape the schema gives it.
type Bump string

// The values of Bump.
const (
	BumpMajor Bump = "major"
	BumpMinor Bump = "minor"
	BumpPatch Bump = "patch"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Bump.
func (v Bump) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Bump has no
// field for, under its exact name, is refused.
func (v *Bump) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Bump) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Bump(x)
	return err
}

// Status is has the shape the schema gives it.
type Status string

// The values of Status.
const (
	StatusProposed Status = "proposed"
	StatusTrial    Status = "trial"
	StatusActive   Status = "active"
	StatusRetired  Status = "retired"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Status.
func (v Status) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Status has no
// field for, under its exact name, is refused.
func (v *Status) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Status) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Status(x)
	return err
}

// Origin is has the shape the schema gives it.
type Origin string

// The values of Origin.
const (
	OriginAuthored Origin = "authored"
	OriginImported Origin = "imported"
	OriginMigrated Origin = "migrated"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Origin.
func (v Origin) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Origin has no
// field for, under its exact name, is refused.
func (v *Origin) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Origin) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Origin(x)
	return err
}

// Rung is has the shape the schema gives it.
// Constraints: at least 0, at most 6.
type Rung int

// ToCanon is v as canon's value tree, as encoding/json wrote Rung.
func (v Rung) ToCanon() canon.Value {
	return wholeOf(int(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Rung has no
// field for, under its exact name, is refused.
func (v *Rung) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Rung) fromAt(c canon.Value, at string) error {
	x, err := readWhole(c, at)
	*v = Rung(x)
	return err
}

// RunPoint is has the shape the schema gives it.
type RunPoint string

// The values of RunPoint.
const (
	RunPointEdit    RunPoint = "edit"
	RunPointCommit  RunPoint = "commit"
	RunPointPush    RunPoint = "push"
	RunPointLand    RunPoint = "land"
	RunPointReview  RunPoint = "review"
	RunPointRelease RunPoint = "release"
	RunPointDeploy  RunPoint = "deploy"
)

// ToCanon is v as canon's value tree, as encoding/json wrote RunPoint.
func (v RunPoint) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member RunPoint has no
// field for, under its exact name, is refused.
func (v *RunPoint) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RunPoint) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = RunPoint(x)
	return err
}

// TargetType is has the shape the schema gives it.
type TargetType string

// The values of TargetType.
const (
	TargetTypeSource            TargetType = "source"
	TargetTypeConfig            TargetType = "config"
	TargetTypeMigration         TargetType = "migration"
	TargetTypeContainer         TargetType = "container"
	TargetTypeWorkflow          TargetType = "workflow"
	TargetTypeIac               TargetType = "iac"
	TargetTypeAPISpec           TargetType = "api-spec"
	TargetTypeProse             TargetType = "prose"
	TargetTypeCommit            TargetType = "commit"
	TargetTypeTelemetryRegistry TargetType = "telemetry-registry"
	TargetTypeDependency        TargetType = "dependency"
	TargetTypePackage           TargetType = "package"
)

// ToCanon is v as canon's value tree, as encoding/json wrote TargetType.
func (v TargetType) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member TargetType has no
// field for, under its exact name, is refused.
func (v *TargetType) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *TargetType) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = TargetType(x)
	return err
}

// Severity is has the shape the schema gives it.
type Severity string

// The values of Severity.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Severity.
func (v Severity) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Severity has no
// field for, under its exact name, is refused.
func (v *Severity) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Severity) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Severity(x)
	return err
}

// Standing is derived from status, bindings and proof; a document carries it, no person types it.
type Standing string

// The values of Standing.
const (
	StandingProposed Standing = "proposed"
	StandingEnforced Standing = "enforced"
	StandingReviewed Standing = "reviewed"
	StandingPlanned  Standing = "planned"
	StandingAdvisory Standing = "advisory"
	StandingRetired  Standing = "retired"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Standing.
func (v Standing) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Standing has no
// field for, under its exact name, is refused.
func (v *Standing) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Standing) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Standing(x)
	return err
}

// StandingMark is a mark beside a standing, derived like it and never replacing it (S32).
type StandingMark string

// The values of StandingMark.
const (
	StandingMarkTrial    StandingMark = "trial"
	StandingMarkBlocked  StandingMark = "blocked"
	StandingMarkUnproven StandingMark = "unproven"
)

// ToCanon is v as canon's value tree, as encoding/json wrote StandingMark.
func (v StandingMark) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member StandingMark has no
// field for, under its exact name, is refused.
func (v *StandingMark) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *StandingMark) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = StandingMark(x)
	return err
}

// Deprecation is the window of a deprecated shape: x-deprecated on a field carries this.
type Deprecation struct {
	// Display only.
	// Since contract 1.0.
	Since Contract `json:"since"`
	// the major that removes the old shape, two minors after both shapes were first emitted.
	// Display only.
	// Since contract 1.0.
	// Constraints: matches ^[2-9][0-9]*\.0$.
	RemovedIn string `json:"removed_in"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Deprecation.
func (v Deprecation) ToCanon() canon.Value {
	o := canon.Object{}
	o["since"] = v.Since.ToCanon()
	o["removed_in"] = textOf(v.RemovedIn)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Deprecation has no
// field for, under its exact name, is refused.
func (v *Deprecation) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Deprecation) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "since", "removed_in")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "since", &v.Since, readVia[Contract]),
		field(o, at, "removed_in", &v.RemovedIn, readText),
	)
}

// Mark is has the shape the schema gives it.
type Mark string

// The values of Mark.
const (
	MarkSame    Mark = "same"
	MarkTighter Mark = "tighter"
	MarkWeaker  Mark = "weaker"
	MarkNone    Mark = "none"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Mark.
func (v Mark) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Mark has no
// field for, under its exact name, is refused.
func (v *Mark) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Mark) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Mark(x)
	return err
}

// Result is has the shape the schema gives it.
type Result string

// The values of Result.
const (
	ResultPass     Result = "pass"
	ResultFail     Result = "fail"
	ResultNotRun   Result = "not run"
	ResultReviewed Result = "reviewed"
)

// ToCanon is v as canon's value tree, as encoding/json wrote Result.
func (v Result) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member Result has no
// field for, under its exact name, is refused.
func (v *Result) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Result) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = Result(x)
	return err
}

// TextOrEmpty is text whose absence means empty: the one text shape that takes the empty string.
type TextOrEmpty string

// ToCanon is v as canon's value tree, as encoding/json wrote TextOrEmpty.
func (v TextOrEmpty) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member TextOrEmpty has no
// field for, under its exact name, is refused.
func (v *TextOrEmpty) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *TextOrEmpty) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = TextOrEmpty(x)
	return err
}

// DigestOrEmpty is a digest whose absence means empty: no digest.
// Constraints: matches ^(sha256:[0-9a-f]{64})?$.
type DigestOrEmpty string

// ToCanon is v as canon's value tree, as encoding/json wrote DigestOrEmpty.
func (v DigestOrEmpty) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member DigestOrEmpty has no
// field for, under its exact name, is refused.
func (v *DigestOrEmpty) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *DigestOrEmpty) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = DigestOrEmpty(x)
	return err
}
