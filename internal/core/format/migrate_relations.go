package format

import (
	"strconv"

	"github.com/birdtable/gragas/internal/ordered"
)

// Relation is one word an old relation's how could hold, the relation of
// format 3 it goes to, and how many relations of the old rules held it.
type Relation struct {
	How       string
	To        string
	Relations int
}

// relationFolds are the old how words in the order the plan prints them, each
// with the relation of format 3 it goes to. A conflict is written as
// conflicts. The format also has relations.depends, and the reader takes it,
// but depends still folds to relates: the old format goes at the switch and
// main holds no depends, so none exists to carry. Every other word
// goes to relates and is parked, since relations cannot be edited by hand
// after the conversion (S59) and the parked file is where the word stays.
func relationFolds() []Relation {
	return []Relation{
		{How: "see", To: "relates"},
		{How: "refines", To: "relates"},
		{How: "narrows", To: "relates"},
		{How: "depends", To: "relates"},
		{How: "conflicts", To: "conflicts"},
	}
}

// relations is the relations of an old rule at format 3. A bare id relates;
// an entry's how decides between relates and conflicts and is parked unless it
// is conflicts, which the relation holds whole. The entry's wins, and any key
// besides id and how, is parked under relates and the id: at format 3 a
// winner is a decision that cites the binding, not a field of the rule. A
// word the old format did not have is refused, as an old status is.
func (g *migrator) relations(r *ordered.Map, id string) *ordered.Map {
	var relates, conflicts []string
	seen := map[string]bool{}
	for i, v := range r.List("relates") {
		switch x := v.(type) {
		case string:
			relates = append(relates, x)
		case *ordered.Map:
			to, conflict, ok := g.relationEntry(id, i, x, seen)
			switch {
			case !ok:
			case conflict:
				conflicts = append(conflicts, to)
			default:
				relates = append(relates, to)
			}
		}
	}
	if len(relates) == 0 && len(conflicts) == 0 {
		return nil
	}
	rm := newDoc()
	putIf(rm, "relates", anyStrings(relates))
	putIf(rm, "conflicts", anyStrings(conflicts))
	return rm
}

// relationEntry reads entry x, the i-th (from 0) of rule id's relates: the id
// it relates to, and whether its how folds to conflicts. It parks the how of
// an entry going to relates, counts a wins and parks it with every other key
// under relates and the id. An entry with no id is parked whole by its
// position, and ok is false.
func (g *migrator) relationEntry(id string, i int, x *ordered.Map, seen map[string]bool) (to string, conflict, ok bool) {
	to = x.Str("id")
	if to == "" {
		g.unread(id, []string{"relates", strconv.Itoa(i + 1)}, x)
		return "", false, false
	}
	key := to
	if seen[to] {
		key = to + " " + strconv.Itoa(i+1) // the same id twice keeps both
	}
	seen[to] = true
	at := []string{"relates", key}
	how := x.Str("how")
	if how == "" {
		how = "see" // the old format's word when none is given
	}
	conflict = g.relation(id, how) == "conflicts"
	if _, given := x.Get("how"); given && !conflict {
		g.park(id, at, "how", how)
	}
	if _, given := x.Get("wins"); given {
		g.m.Wins++
	}
	g.unread(id, at, x, "id", "how")
	return to, conflict, true
}

// relation counts one relation of rule id under its how word and is the
// relation of format 3 it goes to; a word with none is refused.
func (g *migrator) relation(id, how string) string {
	for i, f := range g.m.Relations {
		if f.How == how {
			g.m.Relations[i].Relations++
			return f.To
		}
	}
	g.refuse(id, "relates how", how)
	return ""
}
