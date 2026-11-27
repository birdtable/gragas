package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parentRelease is where the release of fixture-base's parent, fixture-parent
// 0.9.0, is read in the cache folder: coreapi.ParentBundle of the pin, the
// rulebook's folder the first sixteen hex digits of its digest. It is written
// out here so a move of M2's placement is seen; coreapi's tests hold the
// function to it.
const parentRelease = "releases/pinned/932251d4ef17cc0b/0.9.0/bundle.json"

// overlayRepo is fixture-base, an overlay of fixture-parent 0.9.0 that
// excludes std.errors.2 and has lifted its exclusion of std.errors.3.
func overlayRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyTree(t, filepath.Join(moduleRoot(), "internal/host/assets/fixture/base"), dir)
	return dir
}

// fetchParent puts the parent stand-in in the cache at the pin, as M5's
// fetch will put the release.
func fetchParent(t *testing.T, env map[string]string) {
	t.Helper()
	copyFile(t, filepath.Join(moduleRoot(), "internal/core/api/testdata/parent/bundle.json"), filepath.Join(appFolders(t, env).Cache, parentRelease))
}

// lastEnding is the closing word the run log holds for the last run.
func lastEnding(t *testing.T, env map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(appFolders(t, env).State, "runs.jsonl"))
	if err != nil {
		t.Fatalf("no run record: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var r record
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &r); err != nil {
		t.Fatal(err)
	}
	return r.Ending
}

func decided(title string) []string {
	return []string{"--decision-title", title, "--decision-text", "The house reads the parent's rule this way."}
}

// TestExcludeAndIncludeOverTheParentStandIn is the done-when of exclude and include on
// the binary: without the parent in the cache the run is not ready naming the
// pin; with it, exclude writes the record and one history line, again is
// nothing to do, include lifts it and keeps the record, include of a rule not
// excluded is usage and of one the parent retired is not ready. Each ending
// is in the run log. Risk: an overlay that drops a parent's rule with no
// record, or a lift that deletes one.
func TestExcludeAndIncludeOverTheParentStandIn(t *testing.T) {
	t.Parallel()
	dir, env := overlayRepo(t), ruleEnv(t)
	env["COLUMNS"] = "80"
	exclusions, was := read(t, dir, "exclusions.yaml"), hist(t, dir)
	step := func(code int, word string, args ...string) result {
		t.Helper()
		r := run(t, dir, env, append(args, "--apply")...)
		if r.code != code || !strings.Contains(r.stdout, word) {
			t.Fatalf("%v: exit %d, want %d and %q\n%s%s", args, r.code, code, word, r.stdout, r.stderr)
		}
		if got := lastEnding(t, env); got != word {
			t.Fatalf("%v: the run log ends %q, want %q", args, got, word)
		}
		return r
	}

	r := step(4, "not ready", with([]string{"rule", "exclude", "std.errors.4"}, decided("Not for this house")...)...)
	if !strings.Contains(flat(r.stdout), "not ready parent not fetched: fixture-parent 0.9.0 is not in the releases cache") || !strings.Contains(r.stdout, "gragas validate") || read(t, dir, "exclusions.yaml") != exclusions || hist(t, dir) != was {
		t.Fatalf("not fetched:\n%s", r.stdout)
	}

	fetchParent(t, env)
	step(0, "done", with([]string{"rule", "exclude", "std.errors.4"}, decided("Not for this house")...)...)
	ex := read(t, dir, "exclusions.yaml")
	if !strings.HasPrefix(ex, exclusions) || !strings.Contains(ex[len(exclusions):], "  - overlay: fixture-base\n    rule: std.errors.4\n    number: 1\n    decision: decide-exclude-std-errors-4\n") {
		t.Errorf("exclusions.yaml:\n%s", ex)
	}
	h := read(t, dir, "history.jsonl")
	if hist(t, dir) != was+1 || !strings.Contains(h, `"entity":"exclusion:fixture-base/std.errors.4/1"`) || !strings.Contains(h, `"kind":"exclude"`) {
		t.Errorf("history:\n%s", h)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate after exclude: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}

	again := step(0, "done", "rule", "exclude", "std.errors.4", "--decision", "decide-exclude-std-errors-4")
	if !strings.Contains(again.stdout, "nothing to do") || read(t, dir, "exclusions.yaml") != ex || hist(t, dir) != was+1 {
		t.Errorf("exclude twice:\n%s", again.stdout)
	}

	step(0, "done", with([]string{"rule", "exclude", "std.errors.3"}, decided("Not again")...)...)
	if !strings.Contains(read(t, dir, "exclusions.yaml"), "rule: std.errors.3\n    number: 2\n") {
		t.Error("an exclusion after a lifted one does not take the next number")
	}

	ex = read(t, dir, "exclusions.yaml")
	step(0, "done", with([]string{"rule", "include", "std.errors.4"}, decided("Back in this house")...)...)
	lifted := read(t, dir, "exclusions.yaml")
	if len(lifted) <= len(ex) || !strings.Contains(lifted, "    lifted:\n      decision: decide-include-std-errors-4\n") || !strings.Contains(lifted, "rule: std.errors.4\n    number: 1\n    decision: decide-exclude-std-errors-4\n") {
		t.Errorf("include did not keep the record:\n%s", lifted)
	}
	h = read(t, dir, "history.jsonl")
	if hist(t, dir) != was+3 || !strings.Contains(h, `"kind":"include"`) {
		t.Errorf("history:\n%s", h)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate after include: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}

	step(2, "usage", with([]string{"rule", "include", "std.errors.4"}, "--decision", "decide-include-std-errors-4")...)
	step(0, "done", with([]string{"rule", "exclude", "std.errors.5"}, decided("Gone upstream")...)...)
	r = step(4, "not ready", with([]string{"rule", "include", "std.errors.5"}, decided("Want it back")...)...)
	if !strings.Contains(flat(r.stdout), "retired in fixture-parent 0.9.0") {
		t.Errorf("include of a retired rule:\n%s", r.stdout)
	}
	step(5, "refused", with([]string{"rule", "exclude", "std.errors.9"}, decided("No such rule")...)...)
	if hist(t, dir) != was+4 {
		t.Errorf("history holds %d lines after the refusals, want %d", hist(t, dir), was+4)
	}
}

