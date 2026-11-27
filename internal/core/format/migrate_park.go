package format

import (
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// ParkedFile is the side file migrate writes in the rulebook's folder: every
// value of the old layout with no field at format 3, so that nothing the old
// files held is lost. The format's loader reads no file of this
// name, and nothing else does.
const ParkedFile = "parked.yaml"

// parkedHead is the first line of the parked file: what it is, and that no
// program reads it.
const parkedHead = "# The values of the old rule files with no field at format 3, by rule id (or part, kind, layer, source or file), then field, in the old files' order. No program reads this file."

// Parked is the side file of a migration: its path in the rulebook's folder
// and how many lines it holds.
type Parked struct {
	Path  string
	Lines int
}

// Strength is one strength of format 3, the old words that map to it, and how
// many rules the migration gave it.
type Strength struct {
	From  []string
	To    string
	Rules int
}

// strengthFolds are the old strength words by the strength each maps to, in
// the order the plan prints them: a rule that binds (Hard) is must, one that
// is answered, recorded in a session or by a record is should, and a lens or
// a review reading is may.
func strengthFolds() []Strength {
	return []Strength{
		{From: []string{"Hard"}, To: "must"},
		{From: []string{"Answered", "Session", "Record"}, To: "should"},
		{From: []string{"Lens", "Review"}, To: "may"},
	}
}

// strengths maps each old strength word to must, should or may.
func strengths() map[string]string {
	out := map[string]string{}
	for _, f := range strengthFolds() {
		for _, w := range f.From {
			out[w] = f.To
		}
	}
	return out
}

// strength maps the old strength word of rule id, counts the rule under the
// strength it maps to, and parks the old word where more than one old word
// maps to that strength, since nothing else records which it was.
func (g *migrator) strength(id, word string) string {
	to := g.mapped(id, "strength", word, strengths())
	for i, f := range g.m.Strengths {
		if f.To != to {
			continue
		}
		g.m.Strengths[i].Rules++
		if len(f.From) > 1 {
			g.park(id, nil, "strength", word)
		}
	}
	return to
}

// park keeps v as field of subject, below the keys of at: a rule id or a
// "part", "kind", "layer" or "source" with its slug, or a file, then the
// field's path in the old file.
func (g *migrator) park(subject string, at []string, field string, v any) {
	m := g.parked.Sub(subject)
	if m == nil {
		m = newDoc()
		put(g.parked, subject, m)
	}
	for _, k := range at {
		next := m.Sub(k)
		if next == nil {
			next = newDoc()
			put(m, k, next)
		}
		m = next
	}
	put(m, field, v)
}

// reorder puts the fields parked for subject in the order of keys, the order
// of its old file, since a field is parked where the migration reads it.
func (g *migrator) reorder(subject string, keys []string) {
	m := g.parked.Sub(subject)
	if m == nil {
		return
	}
	out := newDoc()
	for _, k := range keys {
		if v, ok := m.Get(k); ok {
			put(out, k, v)
		}
	}
	put(g.parked, subject, out)
}

// unread parks every key of m that the migration does not read, under subject
// and at: a v2 field has a home at format 3 or in the parked file, never
// dropped without a word. read is the keys the migration reads from m.
func (g *migrator) unread(subject string, at []string, m *ordered.Map, read ...string) {
	if m == nil {
		return
	}
	for _, k := range m.Keys() {
		if !slices.Contains(read, k) {
			g.park(subject, at, k, m.Val(k))
		}
	}
}

// unparked lists each key of m that the migration neither reads nor parks:
// the remainder that stays out of the parked file, which is the top level of
// the parts and products files and a kind's instances. read is the keys the
// migration reads or parks from m.
func (g *migrator) unparked(item string, m *ordered.Map, read ...string) {
	for _, k := range m.Keys() {
		if !slices.Contains(read, k) {
			g.skip(item+" "+k, notParked(k))
		}
	}
}

// notParked is the reason a field of the remainder is listed.
func notParked(field string) string {
	return "the field " + field + " has no home at format 3 and is not parked"
}

// parkedText is the parked file: its first line, then every value parked.
func parkedText(parked *ordered.Map) string {
	return parkedHead + "\n" + ordered.YAML(parked)
}

// parkedTarget is the parked file as a target, and its size for the plan.
func (g *migrator) parkedTarget() {
	text := parkedText(g.parked)
	g.m.Parked = Parked{Path: ParkedFile, Lines: strings.Count(text, "\n")}
	g.m.Targets = append(g.m.Targets, model.Target{Path: ParkedFile, Data: []byte(text)})
}
