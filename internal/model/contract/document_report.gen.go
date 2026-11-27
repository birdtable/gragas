// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-report.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// ReportDocument is The project report (document, spec 16.8): carries no project name, path, remote or author.
type ReportDocument struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	Versions Versions `json:"versions"`
	// Display only.
	// Since contract 1.0.
	Pin ReportDocumentPin `json:"pin"`
	// Display only.
	// Since contract 1.0.
	Release Release `json:"release"`
	// Display only.
	// Since contract 1.0.
	RunTime Timestamp `json:"run_time"`
	// Display only.
	// Since contract 1.0.
	LockDigest Digest `json:"lock_digest"`
	// Display only.
	// Since contract 1.0.
	Sections []ReportDocumentSections `json:"sections"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Waivers []Waiver `json:"waivers,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Baselines []Baseline `json:"baselines,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Drift []ReportDocumentDrift `json:"drift,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Mismatches []ReportDocumentMismatches `json:"mismatches,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Renderings []ReportDocumentRenderings `json:"renderings,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Wiring []ReportDocumentWiring `json:"wiring,omitempty"`
	// Display only.
	// Since contract 1.0.
	Trust ReportDocumentTrust `json:"trust"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	LastChange *ReportDocumentLastChange `json:"last_change,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocument.
func (v ReportDocument) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["versions"] = v.Versions.ToCanon()
	o["pin"] = v.Pin.ToCanon()
	o["release"] = v.Release.ToCanon()
	o["run_time"] = v.RunTime.ToCanon()
	o["lock_digest"] = v.LockDigest.ToCanon()
	o["sections"] = listOf(v.Sections, ReportDocumentSections.ToCanon)
	keep(o, "waivers", listOf(v.Waivers, Waiver.ToCanon), len(v.Waivers) > 0)
	keep(o, "baselines", listOf(v.Baselines, Baseline.ToCanon), len(v.Baselines) > 0)
	keep(o, "drift", listOf(v.Drift, ReportDocumentDrift.ToCanon), len(v.Drift) > 0)
	keep(o, "mismatches", listOf(v.Mismatches, ReportDocumentMismatches.ToCanon), len(v.Mismatches) > 0)
	keep(o, "renderings", listOf(v.Renderings, ReportDocumentRenderings.ToCanon), len(v.Renderings) > 0)
	keep(o, "wiring", listOf(v.Wiring, ReportDocumentWiring.ToCanon), len(v.Wiring) > 0)
	o["trust"] = v.Trust.ToCanon()
	keepPtr(o, "last_change", v.LastChange, ReportDocumentLastChange.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocument has no
// field for, under its exact name, is refused.
func (v *ReportDocument) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocument) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "versions", "pin", "release", "run_time", "lock_digest", "sections", "waivers", "baselines", "drift", "mismatches", "renderings", "wiring", "trust", "last_change")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "versions", &v.Versions, readVia[Versions]),
		field(o, at, "pin", &v.Pin, readVia[ReportDocumentPin]),
		field(o, at, "release", &v.Release, readVia[Release]),
		field(o, at, "run_time", &v.RunTime, readVia[Timestamp]),
		field(o, at, "lock_digest", &v.LockDigest, readVia[Digest]),
		field(o, at, "sections", &v.Sections, listRead(readVia[ReportDocumentSections])),
		field(o, at, "waivers", &v.Waivers, listRead(readVia[Waiver])),
		field(o, at, "baselines", &v.Baselines, listRead(readVia[Baseline])),
		field(o, at, "drift", &v.Drift, listRead(readVia[ReportDocumentDrift])),
		field(o, at, "mismatches", &v.Mismatches, listRead(readVia[ReportDocumentMismatches])),
		field(o, at, "renderings", &v.Renderings, listRead(readVia[ReportDocumentRenderings])),
		field(o, at, "wiring", &v.Wiring, listRead(readVia[ReportDocumentWiring])),
		field(o, at, "trust", &v.Trust, readVia[ReportDocumentTrust]),
		field(o, at, "last_change", &v.LastChange, ptrRead(readVia[ReportDocumentLastChange])),
	)
}

