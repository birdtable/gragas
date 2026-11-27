package coreapi_test

import (
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// dated encodes as a JSON string by its own MarshalJSON, as a date type may.
type dated struct{}

func (dated) MarshalJSON() ([]byte, error) { return []byte(`"2026-12-01"`), nil }

// texted encodes as a JSON string by its own MarshalText.
type texted struct{}

func (texted) MarshalText() ([]byte, error) { return []byte("a \"quoted\" <text>\n"), nil }

// TestShownPrintsEachKindOfLifecycleValueAsItAlwaysDid is the table: one value
// of each kind the guard's rule fields hold, and what shown prints for it. It was
// written and run green on the code that decoded its own encoding, so the same
// table holds the change to that code.
func TestShownPrintsEachKindOfLifecycleValueAsItAlwaysDid(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		v    any
		want string
	}{
		"a named string type is bare":          {model.Status("active"), "active"},
		"a strength is bare":                   {model.Strength("must"), "must"},
		"a date string is bare":                {"2026-12-01", "2026-12-01"},
		"an empty string is empty":             {"", ""},
		"an escaped string is bare, unescaped": {"a \"q\" <b>\\ \n é", "a \"q\" <b>\\ \n é"},
		"a type with MarshalJSON to a string":  {dated{}, "2026-12-01"},
		"a type with MarshalText":              {texted{}, "a \"quoted\" <text>\n"},
		"a nil list is empty":                  {[]model.RuleID(nil), ""},
		"an empty list is empty":               {[]model.RuleID{}, ""},
		"a nil scope is empty":                 {(*model.RuleScope)(nil), ""},
		"no relations is empty":                {model.Relations{}, ""},
		"a list is compact JSON":               {[]model.RuleID{"a.b.1", "a.b.2"}, `["a.b.1","a.b.2"]`},
		"relations are compact JSON":           {model.Relations{Relates: []model.RuleID{"a.b.2"}}, `{"relates":["a.b.2"]}`},
		"exemptions are compact JSON":          {[]model.Exemption{{Paths: []string{"x/**"}, Reason: "r"}}, `[{"paths":["x/**"],"reason":"r"}]`},
		"a number is compact JSON":             {3, "3"},
		"a bool is compact JSON":               {true, "true"},
		"an unencodable value is unreadable":   {func() {}, "unreadable: json: unsupported type: func()"},
	} {
		if got := coreapi.Shown(tc.v); got != tc.want {
			t.Errorf("%s: shown = %q, want %q", name, got, tc.want)
		}
	}
}
