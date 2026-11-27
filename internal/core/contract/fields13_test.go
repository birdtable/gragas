package contract

import (
	"testing"
)

// TestTheFixtureBundleCarriesTheFieldsOfContract13: a reader of the fixture
// bundle, through ReadBundle and the schema, gets the fields contract 1.3 adds
// for the explorer's views: a rule's pros and cons and the text
// of its pair, the status a history step moved a rule to, the waiver syntax of
// an adapter used, and the rulebook's successor, which is the rulebook its
// rule map leads to.
//
// What it costs when this fails: a view that reads only the contract shows a
// dash where the spec says it shows the value, and the field list names a
// field no document carries.
func TestTheFixtureBundleCarriesTheFieldsOfContract13(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	got, err := ReadBundle(fixtureBytes(t, "bundle.json"), s)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range got.Rules {
		if r.ID != "go.errors.1" {
			continue
		}
		found = true
		if len(r.Pros) != 2 || len(r.Cons) != 1 {
			t.Errorf("go.errors.1 has %d pros and %d cons, want 2 and 1", len(r.Pros), len(r.Cons))
		}
		if len(r.Pairs) == 0 || r.Pairs[0].Good == nil || r.Pairs[0].Bad == nil || *r.Pairs[0].Good == *r.Pairs[0].Bad {
			t.Errorf("go.errors.1's first pair = %+v, want a good and a bad example that differ", r.Pairs)
		}
	}
	if !found {
		t.Fatal("the fixture bundle holds no go.errors.1")
	}
	if len(got.AdaptersUsed) == 0 || got.AdaptersUsed[0].WaiverSyntax == nil {
		t.Errorf("adapters_used = %+v, want the first to carry a waiver syntax", got.AdaptersUsed)
	}
	if len(got.RuleMap) == 0 || got.Identity.Successor == nil || string(*got.Identity.Successor) != string(got.RuleMap[0].Rulebook) {
		t.Errorf("identity.successor = %v, want the rulebook the rule map leads to", got.Identity.Successor)
	}
	n := 0
	for _, h := range got.History {
		if h.Status != nil {
			n++
		}
	}
	if n == 0 {
		t.Error("no history step carries a status")
	}
}