// ReportDocumentPin is has the shape the schema gives it.
type ReportDocumentPin struct {
	// Display only.
	// Since contract 1.0.
	Rulebook Text `json:"rulebook"`
	// Display only.
	// Since contract 1.0.
	Version Semver `json:"version"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentPin.
func (v ReportDocumentPin) ToCanon() canon.Value {
	o := canon.Object{}
	o["rulebook"] = v.Rulebook.ToCanon()
	o["version"] = v.Version.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentPin has no
// field for, under its exact name, is refused.
func (v *ReportDocumentPin) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentPin) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rulebook", "version")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rulebook", &v.Rulebook, readVia[Text]),
		field(o, at, "version", &v.Version, readVia[Semver]),
	)
}

// ReportDocumentSectionsPerRuleObservation is has the shape the schema gives it.
type ReportDocumentSectionsPerRuleObservation string

// The values of ReportDocumentSectionsPerRuleObservation.
const (
	ReportDocumentSectionsPerRuleObservationHeld        ReportDocumentSectionsPerRuleObservation = "held"
	ReportDocumentSectionsPerRuleObservationNotHeld     ReportDocumentSectionsPerRuleObservation = "not held"
	ReportDocumentSectionsPerRuleObservationNotObserved ReportDocumentSectionsPerRuleObservation = "not observed"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentSectionsPerRuleObservation.
func (v ReportDocumentSectionsPerRuleObservation) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentSectionsPerRuleObservation has no
// field for, under its exact name, is refused.
func (v *ReportDocumentSectionsPerRuleObservation) FromCanon(c canon.Value) error {
	return v.fromAt(c, "")
}

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentSectionsPerRuleObservation) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReportDocumentSectionsPerRuleObservation(x)
	return err
}

// ReportDocumentSectionsPerRuleProvenance is has the shape the schema gives it.
type ReportDocumentSectionsPerRuleProvenance struct {
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Layer *Slug `json:"layer,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Decision *Slug `json:"decision,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Switch *Slug `json:"switch,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentSectionsPerRuleProvenance.
func (v ReportDocumentSectionsPerRuleProvenance) ToCanon() canon.Value {
	o := canon.Object{}
	keepPtr(o, "layer", v.Layer, Slug.ToCanon)
	keepPtr(o, "decision", v.Decision, Slug.ToCanon)
	keepPtr(o, "switch", v.Switch, Slug.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentSectionsPerRuleProvenance has no
// field for, under its exact name, is refused.
func (v *ReportDocumentSectionsPerRuleProvenance) FromCanon(c canon.Value) error {
	return v.fromAt(c, "")
}

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentSectionsPerRuleProvenance) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "layer", "decision", "switch")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "layer", &v.Layer, ptrRead(readVia[Slug])),
		field(o, at, "decision", &v.Decision, ptrRead(readVia[Slug])),
		field(o, at, "switch", &v.Switch, ptrRead(readVia[Slug])),
	)
}

// ReportDocumentSectionsPerRule is has the shape the schema gives it.
type ReportDocumentSectionsPerRule struct {
	// Display only.
	// Since contract 1.0.
	Rule ID `json:"rule"`
	// Display only.
	// Since contract 1.0.
	Standing Standing `json:"standing"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Marks []Slug `json:"marks,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "held".
	Observation *ReportDocumentSectionsPerRuleObservation `json:"observation,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Provenance *ReportDocumentSectionsPerRuleProvenance `json:"provenance,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Bindings []Text `json:"bindings,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentSectionsPerRule.
func (v ReportDocumentSectionsPerRule) ToCanon() canon.Value {
	o := canon.Object{}
	o["rule"] = v.Rule.ToCanon()
	o["standing"] = v.Standing.ToCanon()
	keep(o, "marks", listOf(v.Marks, Slug.ToCanon), len(v.Marks) > 0)
	keepPtr(o, "observation", v.Observation, ReportDocumentSectionsPerRuleObservation.ToCanon)
	keepPtr(o, "provenance", v.Provenance, ReportDocumentSectionsPerRuleProvenance.ToCanon)
	keep(o, "bindings", listOf(v.Bindings, Text.ToCanon), len(v.Bindings) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentSectionsPerRule has no
// field for, under its exact name, is refused.
func (v *ReportDocumentSectionsPerRule) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentSectionsPerRule) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rule", "standing", "marks", "observation", "provenance", "bindings")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rule", &v.Rule, readVia[ID]),
		field(o, at, "standing", &v.Standing, readVia[Standing]),
		field(o, at, "marks", &v.Marks, listRead(readVia[Slug])),
		field(o, at, "observation", &v.Observation, ptrRead(readVia[ReportDocumentSectionsPerRuleObservation])),
		field(o, at, "provenance", &v.Provenance, ptrRead(readVia[ReportDocumentSectionsPerRuleProvenance])),
		field(o, at, "bindings", &v.Bindings, listRead(readVia[Text])),
	)
}

// ReportDocumentSections is has the shape the schema gives it.
type ReportDocumentSections struct {
	// Display only.
	// Since contract 1.0.
	Manifest Digest `json:"manifest"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Name TextOrEmpty `json:"name,omitempty"`
	// Display only.
	// Since contract 1.0.
	PerRule []ReportDocumentSectionsPerRule `json:"per_rule"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentSections.
func (v ReportDocumentSections) ToCanon() canon.Value {
	o := canon.Object{}
	o["manifest"] = v.Manifest.ToCanon()
	keep(o, "name", v.Name.ToCanon(), v.Name != "")
	o["per_rule"] = listOf(v.PerRule, ReportDocumentSectionsPerRule.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentSections has no
// field for, under its exact name, is refused.
func (v *ReportDocumentSections) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentSections) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "manifest", "name", "per_rule")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "manifest", &v.Manifest, readVia[Digest]),
		field(o, at, "name", &v.Name, readVia[TextOrEmpty]),
		field(o, at, "per_rule", &v.PerRule, listRead(readVia[ReportDocumentSectionsPerRule])),
	)
}

// ReportDocumentDriftKind is has the shape the schema gives it.
type ReportDocumentDriftKind string

// The values of ReportDocumentDriftKind.
const (
	ReportDocumentDriftKindWeaker     ReportDocumentDriftKind = "weaker"
	ReportDocumentDriftKindTighter    ReportDocumentDriftKind = "tighter"
	ReportDocumentDriftKindAbsent     ReportDocumentDriftKind = "absent"
	ReportDocumentDriftKindUnmapped   ReportDocumentDriftKind = "unmapped"
	ReportDocumentDriftKindEditedCopy ReportDocumentDriftKind = "edited copy"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentDriftKind.
func (v ReportDocumentDriftKind) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentDriftKind has no
// field for, under its exact name, is refused.
func (v *ReportDocumentDriftKind) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentDriftKind) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReportDocumentDriftKind(x)
	return err
}

// ReportDocumentDrift is has the shape the schema gives it.
type ReportDocumentDrift struct {
	// Display only.
	// Since contract 1.0.
	Entry Text `json:"entry"`
	// Decides: the fidelity finding the report states for its entry.
	// Since contract 1.0.
	Kind ReportDocumentDriftKind `json:"kind"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentDrift.
func (v ReportDocumentDrift) ToCanon() canon.Value {
	o := canon.Object{}
	o["entry"] = v.Entry.ToCanon()
	o["kind"] = v.Kind.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentDrift has no
// field for, under its exact name, is refused.
func (v *ReportDocumentDrift) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentDrift) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "entry", "kind")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "entry", &v.Entry, readVia[Text]),
		field(o, at, "kind", &v.Kind, readVia[ReportDocumentDriftKind]),
	)
}

