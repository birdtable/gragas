package internal

// The members that skip when their inputs are the base's (gragas#1159): go-task's status: runs git and
// writes no checksum, so a killed or red run leaves nothing a later run could take for a pass. These
// tests run go-task's own `--status` in a scratch repository, hold each member's input list to what
// go list says the module reads, and hold the release tier to --force.

import (
	"encoding/json"
	"fmt"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// skipMembers are the members with a status:, each with the platform variable it needs (the -system
// tasks are given one) and whether a change to a Go file, to the explorer or to the contract files
// makes it run.
type skipMember struct {
	name, platform     string
	goFile, explorer   bool
	contract, licences bool
}

func skipMembers() []skipMember {
	return []skipMember{
		{"build", "", true, false, false, false},
		{"vet-system", "linux/amd64", true, false, false, false},
		{"licences-system", "linux/amd64", true, false, false, true},
		{"subset-build", "", true, false, false, false},
		{"explorer-typecheck", "", false, true, false, false},
		{"explorer-test", "", false, true, true, false},
		{"os-code", "", true, false, false, false},
		{"standin", "", true, false, false, false},
		{"lint-diff-system", "", true, false, false, false},
	}
}

// skipRepo is a scratch repository: the repository's Taskfile and the two lists its variables read,
// the files the members name as inputs, one commit, and origin/main at it.
func skipRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	put := func(rel, text string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"Taskfile.yml", "internal/journeys/smoke.txt", "internal/journeys/goldens.txt"} {
		put(rel, read(t, rel))
	}
	put(".gitignore", "node_modules/\n")
	put("main.go", "package main\n")
	put("pkg/a.go", "package pkg\n")
	put("go.mod", "module x\n")
	put("go.sum", "")
	put("gate/licences.txt", "MIT\n")
	put("gate/licence-sources.txt", "")
	put("gate/one-LICENSE", "text\n")
	put("gate/two-LICENSE", "text\n")
	put("LICENSE", "text\n")
	put("tools.yaml", "tools: []\n")
	put("gate/os-code-allowlist.txt", "# rows\n")
	put("gate/os-api.txt", "# rows\n")
	put("gate/os-knowledge-allowlist.txt", "# rows\n")
	put(".golangci.yml", "version: \"2\"\n")
	put("lint/project.yml", "linters: {}\n")
	put("layers.yaml", "layers: []\n")
	put(".node-version", "1\n")
	put("explorer/a.ts", "export {}\n")
	put("internal/testdata/contract-v1/x.json", "{}\n")
	put("internal/host/assets/a.txt", "a\n")
	put("internal/journeys/testdata/a.golden", "g\n")
	wideGit(t, dir, "init", "-q", "-b", "main")
	wideGit(t, dir, "add", "-A")
	wideGit(t, dir, "commit", "-q", "-m", "base")
	wideGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	return dir
}

// taskIn runs the repository's own go-task in dir with args and returns its output and whether it
// succeeded.
func taskIn(t *testing.T, dir string, args ...string) (string, bool) {
	t.Helper()
	return taskWith(t, dir, nil, args...)
}

