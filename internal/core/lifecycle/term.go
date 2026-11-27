package lifecycle

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// The three plans of the term family: a word the rulebook defines once, its
// literal changed, and its retirement. A term's definition and translations
// are prose and a file edit (S59), so no plan rewrites them. Nothing is
// deleted: a retired term stays in terms.yaml with the status retired. A term
// that carries a literal (a header name, a package) is what the names check
// lets a statement name; the lint reads it from the book, so the plans only
// write the field.

// termRef is a term written in a statement, {slug}.
var termRef = regexp.MustCompile(`\{([^{}]*)\}`)

// PlanTermNew plans a new term. A term the book holds with the same words
// plans nothing, so the second run ends "done, nothing to do".
func PlanTermNew(b model.Book, r model.NewTerm, m Moment) (Plan, error) {
	switch {
	case !validSlug(r.Slug):
		return Plan{}, refuse("write the term as lower-case words joined by hyphens", "the term %q is not a slug", r.Slug)
	case strings.TrimSpace(r.Definition) == "" || !clean(r.Definition):
		return Plan{}, refuse("give the term's meaning in one line with --term-definition", "the definition is empty or runs over more than one line")
	case !clean(r.Literal):
		return Plan{}, refuse("write the literal on one line", "the literal runs over more than one line")
	}
	if t, ok := termOf(b, r.Slug); ok {
		if t.Status != model.Retired && t.Definition == r.Definition && t.Literal == r.Literal {
			return nothingTerm("new", t, fmt.Sprintf("%s is defined already, with these words", r.Slug)), nil
		}
		if t.Status == model.Retired {
			return Plan{}, refuse("pick another slug: a retired term keeps its slug", "the term %s is retired, and its slug is never given again", r.Slug)
		}
		return Plan{}, refuse("change its literal with gragas term edit "+r.Slug+", or its definition in terms.yaml", "the term %s is defined already, with other words", r.Slug)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "slug: %s\ndefinition: %s\n", r.Slug, scalar(r.Definition))
	changes := []string{fmt.Sprintf("new term %s: %s", r.Slug, r.Definition)}
	if r.Literal != "" {
		fmt.Fprintf(&body, "literal: %s\n", scalar(r.Literal))
		changes = append(changes, fmt.Sprintf("%s stands for %s, which the names check lets a statement use through {%s}", r.Slug, r.Literal, r.Slug))
	}
	written := []string{r.Files.Terms + ": adds term " + r.Slug, r.Files.History + ": appends one entry (new)"}
	return Plan{
		Delta:    model.Delta{Changes: []model.Change{{Entity: "term:" + r.Slug, After: body.String()}}},
		Record:   Record{Kind: "new", Entity: "term:" + r.Slug, After: model.Lifecycle{Status: model.Active, Literal: r.Literal}},
		Sections: sections(changes, written, "none: no project binds a term"),
	}, nil
}

// PlanTermEdit plans a change of a term's literal. A literal the term has
// plans nothing. An empty literal takes the field away.
func PlanTermEdit(b model.Book, r model.EditTerm, m Moment) (Plan, error) {
	t, ok := termOf(b, r.Slug)
	switch {
	case !ok:
		return Plan{}, refuse("list the terms in terms.yaml, or define it with gragas term new", "the term %s is not in the book", r.Slug)
	case t.Status == model.Retired:
		return Plan{}, refuse("a retired term is not edited: define a new term", "the term %s is retired", r.Slug)
	case r.Literal == nil:
		return Plan{}, refuse("give the new literal with --literal; the definition is prose, edit terms.yaml", "no field is given to change")
	case !clean(*r.Literal):
		return Plan{}, refuse("write the literal on one line", "the literal runs over more than one line")
	}
	if *r.Literal == t.Literal {
		return nothingTerm("edit", t, fmt.Sprintf("%s has this literal already", r.Slug)), nil
	}
	before := termLifecycle(t)
	after := before
	after.Literal = *r.Literal
	var line string
	switch {
	case *r.Literal == "":
		line = fmt.Sprintf("%s: the literal %s is taken away", r.Slug, t.Literal)
	case t.Literal == "":
		line = fmt.Sprintf("%s: stands for %s", r.Slug, *r.Literal)
	default:
		line = fmt.Sprintf("%s: the literal goes from %s to %s", r.Slug, t.Literal, *r.Literal)
	}
	entity := "term:" + r.Slug
	written := []string{r.Files.Terms + ": sets literal of " + r.Slug, r.Files.History + ": appends one entry (edit)"}
	return Plan{
		Delta:    model.Delta{Changes: []model.Change{{Entity: entity, Field: "literal", Before: t.Literal, After: *r.Literal}}},
		Record:   Record{Kind: "edit", Entity: entity, Before: &before, After: after},
		Sections: sections([]string{line}, written, "none: no project binds a term"),
	}, nil
}

