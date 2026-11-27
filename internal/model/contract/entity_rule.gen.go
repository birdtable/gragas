// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-rule.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Rule is Rule (entity, spec 16.1).
type Rule struct {
	// Decides: which parent rule an overlay's exclusion names, so an id the parent does not hold is refused.
	// Since contract 1.0.
	ID ID `json:"id"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Aliases []ID `json:"aliases,omitempty"`
	// Display only.
	// Since contract 1.0.
	Statement Text `json:"statement"`
	// Display only.
	// Since contract 1.0.
	Strength Strength `json:"strength"`
	// Decides: whether a parent's rule is retired, so including it in an overlay ends not ready (S4).
	// Since contract 1.0.
	Status Status `json:"status"`
	// Display only.
	// Since contract 1.0.
	Standing Standing `json:"standing"`
	// trial, blocked and unproven, those that apply, in that order.
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Marks []StandingMark `json:"marks,omitempty"`
	// the releases that shipped the rule on trial, in release order: each one holds an entry of the rule (by its id or an alias) that leaves it on trial, which is what the gate's trial window counts, so a reader reaches the gate's finding when its length is at or past identity.governance.trial_releases, when that is above 0 (at 0 or below the gate sets no window). Derived from the history by the engine; no rulebook file authors it. Empty when the rule is not on trial or no release shipped it so.
	// Display only.
	// Since contract 1.1.
	// Absent means [].
	TrialShipped []Semver `json:"trial_shipped,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Scope *RuleScope `json:"scope,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Exemptions []RuleExemptions `json:"exemptions,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Reasoning TextOrEmpty `json:"reasoning,omitempty"`
	// the slug of the decision the rule's reasoning names, a slug of decisions[]; absent or null when the reasoning names none.
	// Decides: whether a rule is the attention kind missing decision: with identity.governance.decision_required, a rule that is not retired and has none is listed so, as the engine lists it.
	// Since contract 1.2.
	// Absent means null.
	ReasoningDecision *Slug `json:"reasoning_decision,omitempty"`
	// the rule's anatomy, what speaks for it, one point an item, in the rulebook's own words; beside reasoning, which stays the whole text.
	// Display only.
	// Since contract 1.3.
	// Absent means [].
	Pros []Text `json:"pros,omitempty"`
	// the rule's anatomy, what speaks against it, one point an item, in the rulebook's own words.
	// Display only.
	// Since contract 1.3.
	// Absent means [].
	Cons []Text `json:"cons,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Pairs []RulePairs `json:"pairs,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Owner TextOrEmpty `json:"owner,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	ReviewBy *Date `json:"review_by,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	ZeroBy *Date `json:"zero_by,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Relations []RuleRelations `json:"relations,omitempty"`
	// Display only.
	// Since contract 1.0.
	Origin Origin `json:"origin"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Sources []Slug `json:"sources,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Bindings []Text `json:"bindings,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Translations []Translation `json:"translations,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	ReplacedBy *ID `json:"replaced_by,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Anchor TextOrEmpty `json:"anchor,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Rule.
func (v Rule) ToCanon() canon.Value {
	o := canon.Object{}
	o["id"] = v.ID.ToCanon()
	keep(o, "aliases", listOf(v.Aliases, ID.ToCanon), len(v.Aliases) > 0)
	o["statement"] = v.Statement.ToCanon()
	o["strength"] = v.Strength.ToCanon()
	o["status"] = v.Status.ToCanon()
	o["standing"] = v.Standing.ToCanon()
	keep(o, "marks", listOf(v.Marks, StandingMark.ToCanon), len(v.Marks) > 0)
	keep(o, "trial_shipped", listOf(v.TrialShipped, Semver.ToCanon), len(v.TrialShipped) > 0)
	keepPtr(o, "scope", v.Scope, RuleScope.ToCanon)
	keep(o, "exemptions", listOf(v.Exemptions, RuleExemptions.ToCanon), len(v.Exemptions) > 0)
	keep(o, "reasoning", v.Reasoning.ToCanon(), v.Reasoning != "")
	keepPtr(o, "reasoning_decision", v.ReasoningDecision, Slug.ToCanon)
	keep(o, "pros", listOf(v.Pros, Text.ToCanon), len(v.Pros) > 0)
	keep(o, "cons", listOf(v.Cons, Text.ToCanon), len(v.Cons) > 0)
	keep(o, "pairs", listOf(v.Pairs, RulePairs.ToCanon), len(v.Pairs) > 0)
	keep(o, "owner", v.Owner.ToCanon(), v.Owner != "")
	keepPtr(o, "review_by", v.ReviewBy, Date.ToCanon)
	keepPtr(o, "zero_by", v.ZeroBy, Date.ToCanon)
	keep(o, "relations", listOf(v.Relations, RuleRelations.ToCanon), len(v.Relations) > 0)
	o["origin"] = v.Origin.ToCanon()
	keep(o, "sources", listOf(v.Sources, Slug.ToCanon), len(v.Sources) > 0)
	keep(o, "bindings", listOf(v.Bindings, Text.ToCanon), len(v.Bindings) > 0)
	keep(o, "translations", listOf(v.Translations, Translation.ToCanon), len(v.Translations) > 0)
	keepPtr(o, "replaced_by", v.ReplacedBy, ID.ToCanon)
	keep(o, "anchor", v.Anchor.ToCanon(), v.Anchor != "")
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Rule has no
// field for, under its exact name, is refused.
func (v *Rule) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Rule) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "id", "aliases", "statement", "strength", "status", "standing", "marks", "trial_shipped", "scope", "exemptions", "reasoning", "reasoning_decision", "pros", "cons", "pairs", "owner", "review_by", "zero_by", "relations", "origin", "sources", "bindings", "translations", "replaced_by", "anchor")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "id", &v.ID, readVia[ID]),
		field(o, at, "aliases", &v.Aliases, listRead(readVia[ID])),
		field(o, at, "statement", &v.Statement, readVia[Text]),
		field(o, at, "strength", &v.Strength, readVia[Strength]),
		field(o, at, "status", &v.Status, readVia[Status]),
		field(o, at, "standing", &v.Standing, readVia[Standing]),
		field(o, at, "marks", &v.Marks, listRead(readVia[StandingMark])),
		field(o, at, "trial_shipped", &v.TrialShipped, listRead(readVia[Semver])),
		field(o, at, "scope", &v.Scope, ptrRead(readVia[RuleScope])),
		field(o, at, "exemptions", &v.Exemptions, listRead(readVia[RuleExemptions])),
		field(o, at, "reasoning", &v.Reasoning, readVia[TextOrEmpty]),
		field(o, at, "reasoning_decision", &v.ReasoningDecision, ptrRead(readVia[Slug])),
		field(o, at, "pros", &v.Pros, listRead(readVia[Text])),
		field(o, at, "cons", &v.Cons, listRead(readVia[Text])),
		field(o, at, "pairs", &v.Pairs, listRead(readVia[RulePairs])),
		field(o, at, "owner", &v.Owner, readVia[TextOrEmpty]),
		field(o, at, "review_by", &v.ReviewBy, ptrRead(readVia[Date])),
		field(o, at, "zero_by", &v.ZeroBy, ptrRead(readVia[Date])),
		field(o, at, "relations", &v.Relations, listRead(readVia[RuleRelations])),
		field(o, at, "origin", &v.Origin, readVia[Origin]),
		field(o, at, "sources", &v.Sources, listRead(readVia[Slug])),
		field(o, at, "bindings", &v.Bindings, listRead(readVia[Text])),
		field(o, at, "translations", &v.Translations, listRead(readVia[Translation])),
		field(o, at, "replaced_by", &v.ReplacedBy, ptrRead(readVia[ID])),
		field(o, at, "anchor", &v.Anchor, readVia[TextOrEmpty]),
	)
}

