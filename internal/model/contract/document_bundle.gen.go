// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-bundle.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// BundleDocument is The rulebook bundle (document, spec 16.9).
type BundleDocument struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	Versions Versions `json:"versions"`
	// Display only.
	// Since contract 1.0.
	Release Release `json:"release"`
	// false for a working build marked unreleased.
	// Display only.
	// Since contract 1.0.
	Released bool `json:"released"`
	// Display only.
	// Since contract 1.0.
	Identity Rulebook `json:"identity"`
	// Display only.
	// Since contract 1.0.
	Structure Structure `json:"structure"`
	// Display only.
	// Since contract 1.0.
	Terms []Term `json:"terms"`
	// Decides: which rules a parent holds, so which an overlay may exclude or include.
	// Since contract 1.0.
	Rules []Rule `json:"rules"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Exclusions []Exclusion `json:"exclusions,omitempty"`
	// Decides: which bindings a parent or an import lends, so what an overlay's or an importer's binding is held to, S70.
	// Since contract 1.0.
	Bindings []Binding `json:"bindings"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	AdaptersUsed []BundleDocumentAdaptersUsed `json:"adapters_used,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Presets []Preset `json:"presets,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	CheckerCode []CheckerCode `json:"checker_code,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	ProofGrid []ProofResult `json:"proof_grid,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Coverage *BundleDocumentCoverage `json:"coverage,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Conflicts []Finding `json:"conflicts,omitempty"`
	// Display only.
	// Since contract 1.0.
	Decisions []Decision `json:"decisions"`
	// Display only.
	// Since contract 1.0.
	Sources []Source `json:"sources"`
	// Display only.
	// Since contract 1.0.
	History []HistoryEntry `json:"history"`
	// Display only.
	// Since contract 1.0.
	Changelog []ChangelogEntry `json:"changelog"`
	// Display only.
	// Since contract 1.0.
	TestedSet TestedSet `json:"tested_set"`
	// Display only.
	// Since contract 1.0.
	Health Health `json:"health"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	RuleMap []RuleMap `json:"rule_map,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	RenderingManifest []RenderingManifest `json:"rendering_manifest,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote BundleDocument.
func (v BundleDocument) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["versions"] = v.Versions.ToCanon()
	o["release"] = v.Release.ToCanon()
	o["released"] = truthOf(v.Released)
	o["identity"] = v.Identity.ToCanon()
	o["structure"] = v.Structure.ToCanon()
	o["terms"] = listOf(v.Terms, Term.ToCanon)
	o["rules"] = listOf(v.Rules, Rule.ToCanon)
	keep(o, "exclusions", listOf(v.Exclusions, Exclusion.ToCanon), len(v.Exclusions) > 0)
	o["bindings"] = listOf(v.Bindings, Binding.ToCanon)
	keep(o, "adapters_used", listOf(v.AdaptersUsed, BundleDocumentAdaptersUsed.ToCanon), len(v.AdaptersUsed) > 0)
	keep(o, "presets", listOf(v.Presets, Preset.ToCanon), len(v.Presets) > 0)
	keep(o, "checker_code", listOf(v.CheckerCode, CheckerCode.ToCanon), len(v.CheckerCode) > 0)
	keep(o, "proof_grid", listOf(v.ProofGrid, ProofResult.ToCanon), len(v.ProofGrid) > 0)
	keepPtr(o, "coverage", v.Coverage, BundleDocumentCoverage.ToCanon)
	keep(o, "conflicts", listOf(v.Conflicts, Finding.ToCanon), len(v.Conflicts) > 0)
	o["decisions"] = listOf(v.Decisions, Decision.ToCanon)
	o["sources"] = listOf(v.Sources, Source.ToCanon)
	o["history"] = listOf(v.History, HistoryEntry.ToCanon)
	o["changelog"] = listOf(v.Changelog, ChangelogEntry.ToCanon)
	o["tested_set"] = v.TestedSet.ToCanon()
	o["health"] = v.Health.ToCanon()
	keep(o, "rule_map", listOf(v.RuleMap, RuleMap.ToCanon), len(v.RuleMap) > 0)
	keep(o, "rendering_manifest", listOf(v.RenderingManifest, RenderingManifest.ToCanon), len(v.RenderingManifest) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member BundleDocument has no
// field for, under its exact name, is refused.
func (v *BundleDocument) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *BundleDocument) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "versions", "release", "released", "identity", "structure", "terms", "rules", "exclusions", "bindings", "adapters_used", "presets", "checker_code", "proof_grid", "coverage", "conflicts", "decisions", "sources", "history", "changelog", "tested_set", "health", "rule_map", "rendering_manifest")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "versions", &v.Versions, readVia[Versions]),
		field(o, at, "release", &v.Release, readVia[Release]),
		field(o, at, "released", &v.Released, readTruth),
		field(o, at, "identity", &v.Identity, readVia[Rulebook]),
		field(o, at, "structure", &v.Structure, readVia[Structure]),
		field(o, at, "terms", &v.Terms, listRead(readVia[Term])),
		field(o, at, "rules", &v.Rules, listRead(readVia[Rule])),
		field(o, at, "exclusions", &v.Exclusions, listRead(readVia[Exclusion])),
		field(o, at, "bindings", &v.Bindings, listRead(readVia[Binding])),
		field(o, at, "adapters_used", &v.AdaptersUsed, listRead(readVia[BundleDocumentAdaptersUsed])),
		field(o, at, "presets", &v.Presets, listRead(readVia[Preset])),
		field(o, at, "checker_code", &v.CheckerCode, listRead(readVia[CheckerCode])),
		field(o, at, "proof_grid", &v.ProofGrid, listRead(readVia[ProofResult])),
		field(o, at, "coverage", &v.Coverage, ptrRead(readVia[BundleDocumentCoverage])),
		field(o, at, "conflicts", &v.Conflicts, listRead(readVia[Finding])),
		field(o, at, "decisions", &v.Decisions, listRead(readVia[Decision])),
		field(o, at, "sources", &v.Sources, listRead(readVia[Source])),
		field(o, at, "history", &v.History, listRead(readVia[HistoryEntry])),
		field(o, at, "changelog", &v.Changelog, listRead(readVia[ChangelogEntry])),
		field(o, at, "tested_set", &v.TestedSet, readVia[TestedSet]),
		field(o, at, "health", &v.Health, readVia[Health]),
		field(o, at, "rule_map", &v.RuleMap, listRead(readVia[RuleMap])),
		field(o, at, "rendering_manifest", &v.RenderingManifest, listRead(readVia[RenderingManifest])),
	)
}

