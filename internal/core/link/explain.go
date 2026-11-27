package link

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// ExplainRule is what a rulebook says of the rule asked for, by its id or by an
// alias it had before it was renumbered (S62). shipped says where each binding
// went, as Generate lists them; held is the standing Derive gave. The second
// result is false for an id the rulebook holds under no name. It reads no file.
func ExplainRule(book model.Book, held []model.Held, shipped []Shipped, asked string) (model.Explained, bool) {
	for _, r := range book.Rules {
		if string(r.ID) != asked && !slices.Contains(r.Aliases, model.RuleID(asked)) {
			continue
		}
		out := model.Explained{Asked: asked, Rules: []model.ExplainedRule{explained(book.Identity, r, held, shipped)}}
		if string(r.ID) != asked {
			out.Alias = r.ID
		}
		return out, true
	}
	return model.Explained{}, false
}

// ExplainEntry is the rules that hold the entry of tool with key, in any
// target. No rule is Unmapped: no rule explains the entry.
func ExplainEntry(book model.Book, held []model.Held, shipped []Shipped, tool, key string) model.Explained {
	out := model.Explained{Asked: tool + " " + key, Tool: tool, Entry: key, Rules: []model.ExplainedRule{}}
	for _, r := range book.Rules {
		if slices.ContainsFunc(r.Bindings, func(b model.Binding) bool { return b.Tool == tool && b.Key == key }) {
			out.Rules = append(out.Rules, explained(book.Identity, r, held, shipped))
		}
	}
	out.Unmapped = len(out.Rules) == 0
	return out
}

func explained(id model.RulebookIdentity, r model.Rule, held []model.Held, shipped []Shipped) model.ExplainedRule {
	out := model.ExplainedRule{Rule: r.ID, Status: r.Status, Standing: model.Standing(r.Status), Marks: []model.Mark{}, Bindings: []model.ExplainedBinding{},
		Page: link(id.DocsURL, r.ID), Explorer: link(id.ExplorerURL, r.ID)}
	if i := slices.IndexFunc(held, func(h model.Held) bool { return h.Rule == r.ID }); i >= 0 {
		out.Standing, out.Marks = held[i].Standing, append(out.Marks, held[i].Marks...)
	}
	for _, b := range r.Bindings {
		e := model.ExplainedBinding{Tool: b.Tool, Target: b.Target, Key: b.Key, Value: b.Value, Severity: b.Severity, Rung: b.Rung, RunPoint: b.RunPoint, Proof: model.TruthNotRun}
		if b.Rung == model.MaxRung && b.Pair > 0 {
			e.Proof = model.TruthReviewed
		}
		ref := b.Ref(r.ID)
		if i := slices.IndexFunc(shipped, func(s Shipped) bool { return s.Rule == r.ID && s.Binding.Ref(r.ID) == ref }); i >= 0 {
			e.Preset = shipped[i].File
		}
		out.Bindings = append(out.Bindings, e)
	}
	return out
}

// link is the address of the rule's page below base, "" where the identity
// gives no base: an address is never guessed (S72).
func link(base string, id model.RuleID) string {
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", strings.TrimRight(base, "/"), id)
}
