package audit_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// TestAListsTextTellsAStringWithACommaFromTwoItems: a keep-list of one string holding a comma matches no file, so
// it drops every finding, while two items keep two files. The two must be two
// rows, so a list rewritten from one to the other is a new loosening against
// the merge base, and never the same row passed as unchanged.
// Risk: paths-except: ["a.go, b.go"] reads as [a.go, b.go], and a keep-list
// that drops every finding passes the count.
func TestAListsTextTellsAStringWithACommaFromTwoItems(t *testing.T) {
	t.Parallel()
	count := func(list string) []audit.Counted {
		return audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: "linters:\n  exclusions:\n    paths-except: " + list + "\n"}, formed())
	}
	two, one := count("[a.go, b.go]"), count(`["a.go, b.go"]`)
	if len(two) != 1 || len(one) != 1 {
		t.Fatalf("rows %+v and %+v, want one each", two, one)
	}
	if two[0].Text == one[0].Text {
		t.Errorf("two items and one string with a comma both read %q, want two rows", two[0].Text)
	}
	if got := audit.CheckCounts(rules(), two, one); len(got) != 1 || !strings.Contains(got[0].Missing, "is a new loosening") {
		t.Errorf("two items rewritten as one string: %+v, want a new loosening", got)
	}
}
