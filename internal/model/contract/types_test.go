package contract_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/contract"
)

// The generated Go types of the contract are held to the hand-authored
// documents the schemas and the zod side are held to: the same files under
// internal/testdata/contract-v1, so the Go types, the zod schemas and the
// schemas accept the same documents (the TypeScript side is in
// explorer/packages/contract/test/conformance.test.ts). Each type is read and
// written by its generated FromCanon and ToCanon.

const contractExamples = "../../testdata/contract-v1"

// codec is a document's generated type, by pointer.
type codec interface {
	FromCanon(c canon.Value) error
	ToCanon() canon.Value
}

// contractKinds are the emitted documents and a fresh value of each one's
// generated type.
func contractKinds() map[string]func() codec {
	return map[string]func() codec{
		"bundle":    func() codec { return new(contract.BundleDocument) },
		"report":    func() codec { return new(contract.ReportDocument) },
		"changelog": func() codec { return new(contract.ChangelogDocument) },
		"sarif":     func() codec { return new(contract.SarifDocument) },
		"plan":      func() codec { return new(contract.PlanDocument) },
		"proposal":  func() codec { return new(contract.ProposalDocument) },
	}
}

func contractDocs(t *testing.T, kind, which string) canon.Array {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(contractExamples, kind+"."+which+".json"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := canon.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	docs, ok := v.(canon.Array)
	if !ok || len(docs) == 0 {
		t.Fatalf("%s.%s.json is not a list of documents", kind, which)
	}
	return docs
}

// held says whether every key and value of want is in got, to any depth.
func held(want, got canon.Value) bool {
	switch w := want.(type) {
	case canon.Object:
		g, ok := got.(canon.Object)
		if !ok {
			return false
		}
		for k, v := range w {
			if _, there := g[k]; !there && isZero(v) {
				continue // an explicit zero that is the default is written as absent
			}
			if !held(v, g[k]) {
				return false
			}
		}
		return true
	case canon.Array:
		g, ok := got.(canon.Array)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !held(w[i], g[i]) {
				return false
			}
		}
		return true
	}
	a, errA := canon.Encode(want, canon.Canonical)
	b, errB := canon.Encode(got, canon.Canonical)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

// isZero says whether v is a JSON zero: null, empty, false, 0, [] or {}.
func isZero(v canon.Value) bool {
	switch x := v.(type) {
	case canon.Null:
		return true
	case canon.String:
		return x == ""
	case canon.Bool:
		return !bool(x)
	case canon.Int:
		return x == 0
	case canon.Float:
		return x == 0
	case canon.Array:
		return len(x) == 0
	case canon.Object:
		return len(x) == 0
	}
	return false
}

// Each hand-authored document reads into its generated type with no field
// left over and writes back holding everything it had.
func TestEachHandAuthoredDocumentRoundTripsThroughItsGeneratedType(t *testing.T) {
	t.Parallel()
	for kind, fresh := range contractKinds() {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			for _, doc := range contractDocs(t, kind, "valid") {
				v := fresh()
				if err := v.FromCanon(doc); err != nil {
					t.Fatalf("the generated type does not take the document: %v", err)
				}
				if got := v.ToCanon(); !held(doc, got) {
					out, _ := canon.Encode(got, canon.Canonical)
					in, _ := canon.Encode(doc, canon.Canonical)
					t.Errorf("the type lost something of the document:\n have %s\n want %s", out, in)
				}
			}
		})
	}
}

// ToCanon writes what encoding/json wrote for the same value, member for
// member: the omitempty fields left out, a required nil list as null, and a
// string's bytes that are not UTF-8 as U+FFFD. Each kind's zero value and
// each hand-authored document is written both ways.
func TestToCanonWritesWhatEncodingJSONWrote(t *testing.T) {
	t.Parallel()
	for kind, fresh := range contractKinds() {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			values := []codec{fresh()}
			for _, doc := range contractDocs(t, kind, "valid") {
				v := fresh()
				if err := v.FromCanon(doc); err != nil {
					t.Fatal(err)
				}
				values = append(values, v)
			}
			for i, v := range values {
				if got, want := encoded(t, v.ToCanon()), viaJSON(t, v); got != want {
					t.Errorf("value %d: ToCanon writes\n %s\nand encoding/json wrote\n %s", i, got, want)
				}
			}
		})
	}
	bad := contract.Finding{Kind: "a\xffb", Entity: "\xed\xa0\x80", Message: "ok �"}
	if got, want := encoded(t, bad.ToCanon()), viaJSON(t, bad); got != want || !strings.Contains(got, "a�b") {
		t.Errorf("a string that is not UTF-8: ToCanon writes %s, encoding/json wrote %s", got, want)
	}
}

