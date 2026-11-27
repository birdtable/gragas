package format

import "github.com/birdtable/gragas/internal/model"

// recorded is the digest of each statement a conversion recorded, by rule id:
// the ones the load was given, or, given none, the ones the migrate entries of
// the history it read hold, a later entry's digest for a rule in place of an
// earlier one's. No history, or no migrate entry, records none.
func (r reader) recorded() map[model.RuleID]model.Digest {
	if r.opts.Statements != nil {
		return r.opts.Statements
	}
	return Recorded(r.out.Book.History)
}

// Recorded is the digest of each statement the migrate entries of history
// record, by rule id, in the order of its lines: a later entry's digest for a
// rule stands in place of an earlier one's.
func Recorded(history []model.HistoryEntry) map[model.RuleID]model.Digest {
	out := map[model.RuleID]model.Digest{}
	for _, h := range history {
		if h.Kind != "migrate" {
			continue
		}
		for id := range h.Statements { // the core layer imports no maps (layers.yaml)
			out[id] = h.Statements[id]
		}
	}
	return out
}

// held is whether the prose lint holds the statement p: its rule came from a
// migration, and the statement still has the digest the conversion recorded
// for it. A reworded statement, by a plan or by hand, has another digest and
// is linted like every other; the hold is never given on nothing recorded.
func held(p proseItem, recorded map[model.RuleID]model.Digest) bool {
	if p.origin != model.Migrated || p.rule == "" {
		return false
	}
	d, ok := recorded[p.rule]
	return ok && d != "" && d == StatementDigest(p.text)
}
