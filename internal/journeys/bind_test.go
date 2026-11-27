package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

func bindArgs(more ...string) []string {
	return append([]string{"bind", "go.errors.2", "--tool", "golangci-lint", "--target", "source", "--key", "errorlint",
		"--value", "true", "--severity", "error", "--rung", "1", "--run-point", "commit"}, more...)
}

// TestBindUnbindWriteOneRecordAndOneHistoryLineEachAndTheSecondRunHasNothingToDo:
// bind --apply adds the binding to the rule's file and one history line, the
// same words again end "nothing to do", a bind of the same entry with another
// severity is refused naming rebind, rebind moves it with a history line, and
// unbind takes it out; validate ends done after each. Risk: a binding written
// twice, a refused run that wrote, or an unbind that leaves the record.
func TestBindUnbindWriteOneRecordAndOneHistoryLineEachAndTheSecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	dir, env := bindRepo(t), ruleEnv(t)
	servedBy(t, dir, env)
	const before = 0 // the book has no history yet
	if r := run(t, dir, env, bindArgs("--apply")...); r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("bind: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if rules := read(t, dir, "language/go/rules/errors.yaml"); strings.Count(rules, "key: errorlint") != 1 || !strings.Contains(rules, "        value: true\n") {
		t.Errorf("the binding is not written once, value bare:\n%s", rules)
	}
	if hist(t, dir) != before+1 {
		t.Errorf("history holds %d lines, want %d", hist(t, dir), before+1)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	if again := run(t, dir, env, bindArgs("--apply")...); again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || hist(t, dir) != before+1 {
		t.Errorf("second bind: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	other := run(t, dir, env, "bind", "go.errors.2", "--tool", "golangci-lint", "--target", "source", "--key", "errorlint", "--severity", "warning", "--rung", "1", "--run-point", "commit", "--apply")
	if said := strings.Join(strings.Fields(other.stdout+other.stderr), " "); other.code != 5 || !strings.Contains(said, "gragas rebind go.errors.2") {
		t.Errorf("a bind with other fields: exit %d, want refused (5) naming rebind\n%s%s", other.code, other.stdout, other.stderr)
	}
	move := []string{"rebind", "go.errors.2", "--tool", "golangci-lint", "--target", "source", "--key", "errorlint", "--to-key", "wrapcheck", "--apply"}
	if r := run(t, dir, env, move...); r.code != 5 || hist(t, dir) != before+1 {
		t.Fatalf("a move with no decision: exit %d, history %d, want refused (5) and nothing written\n%s%s", r.code, hist(t, dir), r.stdout, r.stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "decisions.yaml"), []byte(wrapDecisions), 0o644); err != nil {
		t.Fatal(err)
	}
	move = append(move, "--decision", "wrap")
	if r := run(t, dir, env, move...); r.code != 0 || hist(t, dir) <= before+1 {
		t.Fatalf("a move on a decision: exit %d, history %d\n%s%s", r.code, hist(t, dir), r.stdout, r.stderr)
	}
	moved := hist(t, dir)
	if r := run(t, dir, env, "unbind", "go.errors.2", "--tool", "golangci-lint", "--target", "source", "--key", "wrapcheck", "--apply"); r.code != 0 || hist(t, dir) != moved+1 {
		t.Fatalf("unbind: exit %d, history %d\n%s%s", r.code, hist(t, dir), r.stdout, r.stderr)
	}
	if rules := read(t, dir, "language/go/rules/errors.yaml"); strings.Contains(rules, "bindings:") && strings.Contains(rules, "key: wrapcheck") {
		t.Errorf("the binding is still in the file:\n%s", rules)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
}

// TestTheDryRunsOfTheBindWordsAreGoldensAtThreeWidths: the plan of bind off a
// terminal, with its tool entries, its standing and its proof, its footer and
// "declined" with exit 6, against goldens written only with -update by a person
// who reads the diff.
func TestTheDryRunsOfTheBindWordsAreGoldensAtThreeWidths(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		t.Run("bind-declined-"+cols, func(t *testing.T) {
			t.Parallel()
			dir, env := bindRepo(t), ruleEnv(t)
			servedBy(t, dir, env)
			env["COLUMNS"] = cols
			args := bindArgs()
			r := run(t, dir, env, args...)
			if r.code != 6 {
				t.Fatalf("exit %d, want 6\n%s%s", r.code, r.stdout, r.stderr)
			}
			uitest.Golden(t, "bind-declined-"+cols, "$ gragas "+strings.Join(args, " ")+"\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr)
		})
	}
}

// bindRepo is the promote book with go.errors.2 active, since a proposed rule
// is in no preset and a binding of it moves no entry.
func bindRepo(t *testing.T) string {
	t.Helper()
	dir := promoteRepo(t)
	rules := strings.Replace(promoteRules, "    strength: may\n    status: proposed\n", "    strength: may\n    status: active\n", 1)
	if rules == promoteRules {
		t.Fatal("the fixture's may rule was not made active")
	}
	if err := os.WriteFile(filepath.Join(dir, "language", "go", "rules", "errors.yaml"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestBindWritesOptionsAsAMappingAndALaterBindOfThemHasNothingToDo: bind
// --option writes the options under the binding in the rule's file, in the
// order of their names, the file still validates, the same words again end
// "nothing to do", and unbind takes the record with its option lines out.
// Risk: options dropped, written flat, or left behind by an unbind.
func TestBindWritesOptionsAsAMappingAndALaterBindOfThemHasNothingToDo(t *testing.T) {
	t.Parallel()
	dir, env := bindRepo(t), ruleEnv(t)
	args := bindArgs("--option", "skip=a", "--option", "skip=b", "--option", "max=3")
	// no plugin writes golangci-lint's preset here, so the apply
	// writes the record and ends not ready; the stand-in takes no options
	if r := run(t, dir, env, append(args, "--apply")...); r.code != 4 || !strings.Contains(r.stdout, "not ready") {
		t.Fatalf("bind: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if rules := read(t, dir, "language/go/rules/errors.yaml"); !strings.Contains(rules, "        options:\n          max: 3\n          skip: [a, b]\n") {
		t.Errorf("the options are not written as a mapping under the binding:\n%s", rules)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	uitest.Golden(t, "bind-options-rules", "--- language/go/rules/errors.yaml after bind --option\n"+read(t, dir, "language/go/rules/errors.yaml"))
	if again := run(t, dir, env, append(args, "--apply")...); again.code != 0 || !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("second bind: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	if r := run(t, dir, env, bindArgs("--option", "max=4", "--apply")...); r.code != 5 {
		t.Errorf("a bind with other options: exit %d, want refused (5)\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := run(t, dir, env, "unbind", "go.errors.2", "--tool", "golangci-lint", "--target", "source", "--key", "errorlint", "--apply"); r.code != 0 {
		t.Fatalf("unbind: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if rules := read(t, dir, "language/go/rules/errors.yaml"); strings.Contains(rules, "options:") {
		t.Errorf("the options are still in the file:\n%s", rules)
	}
	uitest.Golden(t, "bind-options-unbound", "--- language/go/rules/errors.yaml after unbind\n"+read(t, dir, "language/go/rules/errors.yaml"))
}

// TestASeverityOnlyRebindKeepsTheOptionsWrittenByHand: a binding
// with nested options written into the rule's file by hand is rebound to a
// stricter severity and succeeds, the file changes on the severity's line only,
// and validate ends done.
// Risk: a rebind that refuses what it does not change, or that rewrites the
// record and loses the options or the comment beside them.
func TestASeverityOnlyRebindKeepsTheOptionsWrittenByHand(t *testing.T) {
	t.Parallel()
	dir, env := bindRepo(t), ruleEnv(t)
	bind := bindArgs("--apply")
	for i, a := range bind {
		if a == "error" {
			bind[i] = "warning"
		}
	}
	// no plugin writes golangci-lint's preset here: the record is
	// written and the run ends not ready (4)
	if r := run(t, dir, env, bind...); r.code != 4 {
		t.Fatalf("bind: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	file := filepath.Join(dir, "language", "go", "rules", "errors.yaml")
	const nested = "        options:\n          # kept as written\n          ignore:\n            tests: [a, b]\n          depth: 2\n"
	rules := read(t, dir, "language/go/rules/errors.yaml")
	if !strings.Contains(rules, "        value: true\n") {
		t.Fatalf("the binding is not in the file:\n%s", rules)
	}
	planted := strings.Replace(rules, "        value: true\n", "        value: true\n"+nested, 1)
	if err := os.WriteFile(file, []byte(planted), 0o644); err != nil {
		t.Fatal(err)
	}
	rebind := []string{"rebind", "go.errors.2", "--tool", "golangci-lint", "--target", "source", "--key", "errorlint", "--severity", "error", "--apply"}
	if r := run(t, dir, env, rebind...); r.code != 4 {
		t.Fatalf("rebind: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	want := strings.Replace(planted, "severity: warning\n", "severity: error\n", 1)
	if got := read(t, dir, "language/go/rules/errors.yaml"); got != want {
		t.Errorf("the file after the rebind:\n%s\nwant:\n%s", got, want)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
}
