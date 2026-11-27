package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// lintDiffProblems is what is wrong with the command lines of lint-diff-system
// (gragas#1678): the run must keep the issues of every file the change touched
// (--whole-files), because gocognit, gocyclo and funlen anchor a finding at the
// func line, which a change inside the body leaves out of every hunk; and it
// must still be the diff's run, from the merge base with origin/main, on every
// package.
func lintDiffProblems(cmds []string) []string {
	var runs []string
	for _, c := range cmds {
		if strings.Contains(c, "golangci-lint") && strings.Contains(c, " run ") {
			runs = append(runs, c)
		}
	}
	if len(runs) != 1 {
		return []string{"lint-diff-system runs golangci-lint " + string(rune('0'+len(runs))) + " times, want once"}
	}
	var problems []string
	run := runs[0]
	for _, want := range []string{"--new-from-merge-base origin/main", "--whole-files", " ./..."} {
		if !strings.Contains(run, want) {
			problems = append(problems, "the lint-diff run lacks "+want)
		}
	}
	return problems
}

// TestLintDiffKeepsTheIssuesOfEveryFileTheChangeTouched holds the Taskfile's
// own command, and plants each way to lose the option: it dropped, the merge
// base dropped (a whole-tree run), and the run gone.
func TestLintDiffKeepsTheIssuesOfEveryFileTheChangeTouched(t *testing.T) {
	t.Parallel()
	got := lintDiffProblems(strings.Split(taskSection(t, "lint-diff-system"), "\n"))
	if len(got) != 0 {
		t.Errorf("lint-diff-system: %q", got)
	}
	const (
		diff  = `GOOS=linux {{.TOOLS}}/golangci-lint run --allow-parallel-runners --new-from-merge-base origin/main --whole-files ./... || exit 1`
		plain = `GOOS=linux {{.TOOLS}}/golangci-lint run --allow-parallel-runners --new-from-merge-base origin/main ./... || exit 1`
		tree  = `GOOS=linux {{.TOOLS}}/golangci-lint run --allow-parallel-runners --whole-files ./... || exit 1`
	)
	for _, tc := range []struct {
		name string
		cmds []string
		want string // a problem says; none when green
	}{
		{"the command with the option", []string{diff}, ""},
		{"the option dropped", []string{plain}, "lacks --whole-files"},
		{"the merge base dropped", []string{tree}, "lacks --new-from-merge-base origin/main"},
		{"no run", []string{"echo ok"}, "0 times"},
		{"two runs", []string{diff, diff}, "2 times"},
	} {
		got := lintDiffProblems(tc.cmds)
		switch {
		case tc.want == "" && len(got) != 0:
			t.Errorf("%s: red: %q", tc.name, got)
		case tc.want != "" && !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.want) }):
			t.Errorf("%s: want a problem saying %q, got %q", tc.name, tc.want, got)
		}
	}
}

// flatFunc is a function of n flat ifs: gocognit scores it n, gocyclo n+1.
func flatFunc(n int, inner string) string {
	var b strings.Builder
	b.WriteString("package fx\n\n// F is a ladder of ifs.\nfunc F(x int) int {\n")
	for i := range n {
		b.WriteString("\tif x > " + strconv.Itoa(i) + " {\n\t\t" + inner + "\n\t}\n")
		inner = "x++"
	}
	b.WriteString("\treturn x\n}\n")
	return b.String()
}

// TestLintDiffSeesAFunctionLevelFindingWhenOnlyTheBodyChanged is the heavy plant
// of gragas#1678: origin/main holds a function exactly at a limit (gocognit 20,
// gocyclo 30); the next commit nests one if inside its body and leaves the func
// line alone, which takes it over. The run of lint-diff-system's command (the
// pinned golangci-lint over the repository's config, --new-from-merge-base
// origin/main) reports the finding with --whole-files and is quiet without it,
// which is the gap. funlen is not in the config, so it has no plant here.
func TestLintDiffSeesAFunctionLevelFindingWhenOnlyTheBodyChanged(t *testing.T) {
	bin := pinnedLint(t)
	for _, tc := range []struct {
		linter string
		ifs    int
		want   string
	}{
		{"gocognit", 20, "cognitive complexity"},
		{"gocyclo", 29, "cyclomatic complexity"},
	} {
		t.Run(tc.linter, func(t *testing.T) {
			dir := writeModule(t, map[string]string{
				".golangci.yml": lintConfig(t),
				"go.mod":        "module example.com/planted\n\ngo 1.23.0\n",
				"fx/f.go":       flatFunc(tc.ifs, "x++"),
			})
			wideGit(t, dir, "init", "-q", "-b", "main")
			wideGit(t, dir, "add", ".")
			wideGit(t, dir, "commit", "-q", "-m", "main")
			wideGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
			f, err := os.Create(filepath.Join(dir, "fx", "f.go"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(flatFunc(tc.ifs, "if x > 99 {\n\t\t\tx--\n\t\t}")); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			wideGit(t, dir, "commit", "-q", "-am", "nest one if in the body")
			run := func(extra ...string) string {
				cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--new-from-merge-base", "origin/main")
				cmd.Args = append(cmd.Args, "--enable-only="+tc.linter)
				cmd.Args = append(cmd.Args, extra...)
				cmd.Args = append(cmd.Args, "./...")
				out, _ := lint(t, bin, dir, cmd)
				return out
			}
			if out := run("--whole-files"); !strings.Contains(out, "fx/f.go:4:1") || !strings.Contains(out, tc.want) {
				t.Errorf("with --whole-files: want the %s finding at the func line, got:\n%s", tc.linter, out)
			}
			if out := run(); strings.Contains(out, "fx/f.go") {
				t.Errorf("without --whole-files the run was meant to be quiet (the gap of #1678), got:\n%s", out)
			}
		})
	}
}
