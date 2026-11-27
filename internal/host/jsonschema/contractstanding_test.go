package jsonschema_test

import (
	"testing"
)

// A rule's standing is one of S32's six and its marks are the three of S32,
// each at most once.
func TestARuleCarriesAStandingOfSixAndMarksOfThree(t *testing.T) {
	t.Parallel()
	s := contractSchema(t)
	rule := func(standing, marks string) []byte {
		return []byte(`{"id":"go.errors.1","statement":"s","strength":"must","status":"trial",` +
			`"standing":"` + standing + `","marks":` + marks + `,"origin":"authored"}`)
	}
	for _, tc := range []struct {
		name     string
		standing string
		marks    string
		bad      bool
	}{
		{"enforced with trial", "enforced", `["trial"]`, false},
		{"planned with every mark", "planned", `["trial","blocked","unproven"]`, false},
		{"advisory with none", "advisory", `[]`, false},
		{"trial is a mark, not a standing", "trial", `[]`, true},
		{"proven is no standing", "proven", `[]`, true},
		{"a mark twice", "enforced", `["trial","trial"]`, true},
		{"an unknown mark", "enforced", `["stale"]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.Check("v1/entity-rule.schema.json", rule(tc.standing, tc.marks))
			if err != nil {
				t.Fatal(err)
			}
			if (len(got) > 0) != tc.bad {
				t.Errorf("want refused: %v, got %v", tc.bad, got)
			}
		})
	}
}
