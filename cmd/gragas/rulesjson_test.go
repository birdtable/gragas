package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/birdtable/gragas/internal/host/jsonschema"
)

// rulesJSONSchema compiles the schema of the rules.json gen writes.
func rulesJSONSchema(t *testing.T) *jsonschema.Set {
	t.Helper()
	set, err := jsonschema.Compile(os.DirFS("../.."), "rulebook/schema", "rules-json-3.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// TestTheCommittedRulesJSONMeetsItsSchema: rulebook/rules.json, which gen
// --check holds to what gen writes, is a document of schema 3 as
// rules-json-3.schema.json says, so the schema describes the file gen writes.
func TestTheCommittedRulesJSONMeetsItsSchema(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../rulebook/rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, p := range rulesJSONSchema(t).Validate("rules-json-3.schema.json", doc) {
		t.Errorf("rules.json: %s", p.Msg)
	}
}

// TestTheRulesJSONSchemaRefusesWhatGenDoesNotWrite: a schema-2 document, a
// rule with a key gen does not write and a rule with no id are each refused.
func TestTheRulesJSONSchemaRefusesWhatGenDoesNotWrite(t *testing.T) {
	t.Parallel()
	set := rulesJSONSchema(t)
	for name, body := range map[string]string{
		"schema 2":          `{"generated_by":"gragas gen","schema":2,"parts":[],"kinds":[],"sources":[],"rules":[]}`,
		"a key not written": `{"generated_by":"gragas gen","schema":3,"parts":[],"kinds":[],"sources":[],"rules":[{"id":"a.b.1","part":"a","concern":"b","statement":"s","strength":"must","status":"active","origin":"authored","why":"x"}]}`,
		"a rule with no id": `{"generated_by":"gragas gen","schema":3,"parts":[],"kinds":[],"sources":[],"rules":[{"part":"a","concern":"b","statement":"s","strength":"must","status":"active","origin":"authored"}]}`,
	} {
		var doc any
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatal(err)
		}
		if len(set.Validate("rules-json-3.schema.json", doc)) == 0 {
			t.Errorf("%s: the schema accepted it", name)
		}
	}
}
