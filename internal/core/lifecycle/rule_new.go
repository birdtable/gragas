package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// PlanRuleNew plans a new rule. Its number is the highest the concern has
// ever used, in an id or an alias, plus one, so a retired or renumbered id is
// never given again (A5). The prose is seeded once here; nothing rewrites it
// later (S59).
func PlanRuleNew(b model.Book, r model.NewRule, m Moment) (Plan, error) {
	if err := checkNew(b, r); err != nil {
		return Plan{}, err
	}
	if i := slices.IndexFunc(b.Rules, func(x model.Rule) bool {
		return x.Statement == r.Statement && x.Status != model.Retired && strings.HasPrefix(string(x.ID), r.Part+"."+r.Concern+".")
	}); i >= 0 {
		same := b.Rules[i]
		return Plan{
			Record:   Record{Kind: "new", Entity: "rule:" + string(same.ID), After: lifecycleOf(same)},
			Sections: sections([]string{"nothing: " + string(same.ID) + " already says this"}, nil, "none"),
		}, nil
	}
	id := fmt.Sprintf("%s.%s.%d", r.Part, r.Concern, nextNumber(b, r.Part, r.Concern))
	slug := strings.ReplaceAll(id, ".", "-")
	cite := slices.Clone(r.Cite)
	var d model.Delta
	var changes, written []string
	var decision string
	if r.Decision != nil {
		decision = r.Decision.Slug
		if decision == "" {
			decision = "decide-" + slug
		}
	}
	if r.Own {
		own := "own-" + slug
		if hasSource(b, own) {
			return Plan{}, refuse("remove the source or cite it with --source", "the source %s is in the book already", own)
		}
		cite = append(cite, own)
		d.Changes = append(d.Changes, model.Change{Entity: "source:" + own, After: fmt.Sprintf(
			"slug: %s\nkind: %s\ntitle: %s\nlocator: %s\n", own, model.SourceOwnDecision, scalar("Own decision on "+id), decision)})
		changes = append(changes, "add source "+own+" (own decision)")
		written = append(written, r.Files.Sources+": adds source "+own)
	}
	if r.Decision != nil {
		d.Changes = append(d.Changes, model.Change{Entity: "decision:" + decision, After: fmt.Sprintf(
			"slug: %s\ntitle: %s\ntext: %s\nwho: %s\ndate: %s\n", decision, scalar(r.Decision.Title), scalar(r.Decision.Text), scalar(m.Who), scalar(m.Date))})
		changes = append(changes, "add decision "+decision)
		written = append(written, r.Files.Decisions+": adds decision "+decision)
	}
	host, err := ruleHost(b, r, id)
	if err != nil {
		return Plan{}, err
	}
	host.After = newBody(id, r, cite, decision)
	d.Changes = append(d.Changes, host)
	changes = append([]string{fmt.Sprintf("new rule %s: %s, %s. Standing: none before, proposed after", id, r.Strength, model.Proposed)}, changes...)
	written = append([]string{r.Files.Rules + ": adds rule " + id}, written...)
	written = append(written, r.Files.History+": appends one entry (new)")
	return Plan{
		Delta:    d,
		Record:   Record{Kind: "new", Entity: "rule:" + id, After: model.Lifecycle{Status: model.Proposed, Owner: r.Owner, ReviewBy: r.ReviewBy}, Decision: decision},
		Sections: sections(changes, written, "none: a proposed rule binds no project"),
	}, nil
}

func checkNew(b model.Book, r model.NewRule) error {
	switch {
	case !hasPart(b, r.Part):
		return refuse("name a part of structure.yaml with --part", "the part %q is not in the structure", r.Part)
	case !hasConcern(b, r.Part, r.Concern):
		return refuse("name a concern of "+r.Part+" with --concern", "the concern %q is not a concern of %s", r.Concern, r.Part)
	case strings.TrimSpace(r.Statement) == "" || !clean(r.Statement):
		return refuse("give the rule in one line with --statement", "the statement is empty or runs over more than one line")
	case !slices.Contains([]model.Strength{model.Must, model.Should, model.May}, r.Strength):
		return refuse("use --strength must, should or may", "the strength %q is not one of must, should or may", r.Strength)
	case !clean(r.Owner) || !clean(r.Definition):
		return refuse("write the owner and the definition on one line", "the owner or the definition runs over more than one line")
	case r.ReviewBy != "" && !validDate(r.ReviewBy):
		return refuse("write the date as YYYY-MM-DD with --review-by", "review_by %q is not a date", r.ReviewBy)
	}
	for _, s := range r.Cite {
		if !hasSource(b, s) {
			return refuse("list the sources with gragas source list, or add one first", "the source %s is not in the book", s)
		}
	}
	if err := checkScope(b, r.Scope); err != nil {
		return err
	}
	return checkNewDecision(b, r)
}

