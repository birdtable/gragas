package planned_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/cli/planned"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// TestThePlannedFilesLoad: every file in the folder is a row the registry takes,
// not built, with an order of its own. A file that does not load is told by
// name, and the registry's rules are the same as for a row written in Go.
func TestThePlannedFilesLoad(t *testing.T) {
	t.Parallel()
	rows, err := planned.Load(planned.Files)
	if err != nil {
		t.Fatal(err)
	}
	// attention.yaml (built by), import.yaml (built by), undo.yaml (built
	// by #1148) and bind, rebind, unbind, explain and resolve (built by) are
	// all gone
	if len(rows) < 27 {
		t.Fatalf("%d planned commands, so the folder is not being read", len(rows))
	}
	for _, c := range rows {
		if c.State != kit.NotBuilt || c.Order < 0 {
			t.Errorf("%s: state %q, order %d, want not built; a file with no order is placed by the registry", c.Name, c.State, c.Order)
		}
	}
	if _, err := kit.NewRegistry(kit.Assemble(rows, nil).Rows()); err != nil {
		t.Error(err)
	}
}

const good = `name: waive
order: 420
context: P
ends: plan
summary: "Waive a rule"
examples:
  - {line: "gragas waive <id>", does: "show the plan"}
  - {line: "gragas waive <id> --apply", does: "carry it out"}
exits: ["done", "declined"]
json: plan
`

// TestAFileThatIsNotARowIsTold: each way a file can be wrong is said, with its name.
func TestAFileThatIsNotARowIsTold(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, from, to, says string }{
		{"an unknown key", "json: plan", "json: plan\ncolour: red", "colour is not a key"},
		{"a missing key", "ends: plan\n", "", "ends is missing"},
		{"another name than the file's", "name: waive", "name: other", `name "other" is not the file's`},
		{"an order that is no number", "order: 420", "order: soon", "order is not a whole number"},
		{"one example", "  - {line: \"gragas waive <id> --apply\", does: \"carry it out\"}\n", "", "examples are 1, want two"},
		{"a document that is no map", good, "- a\n- b\n", "not a map"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text := strings.Replace(good, tc.from, tc.to, 1)
			if text == good {
				t.Fatalf("the plant %q is not in the file", tc.from)
			}
			_, err := planned.Load(fstest.MapFS{"waive.yaml": {Data: []byte(text)}})
			if err == nil || !strings.Contains(err.Error(), "waive.yaml") || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Load = %v, want waive.yaml saying %q", err, tc.says)
			}
		})
	}
	if rows, err := planned.Load(fstest.MapFS{"waive.yaml": {Data: []byte(good)}}); err != nil || len(rows) != 1 || rows[0].Name != "waive" {
		t.Errorf("a good file: %v, %v", rows, err)
	}
}

// TestAFileWithNoOrderIsPlacedByTheRegistry: the file gragas-new writes sets no
// order, and the registry gives it the next free slot beside the others.
func TestAFileWithNoOrderIsPlacedByTheRegistry(t *testing.T) {
	t.Parallel()
	text := strings.Replace(good, "order: 420\n", "", 1)
	rows, err := planned.Load(fstest.MapFS{"waive.yaml": {Data: []byte(text)}})
	if err != nil || len(rows) != 1 || rows[0].Order != 0 {
		t.Fatalf("Load = %v, %v, want one row with no order", rows, err)
	}
	all, err := planned.Load(planned.Files)
	if err != nil {
		t.Fatal(err)
	}
	more, err := planned.Load(fstest.MapFS{"clear.yaml": {Data: []byte(strings.ReplaceAll(text, "waive", "clear"))}})
	if err != nil {
		t.Fatal(err)
	}
	placed := kit.Assemble(append(all, more...), nil).Rows()
	for _, c := range placed {
		if c.Order <= 0 {
			t.Errorf("%s has order %d after the registry placed the rows", c.Name, c.Order)
		}
	}
	if _, err := kit.NewRegistry(placed); err != nil {
		t.Error(err)
	}
}
