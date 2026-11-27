package model_test

import (
	"encoding/json"
	"fmt"
	"path"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestTheRungIsHeldAtMaxRungByTheSchemaAndItsExamples (boundary: MaxRung): the
// rung is a field the schema bounds, and the Go code reads it without a check
// of its own, so the cap is held where the schema is run: host/jsonschema's
// format tests run every example of examples/rules.valid.json and
// rules.invalid.json through the real adapter. This test holds those examples
// to the constant: the schema's maximum is MaxRung, a valid example binds at
// MaxRung, and an invalid one binds one rung above it and names the maximum as
// its problem, so the two sides of the cap are examples that cannot drift from
// it.
func TestTheRungIsHeldAtMaxRungByTheSchemaAndItsExamples(t *testing.T) {
	t.Parallel()
	_, node := loadSet(t).pointer(t, "v3/rule.schema.json", "/$defs/binding/properties/rung")
	if node["maximum"] != float64(model.MaxRung) {
		t.Errorf("the schema's maximum rung is %v, want MaxRung = %d", node["maximum"], model.MaxRung)
	}
	rungs := map[float64]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if r, ok := n["rung"].(float64); ok {
				rungs[r] = true
			}
			for _, c := range n {
				walk(c)
			}
		case []any:
			for _, c := range n {
				walk(c)
			}
		}
	}
	walk(readJSON(t, path.Join(examples, "rules.valid.json")))
	if !rungs[float64(model.MaxRung)] {
		t.Errorf("no valid example binds at MaxRung = %d", model.MaxRung)
	}
	var invalid []struct {
		Name  string `json:"name"`
		Patch []struct {
			Op, Path string
			Value    any
		} `json:"patch"`
		Problem string `json:"problem"`
	}
	b := readJSON(t, path.Join(examples, "rules.invalid.json"))
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &invalid); err != nil {
		t.Fatal(err)
	}
	over := fmt.Sprint(float64(model.MaxRung + 1))
	found := false
	for _, c := range invalid {
		for _, p := range c.Patch {
			if p.Op == "replace" && path.Base(p.Path) == "rung" && fmt.Sprint(p.Value) == over && c.Problem == "maximum" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no invalid example binds at MaxRung+1 = %d and is refused for its maximum", model.MaxRung+1)
	}
}
