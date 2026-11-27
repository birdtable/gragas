package jsonschema_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/port"
)

// The pin's schema, read where it stands beside host/assets,
// which embeds it, and compiled through the same adapter as the format's.

// pinSchema is the pin's schema, by the name the Schema port knows it.
const pinSchema = "v1/pin.schema.json"

// cleanPinJSON is internal/core/project/testdata/pin/clean.yaml as JSON: the
// clean pin, which each row of TestPinSchema breaks in one key.
const cleanPinJSON = `{"rulebook": "fixture-base", "source": "rulebook", "parts": ["base", "go", "web"], "format": 1}`

func projectSchema(t *testing.T) port.Schema {
	t.Helper()
	s, err := jsonschema.FromAssets(disk{}, "project/schema")
	if err != nil {
		t.Fatalf("the project schemas do not compile: %v", err)
	}
	return s
}

func TestPinSchema(t *testing.T) {
	t.Parallel()
	s := projectSchema(t)
	for _, doc := range []string{
		cleanPinJSON,
		`{"rulebook": "fixture-base", "source": ".", "parts": ["base"], "format": 1}`,
		`{"rulebook": "fixture-base", "source": "books/main", "parts": ["base"], "format": 1}`,
		`{"rulebook": "fixture-base", "source": ".hidden/...x", "parts": ["base"], "format": 1}`,
	} {
		if probs, err := s.Check(pinSchema, []byte(doc)); err != nil || len(probs) != 0 {
			t.Errorf("the pin schema refuses %s: %v, %v", doc, probs, err)
		}
	}
	for name, doc := range map[string]string{
		"rulebook missing":    `{"source": "rulebook", "parts": ["base"], "format": 1}`,
		"rulebook empty":      `{"rulebook": " ", "source": "rulebook", "parts": ["base"], "format": 1}`,
		"rulebook a number":   `{"rulebook": 3, "source": "rulebook", "parts": ["base"], "format": 1}`,
		"source missing":      `{"rulebook": "fixture-base", "parts": ["base"], "format": 1}`,
		"source empty":        `{"rulebook": "fixture-base", "source": "", "parts": ["base"], "format": 1}`,
		"source leaves":       `{"rulebook": "fixture-base", "source": "../rulebook", "parts": ["base"], "format": 1}`,
		"source leaves below": `{"rulebook": "fixture-base", "source": "books/..", "parts": ["base"], "format": 1}`,
		"source absolute":     `{"rulebook": "fixture-base", "source": "/books/rulebook", "parts": ["base"], "format": 1}`,
		"source a git host":   `{"rulebook": "fixture-base", "source": "https://example.com/r.git", "parts": ["base"], "format": 1}`,
		"source unclean":      `{"rulebook": "fixture-base", "source": "./rulebook", "parts": ["base"], "format": 1}`,
		"source a slash last": `{"rulebook": "fixture-base", "source": "rulebook/", "parts": ["base"], "format": 1}`,
		"parts missing":       `{"rulebook": "fixture-base", "source": "rulebook", "format": 1}`,
		"parts empty":         `{"rulebook": "fixture-base", "source": "rulebook", "parts": [], "format": 1}`,
		"a part empty":        `{"rulebook": "fixture-base", "source": "rulebook", "parts": ["base", ""], "format": 1}`,
		"parts a string":      `{"rulebook": "fixture-base", "source": "rulebook", "parts": "base", "format": 1}`,
		"format missing":      `{"rulebook": "fixture-base", "source": "rulebook", "parts": ["base"]}`,
		"format newer":        `{"rulebook": "fixture-base", "source": "rulebook", "parts": ["base"], "format": 2}`,
		"format zero":         `{"rulebook": "fixture-base", "source": "rulebook", "parts": ["base"], "format": 0}`,
		"format a string":     `{"rulebook": "fixture-base", "source": "rulebook", "parts": ["base"], "format": "1"}`,
		"a key it lacks":      `{"rulebook": "fixture-base", "source": "rulebook", "parts": ["base"], "format": 1, "version": "1.2.0"}`,
	} {
		if probs, err := s.Check(pinSchema, []byte(doc)); err != nil || len(probs) == 0 {
			t.Errorf("the pin schema accepts a pin with its %s: %s (%v)", name, doc, err)
		}
	}
}

func TestPinSchemaRefusesAnEmptyDocument(t *testing.T) {
	t.Parallel()
	s := projectSchema(t)
	for _, doc := range []string{`{}`, `null`, `[]`} {
		if probs, err := s.Check(pinSchema, []byte(doc)); err != nil || len(probs) == 0 {
			t.Errorf("the pin schema accepts %s: %v", doc, err)
		}
	}
}
