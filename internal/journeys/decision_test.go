package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

const wrapDecisions = `format: 3
decisions:
  - slug: wrap
    title: Wrap
    text: Wrap errors.
    rejected:
      - bare
    who: Ada
    date: "2026-01-01"
`

// decisionRepo is the promote book with a decision, "wrap", that go.errors.1
// and go.errors.2 stand on.
func decisionRepo(t *testing.T) string {
	t.Helper()
	dir := promoteRepo(t)
	rules := strings.Replace(promoteRules, "    pairs:\n", "    reasoning:\n      decision: wrap\n    pairs:\n", 1)
	rules = strings.Replace(rules, "    strength: may\n", "    strength: may\n    reasoning:\n      decision: wrap\n", 1)
	if rules == promoteRules {
		t.Fatal("the fixture's rules took no decision")
	}
	for name, body := range map[string]string{"language/go/rules/errors.yaml": rules, "decisions.yaml": wrapDecisions} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func supersedeArgs(more ...string) []string {
	return append([]string{"decision", "supersede", "wrap", "--decision-slug", "wrap-with-context", "--title", "Wrap with context",
		"--text", "Say what was being done.", "--rejected", "the bare cause: it loses where it failed"}, more...)
}

// TestSupersedingADecisionTwoRulesCiteMarksTheOldAndMovesBothInOnePlan: on a
// book where two rules stand on "wrap", decision supersede --apply marks the
// old record superseded and keeps it, writes the new one naming it, moves both
// rules to the new one and adds a history line for each of the two decisions
// and for each rule, all one plan; history <id> shows the move on a rule, and
// validate ends done. The same word again ends "nothing to do". A supersede of
// the old one by a third is refused, naming the head of the chain.
// Planted failure: the old record is not marked, so the link has one side.
func TestSupersedingADecisionTwoRulesCiteMarksTheOldAndMovesBothInOnePlan(t *testing.T) {
	t.Parallel()
	dir, env := decisionRepo(t), ruleEnv(t)
	const before = 0 // the book has no history yet: the supersession starts it
	if r := run(t, dir, env, supersedeArgs("--apply")...); r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	decisions := read(t, dir, "decisions.yaml")
	for _, want := range []string{"  - slug: wrap\n", "superseded_by: wrap-with-context", "  - slug: wrap-with-context\n", "supersedes: wrap\n", "      rules:\n        - go.errors.1\n        - go.errors.2\n"} {
		if !strings.Contains(decisions, want) {
			t.Errorf("decisions.yaml lacks %q:\n%s", want, decisions)
		}
	}
	rules := read(t, dir, "language/go/rules/errors.yaml")
	if strings.Count(rules, "decision: wrap-with-context") != 2 || strings.Contains(rules, "decision: wrap\n") {
		t.Errorf("the two rules do not both stand on the new decision:\n%s", rules)
	}
	lines := strings.Split(strings.TrimSpace(read(t, dir, "history.jsonl")), "\n")
	if len(lines) != before+4 {
		t.Fatalf("history holds %d lines, want %d: the two decisions and two rules\n%s", len(lines), before+4, strings.Join(lines, "\n"))
	}
	plan := between(lines[before], `"plan":"`, `"`)
	for i, want := range []string{`"entity":"decision:wrap"`, `"entity":"decision:wrap-with-context"`, `"entity":"rule:go.errors.1"`, `"entity":"rule:go.errors.2"`} {
		if l := lines[before+i]; !strings.Contains(l, want) || between(l, `"plan":"`, `"`) != plan {
			t.Errorf("line %d, want %s in plan %s:\n%s", i, want, plan, l)
		}
	}
	if h := run(t, dir, env, "history", "go.errors.1"); h.code != 0 || !strings.Contains(h.stdout, "decision-superseded") {
		t.Errorf("history go.errors.1: exit %d\n%s%s", h.code, h.stdout, h.stderr)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	if again := run(t, dir, env, supersedeArgs("--apply")...); again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || hist(t, dir) != before+4 {
		t.Errorf("second run: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	third := run(t, dir, env, "decision", "supersede", "wrap", "--decision-slug", "wrap-again", "--title", "Again", "--text", "Again.", "--rejected", "x: y", "--apply")
	said := strings.Join(strings.Fields(third.stdout+third.stderr), " ") // the terminal wraps a long line
	if third.code != 5 || !strings.Contains(said, "the head of its chain is wrap-with-context") || !strings.Contains(said, "gragas decision supersede wrap-with-context") {
		t.Errorf("a supersede of a superseded decision: exit %d, want refused (5) naming the head\n%s%s", third.code, third.stdout, third.stderr)
	}
	if hist(t, dir) != before+4 {
		t.Error("a refused supersede wrote to the history")
	}
}

// TestANewDecisionNeedsARejectedOptionAndTwiceHasNothingToDo: decision new
// without --rejected, off a terminal, ends usage naming the flag; with it the
// decision is written and the same words again end "nothing to do".
func TestANewDecisionNeedsARejectedOptionAndTwiceHasNothingToDo(t *testing.T) {
	t.Parallel()
	dir, env := decisionRepo(t), ruleEnv(t)
	base := []string{"decision", "new", "--decision-slug", "small", "--title", "Small", "--text", "Keep it small."}
	if r := run(t, dir, env, with(base, "--apply")...); r.code != 2 || !strings.Contains(r.stdout+r.stderr, "--rejected") {
		t.Fatalf("no rejected option: exit %d, want usage (2) naming --rejected\n%s%s", r.code, r.stdout, r.stderr)
	}
	full := with(base, "--rejected", "big: slow", "--cite-source", "effective-go", "--cite-rule", "go.errors.3", "--apply")
	if r := run(t, dir, env, full...); r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got := read(t, dir, "decisions.yaml"); !strings.Contains(got, "  - slug: small\n") || !strings.Contains(got, "      - \"big: slow\"\n") {
		t.Errorf("decisions.yaml:\n%s", got)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	if again := run(t, dir, env, full...); again.code != 0 || !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("second run: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
}

// TestTheDryRunsOfTheDecisionWordsAreGoldensAtThreeWidths: the plan of each
// word off a terminal, its footer and "declined" with exit 6, against goldens
// written only with -update by a person who reads the diff.
func TestTheDryRunsOfTheDecisionWordsAreGoldensAtThreeWidths(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			args []string
		}{
			{"decision-new-declined", []string{"decision", "new", "--decision-slug", "small", "--title", "Small", "--text", "Keep it small.", "--rejected", "big: slow", "--cite-source", "effective-go"}},
			{"decision-supersede-declined", supersedeArgs()},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				t.Parallel()
				dir, env := decisionRepo(t), ruleEnv(t)
				env["COLUMNS"] = cols
				r := run(t, dir, env, tc.args...)
				if r.code != 6 {
					t.Fatalf("exit %d, want 6\n%s%s", r.code, r.stdout, r.stderr)
				}
				uitest.Golden(t, tc.name+"-"+cols, "$ gragas "+strings.Join(tc.args, " ")+"\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr)
			})
		}
	}
}