// ReportDocumentMismatchesWhat is has the shape the schema gives it.
type ReportDocumentMismatchesWhat string

// The values of ReportDocumentMismatchesWhat.
const (
	ReportDocumentMismatchesWhatTool    ReportDocumentMismatchesWhat = "tool"
	ReportDocumentMismatchesWhatPreset  ReportDocumentMismatchesWhat = "preset"
	ReportDocumentMismatchesWhatAdapter ReportDocumentMismatchesWhat = "adapter"
	ReportDocumentMismatchesWhatEngine  ReportDocumentMismatchesWhat = "engine"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentMismatchesWhat.
func (v ReportDocumentMismatchesWhat) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentMismatchesWhat has no
// field for, under its exact name, is refused.
func (v *ReportDocumentMismatchesWhat) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentMismatchesWhat) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReportDocumentMismatchesWhat(x)
	return err
}

// ReportDocumentMismatches is has the shape the schema gives it.
type ReportDocumentMismatches struct {
	// Display only.
	// Since contract 1.0.
	What ReportDocumentMismatchesWhat `json:"what"`
	// Display only.
	// Since contract 1.0.
	Found Text `json:"found"`
	// Display only.
	// Since contract 1.0.
	Expected Text `json:"expected"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentMismatches.
func (v ReportDocumentMismatches) ToCanon() canon.Value {
	o := canon.Object{}
	o["what"] = v.What.ToCanon()
	o["found"] = v.Found.ToCanon()
	o["expected"] = v.Expected.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentMismatches has no
// field for, under its exact name, is refused.
func (v *ReportDocumentMismatches) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentMismatches) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "what", "found", "expected")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "what", &v.What, readVia[ReportDocumentMismatchesWhat]),
		field(o, at, "found", &v.Found, readVia[Text]),
		field(o, at, "expected", &v.Expected, readVia[Text]),
	)
}

// ReportDocumentRenderingsState is has the shape the schema gives it.
type ReportDocumentRenderingsState string

// The values of ReportDocumentRenderingsState.
const (
	ReportDocumentRenderingsStateStale  ReportDocumentRenderingsState = "stale"
	ReportDocumentRenderingsStateEdited ReportDocumentRenderingsState = "edited"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentRenderingsState.
func (v ReportDocumentRenderingsState) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentRenderingsState has no
// field for, under its exact name, is refused.
func (v *ReportDocumentRenderingsState) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentRenderingsState) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReportDocumentRenderingsState(x)
	return err
}

// ReportDocumentRenderings is has the shape the schema gives it.
type ReportDocumentRenderings struct {
	// Display only.
	// Since contract 1.0.
	Path Text `json:"path"`
	// Display only.
	// Since contract 1.0.
	State ReportDocumentRenderingsState `json:"state"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentRenderings.
func (v ReportDocumentRenderings) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = v.Path.ToCanon()
	o["state"] = v.State.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentRenderings has no
// field for, under its exact name, is refused.
func (v *ReportDocumentRenderings) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentRenderings) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "path", "state")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "path", &v.Path, readVia[Text]),
		field(o, at, "state", &v.State, readVia[ReportDocumentRenderingsState]),
	)
}