// taskWith is taskIn with more variables in the environment.
func taskWith(t *testing.T, dir string, env []string, args ...string) (string, bool) {
	t.Helper()
	abs := pinnedTask(t)
	res, err := proc.Run(t.Context(), proc.Cmd{
		Path:      abs,
		Args:      args,
		Dir:       dir,
		Env:       append(append(configtest.Inherited(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull), env...),
		Timeout:   5 * time.Minute,
		MaxOutput: 4 << 20,
		Drain:     10 * time.Second,
	})
	return string(res.Stdout) + string(res.Stderr), err == nil && res.Code == 0
}

// skipped reports whether go-task takes the member for up to date in dir.
func skipped(t *testing.T, dir, name, platform string) bool {
	t.Helper()
	args := []string{"--status", name, "SKIP_UNCHANGED=1"}
	if platform != "" {
		args = append(args, "PLATFORM="+platform)
	}
	out, ok := taskIn(t, dir, args...)
	if !ok && !strings.Contains(out, "is not up-to-date") {
		t.Fatalf("task --status %s failed for another reason:\n%s", name, out)
	}
	return ok
}

// wantRuns holds, in dir, that exactly the members of the table whose flag is set (and no other) run.
func wantRuns(t *testing.T, dir, what string, runs func(i int) bool) {
	t.Helper()
	for i, m := range skipMembers() {
		if got := !skipped(t, dir, m.name, m.platform); got != runs(i) {
			t.Errorf("%s: %s runs = %v, want %v", what, m.name, got, runs(i))
		}
	}
}

// TestAMemberIsSkippedOnlyWhenItsInputsAreTheBasesAndAnyChangeRunsIt holds the cases of gragas#1159:
// a tree equal to origin/main skips every member; a change to a Go file runs the Go members and leaves
// the explorer's; a change to the explorer runs its two and not the Go ones; the contract files run
// explorer-test alone; the licence list runs licences alone; a new untracked input, a committed change
// and a missing base each run what they touch or, for the missing base, everything.
func TestAMemberIsSkippedOnlyWhenItsInputsAreTheBasesAndAnyChangeRunsIt(t *testing.T) {
	_ = pinnedTask(t)
	cases := []struct {
		what   string
		change func(t *testing.T, dir string)
		runs   func(i int) bool
	}{
		{"nothing changed", func(*testing.T, string) {}, func(int) bool { return false }},
		{"a Go file edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "pkg/a.go", "var X = 1\n") }, func(i int) bool { return skipMembers()[i].goFile }},
		{"go.mod edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "go.mod", "go 1.27\n") }, func(i int) bool { return skipMembers()[i].goFile }},
		{"an embedded file edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "internal/host/assets/a.txt", "b\n") }, func(i int) bool { return skipMembers()[i].goFile }},
		{"an untracked Go file", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "pkg/new.go", "package pkg\n") }, func(i int) bool { return skipMembers()[i].goFile }},
		{"the explorer edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "explorer/a.ts", "export const x = 1\n") }, func(i int) bool { return skipMembers()[i].explorer }},
		{"an untracked explorer file", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "explorer/b.ts", "export {}\n") }, func(i int) bool { return skipMembers()[i].explorer }},
		{"an ignored file under the explorer", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "explorer/node_modules/p/i.js", "x\n") }, func(int) bool { return false }},
		{"a contract file edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "internal/testdata/contract-v1/x.json", "\n") }, func(i int) bool { return skipMembers()[i].contract }},
		{"the licence list edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "gate/licences.txt", "ISC\n") }, func(i int) bool {
			return skipMembers()[i].licences || skipMembers()[i].name == "explorer-test" // licences.test.ts reads the list
		}},
		{"the sources edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "gate/licence-sources.txt", "# row\n") }, func(i int) bool { return skipMembers()[i].licences }},
		// One copy of two goes: a glob the shell expands names only the one left, so the inputs
		// hold the pattern quoted, for git to match.
		{"a licence copy deleted", func(t *testing.T, d string) {
			t.Helper()
			if err := os.Remove(filepath.Join(d, "gate", "one-LICENSE")); err != nil {
				t.Fatal(err)
			}
		}, func(i int) bool { return skipMembers()[i].licences }},
		{"an untracked licence copy", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "gate/three-LICENSE", "text\n") }, func(i int) bool { return skipMembers()[i].licences }},
		{"a Go file committed past the base", func(t *testing.T, d string) {
			t.Helper()
			appendTo(t, d, "pkg/a.go", "var Y = 2\n")
			wideGit(t, d, "commit", "-q", "-a", "-m", "change")
		}, func(i int) bool { return skipMembers()[i].goFile }},
		// The two members the Go files do not alone key (gragas#1710): the allowlist runs os-code, and each file the lint
		// config is rendered from, or the tool's pin, runs the lint of the diff.
		{"the os-code allowlist edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "gate/os-code-allowlist.txt", "row\n") }, func(i int) bool {
			return skipMembers()[i].goFile && skipMembers()[i].name == "os-code"
		}},
		// The census's list and the lint config it is held to run os-code too (gragas#1750).
		{"the os-api list edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "gate/os-api.txt", "row\n") }, func(i int) bool {
			return skipMembers()[i].goFile && skipMembers()[i].name == "os-code"
		}},
		// The OS-knowledge list runs it as well (gragas#2077).
		{"the os-knowledge list edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "gate/os-knowledge-allowlist.txt", "row\n") }, func(i int) bool {
			return skipMembers()[i].goFile && skipMembers()[i].name == "os-code"
		}},
		{"the rendered lint config edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, ".golangci.yml", "# x\n") }, func(i int) bool {
			return skipMembers()[i].name == "lint-diff-system" || skipMembers()[i].name == "os-code"
		}},
		{"a lint rule file edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "lint/project.yml", "# x\n") }, func(i int) bool { return skipMembers()[i].name == "lint-diff-system" }},
		{"the layers edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "layers.yaml", "# x\n") }, func(i int) bool { return skipMembers()[i].name == "lint-diff-system" }},
		{"the tools pins edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "tools.yaml", "# x\n") }, func(i int) bool { return skipMembers()[i].explorer || skipMembers()[i].name == "lint-diff-system" }},
		{"the Taskfile edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "Taskfile.yml", "# a stricter member\n") }, func(int) bool { return true }},
		{"a doc edited", func(t *testing.T, d string) { t.Helper(); appendTo(t, d, "README.md", "x\n") }, func(int) bool { return false }},
		{"no origin/main", func(t *testing.T, d string) {
			t.Helper()
			wideGit(t, d, "update-ref", "-d", "refs/remotes/origin/main")
		}, func(int) bool { return true }},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()
			dir := skipRepo(t)
			c.change(t, dir)
			wantRuns(t, dir, c.what, c.runs)
		})
	}
}

