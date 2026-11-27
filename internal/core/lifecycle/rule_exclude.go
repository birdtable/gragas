package lifecycle

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// The two plans of an overlay's exclusions (S4): exclude drops a rule of the
// parent, on a decision, and include lifts the exclusion, on a decision. Each
// is one record in exclusions.yaml keyed by the overlay, the parent's rule id
// and a sequence number, and one history line on that record. A lifted record
// stays, marked lifted, and a rule excluded again takes the next number.
// Exclusion is the only way an overlay weakens a parent's rule: there is no
// narrow (ruling 14).
//
// The parent's rules arrive as an argument, read by coreapi from the parent's
// bundle (A50, arch.ports.8): this context reads no file and imports nothing
// of a release. Only their ids and whether the parent retired each decide.

// Parent is what a plan knows of an overlay's parent.
type Parent struct {
	// Rulebook and Version are the pin the bundle was read at.
	Rulebook, Version string
	// Rules are the parent's rule ids, each true when the parent retired it.
	Rules map[model.RuleID]bool
}

// PlanRuleExclude plans the exclusion of a parent's rule from the overlay. A
// rule the overlay excludes already plans nothing, so the second run ends
// "done, nothing to do" (cli.commands.2).
func PlanRuleExclude(b model.Book, p Parent, r model.ExcludeRule, m Moment) (Plan, error) {
	id := model.RuleID(r.ID)
	if err := checkOverlay(b, "exclude"); err != nil {
		return Plan{}, err
	}
	if _, ok := p.Rules[id]; !ok {
		return Plan{}, unknownParentRule(p, r.ID)
	}
	if open := openExclusions(b, id); len(open) > 0 {
		return Plan{Record: exclusionRecord("exclude", open[0], model.Active, model.Active, open[0].Decision),
			Sections: sections([]string{fmt.Sprintf("nothing: %s is excluded already, by exclusion %d", r.ID, open[0].Number)}, nil, "none")}, nil
	}
	if err := checkWhy(b, r.Why, "exclude", r.ID); err != nil {
		return Plan{}, err
	}
	slug := decisionSlug(r.Why, "exclude", r.ID)
	e := model.Exclusion{Overlay: b.Identity.Name, Rule: id, Number: nextExclusion(b, id), Decision: slug, Who: m.Who, When: m.When}
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	d.Changes = append(d.Changes, model.Change{Entity: exclusionEntity(e), After: fmt.Sprintf(
		"overlay: %s\nrule: %s\nnumber: %d\ndecision: %s\nwho: %s\nwhen: %s\n", e.Overlay, e.Rule, e.Number, slug, scalar(e.Who), scalar(e.When))})
	changes = append([]string{fmt.Sprintf("%s of %s %s: excluded from %s as exclusion %d, decision %s", r.ID, p.Rulebook, p.Version, e.Overlay, e.Number, slug)}, changes...)
	written = append([]string{r.Files.Exclusions + ": adds exclusion " + strconv.Itoa(e.Number) + " of " + r.ID}, written...)
	written = append(written, r.Files.History+": appends one entry (exclude)")
	record := exclusionRecord("exclude", e, "", model.Active, slug)
	return Plan{Delta: d, Record: record, Sections: sections(changes, written,
		fmt.Sprintf("projects that pin %s see %s as excluded, with the decision %s", e.Overlay, r.ID, slug))}, nil
}