// ReportDocumentWiring is has the shape the schema gives it.
type ReportDocumentWiring struct {
	// Display only.
	// Since contract 1.0.
	Line Text `json:"line"`
	// Display only.
	// Since contract 1.0.
	Present bool `json:"present"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentWiring.
func (v ReportDocumentWiring) ToCanon() canon.Value {
	o := canon.Object{}
	o["line"] = v.Line.ToCanon()
	o["present"] = truthOf(v.Present)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentWiring has no
// field for, under its exact name, is refused.
func (v *ReportDocumentWiring) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentWiring) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "line", "present")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "line", &v.Line, readVia[Text]),
		field(o, at, "present", &v.Present, readTruth),
	)
}

// ReportDocumentTrustSignature is has the shape the schema gives it.
type ReportDocumentTrustSignature string

// The values of ReportDocumentTrustSignature.
const (
	ReportDocumentTrustSignatureVerified         ReportDocumentTrustSignature = "verified"
	ReportDocumentTrustSignatureUnsignedAccepted ReportDocumentTrustSignature = "unsigned-accepted"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentTrustSignature.
func (v ReportDocumentTrustSignature) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentTrustSignature has no
// field for, under its exact name, is refused.
func (v *ReportDocumentTrustSignature) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentTrustSignature) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReportDocumentTrustSignature(x)
	return err
}

// ReportDocumentTrustReleaseState is has the shape the schema gives it.
type ReportDocumentTrustReleaseState string

// The values of ReportDocumentTrustReleaseState.
const (
	ReportDocumentTrustReleaseStateCurrent   ReportDocumentTrustReleaseState = "current"
	ReportDocumentTrustReleaseStateWithdrawn ReportDocumentTrustReleaseState = "withdrawn"
	ReportDocumentTrustReleaseStateArchived  ReportDocumentTrustReleaseState = "archived"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentTrustReleaseState.
func (v ReportDocumentTrustReleaseState) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentTrustReleaseState has no
// field for, under its exact name, is refused.
func (v *ReportDocumentTrustReleaseState) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentTrustReleaseState) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReportDocumentTrustReleaseState(x)
	return err
}

// ReportDocumentTrust is has the shape the schema gives it.
type ReportDocumentTrust struct {
	// Display only.
	// Since contract 1.0.
	Signature ReportDocumentTrustSignature `json:"signature"`
	// Display only.
	// Since contract 1.0.
	DigestsMatched bool `json:"digests_matched"`
	// Display only.
	// Since contract 1.0.
	ReleaseState ReportDocumentTrustReleaseState `json:"release_state"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentTrust.
func (v ReportDocumentTrust) ToCanon() canon.Value {
	o := canon.Object{}
	o["signature"] = v.Signature.ToCanon()
	o["digests_matched"] = truthOf(v.DigestsMatched)
	o["release_state"] = v.ReleaseState.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentTrust has no
// field for, under its exact name, is refused.
func (v *ReportDocumentTrust) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentTrust) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "signature", "digests_matched", "release_state")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "signature", &v.Signature, readVia[ReportDocumentTrustSignature]),
		field(o, at, "digests_matched", &v.DigestsMatched, readTruth),
		field(o, at, "release_state", &v.ReleaseState, readVia[ReportDocumentTrustReleaseState]),
	)
}

