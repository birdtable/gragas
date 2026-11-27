package lifecycle

import (
	"cmp"
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// Attend lists what needs the maintainer in a book, as of today, a day as
// YYYY-MM-DD that the caller read from the clock: this function reads none, so
// the same book and day give the same list (arch.ports.1, S55). A retired rule
// asks for nothing. When every rule is proposed, each rule is listed, under
// the kinds it misses or as complete, so the whole book is read at once.
func Attend(b model.Book, today string) model.AttentionListing {
	out := model.AttentionListing{Today: today, Items: []model.AttentionItem{}, Findings: []model.Finding{}}
	var live []model.Rule
	for _, r := range b.Rules {
		if r.Status != model.Retired {
			live = append(live, r)
		}
	}
	out.AllProposed = len(live) > 0 && !slices.ContainsFunc(live, func(r model.Rule) bool { return r.Status != model.Proposed })
	for _, r := range live {
		items := attentionOf(b, r, today)
		if len(items) == 0 && out.AllProposed {
			items = []model.AttentionItem{{Rule: r.ID, Kind: model.AttentionComplete, Status: r.Status,
				Why: "it has a source, an example pair, a decision and an owner", Fix: "gragas rule promote " + string(r.ID)}}
		}
		out.Items = append(out.Items, items...)
	}
	order := model.AttentionKinds()
	slices.SortStableFunc(out.Items, func(a, c model.AttentionItem) int {
		return cmp.Or(cmp.Compare(slices.Index(order, a.Kind), slices.Index(order, c.Kind)), cmp.Compare(a.Rule, c.Rule))
	})
	return out
}

// attentionOf are the kinds one live rule is in.
func attentionOf(b model.Book, r model.Rule, today string) []model.AttentionItem {
	id := string(r.ID)
	var out []model.AttentionItem
	add := func(k model.AttentionKind, why, fix string) {
		out = append(out, model.AttentionItem{Rule: r.ID, Kind: k, Status: r.Status, Why: why, Fix: fix})
	}
	if r.Status == model.Active && validDate(r.ReviewBy) && r.ReviewBy < today {
		add(model.AttentionOverdue, "its review-by of "+r.ReviewBy+" is past", "gragas rule confirm "+id)
	}
	if len(r.Sources) == 0 {
		add(model.AttentionNoSource, "it cites no source", "gragas rule edit "+id+" --cite <source slug>")
	}
	if len(r.Pairs) == 0 && (r.Strength != model.May || len(r.Bindings) > 0) {
		add(model.AttentionNoPair, "it has no example pair", "add good.* and bad.* under pairs/"+id+"/1/ and list the pair in the rule")
	}
	if b.Identity.Governance.DecisionRequired && (r.Reasoning == nil || r.Reasoning.Decision == "") {
		add(model.AttentionNoDecision, "its reasoning names no decision, and the governance asks for one", "gragas rule promote "+id+" --decision-title \"...\" --decision-text \"...\", or name one in its reasoning")
	}
	if r.Owner == "" {
		add(model.AttentionNoOwner, "nobody answers for it", "gragas rule edit "+id+" --rule-owner <name>")
	}
	return out
}