// TestExcludeInABaseIsRefused: a base has no parent to drop a rule of.
func TestExcludeInABaseIsRefused(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	r := run(t, dir, env, with([]string{"rule", "exclude", "std.errors.1"}, append(decided("Not here"), "--apply")...)...)
	if r.code != 5 || !strings.Contains(r.stdout, "refused") || !strings.Contains(flat(r.stdout), "only an overlay can exclude") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got := lastEnding(t, env); got != "refused" {
		t.Errorf("the run log ends %q", got)
	}
}

// TestTwoBranchesThatTookOneNumberAreListedByValidate: the merge of two
// branches that each excluded a rule holds its key twice; validate lists the
// second record with the line of the first.
func TestTwoBranchesThatTookOneNumberAreListedByValidate(t *testing.T) {
	t.Parallel()
	dir, env := overlayRepo(t), ruleEnv(t)
	second := "  - overlay: fixture-base\n    rule: std.errors.2\n    number: 1\n    decision: house-call\n    who: alex\n    when: \"2026-10-23T09:00:00Z\"\n"
	if err := os.WriteFile(filepath.Join(dir, "exclusions.yaml"), []byte(read(t, dir, "exclusions.yaml")+second), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, dir, env, "validate")
	if r.code != 1 || !strings.Contains(r.stdout, "exclusions.yaml:19  the exclusion fixture-base/std.errors.2/1 is written twice\n") || !strings.Contains(r.stdout, "(first on line 3)") {
		t.Errorf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// flat is s with every run of spaces and line breaks one space, so a check
// reads a message whatever the width wrapped it at.
func flat(s string) string { return strings.Join(strings.Fields(s), " ") }