// RuleScope is has the shape the schema gives it.
type RuleScope struct {
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Parts []Slug `json:"parts,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Paths []Text `json:"paths,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Kinds []Slug `json:"kinds,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RuleScope.
func (v RuleScope) ToCanon() canon.Value {
	o := canon.Object{}
	keep(o, "parts", listOf(v.Parts, Slug.ToCanon), len(v.Parts) > 0)
	keep(o, "paths", listOf(v.Paths, Text.ToCanon), len(v.Paths) > 0)
	keep(o, "kinds", listOf(v.Kinds, Slug.ToCanon), len(v.Kinds) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RuleScope has no
// field for, under its exact name, is refused.
func (v *RuleScope) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RuleScope) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "parts", "paths", "kinds")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "parts", &v.Parts, listRead(readVia[Slug])),
		field(o, at, "paths", &v.Paths, listRead(readVia[Text])),
		field(o, at, "kinds", &v.Kinds, listRead(readVia[Slug])),
	)
}

// RuleExemptions is has the shape the schema gives it.
type RuleExemptions struct {
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Path TextOrEmpty `json:"path,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Kind *Slug `json:"kind,omitempty"`
	// Display only.
	// Since contract 1.0.
	Reason Reason `json:"reason"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RuleExemptions.
func (v RuleExemptions) ToCanon() canon.Value {
	o := canon.Object{}
	keep(o, "path", v.Path.ToCanon(), v.Path != "")
	keepPtr(o, "kind", v.Kind, Slug.ToCanon)
	o["reason"] = v.Reason.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RuleExemptions has no
// field for, under its exact name, is refused.
func (v *RuleExemptions) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RuleExemptions) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "path", "kind", "reason")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "path", &v.Path, readVia[TextOrEmpty]),
		field(o, at, "kind", &v.Kind, ptrRead(readVia[Slug])),
		field(o, at, "reason", &v.Reason, readVia[Reason]),
	)
}

