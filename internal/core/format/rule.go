package format

import (
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// ruleOf reads a rule the rule schema accepted. A field the file does not
// carry is the zero value: a rule loads with whatever its status allows, and
// the checks that need more are lifecycle's.
func ruleOf(m *ordered.Map) model.Rule {
	r := model.Rule{
		ID:         model.RuleID(m.Str("id")),
		Aliases:    ruleIDs(m.List("aliases")),
		Statement:  m.Str("statement"),
		Strength:   model.Strength(m.Str("strength")),
		Status:     model.Status(m.Str("status")),
		Owner:      m.Str("owner"),
		ReviewBy:   m.Str("review_by"),
		ZeroBy:     m.Str("zero_by"),
		Origin:     model.Origin(m.Str("origin")),
		ReplacedBy: model.RuleID(m.Str("replaced_by")),
	}
	if s := m.Sub("scope"); s != nil {
		r.Scope = &model.RuleScope{Parts: ordered.Strings(s.List("parts")), Paths: ordered.Strings(s.List("paths")), Kinds: ordered.Strings(s.List("kinds"))}
	}
	for _, e := range ordered.Maps(m.List("exemptions")) {
		r.Exemptions = append(r.Exemptions, model.Exemption{Paths: ordered.Strings(e.List("paths")), Kinds: ordered.Strings(e.List("kinds")), Reason: e.Str("reason")})
	}
	if g := m.Sub("reasoning"); g != nil {
		r.Reasoning = &model.Reasoning{Definition: g.Str("definition"), Pros: ordered.Strings(g.List("pros")), Cons: ordered.Strings(g.List("cons")), Decision: g.Str("decision")}
	}
	for _, p := range ordered.Maps(m.List("pairs")) {
		r.Pairs = append(r.Pairs, model.Pair{Number: intOf(p.Val("number")), Target: model.TargetType(p.Str("target"))})
	}
	if x := m.Sub("relations"); x != nil {
		r.Relations = &model.Relations{Relates: ruleIDs(x.List("relates")), Conflicts: ruleIDs(x.List("conflicts")), Depends: ruleIDs(x.List("depends")), Covers: ruleIDs(x.List("covers"))}
	}
	for _, c := range ordered.Maps(m.List("sources")) {
		r.Sources = append(r.Sources, model.Citation{Ref: c.Str("ref"), Where: c.Str("where"), Says: c.Str("says"), Backs: model.Backing(c.Str("backs"))})
	}
	for _, b := range ordered.Maps(m.List("bindings")) {
		r.Bindings = append(r.Bindings, bindingOf(b))
	}
	return r
}

// bindingOf reads a binding as given: its value and options are kept as the
// file wrote them and not interpreted, since link owns what they mean.
func bindingOf(b *ordered.Map) model.Binding {
	out := model.Binding{
		Tool: b.Str("tool"), Target: model.TargetType(b.Str("target")), Key: b.Str("key"),
		Value:    ordered.Plain(b.Val("value")),
		Severity: model.Severity(b.Str("severity")), Rung: model.Rung(intOf(b.Val("rung"))),
		RunPoint: model.RunPoint(b.Str("run_point")), Remedy: b.Str("remedy"),
		Pair: intOf(b.Val("pair")), PairPath: b.Str("pair_path"), Tightens: model.RuleID(b.Str("tightens")),
		Checker: b.Str("checker"),
	}
	// a binding with no options has none: Sub answers a nil map then, which
	// Plain must not be handed
	if opts := b.Sub("options"); opts != nil {
		if o, ok := ordered.Plain(opts).(map[string]any); ok {
			out.Options = o
		}
	}
	return out
}

func ruleIDs(l []any) []model.RuleID {
	var out []model.RuleID
	for _, s := range ordered.Strings(l) {
		out = append(out, model.RuleID(s))
	}
	return out
}
