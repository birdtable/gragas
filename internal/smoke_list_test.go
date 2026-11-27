package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// smokeLines are the lines of internal/journeys/smoke.txt that name a command and its test, as
// the Taskfile's own loop reads them: two fields, the first beginning with a letter.
func smokeLines(text string) (pairs [][2]string, bad []string) {
	for i, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || !regexp.MustCompile(`^[a-z]`).MatchString(f[0]) {
			continue
		}
		if len(f) != 2 {
			bad = append(bad, fmt.Sprintf("smoke.txt line %d is not a command and one test: %q", i+1, l))
			continue
		}
		pairs = append(pairs, [2]string{f[0], f[1]})
	}
	return pairs, bad
}

// treeCommands are the commands of docs/commands.md's "In the tree" table, the first word of
// each row's first cell.
func treeCommands(doc string) []string {
	_, rest, ok := strings.Cut(doc, "## In the tree\n")
	if !ok {
		return nil
	}
	var out []string
	for l := range strings.SplitSeq(rest, "\n") {
		if strings.HasPrefix(l, "## ") {
			break
		}
		cell, _, _ := strings.Cut(strings.TrimPrefix(l, "|"), "|")
		if f := strings.Fields(cell); strings.HasPrefix(l, "| ") && len(f) > 0 && f[0] != "Command" && !strings.HasPrefix(f[0], "-") {
			out = append(out, f[0])
		}
	}
	return out
}

// smokeProblems holds the smoke list to the commands in the tree and to the journey tests: a
// command with no line, a line with no command, a test that is not defined, a command twice, and an
// empty list (gragas#1733, class 5: a gate list not updated).
func smokeProblems(smoke, commands, journeys string) []string {
	pairs, out := smokeLines(smoke)
	cmds := treeCommands(commands)
	if len(pairs) == 0 {
		out = append(out, "smoke.txt names no command and test")
	}
	if len(cmds) == 0 {
		out = append(out, "docs/commands.md lists no command in the tree")
	}
	seen := map[string]bool{}
	for _, p := range pairs {
		if seen[p[0]] {
			out = append(out, "smoke.txt names "+p[0]+" twice")
		}
		seen[p[0]] = true
		if !slices.Contains(cmds, p[0]) {
			out = append(out, "smoke.txt names "+p[0]+", which is not a command in the tree")
		}
		if !strings.Contains(journeys, "func "+p[1]+"(") {
			out = append(out, "smoke.txt names "+p[1]+" for "+p[0]+", and no test of internal/journeys defines it")
		}
	}
	for _, c := range cmds {
		if !seen[c] {
			out = append(out, "the command "+c+" is in the tree and has no line in smoke.txt: add `"+c+" <the test that runs it>`")
		}
	}
	return out
}

// journeySources is every test file of internal/journeys, joined.
func journeySources(tb testing.TB) string {
	tb.Helper()
	files, err := filepath.Glob(filepath.Join(root, "internal", "journeys", "*_test.go"))
	if err != nil {
		tb.Fatal(err)
	}
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			tb.Fatal(err)
		}
		b.Write(data)
	}
	return b.String()
}

// TestTheSmokeListHoldsTheCommandsOfTheTree: smoke.txt names each command in the tree once and a
// journey test that exists, so a command added without its line, or a test renamed, is red at
// push, in the gate-lists member, and not at landing. The plants are a command with no line, a line
// with no command, a test not defined, a command twice and an empty list; the control is the list
// that agrees.
func TestTheSmokeListHoldsTheCommandsOfTheTree(t *testing.T) {
	t.Parallel()
	data := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, p := range smokeProblems(data("internal/journeys/smoke.txt"), data("docs/commands.md"), journeySources(t)) {
		t.Error(p)
	}

	const doc = "## In the tree\n\n| Command | Context |\n|---|---|\n| validate | R |\n| rule (new, edit) | R |\n\n## Next\n"
	const tests = "func TestV(t *testing.T) {}\nfunc TestR(t *testing.T) {}\n"
	if got := smokeProblems("# c\nvalidate TestV\nrule TestR\n", doc, tests); len(got) != 0 {
		t.Fatalf("the agreeing control gave %v", got)
	}
	for want, smoke := range map[string]string{
		"the command rule is in the tree and has no line": "validate TestV\n",
		"names gone, which is not a command":              "validate TestV\nrule TestR\ngone TestV\n",
		"no test of internal/journeys defines it":         "validate TestV\nrule TestMissing\n",
		"names validate twice":                            "validate TestV\nvalidate TestV\nrule TestR\n",
		"names no command and test":                       "# nothing\n",
	} {
		if got := strings.Join(smokeProblems(smoke, doc, tests), "\n"); !strings.Contains(got, want) {
			t.Errorf("the plant %q gave %q, want %q", smoke, got, want)
		}
	}
}
