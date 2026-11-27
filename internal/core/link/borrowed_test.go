package link_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// recordOf is a whole record of a binding of the rule id at tool t, target
// config and key, at the severity sev.
func recordOf(id, key, sev string) string {
	return "subject: " + id + "\ntool: t\ntarget: config\nkey: " + key + "\nvalue: 2\nseverity: " + sev + "\nrung: 1\nrun_point: commit\n"
}

// TestABorrowedRuleIsHeldToTheBorrowersRecords. Risk: a record that lands
// on another rule, or a lender's binding that a record of a new entry
// replaces.
func TestABorrowedRuleIsHeldToTheBorrowersRecords(t *testing.T) {
	t.Parallel()
	lent := model.Rule{ID: "a.b.1", Status: model.Active, Bindings: []model.Binding{bnd("k1", 1), bnd("k2", 1)}}
	read, refused := link.ReadRecords([]model.Opaque{
		{Path: "bindings/a-b-1.yaml", Data: []byte(recordOf("a.b.1", "k1", "error"))},
		{Path: "bindings/a-b-1-2.yaml", Data: []byte(recordOf("a.b.1", "k3", "warning"))},
		{Path: "bindings/a-b-2.yaml", Data: []byte(recordOf("a.b.2", "k2", "error"))},
	})
	if len(refused) > 0 {
		t.Fatal(refused)
	}
	got := link.Borrowed(lent, read)
	var keys []string
	for _, b := range got.Bindings {
		keys = append(keys, b.Key+"="+string(b.Severity))
	}
	if strings.Join(keys, " ") != "k1=error k2= k3=warning" {
		t.Errorf("held to %v, want k1 replaced by its record, k2 the lender's, k3 added", keys)
	}
	if len(lent.Bindings) != 2 || lent.Bindings[0].Severity != "" {
		t.Errorf("the lender's rule was changed: %+v", lent.Bindings)
	}
	if !reflect.DeepEqual(link.Borrowed(lent, nil), lent) {
		t.Errorf("a rule with no record is not the lender's")
	}
}

// TestResolveReadsAnOverlaysRecordsOfItsParentsRules. Risk: a record that
// tightens nothing in the listing, or one of a rule nobody lent that is
// listed as if it bound.
func TestResolveReadsAnOverlaysRecordsOfItsParentsRules(t *testing.T) {
	t.Parallel()
	parent := layer("base",
		model.Rule{ID: "a.b.1", Bindings: []model.Binding{bnd("k1", 1)}},
		model.Rule{ID: "a.b.2", Bindings: []model.Binding{bnd("k2", 1)}})
	book := layer("over").Book
	book.BindingRecords = []model.Opaque{
		{Path: "bindings/a-b-1.yaml", Data: []byte(recordOf("a.b.1", "k1", "error"))},
		{Path: "bindings/a-b-2.yaml", Data: []byte(recordOf("a.b.2", "k9", "error"))},
	}
	got, err := link.Resolve(book, []model.ChainLayer{parent})
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ rule, key, layer, how, tightens string }
	var rows []row
	for _, e := range got.Entries {
		rows = append(rows, row{string(e.Rule), e.Binding.Key, e.Layer, string(e.How), string(e.Binding.Tightens)})
	}
	want := []row{
		{"a.b.1", "k1", "over", "changed", "a.b.1"},
		{"a.b.2", "k2", "base", "set", ""},
		{"a.b.2", "k9", "over", "changed", "a.b.2"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows\n%v\nwant\n%v", rows, want)
	}

	// a record of a rule the overlay did not borrow, and one that cannot be
	// read, are errors naming the layer, and neither sets an entry
	book.BindingRecords = []model.Opaque{
		{Path: "bindings/x.yaml", Data: []byte(recordOf("z.z.1", "k1", "error"))},
		{Path: "bindings/y.yaml", Data: []byte("subject: a.b.1\n")},
	}
	got, err = link.Resolve(book, []model.ChainLayer{parent})
	if err == nil {
		t.Fatal("records nobody can hold were read")
	}
	for _, w := range []string{"layer over: its record bindings/x.yaml names rule z.z.1", "bindings/y.yaml"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("err = %v, want it to say %q", err, w)
		}
	}
	for _, e := range got.Entries {
		if e.Layer == "over" {
			t.Errorf("an entry was set by a record that was refused: %+v", e)
		}
	}
}

// TestAnImportersRecordTightensTheImportedRule: an imported rule is the
// layer's own book's, and its record tightens it in place.
func TestAnImportersRecordTightensTheImportedRule(t *testing.T) {
	t.Parallel()
	book := layer("app", model.Rule{ID: "a.b.1", Origin: model.Imported, Bindings: []model.Binding{bnd("k1", 1)}}).Book
	book.BindingRecords = []model.Opaque{{Path: "bindings/a-b-1.yaml", Data: []byte(recordOf("a.b.1", "k1", "error"))}}
	got, err := link.Resolve(book, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 || got.Entries[0].How != model.ResolvedChanged || got.Entries[0].Binding.Severity != model.SeverityError {
		t.Errorf("entries = %+v, want the imported entry changed to error", got.Entries)
	}
}