func checkScope(b model.Book, s *model.RuleScope) error {
	if s == nil {
		return nil
	}
	for _, p := range s.Parts {
		if !hasPart(b, p) {
			return refuse("name a part of structure.yaml", "the scope names the part %q, which is not in the structure", p)
		}
	}
	for _, k := range s.Kinds {
		if !hasKind(b, k) {
			return refuse("name a kind of structure.yaml", "the scope names the kind %q, which is not in the structure", k)
		}
	}
	for _, p := range s.Paths {
		if p == "" || !clean(p) {
			return refuse("write each path as a glob on one line", "a path of the scope is empty or runs over more than one line")
		}
	}
	return nil
}

func checkNewDecision(b model.Book, r model.NewRule) error {
	d := r.Decision
	if d == nil {
		if r.Own {
			return refuse("give the decision with --decision-title and --decision-text", "an own decision needs a decision to point at")
		}
		return nil
	}
	switch {
	case strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Text) == "" || !clean(d.Title) || !clean(d.Text):
		return refuse("give the decision a title and a text, each on one line", "the decision has no title or no text, or one runs over more than one line")
	case d.Slug != "" && !validSlug(d.Slug):
		return refuse("use lower-case words joined by hyphens", "the decision slug %q is not a slug", d.Slug)
	case d.Slug != "" && hasDecision(b, d.Slug):
		return refuse("name a decision that is not taken, or cite this one", "the decision %s is taken", d.Slug)
	}
	return nil
}

// nextNumber is the number after the highest that part.concern has used in
// any id or alias of the book.
func nextNumber(b model.Book, part, concern string) int {
	prefix := part + "." + concern + "."
	top := 0
	for _, r := range slices.Concat(b.Rules, b.Clashed) {
		for _, id := range append([]model.RuleID{r.ID}, r.Aliases...) {
			if rest, ok := strings.CutPrefix(string(id), prefix); ok {
				if n, ok := number(rest); ok {
					top = max(top, n)
				}
			}
		}
	}
	return top + 1
}

// newBody is the rule as the mapping Render adds to the concern file: the
// anatomy in the order the files keep it, with only what was given.
func newBody(id string, r model.NewRule, cite []string, decision string) string {
	var w strings.Builder
	fmt.Fprintf(&w, "id: %s\nstatement: %q\nstrength: %s\nstatus: %s\norigin: %s\n", id, r.Statement, r.Strength, model.Proposed, model.Authored)
	if r.Owner != "" {
		fmt.Fprintf(&w, "owner: %s\n", scalar(r.Owner))
	}
	if r.ReviewBy != "" {
		fmt.Fprintf(&w, "review_by: %s\n", scalar(r.ReviewBy))
	}
	w.WriteString(scopeBody(r.Scope))
	if r.Definition != "" || decision != "" {
		w.WriteString("reasoning:\n")
		if r.Definition != "" {
			fmt.Fprintf(&w, "  definition: %s\n", scalar(r.Definition))
		}
		if decision != "" {
			fmt.Fprintf(&w, "  decision: %s\n", decision)
		}
	}
	if len(cite) > 0 {
		w.WriteString("sources:\n")
		for _, c := range cite {
			fmt.Fprintf(&w, "  - ref: %s\n    backs: %s\n", c, model.BacksRule)
		}
	}
	return w.String()
}

// ruleHost is the change that adds the rule: a whole rule to its concern
// file, or, when the concern is written in a guide, a rule to the block its
// other rules stand in. A guide concern with no rule in a block is refused,
// since no block is known to be the one.
func ruleHost(b model.Book, r model.NewRule, id string) (model.Change, error) {
	if !strings.HasSuffix(r.Files.Rules, ".md") {
		return model.Change{Entity: "rule:" + id}, nil
	}
	prefix := r.Part + "." + r.Concern + "."
	for _, x := range b.Rules {
		if a, ok := b.Anchors[x.ID]; ok && strings.HasPrefix(string(x.ID), prefix) {
			return model.Change{Entity: "block:" + r.Files.Rules + "#" + a, Field: "rules[]"}, nil
		}
	}
	return model.Change{}, refuse("add the first block of "+r.Part+"."+r.Concern+" to "+r.Files.Rules+" by hand, then run again",
		"the guide %s holds no block for %s.%s to add the rule to", r.Files.Rules, r.Part, r.Concern)
}

// scopeBody is the scope mapping of a rule, "" when it names nothing.
func scopeBody(s *model.RuleScope) string {
	if s == nil || len(s.Parts)+len(s.Paths)+len(s.Kinds) == 0 {
		return ""
	}
	var w strings.Builder
	w.WriteString("scope:\n")
	for _, l := range []struct {
		key   string
		items []string
	}{{"parts", s.Parts}, {"paths", s.Paths}, {"kinds", s.Kinds}} {
		if len(l.items) == 0 {
			continue
		}
		fmt.Fprintf(&w, "  %s:\n", l.key)
		for _, it := range l.items {
			fmt.Fprintf(&w, "    - %s\n", scalar(it))
		}
	}
	return w.String()
}
