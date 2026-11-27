// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-checker-code.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// CheckerCode is Checker-code artifact (entity, spec 16.1).
type CheckerCode struct {
	// Display only.
	// Since contract 1.0.
	Name Slug `json:"name"`
	// Display only.
	// Since contract 1.0.
	Publisher Text `json:"publisher"`
	// Display only.
	// Since contract 1.0.
	Digest Digest `json:"digest"`
	// Display only.
	// Since contract 1.0.
	Tool Text `json:"tool"`
	// Display only.
	// Since contract 1.0.
	TestsPresent bool `json:"tests_present"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	ReviewedBy []Text `json:"reviewed_by,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Licence TextOrEmpty `json:"licence,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means false.
	ByReference bool `json:"by_reference,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote CheckerCode.
func (v CheckerCode) ToCanon() canon.Value {
	o := canon.Object{}
	o["name"] = v.Name.ToCanon()
	o["publisher"] = v.Publisher.ToCanon()
	o["digest"] = v.Digest.ToCanon()
	o["tool"] = v.Tool.ToCanon()
	o["tests_present"] = truthOf(v.TestsPresent)
	keep(o, "reviewed_by", listOf(v.ReviewedBy, Text.ToCanon), len(v.ReviewedBy) > 0)
	keep(o, "licence", v.Licence.ToCanon(), v.Licence != "")
	keep(o, "by_reference", truthOf(v.ByReference), v.ByReference)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member CheckerCode has no
// field for, under its exact name, is refused.
func (v *CheckerCode) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *CheckerCode) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "name", "publisher", "digest", "tool", "tests_present", "reviewed_by", "licence", "by_reference")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "name", &v.Name, readVia[Slug]),
		field(o, at, "publisher", &v.Publisher, readVia[Text]),
		field(o, at, "digest", &v.Digest, readVia[Digest]),
		field(o, at, "tool", &v.Tool, readVia[Text]),
		field(o, at, "tests_present", &v.TestsPresent, readTruth),
		field(o, at, "reviewed_by", &v.ReviewedBy, listRead(readVia[Text])),
		field(o, at, "licence", &v.Licence, readVia[TextOrEmpty]),
		field(o, at, "by_reference", &v.ByReference, readTruth),
	)
}
