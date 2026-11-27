package jsonschema_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

func TestTheSchemaAdapterPassesTheSchemaSuite(t *testing.T) {
	t.Parallel()
	contracttest.Schema(t, func(t *testing.T, c contracttest.SchemaCase) port.Schema {
		t.Helper()
		s, err := jsonschema.New([]model.File{{Path: c.Name, Data: []byte(c.Text)}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return s
	})
}

func TestASchemaThatDoesNotCompileIsNoAdapter(t *testing.T) {
	t.Parallel()
	if _, err := jsonschema.New([]model.File{{Path: "bad.schema.json", Data: []byte(`{"type": 5}`)}}); err == nil {
		t.Error("New took a schema that does not compile")
	}
}

func TestACheckSaysTheLineOfTheProblem(t *testing.T) {
	t.Parallel()
	s, err := jsonschema.New([]model.File{{Path: "d.schema.json", Data: []byte(`{"type":"object","properties":{"a":{"type":"integer"}}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Check("d.schema.json", []byte("{\n\"a\": \"x\"\n}"))
	if err != nil || len(got) != 1 || got[0].Line != 2 {
		t.Errorf("Check = %v, %v, want one problem on line 2", got, err)
	}
}
