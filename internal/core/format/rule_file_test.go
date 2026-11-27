package format

import (
	"strings"
	"testing"

	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestRuleFileIsWhereARuleIsWrittenOrWhereANewOneWouldGo(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", fullBook(t))
	l, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, want string }{
		{"go.errors.1", errorsFile},  // a rule the book holds
		{"go.errors.99", errorsFile}, // a new number in the same concern
		{"go.nothing.1", ""},         // a concern the structure lacks
		{"nothing.errors.1", ""},     // a part the structure lacks
		{"go", ""},                   // not an id
	} {
		got, err := RuleFile(l, tc.id)
		if tc.want == "" {
			if err == nil || !strings.Contains(err.Error(), "structure.yaml") {
				t.Errorf("%s: got %q, %v; want an error naming structure.yaml", tc.id, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %s", tc.id, got, err, tc.want)
		}
	}
}

func TestANewRuleOfAConcernWrittenInAGuideGoesToTheGuide(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files[guideFile] = proseGuide
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", files)
	l, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"go.layout.9", "go.layout.10"} {
		if got, err := RuleFile(l, id); err != nil || got != guideFile {
			t.Errorf("%s: got %q, %v; want %s", id, got, err, guideFile)
		}
	}
}
