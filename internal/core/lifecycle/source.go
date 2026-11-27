package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// The two plans of the source family: a thing a rule can cite, recorded once,
// and its title, locator or attribution changed. A source is never replaced or
// retired (S15): the rules that cite it keep their citation. An edit notes
// itself in the history of each rule that cites the source, so a reader of a
// rule's trail sees the change, and the notes are one plan with the edit.

// PlanSourceNew plans a new source. A source the book holds with the same
// words plans nothing, so the second run ends "done, nothing to do".
func PlanSourceNew(b model.Book, r model.NewSource, m Moment) (Plan, error) {
	switch {
	case !validSlug(r.Slug):
		return Plan{}, refuse("write the source as lower-case words joined by hyphens with --source-slug", "the source %q is not a slug", r.Slug)
	case !slices.Contains(model.SourceKinds(), r.Kind):
		return Plan{}, refuse("give one of "+kindList()+" with --source-kind", "the kind %q is not a kind of source", r.Kind)
	case strings.TrimSpace(r.Title) == "" || !clean(r.Title):
		return Plan{}, refuse("give the title on one line with --title", "the title is empty or runs over more than one line")
	case strings.TrimSpace(r.Locator) == "" || !clean(r.Locator):
		return Plan{}, refuse("give a URL, an ISBN, a DOI, an RFC or a decision slug on one line with --locator", "the locator is empty or runs over more than one line")
	case !clean(r.Attribution):
		return Plan{}, refuse("write the attribution on one line", "the attribution runs over more than one line")
	}
	if s, ok := sourceOf(b, r.Slug); ok {
		if s.Kind == r.Kind && s.Title == r.Title && s.Locator == r.Locator && s.Attribution == r.Attribution {
			return nothingSource("new", s, fmt.Sprintf("%s is recorded already, with these words", r.Slug)), nil
		}
		return Plan{}, refuse("change it with gragas source edit "+r.Slug+", or pick another slug", "the source %s is in the book already, with other words", r.Slug)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "slug: %s\nkind: %s\ntitle: %s\nlocator: %s\n", r.Slug, r.Kind, scalar(r.Title), scalar(r.Locator))
	if r.Attribution != "" {
		fmt.Fprintf(&body, "attribution: %s\n", scalar(r.Attribution))
	}
	entity := "source:" + r.Slug
	written := []string{r.Files.Sources + ": adds source " + r.Slug, r.Files.History + ": appends one entry (new)"}
	return Plan{
		Delta:    model.Delta{Changes: []model.Change{{Entity: entity, After: body.String()}}},
		Record:   Record{Kind: "new", Entity: entity, After: sourceLifecycle(model.Source{Locator: r.Locator})},
		Sections: sections([]string{fmt.Sprintf("new source %s: %s (%s), found at %s", r.Slug, r.Title, r.Kind, r.Locator)}, written, "none: no rule cites it yet"),
	}, nil
}

// PlanSourceEdit plans a change of a source's title, locator or attribution.
// Fields that already hold plan nothing. A change is one entry for the source
// and one entry on each rule that cites it, in the book's order of rules.
func PlanSourceEdit(b model.Book, r model.EditSource, m Moment) (Plan, error) {
	s, ok := sourceOf(b, r.Slug)
	switch {
	case !ok:
		return Plan{}, refuse("list the sources in sources.yaml, or record it with gragas source new", "the source %s is not in the book", r.Slug)
	case r.Title == nil && r.Locator == nil && r.Attribution == nil:
		return Plan{}, refuse("give --title, --locator or --attribution", "no field is given to change")
	case r.Title != nil && (strings.TrimSpace(*r.Title) == "" || !clean(*r.Title)):
		return Plan{}, refuse("give the title on one line", "the title is empty or runs over more than one line")
	case r.Locator != nil && (strings.TrimSpace(*r.Locator) == "" || !clean(*r.Locator)):
		return Plan{}, refuse("give the locator on one line", "the locator is empty or runs over more than one line")
	case r.Attribution != nil && !clean(*r.Attribution):
		return Plan{}, refuse("write the attribution on one line", "the attribution runs over more than one line")
	}
	entity := "source:" + r.Slug
	var d model.Delta
	var changes []string
	for _, f := range []struct {
		name string
		now  string
		want *string
	}{
		{"title", s.Title, r.Title},
		{"locator", s.Locator, r.Locator},
		{"attribution", s.Attribution, r.Attribution},
	} {
		if f.want == nil || *f.want == f.now {
			continue
		}
		d.Changes = append(d.Changes, model.Change{Entity: entity, Field: f.name, Before: f.now, After: *f.want})
		changes = append(changes, fmt.Sprintf("%s: the %s goes from %q to %q", r.Slug, f.name, f.now, *f.want))
	}
	if len(d.Changes) == 0 {
		return nothingSource("edit", s, fmt.Sprintf("%s has these words already", r.Slug)), nil
	}
	after := s
	for _, c := range d.Changes {
		switch c.Field {
		case "title":
			after.Title = c.After
		case "locator":
			after.Locator = c.After
		case "attribution":
			after.Attribution = c.After
		}
	}
	before, now := sourceLifecycle(s), sourceLifecycle(after)
	rec := Record{Kind: "edit", Entity: entity, Before: &before, After: now}
	var also []Record
	var cited []string
	for _, rule := range b.Rules {
		if slices.ContainsFunc(rule.Sources, func(c model.Citation) bool { return c.Ref == r.Slug }) {
			lc := lifecycleOf(rule)
			also = append(also, Record{Kind: "source-edit", Entity: "rule:" + string(rule.ID), Before: &lc, After: lc})
			cited = append(cited, string(rule.ID))
		}
	}
	consumer := "none: a rule cites a source by its slug, which stays"
	if len(cited) > 0 {
		changes = append(changes, fmt.Sprintf("noted in the history of %d rule(s) that cite %s: %s", len(cited), r.Slug, strings.Join(cited, ", ")))
	}
	written := []string{r.Files.Sources + ": sets " + fieldList(d.Changes) + " of " + r.Slug,
		fmt.Sprintf("%s: appends %d entries (edit, and a note on each citing rule)", r.Files.History, 1+len(also))}
	if len(also) == 0 {
		written[1] = r.Files.History + ": appends one entry (edit)"
	}
	return Plan{Delta: d, Record: rec, Also: also, Sections: sections(changes, written, consumer)}, nil
}

func sourceOf(b model.Book, slug string) (model.Source, bool) {
	return bySlug(b.Sources, slug, func(s model.Source) string { return s.Slug })
}

// sourceLifecycle is the guarded fields of a source as a history entry names
// them: a source has no status of its own, so it is active, and its literal is
// its locator, the one thing a citation depends on.
func sourceLifecycle(s model.Source) model.Lifecycle {
	return model.Lifecycle{Status: model.Active, Literal: s.Locator}
}

func nothingSource(kind string, s model.Source, why string) Plan {
	return nothingPlan(kind, "source:"+s.Slug, sourceLifecycle(s), why)
}

func kindList() string {
	var ks []string
	for _, k := range model.SourceKinds() {
		ks = append(ks, string(k))
	}
	return strings.Join(ks, ", ")
}

func fieldList(cs []model.Change) string {
	var fs []string
	for _, c := range cs {
		fs = append(fs, c.Field)
	}
	return strings.Join(fs, ", ")
}
