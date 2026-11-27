// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-proof-result.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// ProofResult is Proof result of one binding under one tool version (entity, spec 16.1).
type ProofResult struct {
	// Display only.
	// Since contract 1.0.
	Binding Text `json:"binding"`
	// Display only.
	// Since contract 1.0.
	ToolVersion Text `json:"tool_version"`
	// Display only.
	// Since contract 1.0.
	Result Result `json:"result"`
	// Display only.
	// Since contract 1.0.
	// Absent means 0.
	Rung Rung `json:"rung,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "fixture".
	RunLevel *ProofResultRunLevel `json:"run_level,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	AdapterVersion TextOrEmpty `json:"adapter_version,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "no pair".
	Reason *ProofResultReason `json:"reason,omitempty"`
	// Decides: whether the cell is run again: a cell keeps its result only while its inputs equal the digest of what it would be proven on now; a cell with none is always run again.
	// Since contract 1.0.
	// Absent means "".
	Inputs DigestOrEmpty `json:"inputs,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Tool TextOrEmpty `json:"tool,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote ProofResult.
func (v ProofResult) ToCanon() canon.Value {
	o := canon.Object{}
	o["binding"] = v.Binding.ToCanon()
	o["tool_version"] = v.ToolVersion.ToCanon()
	o["result"] = v.Result.ToCanon()
	keep(o, "rung", v.Rung.ToCanon(), v.Rung != 0)
	keepPtr(o, "run_level", v.RunLevel, ProofResultRunLevel.ToCanon)
	keep(o, "adapter_version", v.AdapterVersion.ToCanon(), v.AdapterVersion != "")
	keepPtr(o, "reason", v.Reason, ProofResultReason.ToCanon)
	keep(o, "inputs", v.Inputs.ToCanon(), v.Inputs != "")
	keep(o, "tool", v.Tool.ToCanon(), v.Tool != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member ProofResult has no
// field for, under its exact name, is refused.
func (v *ProofResult) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProofResult) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "binding", "tool_version", "result", "rung", "run_level", "adapter_version", "reason", "inputs", "tool")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "binding", &v.Binding, readVia[Text]),
		field(o, at, "tool_version", &v.ToolVersion, readVia[Text]),
		field(o, at, "result", &v.Result, readVia[Result]),
		field(o, at, "rung", &v.Rung, readVia[Rung]),
		field(o, at, "run_level", &v.RunLevel, ptrRead(readVia[ProofResultRunLevel])),
		field(o, at, "adapter_version", &v.AdapterVersion, readVia[TextOrEmpty]),
		field(o, at, "reason", &v.Reason, ptrRead(readVia[ProofResultReason])),
		field(o, at, "inputs", &v.Inputs, readVia[DigestOrEmpty]),
		field(o, at, "tool", &v.Tool, readVia[TextOrEmpty]),
	)
}

// ProofResultRunLevel is has the shape the schema gives it.
type ProofResultRunLevel string

// The values of ProofResultRunLevel.
const (
	ProofResultRunLevelFixture ProofResultRunLevel = "fixture"
	ProofResultRunLevelPair    ProofResultRunLevel = "pair"
	ProofResultRunLevelProject ProofResultRunLevel = "project"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ProofResultRunLevel.
func (v ProofResultRunLevel) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ProofResultRunLevel has no
// field for, under its exact name, is refused.
func (v *ProofResultRunLevel) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProofResultRunLevel) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ProofResultRunLevel(x)
	return err
}

// ProofResultReason is has the shape the schema gives it.
type ProofResultReason string

// The values of ProofResultReason.
const (
	ProofResultReasonNoPair         ProofResultReason = "no pair"
	ProofResultReasonToolMissing    ProofResultReason = "tool missing"
	ProofResultReasonLimitHit       ProofResultReason = "limit hit"
	ProofResultReasonSandboxMissing ProofResultReason = "sandbox missing"
)

// ToCanon is v as canon's value tree, as encoding/json wrote ProofResultReason.
func (v ProofResultReason) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member ProofResultReason has no
// field for, under its exact name, is refused.
func (v *ProofResultReason) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *ProofResultReason) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = ProofResultReason(x)
	return err
}