// appendTo adds text to a file of dir, which it makes if it is not there.
func appendTo(t *testing.T, dir, rel, text string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(old, text...), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAMemberKilledMidRunIsFollowedByAFullRun plants the fault a review found in go-task's sources:
// the member's own command kills go-task (SIGKILL) after the inputs changed. Nothing is written, so the
// next run still takes the member for not up to date, and go-task has made no checksum folder.
func TestAMemberKilledMidRunIsFollowedByAFullRun(t *testing.T) {
	t.Parallel()
	dir := skipRepo(t)
	text := read(t, "Taskfile.yml")
	re := regexp.MustCompile(`(?s)(\n  build:\n.*?\n    cmds:\n).*?(\n\n  rulebook:)`)
	if !re.MatchString(text) {
		t.Fatal("the build task was not found to plant a kill in")
	}
	planted := re.ReplaceAllString(text, "${1}      - kill -9 $$$$${2}")
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(planted), 0o600); err != nil {
		t.Fatal(err)
	}
	appendTo(t, dir, "pkg/a.go", "var Z = 3\n")
	if out, ok := taskIn(t, dir, "build"); ok {
		t.Fatalf("the killed member ended green:\n%s", out)
	}
	if skipped(t, dir, "build", "") {
		t.Error("after the killed run the member is taken for up to date: a kill was recorded as a pass")
	}
	if _, err := os.Stat(filepath.Join(dir, ".task")); err == nil {
		t.Error("go-task wrote a .task folder of checksums: nothing may be recorded")
	}
}

// TestTheReleaseTierRefusesToRunWithoutForce: the release never skips a member, so it holds --force.
func TestTheReleaseTierRefusesToRunWithoutForce(t *testing.T) {
	t.Parallel()
	dir := skipRepo(t)
	out, ok := taskIn(t, dir, "release")
	if ok || !strings.Contains(out, "task --force release") {
		t.Errorf("task release with no --force: ok %v, want it refused with the line that says --force:\n%s", ok, out)
	}
	if out, _ := taskIn(t, dir, "--force", "--dry", "release"); strings.Contains(out, "task --force release") {
		t.Errorf("task --force release is refused by its own precondition:\n%s", out)
	}
}

// ran reports whether the dry run's output shows the task running; it is red if the output shows it
// neither run nor up to date, so a task the tier no longer calls is not read as a pass.
func ran(t *testing.T, out, name string) bool {
	t.Helper()
	if strings.Contains(out, "task: ["+name+"]") {
		return true
	}
	if !strings.Contains(out, "Task \""+name+"\" is up to date") {
		t.Fatalf("the dry run shows %s neither run nor up to date:\n%s", name, out)
	}
	return false
}

// hostile is the settings a person's machine may carry: the variables a skip reads, and go-task's
// gentle-force experiment, which makes --force reach only the task it names.
func hostile(t *testing.T, dir string) []string {
	t.Helper()
	appendTo(t, dir, ".env", "SKIP_UNCHANGED=1\nSTAGED_ONLY=1\nCOMMIT_STAGED_ONLY=1\n")
	appendTo(t, dir, ".taskrc.yml", "experiments:\n  GENTLE_FORCE: 1\n")
	return []string{"TASK_X_GENTLE_FORCE=1", "SKIP_UNCHANGED=1", "STAGED_ONLY=1", "COMMIT_STAGED_ONLY=1"}
}

// TestNoSettingOutsideTheTaskfileTurnsASkipOn holds the ruling of gragas#1311: the skip is off by
// default and is turned on only by the command-line variable the pre-push hook passes, which the push
// hands down its calls; a bare task run, the release and the commit's whole members run whole whatever
// the environment, a .env, a taskrc or the gentle-force experiment say. full takes the variable the
// same way the push does (gragas#1619).
func TestNoSettingOutsideTheTaskfileTurnsASkipOn(t *testing.T) {
	t.Parallel()
	push := []string{"build", "vet-system", "os-code", "lint-diff-system"}
	full := []string{"licences-system", "subset-build", "explorer-typecheck", "explorer-test"}
	commit := []string{"format", "tidy", "explorer-format", "boards"}
	tiers := []struct {
		name    string
		members []string
		commit  bool
	}{{"pre-push", push, true}, {"full", full, false}}
	t.Run("the release runs every member, with --force", func(t *testing.T) {
		t.Parallel()
		dir := skipRepo(t)
		env := hostile(t, dir)
		out, ok := taskWith(t, dir, env, "--force", "--dry", "release-body", "SKIP_UNCHANGED=1")
		if !ok {
			t.Fatalf("task --force --dry release:\n%s", out)
		}
		for _, name := range slices.Concat(push, full, commit) {
			if name == "lint-diff-system" {
				continue // the push's alone: the release lints the whole tree
			}
			if !ran(t, out, name) {
				t.Errorf("the release skipped %s under the gentle-force experiment and a hostile environment", name)
			}
		}
	})
	for _, tier := range tiers {
		whole := slices.Clone(tier.members)
		if tier.commit {
			whole = append(whole, commit...)
		}
		t.Run("a bare "+tier.name+" runs every member whatever the environment and .env say", func(t *testing.T) {
			t.Parallel()
			dir := skipRepo(t)
			env := hostile(t, dir)
			out, ok := taskWith(t, dir, env, "--dry", tier.name+"-run")
			if !ok {
				t.Fatalf("task --dry %s-members:\n%s", tier.name, out)
			}
			for _, name := range whole {
				if !ran(t, out, name) {
					t.Errorf("a bare %s skipped %s: the environment or .env turned the skip on", tier.name, name)
				}
			}
		})
		t.Run(tier.name+" skips by the command-line variable, whatever the environment says", func(t *testing.T) {
			t.Parallel()
			dir := skipRepo(t)
			out, ok := taskWith(t, dir, []string{"SKIP_UNCHANGED=0", "STAGED_ONLY=1", "COMMIT_STAGED_ONLY=1"}, "--dry", tier.name+"-run", "SKIP_UNCHANGED=1")
			if !ok {
				t.Fatalf("task --dry %s SKIP_UNCHANGED=1:\n%s", tier.name, out)
			}
			for _, name := range tier.members {
				if ran(t, out, name) {
					t.Errorf("%s ran %s over the base's own tree", tier.name, name)
				}
			}
			if !tier.commit {
				return
			}
			for _, name := range commit {
				if !ran(t, out, name) {
					t.Errorf("%s skipped %s because the environment said it was a commit", tier.name, name)
				}
			}
		})
		t.Run(tier.name+" with --force runs every member, gentle or not", func(t *testing.T) {
			t.Parallel()
			dir := skipRepo(t)
			env := hostile(t, dir)
			out, ok := taskWith(t, dir, env, "--force", "--dry", tier.name+"-run", "SKIP_UNCHANGED=1")
			if !ok {
				t.Fatalf("task --force --dry %s:\n%s", tier.name, out)
			}
			for _, name := range tier.members {
				if !ran(t, out, name) {
					t.Errorf("task --force %s skipped %s under the gentle-force experiment", tier.name, name)
				}
			}
		})
	}
	t.Run("the commit skips only what it passes down", func(t *testing.T) {
		t.Parallel()
		dir := skipRepo(t)
		out, ok := taskWith(t, dir, []string{"SKIP_UNCHANGED=1"}, "--dry", "pre-commit-run")
		if !ok {
			t.Fatalf("task --dry pre-commit:\n%s", out)
		}
		for _, name := range commit {
			if ran(t, out, name) {
				t.Errorf("the commit ran %s with nothing staged", name)
			}
		}
	})
}

