package coreapi_test

import (
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func readersFrom(t *testing.T, doc string) coreapi.Readers {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedAssets(map[string]string{coreapi.ReadersFile: doc})
	return coreapi.LoadReaders(w.Ports().Assets)
}

// TestTheReadersFileIsLoadedInItsOrder: one row per tool, files and kind as
// written, the order the rows are in.
func TestTheReadersFileIsLoadedInItsOrder(t *testing.T) {
	t.Parallel()
	r := readersFrom(t, "readers:\n  - {tool: b, files: [b.yml, b.yaml], kind: k1}\n  - {tool: a, files: [a.toml], kind: k2}\n")
	want := []coreapi.Reader{{Tool: "b", Files: []string{"b.yml", "b.yaml"}, Kind: "k1"}, {Tool: "a", Files: []string{"a.toml"}, Kind: "k2"}}
	if r.Refused != "" || !slices.EqualFunc(r.Rows, want, func(x, y coreapi.Reader) bool {
		return x.Tool == y.Tool && x.Kind == y.Kind && slices.Equal(x.Files, y.Files)
	}) {
		t.Errorf("Readers = %+v, want %+v", r, want)
	}
}

// TestAReadersFileThatIsNotWholeRefusesEveryTool: a row with a path for a
// file, a missing field, a repeated tool or no row at all is refused as a
// whole, so the tool configs are never half checked.
func TestAReadersFileThatIsNotWholeRefusesEveryTool(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ doc, want string }{
		"a path for a file":  {"readers:\n  - {tool: a, files: [../x.yml], kind: k}\n", "not a name at the root"},
		"no files":           {"readers:\n  - {tool: a, kind: k}\n", "needs a tool, a kind"},
		"no kind":            {"readers:\n  - {tool: a, files: [a.yml]}\n", "needs a tool, a kind"},
		"a tool twice":       {"readers:\n  - {tool: a, files: [a.yml], kind: k}\n  - {tool: a, files: [b.yml], kind: k}\n", "named twice"},
		"no rows":            {"readers: []\n", "holds no readers"},
		"not a mapping":      {"- a\n", "no mapping"},
		"a file that is not": {"readers:\n  - {tool: a, files: [3], kind: k}\n", "is not a name"},
	} {
		r := readersFrom(t, tc.doc)
		if len(r.Rows) != 0 || !strings.Contains(r.Refused, tc.want) {
			t.Errorf("%s: Readers = %+v, want no rows and a reason with %q", name, r, tc.want)
		}
	}
}

// TestAnAssetsPortWithoutTheFileGivesNoReadersAndNoRefusal is the subset
// build's case: the tool configs are not read, and nothing is wrong.
func TestAnAssetsPortWithoutTheFileGivesNoReadersAndNoRefusal(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	if r := coreapi.LoadReaders(w.Ports().Assets); len(r.Rows) != 0 {
		t.Errorf("Readers = %+v, want none", r)
	}
}
