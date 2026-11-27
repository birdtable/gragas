package journeys

import (
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

func sourceNewArgs(slug, kind, title, locator string, more ...string) []string {
	return append([]string{"source", "new", "--source-slug", slug, "--source-kind", kind, "--title", title, "--locator", locator}, more...)
}

// TestASourceEditNotesItselfOnEachRuleThatCitesItInOnePlan: on a book where
// three rules cite effective-go, source edit --apply sets the locator, adds one
// line for the source and one on each of the three rules, the four lines share
// the plan digest and the chain runs through them, history <id> shows the note
// on each rule, and validate ends done; the same edit again ends "nothing to
// do" and writes nothing more.
// Risk: a rule that cites the source and is not told, a note on a rule that
// does not cite it, or lines that are not one plan.
func TestASourceEditNotesItselfOnEachRuleThatCitesItInOnePlan(t *testing.T) {
	t.Parallel()
	dir, env := promoteRepo(t), ruleEnv(t)
	const before = 0 // the book has no history yet: the edit starts it
	edit := []string{"source", "edit", "effective-go", "--locator", "https://go.dev/doc/effective_go#errors", "--apply"}
	if r := run(t, dir, env, edit...); r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got := read(t, dir, "sources.yaml"); !strings.Contains(got, `locator: "https://go.dev/doc/effective_go#errors"`+"\n") {
		t.Errorf("sources.yaml:\n%s", got)
	}
	lines := strings.Split(strings.TrimSpace(read(t, dir, "history.jsonl")), "\n")
	if len(lines) != before+4 {
		t.Fatalf("history holds %d lines, want %d: the source and three rules\n%s", len(lines), before+4, strings.Join(lines, "\n"))
	}
	added := lines[before:]
	if !strings.Contains(added[0], `"entity":"source:effective-go"`) || !strings.Contains(added[0], `"kind":"edit"`) {
		t.Errorf("the first line is not the source's edit:\n%s", added[0])
	}
	plan := between(added[0], `"plan":"`, `"`)
	for i, id := range []string{"go.errors.1", "go.errors.2", "go.errors.3"} {
		line := added[i+1]
		if !strings.Contains(line, `"entity":"rule:`+id+`"`) || !strings.Contains(line, `"kind":"source-edit"`) || between(line, `"plan":"`, `"`) != plan {
			t.Errorf("the note on %s:\n%s", id, line)
		}
		if h := run(t, dir, env, "history", id); h.code != 0 || !strings.Contains(h.stdout, "source-edit") {
			t.Errorf("history %s: exit %d\n%s%s", id, h.code, h.stdout, h.stderr)
		}
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	again := run(t, dir, env, edit...)
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || hist(t, dir) != before+4 {
		t.Errorf("second run: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
}

func between(s, from, to string) string {
	_, rest, _ := strings.Cut(s, from)
	got, _, _ := strings.Cut(rest, to)
	return got
}

// TestANewOwnDecisionSourcePassesPromotesSourceCheck: a source recorded as an
// own decision is cited by a new rule, which then leaves proposed.
// Risk: a source the book holds that promote does not take as a source.
func TestANewOwnDecisionSourcePassesPromotesSourceCheck(t *testing.T) {
	t.Parallel()
	dir, env := promoteRepo(t), ruleEnv(t)
	steps := [][]string{
		sourceNewArgs("house-rule", "own-decision", "What the owners decided", "house-rule", "--apply"),
		{"rule", "new", "--concern", "go.errors", "--statement", "Say what failed.", "--strength", "may", "--cite", "house-rule", "--apply"},
		with([]string{"rule", "promote", "go.errors.4"}, append(decide(), "--apply")...),
	}
	for _, a := range steps {
		if r := run(t, dir, env, a...); r.code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", a, r.code, r.stdout, r.stderr)
		}
	}
	if got := read(t, dir, "sources.yaml"); !strings.Contains(got, "slug: house-rule\n    kind: own-decision\n") {
		t.Errorf("sources.yaml:\n%s", got)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	same := run(t, dir, env, sourceNewArgs("house-rule", "own-decision", "What the owners decided", "house-rule", "--apply")...)
	if same.code != 0 || !strings.Contains(same.stdout, "nothing to do") {
		t.Errorf("source new twice: exit %d\n%s%s", same.code, same.stdout, same.stderr)
	}
	if bad := run(t, dir, env, sourceNewArgs("house-rule", "guide", "Other words", "elsewhere", "--apply")...); bad.code != 5 || !strings.Contains(bad.stdout+bad.stderr, "gragas source edit house-rule") {
		t.Errorf("a taken slug with other words: exit %d, want refused (5) naming source edit\n%s%s", bad.code, bad.stdout, bad.stderr)
	}
}

// TestTheDryRunsOfTheSourceWordsAreGoldensAtThreeWidths: the plan of each word
// off a terminal, its footer and "declined" with exit 6, against goldens
// written only with -update by a person who reads the diff.
func TestTheDryRunsOfTheSourceWordsAreGoldensAtThreeWidths(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			args []string
		}{
			{"source-new-declined", sourceNewArgs("rfc-9110", "standard", "HTTP Semantics", "RFC 9110", "--attribution", "IETF")},
			{"source-edit-declined", []string{"source", "edit", "effective-go", "--locator", "https://go.dev/doc/effective_go#errors", "--attribution", "the Go team"}},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				t.Parallel()
				dir, env := promoteRepo(t), ruleEnv(t)
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