// ReportDocumentLastChangeKind is has the shape the schema gives it.
type ReportDocumentLastChangeKind string

// The values of ReportDocumentLastChangeKind.
const (
	ReportDocumentLastChangeKindUpgrade ReportDocumentLastChangeKind = "upgrade"
	ReportDocumentLastChangeKindSwitch  ReportDocumentLastChangeKind = "switch"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentLastChangeKind.
func (v ReportDocumentLastChangeKind) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentLastChangeKind has no
// field for, under its exact name, is refused.
func (v *ReportDocumentLastChangeKind) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentLastChangeKind) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ReportDocumentLastChangeKind(x)
	return err
}

// ReportDocumentLastChange is has the shape the schema gives it.
type ReportDocumentLastChange struct {
	// Display only.
	// Since contract 1.0.
	Kind ReportDocumentLastChangeKind `json:"kind"`
	// Display only.
	// Since contract 1.0.
	When Timestamp `json:"when"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ReportDocumentLastChange.
func (v ReportDocumentLastChange) ToCanon() canon.Value {
	o := canon.Object{}
	o["kind"] = v.Kind.ToCanon()
	o["when"] = v.When.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ReportDocumentLastChange has no
// field for, under its exact name, is refused.
func (v *ReportDocumentLastChange) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ReportDocumentLastChange) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "kind", "when")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "kind", &v.Kind, readVia[ReportDocumentLastChangeKind]),
		field(o, at, "when", &v.When, readVia[Timestamp]),
	)
}