// PlanTermRetire plans the retirement of a term. A term a rule still uses is
// refused, and the refusal names the rules; a retired term plans nothing.
func PlanTermRetire(b model.Book, r model.RetireTerm, m Moment) (Plan, error) {
	t, ok := termOf(b, r.Slug)
	if !ok {
		return Plan{}, refuse("list the terms in terms.yaml", "the term %s is not in the book", r.Slug)
	}
	if err := checkWhy(b, r.Why, "retire", r.Slug); err != nil {
		return Plan{}, err
	}
	if t.Status == model.Retired {
		return nothingTerm("retire", t, fmt.Sprintf("%s is retired already", r.Slug)), nil
	}
	if users := rulesUsing(b, r.Slug); len(users) > 0 {
		return Plan{}, refuse("reword those rules with gragas rule replace, or take the term out of them, then retire it again",
			"the term %s is used by %d rule(s) and is not retired: %s", r.Slug, len(users), strings.Join(users, ", "))
	}
	before := termLifecycle(t)
	after := before
	after.Status = model.Retired
	entity := "term:" + r.Slug
	slug := decisionSlug(r.Why, "retire", r.Slug)
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "status", Before: string(t.Status), After: string(model.Retired)})
	changes = append([]string{fmt.Sprintf("%s: active, then retired. The term stays in its file and no statement may use it", r.Slug)}, changes...)
	written = append([]string{r.Files.Terms + ": sets status of " + r.Slug}, written...)
	written = append(written, r.Files.History+": appends one entry (retire)")
	return Plan{
		Delta:    d,
		Record:   Record{Kind: "retire", Entity: entity, Before: &before, After: after, Decision: slug},
		Sections: sections(changes, written, "none: no rule uses the term"),
	}, nil
}

func termOf(b model.Book, slug string) (model.Term, bool) {
	return bySlug(b.Terms, slug, func(t model.Term) string { return t.Slug })
}

// termLifecycle is the guarded fields of a term as the book holds them: an
// absent status is active.
func termLifecycle(t model.Term) model.Lifecycle {
	s := t.Status
	if s == "" {
		s = model.Active
	}
	return model.Lifecycle{Status: s, Literal: t.Literal}
}

func nothingTerm(kind string, t model.Term, why string) Plan {
	return nothingPlan(kind, "term:"+t.Slug, termLifecycle(t), why)
}

// rulesUsing are the ids of the rules that write {slug} in their statement or
// in the words of their reasoning, in the book's order.
func rulesUsing(b model.Book, slug string) []string {
	var ids []string
	for _, r := range b.Rules {
		texts := []string{r.Statement}
		if rs := r.Reasoning; rs != nil {
			texts = append(texts, rs.Definition)
			texts = append(texts, rs.Pros...)
			texts = append(texts, rs.Cons...)
		}
		if slices.ContainsFunc(texts, func(s string) bool { return mentions(s, slug) }) {
			ids = append(ids, string(r.ID))
		}
	}
	return ids
}

func mentions(text, slug string) bool {
	for _, m := range termRef.FindAllStringSubmatch(text, -1) {
		if m[1] == slug {
			return true
		}
	}
	return false
}
