package gatetest

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
)

// TestShippedReadsTheMarkedFilesOfTheComparedToolsAlone: of three
// files that each name a rule on a generated line, Shipped reads the marked one of a tool
// preset --check compares, and leaves out the marked one of a tool the identity lists in
// hand_written_presets and the one whose mark a project removed. Planted failures: a Shipped
// that reads every marked file ships hand.listed.2, and one that finds the mark anywhere ships
// took.over.3.
func TestShippedReadsTheMarkedFilesOfTheComparedToolsAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	block := func(id string) string {
		return "# generated:linters:begin\n  - lint # " + id + "\n# generated:linters:end\n"
	}
	mark := "# " + link.Mark() + "\n"
	files := map[string]string{
		"rulebook/rulebook.yaml":    "name: book\nhand_written_presets: [lint-h]\n",
		"presets/lint-g/lint.yml":   mark + block("gen.served.1"),
		"presets/lint-h/lint.yml":   mark + block("hand.listed.2"),
		"presets/lint-o/lint.yml":   "# owned now\n" + mark + block("took.over.3"),
		"presets/lint-g/notes.text": "no mark\n" + block("not.generated.4"),
	}
	for rel, text := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := Book{Dir: dir, Rulebook: os.DirFS(filepath.Join(dir, "rulebook"))}
	if got := slices.Sorted(maps.Keys(b.Shipped(t))); !slices.Equal(got, []string{"gen.served.1"}) {
		t.Errorf("Shipped = %q, want [gen.served.1]", got)
	}
}
