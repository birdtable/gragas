package lifecycle

import (
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// PlanRuleRenumber plans a new id for a rule: the next number of its concern,
// which no id or alias of the book has held, with the id it had kept as an
// alias for good (S62). Nothing that names the old id is rewritten: every
// reference resolves through the alias. The change is one history line, on the
// rule by its new id.
//
// Where two rules took one id, as two branches merged, the rule renumbered is
// the later one, which the load put aside, and the rule that kept the id is
// left as it is. A rule asked for by an id it no longer has is refused with the
// id it has now, so a second run of the same command changes nothing.
func PlanRuleRenumber(b model.Book, r model.RenumberRule, m Moment) (Plan, error) {
	rule, clashed, err := renumbered(b, r.ID)
	if err != nil {
		return Plan{}, err
	}
	if err := checkWhy(b, r.Why, "renumber", r.ID); err != nil {
		return Plan{}, err
	}
	part, concern := partOf(r.ID), concernOf(r.ID)
	newID := fmt.Sprintf("%s.%s.%d", part, concern, nextNumber(b, part, concern))
	slug := decisionSlug(r.Why, "renumber", r.ID)
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	// The alias goes in before the id changes: the delta names the rule by the
	// id it has until the last change.
	entity := "rule:" + r.ID
	// Of two rules that took an id the later keeps no alias of it: the id is
	// the other rule's, and a second claim on it is the clash over again.
	if !clashed {
		d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "aliases[]", After: r.ID})
	}
	d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "id", Before: r.ID, After: newID})
	now := lifecycleOf(rule)
	what := fmt.Sprintf("%s becomes %s, and keeps %s as an alias for good", r.ID, newID, r.ID)
	wrote := r.Files.Rules + ": sets the id of " + r.ID + " to " + newID + " and adds " + r.ID + " to its aliases"
	consumers := "projects, waivers and rule maps that name " + r.ID + " keep working: it is an alias of " + newID
	next := "nothing that names " + r.ID + " is rewritten: it resolves to " + newID + " through the alias"
	if clashed {
		what = fmt.Sprintf("%s, the later of two rules that took the id, becomes %s; the first keeps %s", r.ID, newID, r.ID)
		wrote = r.Files.Rules + ": sets the id of the later rule that took " + r.ID + " to " + newID
		consumers = "what names " + r.ID + " still names the rule that kept it; the later rule is " + newID
		next = "nothing that names " + r.ID + " is rewritten"
	}
	changes = append([]string{what, next}, changes...)
	written = append([]string{wrote}, written...)
	written = append(written, r.Files.History+": appends one entry (renumber)")
	return Plan{Delta: d, Record: Record{Kind: "renumber", Entity: "rule:" + newID, Before: &now, After: now, Decision: slug},
		Sections: sections(changes, written, consumers)}, nil
}

// renumbered is the rule a renumber of id means, and whether it is the later of
// two that took the id.
func renumbered(b model.Book, id string) (model.Rule, bool, error) {
	if i := slices.IndexFunc(b.Clashed, func(x model.Rule) bool { return string(x.ID) == id }); i >= 0 {
		return b.Clashed[i], true, nil
	}
	rule, err := ruleOf(b, id)
	return rule, false, err
}
