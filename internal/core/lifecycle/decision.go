package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// The two plans of the decision family: a decision recorded, and a decision
// superseded. A decision is never edited and never deleted (S59): a change of
// meaning is a new record that names the old one, and the old one names it
// back, so the chain reads from either end. Rewording is a plain file edit.

// PlanDecisionNew plans a new decision. A decision the book holds with the same
// words plans nothing, so the second run ends "done, nothing to do".
func PlanDecisionNew(b model.Book, r model.NewDecisionRecord, m Moment) (Plan, error) {
	if err := checkDecisionWords(r.Slug, r.Title, r.Text, r.Rejected); err != nil {
		return Plan{}, err
	}
	if err := checkCites(b, r); err != nil {
		return Plan{}, err
	}
	body := decisionBody(model.Decision{Slug: r.Slug, Title: r.Title, Text: r.Text, Rejected: r.Rejected,
		Cites: &model.Cites{Sources: r.Sources, Rules: r.Rules, Bindings: r.Bindings}}, m)
	if d, ok := decisionOf(b, r.Slug); ok {
		if sameDecision(d, r) {
			return nothingDecision("new", d, fmt.Sprintf("%s is recorded already, with these words", r.Slug)), nil
		}
		return Plan{}, refuse("supersede it with gragas decision supersede "+r.Slug+", or pick another slug", "the decision %s is in the book already, with other words", r.Slug)
	}
	entity := "decision:" + r.Slug
	written := []string{r.Files.Decisions + ": adds decision " + r.Slug, r.Files.History + ": appends one entry (new)"}
	return Plan{
		Delta:    model.Delta{Changes: []model.Change{{Entity: entity, After: body}}},
		Record:   Record{Kind: "new", Entity: entity, After: decisionLifecycle(model.Decision{Slug: r.Slug})},
		Sections: sections([]string{fmt.Sprintf("new decision %s: %s, with %d rejected option(s)", r.Slug, r.Title, len(r.Rejected))}, written, "none: no rule stands on it yet"),
	}, nil
}

// PlanDecisionSupersede plans a new decision that replaces an old one. The old
// record stays and is marked superseded by the new one, the new one names the
// old, and each rule that stood on the old decision moves to the new one and
// is listed in the new one's citations. It is one plan: the two records, the
// rules and a history line for each. The old decision must be the head of its
// chain; a superseded one is refused, naming the head.
func PlanDecisionSupersede(b model.Book, r model.SupersedeDecision, m Moment) (Plan, error) {
	old, ok := decisionOf(b, r.Old)
	if !ok {
		return Plan{}, refuse("list the decisions in decisions.yaml", "the decision %s is not in the book", r.Old)
	}
	if err := checkDecisionWords(r.Slug, r.Title, r.Text, r.Rejected); err != nil {
		return Plan{}, err
	}
	if old.SupersededBy != "" {
		if r.Slug == old.SupersededBy {
			if n, found := decisionOf(b, r.Slug); found && n.Supersedes == old.Slug && n.Title == r.Title && n.Text == r.Text && slices.Equal(n.Rejected, r.Rejected) {
				return nothingDecision("supersede", old, fmt.Sprintf("%s is superseded by %s already, with these words", old.Slug, r.Slug)), nil
			}
		}
		head := decisionHead(b, old.Slug)
		return Plan{}, refuse("supersede the head of the chain: gragas decision supersede "+head, "the decision %s is superseded already: the head of its chain is %s", old.Slug, head)
	}
	if r.Slug == r.Old || hasDecision(b, r.Slug) {
		return Plan{}, refuse("name the new decision with --decision-slug", "the decision %s is in the book already", r.Slug)
	}
	var stood []model.RuleID
	for _, rule := range b.Rules {
		if rule.Reasoning != nil && rule.Reasoning.Decision == old.Slug {
			stood = append(stood, rule.ID)
		}
	}
	d := model.Delta{Changes: []model.Change{
		{Entity: "decision:" + r.Slug, After: decisionBody(model.Decision{Slug: r.Slug, Title: r.Title, Text: r.Text, Rejected: r.Rejected,
			Cites: &model.Cites{Rules: stood}, Supersedes: old.Slug}, m)},
		{Entity: "decision:" + old.Slug, Field: "superseded_by", After: r.Slug},
	}}
	changes := []string{
		fmt.Sprintf("new decision %s: %s, which supersedes %s", r.Slug, r.Title, old.Slug),
		fmt.Sprintf("%s is marked superseded by %s and stays in the book", old.Slug, r.Slug),
	}
	var also []Record
	newLife := model.Lifecycle{Literal: "supersedes " + old.Slug}
	also = append(also, Record{Kind: "new", Entity: "decision:" + r.Slug, After: newLife, Decision: r.Slug})
	for _, id := range stood {
		rule, _ := ruleByID(b, id)
		lc := lifecycleOf(rule)
		d.Changes = append(d.Changes, model.Change{Entity: "rule:" + string(id), Field: "reasoning.decision", Before: old.Slug, After: r.Slug})
		also = append(also, Record{Kind: "decision-superseded", Entity: "rule:" + string(id), Before: &lc, After: lc, Decision: r.Slug})
	}
	consumer := "none: no rule stands on " + old.Slug
	if len(stood) > 0 {
		consumer = fmt.Sprintf("%d rule(s) stand on %s now, and are listed in its citations: %s", len(stood), r.Slug, joinIDs(stood))
		changes = append(changes, fmt.Sprintf("%d rule(s) move from %s to %s: %s", len(stood), old.Slug, r.Slug, joinIDs(stood)))
	}
	written := []string{r.Files.Decisions + ": adds " + r.Slug + " and sets superseded_by of " + old.Slug}
	if files := r.Files.RuleFilesOf(stood); len(files) > 0 {
		written = append(written, fmt.Sprintf("%s: sets the decision of %d rule(s)", strings.Join(files, ", "), len(stood)))
	}
	written = append(written, fmt.Sprintf("%s: appends %d entries (the supersession, the new decision, and one for each rule)", r.Files.History, 2+len(stood)))
	before, after := decisionLifecycle(old), decisionLifecycle(model.Decision{Slug: old.Slug, SupersededBy: r.Slug})
	return Plan{
		Delta:    d,
		Record:   Record{Kind: "supersede", Entity: "decision:" + old.Slug, Before: &before, After: after, Decision: r.Slug},
		Also:     also,
		Sections: sections(changes, written, consumer),
	}, nil
}