// statusProblems reads the Taskfile text: a status: of a member must be the one git-diff shape over
// {{.INPUTS}} and nothing else, a member may not have sources, generates or method, and preconditions
// belong to the release tier alone.
func statusProblems(text string) []string {
	var bad []string
	const shape = "    status:\n      - '[ \"{{.CLI_FORCE}}\" != true ] && [ \"{{.SKIP_UNCHANGED}}\" = 1 ]'\n      - git diff --quiet {{.SKIP_BASE}} -- {{.INPUTS}}\n      - '[ -z \"$(git ls-files --others --exclude-standard -- {{.INPUTS}})\" ]'\n"
	const staged = "    status:\n      - '[ \"{{.CLI_FORCE}}\" != true ] && [ \"{{.STAGED_ONLY}}\" = 1 ] && [ -z \"$(git diff --cached --name-only -- {{.INPUTS}})\" ]'\n"
	if n, m := strings.Count(text, shape)+strings.Count(text, staged), len(regexp.MustCompile(`(?m)^    status:`).FindAllString(text, -1)); n != m {
		bad = append(bad, "a status: that is not the git diff over {{.INPUTS}} and the untracked check, or the commit's staged one")
	}
	for _, key := range regexp.MustCompile(`(?m)^    (sources|generates|method):`).FindAllString(text, -1) {
		bad = append(bad, "the Taskfile has "+strings.TrimSpace(key)+" on a member: go-task would write a checksum")
	}
	for _, loc := range regexp.MustCompile(`(?m)^    preconditions?:`).FindAllStringIndex(text, -1) {
		heads := regexp.MustCompile(`\n  ([a-z][a-z-]*):\n`).FindAllStringSubmatch(text[:loc[0]]+"\n", -1)
		if len(heads) == 0 || (heads[len(heads)-1][1] != "release" && heads[len(heads)-1][1] != "release-body") {
			bad = append(bad, "preconditions: on a task other than release and release-body")
		}
	}
	return bad
}

func TestOnlyTheGitDiffStatusIsOnAMemberAndPreconditionsAreTheReleases(t *testing.T) {
	t.Parallel()
	for _, b := range statusProblems(read(t, "Taskfile.yml")) {
		t.Error(b)
	}
	if got := statusProblems("  m:\n    status:\n      - test -f stamp\n"); len(got) != 1 {
		t.Errorf("a planted stamp status: %v, want one problem", got)
	}
	if got := statusProblems("  m:\n    status:\n      - git diff --quiet {{.SKIP_BASE}} -- {{.INPUTS}}\n      - '[ -z \"$(git ls-files --others --exclude-standard -- {{.INPUTS}})\" ]'\n"); len(got) != 1 {
		t.Errorf("a planted git diff status with no force or pre-push check: %v, want one problem", got)
	}
	if got := statusProblems("  m:\n    sources:\n      - a\n"); len(got) != 1 {
		t.Errorf("a planted sources: %v, want one problem", got)
	}
	if got := statusProblems("\n  other:\n    preconditions:\n      - sh: x\n"); len(got) != 1 {
		t.Errorf("a planted precondition off the release: %v, want one problem", got)
	}
}

// taskVar is the value of a top-level variable of the Taskfile with its {{.NAME}} references expanded.
func taskVar(t *testing.T, name string) string {
	t.Helper()
	text := read(t, "Taskfile.yml")
	vars := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^  ([A-Z_]+): (.+)$`).FindAllStringSubmatch(text, -1) {
		vars[m[1]] = strings.Trim(m[2], `"`)
	}
	v, ok := vars[name]
	if !ok {
		t.Fatalf("the Taskfile has no variable %s", name)
	}
	for range 5 {
		v = regexp.MustCompile(`\{\{\.([A-Z_]+)\}\}`).ReplaceAllStringFunc(v, func(r string) string { return vars[r[3:len(r)-2]] })
	}
	return v
}

