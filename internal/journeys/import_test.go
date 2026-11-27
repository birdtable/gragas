package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// importRepo is the history rulebook with a guide folder beside it: the guide's
// Markdown, the one the lifecycle's goldens draft from.
func importRepo(t *testing.T) string {
	t.Helper()
	dir := historyRepo(t, "")
	guideInto(t, dir)
	return dir
}

// guideInto puts the guide folder into the rulebook at dir, and the style
// concern its import files into.
func guideInto(t *testing.T, dir string) {
	t.Helper()
	copyTree(t, filepath.Join(moduleRoot(), "internal/journeys/testdata/import/guide"), filepath.Join(dir, "guide"))
	structure := strings.Replace(read(t, dir, "structure.yaml"), "            name: Errors\n", "            name: Errors\n          - slug: style\n            name: Style\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "structure.yaml"), []byte(structure), 0o644); err != nil {
		t.Fatal(err)
	}
}

func importArgs(more ...string) []string {
	return append([]string{"import", "guide", "--into-part", "go", "--into-concern", "style", "--guide-slug", "handler-guide",
		"--guide-title", "Handler guide", "--guide-url", "https://example.org/handlers", "--guide-tag", "v1.2.0"}, more...)
}

// TestImportOfAGuideIsAPlanThenFilesAndASecondRunIsANoOp: without --apply the
// import shows the plan and writes nothing; with it the rules, the guide as a
// source, the import's decision and the history are written, every rule enters
// proposed and ships in no preset, validate then lists what the guide's prose leaves to reword as findings (the drafted statements are the guide's own words), and the same import
// again ends "nothing to do" and adds no history line.
// Risk: a dry run that writes, a rule that enters active with no binding, or a
// rerun that doubles the rules.
func TestImportOfAGuideIsAPlanThenFilesAndASecondRunIsANoOp(t *testing.T) {
	t.Parallel()
	dir, env := importRepo(t), ruleEnv(t)
	env["COLUMNS"] = "80"
	dry := run(t, dir, env, importArgs()...)
	if dry.code != 6 || !strings.Contains(dry.stdout, "declined") || !strings.Contains(dry.stdout, "proposed") || histN(t, dir) != 0 {
		t.Fatalf("dry run (a dry run ends declined, 6): exit %d, history %d\n%s%s", dry.code, histN(t, dir), dry.stdout, dry.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "language/go/rules/style.yaml")); err == nil {
		t.Fatal("a dry run wrote the rules file")
	}
	if r := run(t, dir, env, importArgs("--apply")...); r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	rules := read(t, dir, "language/go/rules/style.yaml")
	if strings.Contains(rules, "status: active") || !strings.Contains(rules, "origin: imported") || !strings.Contains(rules, "status: proposed") {
		t.Errorf("the imported rules are not all proposed and imported:\n%s", rules)
	}
	if src := read(t, dir, "sources.yaml"); !strings.Contains(src, "slug: handler-guide") || !strings.Contains(src, "https://example.org/handlers@v1.2.0") {
		t.Errorf("sources.yaml lacks the guide at its tag:\n%s", src)
	}
	if d := read(t, dir, "decisions.yaml"); !strings.Contains(d, "slug: imported-handler-guide-v1-2-0") {
		t.Errorf("decisions.yaml lacks the import's decision:\n%s", d)
	}
	n := histN(t, dir)
	if n == 0 || !strings.Contains(read(t, dir, "history.jsonl"), `"import"`) {
		t.Errorf("history holds %d lines, none an import", n)
	}
	if v := run(t, dir, env, "validate"); v.code != 1 || !strings.Contains(v.stdout, "style.yaml") {
		t.Fatalf("validate exit %d, want findings (1) naming style.yaml\n%s%s", v.code, v.stdout, v.stderr)
	}
	again := run(t, dir, env, importArgs("--apply")...)
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || histN(t, dir) != n || read(t, dir, "language/go/rules/style.yaml") != rules {
		t.Errorf("second run: exit %d, history %d, want %d\n%s%s", again.code, histN(t, dir), n, again.stdout, again.stderr)
	}
}

// TestImportOfNothingEndsDoneNothingToImport: no guide and no config ends done
// with the sentence, and writes nothing.
// Risk: an empty import that fails, or one that writes an empty plan.
func TestImportOfNothingEndsDoneNothingToImport(t *testing.T) {
	t.Parallel()
	dir, env := importRepo(t), ruleEnv(t)
	r := run(t, dir, env, "import", "--apply")
	if r.code != 0 || !strings.Contains(r.stdout, "nothing to import") || histN(t, dir) != 0 {
		t.Errorf("exit %d, history %d\n%s%s", r.code, histN(t, dir), r.stdout, r.stderr)
	}
}

// histN is the lines of history.jsonl, 0 when the book has none yet.
func histN(t *testing.T, dir string) int {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "history.jsonl")); err != nil {
		return 0
	}
	return hist(t, dir)
}

// TestImportOfAConfigWithNoPluginIsRefusedNamingTheTool: with no
// plugin installed for the tool no adapter reads its config; the import ends
// failed saying which tool and how to install a plugin, and writes nothing.
// Risk: a config read by a built-in adapter release 1 does not ship,
// or a refused import that wrote.
func TestImportOfAConfigWithNoPluginIsRefusedNamingTheTool(t *testing.T) {
	t.Parallel()
	dir, env := importRepo(t), ruleEnv(t)
	env["COLUMNS"] = "200"
	if err := os.WriteFile(filepath.Join(dir, "eslint.json"), []byte(`{"rules":{"no-console":[2],"eqeqeq":[1,"always"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := run(t, dir, env, importArgs("--tool", "eslint", "--target", "source", "--config", "eslint.json", "--apply")...)
	said := strings.Join(strings.Fields(r.stdout+r.stderr), " ")
	if r.code == 0 || !strings.Contains(said, "no plugin is installed for eslint") || !strings.Contains(said, "gragas plugin add") || histN(t, dir) != 0 {
		t.Errorf("exit %d, history %d, want a refusal naming eslint and plugin add, nothing written\n%s%s", r.code, histN(t, dir), r.stdout, r.stderr)
	}
}