func checkDecisionWords(slug, title, text string, rejected []string) error {
	switch {
	case !validSlug(slug):
		return refuse("write the decision as lower-case words joined by hyphens with --decision-slug", "the decision %q is not a slug", slug)
	case strings.TrimSpace(title) == "" || !clean(title) || strings.TrimSpace(text) == "" || !clean(text):
		return refuse("give a title and a text, each on one line", "the decision has no title or no text, or one runs over more than one line")
	case len(rejected) == 0:
		return refuse("give the option not taken, and why, with --rejected", "the decision names no rejected option, and a record without one is not a decision")
	}
	for _, o := range rejected {
		if strings.TrimSpace(o) == "" || !clean(o) {
			return refuse("give each rejected option on one line", "a rejected option is empty or runs over more than one line")
		}
	}
	return nil
}

func checkCites(b model.Book, r model.NewDecisionRecord) error {
	for _, s := range r.Sources {
		if !hasSource(b, s) {
			return refuse("record it with gragas source new", "the source %s is not in the book", s)
		}
	}
	for _, id := range r.Rules {
		if _, ok := ruleByID(b, id); !ok {
			return refuse("list the rules with gragas rule list", "the rule %s is not in the book", id)
		}
	}
	for _, bd := range r.Bindings {
		if len(strings.Fields(bd)) != 3 || !clean(bd) {
			return refuse("write a binding as <rule id> <tool> <key>", "%q is not a binding", bd)
		}
	}
	return nil
}

// decisionBody is the YAML mapping of a new decision: d's words, cites and
// supersedes, and who and when from m.
func decisionBody(d model.Decision, m Moment) string {
	var w strings.Builder
	fmt.Fprintf(&w, "slug: %s\ntitle: %s\ntext: %s\n", d.Slug, scalar(d.Title), scalar(d.Text))
	list := func(name string, items []string) {
		if len(items) > 0 {
			fmt.Fprintf(&w, "%s:\n", name)
			for _, it := range items {
				fmt.Fprintf(&w, "  - %s\n", scalar(it))
			}
		}
	}
	list("rejected", d.Rejected)
	var cs model.Cites
	if d.Cites != nil {
		cs = *d.Cites
	}
	if len(cs.Sources)+len(cs.Rules)+len(cs.Bindings) > 0 {
		w.WriteString("cites:\n")
		for _, c := range []struct {
			name  string
			items []string
		}{{"sources", cs.Sources}, {"rules", idStrings(cs.Rules)}, {"bindings", cs.Bindings}} {
			if len(c.items) > 0 {
				fmt.Fprintf(&w, "  %s:\n", c.name)
				for _, it := range c.items {
					fmt.Fprintf(&w, "    - %s\n", scalar(it))
				}
			}
		}
	}
	fmt.Fprintf(&w, "who: %s\ndate: %s\n", scalar(m.Who), scalar(m.Date))
	if d.Supersedes != "" {
		fmt.Fprintf(&w, "supersedes: %s\n", d.Supersedes)
	}
	return w.String()
}

func sameDecision(d model.Decision, r model.NewDecisionRecord) bool {
	var cs model.Cites
	if d.Cites != nil {
		cs = *d.Cites
	}
	return d.Title == r.Title && d.Text == r.Text && slices.Equal(d.Rejected, r.Rejected) &&
		slices.Equal(cs.Sources, r.Sources) && slices.Equal(cs.Rules, r.Rules) && slices.Equal(cs.Bindings, r.Bindings)
}

// decisionHead is the last decision of the chain old starts: the one nothing
// supersedes. A loop in the chain ends it where it closes.
func decisionHead(b model.Book, slug string) string {
	seen := map[string]bool{}
	for !seen[slug] {
		seen[slug] = true
		d, ok := decisionOf(b, slug)
		if !ok || d.SupersededBy == "" {
			return slug
		}
		slug = d.SupersededBy
	}
	return slug
}

func decisionOf(b model.Book, slug string) (model.Decision, bool) {
	return bySlug(b.Decisions, slug, func(d model.Decision) string { return d.Slug })
}

func ruleByID(b model.Book, id model.RuleID) (model.Rule, bool) {
	i := slices.IndexFunc(b.Rules, func(r model.Rule) bool { return r.ID == id })
	if i < 0 {
		return model.Rule{}, false
	}
	return b.Rules[i], true
}

// decisionLifecycle is the guarded state of a decision as a history entry names
// it: it has no status of its own, so its literal says whether it stands or
// which decision replaced it.
func decisionLifecycle(d model.Decision) model.Lifecycle {
	if d.SupersededBy != "" {
		return model.Lifecycle{Literal: "superseded by " + d.SupersededBy}
	}
	return model.Lifecycle{Literal: "head"}
}

func nothingDecision(kind string, d model.Decision, why string) Plan {
	return nothingPlan(kind, "decision:"+d.Slug, decisionLifecycle(d), why)
}

func idStrings(ids []model.RuleID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func joinIDs(ids []model.RuleID) string { return strings.Join(idStrings(ids), ", ") }
