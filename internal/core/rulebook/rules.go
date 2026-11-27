package rulebook

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// checkRule runs the checks that need only the rule itself; the schema has
// already held its shape.
func (b *Book) checkRule(short string, r *ordered.Map) []finding {
	var bad []finding
	add := func(format string, args ...any) {
		bad = append(bad, finding{line: r.Line, msg: fmt.Sprintf(format, args...)})
	}
	id, status, strength := r.Str("id"), r.Str("status"), r.Str("strength")
	if status != "retired" && strings.Split(id, ".")[0] != short {
		add("%s: a rule's id starts with its part (%s); a moved rule is retired with replaced_by", id, short)
	}
	// held for every status: a retired rule's id is not held to its part
	// above, and the reserved word is never a rule's
	if first, _, _ := strings.Cut(id, "."); first == model.EnginePart {
		add("%s: a rule's id never starts with %s, the word the engine's own check ids start with, such as %s", id, model.EnginePart, model.ValidateCheck)
	}
	if status == "retired" && !r.Truthy("replaced_by") && !r.Truthy("note") {
		add("%s: retired needs replaced_by or a note saying why", id)
	}
	if status == "active" && !r.Truthy("sources") {
		add("%s: an active rule cites at least one source; with none it is proposed", id)
	}
	if r.Str("kind") == "rule" && status == "active" && (strength == "Answered" || strength == "Session") && !r.Truthy("checks") {
		add("%s: strength %s needs at least one check", id, strength)
	}
	measured := slices.Contains(measuredStrengths(), strength)
	if measured && !r.Truthy("review_question") && !r.Truthy("promoted") {
		add("%s: %s is not yet promoted, so it needs a review_question meanwhile", id, strength)
	}
	public := ordered.NewMap(0)
	for _, k := range r.Keys() {
		if k != "sources" {
			public.Set(k, r.Val(k), 0)
		}
	}
	if m := b.findName(ordered.JSON(public, -1)); m != "" {
		add("%s names %s: public text must stay generic", id, ordered.Repr(m))
	}
	return bad
}

// checkLinks holds what a rule names to what exists: each citation to the
// registry, each kind to products.yaml, each facet to the part that names
// it, each check to its part's bindings and to a tool of the rulebook.
func (b *Book) checkLinks() {
	for _, f := range b.Files {
		part, _ := b.Parts.Part(f.Part)
		for _, r := range f.Rules() {
			var bad []string
			bad = append(bad, b.citationProblems(r)...)
			bad = append(bad, b.applicabilityProblems(r)...)
			bad = append(bad, b.checkProblems(part, r)...)
			for _, msg := range bad {
				b.add(f.Path, finding{line: r.Line, msg: r.Str("id") + ": " + msg})
			}
		}
	}
	for _, k := range b.Kinds {
		for _, c := range ordered.Maps(k.List("instances")) {
			if _, ok := b.Sources.Get(c.Str("ref")); !ok {
				b.add(path.Join(Dir, productsFile), finding{line: c.Line, msg: fmt.Sprintf("%s: source %s is not in %s", k.Str("key"), c.Str("ref"), sourcesFile)})
			}
		}
	}
}

func (b *Book) citationProblems(r *ordered.Map) []string {
	var bad []string
	for _, c := range ordered.Maps(r.List("sources")) {
		ref := c.Str("ref")
		src := b.Sources.Sub(ref)
		switch {
		case src == nil:
			bad = append(bad, fmt.Sprintf("source %s is not in %s", ref, sourcesFile))
		case c.Truthy("covers") && src.Str("kind") != "external-rule":
			bad = append(bad, fmt.Sprintf("covers is only for a source of kind external-rule, and %s is %s", ref, src.Str("kind")))
		}
	}
	return bad
}

func (b *Book) applicabilityProblems(r *ordered.Map) []string {
	conds := ordered.Maps(r.List("unless"))
	if w := r.Sub("when"); w != nil {
		conds = append(conds, w)
	}
	var bad []string
	for _, c := range conds {
		for _, k := range ordered.Strings(c.List("kinds")) {
			if !b.kinds[k] {
				bad = append(bad, fmt.Sprintf("kind %s is not in %s", k, productsFile))
			}
		}
		for _, f := range ordered.Strings(c.List("facets")) {
			if _, ok := b.Parts.FacetOwner(f); !ok {
				bad = append(bad, fmt.Sprintf("facet %s is named by no part", f))
			}
		}
	}
	return bad
}

func (b *Book) checkProblems(part Part, r *ordered.Map) []string {
	var bad []string
	for _, c := range ordered.Maps(r.List("checks")) {
		if bnd := c.Str("binding"); !slices.Contains(part.Bindings, bnd) {
			bad = append(bad, fmt.Sprintf("binding %s is not one of part %s's bindings", bnd, part.Short))
		}
		if t := c.Str("tool"); t != "" && b.tools[t] == nil {
			bad = append(bad, fmt.Sprintf("tool %s is in no tools.yaml", t))
		}
	}
	return bad
}