// RulePairs is has the shape the schema gives it.
type RulePairs struct {
	// Display only.
	// Since contract 1.0.
	// Constraints: at least 1.
	Number int `json:"number"`
	// Display only.
	// Since contract 1.0.
	TargetType TargetType `json:"target_type"`
	// the pair's good example, the text of the file that follows the rule; absent or null when the bundle does not carry the pair's text.
	// Display only.
	// Since contract 1.3.
	// Absent means null.
	Good *Text `json:"good,omitempty"`
	// the pair's bad example, the text of the file that breaks the rule; absent or null when the bundle does not carry the pair's text.
	// Display only.
	// Since contract 1.3.
	// Absent means null.
	Bad *Text `json:"bad,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RulePairs.
func (v RulePairs) ToCanon() canon.Value {
	o := canon.Object{}
	o["number"] = wholeOf(v.Number)
	o["target_type"] = v.TargetType.ToCanon()
	keepPtr(o, "good", v.Good, Text.ToCanon)
	keepPtr(o, "bad", v.Bad, Text.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RulePairs has no
// field for, under its exact name, is refused.
func (v *RulePairs) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RulePairs) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "number", "target_type", "good", "bad")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "number", &v.Number, readWhole),
		field(o, at, "target_type", &v.TargetType, readVia[TargetType]),
		field(o, at, "good", &v.Good, ptrRead(readVia[Text])),
		field(o, at, "bad", &v.Bad, ptrRead(readVia[Text])),
	)
}

// RuleRelationsKind is has the shape the schema gives it.
type RuleRelationsKind string

// The values of RuleRelationsKind.
const (
	RuleRelationsKindPairsWith     RuleRelationsKind = "pairs-with"
	RuleRelationsKindConflictsWith RuleRelationsKind = "conflicts-with"
	RuleRelationsKindRefines       RuleRelationsKind = "refines"
	RuleRelationsKindSeeAlso       RuleRelationsKind = "see-also"
)

// ToCanon is v as canon's value tree, as encoding/json wrote RuleRelationsKind.
func (v RuleRelationsKind) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member RuleRelationsKind has no
// field for, under its exact name, is refused.
func (v *RuleRelationsKind) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RuleRelationsKind) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = RuleRelationsKind(x)
	return err
}

// RuleRelations is has the shape the schema gives it.
type RuleRelations struct {
	// Display only.
	// Since contract 1.0.
	Kind RuleRelationsKind `json:"kind"`
	// Display only.
	// Since contract 1.0.
	Rule ID `json:"rule"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote RuleRelations.
func (v RuleRelations) ToCanon() canon.Value {
	o := canon.Object{}
	o["kind"] = v.Kind.ToCanon()
	o["rule"] = v.Rule.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member RuleRelations has no
// field for, under its exact name, is refused.
func (v *RuleRelations) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *RuleRelations) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "kind", "rule")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "kind", &v.Kind, readVia[RuleRelationsKind]),
		field(o, at, "rule", &v.Rule, readVia[ID]),
	)
}
