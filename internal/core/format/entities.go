package format

import (
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

func termOf(m *ordered.Map) model.Term {
	return model.Term{Slug: m.Str("slug"), Definition: m.Str("definition"), Literal: m.Str("literal"), Status: model.Status(m.Str("status"))}
}

func sourceOf(m *ordered.Map) model.Source {
	return model.Source{
		Slug: m.Str("slug"), Kind: model.SourceKind(m.Str("kind")), Title: m.Str("title"), Locator: m.Str("locator"),
		Attribution: m.Str("attribution"), Verified: m.Str("verified"),
	}
}

func decisionOf(m *ordered.Map) model.Decision {
	d := model.Decision{
		Slug: m.Str("slug"), Title: m.Str("title"), Text: m.Str("text"), Rejected: ordered.Strings(m.List("rejected")),
		Who: m.Str("who"), Date: m.Str("date"), Supersedes: m.Str("supersedes"), SupersededBy: m.Str("superseded_by"),
	}
	if c := m.Sub("cites"); c != nil {
		d.Cites = &model.Cites{Sources: ordered.Strings(c.List("sources")), Rules: ruleIDs(c.List("rules")), Bindings: ordered.Strings(c.List("bindings"))}
	}
	return d
}

func exclusionOf(m *ordered.Map) model.Exclusion {
	e := model.Exclusion{
		Overlay: m.Str("overlay"), Rule: model.RuleID(m.Str("rule")), Number: intOf(m.Val("number")),
		Decision: m.Str("decision"), Who: m.Str("who"), When: m.Str("when"),
	}
	if l := m.Sub("lifted"); l != nil {
		e.Lifted = &model.Lifted{Decision: l.Str("decision"), Who: l.Str("who"), When: l.Str("when")}
	}
	return e
}

// proposalOf reads a proposal; its document is kept opaque, as given, since
// its schema is the contract's.
func proposalOf(m *ordered.Map) model.Proposal {
	p := model.Proposal{
		Format: model.FormatVersion, Slug: m.Str("slug"), Target: m.Str("target"), State: model.ProposalState(m.Str("state")),
		Link: m.Str("link"), Opened: m.Str("opened"),
	}
	if d, ok := ordered.Plain(m.Sub("document")).(map[string]any); ok {
		p.Document = d
	}
	return p
}

func lifecycleOf(m *ordered.Map) model.Lifecycle {
	return model.Lifecycle{
		Status: model.Status(m.Str("status")), Owner: m.Str("owner"), ReviewBy: m.Str("review_by"),
		ZeroBy: m.Str("zero_by"), ReplacedBy: model.RuleID(m.Str("replaced_by")), Literal: m.Str("literal"),
	}
}

// historyOf reads one line of history.jsonl; ordering and the chain are the
// history's own checks.
func historyOf(m *ordered.Map) model.HistoryEntry {
	h := model.HistoryEntry{
		Format: model.FormatVersion, Number: intOf(m.Val("number")), Previous: model.Digest(m.Str("previous")),
		Kind: m.Str("kind"), Entity: m.Str("entity"), Who: m.Str("who"), When: m.Str("when"),
		Plan: model.Digest(m.Str("plan")), Decision: m.Str("decision"), Reverts: model.Digest(m.Str("reverts")), Command: m.Str("command"),
	}
	if b := m.Sub("before"); b != nil {
		l := lifecycleOf(b)
		h.Before = &l
	}
	if a := m.Sub("after"); a != nil {
		h.After = lifecycleOf(a)
	}
	if s := m.Sub("statements"); s != nil {
		h.Statements = make(map[model.RuleID]model.Digest, s.Len())
		for _, id := range s.Keys() {
			h.Statements[model.RuleID(id)] = model.Digest(s.Str(id))
		}
	}
	return h
}
