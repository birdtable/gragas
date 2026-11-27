package format

import (
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
)

// MigrateEntry is the history entry of a migration: the entity names the
// rulebook at its starting version, as init's does (S68), and the entry holds
// the first owner and the digest of each statement the conversion wrote, by
// rule id. A rulebook with no rule records an empty map, never
// none, so the entry says the conversion looked and found no statement.
func MigrateEntry(f model.InitForm, statements map[model.RuleID]model.Digest) Entry {
	// A copy, so the entry never shares the conversion's map; the core layer
	// imports no maps (layers.yaml), so it is copied key by key.
	held := make(map[model.RuleID]model.Digest, len(statements))
	for id := range statements {
		held[id] = statements[id]
	}
	return Entry{Kind: "migrate", Entity: "rulebook:" + f.Name + "@" + f.Version, After: model.Lifecycle{Owner: f.Owners[0]}, Statements: held}
}

// StatementDigest is the digest a conversion records for a statement and the
// one the prose lint compares it with: Hex of internal/digest over the text as
// the rule holds it, in the prefixed form the history stores every digest in.
func StatementDigest(text string) model.Digest {
	return model.Digest(digest.Prefix + digest.Hex([]byte(text)))
}
