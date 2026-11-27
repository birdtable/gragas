package journeys

import (
	"regexp"
	"strings"
	"testing"
)

// digestRef is the digest the dry run's closing line names the entry by.
var digestRef = regexp.MustCompile(`sha256:[0-9a-f]{64}`)

// TestUndoTakesBackThePromoteThenConfirmAndTheNextOneEndsNotReady: a may goes
// proposed to active by promote; the undo dry run lists the field that goes
// back and writes nothing; undo --apply sets the status back, adds one history
// line (the original stays, nothing is edited), and validate ends done; a
// second undo has no change left to take back and ends not ready. With no
// history at all undo ends not ready too.
// Planted failure: undo deletes the original line, or plans over a later value.
func TestUndoTakesBackThePromoteThenConfirmAndTheNextOneEndsNotReady(t *testing.T) {
	t.Parallel()
	dir, env := promoteRepo(t), ruleEnv(t)
	rules := func() string { return read(t, dir, "language/go/rules/errors.yaml") }
	if r := run(t, dir, env, "undo"); r.code != 4 {
		t.Errorf("undo with no history: exit %d, want not ready (4)\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := run(t, dir, env, with([]string{"rule", "promote", "go.errors.2"}, append(decide(), "--apply")...)...); r.code != 0 {
		t.Fatalf("promote: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(rules(), "status: active") {
		t.Fatalf("the may is not active:\n%s", rules())
	}
	before := read(t, dir, "history.jsonl")
	dry := run(t, dir, env, "undo")
	if dry.code != 6 || !strings.Contains(dry.stdout, "undo entry 1 (promote of rule:go.errors.2") || !strings.Contains(dry.stdout, "gragas undo --entry") || !digestRef.MatchString(dry.stdout) {
		t.Errorf("the dry run: exit %d, want declined (6)\n%s%s", dry.code, dry.stdout, dry.stderr)
	}
	if read(t, dir, "history.jsonl") != before || !strings.Contains(rules(), "status: active") {
		t.Errorf("the dry run wrote")
	}
	if r := run(t, dir, env, "undo", "--entry", digestRef.FindString(dry.stdout), "--apply"); r.code != 0 {
		t.Fatalf("undo --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(rules(), "status: proposed") {
		t.Errorf("the status did not go back:\n%s", rules())
	}
	lines := strings.Split(strings.TrimSpace(read(t, dir, "history.jsonl")), "\n")
	if len(lines) != 2 || lines[0] != strings.Split(strings.TrimSpace(before), "\n")[0] || !strings.Contains(lines[1], `"kind":"undo"`) || !strings.Contains(lines[1], `"reverts":"`+previousOf(t, lines[1])+`"`) {
		t.Errorf("the history after undo:\n%s", strings.Join(lines, "\n"))
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate after undo: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	if r := run(t, dir, env, "undo", "--apply"); r.code != 4 {
		t.Errorf("a second undo: exit %d, want not ready (4)\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestUndoWalksBackOneEntryAtATime: promote then retire the same rule; undo
// takes the retire back (it is the last), and
// the next undo, with the rule active again, takes back the promote.
func TestUndoWalksBackOneEntryAtATime(t *testing.T) {
	t.Parallel()
	dir, env := promoteRepo(t), ruleEnv(t)
	if r := run(t, dir, env, with([]string{"rule", "promote", "go.errors.2"}, append(decide(), "--apply")...)...); r.code != 0 {
		t.Fatalf("promote: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := run(t, dir, env, with([]string{"rule", "retire", "go.errors.2"}, append(why(), "--apply")...)...); r.code != 0 {
		t.Fatalf("retire: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	rules := func() string { return read(t, dir, "language/go/rules/errors.yaml") }
	// --apply names the entry: none, or one that is not the last, is refused and writes nothing
	before := read(t, dir, "history.jsonl")
	for name, args := range map[string][]string{"no entry": {"undo", "--apply"}, "an older entry": {"undo", "--entry", "1", "--apply"}} {
		r := run(t, dir, env, args...)
		if r.code != 5 || !strings.Contains(r.stdout+r.stderr, "gragas undo --entry") || read(t, dir, "history.jsonl") != before {
			t.Errorf("%s: exit %d, want refused (5) naming entry 2, nothing written\n%s%s", name, r.code, r.stdout, r.stderr)
		}
	}
	if r := run(t, dir, env, "undo", "--entry", "2", "--apply"); r.code != 0 || !strings.Contains(rules(), "status: active") {
		t.Fatalf("the first undo: exit %d\n%s%s%s", r.code, r.stdout, r.stderr, rules())
	}
	if r := run(t, dir, env, "undo", "--entry", "1", "--apply"); r.code != 0 || !strings.Contains(rules(), "status: proposed") {
		t.Fatalf("the second undo: exit %d\n%s%s%s", r.code, r.stdout, r.stderr, rules())
	}
	if n := hist(t, dir); n != 4 {
		t.Errorf("history holds %d lines, want 4: two changes and two undos", n)
	}
}

// previousOf is the digest a history line names as the line before it; an
// undo of the last entry names the same one in reverts, since the entry it
// takes back is the line before it.
func previousOf(t *testing.T, line string) string {
	t.Helper()
	m := regexp.MustCompile(`"previous":"(sha256:[0-9a-f]{64})"`).FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("the line names no previous: %s", line)
	}
	return m[1]
}

// TestADryRunGivenANumberEndsOnTheDigestAndThatLineIsRefusedInAnotherBook: a
// number typed on the dry run is input only. The closing line names the
// entry by the digest the dry run resolved, so the line applied in another
// book, where that digest is no entry, writes nothing, and a number could
// never have named the entry the dry run showed there.
func TestADryRunGivenANumberEndsOnTheDigestAndThatLineIsRefusedInAnotherBook(t *testing.T) {
	t.Parallel()
	dir, env := promoteRepo(t), ruleEnv(t)
	if r := run(t, dir, env, with([]string{"rule", "promote", "go.errors.2"}, append(decide(), "--apply")...)...); r.code != 0 {
		t.Fatalf("promote: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	r := run(t, dir, env, "undo", "--entry", "1")
	if r.code != 6 {
		t.Fatalf("the dry run: exit %d, want declined (6)\n%s%s", r.code, r.stdout, r.stderr)
	}
	out := r.stdout + r.stderr
	m := regexp.MustCompile(`gragas undo --entry\s+(sha256:[0-9a-f]{64})\s+--apply`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("the dry run given --entry 1 does not end on an apply line holding the digest:\n%s", out)
	}
	if strings.Contains(out, "--entry 1 --apply") {
		t.Errorf("the dry run printed the typed number back:\n%s", out)
	}
	// another book: a different history, so the digest names nothing there
	other := promoteRepo(t)
	if r := run(t, other, env, with([]string{"rule", "promote", "go.errors.1"}, append(decide(), "--apply")...)...); r.code != 0 {
		t.Fatalf("promote in the other book: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	before := read(t, other, "history.jsonl")
	if r := run(t, other, env, "undo", "--entry", m[1], "--apply"); r.code != 5 || read(t, other, "history.jsonl") != before {
		t.Errorf("the line applied in another book: exit %d, want refused (5), nothing written\n%s%s", r.code, r.stdout, r.stderr)
	}
}
