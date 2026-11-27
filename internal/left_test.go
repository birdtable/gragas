package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// leftRow is one row of gate/left.yaml: a class of fault found twice and its
// catch.
type leftRow struct {
	Class, Catch, Rule, Ticket string
	Count                      int64
	Line                       int
}

// leftRows reads the file, strictly. A file with no row, or a row that does
// not read, is a problem, and the problems are returned, not panicked on.
func leftRows(data []byte) ([]leftRow, []string) {
	v, err := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		return nil, []string{fmt.Sprintf("gate/left.yaml does not read: %v", err)}
	}
	var rows []leftRow
	for _, m := range ordered.Maps(doc.List("left")) {
		n, _ := m.Val("count").(int64)
		rows = append(rows, leftRow{Class: m.Str("class"), Catch: m.Str("catch"), Rule: m.Str("rule"), Ticket: m.Str("ticket"), Count: n, Line: m.Line})
	}
	if len(rows) == 0 {
		return nil, []string{"gate/left.yaml holds no row"}
	}
	return rows, nil
}

// leftProblems holds each row to its catch, in the repository's files: lint
// reads the generated lint file, test the test files, task the Taskfile, hook
// a file of the repository, and ticket plain words that name no issue by its
// number.
func leftProblems(rows []leftRow, lintFile, taskfile string, tests func() string, exists func(string) bool) []string {
	var out []string
	for _, r := range rows {
		at := fmt.Sprintf("gate/left.yaml line %d (%q)", r.Line, r.Class)
		if r.Class == "" || r.Count < 2 {
			out = append(out, at+": a row names its class and a count of 2 or more")
		}
		switch r.Catch {
		case "lint":
			if r.Rule == "" || !strings.Contains(lintFile, r.Rule) {
				out = append(out, at+": marked lint, but .golangci.yml holds no "+r.Rule)
			}
		case "test":
			if r.Rule == "" || !strings.Contains(tests(), "func "+r.Rule+"(") {
				out = append(out, at+": marked test, but no test file defines "+r.Rule)
			}
		case "task":
			if r.Rule == "" || !strings.Contains(taskfile, "\n  "+r.Rule+":") {
				out = append(out, at+": marked task, but Taskfile.yml holds no member "+r.Rule)
			}
		case "hook":
			if r.Rule == "" || !exists(r.Rule) {
				out = append(out, at+": marked hook, but the repository has no file "+r.Rule)
			}
		case "ticket":
			if !regexp.MustCompile(`^[a-z][^#\x00-\x1f\x7f]*$`).MatchString(r.Ticket) {
				out = append(out, at+": marked ticket, but does not say in plain words, with no issue number, what will catch it")
			}
		default:
			out = append(out, at+": its catch is "+r.Catch+", not one of lint, test, task, hook or ticket")
		}
	}
	return out
}

// testSources is every _test.go file of the repository's own code, joined.
func testSources(tb testing.TB) string {
	tb.Helper()
	var b strings.Builder
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, "_test.go") {
				data, rerr := os.ReadFile(p)
				if rerr != nil {
					return rerr
				}
				b.Write(data)
			}
			return err
		})
		if err != nil {
			tb.Fatal(err)
		}
	}
	return b.String()
}

// TestEveryClassLeftHasItsCatch holds the real file: each class names a catch
// that stands in the repository, so a lint rule that is dropped from the
// generated file, or a test that is renamed, turns its row red.
func TestEveryClassLeftHasItsCatch(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "gate", "left.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rows, bad := leftRows(data)
	task, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	exists := func(p string) bool { _, err := os.Stat(filepath.Join(root, p)); return err == nil }
	bad = append(bad, leftProblems(rows, lintConfig(t), string(task), func() string { return testSources(t) }, exists)...)
	for _, p := range bad {
		t.Error(p)
	}
}

// TestAClassLeftWhoseCatchIsGoneFails plants each way a row goes wrong: a lint
// rule the generated file does not hold, a test nobody defines, a member the
// Taskfile lacks, a hook file that is not there, a ticket that is no issue, an
// unknown catch, and a count of one; and a file with no row. The agreeing
// control is green, so the check is reading the files it is given.
func TestAClassLeftWhoseCatchIsGoneFails(t *testing.T) {
	t.Parallel()
	tests := func() string { return "func TestOne(t *testing.T) {}" }
	exists := func(p string) bool { return p == "here.go" }
	row := func(catch, rule, ticket string) leftRow {
		return leftRow{Class: "c", Count: 2, Catch: catch, Rule: rule, Ticket: ticket, Line: 3}
	}
	good := []leftRow{row("lint", "forbidigo", ""), row("test", "TestOne", ""), row("task", "gate-lists", ""), row("hook", "here.go", ""), row("ticket", "", "the lint fails on it")}
	if got := leftProblems(good, "linters: forbidigo", "tasks:\n  gate-lists:\n", tests, exists); len(got) != 0 {
		t.Fatalf("the agreeing control gave %v", got)
	}
	plants := map[string]leftRow{
		"holds no ruleX":     row("lint", "ruleX", ""),
		"defines TestTwo":    row("test", "TestTwo", ""),
		"no member nope":     row("task", "nope", ""),
		"no file gone.go":    row("hook", "gone.go", ""),
		"in plain words":     row("ticket", "", ""),
		"not one of lint":    row("maybe", "x", ""),
		"count of 2 or more": {Class: "c", Count: 1, Catch: "lint", Rule: "forbidigo", Line: 3},
	}
	for want, r := range plants {
		got := strings.Join(leftProblems([]leftRow{r}, "linters: forbidigo", "tasks:\n  gate-lists:\n", tests, exists), "\n")
		if !strings.Contains(got, want) {
			t.Errorf("the plant for %q gave %q", want, got)
		}
	}
	for _, ticket := range []string{"gragas#1843", "#12", "the lint, after #12", "Soon", "the lint\nfails"} {
		got := strings.Join(leftProblems([]leftRow{row("ticket", "", ticket)}, "", "", tests, exists), "\n")
		if !strings.Contains(got, "with no issue number") {
			t.Errorf("the ticket %q gave %q", ticket, got)
		}
	}
	if _, bad := leftRows([]byte("left: []\n")); len(bad) != 1 || !strings.Contains(bad[0], "holds no row") {
		t.Errorf("a file with no row gave %v", bad)
	}
	if _, bad := leftRows(nil); len(bad) == 0 {
		t.Error("an empty file gave no problem")
	}
}
