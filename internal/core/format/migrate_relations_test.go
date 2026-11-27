package format

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// relationsOf is the relations the rules files of m write for rule id.
func relationsOf(t *testing.T, m Migration, id string) *ordered.Map {
	t.Helper()
	for _, tg := range m.Targets {
		if !strings.Contains(tg.Path, "/rules/") {
			continue
		}
		v, err := ordered.ParseYAML(tg.Data)
		if err != nil {
			t.Fatalf("%s: %v", tg.Path, err)
		}
		for _, r := range ordered.Maps(v.(*ordered.Map).List("rules")) {
			if r.Str("id") == id {
				return r.Sub("relations")
			}
		}
	}
	return nil
}

// TestMigrateWritesAConflictAsTheRelationAndParksEveryOtherWordAndTheWinner:
// an old conflict is written as conflicts, and the other words go to relates
// with the word parked under relates and the id; the winner is parked beside
// it; the plan counts each word, and the winners. Planted: a word the old
// format did not have is
// refused. Risk: a declared conflict lost for good, since relations are not
// edited by hand after the conversion (S59).
func TestMigrateWritesAConflictAsTheRelationAndParksEveryOtherWordAndTheWinner(t *testing.T) {
	t.Parallel()
	m, err := MigrateV2(withRelations(), migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	rel := relationsOf(t, m, "arch.layers.1")
	if rel == nil || !slices.Equal(ordered.Strings(rel.List("relates")), []string{"arch.layers.2", "arch.gone.1"}) ||
		!slices.Equal(ordered.Strings(rel.List("conflicts")), []string{"arch.other.1"}) {
		t.Errorf("the relations are %s, want arch.layers.2 and arch.gone.1 relating and arch.other.1 in conflict", compact(rel))
	}
	parked := map[string]string{
		"arch.layers.2": `{"how":"see"}`,
		"arch.gone.1":   `{"how":"refines"}`,
		"arch.other.1":  `{"wins":"this"}`,
	}
	for to, want := range parked {
		if got := compact(parkedAt(t, m, "arch.layers.1", "relates", to)); got != want {
			t.Errorf("parked under relates/%s: %s, want %s", to, got, want)
		}
	}
	counts := map[string]int{}
	for _, r := range m.Relations {
		counts[r.How+" to "+r.To] = r.Relations
	}
	want := map[string]int{"see to relates": 1, "refines to relates": 1, "narrows to relates": 0, "depends to relates": 0, "conflicts to conflicts": 1}
	if len(counts) != len(want) || m.Wins != 1 {
		t.Errorf("Relations = %v, Wins = %d; want %v and 1", counts, m.Wins, want)
	}
	for k, n := range want {
		if counts[k] != n {
			t.Errorf("%s: %d relations, want %d", k, counts[k], n)
		}
	}
	v := withRelations()
	v["base/arch/rules/layers.yaml"] = []byte(strings.Replace(string(v["base/arch/rules/layers.yaml"]), "how: refines", "how: overrides", 1))
	if _, err := MigrateV2(v, migrateForm()); err == nil || !strings.Contains(err.Error(), `relates how "overrides" has no counterpart`) {
		t.Errorf("planted: an unknown word migrated, err %v", err)
	}
}

// oldRelations counts the relations of the old rule files of v by how word,
// as "how <word>", and the winners named, as "wins".
func oldRelations(t *testing.T, v V2) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, p := range sortedKeys(v) {
		if !strings.Contains(p, "/rules/") {
			continue
		}
		doc, err := ordered.ParseYAML(v[p])
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range ordered.Maps(doc.(*ordered.Map).List("rules")) {
			for _, e := range ordered.Maps(r.List("relates")) {
				out["how "+e.Str("how")]++
				if _, ok := e.Get("wins"); ok {
					out["wins"]++
				}
			}
		}
	}
	return out
}

// newRelations counts what m writes of them: the conflicts the rules files
// hold, as "how conflicts", and the words and winners the parked file holds
// under relates. A conflict parked is counted apart, since the ruling writes
// each one as the relation.
func newRelations(t *testing.T, m Migration) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tg := range m.Targets {
		if !strings.Contains(tg.Path, "/rules/") {
			continue
		}
		v, err := ordered.ParseYAML(tg.Data)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range ordered.Maps(v.(*ordered.Map).List("rules")) {
			if rel := r.Sub("relations"); rel != nil {
				out["how conflicts"] += len(rel.List("conflicts"))
			}
		}
	}
	parked := ordered.NewMap(0)
	for _, tg := range m.Targets {
		if tg.Path == ParkedFile {
			v, err := ordered.ParseYAML(tg.Data)
			if err != nil {
				t.Fatal(err)
			}
			parked = v.(*ordered.Map)
		}
	}
	for _, subject := range parked.Keys() {
		rs := parked.Sub(subject).Sub("relates")
		if rs == nil {
			continue
		}
		for _, to := range rs.Keys() {
			switch h := rs.Sub(to).Str("how"); h {
			case "":
			case "conflicts":
				out["parked how conflicts"]++ // a conflict sent to relates, not written
			default:
				out["how "+h]++
			}
			if rs.Sub(to).Str("wins") != "" {
				out["wins"]++
			}
		}
	}
	return out
}

// TestMigrateCountsTheOwnersRelationsByKind: of the owner's rulebook, each
// conflict is written as conflicts, and each see and refines word and each
// winner is parked, as many as the old files hold of each; a relation of any
// other kind fails, since the folds name these three (2 conflicts, 649 see,
// 7 refines and 2 winners when they were chosen). Risk: a kind of relation
// the folds never saw, moved by a fold nobody chose.
func TestMigrateCountsTheOwnersRelationsByKind(t *testing.T) {
	t.Parallel()
	v := ownersRulebook(t)
	old := oldRelations(t, v)
	for k := range old {
		if !slices.Contains([]string{"how see", "how refines", "how conflicts", "wins"}, k) {
			t.Errorf("the owner's rulebook holds %d relations of %q, a kind the ruling did not name", old[k], k)
		}
	}
	if old["how see"] == 0 || old["how refines"] == 0 || old["how conflicts"] == 0 || old["wins"] == 0 {
		t.Fatalf("the owner's rulebook holds %v, want each of the ruled kinds", old)
	}
	m, err := MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	if got := newRelations(t, m); !reflect.DeepEqual(got, old) {
		t.Errorf("written and parked %v, want the %v the old files hold", got, old)
	}
	for _, r := range m.Relations {
		if r.Relations != old["how "+r.How] {
			t.Errorf("the plan counts %d %s, want %d", r.Relations, r.How, old["how "+r.How])
		}
	}
	if m.Wins != old["wins"] {
		t.Errorf("the plan counts %d winners, want %d", m.Wins, old["wins"])
	}
}