func encoded(t *testing.T, v canon.Value) string {
	t.Helper()
	b, err := canon.Encode(v, canon.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// viaJSON is v as the contract context encoded it before FromCanon and
// ToCanon: json.Marshal, then canon's decode and encode.
func viaJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := canon.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return encoded(t, tree)
}

// A field the schema does not have is refused by the generated type as it is
// by the schema: the planted unknown field of each kind. The control is the
// valid document itself.
func TestAFieldTheSchemaLacksIsRefusedByTheGeneratedType(t *testing.T) {
	t.Parallel()
	for kind, fresh := range contractKinds() {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			doc, ok := contractDocs(t, kind, "valid")[0].(canon.Object)
			if !ok {
				t.Fatal("the first valid document is not an object")
			}
			planted := canon.Object{"planted_field": canon.Int(1)}
			maps.Copy(planted, doc)
			if err := fresh().FromCanon(planted); err == nil || !strings.Contains(err.Error(), `the document has no field "planted_field"`) {
				t.Errorf("the generated type took a field the schema lacks: %v", err)
			}
			if err := fresh().FromCanon(doc); err != nil {
				t.Errorf("the control: %v", err)
			}
		})
	}
}

func decode(t *testing.T, doc string) canon.Value {
	t.Helper()
	v, err := canon.Decode([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// FromCanon reads a key by its exact name, where encoding/json matched one
// without regard to case, and refuses a member of another JSON type, naming
// where by its JSON Pointer. A whole number reads into a number field, and
// null reads as the field's zero value, as they did.
func TestFromCanonReadsByExactNameAndType(t *testing.T) {
	t.Parallel()
	refused := []struct{ doc, want string }{
		{`{"Kind":"a"}`, `the document has no field "Kind"`},
		{`{"kind":"a","Refusal":false}`, `the document has no field "Refusal"`},
		{`{"kind":7}`, `/kind: want a string, found a number`},
		{`{"refusal":"yes"}`, `/refusal: want a boolean, found a string`},
		{`[]`, `the document: want an object, found an array`},
	}
	for _, c := range refused {
		var f contract.Finding
		if err := f.FromCanon(decode(t, c.doc)); err == nil || err.Error() != c.want {
			t.Errorf("Finding.FromCanon(%s) = %v, want %q", c.doc, err, c.want)
		}
	}
	var w contract.Waiver
	if err := w.FromCanon(decode(t, `{"age_days":1.5}`)); err == nil || err.Error() != "/age_days: want a whole number, found a number" {
		t.Errorf("Waiver.FromCanon of a fraction into a whole number = %v", err)
	}
	if err := w.FromCanon(nil); err == nil || err.Error() != "the document: want an object, found no value" {
		t.Errorf("Waiver.FromCanon(nil) = %v", err)
	}
	var d contract.BundleDocument
	if err := d.FromCanon(decode(t, `{"bindings":[{"rule":"r","options":{"a":[1]},"Value":true}]}`)); err == nil || err.Error() != `/bindings/0 has no field "Value"` {
		t.Errorf("a nested key that differs only in case = %v", err)
	}
	if err := d.FromCanon(decode(t, `{"identity":null,"rules":null,"bindings":[{"rule":"r","value":null,"options":{"a":[1]}}]}`)); err != nil {
		t.Fatalf("the control: %v", err)
	}
	if d.Rules != nil || d.Bindings[0].Value != nil || d.Bindings[0].Options == nil {
		t.Errorf("null read as %+v, want each field's zero value and the options read", d)
	}
}

// What FromCanon reads shares no array or object with the value it read, so
// a caller that changes the value afterwards changes nothing in the model.
func TestFromCanonSharesNothingWithItsValue(t *testing.T) {
	t.Parallel()
	doc := decode(t, `{"bindings":[{"rule":"r","value":[1],"options":{"a":[1]}}]}`)
	var d contract.BundleDocument
	if err := d.FromCanon(doc); err != nil {
		t.Fatal(err)
	}
	b := doc.(canon.Object)["bindings"].(canon.Array)[0].(canon.Object)
	b["value"].(canon.Array)[0] = canon.Int(2)
	b["options"].(canon.Object)["a"] = canon.Int(2)
	if got := encoded(t, d.Bindings[0].ToCanon()); !strings.Contains(got, `"options":{"a":[1]}`) || !strings.Contains(got, `"value":[1]`) {
		t.Errorf("the model changed with its value: %s", got)
	}
}

// An optional field with a default that is not the zero value reads as absent
// when it is absent, and as itself when it is the zero value.
func TestAnAbsentFieldIsNotAZeroOne(t *testing.T) {
	t.Parallel()
	var absent, explicit contract.Finding
	if err := absent.FromCanon(decode(t, `{"kind":"a","entity":"e","message":"m"}`)); err != nil {
		t.Fatal(err)
	}
	if err := explicit.FromCanon(decode(t, `{"kind":"a","entity":"e","message":"m","refusal":true,"notice":false}`)); err != nil {
		t.Fatal(err)
	}
	if absent.Refusal || explicit.Notice || !explicit.Refusal {
		t.Errorf("absent %+v, explicit %+v", absent, explicit)
	}
}
