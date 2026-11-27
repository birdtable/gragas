package lifecycle

import (
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// PlanRuleEdit plans an edit of the lifecycle fields of a rule. A value the
// rule already has plans nothing, so a second identical run ends "done,
// nothing to do". It reads the rule as the book holds it: a field that
// someone edited by hand since is the "before" of the plan, and is changed
// only because the request names it.
func PlanRuleEdit(b model.Book, r model.EditRule, m Moment) (Plan, error) {
	i := slices.IndexFunc(b.Rules, func(x model.Rule) bool { return string(x.ID) == r.ID })
	if i < 0 {
		return Plan{}, notFound(b, r.ID)
	}
	rule := b.Rules[i]
	if err := checkEdit(b, rule, r); err != nil {
		return Plan{}, err
	}
	entity := "rule:" + r.ID
	var d model.Delta
	var changes []string
	after := lifecycleOf(rule)
	for _, f := range []struct {
		field    string
		want     *string
		now      string
		set      func(string)
		describe string
	}{
		{"owner", r.Owner, rule.Owner, func(s string) { after.Owner = s }, "owner"},
		{"review_by", r.ReviewBy, rule.ReviewBy, func(s string) { after.ReviewBy = s }, "review-by"},
		{"zero_by", r.ZeroBy, rule.ZeroBy, func(s string) { after.ZeroBy = s }, "zero-by"},
	} {
		if f.want == nil || *f.want == f.now {
			continue
		}
		d.Changes = append(d.Changes, model.Change{Entity: entity, Field: f.field, Before: f.now, After: *f.want})
		f.set(*f.want)
		changes = append(changes, fmt.Sprintf("%s of %s: %q becomes %q", f.describe, r.ID, f.now, *f.want))
	}
	var scope model.RuleScope
	if rule.Scope != nil {
		scope = *rule.Scope
	}
	var rel model.Relations
	if rule.Relations != nil {
		rel = *rule.Relations
	}
	for _, a := range []struct {
		field string
		have  []string
		add   []string
	}{
		{"scope.parts[]", scope.Parts, r.AddParts},
		{"scope.paths[]", scope.Paths, r.AddPaths},
		{"scope.kinds[]", scope.Kinds, r.AddKinds},
		{"relations.relates[]", ids(rel.Relates), strs(r.Relates)},
		{"relations.conflicts[]", ids(rel.Conflicts), strs(r.Conflicts)},
		{"relations.depends[]", ids(rel.Depends), strs(r.Depends)},
		{"relations.covers[]", ids(rel.Covers), strs(r.Covers)},
	} {
		seen := slices.Clone(a.have)
		for _, v := range a.add {
			if slices.Contains(seen, v) {
				continue
			}
			seen = append(seen, v)
			d.Changes = append(d.Changes, model.Change{Entity: entity, Field: a.field, After: v})
			changes = append(changes, fmt.Sprintf("%s of %s gains %s", a.field[:len(a.field)-2], r.ID, v))
		}
	}
	cd, cs := citeChanges(rule, r.Cite, entity)
	d.Changes = append(d.Changes, cd...)
	changes = append(changes, cs...)
	ud, us := unciteChanges(b, rule, r.Uncite, entity)
	d.Changes = append(d.Changes, ud...)
	changes = append(changes, us...)
	if len(d.Changes) == 0 {
		now := lifecycleOf(rule)
		return Plan{Record: Record{Kind: "edit", Entity: entity, Before: &now, After: now},
			Sections: sections([]string{"nothing: " + r.ID + " already holds every value asked for"}, nil, "none")}, nil
	}
	before := lifecycleOf(rule)
	return Plan{
		Delta:    d,
		Record:   Record{Kind: "edit", Entity: entity, Before: &before, After: after},
		Sections: sections(append([]string{fmt.Sprintf("edit %s. Standing: %s before, %s after", r.ID, rule.Status, rule.Status)}, changes...), []string{r.Files.Rules + ": edits rule " + r.ID, r.Files.History + ": appends one entry (edit)"}, "none: the edit changes no binding"),
	}, nil
}

// citeChanges are the records that cite each source the rule does not yet cite
// as backing it, once each, and the words that say so.
func citeChanges(rule model.Rule, cite []string, entity string) ([]model.Change, []string) {
	var out []model.Change
	var words []string
	done := map[string]bool{}
	for _, s := range cite {
		if done[s] || slices.ContainsFunc(rule.Sources, func(c model.Citation) bool { return c.Ref == s && c.Backs == model.BacksRule }) {
			continue
		}
		done[s] = true
		out = append(out, model.Change{Entity: entity, Field: "sources{}", After: "ref: " + s + "\nbacks: " + string(model.BacksRule)})
		words = append(words, fmt.Sprintf("sources of %s gains %s", rule.ID, s))
	}
	return out, words
}

func notFound(b model.Book, id string) error {
	for _, x := range b.Rules {
		if slices.Contains(x.Aliases, model.RuleID(id)) {
			return refuse("edit it by its current id, "+string(x.ID), "%s is an old id of %s", id, x.ID)
		}
	}
	return refuse("list the rules with gragas rule list", "the rule %s is not in the book", id)
}

func checkEdit(b model.Book, rule model.Rule, r model.EditRule) error {
	asked := r.Owner != nil || r.ReviewBy != nil || r.ZeroBy != nil || len(r.AddParts)+len(r.AddPaths)+len(r.AddKinds) > 0 ||
		len(r.Relates)+len(r.Conflicts)+len(r.Depends)+len(r.Covers)+len(r.Cite)+len(r.Uncite) > 0
	switch {
	case rule.Status == model.Retired:
		return refuse("restore it first with gragas rule restore "+r.ID, "%s is retired, and a retired rule is not edited", r.ID)
	case !asked:
		return refuse("pass a flag for the field to change, such as --rule-owner", "nothing is asked for, so there is nothing to change")
	}
	for name, v := range map[string]*string{"owner": r.Owner} {
		if v != nil && !clean(*v) {
			return refuse("write the "+name+" on one line", "the %s runs over more than one line", name)
		}
	}
	for name, v := range map[string]*string{"review_by": r.ReviewBy, "zero_by": r.ZeroBy} {
		if v != nil && *v != "" && !validDate(*v) {
			return refuse("write the date as YYYY-MM-DD", "%s %q is not a date", name, *v)
		}
	}
	if err := checkSources(b, rule, r); err != nil {
		return err
	}
	if err := checkScope(b, &model.RuleScope{Parts: r.AddParts, Paths: r.AddPaths, Kinds: r.AddKinds}); err != nil {
		return err
	}
	for _, l := range [][]model.RuleID{r.Relates, r.Conflicts, r.Depends, r.Covers} {
		for _, id := range l {
			switch {
			case string(id) == r.ID:
				return refuse("link it to another rule", "a rule cannot be linked to itself (%s)", id)
			case !slices.ContainsFunc(b.Rules, func(x model.Rule) bool { return x.ID == id }):
				return refuse("name the other rule by its current id", "the rule %s is not in the book", id)
			}
		}
	}
	return nil
}

func ids(l []model.RuleID) []string { return strs(l) }

func strs(l []model.RuleID) []string {
	out := make([]string, len(l))
	for i, v := range l {
		out[i] = string(v)
	}
	return out
}

// unciteChanges are the records that take each citation of each source out of
// the rule, whatever it backs, and the words that say so: one line for each
// citation, and one for each source no rule cites once the edit is made. That
// source stays in the book; nothing is deleted.
func unciteChanges(b model.Book, rule model.Rule, uncite []string, entity string) ([]model.Change, []string) {
	var out []model.Change
	var words []string
	done := map[string]bool{}
	for _, s := range uncite {
		if done[s] {
			continue
		}
		done[s] = true
		n := 0
		for _, c := range rule.Sources {
			if c.Ref != s {
				continue
			}
			n++
			out = append(out, model.Change{Entity: entity, Field: "sources{}", Before: citationRecord(c)})
			words = append(words, fmt.Sprintf("sources of %s loses %s, which backs the %s", rule.ID, s, c.Backs))
		}
		if n > 0 && !citedByAnother(b, rule.ID, s) {
			words = append(words, fmt.Sprintf("no rule cites %s now; it stays in the sources of the book", s))
		}
	}
	return out, words
}

// citationRecord is a citation as the YAML mapping of its keys, as the rule's
// file holds it: format takes out the record whose keys and texts are these.
func citationRecord(c model.Citation) string {
	rec := "ref: " + scalar(c.Ref)
	if c.Where != "" {
		rec += "\nwhere: " + scalar(c.Where)
	}
	if c.Says != "" {
		rec += "\nsays: " + scalar(c.Says)
	}
	return rec + "\nbacks: " + scalar(string(c.Backs))
}

func citedByAnother(b model.Book, id model.RuleID, slug string) bool {
	return slices.ContainsFunc(b.Rules, func(x model.Rule) bool {
		return x.ID != id && slices.ContainsFunc(x.Sources, func(c model.Citation) bool { return c.Ref == slug })
	})
}

// checkSources refuses a cite of a source the book lacks, an uncite of a
// source that is both cited and uncited, of one neither the rule nor the book
// holds, and of the last source of a rule past proposed: S24 lets a rule
// leave proposed only with one, so it keeps one for as long as it is trial or
// active.
func checkSources(b model.Book, rule model.Rule, r model.EditRule) error {
	for _, s := range r.Cite {
		if !hasSource(b, s) {
			return refuse("list the sources with gragas source list, or add one first", "the source %s is not in the book", s)
		}
	}
	left := len(rule.Sources)
	seen := map[string]bool{}
	for _, s := range r.Uncite {
		n := 0
		for _, c := range rule.Sources {
			if c.Ref == s {
				n++
			}
		}
		switch {
		case slices.Contains(r.Cite, s):
			return refuse("pass the source to --cite or to --uncite, not both", "%s is asked to be cited and uncited at once", s)
		case n == 0 && !hasSource(b, s):
			return refuse("list the sources with gragas source list", "the source %s is not in the book, and %s does not cite it", s, rule.ID)
		}
		if !seen[s] {
			seen[s] = true
			left -= n
		}
	}
	added, _ := citeChanges(rule, r.Cite, "")
	if len(r.Uncite) == 0 || rule.Status == model.Proposed || left+len(added) > 0 {
		return nil
	}
	return refuse("cite another source first with gragas rule edit "+string(rule.ID)+" --cite <source slug>, or move the rule back to proposed",
		"%s is %s, and the edit takes away its last source; a rule past proposed keeps one (S24)", rule.ID, rule.Status)
}
