package journeys

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// presetStates is the state of each file the document of a preset run holds,
// by path.
func presetStates(t *testing.T, out string) map[string]string {
	t.Helper()
	var doc struct {
		Data struct {
			Files []struct{ Path, State string }
		}
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("the document does not read: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, f := range doc.Data.Files {
		got[f.Path] = f.State
	}
	return got
}

// TestPresetJSONSaysWrittenOnlyForARunThatEndedDone: a dry run, and an apply
// that ends not ready, report the missing preset as differing and write
// nothing; the apply that ends done reports it written, and the file is there.
// Risk: a document that says a file was written in a run that wrote none.
func TestPresetJSONSaysWrittenOnlyForARunThatEndedDone(t *testing.T) {
	t.Parallel()
	dir, env := boundRepo(t), ruleEnv(t)
	servedBy(t, dir, env)
	if r := run(t, dir, env, bindArgs("--apply")...); r.code != 0 {
		t.Fatalf("bind: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	file := presetOf(dir)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	states := func(r result) string {
		for p, s := range presetStates(t, r.stdout) {
			if strings.HasSuffix(p, presetFile) {
				return s
			}
		}
		return ""
	}
	if r := run(t, dir, env, "preset", "--json"); states(r) != "differs" {
		t.Errorf("a dry run: %q, want differs\n%s%s", states(r), r.stdout, r.stderr)
	}
	// the settings name no git, so the apply ends not ready after the check
	bare := map[string]string{}
	maps.Copy(bare, env)
	nogit := filepath.Join(filepath.Dir(env["GRAGAS_CONFIG"]), "nogit.yaml")
	if err := os.WriteFile(nogit, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bare["GRAGAS_CONFIG"] = nogit
	r := run(t, dir, bare, "preset", "--json", "--apply")
	if r.code != 4 || states(r) != "differs" {
		t.Errorf("an apply that ends not ready: exit %d, %q, want 4 and differs\n%s%s", r.code, states(r), r.stdout, r.stderr)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("the not-ready run wrote the file")
	}
	if r := run(t, dir, env, "preset", "--json", "--apply"); r.code != 0 || states(r) != "written" {
		t.Errorf("the apply: exit %d, %q, want 0 and written\n%s%s", r.code, states(r), r.stdout, r.stderr)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("the apply left no file: %v", err)
	}
}

// presetFirsts is where each differing file of the document of a preset
// --check first differs, by path.
func presetFirsts(t *testing.T, out string) map[string]string {
	t.Helper()
	var doc struct {
		Data struct {
			Files []struct{ Path, First string }
		}
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("the document does not read: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, f := range doc.Data.Files {
		got[f.Path] = f.First
	}
	return got
}

// TestPresetCheckHoldsAMarkedFileAndNamesAHandWrittenTool: the
// stand-in's preset is copied once and asked for the mark. Untouched, the
// check ends done; edited while it carries the mark, it ends findings naming
// the first line that differs; with the mark removed it is the project's, named
// so and not counted as compared, and the check ends done. A bound tool no plugin serves ends the check not
// ready; listed in hand_written_presets, it is named as hand-written and the
// check ends done. Risk: a marked preset edited by hand passed as the
// project's, or the list read as leave to skip every tool.
func TestPresetCheckHoldsAMarkedFileAndNamesAHandWrittenTool(t *testing.T) {
	t.Parallel()
	dir, env := boundRepo(t), ruleEnv(t)
	servedBy(t, dir, env)
	if r := run(t, dir, env, bindArgs("--apply")...); r.code != 0 {
		t.Fatalf("bind: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := run(t, dir, env, "preset", "--check"); r.code != 0 || !strings.Contains(r.stdout, "none differs") {
		t.Fatalf("untouched: exit %d, want 0 and none differs\n%s%s", r.code, r.stdout, r.stderr)
	}
	file := presetOf(dir)
	made, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(made, "extra\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	r := run(t, dir, env, "preset", "--check", "--json")
	if first := presetFirsts(t, r.stdout); r.code != 1 || !strings.HasPrefix(first["presets/golangci-lint/"+presetFile], "line 3 is not one the book makes") {
		t.Errorf("edited and marked: exit %d, %q, want 1 and line 3 named\n%s%s", r.code, first, r.stdout, r.stderr)
	}
	_, body, _ := strings.Cut(string(made), "\n")
	if err := os.WriteFile(file, []byte(body+"extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// the board wraps a long note, so the words are read with the lines joined
	if r := run(t, dir, env, "preset", "--check"); r.code != 0 ||
		!strings.Contains(strings.Join(strings.Fields(r.stdout), " "), "presets/golangci-lint/"+presetFile+": the project's (its mark removed), so not compared") ||
		!strings.Contains(r.stdout, "0 preset files compared, none differs") {
		t.Errorf("the mark removed: exit %d, want 0, the copy named as the project's and not counted as compared\n%s%s", r.code, r.stdout, r.stderr)
	}
	hand := []string{"bind", "go.errors.2", "--tool", "lint-h", "--target", "config", "--key", "entry",
		"--value", "kept", "--severity", "warning", "--rung", "1", "--run-point", "commit", "--apply"}
	if r := run(t, dir, env, hand...); r.code != 4 {
		t.Fatalf("bind lint-h: exit %d, want 4, no plugin serves it\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := run(t, dir, env, "preset", "--check"); r.code != 4 || !strings.Contains(r.stdout, "lint-h") {
		t.Errorf("unlisted: exit %d, want 4 naming lint-h\n%s%s", r.code, r.stdout, r.stderr)
	}
	identity := read(t, dir, "rulebook.yaml") + "hand_written_presets: [lint-h]\n"
	if err := os.WriteFile(filepath.Join(dir, "rulebook.yaml"), []byte(identity), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := run(t, dir, env, "preset", "--check"); r.code != 0 || !strings.Contains(r.stdout, "lint-h: hand-written") {
		t.Errorf("listed: exit %d, want 0 naming lint-h as hand-written\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := run(t, dir, env, "preset", "--check", "--json"); r.code != 0 || !strings.Contains(r.stdout, `"hand_written": [
      "lint-h"
    ]`) {
		t.Errorf("listed, --json: exit %d, want 0 and hand_written [lint-h]\n%s%s\n%s", r.code, r.stdout, r.stderr, read(t, dir, "rulebook.yaml"))
	}
}
