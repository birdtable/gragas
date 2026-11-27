package format

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// The README's two tables of derived words name, for each word, the rules or
// the file the fixture holds as its input. This file holds each of them to
// what it names: a rule the table gives for "proposed" has the status
// proposed, and a file the table gives for an observation carries the tool the
// row talks about.

// standingChecks are the words whose input is a field of the rule itself.
// The others (enforced, reviewed, blocked, unproven) derive from bindings and
// proof, which the binary reads; for those the table is held to the rule ids.
func standingChecks() map[string]func(model.Rule) bool {
	return map[string]func(model.Rule) bool{
		"proposed": func(r model.Rule) bool { return r.Status == model.Proposed },
		"retired":  func(r model.Rule) bool { return r.Status == model.Retired && r.ReplacedBy != "" },
		"planned": func(r model.Rule) bool {
			return r.Strength == model.Must && r.Status != model.Proposed && r.Status != model.Retired
		},
		"advisory": func(r model.Rule) bool { return r.Strength == model.May },
		"trial":    func(r model.Rule) bool { return r.Status == model.Trial },
	}
}

var (
	standingRow    = regexp.MustCompile(`(?m)^\| ([a-z]+)( \(mark\))? \| ([^|]+) \| ([^|]+) \|$`)
	observationRow = regexp.MustCompile(`(?m)^\| (as-set|tighter|weaker|absent|unmapped|excluded) \| ([^|]+) \|$`)
	backticked     = regexp.MustCompile("`([^`]+)`")
	ruleIDToken    = regexp.MustCompile(`^[a-z]+\.[a-z]+\.[0-9]+$`)
	toolToken      = regexp.MustCompile(`^[a-z]+$`)
)

// standingProblems is every row of the standings table whose rule is missing
// or does not carry the word.
func standingProblems(readme string, rules map[string]model.Rule) []string {
	var out []string
	for _, m := range standingRow.FindAllStringSubmatch(readme, -1) {
		word, where := m[1], m[4]
		for _, tok := range backticked.FindAllStringSubmatch(where, -1) {
			if !ruleIDToken.MatchString(tok[1]) {
				continue
			}
			r, ok := rules[tok[1]]
			if !ok {
				out = append(out, fmt.Sprintf("%s: %s is no rule of the fixtures", word, tok[1]))
				continue
			}
			if check := standingChecks()[word]; check != nil && !check(r) {
				out = append(out, fmt.Sprintf("%s: %s is %s/%s, which is not %s", word, tok[1], r.Status, r.Strength, word))
			}
		}
	}
	return out
}

// observationProblems is every row of the observations table whose file is
// missing or does not carry the tools the row names. The word "absent" holds
// the other way: the file does not carry them. The excluded row names rule ids
// and a decision, which the file carries.
func observationProblems(readme string, files func(string) (string, bool)) []string {
	var out []string
	for _, m := range observationRow.FindAllStringSubmatch(readme, -1) {
		word := m[1]
		var path string
		var names []string
		for _, tok := range backticked.FindAllStringSubmatch(m[2], -1) {
			switch {
			case strings.Contains(tok[1], "/") || strings.HasSuffix(tok[1], ".yml"):
				if path == "" {
					path = tok[1]
				}
			case toolToken.MatchString(tok[1]) || word == "excluded" && (ruleIDToken.MatchString(tok[1]) || tok[1] == "house-call"):
				names = append(names, tok[1])
			}
		}
		body, ok := files(path)
		if !ok {
			out = append(out, fmt.Sprintf("%s: the file %q does not exist", word, path))
			continue
		}
		if len(names) == 0 {
			out = append(out, fmt.Sprintf("%s: the row names no tool to look for", word))
		}
		for _, n := range names {
			if has := strings.Contains(body, n); has == (word == "absent") {
				out = append(out, fmt.Sprintf("%s: %s %s %q", word, path, map[bool]string{true: "carries", false: "lacks"}[has], n))
			}
		}
	}
	return out
}

func rulesByID(t *testing.T) map[string]model.Rule {
	t.Helper()
	out := map[string]model.Rule{}
	for _, r := range loadFixture(t, "base").Book.Rules {
		out[string(r.ID)] = r
	}
	return out
}

func TestEveryFileTheReadmeTablesNameCarriesTheWordTheRowNames(t *testing.T) {
	t.Parallel()
	readme, files := readmeAndFiles(t)
	rules := rulesByID(t)
	if n := len(standingRow.FindAllString(readme, -1)); n < 9 {
		t.Fatalf("the standings table has %d rows, want 9: the pattern no longer reads it", n)
	}
	if n := len(observationRow.FindAllString(readme, -1)); n != 6 {
		t.Fatalf("the observations table has %d rows, want 6: the pattern no longer reads it", n)
	}
	if p := standingProblems(readme, rules); len(p) > 0 {
		t.Errorf("the standings table:\n%s", strings.Join(p, "\n"))
	}
	if p := observationProblems(readme, files); len(p) > 0 {
		t.Errorf("the observations table:\n%s", strings.Join(p, "\n"))
	}
}

// TestAMovedInputTurnsTheWordTablesRed plants the two ways a fixture drifts
// from its README: a rule whose status is changed, and an observation file
// that no longer has its tool.
func TestAMovedInputTurnsTheWordTablesRed(t *testing.T) {
	t.Parallel()
	readme, files := readmeAndFiles(t)
	rules := rulesByID(t)

	moved := maps.Clone(rules)
	r := moved["go.errors.2"]
	r.Status = model.Active
	moved["go.errors.2"] = r
	if p := standingProblems(readme, moved); len(p) != 1 || !strings.HasPrefix(p[0], "proposed: go.errors.2") {
		t.Errorf("a proposed rule made active: %v", p)
	}

	stripped := func(path string) (string, bool) {
		body, ok := files(path)
		return strings.ReplaceAll(body, "wrapcheck", "w"), ok
	}
	if p := observationProblems(readme, stripped); len(p) == 0 {
		t.Error("a file that lost wrapcheck passed the observations table")
	}
	grown := func(path string) (string, bool) {
		body, ok := files(path)
		if strings.HasSuffix(path, "weakened/.golangci.yml") {
			body += "\nnilerr\n"
		}
		return body, ok
	}
	if p := observationProblems(readme, grown); len(p) == 0 {
		t.Error("a file that gained nilerr passed the absent row")
	}
	gone := func(string) (string, bool) { return "", false }
	if p := observationProblems(readme, gone); len(p) != 6 {
		t.Errorf("with no file, %d problems, want 6: %v", len(p), p)
	}
}
