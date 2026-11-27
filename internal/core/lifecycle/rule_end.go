package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// The three plans that end or return a rule's life. Nothing is deleted: a
// retired rule stays in its file with the status retired and, when there is
// one, a pointer to what took its place. Each stands on a decision, new or
// cited, because a change of a rule's life is a decision (S59); a rule's
// words change only by replace, never by a reword.

// PlanRuleRetire plans the retirement of a rule. A rule that is retired
// already plans nothing, so the second run ends "done, nothing to do".
func PlanRuleRetire(b model.Book, r model.RetireRule, m Moment) (Plan, error) {
	rule, err := ruleOf(b, r.ID)
	if err != nil {
		return Plan{}, err
	}
	if err := checkWhy(b, r.Why, "retire", r.ID); err != nil {
		return Plan{}, err
	}
	if r.Successor != "" {
		if err := checkSuccessor(b, rule, r.Successor); err != nil {
			return Plan{}, err
		}
	}
	before := lifecycleOf(rule)
	if rule.Status == model.Retired {
		return nothing("retire", rule, fmt.Sprintf("%s is retired already", r.ID)), nil
	}
	slug := decisionSlug(r.Why, "retire", r.ID)
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	entity := "rule:" + r.ID
	after := before
	after.Status = model.Retired
	d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "status", Before: string(before.Status), After: string(model.Retired)})
	changes = append([]string{fmt.Sprintf("%s: %s, then retired. The rule stays in its file", r.ID, before.Status)}, changes...)
	if r.Successor != "" && r.Successor != rule.ReplacedBy {
		d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "replaced_by", Before: string(rule.ReplacedBy), After: string(r.Successor)})
		after.ReplacedBy = r.Successor
		changes = append(changes, fmt.Sprintf("%s is replaced by %s", r.ID, r.Successor))
	}
	written = append([]string{r.Files.Rules + ": sets status of " + r.ID}, written...)
	written = append(written, r.Files.History+": appends one entry (retire)")
	return Plan{Delta: d, Record: Record{Kind: "retire", Entity: entity, Before: &before, After: after, Decision: slug},
		Sections: sections(changes, written, consumerLine(r.ID, string(r.Successor)))}, nil
}

// PlanRuleRestore plans the return of a retired rule as active. It skips the
// trial: the rule had been active. Its pointer to a successor is taken away,
// and the history keeps the retirement and the restore as two events.
func PlanRuleRestore(b model.Book, r model.RestoreRule, m Moment) (Plan, error) {
	rule, err := ruleOf(b, r.ID)
	if err != nil {
		return Plan{}, err
	}
	switch {
	case rule.Status != model.Retired:
		return Plan{}, refuse("change a live rule's fields with gragas rule edit "+r.ID, "the status of %s is %s, and only a retired rule is restored", r.ID, rule.Status)
	case !validDate(r.ReviewBy):
		return Plan{}, refuse("write the date it is next confirmed with --review-by YYYY-MM-DD", "review_by %q is not a date", r.ReviewBy)
	}
	if err := checkWhy(b, r.Why, "restore", r.ID); err != nil {
		return Plan{}, err
	}
	before := lifecycleOf(rule)
	slug := decisionSlug(r.Why, "restore", r.ID)
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	entity := "rule:" + r.ID
	after := before
	after.Status, after.ReviewBy, after.ReplacedBy = model.Active, r.ReviewBy, ""
	d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "status", Before: string(model.Retired), After: string(model.Active)})
	if rule.ReviewBy != r.ReviewBy {
		d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "review_by", Before: rule.ReviewBy, After: r.ReviewBy})
	}
	if rule.ReplacedBy != "" {
		d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "replaced_by", Before: string(rule.ReplacedBy), After: ""})
	}
	changes = append([]string{fmt.Sprintf("%s: retired, then active, next confirmed %s. Its bindings are proven again later", r.ID, r.ReviewBy)}, changes...)
	written = append([]string{r.Files.Rules + ": sets status of " + r.ID}, written...)
	written = append(written, r.Files.History+": appends one entry (restore)")
	return Plan{Delta: d, Record: Record{Kind: "restore", Entity: entity, Before: &before, After: after, Decision: slug},
		Sections: sections(changes, written, "none until bindings: the rule binds again once they are proven")}, nil
}

