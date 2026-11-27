package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"

	"github.com/birdtable/go-files/filestest"
)

// ruleEnv is the isolated environment of a rule command: fresh state folders
// and the account's git identity the history line names.
func ruleEnv(t *testing.T) map[string]string {
	t.Helper()
	env, _ := folders(t)
	git := journeyGit(t)
	home := filestest.Private(t)
	settings := filepath.Join(home, "settings.yaml")
	files := map[string]string{
		filepath.Join(home, ".gitconfig"): "[user]\n\tname = Test Person\n\temail = test@example.test\n",
		settings:                          "version: 1\nprograms: {git: " + git + "}\n",
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env[homeKey], env["GRAGAS_CONFIG"] = home, settings
	return env
}

func newArgs() []string {
	return []string{"rule", "new", "--concern", "go.errors", "--statement", "Return errors, do not panic.",
		"--strength", "must", "--own", "--decision-title", "Errors are returned", "--decision-text", "Panics hide failures from the caller."}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRuleNewApplyWritesTheRuleItsSourceAndDecisionAndOneHistoryLine: with the
// flags and --apply the rule is written, an own-decision source and a decision
// are recorded and the history gains one line; validate then ends done; the
// same command again ends "nothing to do" and writes nothing more.
// Risk: a rule written without the record that lets it be cited, or twice.
func TestRuleNewApplyWritesTheRuleItsSourceAndDecisionAndOneHistoryLine(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	env["COLUMNS"] = "80"
	r := run(t, dir, env, append(newArgs(), "--apply")...)
	if r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	rules := read(t, dir, "language/go/rules/errors.yaml")
	for _, want := range []string{"Return errors, do not panic.", "go.errors.3", "status: proposed"} {
		if !strings.Contains(rules, want) {
			t.Errorf("errors.yaml lacks %q:\n%s", want, rules)
		}
	}
	if !strings.Contains(read(t, dir, "sources.yaml"), "own-decision") || !strings.Contains(read(t, dir, "decisions.yaml"), "Panics hide failures") {
		t.Error("the own decision's source and the decision were not recorded")
	}
	if n := strings.Count(strings.TrimSpace(read(t, dir, "history.jsonl")), "\n") + 1; n != 1 {
		t.Errorf("history holds %d lines, want 1", n)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	before := read(t, dir, "history.jsonl")
	again := run(t, dir, env, append(newArgs(), "--apply")...)
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("second run: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	if read(t, dir, "history.jsonl") != before {
		t.Error("the second run wrote a history line")
	}
}

// TestRuleNewOffATerminalWithoutApplyPrintsThePlanAndEndsDeclined: the plan,
// the footer naming --apply, "declined" and exit 6, and nothing written.
func TestRuleNewOffATerminalWithoutApplyPrintsThePlanAndEndsDeclined(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		t.Run(cols, func(t *testing.T) {
			dir, env := historyRepo(t, ""), ruleEnv(t)
			env["COLUMNS"] = cols
			r := run(t, dir, env, newArgs()...)
			if r.code != 6 {
				t.Fatalf("exit %d, want 6\n%s%s", r.code, r.stdout, r.stderr)
			}
			if strings.Contains(read(t, dir, "language/go/rules/errors.yaml"), "do not panic") {
				t.Error("a dry run wrote the rule")
			}
			uitest.Golden(t, "rule-new-declined-"+cols, "$ gragas "+strings.Join(newArgs(), " ")+"\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr)
		})
	}
}

// TestRuleNewWithoutAFlagOffATerminalIsAUsageError: a missing answer names
// the flag that gives it and nothing is asked of nobody.
func TestRuleNewWithoutAFlagOffATerminalIsAUsageError(t *testing.T) {
	t.Parallel()
	r := run(t, historyRepo(t, ""), ruleEnv(t), "rule", "new", "--concern", "go.errors")
	if r.code != 2 || !strings.Contains(r.stdout+r.stderr, "--statement") {
		t.Errorf("exit %d, want 2 naming --statement\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestAHandEditOfALifecycleFieldAfterRuleNewIsNotUndone: an owner written into
// the file by hand survives the next run of the same command.
func TestAHandEditOfALifecycleFieldAfterRuleNewIsNotUndone(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	if r := run(t, dir, env, append(newArgs(), "--apply")...); r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	p := filepath.Join(dir, "language/go/rules/errors.yaml")
	edited := strings.Replace(read(t, dir, "language/go/rules/errors.yaml"), "status: proposed\n    origin: authored\n    statement: Return", "status: proposed\n    owner: team-a\n    origin: authored\n    statement: Return", 1)
	if !strings.Contains(edited, "owner: team-a") {
		edited = strings.Replace(read(t, dir, "language/go/rules/errors.yaml"), "strength: must\n    status: proposed", "strength: must\n    owner: team-a\n    status: proposed", 2)
	}
	if err := os.WriteFile(p, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, dir, env, append(newArgs(), "--apply")...); r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(read(t, dir, "language/go/rules/errors.yaml"), "owner: team-a") {
		t.Error("the hand edit was undone")
	}
}

func why() []string {
	return []string{"--decision-title", "It no longer holds", "--decision-text", "The rule says what the code stopped doing."}
}

func with(args []string, more ...string) []string {
	return append(append([]string{}, args...), more...)
}

func hist(t *testing.T, dir string) int {
	t.Helper()
	b := strings.TrimSpace(read(t, dir, "history.jsonl"))
	if b == "" {
		return 0
	}
	return strings.Count(b, "\n") + 1
}

// TestRetireRestoreAndReplaceEachAddOneHistoryLineAndValidateEndsDone: on the
// fixture, each word applied adds exactly one history line and validate
// ends done after it; retire twice ends "nothing to do"; restore of an
// active rule is refused naming the field; the restore returns the rule as
// active and the history keeps the retire and the restore both.
// Risk: a rule deleted, a line missing or doubled, or a restore of a live rule.
func TestRetireRestoreAndReplaceEachAddOneHistoryLineAndValidateEndsDone(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	step := func(want int, args ...string) result {
		t.Helper()
		r := run(t, dir, env, args...)
		if r.code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", args, r.code, r.stdout, r.stderr)
		}
		if got := hist(t, dir); got != want {
			t.Fatalf("%v: history holds %d lines, want %d", args, got, want)
		}
		if v := run(t, dir, env, "validate"); v.code != 0 {
			t.Fatalf("validate after %v: exit %d\n%s%s", args, v.code, v.stdout, v.stderr)
		}
		return r
	}
	step(1, with([]string{"rule", "retire", "go.errors.2"}, append(why(), "--apply")...)...)
	if !strings.Contains(read(t, dir, "language/go/rules/errors.yaml"), "status: retired") {
		t.Error("the rule is not retired")
	}
	again := step(1, with([]string{"rule", "retire", "go.errors.2"}, "--decision", "decide-retire-go-errors-2", "--apply")...)
	if !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("retire twice:\n%s", again.stdout)
	}
	step(2, "rule", "restore", "go.errors.2", "--review-by", "2027-06-30", "--decision-title", "Back", "--decision-text", "It holds again.", "--apply")
	if !strings.Contains(read(t, dir, "language/go/rules/errors.yaml"), "status: active") {
		t.Error("the restored rule is not active")
	}
	h := read(t, dir, "history.jsonl")
	if !strings.Contains(h, `"kind":"retire"`) || !strings.Contains(h, `"kind":"restore"`) {
		t.Errorf("the history lacks the retire or the restore:\n%s", h)
	}
	bad := run(t, dir, env, "rule", "restore", "go.errors.2", "--review-by", "2027-06-30", "--decision", "decide-retire-go-errors-2", "--apply")
	if bad.code != 5 || !strings.Contains(bad.stdout+bad.stderr, "status") {
		t.Errorf("restore of an active rule: exit %d, want refused (5) naming the status\n%s%s", bad.code, bad.stdout, bad.stderr)
	}
	step(3, "rule", "replace", "go.errors.2", "--statement", "Wrap an error with the call that failed.", "--strength", "must",
		"--decision-title", "Say more", "--decision-text", "The old words were too thin.", "--apply")
	rules := read(t, dir, "language/go/rules/errors.yaml")
	if !strings.Contains(rules, "replaced_by: go.errors.3") || !strings.Contains(rules, "id: go.errors.2") {
		t.Errorf("replace did not keep the old rule or point it at the new:\n%s", rules)
	}
}

// TestTheDryRunsOfRetireRestoreAndReplaceAreGoldensAtThreeWidths: the plan of
// each word off a terminal, its footer and "declined" with exit 6, against a
// golden written only with -update by a person who reads the diff.
func TestTheDryRunsOfRetireRestoreAndReplaceAreGoldensAtThreeWidths(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			args []string
			prep []string
		}{
			{"rule-retire-declined", with([]string{"rule", "retire", "go.errors.2"}, why()...), nil},
			{"rule-restore-declined", []string{"rule", "restore", "go.errors.2", "--review-by", "2027-06-30", "--decision-title", "Back", "--decision-text", "It holds again."}, with([]string{"rule", "retire", "go.errors.2"}, append(why(), "--apply")...)},
			{"rule-replace-declined", []string{"rule", "replace", "go.errors.2", "--statement", "Wrap an error with the call that failed.", "--strength", "must", "--decision-title", "Say more", "--decision-text", "The old words were too thin."}, nil},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				dir, env := historyRepo(t, ""), ruleEnv(t)
				env["COLUMNS"] = cols
				if tc.prep != nil {
					if p := run(t, dir, env, tc.prep...); p.code != 0 {
						t.Fatalf("set-up: exit %d\n%s%s", p.code, p.stdout, p.stderr)
					}
				}
				r := run(t, dir, env, tc.args...)
				if r.code != 6 {
					t.Fatalf("exit %d, want 6\n%s%s", r.code, r.stdout, r.stderr)
				}
				uitest.Golden(t, tc.name+"-"+cols, "$ gragas "+strings.Join(tc.args, " ")+"\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr)
			})
		}
	}
}

// TestRuleEditCiteAppendsTheSourceAsARecordOnceAndValidateEndsDone: on the
// fixture, with a source in the book, --cite --apply puts one record that
// backs the rule under the rule's sources and one history line; validate ends
// done after it; the same command again ends "nothing to do"; a source the book
// lacks is refused naming it.
// Risk: a citation written as text the schema refuses, or written twice.
func TestRuleEditCiteAppendsTheSourceAsARecordOnceAndValidateEndsDone(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	if r := run(t, dir, env, append(newArgs(), "--apply")...); r.code != 0 {
		t.Fatalf("rule new: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	slug := ""
	for l := range strings.SplitSeq(read(t, dir, "sources.yaml"), "\n") {
		if s, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- ")), "slug: "); ok {
			slug = s
		}
	}
	if slug == "" {
		t.Fatalf("no source slug in\n%s", read(t, dir, "sources.yaml"))
	}
	before := hist(t, dir)
	r := run(t, dir, env, "rule", "edit", "go.errors.2", "--cite", slug, "--apply")
	if r.code != 0 {
		t.Fatalf("rule edit --cite: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	rules := read(t, dir, "language/go/rules/errors.yaml")
	if want := "    sources:\n      - ref: " + slug + "\n        backs: rule\n"; !strings.Contains(rules, want) {
		t.Errorf("the rule lacks %q:\n%s", want, rules)
	}
	if got := hist(t, dir); got != before+1 {
		t.Errorf("history holds %d lines, want %d", got, before+1)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	again := run(t, dir, env, "rule", "edit", "go.errors.2", "--cite", slug, "--apply")
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || hist(t, dir) != before+1 {
		t.Errorf("second run: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	bad := run(t, dir, env, "rule", "edit", "go.errors.2", "--cite", "no-such-source", "--apply")
	if bad.code != 5 || !strings.Contains(bad.stdout+bad.stderr, "no-such-source") {
		t.Errorf("a source the book lacks: exit %d, want refused (5) naming it\n%s%s", bad.code, bad.stdout, bad.stderr)
	}
}
