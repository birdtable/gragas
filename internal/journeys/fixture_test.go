package journeys

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// fixtureCopy is the fixture rulebook dir below internal/host/assets/fixture,
// copied into a folder of its own (tests.isolation.1), as a map of its files.
func fixtureCopy(t *testing.T, dir string) map[string]string {
	t.Helper()
	base := filepath.Join(moduleRoot(), "internal", "host", "assets", "fixture", dir)
	files := map[string]string{}
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("fixture/%s: %d files, %v", dir, len(files), err)
	}
	return files
}

// TestValidateOnTheFixtureRulebooksEndsDone: the built binary reads each fixture
// rulebook with the real schemas and ends done with no finding, and a copy with
// one concern file taken out ends on findings. The fixtures are the conformance
// suite, so a schema the fixture breaks is found here and in no cheaper test.
func TestValidateOnTheFixtureRulebooksEndsDone(t *testing.T) {
	t.Parallel()
	type doc struct {
		Word     string            `json:"word"`
		Rules    int               `json:"rules"`
		Total    int               `json:"total"`
		Findings []json.RawMessage `json:"findings"`
	}
	for dir, rules := range map[string]int{"base": 15, "other": 3} {
		r := validateRun(t, rulebookFrom(t, fixtureCopy(t, dir)), "80", "--json")
		var d doc
		if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
			t.Fatalf("fixture/%s: %v\n%s%s", dir, err, r.stdout, r.stderr)
		}
		if r.code != 0 || d.Word != "done" || d.Total != 0 || d.Rules != rules {
			t.Errorf("fixture/%s: exit %d, word %q, %d rules, %d findings; want done, %d rules, none\n%s", dir, r.code, d.Word, d.Rules, d.Total, rules, r.stdout)
		}
	}
	plant := fixtureCopy(t, "base")
	delete(plant, "language/web/rules/forms.yaml")
	r := validateRun(t, rulebookFrom(t, plant), "80", "--json")
	var d doc
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil || r.code != 1 || d.Word != "findings" || d.Total == 0 {
		t.Errorf("a base fixture without a concern file: exit %d, %q, %d findings, %v; want findings", r.code, d.Word, d.Total, err)
	}
}

// rulebookFrom is a folder holding exactly files.
func rulebookFrom(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	put(t, dir, files)
	return dir
}