// PlanRuleReplace plans a successor rule in the old rule's concern and the
// retirement of the old rule with a pointer to it, as one change with one
// history line, on the old rule. A second run, when the old rule is retired
// and points at a rule that says the same, plans nothing.
func PlanRuleReplace(b model.Book, r model.ReplaceRule, m Moment) (Plan, error) {
	old, err := ruleOf(b, r.ID)
	if err != nil {
		return Plan{}, err
	}
	if old.Status == model.Retired {
		if i := slices.IndexFunc(b.Rules, func(x model.Rule) bool { return x.ID == old.ReplacedBy && x.Statement == r.Successor.Statement }); i >= 0 {
			return nothing("replace", old, fmt.Sprintf("%s is replaced by %s already", r.ID, old.ReplacedBy)), nil
		}
		return Plan{}, refuse("restore it with gragas rule restore "+r.ID+" first, or retire nothing: add the rule with gragas rule new", "%s is retired, and a retired rule is not replaced", r.ID)
	}
	if err := checkWhy(b, r.Why, "replace", r.ID); err != nil {
		return Plan{}, err
	}
	slug := decisionSlug(r.Why, "replace", r.ID)
	succ := r.Successor
	succ.Part, succ.Concern, succ.Files = partOf(r.ID), concernOf(r.ID), r.Files
	succ.Decision, succ.Own = nil, false
	made, err := PlanRuleNew(b, succ, m)
	if err != nil {
		return Plan{}, err
	}
	newID := strings.TrimPrefix(made.Record.Entity, "rule:")
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	d.Changes = append(d.Changes, made.Delta.Changes...)
	entity := "rule:" + r.ID
	before := lifecycleOf(old)
	after := before
	after.Status, after.ReplacedBy = model.Retired, model.RuleID(newID)
	d.Changes = append(d.Changes,
		model.Change{Entity: entity, Field: "status", Before: string(before.Status), After: string(model.Retired)},
		model.Change{Entity: entity, Field: "replaced_by", Before: string(old.ReplacedBy), After: newID})
	changes = append([]string{
		fmt.Sprintf("new rule %s: %s, %s. Standing: none before, proposed after", newID, succ.Strength, model.Proposed),
		fmt.Sprintf("%s: %s, then retired, replaced by %s", r.ID, before.Status, newID)}, changes...)
	written = append([]string{r.Files.Rules + ": adds rule " + newID + ", sets status and replaced_by of " + r.ID}, written...)
	written = append(written, r.Files.History+": appends one entry (replace)")
	return Plan{Delta: d, Record: Record{Kind: "replace", Entity: entity, Before: &before, After: after, Decision: slug},
		Sections: sections(changes, written, consumerLine(r.ID, newID))}, nil
}

// nothing is the plan of a goal that holds: no delta, and a record that is
// valid, since the write path asks for it once even for an empty plan.
func nothing(kind string, rule model.Rule, why string) Plan {
	now := lifecycleOf(rule)
	return Plan{Record: Record{Kind: kind, Entity: "rule:" + string(rule.ID), Before: &now, After: now},
		Sections: sections([]string{"nothing: " + why}, nil, "none")}
}

func consumerLine(old, successor string) string {
	if successor == "" {
		return "projects that bind " + old + " are told it is retired, with no rule to move to"
	}
	return "projects that bind " + old + " move to " + successor + " when it is active"
}

// ruleOf finds the rule by its current id; an old id is refused with the
// current one in the hint.
func ruleOf(b model.Book, id string) (model.Rule, error) {
	i := slices.IndexFunc(b.Rules, func(x model.Rule) bool { return string(x.ID) == id })
	if i < 0 {
		return model.Rule{}, notFound(b, id)
	}
	return b.Rules[i], nil
}

func checkSuccessor(b model.Book, rule model.Rule, id model.RuleID) error {
	if id == rule.ID {
		return refuse("name another rule with --successor", "a rule cannot be its own successor: %s names itself", id)
	}
	s, err := ruleOf(b, string(id))
	switch {
	case err != nil:
		return err
	case s.Status == model.Retired:
		return refuse("name a rule that is active or proposed", "the successor %s is retired", id)
	}
	return nil
}

// checkWhy holds a decision to be new or cited, one of the two. verb names
// the command in the hint.
func checkWhy(b model.Book, w model.Why, verb, id string) error {
	switch {
	case w.Cite != "" && w.New != nil:
		return refuse("give one of --decision and --decision-text", "the decision is both cited and new: give one")
	case w.Cite == "" && w.New == nil:
		return refuse("cite a decision with --decision <slug>, or take one with --decision-title and --decision-text", "%s stands on a decision, and none is given", verb)
	case w.Cite != "" && !hasDecision(b, w.Cite):
		return refuse("list the decisions with gragas decision list", "the decision %s is not in the book", w.Cite)
	case w.New == nil:
		return nil
	}
	d := w.New
	switch {
	case strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Text) == "" || !clean(d.Title) || !clean(d.Text):
		return refuse("give the decision a title and a text, each on one line", "the decision has no title or no text, or one runs over more than one line")
	case d.Slug != "" && !validSlug(d.Slug):
		return refuse("use lower-case words joined by hyphens", "the decision slug %q is not a slug", d.Slug)
	case hasDecision(b, decisionSlug(w, verb, id)):
		return refuse("name a new one with --decision-slug, or cite this one with --decision", "the decision %s is taken", decisionSlug(w, verb, id))
	}
	return nil
}

// decisionSlug is the decision a change stands on: the cited one, the slug
// given, or decide-<verb>-<id as a slug>.
func decisionSlug(w model.Why, verb, id string) string {
	switch {
	case w.Cite != "":
		return w.Cite
	case w.New != nil && w.New.Slug != "":
		return w.New.Slug
	}
	return "decide-" + verb + "-" + strings.ReplaceAll(id, ".", "-")
}

// whyChanges adds the decision to the delta when it is new.
func whyChanges(w model.Why, slug string, m Moment, f model.PlanFiles) (d model.Delta, changes, written []string) {
	if w.New == nil {
		return d, nil, nil
	}
	d.Changes = append(d.Changes, model.Change{Entity: "decision:" + slug, After: fmt.Sprintf(
		"slug: %s\ntitle: %s\ntext: %s\nwho: %s\ndate: %s\n", slug, scalar(w.New.Title), scalar(w.New.Text), scalar(m.Who), scalar(m.Date))})
	return d, []string{"add decision " + slug}, []string{f.Decisions + ": adds decision " + slug}
}

func partOf(id string) string    { p, _, _ := strings.Cut(id, "."); return p }
func concernOf(id string) string { parts := strings.Split(id, "."); return parts[1] }
