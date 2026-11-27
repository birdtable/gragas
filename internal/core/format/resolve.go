package format

import (
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// Table answers which rule an id or an alias names. It is built once, from the
// rules of a book, and a lookup is one map read. An id is frozen at birth: a
// renumber adds an alias, and every reference to the old id keeps resolving.
type Table struct {
	rules []model.Rule
	at    map[model.RuleID]int
}

// NewTable indexes rules by id and by alias. Where an id or alias is used
// twice the first rule keeps it; Load reports the second as a finding.
func NewTable(rules []model.Rule) *Table {
	t := &Table{rules: rules, at: make(map[model.RuleID]int, len(rules))}
	for i, r := range rules {
		for _, id := range append([]model.RuleID{r.ID}, r.Aliases...) {
			if _, taken := t.at[id]; !taken {
				t.at[id] = i
			}
		}
	}
	return t
}

// Resolve returns the rule that has id as its id or as one of its aliases.
func (t *Table) Resolve(id model.RuleID) (model.Rule, bool) {
	i, ok := t.at[id]
	if !ok {
		return model.Rule{}, false
	}
	return t.rules[i], true
}

// ref is one place that names a rule, kept to be checked once every file is read.
type ref struct {
	id   model.RuleID
	file string
	line int
	what string
}

// ruleRefs lists what the rule at rm names: its relations, the rule that
// replaces it. A binding's tightens and an exclusion's rule name a rule of
// the parent book, which this book does not hold, so they are not checked
// here. off is the line above the
// first line of rm's text in file.
func ruleRefs(rule model.Rule, rm *ordered.Map, file string, off int) []ref {
	var out []ref
	add := func(ids []model.RuleID, line int, what string) {
		for _, id := range ids {
			out = append(out, ref{id: id, file: file, line: off + line, what: what})
		}
	}
	if x := rule.Relations; x != nil {
		rel := rm.Sub("relations")
		add(x.Relates, rel.KeyLine("relates"), "relates")
		add(x.Conflicts, rel.KeyLine("conflicts"), "conflicts")
		add(x.Depends, rel.KeyLine("depends"), "depends")
		add(x.Covers, rel.KeyLine("covers"), "covers")
	}
	if rule.ReplacedBy != "" {
		add([]model.RuleID{rule.ReplacedBy}, rm.KeyLine("replaced_by"), "replaced_by")
	}
	return out
}

// checkRefs reports each reference that names no rule, and each decision a
// rule's reasoning names that the book does not hold.
func (r reader) checkRefs() {
	t := NewTable(r.out.Book.Rules)
	for _, x := range *r.refs {
		if _, ok := t.Resolve(x.id); !ok {
			r.add(x.file, x.line, fmt.Sprintf("%s names %s, which is no rule's id or alias", x.what, x.id))
		}
	}
	decisions := map[string]bool{}
	for _, d := range r.out.Book.Decisions {
		decisions[d.Slug] = true
	}
	for _, x := range *r.slugs {
		if !decisions[x.id] {
			r.add(x.file, x.line, fmt.Sprintf("decision names %s, which decisions.yaml does not hold", x.id))
		}
	}
}

// slugRef is one place that names a decision by its slug.
type slugRef struct {
	id   string
	file string
	line int
}