// PlanRuleInclude plans the lifting of the overlay's exclusion of a parent's
// rule. The record stays and is marked lifted. A rule that is not excluded
// ends "usage", and one the parent has retired ends "not ready" naming the
// parent's release (S4). The plan names the overlay's own rules that cover
// the parent's, since both apply once it is included.
func PlanRuleInclude(b model.Book, p Parent, r model.IncludeRule, m Moment) (Plan, error) {
	id := model.RuleID(r.ID)
	if err := checkOverlay(b, "include"); err != nil {
		return Plan{}, err
	}
	retired, ok := p.Rules[id]
	if !ok {
		return Plan{}, unknownParentRule(p, r.ID)
	}
	open := openExclusions(b, id)
	switch {
	case len(open) == 0:
		return Plan{}, refuseAs(model.Usage, "list what the overlay excludes in exclusions.yaml; exclude a rule with gragas rule exclude "+r.ID,
			"%s is not excluded by %s, so there is nothing to include", r.ID, b.Identity.Name)
	case retired:
		return Plan{}, refuseAs(model.NotReady, "move the pin to a release of "+p.Rulebook+" that holds "+r.ID+" as a live rule",
			"%s is retired in %s %s, so including it would bind nothing", r.ID, p.Rulebook, p.Version)
	}
	if err := checkWhy(b, r.Why, "include", r.ID); err != nil {
		return Plan{}, err
	}
	for i := range open {
		for _, o := range open[i+1:] {
			if o.Number == open[i].Number {
				return Plan{}, refuse("gragas validate lists the pair; give one of them the next number by hand",
					"two open exclusions of %s by %s share the number %d", r.ID, b.Identity.Name, o.Number)
			}
		}
	}
	slug := decisionSlug(r.Why, "include", r.ID)
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	var numbers []string
	for _, e := range open {
		entity := exclusionEntity(e)
		d.Changes = append(d.Changes,
			model.Change{Entity: entity, Field: "lifted.decision", After: slug},
			model.Change{Entity: entity, Field: "lifted.who", After: m.Who},
			model.Change{Entity: entity, Field: "lifted.when", After: m.When})
		numbers = append(numbers, strconv.Itoa(e.Number))
	}
	changes = append([]string{fmt.Sprintf("%s of %s %s: included again in %s, exclusion %s lifted, decision %s. The record stays",
		r.ID, p.Rulebook, p.Version, b.Identity.Name, joinWords(numbers), slug)}, changes...)
	if cover := coveredBy(b, id); len(cover) > 0 {
		changes = append(changes, fmt.Sprintf("both apply: %s of %s cover %s", joinWords(cover), b.Identity.Name, r.ID))
	}
	written = append([]string{r.Files.Exclusions + ": marks exclusion " + joinWords(numbers) + " of " + r.ID + " lifted"}, written...)
	written = append(written, r.Files.History+": appends one entry (include)")
	return Plan{Delta: d, Record: exclusionRecord("include", open[len(open)-1], model.Active, model.Retired, slug),
		Sections: sections(changes, written, fmt.Sprintf("projects that pin %s are held to %s again", b.Identity.Name, r.ID))}, nil
}

// checkOverlay refuses a rulebook that is not an overlay with a parent: only
// an overlay has a parent's rules to drop.
func checkOverlay(b model.Book, verb string) error {
	if b.Identity.Kind != model.KindOverlay || b.Identity.Extends == nil {
		return refuse("drop a rule of this rulebook's own with gragas rule retire; "+verb+" is run in an overlay, on its parent's rules",
			"%s is a %s rulebook with no parent, and only an overlay can %s a parent's rule", b.Identity.Name, b.Identity.Kind, verb)
	}
	return nil
}

func unknownParentRule(p Parent, id string) error {
	return refuse("name a rule that "+p.Rulebook+" "+p.Version+" holds, by its current id",
		"the rule %s is not in the parent %s %s", id, p.Rulebook, p.Version)
}

// openExclusions are the overlay's exclusions of the rule not lifted, by
// number.
func openExclusions(b model.Book, id model.RuleID) []model.Exclusion {
	var out []model.Exclusion
	for _, e := range b.Exclusions {
		if e.Overlay == b.Identity.Name && e.Rule == id && e.Lifted == nil {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(x, y model.Exclusion) int { return x.Number - y.Number })
	return out
}

// nextExclusion is the sequence number of a new exclusion of the rule: the
// highest the overlay has used for it, lifted or not, plus one.
func nextExclusion(b model.Book, id model.RuleID) int {
	n := 0
	for _, e := range b.Exclusions {
		if e.Overlay == b.Identity.Name && e.Rule == id {
			n = max(n, e.Number)
		}
	}
	return n + 1
}

// exclusionEntity is the entity an exclusion is changed and recorded under:
// its key.
func exclusionEntity(e model.Exclusion) string {
	return fmt.Sprintf("exclusion:%s/%s/%d", e.Overlay, e.Rule, e.Number)
}

// exclusionRecord is the history line of an exclusion. The record is its own
// entity: made active by exclude, and retired by include, when it stays as a
// lifted record. before "" is a new record.
func exclusionRecord(kind string, e model.Exclusion, before, after model.Status, decision string) Record {
	r := Record{Kind: kind, Entity: exclusionEntity(e), After: model.Lifecycle{Status: after}, Decision: decision}
	if before != "" {
		r.Before = &model.Lifecycle{Status: before}
	}
	return r
}

// coveredBy are the overlay's own live rules whose covers relation names id.
func coveredBy(b model.Book, id model.RuleID) []string {
	var out []string
	for _, r := range b.Rules {
		if r.Status != model.Retired && r.Relations != nil && slices.Contains(r.Relations.Covers, id) {
			out = append(out, string(r.ID))
		}
	}
	return out
}

func joinWords(s []string) string {
	switch len(s) {
	case 0:
		return ""
	case 1:
		return s[0]
	}
	return strings.Join(s[:len(s)-1], ", ") + " and " + s[len(s)-1]
}
