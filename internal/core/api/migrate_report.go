package coreapi

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
)

// MigrateSchema is the schema number of the Migration document migrate
// answers with under --json.
const MigrateSchema = 1

// Carried is how many of each kind of thing moved to the format.
type Carried struct {
	Rules    int `json:"rules"`
	Bindings int `json:"bindings"`
	Sources  int `json:"sources"`
	Guides   int `json:"guides"`
	Kinds    int `json:"kinds"`
}

// NotCarried is one thing of the old layout with no home in the format, and why.
type NotCarried struct {
	Item   string `json:"item"`
	Reason string `json:"reason"`
}

// Parked is the side file migrate writes: its path from the root, and how many
// lines it holds.
type Parked struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}

// Strength is one strength of the format, the old words that map to it, and
// how many rules migrate gave it.
type Strength struct {
	From  []string `json:"from"`
	To    string   `json:"to"`
	Rules int      `json:"rules"`
}

// Relation is one word of an old relation's how, the relation of the format it
// went to, and how many relations held it.
type Relation struct {
	How       string `json:"how"`
	To        string `json:"to"`
	Relations int    `json:"relations"`
}

// Migration is what migrate prints besides its plan, so that a script reads
// it too: the carry-over count per kind, each facet it turned into a kind, the
// side file every value with no field in the format is parked in, every item
// in no file at all, with the reason (S62), and the strength mapping.
type Migration struct {
	Schema     int          `json:"schema"`
	Carried    Carried      `json:"carried"`
	Converted  []string     `json:"converted,omitempty"`
	Parked     Parked       `json:"parked"`
	NotCarried []NotCarried `json:"not_carried,omitempty"`
	Strengths  []Strength   `json:"strengths"`
	// Relations is each old how word met, with the relation it went to; Wins
	// is how many relations named a winner, each parked.
	Relations []Relation `json:"relations"`
	Wins      int        `json:"wins"`
	// Prose is how many findings of the prose lint validate reports on the
	// result: the rewording left to the owner (S59).
	Prose int `json:"prose_findings"`
	// Statements is how many statement digests the history entry records,
	// one per rule carried: the prose lint holds each such rule until its
	// statement changes.
	Statements int `json:"statements"`
}

func migrationOf(m format.Migration, prose int) Migration {
	c := m.Carried
	out := Migration{Schema: MigrateSchema, Converted: m.Converted, Prose: prose, Statements: len(m.Statements),
		Carried: Carried{Rules: c.Rules, Bindings: c.Bindings, Sources: c.Sources, Guides: c.Guides, Kinds: c.Kinds},
		Parked:  Parked{Path: m.Parked.Path, Lines: m.Parked.Lines}, Strengths: []Strength{}, Relations: []Relation{}, Wins: m.Wins}
	for _, n := range m.NotCarried {
		out.NotCarried = append(out.NotCarried, NotCarried{Item: n.Item, Reason: n.Reason})
	}
	for _, s := range m.Strengths {
		out.Strengths = append(out.Strengths, Strength{From: s.From, To: s.To, Rules: s.Rules})
	}
	for _, r := range m.Relations {
		if r.Relations > 0 {
			out.Relations = append(out.Relations, Relation{How: r.How, To: r.To, Relations: r.Relations})
		}
	}
	return out
}

// MigrateLines are the carry-over report as lines a person reads: the count
// per kind, then each facet turned into a kind, then the strength mapping,
// then where the old relations went, then the parked file, then each item in
// no file at all.
func MigrateLines(m Migration) []string {
	c := m.Carried
	out := []string{
		fmt.Sprintf("carried: %s, %s, %s, %s, %s", model.Plural(c.Rules, "rule"), model.Plural(c.Bindings, "binding"),
			model.Plural(c.Sources, "source"), model.Plural(c.Guides, "guide"), model.Plural(c.Kinds, "kind")),
	}
	for _, f := range m.Converted {
		out = append(out, "facet "+f+" became a kind")
	}
	out = append(out, strengthsLine(m.Strengths))
	if len(m.Relations) > 0 {
		out = append(out, relationsLine(m.Relations, m.Wins))
	}
	out = append(out, "parked: "+model.Plural(m.Parked.Lines, "line")+" in "+m.Parked.Path+", each value with no field in the format, by rule id, then field; no program reads it")
	for _, n := range m.NotCarried {
		out = append(out, "not carried: "+n.Item+": "+n.Reason)
	}
	return append(out, statementsLine(m.Statements), model.Plural(m.Prose, "prose finding")+" for validate to list on the result, to reword by hand")
}

// strengthsLine is the strength mapping with the count of rules of each, and
// where a word folded with others is kept.
func strengthsLine(s []Strength) string {
	parts := make([]string, 0, len(s))
	for _, f := range s {
		parts = append(parts, andList(f.From)+" to "+f.To+", "+model.Plural(f.Rules, "rule"))
	}
	return "strengths: " + strings.Join(parts, "; ") + "; the parked file keeps each word folded with another"
}

// relationsLine is the relation each old how word went to, with the count of
// each, and what the parked file keeps of them.
func relationsLine(r []Relation, wins int) string {
	parts := make([]string, 0, len(r))
	for _, f := range r {
		parts = append(parts, f.How+" to "+f.To+", "+model.Plural(f.Relations, "relation"))
	}
	kept := "; the parked file keeps each word that went to relates"
	if wins > 0 {
		kept += ", and the " + model.Plural(wins, "winner") + " named, since at format 3 a winner is a decision"
	}
	return "relations: " + strings.Join(parts, "; ") + kept
}

// andList is words as a person writes them: "a", "a and b", "a, b and c".
func andList(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// statementsLine says what the history entry records of the statements: a
// rulebook with no rule records an empty map, and the line says so.
func statementsLine(n int) string {
	if n == 0 {
		return "the history records no statement digest: the rulebook holds no rule"
	}
	return "the history records " + model.Plural(n, "statement digest") + ": the prose lint holds each migrated rule until its statement changes"
}
