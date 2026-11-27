package jsonschema_test

import (
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/jsonschema"
)

// files is an Assets port over a map, for the schemas a test hands over.
type files map[string]string

func (f files) Read(p string) ([]byte, error) {
	if c, ok := f[p]; ok {
		return []byte(c), nil
	}
	return nil, fs.ErrNotExist
}

func (f files) List(dir string) ([]string, error) {
	var out []string
	for p := range f {
		if strings.HasPrefix(p, dir+"/") {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

const (
	common = `{"$id":"common.schema.json","$defs":{"id":{"type":"integer"}}}`
	doc    = `{"type":"object","required":["id"],"properties":{"id":{"$ref":"common.schema.json#/$defs/id"}}}`
)

func TestSchemasAreReadFromAssetsAndNamedBelowTheRoot(t *testing.T) {
	t.Parallel()
	a := files{"s/v1/common.schema.json": common, "s/v1/doc.schema.json": doc, "s/v1/notes.txt": "not a schema"}
	s, err := jsonschema.FromAssets(a, "s")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Check("v1/doc.schema.json", []byte(`{"id":1}`)); err != nil || len(got) != 0 {
		t.Errorf("Check(valid) = %v, %v, want none", got, err)
	}
	if got, err := s.Check("v1/doc.schema.json", []byte(`{"id":"x"}`)); err != nil || len(got) == 0 {
		t.Errorf("Check(invalid) = %v, %v, want a problem: the reference to another file did not hold", got, err)
	}
	if got, _ := s.Check("v1/notes.txt", []byte(`{}`)); len(got) != 1 {
		t.Errorf("a file that is not JSON was compiled: %v", got)
	}
}

func TestNoSchemaUnderTheRootIsAnError(t *testing.T) {
	t.Parallel()
	if _, err := jsonschema.FromAssets(files{"other/x.json": common}, "s"); err == nil {
		t.Error("FromAssets took a root with no schema")
	}
}

func TestSchemasThatDoNotCompileAreAnErrorAndEmbeddedTellsItAtCheck(t *testing.T) {
	t.Parallel()
	a := files{"s/bad.schema.json": `{"type": 5}`}
	if _, err := jsonschema.FromAssets(a, "s"); err == nil {
		t.Fatal("FromAssets took a schema that does not compile")
	}
	got, err := jsonschema.Embedded(a, "s").Check("bad.schema.json", []byte(`{}`))
	if err == nil || got != nil {
		t.Errorf("Embedded over bad schemas checked a document: %v, %v", got, err)
	}
	ok := jsonschema.Embedded(files{"s/doc.schema.json": `{"type":"object"}`}, "s")
	if got, err := ok.Check("doc.schema.json", []byte(`{}`)); err != nil || got != nil {
		t.Errorf("Embedded over good schemas = %v, %v, want none", got, err)
	}
}

// TestTwoRootsAreOneSchemaSetAndAClashIsAnError: the format schemas and the
// contract's are compiled together, each named below its own root, and a name
// both hold is refused rather than one silently winning.
func TestTwoRootsAreOneSchemaSetAndAClashIsAnError(t *testing.T) {
	t.Parallel()
	a := files{"f/v1/common.schema.json": common, "f/v1/doc.schema.json": doc, "c/v9/common.schema.json": common, "c/v9/other.schema.json": doc}
	s, err := jsonschema.FromAssets(a, "f", "c")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"v1/doc.schema.json", "v9/other.schema.json"} {
		if got, err := s.Check(name, []byte(`{"id":1}`)); err != nil || len(got) != 0 {
			t.Errorf("Check(%s) = %v, %v, want none", name, got, err)
		}
	}
	clash := files{"f/v1/doc.schema.json": doc, "c/v1/doc.schema.json": doc}
	if _, err := jsonschema.FromAssets(clash, "f", "c"); err == nil {
		t.Error("two roots holding one name were taken")
	}
}
