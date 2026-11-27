package format

import (
	"maps"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestMigrateRecordsADigestForEveryStatement: the migrate entry holds the
// digest of each statement the conversion wrote, by rule id, the line passes
// the history schema, and the loader reads the digests back from it.
// Risk: a migrated rule the prose lint cannot tell from a reworded one.
func TestMigrateRecordsADigestForEveryStatement(t *testing.T) {
	t.Parallel()
	m, err := MigrateV2(v2Fixture(), migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	want := map[model.RuleID]model.Digest{
		"arch.layers.1": StatementDigest("Packages form no cycle."),
		"arch.layers.2": StatementDigest("A reader reads the record."),
	}
	if !maps.Equal(m.Statements, want) {
		t.Fatalf("Statements = %v, want %v", m.Statements, want)
	}
	if d := string(want["arch.layers.1"]); !strings.HasPrefix(d, "sha256:") || len(d) != len("sha256:")+64 {
		t.Errorf("digest %q is not in the history's digest form", d)
	}
	got, err := recordWorld(t, "", MigrateEntry(migrateForm(), m.Statements))(model.Yes{Who: "team-a", At: at(1, 9), Plan: planA})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got.Data), `"statements":{"arch.layers.1":"`+string(want["arch.layers.1"])+`","arch.layers.2":"`+string(want["arch.layers.2"])+`"}`) {
		t.Errorf("the line does not hold both digests in key order:\n%s", got.Data)
	}
	files := fixtureBook()
	files[HistoryFile] = string(got.Data) + "\n"
	w, root := world(t, files)
	l, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Findings) > 0 {
		t.Fatalf("findings on the recorded line: %v", l.Findings)
	}
	if len(l.Book.History) != 1 || !maps.Equal(l.Book.History[0].Statements, want) {
		t.Errorf("history read back = %+v, want the two digests", l.Book.History)
	}
}

// TestMigrateWithNoRuleRecordsNoDigestAndSaysSo: a rulebook with no rule
// records an empty map, never none, so the entry says the conversion looked;
// any other entry writes no field. Risk: a migrate entry a reader cannot tell
// from an entry of another kind.
func TestMigrateWithNoRuleRecordsNoDigestAndSaysSo(t *testing.T) {
	t.Parallel()
	v := v2Fixture()
	delete(v, "base/arch/rules/layers.yaml")
	m, err := MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	if m.Statements == nil || len(m.Statements) != 0 {
		t.Fatalf("Statements = %#v, want an empty map", m.Statements)
	}
	got, err := recordWorld(t, "", MigrateEntry(migrateForm(), m.Statements))(model.Yes{Who: "team-a", At: at(1, 9), Plan: planA})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got.Data), `"statements":{}`) {
		t.Errorf("the line does not say it recorded no statement:\n%s", got.Data)
	}
	other, err := recordWorld(t, "", initEntry())(model.Yes{Who: "team-a", At: at(1, 9), Plan: planA})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(other.Data), "statements") {
		t.Errorf("an init line holds statements:\n%s", other.Data)
	}
}
