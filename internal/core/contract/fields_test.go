package contract

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// referencePage is the field reference the generator writes from the schemas.
const referencePage = "../../../contract/field-reference.md"

// sectionRows are the rows of the table under "## name" in the page, as
// FieldRow: the Derivation column is the handbook's, so it is not read.
func sectionRows(t *testing.T, page, name string) []FieldRow {
	t.Helper()
	_, after, ok := strings.Cut(page, "\n## "+name+"\n")
	if !ok {
		t.Fatalf("the page has no section %q", name)
	}
	if next := strings.Index(after, "\n## "); next >= 0 {
		after = after[:next]
	}
	var rows []FieldRow
	for line := range strings.SplitSeq(after, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Field ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 5 {
			t.Fatalf("%s: a row of %d cells: %q", name, len(cells), line)
		}
		def := strings.Trim(cells[3], "`")
		rows = append(rows, FieldRow{Name: cells[0], Type: cells[1], Since: cells[2], Default: def})
	}
	return rows
}

// TestFieldsOfEachEntityIsItsSectionOfTheFieldReference: for every name
// Entities lists, the rows FieldsOf returns are the rows of that schema's table
// in contract/field-reference.md, the entity's section where a document has the
// same name. Risk: `contract fields` and the page that documents the fields
// drifting apart, one reading the schemas another way.
func TestFieldsOfEachEntityIsItsSectionOfTheFieldReference(t *testing.T) {
	t.Parallel()
	pageBytes, err := os.ReadFile(referencePage)
	if err != nil {
		t.Fatal(err)
	}
	page := string(pageBytes)
	_, schemas := world(t, schemaFiles(t))
	names, err := Entities(schemas.Files)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < len(Kinds()) {
		t.Fatalf("Entities lists %d names, fewer than the documents: %v", len(names), names)
	}
	for _, name := range names {
		got, err := FieldsOf(name, schemas.Files)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		want := sectionRows(t, page, got.Schema)
		if len(want) == 0 {
			t.Errorf("%s: the section %s holds no rows", name, got.Schema)
		}
		if !slices.Equal(got.Rows, want) {
			t.Errorf("%s: fields differ from the section %s of the page\ngot  %v\nwant %v", name, got.Schema, got.Rows, want)
		}
	}
}

// TestFieldsOfAnUnknownNameIsUnknownEntityListingTheKnownOnes: an empty name, a
// name that holds a separator or a dot, and a name that is neither an entity nor
// a document each end *UnknownEntity naming every name that is one, and reach no
// file. Risk: a name read as a path, or an unknown one told as a read failure.
func TestFieldsOfAnUnknownNameIsUnknownEntityListingTheKnownOnes(t *testing.T) {
	t.Parallel()
	_, schemas := world(t, schemaFiles(t))
	known, err := Entities(schemas.Files)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"", "a/b", "rule.json", "../rule", ".", "nothing"}, table(t, "backslash_names.txt")...) {
		_, err := FieldsOf(name, schemas.Files)
		var u *UnknownEntity
		if !errors.As(err, &u) {
			t.Errorf("%q: error %v, want *UnknownEntity", name, err)
			continue
		}
		if u.Name != name || !slices.Equal(u.Known, known) {
			t.Errorf("%q: names %q and lists %v, want the name and %v", name, u.Name, u.Known, known)
		}
	}
}

// TestAnEntityWinsOverADocumentOfTheSameName: where both exist, FieldsOf reads
// the entity's schema.
func TestAnEntityWinsOverADocumentOfTheSameName(t *testing.T) {
	t.Parallel()
	files := schemaFiles(t)
	_, schemas := world(t, files)
	names, err := Entities(schemas.Files)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		got, err := FieldsOf(name, schemas.Files)
		if err != nil {
			t.Fatal(err)
		}
		if _, hasEntity := files[SchemaDir+"/v1/entity-"+name+".schema.json"]; hasEntity && got.Schema != "entity-"+name {
			t.Errorf("%s read %s, want the entity", name, got.Schema)
		}
	}
}
