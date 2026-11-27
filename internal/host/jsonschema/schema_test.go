package jsonschema_test

import (
	"os"
	"slices"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/ordered"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// schemas compiles a copy of the five schemas the document checks use, kept
// in testdata/schema so no test reads the live rulebook/. Inputs read:
// testdata/schema/*.json.
func schemas(t *testing.T) *jsonschema.Set {
	t.Helper()
	repo := os.DirFS("testdata")
	s, err := jsonschema.Compile(repo, "schema", "common.schema.json", "rules.schema.json", "tools.schema.json", "products.schema.json", "parts.schema.json")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return s
}

func problems(t *testing.T, s *jsonschema.Set, schema, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ordered.ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range s.Validate(schema, doc) {
		out = append(out, ordered.Repr(int64(p.Line))+": "+p.Msg)
	}
	return out
}

func TestASchemaErrorIsSaidAtItsPlaceAndLineInSchemaOrder(t *testing.T) {
	t.Parallel()
	want := []string{
		"6: schema at rules/0: 'sources' is a required property",
		"6: schema at rules/0: Additional properties are not allowed ('colour' was unexpected)",
		"6: schema at rules/0: 'why' is a required property",
		"18: schema at rules/0/checks/0/tier: 5 is not one of ['G', '0', '1', '2', '3', '4', 'R']",
	}
	if got := problems(t, schemas(t), "rules.schema.json", "testdata/schema-error.yaml"); !slices.Equal(got, want) {
		t.Errorf("problems:\n got  %q\n want %q", got, want)
	}
}

func TestAValidDocumentHasNoProblem(t *testing.T) {
	t.Parallel()
	if got := problems(t, schemas(t), "parts.schema.json", "testdata/parts.yaml"); len(got) != 0 {
		t.Errorf("parts.yaml: %q, want none", got)
	}
}

func TestASchemaNotCompiledIsAProblemNotAPanic(t *testing.T) {
	t.Parallel()
	got := schemas(t).Validate("nope.schema.json", nil)
	if len(got) != 1 || got[0].Msg != "schema nope.schema.json was not compiled" {
		t.Errorf("Validate = %v, want one problem naming the schema", got)
	}
}
