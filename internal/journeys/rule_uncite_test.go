package journeys

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

const unciteRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Log an error once.
    strength: may
    status: active
    origin: authored
    review_by: "2027-06-30"
    sources:
      - ref: effective-go
        backs: rule
      # the faq says it too
      - ref: go-faq
        where: "Why: errors"
        backs: rule
`

const unciteSources = "format: 3\nsources:\n" +
	"  - slug: effective-go\n    kind: guide\n    title: Effective Go\n    locator: https://go.dev/doc/effective_go\n" +
	"  - slug: go-faq\n    kind: guide\n    title: Go FAQ\n    locator: https://go.dev/doc/faq\n"

// unciteRepo is the history rulebook with one active rule that cites two
// sources, the second under a comment and with a place.
func unciteRepo(t *testing.T) string {
	t.Helper()
	dir := historyRepo(t, "")
	for name, body := range map[string]string{"language/go/rules/errors.yaml": unciteRules, "sources.yaml": unciteSources} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestRuleEditUnciteTakesOneCiteOutAndRefusesTheLastOfAnActiveRule: on an
// active rule with two cites, --uncite --apply takes the second out with its
// comment, leaves the first and the source in the book, adds one history line,
// and validate ends done; the same command again ends "nothing to do"; the
// uncite of the last cite ends refused (5) naming the rule and writes nothing.
// Risk: a cite taken out with its neighbour's lines, a source deleted from the
// book, or an active rule left with no source.
func TestRuleEditUnciteTakesOneCiteOutAndRefusesTheLastOfAnActiveRule(t *testing.T) {
	t.Parallel()
	dir, env := unciteRepo(t), ruleEnv(t)
	r := run(t, dir, env, "rule", "edit", "go.errors.1", "--uncite", "go-faq", "--apply")
	if r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("rule edit --uncite: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	want := strings.Replace(unciteRules, "      # the faq says it too\n      - ref: go-faq\n        where: \"Why: errors\"\n        backs: rule\n", "", 1)
	if got := read(t, dir, "language/go/rules/errors.yaml"); got != want {
		t.Errorf("the rules file is\n%s\nwant\n%s", got, want)
	}
	if got := read(t, dir, "sources.yaml"); got != unciteSources {
		t.Errorf("the sources changed:\n%s", got)
	}
	if got := hist(t, dir); got != 1 {
		t.Errorf("history holds %d lines, want 1", got)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Errorf("validate exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	again := run(t, dir, env, "rule", "edit", "go.errors.1", "--uncite", "go-faq", "--apply")
	if again.code != 0 || !strings.Contains(again.stdout, "nothing to do") || hist(t, dir) != 1 {
		t.Errorf("second run: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	last := run(t, dir, env, "rule", "edit", "go.errors.1", "--uncite", "effective-go", "--apply")
	if out := last.stdout + last.stderr; last.code != 5 || !strings.Contains(out, "refused") || !strings.Contains(out, "go.errors.1 is active") {
		t.Errorf("the last cite: exit %d, want refused (5) naming the rule\n%s%s", last.code, last.stdout, last.stderr)
	}
	if got := read(t, dir, "language/go/rules/errors.yaml"); got != want || hist(t, dir) != 1 {
		t.Errorf("a refused uncite wrote:\n%s", got)
	}
}

// TestTheDryRunOfAnUnciteIsAGoldenAtThreeWidths: the plan of an uncite off a
// terminal, ending on the line that applies it and "declined" with exit 6, and
// the refusal of the last cite with exit 5, against goldens written only with
// -update by a person who reads the diff.
func TestTheDryRunOfAnUnciteIsAGoldenAtThreeWidths(t *testing.T) {
	t.Parallel()
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			code int
			args []string
			prep []string
		}{
			{"rule-uncite-declined", 6, []string{"rule", "edit", "go.errors.1", "--uncite", "go-faq"}, nil},
			{"rule-uncite-refused", 5, []string{"rule", "edit", "go.errors.1", "--uncite", "effective-go"}, []string{"rule", "edit", "go.errors.1", "--uncite", "go-faq", "--apply"}},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				dir, env := unciteRepo(t), ruleEnv(t)
				env["COLUMNS"] = cols
				if tc.prep != nil {
					if p := run(t, dir, env, tc.prep...); p.code != 0 {
						t.Fatalf("set-up: exit %d\n%s%s", p.code, p.stdout, p.stderr)
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

// TestTheRuleRowSaysEveryWordAndFileOfAnUncite: the rule family's row of the
// command reference names refused among its closing words, and the two files
// the uncite plan writes and no more, so a refusal or a write the row leaves
// out shows here.
func TestTheRuleRowSaysEveryWordAndFileOfAnUncite(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(moduleRoot(), "docs/commands.md"))
	if err != nil {
		t.Fatal(err)
	}
	row := ""
	for l := range strings.SplitSeq(string(b), "\n") {
		if strings.HasPrefix(l, "| rule (") {
			row = l
		}
	}
	cells := strings.Split(row, " | ")
	if len(cells) < 5 {
		t.Fatalf("no rule row in docs/commands.md: %q", row)
	}
	if writes := cells[3]; writes != "the concern's rules file, history.jsonl" {
		t.Errorf("the rule row writes %q; the uncite plan writes the concern's rules file and history.jsonl", writes)
	}
	if exits := strings.Split(cells[4], ", "); !slices.Contains(exits, "refused") || !slices.Contains(exits, "declined") {
		t.Errorf("the rule row ends %q; an uncite ends refused and declined too", cells[4])
	}
}
