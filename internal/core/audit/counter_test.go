package audit_test

import (
	"reflect"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// TestACounterCountsEachFileAsCountOfDoes: the counter made once for a set of tools gives, for each file of a Go source,
// a test file and a config, the rows CountOf gives, and gives them again for
// the same file asked twice.
// Risk: a pattern compiled once that reads one file's state into the next.
func TestACounterCountsEachFileAsCountOfDoes(t *testing.T) {
	t.Parallel()
	nolint := "//" + "nolint:errcheck // a reason"
	skip := "t.Ski" + "p(\"later\")"
	count := audit.NewCounter(configs())
	docs := []audit.Doc{
		{Path: "a/x.go", Text: "package a\n\nfunc F() { _ = g() } " + nolint + "\n"},
		{Path: "a/x_test.go", Text: "package a\n\nfunc T(t *testing.T) {\n\t" + skip + "\n}\n"},
		{Path: ".golangci.yml", Text: "version: \"2\"\nlinters:\n  exclusions:\n    paths:\n      - vendor\n"},
	}
	for round := range 2 {
		for _, d := range docs {
			want := audit.CountOf(d, configs())
			if len(want) == 0 {
				t.Fatalf("%s counts no row, so the fixture proves nothing", d.Path)
			}
			if got := count(d); !reflect.DeepEqual(got, want) {
				t.Errorf("round %d, %s: counter gave %v, CountOf %v", round, d.Path, got, want)
			}
		}
	}
}