// BundleDocumentAdaptersUsed is has the shape the schema gives it.
type BundleDocumentAdaptersUsed struct {
	// Display only.
	// Since contract 1.0.
	Tool Text `json:"tool"`
	// Display only.
	// Since contract 1.0.
	AdapterVersion Text `json:"adapter_version"`
	// the suppression text in the tool's own syntax, as the adapter describes it, with <id> where the waiver id goes; absent or null for a tool with no suppression of its own.
	// Display only.
	// Since contract 1.3.
	// Absent means null.
	WaiverSyntax *Text `json:"waiver_syntax,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote BundleDocumentAdaptersUsed.
func (v BundleDocumentAdaptersUsed) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool"] = v.Tool.ToCanon()
	o["adapter_version"] = v.AdapterVersion.ToCanon()
	keepPtr(o, "waiver_syntax", v.WaiverSyntax, Text.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member BundleDocumentAdaptersUsed has no
// field for, under its exact name, is refused.
func (v *BundleDocumentAdaptersUsed) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *BundleDocumentAdaptersUsed) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "tool", "adapter_version", "waiver_syntax")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "tool", &v.Tool, readVia[Text]),
		field(o, at, "adapter_version", &v.AdapterVersion, readVia[Text]),
		field(o, at, "waiver_syntax", &v.WaiverSyntax, ptrRead(readVia[Text])),
	)
}

// BundleDocumentCoverage is has the shape the schema gives it.
type BundleDocumentCoverage struct {
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	RulesToBindings []ID `json:"rules_to_bindings,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	BindingsToRules []Text `json:"bindings_to_rules,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote BundleDocumentCoverage.
func (v BundleDocumentCoverage) ToCanon() canon.Value {
	o := canon.Object{}
	keep(o, "rules_to_bindings", listOf(v.RulesToBindings, ID.ToCanon), len(v.RulesToBindings) > 0)
	keep(o, "bindings_to_rules", listOf(v.BindingsToRules, Text.ToCanon), len(v.BindingsToRules) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member BundleDocumentCoverage has no
// field for, under its exact name, is refused.
func (v *BundleDocumentCoverage) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *BundleDocumentCoverage) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "rules_to_bindings", "bindings_to_rules")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "rules_to_bindings", &v.RulesToBindings, listRead(readVia[ID])),
		field(o, at, "bindings_to_rules", &v.BindingsToRules, listRead(readVia[Text])),
	)
}