// uncovered returns the files no path of specs names. A spec is "*<suffix>" (git's * crosses
// folders), or a file or a folder; any other shape is refused as uncovered, so the list stays one
// this test can read.
func uncovered(specs []string, files []string) []string {
	var out []string
	for _, f := range files {
		ok := false
		for _, s := range specs {
			s = strings.Trim(s, "'")
			switch {
			case strings.HasPrefix(s, "*") && !strings.ContainsAny(s[1:], "*?[{"):
				ok = ok || strings.HasSuffix(f, s[1:])
			case !strings.ContainsAny(s, "*?[{"):
				ok = ok || f == s || strings.HasPrefix(f, s+"/")
			}
		}
		if !ok {
			out = append(out, f)
		}
	}
	return out
}

// TestTheGoInputsNameEveryFileTheModuleReads holds the list of the Go members to what go list says each
// package reads, so a new embedded folder or a new kind of file is red until the list names it, and
// plants a file the list does not name.
func TestTheGoInputsNameEveryFileTheModuleReads(t *testing.T) {
	out, err := inRoot(nil, "go", "list", "-e", "-json", "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	type pkg struct {
		Dir                                                            string
		GoFiles, CgoFiles, CFiles, CXXFiles, HFiles, SFiles, SwigFiles []string
		SysoFiles, TestGoFiles, XTestGoFiles, IgnoredGoFiles           []string
		EmbedFiles, TestEmbedFiles, XTestEmbedFiles, IgnoredOtherFiles []string
		FFiles, MFiles, SwigCXXFiles                                   []string
	}
	files := []string{"go.mod", "go.sum"}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(mustAbs(t, root), p.Dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, list := range [][]string{p.GoFiles, p.CgoFiles, p.CFiles, p.CXXFiles, p.HFiles, p.SFiles, p.SwigFiles, p.SysoFiles, p.TestGoFiles, p.XTestGoFiles, p.IgnoredGoFiles, p.EmbedFiles, p.TestEmbedFiles, p.XTestEmbedFiles, p.IgnoredOtherFiles, p.FFiles, p.MFiles, p.SwigCXXFiles} {
			for _, f := range list {
				files = append(files, filepath.ToSlash(filepath.Join(rel, f)))
			}
		}
	}
	if len(files) < 100 {
		t.Fatalf("go list named %d files: it read nothing", len(files))
	}
	specs := strings.Fields(taskVar(t, "GO_INPUTS"))
	for _, f := range uncovered(specs, files) {
		t.Errorf("%s is read by the build and is not an input of the Go members (GO_INPUTS %v)", f, specs)
	}
	if got := uncovered(specs, []string{"internal/new/x.tmpl", "pkg/a.go", "go.sum"}); !slices.Equal(got, []string{"internal/new/x.tmpl"}) {
		t.Errorf("a planted file outside the list: %v, want only it", got)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// escapes returns the paths a file of the explorer names by climbing out of it (../ segments) that land
// outside the explorer folder and are a tracked file or folder of the repository (tracked). A string that
// only looks like a climb (a template ending in ../ or in a partial name, a path written inside a planted
// source) resolves to nothing tracked and is not a path the file reads.
func escapes(file, content string, tracked map[string]bool) []string {
	out := templateReads(file, content, tracked)
	for _, m := range regexp.MustCompile(`((?:\.\./)+)([A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*)`).FindAllStringSubmatch(content, -1) {
		p := filepath.ToSlash(filepath.Join(filepath.Dir(file), m[1], m[2]))
		if p != "explorer" && !strings.HasPrefix(p, "explorer/") && !strings.HasPrefix(p, "../") && tracked[p] {
			out = append(out, p)
		}
	}
	return out
}

// templateReads are the paths a file reads through a helper that climbs out of the explorer and appends a
// name it is given (new URL(`../../${path}`, import.meta.url)): the climb alone lands on a folder, so the
// paths are the string literals of the same file that, taken from that folder, are tracked and outside the
// explorer.
func templateReads(file, content string, tracked map[string]bool) []string {
	var out []string
	for _, m := range regexp.MustCompile(`((?:\.\./)+)\$\{`).FindAllStringSubmatch(content, -1) {
		base := path.Join(path.Dir(file), m[1])
		if base == "explorer" || strings.HasPrefix(base, "explorer/") || strings.HasPrefix(base, "../") {
			continue
		}
		for _, l := range regexp.MustCompile("[\"'`]([A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*)/?[\"'`]").FindAllStringSubmatch(content, -1) {
			if p := path.Join(base, l[1]); p != "explorer" && !strings.HasPrefix(p, "explorer/") && tracked[p] && p != base {
				out = append(out, p)
			}
		}
	}
	return out
}

// TestTheExplorerInputsNameEveryPathItsFilesClimbOutTo reads the explorer's tracked files for ../ paths
// that leave the folder (conformance.test.ts reads internal/testdata/contract-v1) and holds each to the
// explorer-test inputs, with a plant of a file that reaches another folder.
func TestTheExplorerInputsNameEveryPathItsFilesClimbOutTo(t *testing.T) {
	list, err := inRoot(nil, "git", "ls-files", "explorer").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	all, err := inRoot(nil, "git", "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	tracked := map[string]bool{}
	for f := range strings.FieldsSeq(string(all)) {
		tracked[f] = true
		for d := path.Dir(f); d != "." && !tracked[d]; d = path.Dir(d) {
			tracked[d] = true
		}
	}
	specs := strings.Fields(taskVar(t, "EXPLORER_TEST_INPUTS"))
	seen := 0
	for f := range strings.FieldsSeq(string(list)) {
		if strings.HasSuffix(f, ".png") || strings.HasSuffix(f, "pnpm-lock.yaml") {
			continue
		}
		for _, p := range escapes(f, read(t, f), tracked) {
			seen++
			if got := uncovered(specs, []string{p}); len(got) != 0 {
				t.Errorf("%s reaches %s, which the explorer-test inputs do not name (%v)", f, p, specs)
			}
		}
	}
	if seen == 0 {
		t.Error("no file of the explorer climbs out of it: the scan read nothing (conformance.test.ts is expected to)")
	}
	here := "explorer/packages/x/test/a.test.ts"
	if got := uncovered(specs, escapes(here, `new URL("../../../../docs/x/", import.meta.url)`, map[string]bool{"docs/x": true})); !slices.Equal(got, []string{"docs/x"}) {
		t.Errorf("a planted path to docs/x: %v, want it named", got)
	}
	// A helper that climbs to the root through a template reads what its literals name.
	helper := "const root = (p) => new URL(`../../../../${p}`, import.meta.url);\nreadFileSync(root(\"docs/x\"));"
	if got := uncovered(specs, escapes(here, helper, map[string]bool{"docs/x": true, "docs": true})); !slices.Equal(got, []string{"docs/x"}) {
		t.Errorf("a planted root helper reading docs/x: %v, want it named", got)
	}
	// The false forms the scan once named: a template that ends in the climb (resolves to the root), one
	// that ends in a partial name, and a path written in a planted source that no file holds.
	for name, text := range map[string]string{
		"a template ending in the climb and no path": "new URL(`../../../../${p}`, import.meta.url)",
		"a template and a path inside the explorer":  "const r = (p) => new URL(`../../../../${p}`, import.meta.url); r(\"explorer/.node-version\")",
		"a partial name":       "expect(c.extends).toBe(`../../../../tsconfig.${mode}.js`)",
		"a path no file holds": `import { one } from "../../../../element/src/plant-target.ts";`,
	} {
		if got := escapes(here, text, map[string]bool{"docs/x": true}); len(got) != 0 {
			t.Errorf("%s: named %v, want nothing", name, got)
		}
	}
}

// TestTheCommitRunsAFormatMemberOnlyWhenAnInputIsStaged holds gragas#1216 through the real paths: in
// the dry run of pre-commit, whose call passes STAGED_ONLY down to the four members, format, tidy,
// explorer-format and boards are up to date when nothing they read is staged and run when it is; in the
// dry run of pre-push, with the variable exported, all four run whole.
func TestTheCommitRunsAFormatMemberOnlyWhenAnInputIsStaged(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		stage string // a file added to the index
		runs  map[string]bool
	}{
		{"nothing staged", "", map[string]bool{"format": false, "tidy": false, "explorer-format": false, "boards": false}},
		{"a doc staged", "README.md", map[string]bool{"format": false, "tidy": false, "explorer-format": false, "boards": false}},
		{"a Go file staged", "pkg/a.go", map[string]bool{"format": true, "tidy": true, "explorer-format": false, "boards": false}},
		{"go.mod of the tools module staged", "tools/go.mod", map[string]bool{"format": true, "tidy": true, "explorer-format": false, "boards": false}},
		{"an explorer file staged", "explorer/a.ts", map[string]bool{"format": false, "tidy": false, "explorer-format": true, "boards": false}},
		{"a board script staged", "docs/explorer-boards/gen/build.mjs", map[string]bool{"format": false, "tidy": false, "explorer-format": true, "boards": true}},
		{"the Taskfile staged", "Taskfile.yml", map[string]bool{"format": true, "tidy": true, "explorer-format": true, "boards": true}},
		{"the parts seed staged", "explorer/core/parts.seed.yaml", map[string]bool{"format": false, "tidy": false, "explorer-format": true, "boards": true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := skipRepo(t)
			if c.stage != "" {
				appendTo(t, dir, c.stage, "# x\n")
				wideGit(t, dir, "add", "--", c.stage)
			}
			commitOut, ok := taskIn(t, dir, "--dry", "pre-commit-run")
			if !ok {
				t.Fatalf("task --dry pre-commit:\n%s", commitOut)
			}
			pushOut, ok := taskWith(t, dir, []string{"STAGED_ONLY=1", "COMMIT_STAGED_ONLY=1"}, "--dry", "pre-push-run")
			if !ok {
				t.Fatalf("task --dry pre-push:\n%s", pushOut)
			}
			for name, want := range c.runs {
				if got := ran(t, commitOut, name); got != want {
					t.Errorf("%s at the commit: runs = %v, want %v", name, got, want)
				}
				if !ran(t, pushOut, name) {
					t.Errorf("%s at the push is up to date: the push runs it whole", name)
				}
			}
		})
	}
}

// TestAStaleGoldenIsRedAtPush is the plant of gragas#1175: in a scratch module whose journeys package
// compares a command's output with a golden, the push member journeys-goldens (run with --force, the
// tree being the base's) is green over the golden and red over one that no longer matches the output.
func TestAStaleGoldenIsRedAtPush(t *testing.T) {
	t.Parallel()
	dir := skipRepo(t)
	appendTo(t, dir, "cmd/gragas-tmp/main.go", `package main

import (
	"fmt"
	"os"
)

func main() {
	d := os.Getenv("TMPDIR") + "/golden-plant"
	if err := os.MkdirAll(d, 0o700); err != nil {
		os.Exit(1)
	}
	fmt.Println(d)
}
`)
	if err := os.WriteFile(filepath.Join(dir, "internal/journeys/goldens.txt"), []byte("TestTheOutput\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	appendTo(t, dir, "internal/journeys/j_test.go", `package journeys

import (
	"os"
	"testing"
)

func output() string { return "the output\n" }

func TestTheOutput(t *testing.T) {
	want, err := os.ReadFile("testdata/a.golden")
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != output() {
		t.Fatalf("golden differs: %q", want)
	}
}
`)
	appendTo(t, dir, "go.mod", "go 1.27\n")
	if err := os.WriteFile(filepath.Join(dir, "internal/journeys/testdata/a.golden"), []byte("the output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gate, env := goldensTools(t, dir)
	if out, ok := taskWith(t, dir, env, "--force", "journeys-goldens", gate); !ok {
		t.Fatalf("journeys-goldens is red over a matching golden:\n%s", out)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal/journeys/testdata/a.golden"), []byte("the old output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, ok := taskWith(t, dir, env, "--force", "journeys-goldens", gate)
	if ok || !strings.Contains(out, "golden differs") {
		t.Fatalf("journeys-goldens is not red over a stale golden (ok=%v):\n%s", ok, out)
	}
}

// goldensTools gives the scratch repository dir what journeys-goldens runs besides go: a copy of
// the pinned hatch in its .tools/bin, whose tests runs go test and holds that the member ran a test
// (gragas#1471), and the GATE variable, returned as a task argument, set to this repository's gragas-dev.
// The repository's path goes in by the environment, as GATE_DIR, and the argument names only the variable: a
// path spliced into the task's shell text would be read again by the shell, and a path with a quote, or a
// Windows path, would not come back as it was (#1655 X173).
func goldensTools(t *testing.T, dir string) (arg string, env []string) {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "hatch"
	bin, err := os.ReadFile(pinnedPath(t, name))
	if err != nil {
		t.Fatalf("the pinned hatch is not installed: run task tools (%v)", err)
	}
	tools := filepath.Join(dir, ".tools", "bin")
	if err := os.MkdirAll(tools, 0o750); err != nil {
		t.Fatal(err)
	}
	filestest.WriteProgram(t, filepath.Join(tools, name+goEXE(t)), bin)
	return `GATE=go run -C "$GATE_DIR" -trimpath ./cmd/gragas-dev`, []string{"GATE_DIR=" + abs}
}

// TestEveryInputListNamesTheTaskfile holds that a change to a member's own command runs the member: the
// Taskfile is an input of every list a skip reads.
func TestEveryInputListNamesTheTaskfile(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"GO_INPUTS", "EXPLORER_INPUTS", "EXPLORER_TEST_INPUTS", "LICENCES_INPUTS", "STAGED_GO_INPUTS", "STAGED_FORMAT_INPUTS", "STAGED_BOARDS_INPUTS", "OS_CODE_INPUTS", "LINT_INPUTS"} {
		if !slices.Contains(strings.Fields(taskVar(t, name)), "Taskfile.yml") {
			t.Errorf("%s does not name Taskfile.yml: a change to a member's own command would skip it", name)
		}
	}
}

// gitOut is the output of a git command in dir, without its newline.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitTry(t, dir, nil, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// gitTry is the output of a git command in dir, without its newline, with env
// added to its environment, or why it failed: it did not run, or it exited
// other than 0, with what it wrote to stderr.
func gitTry(t *testing.T, dir string, env []string, args ...string) (string, error) {
	t.Helper()
	res, err := proc.Run(t.Context(), proc.Cmd{
		Path:      configtest.Git(t),
		Args:      args,
		Dir:       dir,
		Env:       append(append(configtest.Inherited(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull), env...),
		Timeout:   time.Minute,
		MaxOutput: 1 << 20,
		Drain:     10 * time.Second,
	})
	if err != nil {
		return "", fmt.Errorf("git %v: %w", args, err)
	}
	if res.Code != 0 {
		return "", fmt.Errorf("git %v: code %d: %s", args, res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// osCodeTests is the tests os-code must run over ./internal, by what each reads: the OS-code allowlist, the
// census's list, the change against the bases (gragas#1993) and the OS-knowledge list (gragas#2077:
// before it, no member ran the last three, so a file that knew a system was red only at landing).
func osCodeTests() []string {
	return []string{"TestNoNewOsCode", "TestTheOsCodeCheckSeesEachKindRed", "TestTheOsCodeAllowlistOnlyShrinks", "TestTheOsCodeAllowlistNamesItsRows", "TestEveryIdentifierThatDiffersBySystemIsDecided", "TestAChangeThatAddsAnAdapterFileTouchesNoGo", "TestNoNewOsKnowledge", "TestTheOsKnowledgeCheckSeesEachKindRed", "TestTheOsKnowledgeListIsExact"}
}

// osCodeRunProblems is what is wrong with the run of the member os-code: a run that is not one ^(…)$ pattern over
// ./internal, a test of osCodeTests it leaves out, and a name no test file defines, which would run nothing.
func osCodeRunProblems(member, tests string) []string {
	m := regexp.MustCompile(`-run '\^\(([^)]*)\)\$' \./internal\n`).FindStringSubmatch(member)
	if m == nil {
		return []string{"os-code does not run one -run '^(TestA|TestB)$' over ./internal"}
	}
	names := strings.Split(m[1], "|")
	var bad []string
	for _, want := range osCodeTests() {
		if !slices.Contains(names, want) {
			bad = append(bad, "os-code does not run "+want+", so no member of the push holds what it reads")
		}
	}
	for _, n := range names {
		if !strings.Contains(tests, "func "+n+"(") {
			bad = append(bad, "os-code runs "+n+", which no test file defines, so it would run nothing")
		}
	}
	return bad
}

// TestTheOsCodeAndLintInputsNameWhatTheirMembersRead holds the two lists a push skips on (gragas#1710):
// os-code runs the tests of osCodeTests over ./internal, which read every Go file of the tree and
// the adapters folder (both in the Go members' list, gragas#1993), the allowlist, gate/os-api.txt with the lint config it is held to (gragas#1750) and the OS-knowledge
// list (gragas#2077), so its list is the Go members' own with those four, and its run names each of
// those tests, as a plant of each drop shows; the lint of the diff reads the Go files, and the
// config it is rendered from and the pin of the tool, so its list is the Go members' with them. Each
// list names every file of the folders it points at, and a planted file outside it is refused.
func TestTheOsCodeAndLintInputsNameWhatTheirMembersRead(t *testing.T) {
	t.Parallel()
	goSpecs := strings.Fields(taskVar(t, "GO_INPUTS"))
	for _, name := range []string{"OS_CODE_INPUTS", "LINT_INPUTS"} {
		specs := strings.Fields(taskVar(t, name))
		for _, s := range goSpecs {
			if !slices.Contains(specs, s) {
				t.Errorf("%s does not name %s, an input of every Go member", name, s)
			}
		}
	}
	osSpecs := strings.Fields(taskVar(t, "OS_CODE_INPUTS"))
	// The adapters folder is among os-code's inputs, and so is every Go file: the adapter-file test (gragas#1993) runs when either changes.
	adapterFile := adaptersFolder(t) + "/newtool/1.0.0.yaml"
	if got := uncovered(osSpecs, []string{"gate/os-code-allowlist.txt", "gate/os-api.txt", "gate/os-knowledge-allowlist.txt", ".golangci.yml", "internal/x/y.go", "internal/oscode_test.go", adapterFile}); got != nil {
		t.Errorf("os-code's list leaves out %v", got)
	}
	if got := uncovered(osSpecs, []string{"docs/checks.md"}); len(got) != 1 {
		t.Errorf("os-code's list names a file its tests do not read: %v uncovered, want it", got)
	}
	// os-code runs the adapter-file test with -count=1, from git's answer and never from Go's test cache (gragas#1993).
	member, tests := taskSection(t, "os-code"), testSources(t)
	if !regexp.MustCompile(`-count=1 [^\n]*-run '[^'\n]*[(|]TestAChangeThatAddsAnAdapterFileTouchesNoGo[|)][^'\n]*'`).MatchString(member) {
		t.Error("os-code does not run TestAChangeThatAddsAnAdapterFileTouchesNoGo with -count=1: no tier of a release-1 piece would run the adapter-file test")
	}
	for _, p := range osCodeRunProblems(member, tests) {
		t.Error(p)
	}
	for _, drop := range osCodeTests() {
		cut := strings.Replace(strings.Replace(member, "|"+drop+"|", "|", 1), "|"+drop+")", ")", 1)
		if cut == member {
			cut = strings.Replace(member, "^("+drop+"|", "^(", 1)
		}
		if !slices.ContainsFunc(osCodeRunProblems(cut, tests), func(p string) bool { return strings.Contains(p, "does not run "+drop+",") }) {
			t.Errorf("os-code's run with %s dropped is not red", drop)
		}
	}
	if p := osCodeRunProblems(strings.Replace(member, "TestNoNewOsKnowledge|", "TestNoSuchOsTest|TestNoNewOsKnowledge|", 1), tests); !slices.ContainsFunc(p, func(p string) bool { return strings.Contains(p, "TestNoSuchOsTest, which no test file defines") }) {
		t.Errorf("os-code's run with a name that is no test: %q, want it red", p)
	}
	lint := strings.Fields(taskVar(t, "LINT_INPUTS"))
	files := []string{".golangci.yml", "layers.yaml", "tools.yaml", "go.mod"}
	entries, err := os.ReadDir(filepath.Join(root, "lint"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		files = append(files, "lint/"+e.Name())
	}
	if got := uncovered(lint, files); got != nil {
		t.Errorf("the lint's list leaves out %v", got)
	}
	if got := uncovered(lint, []string{"docs/layers.md"}); len(got) != 1 {
		t.Errorf("the lint's list names a file the lint does not read: %v", got)
	}
}
