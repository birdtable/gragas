package layers_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// issuedLint is a lint file whose ban, deny and exclusion each open with an
// issued id, beside a ban that opens with no rule id at all.
const issuedLint = `linters:
  settings:
    forbidigo:
      forbid:
        - pattern: ^os\.Exit$
          msg: "go.errors.4: return the error"
        - pattern: ^fmt\.Sprint$
          msg: "say it with fmt.Sprintf"
    depguard:
      rules:
        ports:
          deny:
            - pkg: syscall
              desc: "arch.ports.24: the host layer's adapters only"
  exclusions:
    rules:
      - path: ^cmd/
        linters: [forbidigo]
        text: "go\\.errors\\.[34]:"
`

// issuedIDs are the ids issuedLint's heads cite.
func issuedIDs() []string { return []string{"arch.ports.24", "go.errors.3", "go.errors.4"} }

// TestARowThatCitesAnUnissuedRuleIsRefused: each head of a lint row is held
// to the ids ever issued. A real id passes; an id no rule ever had is
// refused in a ban's message, a deny's reason and an exclusion's text, with
// the row and the id named. Risk: a ban that cites a rule that
// does not exist.
func TestARowThatCitesAnUnissuedRuleIsRefused(t *testing.T) {
	t.Parallel()
	if got := layers.Unissued([]byte(issuedLint), issuedIDs()); len(got) != 0 {
		t.Fatalf("the issued heads: Unissued = %q, want none", got)
	}
	made := "gates.readers." + "9"
	for _, tc := range []struct{ name, from, to, says string }{
		{"a ban's message", `"go.errors.4: return`, `"` + made + `: return`, `the forbidigo ban ^os\.Exit$ opens with ` + made},
		{"a deny's reason", `"arch.ports.24: the host`, `"` + made + `: the host`, "the depguard ban of syscall in ports opens with " + made},
		{"an exclusion's text", `"go\\.errors\\.[34]:"`, `"gates\\.readers\\.[9]:"`, "the exclusion of ^cmd/ opens with " + made},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lint := strings.Replace(issuedLint, tc.from, tc.to, 1)
			if lint == issuedLint {
				t.Fatalf("the plant %q changed nothing", tc.from)
			}
			got := layers.Unissued([]byte(lint), issuedIDs())
			if len(got) != 1 || !strings.Contains(got[0], tc.says) || !strings.Contains(got[0], layers.IDsFile+" does not hold") {
				t.Errorf("Unissued = %q, want one problem saying %q", got, tc.says)
			}
		})
	}
}

// TestNothingToHoldIsAProblem: no id, a lint file whose rows cite no rule,
// and a lint file that does not read each fail, rather than pass on nothing
// (gates.plants.5).
func TestNothingToHoldIsAProblem(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, lint string
		ids        []string
		says       string
	}{
		{"no id", issuedLint, nil, "holds no id"},
		{"no head", "linters:\n  settings:\n    forbidigo:\n      forbid:\n        - pattern: ^x$\n          msg: \"no rule\"\n", issuedIDs(), "no row of the lint file opens with a rule id"},
		{"no mapping", "- a\n- b\n", issuedIDs(), "does not read as a mapping"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := layers.Unissued([]byte(tc.lint), tc.ids)
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("Unissued = %q, want one saying %q", got, tc.says)
			}
		})
	}
}
