package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
)

const promoteRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
    pairs:
      - number: 1
        target: source
  - id: go.errors.2
    statement: Log an error once.
    strength: may
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
  - id: go.errors.3
    statement: Return an error.
    strength: must
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
`

const promoteSources = "format: 3\nsources:\n  - slug: effective-go\n    kind: guide\n    title: Effective Go\n    locator: https://go.dev/doc/effective_go\n"

// promoteRepo is the history rulebook with three proposed rules of its own: a
// must with its source and pair, a may with its source, and a must with no pair.
func promoteRepo(t *testing.T) string {
	t.Helper()
	dir := historyRepo(t, "")
	for name, body := range map[string]string{"language/go/rules/errors.yaml": promoteRules, "sources.yaml": promoteSources} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func decide() []string {
	return []string{"--decision-title", "Try it", "--decision-text", "Let it run a release."}
}

// TestPromoteAndConfirmEachEndInAPlanAndAHistoryLineAndValidateEndsDone: a must
// goes proposed to trial and stops there, since no release is counted yet; a
// may goes to active; confirm moves the review-by thirty days on and names the
// reviewer; each applied word adds one history line and validate ends done
// after it; a second run ends "nothing to do". A must with no pair is refused
// naming the example pair.
// Risk: a rule that skips trial, a line missing or doubled, or a refusal that
// names no field.
func TestPromoteAndConfirmEachEndInAPlanAndAHistoryLineAndValidateEndsDone(t *testing.T) {
	t.Parallel()
	dir, env := promoteRepo(t), ruleEnv(t)
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
	rules := func() string { return read(t, dir, "language/go/rules/errors.yaml") }
	step(1, with([]string{"rule", "promote", "go.errors.1"}, append(decide(), "--apply")...)...)
	if !strings.Contains(rules(), "status: trial") {
		t.Errorf("the must is not in trial:\n%s", rules())
	}
	early := run(t, dir, env, with([]string{"rule", "promote", "go.errors.1", "--decision", "decide-promote-go-errors-1", "--apply"})...)
	if early.code != 5 || !strings.Contains(early.stdout+early.stderr, "no release has gone by") {
		t.Errorf("a trial before a release: exit %d, want refused (5) naming the release\n%s%s", early.code, early.stdout, early.stderr)
	}
	nopair := run(t, dir, env, with([]string{"rule", "promote", "go.errors.3"}, append(decide(), "--apply")...)...)
	if nopair.code != 5 || !strings.Contains(nopair.stdout+nopair.stderr, "example pair") {
		t.Errorf("a must with no pair: exit %d, want refused (5) naming the example pair\n%s%s", nopair.code, nopair.stdout, nopair.stderr)
	}
	step(2, with([]string{"rule", "promote", "go.errors.2"}, append(decide(), "--apply")...)...)
	if !strings.Contains(rules(), "status: active") {
		t.Errorf("the may is not active:\n%s", rules())
	}
	again := step(2, "rule", "promote", "go.errors.2", "--decision", "decide-promote-go-errors-2", "--apply")
	if !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("promote twice:\n%s", again.stdout)
	}
	step(3, "rule", "confirm", "go.errors.2", "--apply")
	if want := thirtyDaysAfterTheLastEntry(t, dir); !strings.Contains(rules(), want) {
		t.Errorf("review_by is not %s, thirty days after the entry:\n%s", want, rules())
	}
	if h := read(t, dir, "history.jsonl"); !strings.Contains(h, `"kind":"confirm"`) || !strings.Contains(h, `"who":"gragas rule confirm"`) {
		t.Errorf("the history lacks the confirm or its reviewer:\n%s", h)
	}
	if c := step(3, "rule", "confirm", "go.errors.2", "--apply"); !strings.Contains(c.stdout, "nothing to do") {
		t.Errorf("confirm twice:\n%s", c.stdout)
	}
}

// thirtyDaysAfterTheLastEntry is the day thirty days after the last history
// line was written: the clock is the binary's, so the test reads the day from
// the line and never from a clock of its own (arch.ports.1).
func thirtyDaysAfterTheLastEntry(t *testing.T, dir string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(read(t, dir, "history.jsonl")), "\n")
	var e struct {
		When string `json:"when"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &e); err != nil {
		t.Fatal(err)
	}
	when, err := time.Parse(time.RFC3339, e.When)
	if err != nil {
		t.Fatal(err)
	}
	return when.AddDate(0, 0, 30).Format("2006-01-02")
}

var aDay = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// dated masks every day a plan names, so a golden of a plan that says the day
// and the day thirty on is the same on every day.
func dated(s string) string { return aDay.ReplaceAllString(s, "<day>") }

// TestThePlansAndRefusalsOfPromoteAndConfirmAreGoldensAtThreeWidths: each ending
// off a terminal, its footer and its exit (declined 6, refused 5), against a
// golden written only with -update by a person who reads the diff.
func TestThePlansAndRefusalsOfPromoteAndConfirmAreGoldensAtThreeWidths(t *testing.T) {
	t.Parallel()
	toTrial := with([]string{"rule", "promote", "go.errors.1"}, append(decide(), "--apply")...)
	mayActive := with([]string{"rule", "promote", "go.errors.2"}, append(decide(), "--apply")...)
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			args []string
			prep []string
			exit int
		}{
			{"rule-promote-declined", with([]string{"rule", "promote", "go.errors.1"}, decide()...), nil, 6},
			{"rule-promote-may-declined", with([]string{"rule", "promote", "go.errors.2"}, decide()...), nil, 6},
			{"rule-promote-no-pair-refused", with([]string{"rule", "promote", "go.errors.3"}, decide()...), nil, 5},
			{"rule-promote-trial-refused", []string{"rule", "promote", "go.errors.1", "--decision", "decide-promote-go-errors-1"}, toTrial, 5},
			{"rule-confirm-declined", []string{"rule", "confirm", "go.errors.2"}, mayActive, 6},
			{"rule-confirm-proposed-refused", []string{"rule", "confirm", "go.errors.1"}, nil, 5},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				dir, env := promoteRepo(t), ruleEnv(t)
				env["COLUMNS"] = cols
				if tc.prep != nil {
					if p := run(t, dir, env, tc.prep...); p.code != 0 {
						t.Fatalf("set-up: exit %d\n%s%s", p.code, p.stdout, p.stderr)
					}
				}
				r := run(t, dir, env, tc.args...)
				if r.code != tc.exit {
					t.Fatalf("exit %d, want %d\n%s%s", r.code, tc.exit, r.stdout, r.stderr)
				}
				uitest.Golden(t, tc.name+"-"+cols, dated("$ gragas "+strings.Join(tc.args, " ")+"\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr))
			})
		}
	}
}
