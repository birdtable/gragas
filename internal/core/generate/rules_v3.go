package generate

import (
	"slices"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// SchemaVersion3 is the schema rules.json is written in when it is made from
// a format-3 book.
const SchemaVersion3 = 3

// RulesJSONOf is the rules.json of a format-3 book: its parts and kinds from
// structure.yaml, its sources from sources.yaml, and every rule with the part
// and concern it is filed under and the standing held says. A rule held has
// no row in held when its status or strength is refused; it is written with
// no standing, and the book's own check refuses it. The bytes are the same for
// the same book.
func RulesJSONOf(b model.Book, held []model.Held) []byte {
	standing := make(map[model.RuleID]model.Standing, len(held))
	for _, h := range held {
		standing[h.Rule] = h.Standing
	}
	var parts, kinds, sources, rules []any
	for _, l := range b.Structure.Layers {
		for _, p := range l.Parts {
			parts = append(parts, partRowOf(l, p))
		}
	}
	for _, k := range b.Structure.Kinds {
		m := ordered.NewMap(0)
		m.Set("slug", k.Slug, 0)
		m.Set("name", k.Name, 0)
		setText(m, "description", k.Description)
		kinds = append(kinds, m)
	}
	for _, s := range b.Sources {
		sources = append(sources, sourceRowOf(s))
	}
	for _, r := range b.Rules {
		rules = append(rules, ruleRowOf(r, standing[r.ID]))
	}
	top := ordered.NewMap(0)
	top.Set("generated_by", GeneratedBy, 0)
	top.Set("schema", int64(SchemaVersion3), 0)
	top.Set("parts", listOf(parts), 0)
	top.Set("kinds", listOf(kinds), 0)
	top.Set("sources", listOf(sources), 0)
	top.Set("rules", listOf(rules), 0)
	return []byte(ordered.JSON(top, jsonIndent) + "\n")
}

// listOf is the list, never nil, so an empty one is written as [].
func listOf(xs []any) []any {
	if xs == nil {
		return []any{}
	}
	return xs
}

func setText(m *ordered.Map, key, v string) {
	if v != "" {
		m.Set(key, v, 0)
	}
}

func strings3[T ~string](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = string(x)
	}
	return out
}

func setList[T ~string](m *ordered.Map, key string, xs []T) {
	if len(xs) > 0 {
		m.Set(key, strings3(xs), 0)
	}
}

func partRowOf(l model.Layer, p model.Part) *ordered.Map {
	m := ordered.NewMap(0)
	m.Set("slug", p.Slug, 0)
	m.Set("layer", l.Slug, 0)
	m.Set("name", p.Name, 0)
	setList(m, "aliases", p.Aliases)
	setList(m, "owners", p.Owners)
	concerns := make([]any, len(p.Concerns))
	for i, c := range p.Concerns {
		cm := ordered.NewMap(0)
		cm.Set("slug", c.Slug, 0)
		cm.Set("name", c.Name, 0)
		setList(cm, "aliases", c.Aliases)
		concerns[i] = cm
	}
	m.Set("concerns", concerns, 0)
	return m
}

func sourceRowOf(s model.Source) *ordered.Map {
	m := ordered.NewMap(0)
	m.Set("slug", s.Slug, 0)
	m.Set("kind", string(s.Kind), 0)
	m.Set("title", s.Title, 0)
	m.Set("locator", s.Locator, 0)
	setText(m, "attribution", s.Attribution)
	setText(m, "verified", s.Verified)
	return m
}

