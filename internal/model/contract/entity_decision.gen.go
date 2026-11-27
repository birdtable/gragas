// Written by gragas-generate from internal/host/assets/contract/schema/v1/entity-decision.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// Decision is Decision (entity, spec 16.1).
type Decision struct {
	// Display only.
	// Since contract 1.0.
	Slug Slug `json:"slug"`
	// Display only.
	// Since contract 1.0.
	Title Text `json:"title"`
	// Display only.
	// Since contract 1.0.
	Text Text `json:"text"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Rejected []Text `json:"rejected,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Cites *DecisionCites `json:"cites,omitempty"`
	// Display only.
	// Since contract 1.0.
	Who Text `json:"who"`
	// Display only.
	// Since contract 1.0.
	Date Date `json:"date"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Supersedes *Slug `json:"supersedes,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	SupersededBy *Slug `json:"superseded_by,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote Decision.
func (v Decision) ToCanon() canon.Value {
	o := canon.Object{}
	o["slug"] = v.Slug.ToCanon()
	o["title"] = v.Title.ToCanon()
	o["text"] = v.Text.ToCanon()
	keep(o, "rejected", listOf(v.Rejected, Text.ToCanon), len(v.Rejected) > 0)
	keepPtr(o, "cites", v.Cites, DecisionCites.ToCanon)
	o["who"] = v.Who.ToCanon()
	o["date"] = v.Date.ToCanon()
	keepPtr(o, "supersedes", v.Supersedes, Slug.ToCanon)
	keepPtr(o, "superseded_by", v.SupersededBy, Slug.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member Decision has no
// field for, under its exact name, is refused.
func (v *Decision) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *Decision) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "slug", "title", "text", "rejected", "cites", "who", "date", "supersedes", "superseded_by")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "slug", &v.Slug, readVia[Slug]),
		field(o, at, "title", &v.Title, readVia[Text]),
		field(o, at, "text", &v.Text, readVia[Text]),
		field(o, at, "rejected", &v.Rejected, listRead(readVia[Text])),
		field(o, at, "cites", &v.Cites, ptrRead(readVia[DecisionCites])),
		field(o, at, "who", &v.Who, readVia[Text]),
		field(o, at, "date", &v.Date, readVia[Date]),
		field(o, at, "supersedes", &v.Supersedes, ptrRead(readVia[Slug])),
		field(o, at, "superseded_by", &v.SupersededBy, ptrRead(readVia[Slug])),
	)
}

// DecisionCites is has the shape the schema gives it.
type DecisionCites struct {
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Sources []Slug `json:"sources,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Rules []ID `json:"rules,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Bindings []Text `json:"bindings,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote DecisionCites.
func (v DecisionCites) ToCanon() canon.Value {
	o := canon.Object{}
	keep(o, "sources", listOf(v.Sources, Slug.ToCanon), len(v.Sources) > 0)
	keep(o, "rules", listOf(v.Rules, ID.ToCanon), len(v.Rules) > 0)
	keep(o, "bindings", listOf(v.Bindings, Text.ToCanon), len(v.Bindings) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member DecisionCites has no
// field for, under its exact name, is refused.
func (v *DecisionCites) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *DecisionCites) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "sources", "rules", "bindings")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "sources", &v.Sources, listRead(readVia[Slug])),
		field(o, at, "rules", &v.Rules, listRead(readVia[ID])),
		field(o, at, "bindings", &v.Bindings, listRead(readVia[Text])),
	)
}
