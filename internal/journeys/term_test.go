package journeys

import (
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

func termNewArgs(slug, definition string, more ...string) []string {
	return append([]string{"term", "new", "--slug", slug, "--term-definition", definition}, more...)
}

func ruleUsing(statement string) []string {
	return []string{"rule", "new", "--concern", "go.errors", "--statement", statement, "--strength", "must",
		"--own", "--decision-title", "Say it plainly", "--decision-text", "A term keeps the statement neutral.", "--apply"}
}

// TestANewTermWithALiteralIsWrittenOnceAndAStatementMayUseIt: on the fixture,
// which has no terms.yaml, term new --apply makes the file from its head with
// the term and its literal and adds one history line; validate ends done; a
// rule whose statement writes {header} passes the prose lint; the same
// command again ends "nothing to do" and writes nothing more.
// Risk: a term written twice, a literal lost, or a defined term the lint
// still calls undefined.
func TestANewTermWithALiteralIsWrittenOnceAndAStatementMayUseIt(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	args := termNewArgs("header", "The name a request carries.", "--literal", "X-Request-Id", "--apply")
	if r := run(t, dir, env, args...); r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got, want := read(t, dir, "terms.yaml"), "format: 3\nterms:\n  - slug: header\n    definition: The name a request carries.\n    literal: X-Request-Id\n"; got != want {
		t.Errorf("terms.yaml:\n%s", got)
	}
	h := read(t, dir, "history.jsonl")
	if hist(t, dir) != 1 || !strings.Contains(h, `"entity":"term:header"`) || !strings.Contains(h, `"literal":"X-Request-Id"`) {
		t.Errorf("history:\n%s", h)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	if r := run(t, dir, env, ruleUsing("Send {header} on every call.")...); r.code != 0 {
		t.Fatalf("a rule that uses the new term: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate after a statement uses the term: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	before := read(t, dir, "history.jsonl")
	again := run(t, dir, env, args...)
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || read(t, dir, "history.jsonl") != before {
		t.Errorf("second run: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
}

// TestRetiringATermInUseIsRefusedNamingTheRulesAndAnUnusedOneIsRetired: the
// refusal ends refused (5) naming the rule that uses the term and writes
// nothing; an unused term retires, stays in the file with its status,
// validate ends done, a statement that uses it afterward is a finding, and
// retiring it again ends "nothing to do".
// Risk: a term in use removed from under a rule, or deleted from the file.
func TestRetiringATermInUseIsRefusedNamingTheRulesAndAnUnusedOneIsRetired(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	for _, a := range [][]string{
		append(termNewArgs("wrap", "Add what was being done to an error."), "--apply"),
		append(termNewArgs("spare", "Nothing will use it."), "--apply"),
		ruleUsing("Return an error and {wrap} it."),
	} {
		if r := run(t, dir, env, a...); r.code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", a, r.code, r.stdout, r.stderr)
		}
	}
	before, terms := hist(t, dir), read(t, dir, "terms.yaml")
	bad := run(t, dir, env, with(append([]string{"term", "retire", "wrap"}, why()...), "--apply")...)
	if bad.code != 5 || !strings.Contains(bad.stdout+bad.stderr, "go.errors.3") || !strings.Contains(bad.stdout+bad.stderr, "gragas rule replace") {
		t.Errorf("retire a term in use: exit %d, want refused (5) naming the rule\n%s%s", bad.code, bad.stdout, bad.stderr)
	}
	if hist(t, dir) != before || read(t, dir, "terms.yaml") != terms {
		t.Error("a refused retirement wrote")
	}
	if r := run(t, dir, env, with(append([]string{"term", "retire", "spare"}, why()...), "--apply")...); r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("retire an unused term: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got := read(t, dir, "terms.yaml"); !strings.Contains(got, "  - slug: spare\n    definition: Nothing will use it.\n    status: retired\n") {
		t.Errorf("the retired term is not kept with its status:\n%s", got)
	}
	if hist(t, dir) != before+1 || !strings.Contains(read(t, dir, "history.jsonl"), `"kind":"retire"`) {
		t.Errorf("history holds %d lines, want %d with a retire", hist(t, dir), before+1)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	again := run(t, dir, env, "term", "retire", "spare", "--decision", "decide-retire-spare", "--apply")
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || hist(t, dir) != before+1 {
		t.Errorf("retire twice: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	if r := run(t, dir, env, ruleUsing("Never mention {spare} here.")...); r.code != 0 {
		t.Fatalf("a rule that uses the retired term: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if v := run(t, dir, env, "validate"); v.code == 0 || !strings.Contains(v.stdout+v.stderr, "{spare}") {
		t.Errorf("a statement that uses a retired term passes validate, or does not name it: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
}

// TestATermEditChangesTheLiteralAndRecordsBothValues: edit sets, then takes
// away the literal, one history line each; the same value again is "nothing
// to do"; no field given is a usage error naming the flag.
func TestATermEditChangesTheLiteralAndRecordsBothValues(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	if r := run(t, dir, env, append(termNewArgs("gate", "A check that stops a push.", "--literal", "make check"), "--apply")...); r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := run(t, dir, env, "term", "edit", "gate", "--literal", "gate run", "--apply"); r.code != 0 {
		t.Fatalf("edit: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(read(t, dir, "terms.yaml"), "literal: gate run\n") || !strings.Contains(read(t, dir, "history.jsonl"), `"before":{"literal":"make check","status":"active"}`) {
		t.Errorf("terms.yaml:\n%s\nhistory:\n%s", read(t, dir, "terms.yaml"), read(t, dir, "history.jsonl"))
	}
	if again := run(t, dir, env, "term", "edit", "gate", "--literal", "gate run", "--apply"); again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || hist(t, dir) != 2 {
		t.Errorf("edit twice: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	if r := run(t, dir, env, "term", "edit", "gate", "--literal", "", "--apply"); r.code != 0 || strings.Contains(read(t, dir, "terms.yaml"), "literal") || hist(t, dir) != 3 {
		t.Errorf("take the literal away: exit %d\n%s%s\n%s", r.code, r.stdout, r.stderr, read(t, dir, "terms.yaml"))
	}
	if r := run(t, dir, env, "term", "edit", "gate", "--apply"); r.code == 0 {
		t.Errorf("an edit that names no field ended done:\n%s%s", r.stdout, r.stderr)
	}
}

// TestTheDryRunsOfTheTermWordsAreGoldensAtThreeWidths: the plan of each word off
// a terminal, its footer and "declined" with exit 6, and the refusal of a
// term in use with exit 5, against goldens written only with -update by a
// person who reads the diff.
func TestTheDryRunsOfTheTermWordsAreGoldensAtThreeWidths(t *testing.T) {
	t.Parallel()
	gate := append(termNewArgs("gate", "A check that stops a push.", "--literal", "make check"), "--apply")
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			code int
			args []string
			prep [][]string
		}{
			{"term-new-declined", 6, termNewArgs("header", "The name a request carries.", "--literal", "X-Request-Id"), nil},
			{"term-edit-declined", 6, []string{"term", "edit", "gate", "--literal", "gate run"}, [][]string{gate}},
			{"term-retire-declined", 6, with([]string{"term", "retire", "gate"}, why()...), [][]string{gate}},
			{"term-retire-refused", 5, with([]string{"term", "retire", "gate"}, why()...), [][]string{gate, ruleUsing("Run the {gate} before a push.")}},
			{"term-retire-undecided", 5, []string{"term", "retire", "gate"}, [][]string{gate}},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				dir, env := historyRepo(t, ""), ruleEnv(t)
				env["COLUMNS"] = cols
				for _, p := range tc.prep {
					if r := run(t, dir, env, p...); r.code != 0 {
						t.Fatalf("set-up %v: exit %d\n%s%s", p, r.code, r.stdout, r.stderr)
					}
				}
				r := run(t, dir, env, tc.args...)
				if r.code != tc.code {
					t.Fatalf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
				}
				uitest.Golden(t, tc.name+"-"+cols, "$ gragas "+strings.Join(tc.args, " ")+"\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr)
			})
		}
	}
}