// ruleRowOf is one rule: its id, where it is filed, then every field it
// carries, in the order of the model, then its standing.
func ruleRowOf(r model.Rule, s model.Standing) *ordered.Map {
	m := ordered.NewMap(0)
	m.Set("id", string(r.ID), 0)
	part, concern, _ := splitID(r.ID)
	m.Set("part", part, 0)
	m.Set("concern", concern, 0)
	setList(m, "aliases", r.Aliases)
	m.Set("statement", r.Statement, 0)
	m.Set("strength", string(r.Strength), 0)
	m.Set("status", string(r.Status), 0)
	m.Set("origin", string(r.Origin), 0)
	setText(m, "owner", r.Owner)
	setText(m, "review_by", r.ReviewBy)
	setText(m, "zero_by", r.ZeroBy)
	if r.Scope != nil {
		m.Set("scope", scopeRow(r.Scope), 0)
	}
	if len(r.Exemptions) > 0 {
		xs := make([]any, len(r.Exemptions))
		for i, e := range r.Exemptions {
			em := ordered.NewMap(0)
			setList(em, "paths", e.Paths)
			setList(em, "kinds", e.Kinds)
			em.Set("reason", e.Reason, 0)
			xs[i] = em
		}
		m.Set("exemptions", xs, 0)
	}
	if r.Reasoning != nil {
		m.Set("reasoning", reasoningRow(r.Reasoning), 0)
	}
	if r.Relations != nil {
		m.Set("relations", relationsRow(r.Relations), 0)
	}
	if len(r.Sources) > 0 {
		xs := make([]any, len(r.Sources))
		for i, c := range r.Sources {
			cm := ordered.NewMap(0)
			cm.Set("ref", c.Ref, 0)
			setText(cm, "where", c.Where)
			setText(cm, "says", c.Says)
			cm.Set("backs", string(c.Backs), 0)
			xs[i] = cm
		}
		m.Set("sources", xs, 0)
	}
	if len(r.Bindings) > 0 {
		xs := make([]any, len(r.Bindings))
		for i, b := range r.Bindings {
			xs[i] = bindingRow(b)
		}
		m.Set("bindings", xs, 0)
	}
	if len(r.Pairs) > 0 {
		xs := make([]any, len(r.Pairs))
		for i, p := range r.Pairs {
			pm := ordered.NewMap(0)
			pm.Set("number", int64(p.Number), 0)
			pm.Set("target", string(p.Target), 0)
			xs[i] = pm
		}
		m.Set("pairs", xs, 0)
	}
	if r.ReplacedBy != "" {
		m.Set("replaced_by", string(r.ReplacedBy), 0)
	}
	if s != "" {
		m.Set("standing", string(s), 0)
	}
	return m
}

func scopeRow(s *model.RuleScope) *ordered.Map {
	m := ordered.NewMap(0)
	setList(m, "parts", s.Parts)
	setList(m, "paths", s.Paths)
	setList(m, "kinds", s.Kinds)
	return m
}

func bindingRow(b model.Binding) *ordered.Map {
	m := ordered.NewMap(0)
	m.Set("tool", b.Tool, 0)
	m.Set("target", string(b.Target), 0)
	m.Set("key", b.Key, 0)
	if b.Value != nil {
		m.Set("value", plain(b.Value), 0)
	}
	if len(b.Options) > 0 {
		m.Set("options", plain(b.Options), 0)
	}
	m.Set("severity", string(b.Severity), 0)
	m.Set("rung", int64(b.Rung), 0)
	m.Set("run_point", string(b.RunPoint), 0)
	setText(m, "remedy", b.Remedy)
	if b.Pair != 0 {
		m.Set("pair", int64(b.Pair), 0)
	}
	setText(m, "pair_path", b.PairPath)
	setText(m, "tightens", string(b.Tightens))
	setText(m, "checker", b.Checker)
	return m
}

// plain is a binding's free-form value as the writer takes it: a map becomes
// an ordered one with its keys sorted, so the same value gives the same bytes.
func plain(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		m := ordered.NewMap(0)
		for _, k := range keys {
			m.Set(k, plain(x[k]), 0)
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plain(e)
		}
		return out
	}
	return v
}

// splitID is a rule id's part and concern: the first two words of
// part.concern.n.
func splitID(id model.RuleID) (part, concern string, ok bool) {
	s := string(id)
	first := -1
	for i := 0; i < len(s); i++ {
		if s[i] != '.' {
			continue
		}
		if first < 0 {
			first = i
			continue
		}
		return s[:first], s[first+1 : i], true
	}
	return s, "", false
}

func reasoningRow(r *model.Reasoning) *ordered.Map {
	m := ordered.NewMap(0)
	setText(m, "definition", r.Definition)
	setList(m, "pros", r.Pros)
	setList(m, "cons", r.Cons)
	setText(m, "decision", r.Decision)
	return m
}

func relationsRow(r *model.Relations) *ordered.Map {
	m := ordered.NewMap(0)
	setList(m, "relates", r.Relates)
	setList(m, "conflicts", r.Conflicts)
	setList(m, "depends", r.Depends)
	setList(m, "covers", r.Covers)
	return m
}
