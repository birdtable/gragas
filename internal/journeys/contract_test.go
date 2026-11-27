package journeys

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// goldenFixture is the repository's contract/fixture as a map of its files.
func goldenFixture(t *testing.T) map[string]string {
	t.Helper()
	base := filepath.Join(moduleRoot(), "contract", "fixture")
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
		t.Fatalf("contract/fixture: %d files, %v", len(files), err)
	}
	return files
}

func contractRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"contract"}, args...)...)
}

// TestFixtureWritesTheGoldensAndTheGateComparesThemWithoutWriting: a dry run
// writes nothing and ends on the apply line; --apply writes each golden into
// --out and nothing beside it; what it writes is byte for byte contract/fixture,
// and a second run changes nothing. The comparison is the gate's step, and it
// never writes (tests.generated.5). Risk: a fixture the binary writes that is
// not the one the repository holds, or a dry run that writes.
func TestFixtureWritesTheGoldensAndTheGateComparesThemWithoutWriting(t *testing.T) {
	t.Parallel()
	want := goldenFixture(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	dry := contractRun(t, dir, "80", "fixture", "--out", "out")
	if dry.code != 6 || !strings.Contains(dry.stdout+dry.stderr, "--apply") {
		t.Errorf("dry run: exit %d, want 6 and the apply line\n%s%s", dry.code, dry.stdout, dry.stderr)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "out")); len(entries) != 0 {
		t.Fatalf("a dry run wrote %d entries", len(entries))
	}
	dryJSON := contractRun(t, dir, "80", "--json", "fixture", "--out", "out")
	if dryJSON.code != 6 || !strings.Contains(dryJSON.stdout, `"wrote": false`) {
		t.Errorf("dry run under --json: exit %d, want 6 and \"wrote\": false\n%s%s", dryJSON.code, dryJSON.stdout, dryJSON.stderr)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "out")); len(entries) != 0 {
		t.Fatalf("a dry run under --json wrote %d entries", len(entries))
	}
	for pass := 1; pass <= 2; pass++ {
		r := contractRun(t, dir, "80", "fixture", "--out", "out", "--apply")
		if r.code != 0 {
			t.Fatalf("pass %d: exit %d\n%s%s", pass, r.code, r.stdout, r.stderr)
		}
	}
	got := map[string]string{}
	err := filepath.WalkDir(filepath.Join(dir, "out"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join(dir, "out"), p)
		got[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range want {
		if got[name] != body {
			t.Errorf("%s differs from contract/fixture", name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s was written and contract/fixture holds no such file", name)
		}
	}
	if missing := contractRun(t, dir, "80", "fixture", "--out", "nowhere", "--apply"); missing.code == 0 {
		t.Errorf("a folder that does not exist ended done\n%s%s", missing.stdout, missing.stderr)
	}
}

// TestValidateOnEachGoldenEndsDoneAndOnAPlantedOneFindings: each document of the
// fixture is read as the kind its fields name and ends done; a bundle with a
// field of the wrong type ends findings; a kind that is none ends usage and
// lists the kinds; a document in a newer major ends not ready and names both
// versions. Risk: a schema the fixture breaks, a kind guessed wrong, or a
// newer major told as findings.
func TestValidateOnEachGoldenEndsDoneAndOnAPlantedOneFindings(t *testing.T) {
	t.Parallel()
	files := goldenFixture(t)
	dir := t.TempDir()
	put(t, dir, files)
	for name := range files {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		r := contractRun(t, dir, "80", "validate", name, "--json")
		var d struct {
			Kind     string            `json:"kind"`
			Findings []json.RawMessage `json:"findings"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &d); err != nil || r.code != 0 || len(d.Findings) != 0 {
			t.Errorf("%s: exit %d, %d findings, %v\n%s%s", name, r.code, len(d.Findings), err, r.stdout, r.stderr)
		}
	}
	bad := strings.Replace(files["bundle.json"], `"adapters_used": [`, `"adapters_used": "none", "was": [`, 1)
	newer := strings.Replace(files["report-clean.json"], "\n  \"contract\": \"1.3\"", "\n  \"contract\": \"2.0\"", 1)
	if bad == files["bundle.json"] || newer == files["report-clean.json"] {
		t.Fatal("a plant did not change its document")
	}
	put(t, dir, map[string]string{"bad.json": bad, "newer.json": newer, "nokind.json": `{"hello": 1}`})
	cases := []struct {
		name  string
		args  []string
		code  int
		wants []string
	}{
		{"contract-validate-findings", []string{"validate", "bad.json"}, 1, []string{"adapters_used"}},
		{"contract-validate-usage", []string{"validate", "nokind.json"}, 2, []string{"bundle", "report", "changelog", "sarif", "plan", "proposal"}},
		{"contract-validate-bad-kind", []string{"validate", "bundle.json", "--as", "nope"}, 2, []string{"bundle", "proposal"}},
		{"contract-validate-not-ready", []string{"validate", "newer.json"}, 4, []string{"2.0", "1.3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := contractRun(t, dir, "80", tc.args...)
			if r.code != tc.code {
				t.Errorf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
			}
			for _, w := range tc.wants {
				if !strings.Contains(r.stdout+r.stderr, w) {
					t.Errorf("output lacks %q\n%s%s", w, r.stdout, r.stderr)
				}
			}
			uitest.Golden(t, tc.name+"-80", transcript(append([]string{"contract"}, tc.args...), r))
		})
	}
}

// TestFieldsRuleGoldens: the rule entity's fields with their versions, at
// three widths and as JSON. Risk: a table that overflows a narrow screen or a
// JSON answer that is more than one document.
func TestFieldsRuleGoldens(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cols := range []string{"80", "60", "40"} {
		r := contractRun(t, dir, cols, "fields", "rule")
		if r.code != 0 || !strings.Contains(r.stdout, "standing") {
			t.Errorf("%s columns: exit %d\n%s%s", cols, r.code, r.stdout, r.stderr)
		}
		uitest.Golden(t, "contract-fields-rule-"+cols, transcript([]string{"contract", "fields", "rule"}, r))
	}
	r := contractRun(t, dir, "80", "fields", "rule", "--json")
	var d struct {
		Entity string `json:"entity"`
		Fields []struct {
			Name, Since string
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil || d.Entity != "rule" || len(d.Fields) == 0 || d.Fields[0].Since == "" {
		t.Errorf("--json: %v\n%s%s", err, r.stdout, r.stderr)
	}
	uitest.Golden(t, "contract-fields-rule-json", transcript([]string{"contract", "fields", "rule", "--json"}, r))
	if u := contractRun(t, dir, "80", "fields", "nope"); u.code != 2 || !strings.Contains(u.stdout+u.stderr, "rule") {
		t.Errorf("an unknown entity: exit %d\n%s%s", u.code, u.stdout, u.stderr)
	}
}
