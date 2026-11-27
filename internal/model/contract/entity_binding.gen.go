// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-binding.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Binding is Binding of a rule to a tool key (entity, spec 16.1).
type Binding struct {
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	Rule ID `json:"rule"`
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	Tool Text `json:"tool"`
	// the tool's key the binding sets; at rung 6 the review question the reviewer must answer (S51).
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	Key Text `json:"key"`
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	TargetType TargetType `json:"target_type"`
	// the value the binding gives the key; at rung 6 text that says when the review question applies (S51).
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	// Absent means null.
	Value canon.Value `json:"value,omitempty"`
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	// Absent means {}.
	Options canon.Object `json:"options,omitempty"`
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	Severity Severity `json:"severity"`
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	Rung Rung `json:"rung"`
	// Decides: what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	RunPoint RunPoint `json:"run_point"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Remedy TextOrEmpty `json:"remedy,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	// Constraints: at least 1.
	Pair *int `json:"pair,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	PairPath TextOrEmpty `json:"pair_path,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Proof *ProofResult `json:"proof,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Tightens *ID `json:"tightens,omitempty"`
	// the name of the rulebook's checker-code declaration the binding's entry comes from; null for a tool's own entry.
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Checker *Slug `json:"checker,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Binding.
func (v Binding) ToCanon() canon.Value {
	o := canon.Object{}
	o["rule"] = v.Rule.ToCanon()
	o["tool"] = v.Tool.ToCanon()
	o["key"] = v.Key.ToCanon()
	o["target_type"] = v.TargetType.ToCanon()
	keepValue(o, "value", v.Value)
	keep(o, "options", objectOf(v.Options), len(v.Options) > 0)
	o["severity"] = v.Severity.ToCanon()
	o["rung"] = v.Rung.ToCanon()
	o["run_point"] = v.RunPoint.ToCanon()
	keep(o, "remedy", v.Remedy.ToCanon(), v.Remedy != "")
	keepPtr(o, "pair", v.Pair, wholeOf)
	keep(o, "pair_path", v.PairPath.ToCanon(), v.PairPath != "")
	keepPtr(o, "proof", v.Proof, ProofResult.ToCanon)
	keepPtr(o, "tightens", v.Tightens, ID.ToCanon)
	keepPtr(o, "checker", v.Checker, Slug.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Binding has no
// field for, under its exact name, is refused.
func (v *Binding) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Binding) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rule", "tool", "key", "target_type", "value", "options", "severity", "rung", "run_point", "remedy", "pair", "pair_path", "proof", "tightens", "checker")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rule", &v.Rule, readVia[ID]),
		field(o, at, "tool", &v.Tool, readVia[Text]),
		field(o, at, "key", &v.Key, readVia[Text]),
		field(o, at, "target_type", &v.TargetType, readVia[TargetType]),
		field(o, at, "value", &v.Value, readValue),
		field(o, at, "options", &v.Options, readObject),
		field(o, at, "severity", &v.Severity, readVia[Severity]),
		field(o, at, "rung", &v.Rung, readVia[Rung]),
		field(o, at, "run_point", &v.RunPoint, readVia[RunPoint]),
		field(o, at, "remedy", &v.Remedy, readVia[TextOrEmpty]),
		field(o, at, "pair", &v.Pair, ptrRead(readWhole)),
		field(o, at, "pair_path", &v.PairPath, readVia[TextOrEmpty]),
		field(o, at, "proof", &v.Proof, ptrRead(readVia[ProofResult])),
		field(o, at, "tightens", &v.Tightens, ptrRead(readVia[ID])),
		field(o, at, "checker", &v.Checker, ptrRead(readVia[Slug])),
	)
}
