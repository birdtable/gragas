package internal

import (
	"errors"
	"fmt"
	"github.com/birdtable/go-files/filestest"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/gitenv"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// taskSection is the text of one task of Taskfile.yml: from its key to the next task.
func taskSection(t *testing.T, name string) string {
	t.Helper()
	text := read(t, "Taskfile.yml")
	i := strings.Index(text, "\n  "+name+":\n")
	if i < 0 {
		t.Fatalf("the Taskfile has no task %s", name)
	}
	body := text[i+1:]
	if j := regexp.MustCompile(`\n  [a-z][a-z-]*:\n`).FindStringIndex(body); j != nil {
		body = body[:j[0]+1]
	}
	return body
}

// taskNames are the tasks of Taskfile.yml, in file order.
func taskNames(t *testing.T) []string {
	t.Helper()
	text := read(t, "Taskfile.yml")
	_, tasks, ok := strings.Cut(text, "\ntasks:\n")
	if !ok {
		t.Fatal("the Taskfile has no tasks")
	}
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^  ([a-z][a-z-]*):$`).FindAllStringSubmatch(tasks, -1) {
		names = append(names, m[1])
	}
	return names
}

// cmdEntries are the entries of the cmds list of a task's text, each whole: from
// its "      - " line to the line before the next entry or the next key.
func cmdEntries(section string) []string {
	var out []string
	in := false
	for line := range strings.SplitSeq(section, "\n") {
		switch {
		case line == "    cmds:":
			in = true
		case in && strings.HasPrefix(line, "      - "):
			out = append(out, line)
		case in && len(out) > 0 && (strings.HasPrefix(line, "        ") || line == ""):
			out[len(out)-1] += "\n" + line
		case in && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     "):
			in = false
		}
	}
	return out
}

// TestEveryTierMemberOfTheTaskfileHasATimeout holds the rule that a member
// which starts a process tree ends red at a deadline: every entry of a task's
// cmds has a timeout of its own, no task has a task-level one, and none starts a
// command in the background with a trailing &. go-task 3.54.0 reads timeout only
// on a cmds entry or a deps entry: a task-level one is ignored, and held nothing
// for the 51 that were there (gragas#1402).
func TestEveryTierMemberOfTheTaskfileHasATimeout(t *testing.T) {
	t.Parallel()
	for _, name := range taskNames(t) {
		if name == "default" {
			continue
		}
		body := taskSection(t, name)
		if strings.Contains(body, "\n    timeout: ") {
			t.Errorf("task %s has a task-level timeout: go-task 3.54.0 ignores it; put it on each cmds entry (gragas#1402)", name)
		}
		// A task that only calls other tasks runs nothing of its own: the tasks it calls hold the limits.
		entries := cmdEntries(body)
		if !slices.ContainsFunc(entries, func(e string) bool { return !strings.HasPrefix(e, "      - task:") }) {
			entries = nil
		}
		for i, e := range entries {
			if !strings.Contains(e, "\n        timeout: ") {
				t.Errorf("entry %d of task %s has no timeout of its own: %.60q", i+1, name, e)
			}
		}
		for line := range strings.SplitSeq(body, "\n") {
			if l := strings.TrimSpace(line); strings.HasSuffix(l, " &") {
				t.Errorf("task %s starts %q in the background", name, l)
			}
		}
	}
}

// TestAnEntrysTimeoutIsHeldByGoTask is the plant of gragas#1402, run against
// the pinned go-task: a cmds entry that never ends and carries timeout: 1s ends
// red within seconds, naming the timeout. It holds what the pin does, so a
// go-task that stopped reading the entry's timeout, or a Taskfile that put it
// back at the task level, turns the tier's limits into fiction again.
func TestAnEntrysTimeoutIsHeldByGoTask(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	taskfile := "version: '3'\n\ntasks:\n  held:\n    cmds:\n      - cmd: while true; do :; done\n        timeout: 1s\n"
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o600); err != nil {
		t.Fatal(err)
	}
	out, ok := taskIn(t, dir, "held")
	if ok || !strings.Contains(out, "command timeout exceeded (1s)") {
		t.Errorf("a command that never ends, with timeout: 1s on its entry, was not stopped: ok %v\n%s", ok, out)
	}
}

// TestTheSelectionIsDeclaredWithItsMeasuredTime holds tests.selection.1: push
// uses Go's test cache only by a setting declared in the gate's config, with the
// whole-suite time it was measured at, and push's test task carries no -count=1
// (the release tier's flag alone).
func TestTheSelectionIsDeclaredWithItsMeasuredTime(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	if !strings.Contains(text, "\n  SELECTION: go-test-cache\n") {
		t.Error("SELECTION is not declared as go-test-cache")
	}
	if !regexp.MustCompile(`\n  SELECTION_MEASURED_SECONDS: "[0-9]+"\n`).MatchString(text) {
		t.Error("SELECTION_MEASURED_SECONDS is not a whole number of seconds")
	}
	if strings.Contains(taskSection(t, "test"), "-count=1") {
		t.Error("the push's test task passes -count=1: that is the release tier's alone")
	}
}

// osProblems are what the one-machine rule refuses in a Taskfile's text: gragas's tiers name no other machine, so
// the Taskfile reads no system name (uname), has no namespace task, branches on no OS, joins no PATH
// of its own (PATHSEP) and carries no platforms: key. `--platform {{OS}}/{{ARCH}}` on a pinned
// download is the one allowed spelling. A text with no task is refused, so a read of nothing is
// never a pass.
func osProblems(text string) []string {
	if !strings.Contains(text, "\ntasks:\n") {
		return []string{"the Taskfile holds no tasks: nothing was read"}
	}
	var out []string
	if strings.Contains(text, "uname") {
		out = append(out, "the Taskfile reads the shell's system name (uname)")
	}
	for _, gone := range []string{"\n  namespace:\n", "\n  namespace-caps:\n", "namespace-run", "NS_RUN", "NSC_RUN"} {
		if strings.Contains(text, gone) {
			out = append(out, fmt.Sprintf("the Taskfile still has %q, which went with the Linux-only namespace tasks", gone))
		}
	}
	for _, p := range []struct{ what, re string }{
		{"an `eq OS` branch", `\beq\s+OS\b`},
		{"PATHSEP, a PATH separator picked per system (gragas-dev pnpm joins PATH in Go)", `PATHSEP`},
		{"a `platforms:` key", `(?m)^\s*platforms:`},
	} {
		if regexp.MustCompile(p.re).MatchString(text) {
			out = append(out, "the Taskfile has "+p.what+": the tiers name no other machine")
		}
	}
	return out
}

// TestTheTaskfileNamesNoOtherMachine holds osProblems of the Taskfile, and plants each thing it
// refuses in a copy of it: each is named, and a text with no task is refused.
func TestTheTaskfileNamesNoOtherMachine(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	if got := osProblems(text); len(got) != 0 {
		t.Errorf("the Taskfile: %v", got)
	}
	for _, p := range []struct{ name, add, want string }{
		{"an eq OS branch", "\n  planted:\n    cmds:\n      - echo {{if eq OS \"linux\"}}x{{end}}\n", "`eq OS`"},
		{"a platforms key", "\n  planted:\n    platforms: [linux]\n    cmds:\n      - echo x\n", "`platforms:`"},
		{"PATHSEP", "\n  planted:\n    cmds:\n      - echo {{.PATHSEP}}\n", "PATHSEP"},
		{"a namespace task", "\n  namespace:\n    cmds:\n      - echo x\n", "namespace"},
		{"uname", "\n  planted:\n    cmds:\n      - uname\n", "uname"},
	} {
		if got := strings.Join(osProblems(text+p.add), "\n"); !strings.Contains(got, p.want) {
			t.Errorf("%s planted: want a finding naming %s, got %q", p.name, p.want, got)
		}
	}
	for _, empty := range []string{"", "version: \"3\"\n"} {
		if got := osProblems(empty); len(got) != 1 || !strings.Contains(got[0], "no tasks") {
			t.Errorf("a Taskfile with no task, %q, is not refused: %v", empty, got)
		}
	}
}

// shellNull is the null file as a Taskfile's shell text spells it: the shell
// go-task runs reads it the same on every system, so it is no path of this
// one, and the test does not take os.DevNull for it.
const shellNull = "/de" + "v/null"

func wideGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "-c", "user.name=t", "-c", "user.email=t@users.noreply.github.com", "-c", "commit.gpgsign=false")
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = dir
	cmd.Env = gitenv.Env(configtest.Inherited(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestTheTiersCarryTheirPreflightsTheSkipAndTheGateTestsRule holds, as text, what
// the first review found missing: the tools preflight before any pinned program
// runs, a gate-tests that decides
// with scripts/when-touched (a missing base runs it), and wide-packages among the push members (gragas#1349).
func TestTheTiersCarryTheirPreflightsTheSkipAndTheGateTestsRule(t *testing.T) {
	t.Parallel()
	if !strings.Contains(taskSection(t, "pre-commit"), "- task: tools-check\n        vars: {NO_NETWORK: \"1\"}\n        timeout: 1m\n") || !budgetsMembers(taskSection(t, "pre-commit"), "pre-commit", "pre-commit-run") {
		t.Error("pre-commit does not start with the tools preflight")
	}
	push := taskSection(t, "pre-push")
	if !strings.Contains(push, "cmds:\n      - task: gate-build\n        timeout: 5m\n      # tools-check stays first and alone: the members run the tools it pins, and a member on an unpinned tool is worse than the time it saves (gragas#1349).\n      - task: tools-check\n        timeout: 1m\n") {
		t.Error("pre-push does not start with gate-build (gragas#2367), then tools-check alone, then pre-push-members")
	}
	gate := taskSection(t, "gate-tests")
	if strings.Contains(gate, "when-touched") || !strings.Contains(gate, "-- -count=1 -timeout {{.TEST_TIMEOUT}} {{.GATE_PKGS}}") {
		t.Error("gate-tests has a path filter or does not run again (-count=1): it is a release member with neither (gragas#910, RULED)")
	}
	if strings.Contains(push, "gate-tests") || !strings.Contains(taskSection(t, "release-members"), "- task: gate-tests") {
		t.Error("gate-tests is not in release alone")
	}
}

// taskCmds are the entries of the cmds list of a task's text, each without
// its "- ", in order; an entry of more than one line is its first line.
func taskCmds(section string) []string {
	_, rest, ok := strings.Cut(section, "\n    cmds:\n")
	if !ok {
		return nil
	}
	var out []string
	for line := range strings.SplitSeq(rest, "\n") {
		if entry, ok := strings.CutPrefix(line, "      - "); ok {
			out = append(out, strings.TrimSpace(entry))
			continue
		}
		// A blank line does not end a YAML list; a line less indented does.
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "        ") {
			break
		}
	}
	return out
}

// hatchTestsRun is how every test member starts its run: hatch tests with the flags of HATCH_TESTS_FLAGS, then go
// test's own after the -- (gragas#1471).
const hatchTestsRun = "{{.TOOLS}}/hatch{{exeExt}} tests {{.HATCH_TESTS_FLAGS}} -- "

// hatchFlagsProblems holds the flags every test member passes hatch tests (gragas#1471), the value of
// HATCH_TESTS_FLAGS: red when go test fails or a test fails, when no test ran (a -run that matches nothing, or a
// GOFLAGS that empties the run, exits 0 in go test) and when every test skipped, and the whole run killed, tree and
// all, at HATCH_TIMEOUT, which timeoutProblems holds above go test's TEST_TIMEOUT.
func hatchFlagsProblems(flags string) []string {
	var bad []string
	m := regexp.MustCompile(`(?:^| )--red-when ([a-z,-]+)(?: |$)`).FindStringSubmatch(flags)
	if m == nil {
		return append(bad, fmt.Sprintf("HATCH_TESTS_FLAGS %q names no --red-when: hatch tests would judge the run by its default", flags))
	}
	when := strings.Split(m[1], ",")
	for _, w := range []string{"failed", "none-ran", "all-skipped"} {
		if !slices.Contains(when, w) {
			bad = append(bad, fmt.Sprintf("HATCH_TESTS_FLAGS's --red-when %s leaves out %s: a run that %s would pass", m[1], w, map[string]string{
				"failed": "fails", "none-ran": "ran no test (a renamed test under -run)", "all-skipped": "skipped every test"}[w]))
		}
	}
	if !regexp.MustCompile(`(?:^| )--timeout \{\{\.HATCH_TIMEOUT\}\}(?: |$)`).MatchString(flags) {
		bad = append(bad, fmt.Sprintf("HATCH_TESTS_FLAGS %q does not kill the run at --timeout {{.HATCH_TIMEOUT}}: at go test's own, hatch ends a hung run before go test prints its stacks", flags))
	}
	return bad
}

// landingProblems is what is wrong with the landing task, the goldens run it
// adds and the push's own goldens member, given their texts and the flags every
// test member passes hatch tests. task landing is
// the lander's one proof on the merged tree (gates.land.1): it runs the push
// tier, then full (gragas#1619), then landing-goldens, nothing before, between or after them.
// landing-goldens is compared afresh, never served from the test cache (a
// cached journeys run replays green over a broken gragas, gragas#1222), and
// neither goldens run is ever rewritten.
func landingProblems(landing, goldens, pushGoldens, flags string) []string {
	var out []string
	if got, want := taskCmds(landing), []string{"task: gate-build", `cmd: '{{.GATE}} landing begin --base {{.LANDING_BASE | default "main"}}'`, "task: pre-push", "task: warm", "task: full", "task: landing-goldens"}; !slices.Equal(got, want) {
		out = append(out, fmt.Sprintf("landing runs %q, want %q: landing begin, the push tier, warm, full, and then the goldens afresh, and nothing else", got, want))
	}
	for name, text := range map[string]string{"landing-goldens": goldens, "journeys-goldens": pushGoldens} {
		for line := range strings.SplitSeq(text, "\n") {
			// The description says "(no -update)"; every other line, a var's
			// included, is what runs.
			if strings.Contains(line, "-update") && !strings.HasPrefix(line, "    desc:") {
				out = append(out, name+" names -update: the lander would land over goldens it just rewrote")
			}
		}
	}
	var run []string
	for line := range strings.SplitSeq(goldens, "\n") {
		if strings.Contains(line, "go test") || strings.Contains(line, "hatch") || strings.Contains(line, "gotestsum") {
			run = append(run, strings.TrimSpace(line))
		}
	}
	const head = hatchTestsRun + "-count=1 "
	switch {
	case len(run) != 1:
		out = append(out, fmt.Sprintf("landing-goldens has %d test runs, want 1", len(run)))
	case !strings.HasPrefix(run[0], head) || strings.Count(run[0], "-count=") != 1:
		out = append(out, fmt.Sprintf("landing-goldens runs %q: it must start %s and name no other count, or the test cache replays the goldens green (gragas#1222)", run[0], head))
	case !strings.Contains(run[0], "-run '{{.GOLDEN_RUN}}'") || !strings.HasSuffix(run[0], " ./internal/journeys"):
		out = append(out, fmt.Sprintf("landing-goldens runs %q: it must run the goldens list of ./internal/journeys and nothing after it, so hatch tests's red is the member's", run[0]))
	}
	// A rename of the goldens tests leaves -run matching nothing: go test prints
	// "no tests to run" and exits 0. hatch tests
	// holds that red when its flags name none-ran.
	for _, p := range hatchFlagsProblems(flags) {
		out = append(out, "landing-goldens: "+p)
	}
	return out
}

// TestLandingRunsThePushTierThenTheGoldensAfreshAndNeverRewritesThem holds
// task landing as text: with the judge and
// scripts/land gone, nothing else turns red when landing loses the push tier,
// or its goldens run gains -update or loses -count=1. Each of those is planted
// over the Taskfile's own text and must be found; the text as it is must not.
func TestLandingRunsThePushTierThenTheGoldensAfreshAndNeverRewritesThem(t *testing.T) {
	t.Parallel()
	landing, goldens, push := taskSection(t, "landing"), taskSection(t, "landing-goldens"), taskSection(t, "journeys-goldens")
	flags := varLine(read(t, "Taskfile.yml"), "HATCH_TESTS_FLAGS")
	for _, p := range landingProblems(landing, goldens, push, flags) {
		t.Error(p)
	}
	plants := []struct {
		name                          string
		landing, goldens, push, flags string
	}{
		{"pre-push dropped", strings.Replace(landing, "      - task: pre-push\n", "", 1), goldens, push, flags},
		{"full dropped", regexp.MustCompile(`(?m)^      - task: full\n        timeout: \S+\n`).ReplaceAllString(landing, ""), goldens, push, flags},
		{"full before pre-push", strings.Replace(strings.Replace(landing, "task: pre-push", "task: X", 1), "task: full", "task: pre-push", 1), goldens, push, flags},
		{"the goldens before pre-push", strings.Replace(strings.Replace(landing, "task: pre-push", "task: X", 1), "task: landing-goldens", "task: pre-push", 1), goldens, push, flags},
		{"--base dropped", strings.Replace(landing, ` --base {{.LANDING_BASE | default "main"}}`, "", 1), goldens, push, flags},
		{"a member after the goldens", landing + "      - task: gen\n", goldens, push, flags},
		{"the goldens dropped", strings.Replace(landing, "      - task: landing-goldens\n", "", 1), goldens, push, flags},
		{"the push's cached goldens in place of the fresh ones", strings.Replace(landing, "task: landing-goldens", "task: journeys-goldens", 1), goldens, push, flags},
		{"-update added", landing, strings.Replace(goldens, "./internal/journeys", "./internal/journeys -update", 1), push, flags},
		{"-update in the environment", landing, strings.Replace(goldens, "{{.TOOLS}}/hatch{{exeExt}} tests", "GOFLAGS=-update {{.TOOLS}}/hatch{{exeExt}} tests", 1), push, flags},
		{"-update in a var", landing, strings.Replace(goldens, "    cmds:\n", "    vars:\n      ARGS: -update\n    cmds:\n", 1), push, flags},
		{"-update in the push's goldens", landing, goldens, strings.Replace(push, "./internal/journeys", "./internal/journeys -update", 1), flags},
		{"-count=1 dropped", landing, strings.Replace(goldens, "-- -count=1 ", "-- ", 1), push, flags},
		{"a second count", landing, strings.Replace(goldens, "./internal/journeys", "-count=2 ./internal/journeys", 1), push, flags},
		{"another list", landing, strings.Replace(goldens, "'{{.GOLDEN_RUN}}'", "'{{.SMOKE_RUN}}'", 1), push, flags},
		{"a plain go test", landing, strings.Replace(goldens, "{{.TOOLS}}/hatch{{exeExt}} tests {{.HATCH_TESTS_FLAGS}} --", "go test", 1), push, flags},
		{"flags of their own", landing, strings.Replace(goldens, "{{.HATCH_TESTS_FLAGS}}", "--red-when failed", 1), push, flags},
		{"a red not the member's", landing, strings.Replace(goldens, "./internal/journeys\n", "./internal/journeys || true\n", 1), push, flags},
		{"none-ran dropped", landing, goldens, push, strings.Replace(flags, "none-ran,", "", 1)},
		{"all-skipped dropped", landing, goldens, push, strings.Replace(flags, ",all-skipped", "", 1)},
		{"failed dropped", landing, goldens, push, strings.Replace(flags, "failed,", "", 1)},
		{"no --red-when", landing, goldens, push, regexp.MustCompile(`--red-when \S+ ?`).ReplaceAllString(flags, "")},
		{"the run's timeout dropped", landing, goldens, push, strings.Replace(flags, "--timeout {{.HATCH_TIMEOUT}} ", "", 1)},
		{"the run's timeout go test's", landing, goldens, push, strings.Replace(flags, "--timeout {{.HATCH_TIMEOUT}} ", "--timeout {{.TEST_TIMEOUT}} ", 1)},
	}
	for _, p := range plants {
		if p.landing == landing && p.goldens == goldens && p.push == push && p.flags == flags {
			t.Errorf("%s: the plant changed nothing; the Taskfile's text moved under it", p.name)
			continue
		}
		if len(landingProblems(p.landing, p.goldens, p.push, p.flags)) == 0 {
			t.Errorf("%s: planted, and the check found nothing", p.name)
		}
	}
}

// TestEachRunMemberGivesHatchItsPackages holds, as text,
// that a hatch tests command of more than one line continues each line, so the package list
// is an argument of it and not a command of its own.
func TestEachRunMemberGivesHatchItsPackages(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"test", "test-uncached"} {
		lines := strings.Split(taskSection(t, name), "\n")
		for i, l := range lines {
			if strings.Contains(l, "tests {{.HATCH_TESTS_FLAGS}}") && !strings.HasSuffix(strings.TrimSpace(l), `\`) {
				t.Errorf("%s: the hatch tests line %q does not continue to its packages (line %d)", name, strings.TrimSpace(l), i+1)
			}
		}
	}
}

// listVarScript is the shell text of a var of Taskfile.yml that reads a list file.
func listVarScript(t *testing.T, name string) string {
	t.Helper()
	text := read(t, "Taskfile.yml")
	_, rest, ok := strings.Cut(text, "\n  "+name+":\n    sh: |\n")
	if !ok {
		t.Fatalf("the Taskfile has no var %s with a shell script", name)
	}
	var out []string
	for line := range strings.SplitSeq(rest, "\n") {
		if !strings.HasPrefix(line, "      ") {
			break
		}
		out = append(out, strings.TrimPrefix(line, "      "))
	}
	return strings.Join(out, "\n")
}

// TestTheListVarsKeepALastLineThatHasNoNewline plants each list without a final newline and
// runs the Taskfile's own loop: the last name must still be in the pattern, or a protected file
// that lost its final newline would leave a name out of the run.
func TestTheListVarsKeepALastLineThatHasNoNewline(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, file, list, want string }{
		{"SMOKE_RUN", "internal/journeys/smoke.txt", "help TestHelp\nrun TestRun", `^(TestHelp|TestRun)$`},
		{"GOLDEN_RUN", "internal/journeys/goldens.txt", "# note\nTestA\nTestB", `^(TestA|TestB)$`},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(c.file)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, c.file), []byte(c.list), 0o600); err != nil {
			t.Fatal(err)
		}
		script := filepath.Join(dir, "var.sh")
		if err := os.WriteFile(script, []byte(listVarScript(t, c.name)), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(t.Context(), "sh")
		cmd.Args = append(cmd.Args, script)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := strings.TrimSpace(string(out)); got != c.want {
			t.Errorf("%s over a list with no final newline = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestNoTierMemberIsServedFromGoTasksChecksumsAndNoTaskNamesTheHooksPath
// (gragas#911): go-task's sources and generates make a member skip as "up to
// date" on a checksum of fewer files than it reads, a stamp under another
// name; Go's own caches key a package on what it reads. And no task writes
// git config or the hooks folder: a clone arms its hooks with one command,
// git config core.hooksPath hooks (docs/checks.md, Setup). It is also the
// case of two worktrees at one tree: with no checksum or stamp to share, both
// run every member. And no member is wrapped in a network
// block (unshare --net, sandbox-exec, gragas-dev offline, a firewall): the loopback tests of #447
// listen on 127.0.0.1 and must run in full under every tier. A status: is allowed in one shape only, git diff against
// the base (gragas#1159), held by TestOnlyTheGitDiffStatusIsOnAMemberAndPreconditionsAreTheReleases.
func TestNoTierMemberIsServedFromGoTasksChecksumsAndNoTaskNamesTheHooksPath(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	for _, key := range regexp.MustCompile(`(?m)^    (sources|generates|method):`).FindAllString(text, -1) {
		t.Errorf("the Taskfile has %q on a member: go-task would skip it on a checksum of fewer files than it reads", strings.TrimSpace(key))
	}
	for _, block := range regexp.MustCompile(`(?m)(unshare[^\n]*--net|unshare[^\n]*-n\b|sandbox-exec|\boffline\b|\bnft\b|iptables|\bpfctl\b|--network)`).FindAllString(text, -1) {
		t.Errorf("the Taskfile wraps a member in a network block (%q): a loopback test would end early", block)
	}
	// Nothing on a tier line resolves a program from PATH that tools.yaml pins:
	// corepack (the system node's, which cannot run pnpm 12 and writes under
	// ~/.cache) and a bare pnpm are refused; pnpm is {{.TOOLS}}/pnpm{{exeExt}}.
	for _, bad := range pnpmLineProblems(text) {
		t.Error(bad)
	}
	// Only the hooks task writes git config, and only the one line the test
	// below holds it to (gragas#1220); armed reads the path and writes none.
	rest := strings.Replace(strings.Replace(text, taskSection(t, "hooks"), "", 1), taskSection(t, "armed"), "", 1)
	for _, bad := range []string{"core.hooksPath", ".git/hooks", "git config"} {
		if strings.Contains(rest, bad) {
			t.Errorf("the Taskfile names %q outside the hooks and armed tasks: nothing else writes git config or the hooks folder", bad)
		}
	}
}

// TestAToolNotBuiltFromTheLockIsRedAndThePinnedTaskIsNot (supply.install.10,
// gragas#911): tools-check reads each binary of .tools/bin with go version
// -m and holds its module, version and sum to a line of tools/go.sum. A
// stale binary, built outside the lock as a release download or an older pin
// is, is red and named; the pinned task is not. The Taskfile and the lock
// are the repository's, copied to a scratch folder with a .tools/bin of the
// test's own.
func TestAToolNotBuiltFromTheLockIsRedAndThePinnedTaskIsNot(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	gate := gateBinary(t, abs)
	pinned := pinnedTask(t)
	// Each tool by the name tools-check looks for: task.exe on Windows.
	exe := goEXE(t)
	dir := t.TempDir()
	for name, text := range map[string]string{"Taskfile.yml": read(t, "Taskfile.yml"), "tools/go.sum": read(t, "tools/go.sum"),
		"internal/journeys/smoke.txt": read(t, "internal/journeys/smoke.txt"), "internal/journeys/goldens.txt": read(t, "internal/journeys/goldens.txt")} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A module of one file, built with go build: no sum in its go version -m.
	src := filepath.Join(t.TempDir(), "stale")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"go.mod": "module example.com/stale\n\ngo 1.24\n", "main.go": "package main\n\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(dir, ".tools", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	build := exec.CommandContext(t.Context(), "go")
	build.Args = append(build.Args, "build", "-o", filepath.Join(bin, "task"+exe), ".")
	build.Dir = src
	build.Env = append(configtest.Inherited(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	stale, err := os.ReadFile(filepath.Join(bin, "task"+exe))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"gitleaks", "govulncheck", "go-licenses", "dupl", "hatch"} {
		filestest.WriteProgram(t, filepath.Join(bin, n+exe), stale)
	}
	check := func() (string, error) {
		cmd := exec.CommandContext(t.Context(), pinned)
		cmd.Args = append(cmd.Args, "tools-check", "GATE="+gate, "RELEASE_TOOLS=", "PNPM_TREES=")
		cmd.Dir = dir
		cmd.Env = append(configtest.Inherited(), toolchainPin(t), "PATH="+filepath.Dir(pinned)+string(os.PathListSeparator)+inheritedPath())
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := check()
	if err == nil || !strings.Contains(out, "tools-check: task in") || !strings.Contains(out, "not built from tools/go.sum") {
		t.Errorf("a stale task: err %v, output %q, want red naming task", err, out)
	}
	pinnedBytes, err := os.ReadFile(pinned)
	if err != nil {
		t.Fatal(err)
	}
	filestest.WriteProgram(t, filepath.Join(bin, "task"+exe), pinnedBytes)
	out, err = check()
	if err == nil || strings.Contains(out, "tools-check: task in") || !strings.Contains(out, "tools-check: gitleaks in") {
		t.Errorf("the pinned task and a stale gitleaks: err %v, output %q, want red naming gitleaks and not task", err, out)
	}
}

// TestAToolBuiltAgainstOtherDependenciesThanTheLockIsRedAndTheLocksDepsAreNot (supply.install.10,
// gragas#911): a binary whose own module is the lock's but whose dep line
// names a version that is no line of tools/go.sum, as a build on an older
// resolution has, is red and named; the same binary with every dep a line of
// the lock is not. A go of the test's own answers go version -m with the
// build info it is given, so the plant needs no second module to build.
func TestAToolBuiltAgainstOtherDependenciesThanTheLockIsRedAndTheLocksDepsAreNot(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	gate := gateBinary(t, abs)
	pinned := pinnedTask(t)
	exe := goEXE(t)
	sum := read(t, "tools/go.sum")
	var lock []string
	for line := range strings.SplitSeq(sum, "\n") {
		if f := strings.Fields(line); len(f) == 3 && !strings.HasSuffix(f[1], "/go.mod") {
			lock = append(lock, line)
		}
	}
	if len(lock) < 3 {
		t.Fatalf("tools/go.sum has %d module lines", len(lock))
	}
	tab := func(line string) string { return strings.ReplaceAll(line, " ", "\t") }
	dir := t.TempDir()
	bin := filepath.Join(dir, ".tools", "bin")
	fake := filepath.Join(dir, "fakebin")
	for _, d := range []string{bin, fake, filepath.Join(dir, "tools")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range map[string]string{"Taskfile.yml": read(t, "Taskfile.yml"), "tools/go.sum": sum,
		"internal/journeys/smoke.txt": read(t, "internal/journeys/smoke.txt"), "internal/journeys/goldens.txt": read(t, "internal/journeys/goldens.txt")} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The go of the test: go version -m FILE prints the build info in FILE.info,
	// and anything else (the golangci-lint fetch is a go run) prints nothing and
	// succeeds. It is a built program, not a shell script, so it is the go
	// tools-check finds on PATH on Windows too, where a program is go.exe and a
	// shell script is not run (gragas#1391); fakeTool is that program.
	fakeSrc := t.TempDir()
	for name, text := range map[string]string{"go.mod": "module example.com/fakego\n\ngo 1.24\n", "main.go": fakeTool} {
		if err := os.WriteFile(filepath.Join(fakeSrc, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fakeGo := exec.CommandContext(t.Context(), "go")
	fakeGo.Args = append(fakeGo.Args, "build", "-o", filepath.Join(fake, "go"+exe), ".")
	fakeGo.Dir = fakeSrc
	fakeGo.Env = append(configtest.Inherited(), "CGO_ENABLED=0")
	if out, err := fakeGo.CombinedOutput(); err != nil {
		t.Fatalf("go build of the fake go: %v\n%s", err, out)
	}
	names := []string{"task", "gitleaks", "govulncheck", "go-licenses", "dupl", "hatch"}
	// tools-check reads tools.yaml for each go tool's sha256 (gragas#1428); each row here says no-sha256, so the lock alone
	// holds them, and a row that said nothing would be red (gragas#1480).
	rows := "tools:\n"
	for _, n := range names {
		rows += "  - id: " + n + "\n    from: go\n    no-sha256: \"the lock alone holds it here\"\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	info := func(deps ...string) string {
		out := "x: go1.27.4\n\tpath\texample.com/x\n\tmod\t" + tab(lock[0]) + "\n"
		for _, d := range deps {
			out += "\tdep\t" + d + "\n"
		}
		return out
	}
	for _, n := range names {
		configtest.Fake(t, bin, n)
		// tools-check runs go version -m on programName's path, n.exe on
		// Windows, and the fake go cats that path plus .info.
		if err := os.WriteFile(filepath.Join(bin, n+exe+".info"), []byte(info(tab(lock[1]), tab(lock[2]))), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := func() (string, error) {
		cmd := exec.CommandContext(t.Context(), pinned)
		cmd.Args = append(cmd.Args, "tools-check", "GATE="+gate, "RELEASE_TOOLS=", "PNPM_TREES=")
		cmd.Dir = dir
		cmd.Env = append(configtest.Inherited(), toolchainPin(t), "PATH="+fake+string(os.PathListSeparator)+filepath.Dir(pinned)+string(os.PathListSeparator)+inheritedPath())
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := check(); err != nil {
		t.Errorf("every dep a line of the lock: %v\n%s", err, out)
	}
	other := filepath.Join(bin, "gitleaks"+exe+".info")
	if err := os.WriteFile(other, []byte(info(tab(lock[1]), "github.com/charmbracelet/lipgloss\tv0.5.0\th1:planted=")), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := check()
	if err == nil || !strings.Contains(out, "tools-check: gitleaks in") || strings.Contains(out, "tools-check: task in") {
		t.Errorf("a dep that is no line of the lock: err %v, output %q, want red naming gitleaks alone", err, out)
	}
	repl := info(tab(lock[1])) + "\tdep\texample.com/r\tv1.0.0\t\n\t=>\texample.com/r\tv1.0.1\th1:planted=\n"
	if err := os.WriteFile(other, []byte(repl), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = check()
	if err == nil || !strings.Contains(out, "tools-check: gitleaks in") {
		t.Errorf("a replacement that is no line of the lock: err %v, output %q, want red naming gitleaks", err, out)
	}
}

// TestGolangciLintHasOnePinTheReleaseSha (gragas#911):
// golangci-lint is a release program, so tools.yaml's sha256 is its
// one pin. It is not in the Taskfile's go tools, nor in tools/go.mod's tool
// block, and tools and tools-check both name gragas-dev fetch for it, which
// holds the archive and the program to that sha256 (cmd/gragas-dev).
func TestGolangciLintHasOnePinTheReleaseSha(t *testing.T) {
	t.Parallel()
	task := read(t, "Taskfile.yml")
	for line := range strings.SplitSeq(task, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "GO_TOOLS:") && strings.Contains(line, "golangci") {
			t.Errorf("GO_TOOLS builds golangci-lint: %s", line)
		}
	}
	if want := "{{.GATE}} fetch golangci-lint --platform {{OS}}/{{ARCH}}\""; !strings.Contains(taskSection(t, "tools"), want) {
		t.Errorf("tools has no %s", want)
	}
	if !strings.Contains(task, "RELEASE_TOOLS: golangci-lint ") || !strings.Contains(taskSection(t, "tools-check"), "{{.RELEASE_TOOLS}}") {
		t.Error("tools-check does not hand golangci-lint to gragas-dev tools-check through RELEASE_TOOLS")
	}
	if strings.Contains(read(t, "tools/go.mod"), "cmd/golangci-lint") {
		t.Error("tools/go.mod's tool block holds golangci-lint, a second pin of a release program")
	}
}

// pnpmLineProblems are the lines of a Taskfile that run corepack, or a pnpm
// that is not the pinned {{.TOOLS}}/pnpm{{exeExt}}, or the pinned one with pnpm's
// switch to another version (manage-package-manager-versions) left on. A
// desc and a comment name pnpm in prose and are skipped, and so is the fetch
// of its pin.
func pnpmLineProblems(text string) []string {
	var out []string
	word := regexp.MustCompile(`(^|[^\w./-])pnpm\b`)
	for line := range strings.SplitSeq(text, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "desc:") {
			continue
		}
		if strings.Contains(l, "corepack") {
			out = append(out, "a task line runs corepack, which is no pinned program: "+l)
		}
		if strings.Contains(l, "{{.GATE}} fetch pnpm ") || strings.Contains(l, "{{.GATE}} tools-check ") || strings.Contains(l, "{{.GATE}} pnpm -C ") || strings.HasPrefix(l, "RELEASE_TOOLS:") {
			continue
		}
		if word.MatchString(l) {
			out = append(out, "a task line names a pnpm that is not the pinned {{.TOOLS}}/pnpm{{exeExt}}: "+l)
		}
		if strings.Contains(l, "{{.TOOLS}}/pnpm{{exeExt}}") && !strings.Contains(l, "npm_config_manage_package_manager_versions=false ") {
			out = append(out, "a pnpm line leaves manage-package-manager-versions on, so package.json's packageManager could hand it to another pnpm: "+l)
		}
	}
	return out
}

func TestAPnpmLineThatIsBareOrLeavesTheVersionSwitchOnIsRedAndThePinnedOneIsNot(t *testing.T) {
	t.Parallel()
	const pinned = `      - '{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run test'`
	if got := pnpmLineProblems(pinned + "\n      - \"{{.GATE}} fetch pnpm --platform x\"\n    desc: pnpm is pinned\n"); len(got) != 0 {
		t.Fatalf("the pinned line is green: %v", got)
	}
	for _, bad := range []string{
		"      - cd explorer && corepack pnpm run test",
		"      - cd explorer && pnpm run test",
		"      - pnpm --dir explorer run test",
		"      - pnpm -C explorer install",
		"      - npx pnpm run test",
		"      - cd explorer && \"{{.TOOLS}}/pnpm{{exeExt}}\" run test",
	} {
		if got := pnpmLineProblems(bad); len(got) == 0 {
			t.Errorf("%q must be red", bad)
		}
	}
}

// TestEveryPackageManagerNamesTheOnePnpmToolsYamlPins: with pnpm's own switch
// off, a packageManager that names another version is a version nothing
// fetches or ages; it is held to tools.yaml's.
func TestEveryPackageManagerNamesTheOnePnpmToolsYamlPins(t *testing.T) {
	t.Parallel()
	want := pnpmPin(t, read(t, "tools.yaml"))
	for _, f := range []string{"explorer/package.json", "tools/.node/package.json"} {
		m := regexp.MustCompile(`"packageManager":\s*"pnpm@([^"+]+)`).FindStringSubmatch(read(t, f))
		if m == nil || m[1] != want {
			t.Errorf("%s: packageManager is not pnpm@%s, the pin of tools.yaml (%v)", f, want, m)
		}
	}
	if got := pnpmPin(t, "tools:\n  - id: pnpm\n    version: 9.9.9\n  - id: x\n    version: 1.0.0\n"); got != "9.9.9" {
		t.Errorf("the pin is read from the pnpm row: %q", got)
	}
}

func pnpmPin(t *testing.T, yaml string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^  - id: pnpm\n(?:    [^\n]*\n)*?    version: ([^\n]+)`).FindStringSubmatch(yaml)
	if m == nil {
		t.Fatal("tools.yaml has no pnpm row")
	}
	return strings.TrimSpace(m[1])
}

// TestACallersGoFlagsAreRefusedBeforeAnyMemberAndAMemberThatRanNoTestIsRed:
// go-task's env never overrides the
// caller's, so GOFLAGS='-run=^$' would reach every go test and empty the run.
// The Taskfile's tools-check and tools refuse any GOFLAGS but empty or
// -mod=readonly first; and a run that got past them with no test is red in
// hatch tests (none-ran, gragas#1471), which TestAMemberThatRanNoTestIsRed holds.
func TestACallersGoFlagsAreRefusedBeforeAnyMemberAndAMemberThatRanNoTestIsRed(t *testing.T) {
	t.Parallel()
	pinned := pinnedTask(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(read(t, "Taskfile.yml")), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"internal/journeys/smoke.txt", "internal/journeys/goldens.txt"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(read(t, f)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(goflags string, more ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), pinned)
		cmd.Args = append(cmd.Args, "tools-check")
		cmd.Dir = dir
		cmd.Env = append(append(configtest.Inherited(), toolchainPin(t), "GOFLAGS="+goflags, "PATH="+filepath.Dir(pinned)+string(os.PathListSeparator)+inheritedPath()), more...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	for _, flags := range []string{"-run=^$", "-skip=.", "-short", "-mod=readonly -short"} {
		if out, err := run(flags); err == nil || !strings.Contains(out, "GOFLAGS is '"+flags+"'") {
			t.Errorf("GOFLAGS=%q must be refused before any member: err %v, output %q", flags, err, out)
		}
	}
	for _, plant := range []string{"GOWORK=/work/other.work", "GOWORK=", "GOTOOLCHAIN=local", "GOTOOLCHAIN=", "GOTOOLCHAIN=auto", "GOPROXY=https://example.org", "GOPROXY="} {
		name, _, _ := strings.Cut(plant, "=")
		if out, err := run("-mod=readonly", plant); err == nil || !strings.Contains(out, name+" is '") {
			t.Errorf("%s must be refused before any member: err %v, output %q", plant, err, out)
		}
	}
	for _, flags := range []string{"-mod=readonly"} {
		if out, _ := run(flags); strings.Contains(out, "GOFLAGS is") {
			t.Errorf("GOFLAGS=%q is allowed: %q", flags, out)
		}
	}
	if out, err := run(""); err == nil || !strings.Contains(out, "GOFLAGS is ''") {
		t.Errorf("a GOFLAGS the caller sets empty beats env: and must be refused: err %v, %q", err, out)
	}
	tc := regexp.MustCompile(`(?m)^toolchain (go[0-9.]+)$`).FindStringSubmatch(read(t, "go.mod"))
	if tc == nil || !strings.Contains(read(t, "Taskfile.yml"), "  GOTOOLCHAIN: "+tc[1]+"\n") || !strings.Contains(read(t, "Taskfile.yml"), `"${GOTOOLCHAIN:-}" `+tc[1]+"\n") {
		t.Errorf("the Taskfile's GOTOOLCHAIN and its guard are not go.mod's toolchain line %v", tc)
	}
}

// maxprocsGo is a go that writes the GOMAXPROCS it was given, or "unset", to the file MAXPROCS_LOG names, and fails.
// The file is made only if it is not there, so a later call writes nothing: the go command a member runs first, which
// its failure stops, is the one this test reads.
const maxprocsGo = `package main

import "os"

func main() {
	v, ok := os.LookupEnv("GOMAXPROCS")
	if !ok {
		v = "unset"
	}
	f, err := os.OpenFile(os.Getenv("MAXPROCS_LOG"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = f.WriteString(v + "\n")
		_ = f.Close()
	}
	os.Exit(1)
}
`

// TestACallersGOMAXPROCSReachesTheGoCommandsAndNoMemberSetsItsOwn (gragas#1490): slot.mjs shares the machine's
// CPUs between heavy runs by setting GOMAXPROCS, and go's -p defaults to it, so the value must reach every go
// command of every member unchanged. The Taskfile names it nowhere (no env: of its own, no member's GOMAXPROCS=,
// no -p), and with GOMAXPROCS=2 set by the caller the go command of a member that go-task runs is given 2, where
// the env: block and the go guard stand between the two. The go is a fake that writes what it was given.
// Planted: a member's own GOMAXPROCS=8 is red on both counts.
// Risk: a heavy run that takes every CPU of the machine whatever share the slot gave it.
func TestACallersGOMAXPROCSReachesTheGoCommandsAndNoMemberSetsItsOwn(t *testing.T) {
	t.Parallel()
	taskfile := read(t, "Taskfile.yml")
	ownValue := regexp.MustCompile(`GOMAXPROCS|\bgo (?:build|test|vet|run|install|list)\b[^\n]* -p[ =]`)
	if m := ownValue.FindString(taskfile); m != "" {
		t.Errorf("the Taskfile sets a GOMAXPROCS or a -p of its own (%q): the caller's share must reach every go command unchanged", m)
	}
	exe := goEXE(t)
	pinned := pinnedTask(t)
	src, fake := t.TempDir(), t.TempDir()
	for name, text := range map[string]string{"go.mod": "module example.com/maxprocs\n\ngo 1.24\n", "main.go": maxprocsGo} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	build := exec.CommandContext(t.Context(), "go")
	build.Args = append(build.Args, "build", "-o", filepath.Join(fake, "go"+exe), ".")
	build.Dir = src
	build.Env = append(configtest.Inherited(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build of the fake go: %v\n%s", err, out)
	}
	given := func(taskfile string) []string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"internal/journeys/smoke.txt", "internal/journeys/goldens.txt"} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, f), []byte(read(t, f)), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		log := filepath.Join(t.TempDir(), "maxprocs.log")
		cmd := exec.CommandContext(t.Context(), pinned)
		// GATE is the binary gate-build builds (gragas#2367), and this folder has none: tools-check's gate call is the
		// go the test fakes, as it was when GATE was go run.
		cmd.Args = append(cmd.Args, "tools-check", "GATE=go run -trimpath ./cmd/gragas-dev")
		cmd.Dir = dir
		path := fake + string(os.PathListSeparator) + filepath.Dir(pinned) + string(os.PathListSeparator) + inheritedPath()
		cmd.Env = append(configtest.Inherited(), toolchainPin(t), "GOFLAGS=-mod=readonly", "GOMAXPROCS=2", "MAXPROCS_LOG="+log, "PATH="+path)
		out, _ := cmd.CombinedOutput()
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatalf("no go command ran: %v\n%s", err, out)
		}
		return strings.Fields(string(b))
	}
	for _, v := range given(taskfile) {
		if v != "2" {
			t.Errorf("a go command of tools-check was given GOMAXPROCS %s, not the caller's 2", v)
		}
	}
	planted := strings.Replace(taskfile, "      - cmd: '{{.GATE}} tools-check --platform {{OS}}/{{ARCH}} --lock {{.LOCK_FILE}} --pins {{.PINS_FILE}} --bin {{.TOOLS}} --pnpm=",
		"      - cmd: 'GOMAXPROCS=8 {{.GATE}} tools-check --platform {{OS}}/{{ARCH}} --lock {{.LOCK_FILE}} --pins {{.PINS_FILE}} --bin {{.TOOLS}} --pnpm=", 1)
	if planted == taskfile {
		t.Fatal("the plant found no tools-check line to change")
	}
	if ownValue.FindString(planted) == "" {
		t.Error("a member's own GOMAXPROCS=8 is not found in the Taskfile")
	}
	if got := given(planted); !slices.Contains(got, "8") {
		t.Errorf("a member's own GOMAXPROCS=8 did not reach its go command (%v): the fake reads nothing", got)
	}
}

// TestAMemberThatRanNoTestIsRed holds each test member to a run that is red when no test ran (gragas#1471): it runs
// go test through hatch tests with HATCH_TESTS_FLAGS, whose --red-when names none-ran, and writes no file at all. The
// json file and gragas-dev tests count it replaced went under .tools/bin once, which moved that folder's mtime, an
// input of Go's test cache for the packages that read it (gragas#1213); no member writes a json there again.
func TestAMemberThatRanNoTestIsRed(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	for _, p := range hatchFlagsProblems(varLine(text, "HATCH_TESTS_FLAGS")) {
		t.Error(p)
	}
	for _, id := range []string{"test", "test-uncached", "gate-tests", "journeys-smoke", "journeys-goldens", "landing-goldens"} {
		body := taskSection(t, id)
		var runs int
		for line := range strings.SplitSeq(body, "\n") {
			l := strings.TrimSpace(line)
			if !strings.Contains(l, "go test") && !strings.Contains(l, "hatch") && !strings.Contains(l, "gotestsum") {
				continue
			}
			// The package list a run takes is no run (gragas#1470).
			if strings.HasPrefix(l, "$({{.TOOLS}}/hatch{{exeExt}} tests packages ") {
				continue
			}
			runs++
			if !strings.HasPrefix(l, hatchTestsRun) {
				t.Errorf("%s runs %q: every run starts %s, so a run with no test is red", id, l, hatchTestsRun)
			}
		}
		if runs == 0 {
			t.Errorf("%s runs no test through hatch tests", id)
		}
		if strings.Contains(body, "--jsonfile") || strings.Contains(body, "tests count") {
			t.Errorf("%s writes a json file and counts it again: hatch tests holds the stream itself", id)
		}
	}
	// No json of any member goes under .tools/bin, by any name (gragas#1213).
	for _, m := range regexp.MustCompile(`\{\{\.TOOLS\}\}/[A-Za-z0-9._-]+\.json`).FindAllString(text, -1) {
		t.Errorf("the Taskfile writes %s: a file made and removed in .tools/bin moves the folder's mtime, an input of the packages that read it", m)
	}
}

func barePathJoin(text string) []string {
	var out []string
	bare := regexp.MustCompile(`:\$\{?PATH\}?|\$\{?PATH\}?:`)
	for l := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		if bare.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

func TestNoTaskLineJoinsAPathWithABareColonAndTheNodeCheckCallsTheToolsNode(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	if got := barePathJoin(text); len(got) != 0 {
		t.Errorf("a task line joins PATH with a bare colon, which is no separator on Windows; the pinned tools folder goes on PATH through gragas-dev pnpm: %v", got)
	}
	for _, planted := range []string{
		`      - cd explorer && PATH="{{.TOOLS}}:$PATH" "{{.TOOLS}}/pnpm{{exeExt}}" run test`,
		`      - PATH="$PATH:{{.TOOLS}}" run`,
		`      - PATH="{{.TOOLS}}:${PATH}" run`,
	} {
		if len(barePathJoin(planted)) == 0 {
			t.Errorf("%q must be red", planted)
		}
	}
	if len(barePathJoin(`      - '{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run test'`)) != 0 {
		t.Error("the gragas-dev pnpm line is red")
	}
	// gragas-dev tools-check runs node from --bin, by programName's path (gragas#1655).
	if !strings.Contains(text, `{{.GATE}} tools-check --platform {{OS}}/{{ARCH}} --bin {{.TOOLS}} --node-version .node-version`) {
		t.Error("the node version check does not call the tools node by path")
	}
}

// pushMembers are the members of pre-push, which run side by side as the deps of
// pre-push-members and push-checks (gragas#1158): the commit-level checks alone
// (gragas#1619). tools-check comes first and the budget last, in
// pre-push's own cmds. task tools is no part of the push (gragas#1215):
// tools-check holds the tools instead.
func pushMembers() []string {
	return []string{"commit-checks", "build", "secrets", "vet", "lint-diff"}
}

// fullMembers are the members of full, the deps of full-members and full-checks:
// every member the push ran before gragas#1619 but its commit-level ones.
func fullMembers() []string {
	return []string{"wide-packages", "rulebook", "gate-registry", "vuln", "licences", "lint", "test", "journeys-smoke", "journeys-goldens", "explorer-typecheck", "explorer-lint", "explorer-test", "subset-build"}
}

// addedPushMembers are the members the push gained after the cut: gate-lists (gragas#1733), which runs the
// list tests that hold the gate's own lists.
func addedPushMembers() []string {
	return []string{"gate-lists", "generated-files", "os-code", "standin"}
}

// addedFullMembers are the members full gained after the cut: not in the old push, so the union test holds them as
// well as the old push's. None since the namespace tasks went.
func addedFullMembers() []string {
	return nil
}

// oldPushMembers are the members of pre-push as it stood before gragas#1619 cut it
// (main at 7738a562b): what the push and full hold together, but lint-diff.
func oldPushMembers() []string {
	return []string{"wide-packages", "commit-checks", "build", "rulebook", "secrets", "gate-registry", "vuln", "licences", "vet", "lint", "journeys-smoke", "journeys-goldens", "explorer-typecheck", "explorer-lint", "explorer-test", "subset-build", "test"}
}

// budgetsMembers says a tier's text runs its members as the command of its budget (gragas#1779):
// {{.BUDGET}} --tier <tier> --since {{.START}} -- {{.TASK_EXE}} <members>, which hatch holds to the tier's
// cpu_limit and ends red when the members do.
func budgetsMembers(text, tier, members string) bool {
	re := `BUDGET\}\} --tier ` + regexp.QuoteMeta(tier) + ` --since \{\{\.START\}\} -- \{\{\.TASK_EXE\}\} ` + regexp.QuoteMeta(members) + `(\s|'|"|$)`
	return regexp.MustCompile(re).MatchString(text)
}

// pushShapeProblems holds of the text of pre-push, pre-push-members and push-checks that
// pre-push calls tools-check and pre-push-members, in that order, and
// ends on the budget (no pre-commit prefix, gragas#1210; no task tools, gragas#1215);
// that pre-push-members has deps and no cmds; that every push member is among those
// deps and no full member is; and that no member is called in pre-push's cmds and
// no staged member is a dep.
func pushShapeProblems(push, members, checks string) []string {
	var bad []string
	calls := callsIn(push)
	if want := []string{"tools-check"}; !slices.Equal(calls, want) {
		bad = append(bad, fmt.Sprintf("pre-push calls %v in its cmds, want %v: a member belongs in the deps of pre-push-members", calls, want))
	}
	if !budgetsMembers(push, "pre-push", "pre-push-run") {
		bad = append(bad, "pre-push does not run pre-push-members under its budget")
	}
	if !strings.Contains(members, "\n    deps:\n") || strings.Contains(members, "\n    cmds:\n") {
		bad = append(bad, "pre-push-members is not deps alone: its members run in order, not side by side")
	}
	if got, want := callsIn(members), []string{"push-checks", "lint-diff"}; !slices.Equal(got, want) {
		bad = append(bad, fmt.Sprintf("pre-push-members has deps %v, want %v: lint-diff is the push's alone, the rest are push-checks, which the release takes", got, want))
	}
	if !depsAlone(checks) {
		bad = append(bad, "push-checks is not deps alone: its members run in order, not side by side")
	}
	deps := append(callsIn(members), callsIn(checks)...)
	for _, name := range pushMembers() {
		if !slices.Contains(deps, name) {
			bad = append(bad, name+" is not a dep of pre-push-members")
		}
	}
	for _, name := range fullMembers() {
		if slices.Contains(deps, name) || slices.Contains(callsIn(push), name) {
			bad = append(bad, name+" is in the push: it is full's, which landing runs (gragas#1619)")
		}
	}
	for _, name := range []string{"secrets-staged", "pre-commit", "pre-commit-members"} {
		if slices.Contains(deps, name) || slices.Contains(callsIn(push), name) {
			bad = append(bad, name+" is in the push: the stage is empty at push, and secrets scans the pushed commits")
		}
	}
	return bad
}

// TestPrePushRunsItsIndependentMembersAsDeps holds the Taskfile to that shape, and a
// member moved back into pre-push's cmds, or a members task with cmds, to be red.
func TestPrePushRunsItsIndependentMembersAsDeps(t *testing.T) {
	t.Parallel()
	push, members, checks := taskSection(t, "pre-push"), taskSection(t, "pre-push-members"), taskSection(t, "push-checks")
	if bad := pushShapeProblems(push, members, checks); len(bad) != 0 {
		t.Errorf("pre-push (gragas#1158): %v", bad)
	}
	movedBack := strings.Replace(push, "      - task: tools-check\n", "      - task: tools-check\n      - task: vet\n", 1)
	if len(pushShapeProblems(movedBack, members, checks)) == 0 {
		t.Error("a member called in pre-push's cmds is not red")
	}
	inOrder := strings.Replace(members, "    deps:\n", "    cmds:\n", 1)
	if len(pushShapeProblems(push, inOrder, checks)) == 0 {
		t.Error("pre-push-members with cmds in place of deps is not red")
	}
	missing := strings.Replace(checks, "      - task: vet\n", "", 1)
	if len(pushShapeProblems(push, members, missing)) == 0 {
		t.Error("an independent member missing from the deps is not red")
	}
	for _, name := range []string{"test", "rulebook", "vuln"} {
		heavy := strings.Replace(checks, "    deps:\n", "    deps:\n      - task: "+name+"\n", 1)
		if len(pushShapeProblems(push, members, heavy)) == 0 {
			t.Errorf("%s, a member of full, planted in push-checks is not red (gragas#1619)", name)
		}
	}
	noDiff := strings.Replace(members, "      - task: lint-diff\n", "", 1)
	if len(pushShapeProblems(push, noDiff, checks)) == 0 {
		t.Error("a push without lint-diff is not red (gragas#1619)")
	}
	wideFirst := strings.Replace(push, "      - task: tools-check\n", "      - task: tools-check\n      - task: wide-packages\n", 1)
	if len(pushShapeProblems(wideFirst, members, checks)) == 0 {
		t.Error("wide-packages called in pre-push's cmds, ahead of the members, is not red (gragas#1349)")
	}
	noChecks := strings.Replace(checks, "      - task: commit-checks\n", "", 1)
	if len(pushShapeProblems(push, members, noChecks)) == 0 {
		t.Error("commit-checks missing from the deps is not red")
	}
	prefix := strings.Replace(push, "      - task: tools-check\n", "      - task: pre-commit\n      - task: tools-check\n", 1)
	if len(pushShapeProblems(prefix, members, checks)) == 0 {
		t.Error("a pre-commit prefix in pre-push is not red (gragas#1210)")
	}
	planted := strings.Replace(checks, "    deps:\n", "    deps:\n      - task: secrets-staged\n", 1)
	if len(pushShapeProblems(push, members, planted)) == 0 {
		t.Error("secrets-staged among the push's deps is not red (gragas#1210)")
	}
}

// fullShapeProblems holds of the text of full, full-members and full-checks (gragas#1619) that
// full calls tools-check and full-members, in that order, and ends on its own budget; that
// full-members is deps alone, full-checks and test; that full-checks is deps alone, with
// wide-packages and without test, which the release leaves out; that every full member is among
// those deps and no push member is.
func fullShapeProblems(full, members, checks string) []string {
	var bad []string
	if got, want := callsIn(full), []string{"tools-check"}; !slices.Equal(got, want) {
		bad = append(bad, fmt.Sprintf("full calls %v in its cmds, want %v: a member belongs in the deps of full-members", got, want))
	}
	if !budgetsMembers(full, "full", "full-run") {
		bad = append(bad, "full does not run full-members under its budget")
	}
	if !depsAlone(members) {
		bad = append(bad, "full-members is not deps alone: its members run in order, not side by side")
	}
	if got, want := callsIn(members), []string{"full-checks", "test"}; !slices.Equal(got, want) {
		bad = append(bad, fmt.Sprintf("full-members has deps %v, want %v: test is the member the release leaves out, the rest are full-checks", got, want))
	}
	if !depsAlone(checks) {
		bad = append(bad, "full-checks is not deps alone: its members run in order, not side by side")
	}
	if !slices.Contains(callsIn(checks), "wide-packages") {
		bad = append(bad, "full-checks has no wide-packages: full and the release tell a wide change beside the members (gragas#1349)")
	}
	if slices.Contains(callsIn(checks), "test") {
		bad = append(bad, "full-checks holds test: the release tier takes full-checks, and test-uncached covers test")
	}
	deps := append(callsIn(members), callsIn(checks)...)
	for _, name := range fullMembers() {
		if !slices.Contains(deps, name) {
			bad = append(bad, name+" is not a dep of full-members")
		}
	}
	for _, name := range pushMembers() {
		if slices.Contains(deps, name) {
			bad = append(bad, name+" is in full: it is the push's, and a member stays in one place")
		}
	}
	return bad
}

// TestFullRunsEveryMemberThePushLeavesOutAsDeps holds the Taskfile to that shape (gragas#1619);
// a member in full's cmds, a member gone from its deps, wide-packages or test gone, and a push
// member planted in are red.
func TestFullRunsEveryMemberThePushLeavesOutAsDeps(t *testing.T) {
	t.Parallel()
	full, members, checks := taskSection(t, "full"), taskSection(t, "full-members"), taskSection(t, "full-checks")
	if bad := fullShapeProblems(full, members, checks); len(bad) != 0 {
		t.Errorf("full (gragas#1619): %v", bad)
	}
	if !strings.Contains(full, "landing runs full on the release machine; a tag needs task release green on the release machine") {
		t.Error("full's desc does not say landing runs it on the release machine and a tag needs the release green on the release machine")
	}
	for _, p := range []struct{ what, in, from, to string }{
		{"a member called in full's cmds", "full", "      - task: tools-check\n", "      - task: tools-check\n      - task: vuln\n"},
		{"vuln gone from full-checks", "checks", "      - task: vuln\n", ""},
		{"wide-packages gone from full-checks", "checks", "      - task: wide-packages\n", ""},
		{"test gone from full-members", "members", "      - task: test\n", ""},
		{"the cached test in full-checks", "checks", "    deps:\n", "    deps:\n      - task: test\n"},
		{"a push member in full-checks", "checks", "    deps:\n", "    deps:\n      - task: build\n"},
		{"full-members with cmds", "members", "    deps:\n", "    cmds:\n"},
	} {
		f, m, c := full, members, checks
		target := map[string]*string{"full": &f, "members": &m, "checks": &c}[p.in]
		if !strings.Contains(*target, p.from) {
			t.Fatalf("%s: no %q", p.what, p.from)
		}
		*target = strings.Replace(*target, p.from, p.to, 1)
		if len(fullShapeProblems(f, m, c)) == 0 {
			t.Errorf("%s is not red", p.what)
		}
	}
}

// tierMemberProblems holds the members the push and full reach through their members tasks
// (gragas#1619): together they are the push's members before the cut, plus lint-diff, and no
// member is in both, so the cut lost no member and runs none twice.
func tierMemberProblems(push, full []string) []string {
	var bad []string
	want := slices.Concat(oldPushMembers(), []string{"lint-diff"}, addedPushMembers(), addedFullMembers())
	for _, name := range want {
		if !slices.Contains(push, name) && !slices.Contains(full, name) {
			bad = append(bad, name+" is in neither the push nor full: the cut lost it")
		}
	}
	for _, name := range append(slices.Clone(push), full...) {
		if !slices.Contains(want, name) {
			bad = append(bad, name+" is in the push or full but was no push member")
		}
	}
	for _, name := range push {
		if slices.Contains(full, name) {
			bad = append(bad, name+" is in both the push and full")
		}
	}
	return bad
}

// TestThePushAndFullTogetherAreThePushBeforeTheCut is the union test of gragas#1619: the
// members of the push and of full are the old push's, plus lint-diff, each in one place. A
// member dropped and a member in both are red.
func TestThePushAndFullTogetherAreThePushBeforeTheCut(t *testing.T) {
	t.Parallel()
	push := slices.Concat(callsIn(taskSection(t, "pre-push-members")), callsIn(taskSection(t, "push-checks")))
	full := slices.Concat(callsIn(taskSection(t, "full-members")), callsIn(taskSection(t, "full-checks")))
	push = slices.DeleteFunc(push, func(n string) bool { return n == "push-checks" })
	full = slices.DeleteFunc(full, func(n string) bool { return n == "full-checks" })
	if bad := tierMemberProblems(push, full); len(bad) != 0 {
		t.Errorf("the push and full (gragas#1619): %v", bad)
	}
	dropped := slices.DeleteFunc(slices.Clone(full), func(n string) bool { return n == "licences" })
	if len(tierMemberProblems(push, dropped)) == 0 {
		t.Error("a member dropped from full is not red")
	}
	if len(tierMemberProblems(push, append(slices.Clone(full), "vet"))) == 0 {
		t.Error("a member in both the push and full is not red")
	}
	if len(tierMemberProblems(append(slices.Clone(push), "names"), full)) == 0 {
		t.Error("a release member planted in the push is not red")
	}
}

// TestFullHoldsTheVulnScanThatLandingRuns: govulncheck left the push for full (gragas#1619), and
// landing runs full before main (supply.scans.7, which rulebook#55 put in the contract's full tier).
// landingProblems holds that landing calls full; this holds that vuln is among full's members,
// and that vuln gone, or moved back to the push, is red.
func TestFullHoldsTheVulnScanThatLandingRuns(t *testing.T) {
	t.Parallel()
	vulnIn := func(full, push []string) []string {
		var bad []string
		if !slices.Contains(full, "vuln") {
			bad = append(bad, "full-checks has no vuln: govulncheck left the push for the full tier landing runs")
		}
		if slices.Contains(push, "vuln") {
			bad = append(bad, "vuln is in the push: it is full's (gragas#1619)")
		}
		return bad
	}
	full, push := callsIn(taskSection(t, "full-checks")), callsIn(taskSection(t, "push-checks"))
	if bad := vulnIn(full, push); len(bad) != 0 {
		t.Errorf("vuln (gragas#1619): %v", bad)
	}
	gone := slices.DeleteFunc(slices.Clone(full), func(n string) bool { return n == "vuln" })
	if len(vulnIn(gone, push)) == 0 {
		t.Error("vuln gone from full is not red")
	}
	if len(vulnIn(full, append(slices.Clone(push), "vuln"))) == 0 {
		t.Error("vuln back in the push is not red")
	}
}

// TestReleaseRunsTheNamesCheckAndPushDoesNot (gragas#1158): the Makefile's
// names target, the rulebook's names check by cmd/gragas-names, is a release
// member. The command reads the settings key and never GRAGAS_NAMES_FILE
// (cmd/gragas-names, TestTheVariableIsNotReadAndTheKeysListIs); the task adds
// nothing to its command line, so the list is the key's alone.
func TestReleaseRunsTheNamesCheckAndPushDoesNot(t *testing.T) {
	t.Parallel()
	if !strings.Contains(taskSection(t, "release-members"), "- task: names\n") {
		t.Error("release does not run the names task")
	}
	got := taskSection(t, "names")
	if p := namesTaskProblems(got); len(p) != 0 {
		t.Errorf("the names task (%q): %v", got, p)
	}
	for name, plant := range map[string][2]string{
		"the list read by a variable": {"go run ./cmd/gragas-names", "GRAGAS_NAMES_FILE=x go run ./cmd/gragas-names"},
		"not asked by the shell":      {"go run ./cmd/gragas-names -C rulebook{{if .NAMES_NOT_ASKED}} -not-asked{{end}}", "{{if .NAMES_NOT_ASKED}}echo not asked{{else}}go run ./cmd/gragas-names -C rulebook{{end}}"},
		"another variable skips it":   {"{{if .NAMES_NOT_ASKED}}", "{{if .SNAPSHOT}}"},
		"the flag always on":          {"{{if .NAMES_NOT_ASKED}} -not-asked{{end}}", " -not-asked"},
		"the check gone":              {"go run ./cmd/gragas-names -C rulebook{{if", "true{{if"},
		"the root read":               {" -C rulebook{{if", "{{if"},
	} {
		planted := strings.Replace(got, plant[0], plant[1], 1)
		if planted == got {
			t.Errorf("%s: the plant changed nothing; the Taskfile's text moved under it", name)
			continue
		}
		if len(namesTaskProblems(planted)) == 0 {
			t.Errorf("%s: planted, and the check found nothing", name)
		}
	}
	if strings.Contains(taskSection(t, "pre-push"), "- task: names\n") || strings.Contains(taskSection(t, "pre-push-members"), "- task: names\n") {
		t.Error("pre-push runs the names check: it is the release tier's, with the private list")
	}
}

// TestNoTierRunsTheToolsTaskAndThePushNamesNoNetworkStep (gragas#1215): task
// tools needs the network (it fetches release programs and installs pnpm
// packages), so no tier carries it: a push runs tools-check, which is red
// and names task tools when a pin is stale, and nothing that fetches. A clone
// whose tools are in place is green with no network.
func TestNoTierRunsTheToolsTaskAndThePushNamesNoNetworkStep(t *testing.T) {
	t.Parallel()
	// tools-go, the go builds of task tools (gragas#1480), reads the module cache and may fetch, so no tier runs it either.
	tools := regexp.MustCompile(`(?m)^\s*- task: tools(-go)?\s*$`)
	for _, name := range []string{"pre-commit", "pre-push", "pre-push-members", "commit-checks", "push-checks", "full", "full-members", "full-checks", "release", "release-members"} {
		if tools.MatchString(taskSection(t, name)) {
			t.Errorf("%s runs task tools, which needs the network", name)
		}
	}
	if !strings.Contains(taskSection(t, "pre-commit"), "- task: tools-check") {
		t.Error("pre-commit does not run tools-check, the only thing that is red on a stale pin")
	}
	for _, name := range []string{"tools-check", "pre-commit", "pre-push", "pre-push-members", "commit-checks", "push-checks", "full", "full-members", "full-checks"} {
		section := taskSection(t, name)
		if strings.Contains(section, "{{.GATE}} fetch ") || strings.Contains(section, "pnpm\" install") || strings.Contains(section, "go build -C tools") {
			t.Errorf("%s fetches or installs something", name)
		}
	}
	if !strings.Contains(taskSection(t, "tools-check"), "run task tools") && !strings.Contains(read(t, "cmd/gragas-dev/toolscheck.go"), "run task tools") {
		t.Error("tools-check does not name task tools as the fix")
	}
}

// gateBinary is gragas-dev built from the repository at abs into a folder of
// the test's own, so a Taskfile copied to a scratch folder runs the built gate
// (its GATE var is a go run of ./cmd/gragas-dev, which the scratch folder has
// no source for).
func gateBinary(t *testing.T, abs string) string {
	t.Helper()
	// GATE runs this binary through sh, where a backslash is an escape and a
	// program is found only under its .exe on Windows (gragas#1391); the path is
	// built with that suffix and handed back slash-separated. goEXE and ToSlash
	// are identity on Unix.
	out := filepath.Join(t.TempDir(), "gragas-dev"+goEXE(t))
	build := exec.CommandContext(t.Context(), "go")
	build.Args = append(build.Args, "build", "-trimpath", "-o", out, "./cmd/gragas-dev")
	build.Dir = abs
	build.Env = configtest.Inherited()
	if msg, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build gragas-dev: %v\n%s", err, msg)
	}
	return filepath.ToSlash(out)
}

// TestToolsCheckTakesItsFilesThroughVariables (gragas#1215, #1229): the
// tools-check piece names the lock file, the file that pins the release tools
// and the folder of the tools by variables of the Taskfile, with the values
// this repository keeps them at, so it carries no path of its own to the
// preset another repository includes.
func TestToolsCheckTakesItsFilesThroughVariables(t *testing.T) {
	t.Parallel()
	task := read(t, "Taskfile.yml")
	section := taskSection(t, "tools-check")
	for _, want := range []string{"--lock {{.LOCK_FILE}}", "--pins {{.PINS_FILE}}", "--bin {{.TOOLS}}", "--pnpm={{.PNPM_TREES}}"} {
		if !strings.Contains(section, want) {
			t.Errorf("tools-check does not pass %s", want)
		}
	}
	var cmds []string
	for line := range strings.SplitSeq(section, "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "- ") {
			cmds = append(cmds, l)
		}
	}
	for _, path := range []string{"tools/go.sum", "tools.yaml", ".tools/bin", "cmd/gragas", "explorer", "tools/.node"} {
		if strings.Contains(strings.ReplaceAll(strings.Join(cmds, "\n"), "{{.GATE}}", ""), path) {
			t.Errorf("tools-check names the path %s itself, not through a variable", path)
		}
	}
	for _, want := range []string{"\n  LOCK_FILE: tools/go.sum\n", "\n  PINS_FILE: tools.yaml\n"} {
		if !strings.Contains(task, want) {
			t.Errorf("the Taskfile has no var line %q", strings.TrimSpace(want))
		}
	}
}

// varLine is the value of the Taskfile's top-level var name, or "".
func varLine(task, name string) string {
	m := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(name) + `: (.*)$`).FindStringSubmatch(task)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// TestToolsCheckHoldsWhatTaskToolsInstalls (gragas#1088, the review of PR
// 1124): the release tools tools-check holds, RELEASE_TOOLS, are the ids
// task tools fetches, and the pnpm trees it holds, PNPM_TREES, are the
// folders task tools installs with pnpm, by its call of tools-pnpm, the one
// task with the installs (gragas#1522). A tool or tree task tools installs
// and tools-check does not hold would go stale with the push green.
func TestToolsCheckHoldsWhatTaskToolsInstalls(t *testing.T) {
	t.Parallel()
	task := read(t, "Taskfile.yml")
	tools := taskSection(t, "tools")
	if !regexp.MustCompile(`(?m)^\s+- task: tools-pnpm$`).MatchString(tools) {
		t.Error("task tools does not call tools-pnpm, which holds the pnpm installs")
	}
	var fetched, installed []string
	for _, m := range regexp.MustCompile(`\{\{\.GATE\}\} fetch ([a-z0-9-]+) `).FindAllStringSubmatch(tools, -1) {
		fetched = append(fetched, m[1])
	}
	for _, m := range regexp.MustCompile(`(?m)^\s+- (?:cmd: )?'\{\{\.GATE\}\} pnpm -C (\S+) --tools "\{\{\.TOOLS\}\}" -- install `).FindAllStringSubmatch(taskSection(t, "tools-pnpm"), -1) {
		installed = append(installed, m[1])
	}
	held := strings.Fields(varLine(task, "RELEASE_TOOLS"))
	slices.Sort(fetched)
	slices.Sort(held)
	if len(fetched) == 0 || !slices.Equal(fetched, held) {
		t.Errorf("task tools fetches %v and tools-check holds RELEASE_TOOLS %v", fetched, held)
	}
	trees := strings.Split(varLine(task, "PNPM_TREES"), ",")
	slices.Sort(installed)
	slices.Sort(trees)
	if len(installed) == 0 || !slices.Equal(installed, trees) {
		t.Errorf("task tools installs %v with pnpm and tools-check holds PNPM_TREES %v", installed, trees)
	}
}

// TestNoMemberHoldsAReleaseAgeWindow: gragas holds itself to no dependency
// release-age window. No task is named for it, none runs the gate's age command (which is gone), and none reads a
// pnpm lock release's publish time. The planted text is one that names each.
//
// What it costs when this fails: a pin younger than seven days, which the operator ruled no longer waits, is red
// again at the push, at landing or in task tools.
func TestNoMemberHoldsAReleaseAgeWindow(t *testing.T) {
	t.Parallel()
	if got := ageWindowProblems(read(t, "Taskfile.yml")); len(got) != 0 {
		t.Errorf("the Taskfile holds a release-age window: %q", got)
	}
	for name, c := range map[string]struct{ text, says string }{
		"the member back":      {"  supply-age:\n    cmds: []\n", "supply-age"},
		"a call of the member": {"      - task: supply-age\n", "supply-age"},
		"the gate's command":   {"      - cmd: \"{{.GATE}} age --fetch\"\n", "age command"},
		"the times file":       {"    desc: reads .tools/pnpm-times.json\n", "publish time"},
	} {
		got := strings.Join(ageWindowProblems(c.text), "\n")
		if !strings.Contains(got, c.says) {
			t.Errorf("%s: problems %q, want one saying %q", name, got, c.says)
		}
	}
}

// ageWindowProblems is what in the text of a Taskfile holds a release-age window.
func ageWindowProblems(text string) []string {
	var out []string
	if regexp.MustCompile(`(?m)^  supply-age:`).MatchString(text) || strings.Contains(text, "- task: supply-age") {
		out = append(out, "names supply-age, the member that was removed")
	}
	if regexp.MustCompile(`\{\{\.GATE\}\} age\b`).MatchString(text) {
		out = append(out, "runs the gate's age command, which was removed")
	}
	if strings.Contains(text, "pnpm-times") {
		out = append(out, "reads a pnpm lock release's publish time, which no member holds after the pnpm change")
	}
	return out
}

// callsIn lists, in order, the tasks a section calls (`- task: name`), but for gate-build: the tier's first step builds
// the gate's program and is no member (gragas#2367), and TestEachTierBuildsTheGatesProgramOnceBeforeItsFirstGateCall
// holds where it stands.
func callsIn(section string) []string {
	var calls []string
	for _, m := range regexp.MustCompile(`(?m)^\s+- task: ([a-z][a-z0-9-]*)$`).FindAllStringSubmatch(section, -1) {
		if m[1] == "gate-build" {
			continue
		}
		calls = append(calls, m[1])
	}
	return calls
}

// depsAlone holds of a members task that it has deps and no cmds.
func depsAlone(section string) bool {
	return strings.Contains(section, "\n    deps:\n") && !strings.Contains(section, "\n    cmds:\n")
}

// commitShapeProblems holds of the text of pre-commit, pre-commit-members and
// commit-checks (gragas#1211) that pre-commit calls tools-check, then
// pre-commit-members, and ends on the budget; that pre-commit-members and
// commit-checks are deps alone; and that the commit's members are those deps:
// the two staged ones in pre-commit-members, the five the push shares in
// commit-checks (os-code and generated-files are the push's, gragas#2228).
func commitShapeProblems(commit, members, checks string) []string {
	var bad []string
	if got, want := callsIn(commit), []string{"tools-check"}; !slices.Equal(got, want) {
		bad = append(bad, fmt.Sprintf("pre-commit calls %v in its cmds, want %v: a member belongs in the deps of pre-commit-members", got, want))
	}
	if !budgetsMembers(commit, "pre-commit", "pre-commit-run") {
		bad = append(bad, "pre-commit does not run pre-commit-members under its budget")
	}
	if !depsAlone(members) {
		bad = append(bad, "pre-commit-members is not deps alone: its members run in order, not side by side")
	}
	if !depsAlone(checks) {
		bad = append(bad, "commit-checks is not deps alone: its members run in order, not side by side")
	}
	for _, name := range []string{"commit-checks", "secrets-staged"} {
		if !slices.Contains(callsIn(members), name) {
			bad = append(bad, name+" is not a dep of pre-commit-members")
		}
	}
	for _, name := range []string{"format", "explorer-format", "boards", "tidy"} {
		if !slices.Contains(callsIn(checks), name) {
			bad = append(bad, name+" is not a dep of commit-checks")
		}
	}
	return bad
}

// TestPreCommitRunsItsMembersAsDeps holds the Taskfile to that shape (gragas#1211),
// and a member moved back into pre-commit's cmds, or a members task with cmds, to be red.
func TestPreCommitRunsItsMembersAsDeps(t *testing.T) {
	t.Parallel()
	commit, members, checks := taskSection(t, "pre-commit"), taskSection(t, "pre-commit-members"), taskSection(t, "commit-checks")
	if bad := commitShapeProblems(commit, members, checks); len(bad) != 0 {
		t.Errorf("pre-commit (gragas#1211): %v", bad)
	}
	movedBack := strings.Replace(commit, "      - task: tools-check\n", "      - task: tools-check\n      - task: format\n", 1)
	if len(commitShapeProblems(movedBack, members, checks)) == 0 {
		t.Error("a member called in pre-commit's cmds is not red")
	}
	inOrder := strings.Replace(members, "    deps:\n", "    cmds:\n", 1)
	if len(commitShapeProblems(commit, inOrder, checks)) == 0 {
		t.Error("pre-commit-members with cmds in place of deps is not red")
	}
	checksInOrder := strings.Replace(checks, "    deps:\n", "    cmds:\n", 1)
	if len(commitShapeProblems(commit, members, checksInOrder)) == 0 {
		t.Error("commit-checks with cmds in place of deps is not red")
	}
	missing := strings.Replace(checks, "      - task: tidy\n", "", 1)
	if len(commitShapeProblems(commit, members, missing)) == 0 {
		t.Error("a commit member missing from the deps is not red")
	}
}

// releaseMembers are the members only a release proves, deps of release-members beside
// push-checks and full-checks (gragas#1214, gragas#1619).
func releaseMembers() []string {
	return []string{"test-uncached", "gate-tests", "test-heavy", "names", "secrets-full", "pty"}
}

// releaseShapeProblems holds of the text of release, release-members and push-checks with full-checks
// (gragas#1214) that release calls no task and ends on the budget, whose release-body calls tools-check,
// release-guard, warm and release-members, in that order (TestTheReleaseTierKeepsWhatATreeMustPassBeforeATag); that release-members is deps alone; that its deps are push-checks, full-checks and
// every release member, test-uncached among them; and that neither it nor push-checks holds
// the cached test, which test-uncached covers.
func releaseShapeProblems(release, members, checks string) []string {
	var bad []string
	if got := callsIn(release); len(got) != 0 {
		want := []string(nil)
		bad = append(bad, fmt.Sprintf("release calls %v in its cmds, want %v: a member belongs in the deps of release-members", got, want))
	}
	if !budgetsMembers(release, "release", "release-body") {
		bad = append(bad, "release does not run release-body under its budget")
	}
	if !depsAlone(members) {
		bad = append(bad, "release-members is not deps alone: its members run in order, not side by side")
	}
	deps := callsIn(members)
	for _, name := range append([]string{"push-checks", "full-checks"}, releaseMembers()...) {
		if !slices.Contains(deps, name) {
			bad = append(bad, name+" is not a dep of release-members")
		}
	}
	if slices.Contains(deps, "test") || slices.Contains(callsIn(checks), "test") || slices.Contains(callsIn(release), "test") || slices.Contains(callsIn(release), "pre-push") || slices.Contains(callsIn(release), "full") {
		bad = append(bad, "the release runs the cached test, or the whole push tier or full: test-uncached covers test's packages")
	}
	return bad
}

// TestReleaseRunsItsMembersAsDepsAndTheCachedTestIsNotAmongThem holds the Taskfile to that
// shape (gragas#1214); a member in release's cmds, the cached test planted in, and
// test-uncached taken out are red.
func TestReleaseRunsItsMembersAsDepsAndTheCachedTestIsNotAmongThem(t *testing.T) {
	t.Parallel()
	release, members, checks := taskSection(t, "release"), taskSection(t, "release-members"), taskSection(t, "push-checks")+taskSection(t, "full-checks")
	if bad := releaseShapeProblems(release, members, checks); len(bad) != 0 {
		t.Errorf("release (gragas#1214): %v", bad)
	}
	movedBack := strings.Replace(release, "    cmds:\n", "    cmds:\n      - task: crash\n", 1)
	if len(releaseShapeProblems(movedBack, members, checks)) == 0 {
		t.Error("a member called in release's cmds is not red")
	}
	pushWhole := strings.Replace(release, "    cmds:\n", "    cmds:\n      - task: pre-push\n", 1)
	if len(releaseShapeProblems(pushWhole, members, checks)) == 0 {
		t.Error("the whole push tier called in release is not red")
	}
	inOrder := strings.Replace(members, "    deps:\n", "    cmds:\n", 1)
	if len(releaseShapeProblems(release, inOrder, checks)) == 0 {
		t.Error("release-members with cmds in place of deps is not red")
	}
	cached := strings.Replace(members, "    deps:\n", "    deps:\n      - task: test\n", 1)
	if len(releaseShapeProblems(release, cached, checks)) == 0 {
		t.Error("the cached test among release's deps is not red")
	}
	inChecks := strings.Replace(checks, "    deps:\n", "    deps:\n      - task: test\n", 1)
	if len(releaseShapeProblems(release, members, inChecks)) == 0 {
		t.Error("the cached test in push-checks, which the release takes, is not red")
	}
	noUncached := strings.Replace(members, "      - task: test-uncached\n", "", 1)
	if len(releaseShapeProblems(release, noUncached, checks)) == 0 {
		t.Error("a release without test-uncached is not red")
	}
}

// runsIn are the runs a task's text calls, in order: each `- task:` line's name, with the
// line of vars after it when it has one, since a call with other vars is another run (the
// secret scan of the commits past main, of HEAD and of the tree are one task, secrets-scan).
// Every call of the Taskfile writes its vars on the one line, as {...}. A short-form deps list,
// `deps: [a, b]`, calls each name it holds, with no vars.
func runsIn(section string) []string {
	var runs []string
	for _, m := range regexp.MustCompile(`(?m)^\s+- task: ([a-z][a-z0-9-]*)\n(?:\s+(vars: \{.*\})\n)?`).FindAllStringSubmatch(section, -1) {
		runs = append(runs, strings.TrimSpace(m[1]+" "+m[2]))
	}
	for _, m := range regexp.MustCompile(`(?m)^\s+deps: \[([^\]]*)\]`).FindAllStringSubmatch(section, -1) {
		for name := range strings.SplitSeq(m[1], ",") {
			if name = strings.TrimSpace(name); name != "" {
				runs = append(runs, name)
			}
		}
	}
	return runs
}

// budgetAsCall reads a tier's budget command, `{{.BUDGET}} ... -- {{.TASK_EXE}} <members>`, as the call of
// <members> it makes, so a walk of the tasks a tier reaches goes through its budget (gragas#1779).
func budgetAsCall(text string) string {
	re := regexp.MustCompile(`(?m)^(\s+)- cmd: '\{\{\.BUDGET\}\}[^']*-- \{\{\.TASK_EXE\}\} ([a-z][a-z0-9-]*)[^']*'\n\s+timeout: \S+\n`)
	return re.ReplaceAllString(text, "${1}- task: $2\n")
}

// memberCounts walks the tasks name calls, through cmds and deps and theirs, and counts
// how often each run, a task with its vars, is reached. Each task is walked once, so a
// count over one is a run the tier makes twice: go-task runs a task again each time it is
// called, unless the task says run: once, which explorer-contract does, so its calls count as
// one run (gragas#1628). It also gives
// each task walked whose text spells more calls than runsIn reads, since a call the walk
// cannot read is one it cannot count (gragas#1351).
func memberCounts(section func(string) string, name string) (map[string]int, []string) {
	counts := map[string]int{}
	var unread []string
	// Every spelling of a call: a task: anywhere, with a comment after it or behind a #,
	// a short-form deps list, and a deps item that is a bare task name.
	spelledCalls := regexp.MustCompile(`task:`)
	shortDeps := regexp.MustCompile(`deps:\s*\[([^\]]*)\]`)
	walked := map[string]bool{name: true}
	var walk func(string)
	walk = func(name string) {
		text := budgetAsCall(section(name))
		runs := runsIn(text)
		spelled := len(spelledCalls.FindAllString(text, -1)) + bareDeps(text)
		// A short-form list spells one call for each name it holds, and is found unanchored, so a
		// list behind a comment spells calls that runsIn does not read.
		for _, m := range shortDeps.FindAllStringSubmatch(text, -1) {
			for name := range strings.SplitSeq(m[1], ",") {
				if strings.TrimSpace(name) != "" {
					spelled++
				}
			}
		}
		if spelled != len(runs) {
			unread = append(unread, fmt.Sprintf("%s spells %d calls and %d are read", name, spelled, len(runs)))
		}
		for _, run := range runs {
			counts[run]++
			if task, _, _ := strings.Cut(run, " "); !walked[task] {
				walked[task] = true
				walk(task)
			}
		}
	}
	walk(name)
	for run := range counts {
		if task, vars, _ := strings.Cut(run, " "); vars == "" && regexp.MustCompile(`(?m)^    run: once$`).MatchString(section(task)) {
			counts[run] = 1
		}
	}
	return counts, unread
}

// bareDeps counts the items of a block deps list given as a bare task name, `- test`,
// which go-task runs and runsIn does not read.
func bareDeps(text string) int {
	n, in, indent := 0, false, 0
	for line := range strings.SplitSeq(text, "\n") {
		item := strings.TrimLeft(line, " ")
		depth := len(line) - len(item)
		switch {
		case strings.TrimSpace(item) == "deps:":
			in, indent = true, depth
		case !in || item == "" || strings.HasPrefix(item, "#"):
		case depth <= indent && !strings.HasPrefix(item, "- "):
			in = false
		case strings.HasPrefix(item, "- ") && !strings.HasPrefix(item, "- task:"):
			n++
		}
	}
	return n
}

// onceProblems holds of the counts memberCounts gives for a tier that it makes every run
// once, and never the cached test, with any vars (gragas#1351): test-uncached is the
// release's one run of the unit tests. A call the walk could not read is red too, so a
// spelling runsIn does not know fails rather than passes unseen.
func onceProblems(counts map[string]int, unread []string) []string {
	bad := slices.Clone(unread)
	for _, run := range slices.Sorted(maps.Keys(counts)) {
		if counts[run] > 1 {
			bad = append(bad, fmt.Sprintf("%s runs %d times", run, counts[run]))
		}
		if task, _, _ := strings.Cut(run, " "); task == "test" {
			bad = append(bad, "the cached test runs: test-uncached is the release's one unit test run")
		}
	}
	return bad
}

// TestReleaseRunsEachMemberOnce holds the release tier, walked whole through its calls,
// to make each run once and never the cached test (gragas#1351). Its three secret scans,
// one task with three SCANs, are three runs; a member called again at another depth, a
// scan made twice with the same vars, and the cached test reached through a members task,
// with vars or none, are red.
func TestReleaseRunsEachMemberOnce(t *testing.T) {
	t.Parallel()
	section := func(name string) string { return taskSection(t, name) }
	// release-body checks the tools for its guard, and warm's door (warm-members) checks them again in its
	// own process, since a command line may start that door alone: that one second check is not a member's rerun.
	releaseCounts := func(section func(string) string) (map[string]int, []string) {
		counts, unread := memberCounts(section, "release")
		if counts["tools-check"] > 1 {
			counts["tools-check"]--
		}
		// The recipe's release calls the tier again, and each of the two builds the gate's program first: a second go build is
		// a cached no-op, not a member run twice (gragas#2367).
		if counts["gate-build"] > 1 {
			counts["gate-build"] = 1
		}
		return counts, unread
	}
	if bad := onceProblems(releaseCounts(section)); len(bad) != 0 {
		t.Errorf("release (gragas#1351): %v", bad)
	}
	for _, p := range []struct{ what, task, from, to string }{
		{"a push member again among the release's own", "release-members", "      - task: pty\n", "      - task: pty\n      - task: lint\n"},
		{"a commit member again among the push's", "push-checks", "      - task: secrets\n", "      - task: secrets\n      - task: tidy\n        vars: {STAGED_ONLY: \"{{.STAGED_ONLY}}\", NO_NETWORK: \"{{.NO_NETWORK}}\"}\n"},
		{"tools-check again among the members", "release-members", "      - task: pty\n", "      - task: pty\n      - task: tools-check\n"},
		{"the cached test reached through commit-checks", "commit-checks", "    deps:\n", "    deps:\n      - task: test\n"},
		{"the cached test with vars", "release-members", "      - task: pty\n", "      - task: pty\n      - task: test\n        vars: {SKIP_UNCHANGED: \"0\"}\n"},
		{"the scan of HEAD made twice", "secrets-full", "    cmds:\n", "    cmds:\n      - task: secrets-scan\n        vars: {SCAN: HEAD}\n"},
		{"the cached test as a short-form dep", "commit-checks", "    deps:\n", "    deps: [test]\n    deps:\n"},
		{"the cached test as a bare-name dep", "commit-checks", "    deps:\n", "    deps:\n      - test\n"},
		{"the cached test with a comment after it", "release-members", "      - task: pty\n", "      - task: pty\n      - task: test # the cached run\n"},
		{"a call behind a comment", "release-members", "      - task: pty\n", "      - task: pty\n      # - task: test\n"},
		{"the contract's build without run: once, called by three members", "explorer-contract", "    run: once\n", ""},
		{"the cached test with a comment after it, beside a two-name short-form deps list", "secrets-full", "    cmds:\n", "    deps: [explorer-contract, explorer-contract]\n    cmds:\n      - task: test # the cached run\n"},
	} {
		planted := func(name string) string {
			text := section(name)
			if name != p.task {
				return text
			}
			if !strings.Contains(text, p.from) {
				t.Fatalf("%s: %s has no %q", p.what, p.task, p.from)
			}
			return strings.Replace(text, p.from, p.to, 1)
		}
		if len(onceProblems(releaseCounts(planted))) == 0 {
			t.Errorf("%s is not red", p.what)
		}
	}
}

// releaseVarsProblems is what is wrong with the release's budget line, given the text of the
// release task: release-body runs as a child process, and a variable of the command line
// (SNAPSHOT for the guard, NAMES_NOT_ASKED for the names check) reaches it only when the line
// passes it on.
func releaseVarsProblems(release string) []string {
	var line string
	for l := range strings.SplitSeq(release, "\n") {
		if strings.Contains(l, "release-body --force") {
			line = l
		}
	}
	if line == "" {
		return []string{"release has no budget line that runs release-body --force"}
	}
	var out []string
	for _, v := range []string{"SNAPSHOT", "NAMES_NOT_ASKED"} {
		if !strings.Contains(line, " "+v+"={{.") {
			out = append(out, "the budget line does not pass "+v+" to release-body: it is lost in the child process")
		}
	}
	return out
}

// TestTheReleaseBudgetLinePassesItsVarsToTheChild holds the release's budget line to carrying
// SNAPSHOT and NAMES_NOT_ASKED into release-body, and a line without either is red.
func TestTheReleaseBudgetLinePassesItsVarsToTheChild(t *testing.T) {
	t.Parallel()
	release := taskSection(t, "release")
	if bad := releaseVarsProblems(release); len(bad) != 0 {
		t.Errorf("release: %v", bad)
	}
	for _, p := range []struct{ what, from, to string }{
		{"SNAPSHOT dropped", " SNAPSHOT={{.SNAPSHOT}}", ""},
		{"NAMES_NOT_ASKED dropped", " NAMES_NOT_ASKED={{.NAMES_NOT_ASKED | default \"\"}}", ""},
	} {
		if !strings.Contains(release, p.from) {
			t.Fatalf("%s: release has no %q", p.what, p.from)
		}
		if len(releaseVarsProblems(strings.Replace(release, p.from, p.to, 1))) == 0 {
			t.Errorf("%s is not red", p.what)
		}
	}
}

// TestVulnLicencesAndVetRunTheirSystemsSideBySide holds (gragas#1217) that vuln, licences
// and vet are deps alone, one for each system of the list through go-task's for with the
// matrix of CROSS, each calling the member's own -system task, which carries a timeout and
// names the member and the system when it is red; and, by a run of those same texts over
// a list of two systems with one planted red, that the member ends red and names that
// system and not the other.
func TestVulnLicencesAndVetRunTheirSystemsSideBySide(t *testing.T) {
	t.Parallel()
	task := pinnedTask(t)
	for _, name := range []string{"vuln", "licences", "vet"} {
		member, system := taskSection(t, name), taskSection(t, name+"-system")
		if !depsAlone(member) || !strings.Contains(member, "for: {matrix: {PLATFORM: {ref: .CROSS}}}") || !strings.Contains(member, "task: "+name+"-system\n") {
			t.Errorf("%s does not call %s-system once for each system of CROSS as deps (gragas#1217):\n%s", name, name, member)
		}
		if !strings.Contains(system, "internal: true") || !strings.Contains(system, "\n        timeout: ") || !strings.Contains(system, `echo "`+name+`: {{.PLATFORM}} is red" >&2; exit 1`) {
			t.Errorf("%s-system is not internal, with a timeout, naming the member and the system when red:\n%s", name, system)
		}
		// The same texts in a Taskfile of two systems, whose command is a check of the system's own: red on plan9.
		// (The member's whole script is replaced, so a member that reads a file or a list first is planted the same way.)
		cmd := regexp.MustCompile(`(?m)^(      - cmd: \|\n)(?:          .*\n?)+`).ReplaceAllString(system, "${1}          [ \"{{index (splitList \"/\" .PLATFORM) 0}}\" != plan9 ] || { echo \""+name+": {{.PLATFORM}} is red\" >&2; exit 1; }\n")
		if cmd == system {
			t.Fatalf("%s-system has no command to plant a red in", name)
		}
		dir := t.TempDir()
		file := "version: \"3\"\nvars:\n  CROSS: [linux/amd64, plan9/arm64]\n  GATE: \"true\"\ntasks:\n  " + name + ":\n" + strings.SplitN(member, "\n", 2)[1] + "\n  " + name + "-system:\n" + strings.SplitN(cmd, "\n", 2)[1]
		if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
		run := exec.CommandContext(t.Context(), "task")
		run.Path, run.Err = task, nil // the pinned one, set after the call: the constant above is what the rule reads
		run.Args = append(run.Args, name)
		run.Dir = dir
		run.Env = append(configtest.Inherited(), "PATH="+filepath.Dir(task)+string(os.PathListSeparator)+inheritedPath())
		out, err := run.CombinedOutput()
		if err == nil || !strings.Contains("\n"+string(out), "\n"+name+": plan9/arm64 is red\n") || strings.Contains("\n"+string(out), "\n"+name+": linux/amd64 is red\n") {
			t.Errorf("%s with plan9/arm64 planted red: err %v, want a failure naming plan9/arm64 alone:\n%s", name, err, out)
		}
	}
}

// timeoutProblems holds of the Taskfile's text (gragas#1135) that TEST_TIMEOUT is set once, to a
// duration go accepts, and that every line that runs go test or hatch tests passes
// -timeout {{.TEST_TIMEOUT}} and no other -timeout. HATCH_TIMEOUT, hatch tests' own --timeout for the
// whole run, is set once in whole minutes too, and above TEST_TIMEOUT: hatch's counts from the build and
// go test's from the test binary's start, so at one value hatch ends a hung run before go test prints its
// stacks (gragas#1471).
func timeoutProblems(taskfile string) []string {
	var bad []string
	minutes := map[string]int{}
	for _, name := range []string{"TEST_TIMEOUT", "HATCH_TIMEOUT"} {
		set := regexp.MustCompile(`(?m)^  `+name+`: ([0-9]+)m$`).FindAllStringSubmatch(taskfile, -1)
		if len(set) != 1 {
			bad = append(bad, fmt.Sprintf("%s is set %d times in whole minutes, want once", name, len(set)))
			continue
		}
		minutes[name], _ = strconv.Atoi(set[0][1])
	}
	if h, ok := minutes["HATCH_TIMEOUT"]; ok && h <= minutes["TEST_TIMEOUT"] {
		bad = append(bad, fmt.Sprintf("HATCH_TIMEOUT %dm is not above TEST_TIMEOUT %dm: hatch would end a hung run before go test prints its stacks", h, minutes["TEST_TIMEOUT"]))
	}
	for i, line := range strings.Split(taskfile, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "#") {
			continue
		}
		// hatch's own --timeout is the one other, held above.
		l = strings.ReplaceAll(l, "--timeout {{.HATCH_TIMEOUT}}", "")
		// The package list a run takes is no run (gragas#1470).
		runs := strings.Contains(l, "go test ") || (strings.Contains(l, "hatch{{exeExt}} tests ") && !strings.Contains(l, "hatch{{exeExt}} tests packages "))
		if runs && !strings.Contains(l, "-timeout {{.TEST_TIMEOUT}}") {
			bad = append(bad, fmt.Sprintf("line %d runs go test with no -timeout {{.TEST_TIMEOUT}}: %s", i+1, l))
		}
		if strings.Contains(l, "-timeout ") && !strings.Contains(l, "-timeout {{.TEST_TIMEOUT}}") {
			bad = append(bad, fmt.Sprintf("line %d passes a -timeout of its own: %s", i+1, l))
		}
	}
	return bad
}

// orderProblems holds of the Taskfile's text (gragas#1601) that every command that runs Go tests nests its
// timeouts, so the innermost fires first and a hung test ends with go test's stacks: go test's -timeout,
// TEST_TIMEOUT, under hatch tests' --timeout, HATCH_TIMEOUT, where the command runs hatch tests, and both
// under the command's own go-task timeout, which an outer one would end with the tree killed and no stacks.
// A command is the lines from one that runs go test or hatch tests to its timeout: key; one with none is red.
func orderProblems(taskfile string) []string {
	var bad []string
	limit := func(name string) time.Duration {
		d, _ := time.ParseDuration(varLine(taskfile, name))
		return d
	}
	goTest, hatchTest := limit("TEST_TIMEOUT"), limit("HATCH_TIMEOUT")
	if goTest <= 0 || hatchTest <= goTest {
		return append(bad, fmt.Sprintf("TEST_TIMEOUT %v is not under HATCH_TIMEOUT %v", goTest, hatchTest))
	}
	timeoutKey := regexp.MustCompile(`^ +timeout: (\S+)$`)
	task, open, hatch, first := "", false, false, 0
	for i, line := range strings.Split(taskfile, "\n") {
		if m := regexp.MustCompile(`^  ([a-z][a-z0-9-]*):$`).FindStringSubmatch(line); m != nil {
			if open {
				bad = append(bad, fmt.Sprintf("%s: the go test of line %d has no go-task timeout", task, first))
			}
			task, open = m[1], false
			continue
		}
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "#") {
			continue
		}
		if strings.Contains(l, "go test ") || strings.Contains(l, "hatch{{exeExt}} tests ") {
			if !open {
				first, hatch = i+1, false
			}
			open, hatch = true, hatch || strings.Contains(l, "hatch{{exeExt}} tests ")
			continue
		}
		m := timeoutKey.FindStringSubmatch(line)
		if !open || m == nil {
			continue
		}
		open = false
		inner, name := goTest, "TEST_TIMEOUT"
		if hatch {
			inner, name = hatchTest, "HATCH_TIMEOUT"
		}
		if d, err := time.ParseDuration(m[1]); err != nil || d <= inner {
			bad = append(bad, fmt.Sprintf("%s: the go-task timeout %s of line %d's run is not above %s %v: go-task would kill the tree before go test prints its stacks", task, m[1], first, name, inner))
		}
	}
	if open {
		bad = append(bad, fmt.Sprintf("%s: the go test of line %d has no go-task timeout", task, first))
	}
	return bad
}

// TestEveryGoTestMemberNestsItsTimeouts holds the Taskfile to orderProblems (gragas#1601). The plants are red:
// a hatch member under HATCH_TIMEOUT, one at it, a go test member under TEST_TIMEOUT, a member with no timeout,
// and a HATCH_TIMEOUT raised past the members'.
func TestEveryGoTestMemberNestsItsTimeouts(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	if bad := orderProblems(text); len(bad) != 0 {
		t.Errorf("the Taskfile (gragas#1601): %v", bad)
	}
	member := func(name, timeout string) string {
		section := taskSection(t, name)
		planted := regexp.MustCompile(`(?m)^( +timeout: )\S+$`).ReplaceAllString(section, "${1}"+timeout)
		return strings.Replace(text, section, planted, 1)
	}
	plants := []struct{ name, text string }{
		{"journeys-smoke back at 10m", member("journeys-smoke", "10m")},
		{"gate-tests at HATCH_TIMEOUT", member("gate-tests", taskVar(t, "HATCH_TIMEOUT"))},
		{"pty at TEST_TIMEOUT", member("pty", taskVar(t, "TEST_TIMEOUT"))},
		{"subset-build with no timeout", strings.Replace(text, taskSection(t, "subset-build"), regexp.MustCompile(`(?m)^ +timeout: \S+\n`).ReplaceAllString(taskSection(t, "subset-build"), ""), 1)},
		{"HATCH_TIMEOUT past the members", regexp.MustCompile(`(?m)^  HATCH_TIMEOUT: \S+$`).ReplaceAllString(text, "  HATCH_TIMEOUT: 16m")},
	}
	for _, p := range plants {
		if p.text == text {
			t.Errorf("%s: the plant changed nothing; the Taskfile's text moved under it", p.name)
			continue
		}
		if len(orderProblems(p.text)) == 0 {
			t.Errorf("%s: orderProblems is not red", p.name)
		}
	}
}

// TestEveryGoTestOfTheGateCarriesTheOneTimeout holds the Taskfile to that, and a planted
// go test line without the flag, one with a value of its own, and a second setting are red.
func TestEveryGoTestOfTheGateCarriesTheOneTimeout(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	if bad := timeoutProblems(text); len(bad) != 0 {
		t.Errorf("the Taskfile (gragas#1135): %v", bad)
	}
	if len(timeoutProblems(text+"\n      - go test ./internal\n")) == 0 {
		t.Error("a go test line with no -timeout is not red")
	}
	if len(timeoutProblems(text+"\n      - go test -timeout 5m ./internal\n")) == 0 {
		t.Error("a go test line with a -timeout of its own is not red")
	}
	if len(timeoutProblems(strings.Replace(text, "  TEST_TIMEOUT: ", "  TEST_TIMEOUT_X: ", 1))) == 0 {
		t.Error("a Taskfile with no TEST_TIMEOUT is not red")
	}
	if len(timeoutProblems(text+"\n  TEST_TIMEOUT: 5m\n")) == 0 {
		t.Error("a second TEST_TIMEOUT is not red")
	}
	hatch := regexp.MustCompile(`(?m)^  HATCH_TIMEOUT: [0-9]+m$`)
	if len(timeoutProblems(hatch.ReplaceAllString(text, "  HATCH_TIMEOUT: "+taskVar(t, "TEST_TIMEOUT")))) == 0 {
		t.Error("a HATCH_TIMEOUT equal to TEST_TIMEOUT is not red")
	}
	if len(timeoutProblems(hatch.ReplaceAllString(text, "  HATCH_TIMEOUT: 1m"))) == 0 {
		t.Error("a HATCH_TIMEOUT under TEST_TIMEOUT is not red")
	}
	if len(timeoutProblems(hatch.ReplaceAllString(text, ""))) == 0 {
		t.Error("a Taskfile with no HATCH_TIMEOUT is not red")
	}
	for _, name := range []string{"test", "test-uncached", "test-heavy", "gate-tests"} {
		if !strings.Contains(taskSection(t, name), "-timeout {{.TEST_TIMEOUT}}") {
			t.Errorf("%s has no -timeout {{.TEST_TIMEOUT}}", name)
		}
	}
}

// TestTheTiersTakeTheirStartWhenTheyAreCalledWithNoProgram holds gragas#1228: START is go-task's
// own clock (no date program, which Windows lacks), set in the tier task's vars, so it is worked
// out when the tier is called and never when the file loads, and the budget's --since is the
// value it took.
func TestTheTiersTakeTheirStartWhenTheyAreCalledWithNoProgram(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	head := text[:strings.Index(text, "\ntasks:")]
	if strings.Contains(head, "START") || strings.Contains(text, "date +%s") {
		t.Error("START is set where the file loads, or by the date program")
	}
	for _, tier := range []string{"pre-commit", "pre-push", "warm", "full", "release"} {
		section := taskSection(t, tier)
		v, c := strings.Index(section, "START: '{{now | unixEpoch}}'"), strings.Index(section, "cmds:")
		if v < 0 || c < 0 || v > c {
			t.Errorf("%s does not take START from the clock in its own vars, before its commands", tier)
		}
	}
}

// TestHatchListsTheLightPackagesTheHeavyOnesLeaveAndIsRedOnNone holds gragas#1470: the light list is
// hatch tests packages over the module without HEAVY, every HEAVY path is a package of the module
// (an exclude of a path that is none would drop nothing and say nothing), and a list with nothing
// left exits 1.
func TestHatchListsTheLightPackagesTheHeavyOnesLeaveAndIsRedOnNone(t *testing.T) {
	t.Parallel()
	hatch := pinnedPath(t, "hatch")
	if _, err := os.Stat(hatch); err != nil {
		t.Fatal("the pinned hatch is not installed: run task tools")
	}
	list := func(exclude ...string) (string, int) {
		cmd := exec.CommandContext(t.Context(), hatch)
		cmd.Args = append(cmd.Args, "tests", "packages")
		cmd.Args = append(cmd.Args, exclude...)
		cmd.Dir = filepath.Join("..")
		cmd.Env = configtest.Inherited()
		out, err := cmd.Output()
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return string(out), exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(out), 0
	}
	heavy := strings.Fields(varLine(read(t, "Taskfile.yml"), "HEAVY"))
	all, code := list()
	if code != 0 {
		t.Fatalf("hatch tests packages: exit %d", code)
	}
	light, code := list(heavy...)
	if code != 0 {
		t.Fatalf("hatch tests packages over HEAVY: exit %d", code)
	}
	have := strings.Fields(all)
	for _, p := range heavy {
		if !slices.Contains(have, p) {
			t.Errorf("HEAVY names %s, which is no package of the module: the exclude would drop nothing", p)
		}
		if strings.Contains("\n"+light, "\n"+p+"\n") {
			t.Errorf("the light list holds the heavy package %s", p)
		}
	}
	if got, want := len(strings.Fields(light)), len(have)-len(heavy); got != want {
		t.Errorf("the light list has %d packages, want %d: the module's %d less the %d heavy", got, want, len(have), len(heavy))
	}
	if out, code := list("github.com/birdtable/gragas/..."); code != 1 || out != "" {
		t.Errorf("every package excluded: exit %d, %q; want exit 1 and no list", code, out)
	}
}

// TestThePushesTestRunsEveryPackageHeavyOnesToo holds gragas#1246: the push tier's test member lists
// no package out, so a break in a heavy package reaches the push and not only the release; the
// heavy list adds -count=1 and shuffle at release alone (test-heavy). gragas#1194 runs the
// slowest groups of the repository's checks (NO_RACE_PUSH, held to gate/race-push.txt by
// TestNoRacePushIsTheGroupsOverTheLine) without -race at push: the race run is every package
// but those, and a second run is exactly those, so the two together are ./... and nothing
// else is left out; test-uncached keeps -race on every package.
func TestThePushesTestRunsEveryPackageHeavyOnesToo(t *testing.T) {
	t.Parallel()
	section := taskSection(t, "test")
	if bad := pushTestProblems(section); len(bad) != 0 {
		t.Errorf("test (gragas#1246, gragas#1194): %v", bad)
	}
	const noRace = "github.com/birdtable/gragas/internal github.com/birdtable/gragas/internal/repo/rulebook github.com/birdtable/gragas/internal/repo/gatetest"
	if got := varLine(read(t, "Taskfile.yml"), "NO_RACE_PUSH"); got != noRace {
		t.Errorf("NO_RACE_PUSH is %q, not the groups over #1194's line, %s", got, noRace)
	}
	for name, plant := range map[string][2]string{
		"the heavy list left out of the race run":   {"tests packages {{.NO_RACE_PUSH}}", "tests packages {{.NO_RACE_PUSH}} {{.HEAVY}}"},
		"the no-race run of another list":           {"          {{.NO_RACE_PUSH}}\n", "          ./internal\n"},
		"-race dropped from the race run":           {"-- -race -timeout", "-- -timeout"},
		"-race added to the no-race run":            {"-- -timeout", "-- -race -timeout"},
		"the no-race run gone":                      {"          {{.NO_RACE_PUSH}}\n", ""},
		"a package left out with a list of its own": {"tests packages {{.NO_RACE_PUSH}}", "tests packages ./internal/journeys"},
	} {
		planted := strings.Replace(section, plant[0], plant[1], 1)
		if planted == section {
			t.Fatalf("%s: the plant did not change the member", name)
		}
		if len(pushTestProblems(planted)) == 0 {
			t.Errorf("%s is not red", name)
		}
	}
	uncached := taskSection(t, "test-uncached")
	if !strings.Contains(uncached, "-race") || strings.Contains(uncached, "NO_RACE_PUSH") {
		t.Error("test-uncached does not keep -race on every package")
	}
	if !strings.Contains(taskSection(t, "test-heavy"), "-count=1") || !strings.Contains(taskSection(t, "test-heavy"), "-shuffle=on") {
		t.Error("test-heavy does not add -count=1 and shuffle")
	}
}

// pushTestProblems holds of the text of the push's test member that it runs, with -race,
// every package but NO_RACE_PUSH, then, without -race, NO_RACE_PUSH itself, each through
// hatch tests of its own, the first stopping the member on its red, and names neither the
// heavy list nor any other.
func pushTestProblems(section string) []string {
	var bad []string
	runs := strings.Split(section, "hatch{{exeExt}} tests {{.HATCH_TESTS_FLAGS}}")
	if len(runs) != 3 {
		return append(bad, fmt.Sprintf("test has %d hatch tests runs, want 2: the race run and the no-race run", len(runs)-1))
	}
	race, plain := runs[1], runs[2]
	if !strings.Contains(race, "-- -race ") || !strings.Contains(race, "          $({{.TOOLS}}/hatch{{exeExt}} tests packages {{.NO_RACE_PUSH}}) || exit 1\n") {
		bad = append(bad, "the first run is not -race over every package but NO_RACE_PUSH")
	}
	if regexp.MustCompile(`\s-race\b`).MatchString(plain) || !strings.Contains(plain, "          {{.NO_RACE_PUSH}}\n") {
		bad = append(bad, "the second run is not NO_RACE_PUSH without -race")
	}
	// The member is one shell script: without the exit, a red race run followed by a green
	// no-race run would leave the member green.
	if !strings.Contains(race, "|| exit 1\n") {
		bad = append(bad, "the race run does not stop the member on its red")
	}
	if strings.Contains(section, ".HEAVY") || strings.Count(section, "tests packages") != 1 || strings.Contains(section, "./...") {
		bad = append(bad, "test names a list of its own: a package it leaves out is never run by a push")
	}
	return bad
}

// TestNoRacePushIsTheGroupsOverTheLine (gragas#1194): the push runs a group of the repository's checks without -race
// only when its measured -race time, in gate/race-push.txt, is over the line
// #1194 set, and runs every other group under -race. Every group (./internal
// and each folder of internal/repo) has a row there, so a new group is
// measured before it is placed. Risk: a group moved out of the race run that
// was never slow under it, or a slow one added and never measured.
func TestNoRacePushIsTheGroupsOverTheLine(t *testing.T) {
	t.Parallel()
	groups := []string{"./internal"}
	entries, err := os.ReadDir(filepath.Join(root, "internal", "repo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			groups = append(groups, "./internal/repo/"+e.Name())
		}
	}
	times := read(t, "gate/race-push.txt")
	// race-push.txt names a group as ./path; the Taskfile spells it by import path, which hatch tests packages drops.
	list := strings.Fields(strings.ReplaceAll(varLine(read(t, "Taskfile.yml"), "NO_RACE_PUSH"), "github.com/birdtable/gragas", "."))
	for _, p := range noRacePushProblems(times, groups, list) {
		t.Error(p)
	}
	plants := map[string][2]string{
		"a group under the line in the list":     {"./internal/repo/docs 6.315", "./internal/repo/docs 16.315"},
		"a group in the list now under the line": {"./internal/repo/rulebook 17.180", "./internal/repo/rulebook 7.180"},
		"a group with no row":                    {"./internal/repo/gocode 5.178\n", ""},
		"the line moved":                         {"line 15\n", "line 50\n"},
		"a row that is no number":                {"./internal 44.618", "./internal slow"},
	}
	for name, p := range plants {
		planted := strings.Replace(times, p[0], p[1], 1)
		if planted == times {
			t.Fatalf("%s: the plant changed nothing", name)
		}
		if len(noRacePushProblems(planted, groups, list)) == 0 {
			t.Errorf("%s: no problem found", name)
		}
	}
}

// noRacePushProblems holds NO_RACE_PUSH's list to the measured times: the line
// is #1194's 15 seconds, each group has one row with a number, no row names
// another, and the list is exactly the groups whose time is over the line.
func noRacePushProblems(times string, groups, list []string) []string {
	var problems []string
	line := -1.0
	measured := map[string]float64{}
	for raw := range strings.SplitSeq(times, "\n") {
		f := strings.Fields(raw)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) != 2 {
			problems = append(problems, "gate/race-push.txt: a row is not a name and a number: "+raw)
			continue
		}
		v, err := strconv.ParseFloat(f[1], 64)
		if err != nil {
			problems = append(problems, "gate/race-push.txt: "+f[0]+" is not a number of seconds: "+f[1])
			continue
		}
		if f[0] == "line" {
			line = v
			continue
		}
		if _, ok := measured[f[0]]; ok {
			problems = append(problems, "gate/race-push.txt: "+f[0]+" has two rows")
		}
		measured[f[0]] = v
	}
	if line != 15 {
		problems = append(problems, fmt.Sprintf("gate/race-push.txt: the line is %v, not #1194's 15 seconds", line))
	}
	var want []string
	for _, g := range groups {
		v, ok := measured[g]
		switch {
		case !ok:
			problems = append(problems, "gate/race-push.txt: "+g+" has no measured -race time: measure it before it is placed")
		case v > line:
			want = append(want, g)
		}
	}
	for g := range measured {
		if !slices.Contains(groups, g) {
			problems = append(problems, "gate/race-push.txt: "+g+" is no group of the repository's checks")
		}
	}
	got := slices.Sorted(slices.Values(list))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		problems = append(problems, fmt.Sprintf("NO_RACE_PUSH is %v, and the groups over the line are %v", got, want))
	}
	return problems
}

// TestGeneratedFilesRunsItsSixChecksSideBySideAndARedOneStillTurnsItRed (gragas#1348): the commit's
// generated-files member is deps alone, one task whose one step has a timeout for each check, so the six run
// side by side. Each is internal. generated-presets is not one of them for now: it needs the plugins that write
// the bound presets installed in the tree, and it returns to the member, and this test to seven, when they are.
// Planted: the same texts in a Taskfile whose third check fails turn the member red, and
// the checks after it still ran (a red dep stops none of the others).
func TestGeneratedFilesRunsItsSixChecksSideBySideAndARedOneStillTurnsItRed(t *testing.T) {
	t.Parallel()
	task := pinnedTask(t)
	member := taskSection(t, "generated-files")
	calls := regexp.MustCompile(`(?m)^      - task: (generated-[a-z]+)$`).FindAllStringSubmatch(member, -1)
	if !depsAlone(member) || len(calls) != 6 {
		t.Fatalf("generated-files is not six deps alone (gragas#1348):\n%s", member)
	}
	if slices.ContainsFunc(calls, func(c []string) bool { return c[1] == "generated-presets" }) {
		t.Errorf("generated-files runs generated-presets, which is out of the member until its plugins are installed:\n%s", member)
	}
	file := "version: \"3\"\ntasks:\n  generated-files:\n" + strings.SplitN(member, "\n", 2)[1]
	for i, c := range calls {
		section := taskSection(t, c[1])
		if !strings.Contains(section, "\n        timeout: ") {
			t.Errorf("%s has no timeout:\n%s", c[1], section)
		}
		if !strings.Contains(section, "internal: true") {
			t.Errorf("%s is not internal:\n%s", c[1], section)
		}
		cmd := "echo ran " + c[1]
		if i == 2 {
			cmd += "; exit 1"
		}
		file += "\n  " + c[1] + ":\n    cmds:\n      - " + cmd + "\n"
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	run := exec.CommandContext(t.Context(), "task")
	run.Path, run.Err = task, nil
	run.Args = append(run.Args, "generated-files")
	run.Dir = dir
	run.Env = append(configtest.Inherited(), "PATH="+filepath.Dir(task)+string(os.PathListSeparator)+inheritedPath())
	out, err := run.CombinedOutput()
	if err == nil {
		t.Errorf("a red check did not turn generated-files red:\n%s", out)
	}
	for _, c := range calls {
		if !strings.Contains(string(out), "ran "+c[1]) {
			t.Errorf("%s did not run beside the red one:\n%s", c[1], out)
		}
	}
}

// TestTheBuildMemberLeavesAnUnchangedBinaryAlone (gragas#1381): the build
// member builds into the stable temp folder and places the result, and never
// passes bin/gragas to go build as its output, which would write the file
// again on every push and move the modification time that Go's test cache
// reads for the tests that stat it.
func TestTheBuildMemberLeavesAnUnchangedBinaryAlone(t *testing.T) {
	t.Parallel()
	body := taskSection(t, "build")
	if strings.Contains(body, "-o bin/gragas") {
		t.Error("the build member writes bin/gragas with go build: an unchanged binary gets a new mtime on every push")
	}
	if !strings.Contains(body, "{{.GATE}} place \"$out\" bin/gragas") || !strings.Contains(body, "-o \"$out\"") {
		t.Error("the build member does not build to a file of its own and place it with gragas-dev place")
	}
}

// TestLintRunsOnceOverEveryFile holds gragas#1710 (M3): the lint is one run of golangci-lint, with no
// system set and no matrix over systems, and the run is safe because TestNoFileIsLeftOut holds that
// go list leaves no file out of it under any system of CROSS. Planted failure: a matrix back on lint or
// on lint-diff, or a GOOS set on either run.
func TestLintRunsOnceOverEveryFile(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"lint", "lint-diff", "lint-diff-system"} {
		section := taskSection(t, name)
		if strings.Contains(section, "matrix") || strings.Contains(section, "GOOS") {
			t.Errorf("%s runs over systems: the lint is one run (gragas#1710)", name)
		}
	}
	for _, name := range []string{"lint", "lint-diff-system"} {
		if n := strings.Count(taskSection(t, name), "golangci-lint{{exeExt}} run"); n != 1 {
			t.Errorf("%s runs golangci-lint %d times, want once", name, n)
		}
	}
}

// TestAnUnverifiedHatchNeverRunsBeforeToolsCheckRefusesIt holds gragas#1468: a tier samples the load with
// hatch only after tools-check has held it to its pin. A hatch of the test's own, built outside the lock,
// writes a marker file when it runs; each tier is called in a scratch copy of the repository's Taskfile with
// every tool stale, and ends red at tools-check with no marker, so no program of .tools/bin ran before it.
func TestAnUnverifiedHatchNeverRunsBeforeToolsCheckRefusesIt(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	gate := gateBinary(t, abs)
	pinned := pinnedTask(t)
	exe := goEXE(t)
	pinnedBytes, err := os.ReadFile(pinned)
	if err != nil {
		t.Fatal(err)
	}
	build := func(name, main string) []byte {
		t.Helper()
		src := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		for file, text := range map[string]string{"go.mod": "module example.com/" + name + "\n\ngo 1.24\n", "main.go": main} {
			if err := os.WriteFile(filepath.Join(src, file), []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		out := filepath.Join(src, name+exe)
		cmd := exec.CommandContext(t.Context(), "go")
		cmd.Args = append(cmd.Args, "build", "-o", out, ".")
		cmd.Dir = src
		cmd.Env = append(configtest.Inherited(), "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, b)
		}
		bytes, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return bytes
	}
	stale := build("stale", "package main\n\nfunc main() {}\n")
	stub := build("stub", "package main\n\nimport \"os\"\n\nfunc main() { _ = os.WriteFile(os.Getenv(\"HATCH_RAN\"), []byte(\"ran\"), 0o600) }\n")
	// The tiers, and every public door the Taskfile gives hatch to start: a command line can
	// start each, so each ends red at tools-check with the stub unrun, and the release's door is given the --force it needs.
	for _, tier := range []string{"pre-commit", "pre-push", "pre-commit-run", "pre-push-run", "full-run", "warm-members", "release-body --force"} {
		dir := t.TempDir()
		// The folder holds no go.mod to build the gate's program from: the tier's first step, gate-build (gragas#2367), is left
		// out, and the claim is that no tier runs the unverified hatch before tools-check refuses it.
		noBuild := strings.ReplaceAll(read(t, "Taskfile.yml"), "      - task: gate-build\n        timeout: 5m\n", "")
		noBuild = strings.ReplaceAll(noBuild, "      - task: gate-build\n        vars: {NO_NETWORK: \"1\"}\n        timeout: 5m\n", "")
		for name, text := range map[string]string{"Taskfile.yml": noBuild, "tools/go.sum": read(t, "tools/go.sum"),
			"internal/journeys/smoke.txt": read(t, "internal/journeys/smoke.txt"), "internal/journeys/goldens.txt": read(t, "internal/journeys/goldens.txt")} {
			p := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		bin := filepath.Join(dir, ".tools", "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		filestest.WriteProgram(t, filepath.Join(bin, "task"+exe), pinnedBytes)
		for _, n := range []string{"gitleaks", "govulncheck", "go-licenses", "dupl"} {
			filestest.WriteProgram(t, filepath.Join(bin, n+exe), stale)
		}
		filestest.WriteProgram(t, filepath.Join(bin, "hatch"+exe), stub)
		marker := filepath.Join(dir, "hatch-ran")
		run := func() (string, error) {
			cmd := exec.CommandContext(t.Context(), pinned)
			cmd.Args = append(cmd.Args, strings.Fields(tier)...)
			cmd.Args = append(cmd.Args, "GATE="+gate, "RELEASE_TOOLS=", "PNPM_TREES=")
			cmd.Dir = dir
			cmd.Env = append(configtest.Inherited(), toolchainPin(t), "HATCH_RAN="+marker, "PATH="+filepath.Dir(pinned)+string(os.PathListSeparator)+inheritedPath())
			out, err := cmd.CombinedOutput()
			return string(out), err
		}
		out, err := run()
		if err == nil || !strings.Contains(string(out), "tools-check: ") {
			t.Errorf("%s with every tool stale: err %v, output %q, want red at tools-check", tier, err, out)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Errorf("%s ran the unverified hatch before tools-check refused it", tier)
		}
	}
}

// contractProblems is what is wrong with how the explorer's members place the contract's dist,
// given the text of each task's section by name. The typecheck, the lint and the tests run side
// by side in full, each through a package script that begins with the contract's build; were the
// build theirs alone, one that finds the dist stale would write it while another member's
// write-nothing wrapper watches, and that member would go red on writes no command of its own made
// (gragas#1517). So each of the three depends on explorer-contract, which runs once, and which
// places the dist through ensure-contract.ts, only when it is stale (gragas#1628).
func contractProblems(sections map[string]string) []string {
	var out []string
	for _, name := range []string{"explorer-typecheck", "explorer-lint", "explorer-test"} {
		if !strings.Contains(sections[name], "deps: [explorer-contract]") {
			out = append(out, name+" does not depend on explorer-contract: its package script's contract build could write the dist under another member's write-nothing wrapper")
		}
	}
	contract := sections["explorer-contract"]
	if !strings.Contains(contract, "\n    run: once\n") {
		out = append(out, "explorer-contract is not run: once: each member would place the dist itself, side by side")
	}
	if !strings.Contains(contract, "scripts/ensure-contract.ts") {
		out = append(out, "explorer-contract does not run the build through scripts/ensure-contract.ts: it would rebuild a dist that is current")
	}
	return out
}

func TestTheExplorersMembersPlaceTheContractOnceBeforeTheyRunSideBySide(t *testing.T) {
	t.Parallel()
	names := []string{"explorer-typecheck", "explorer-lint", "explorer-test", "explorer-contract"}
	sections := map[string]string{}
	for _, name := range names {
		sections[name] = taskSection(t, name)
	}
	for _, p := range contractProblems(sections) {
		t.Error(p)
	}
	plant := func(name, from, to string) map[string]string {
		planted := maps.Clone(sections)
		if !strings.Contains(planted[name], from) {
			t.Fatalf("%s holds no %q to plant over", name, from)
		}
		planted[name] = strings.Replace(planted[name], from, to, 1)
		return planted
	}
	for _, c := range []struct {
		name, from, to, want string
	}{
		{"explorer-typecheck", "deps: [explorer-contract]", "", "explorer-typecheck does not depend"},
		{"explorer-lint", "deps: [explorer-contract]", "", "explorer-lint does not depend"},
		{"explorer-test", "deps: [explorer-contract]", "", "explorer-test does not depend"},
		{"explorer-contract", "\n    run: once\n", "\n", "not run: once"},
		{"explorer-contract", "scripts/ensure-contract.ts", "scripts/other.ts", "ensure-contract.ts"},
	} {
		got := strings.Join(contractProblems(plant(c.name, c.from, c.to)), "\n")
		if !strings.Contains(got, c.want) {
			t.Errorf("a plant of %s without %q: problems %q, want one holding %q", c.name, c.from, got, c.want)
		}
	}
}

// bareToolPaths lists the places in the Taskfile's text that name a program of .tools/bin by its path with no
// {{exeExt}} right after its name (gragas#1566). go-task's shell adds no .exe to an explicit path, so on Windows such a
// call is not found (127): the release tier stopped at release-guard on {{.TOOLS}}/changie. The suffix keeps the
// explicit path, so a program of the same name elsewhere on PATH cannot stand in (one suffix, no branch per
// system). A shell variable after the folder ({{.TOOLS}}/$name) is red too, since no check can read its suffix: the
// tools loop that named one went into gragas-dev tools-go, which names each program for its system (gragas#1655, X107).
func bareToolPaths(text string) []string {
	var bad []string
	path := regexp.MustCompile(`\{\{\.TOOLS\}\}/[A-Za-z][A-Za-z0-9_-]*(?:\.[A-Za-z0-9_-]+)*`)
	for i, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "{{.TOOLS}}/$") {
			bad = append(bad, fmt.Sprintf("line %d names a program of the tools folder by a shell variable, whose suffix no check reads: %s", i+1, strings.TrimSpace(l)))
		}
		for _, m := range path.FindAllStringIndex(l, -1) {
			if !strings.HasPrefix(l[m[1]:], "{{exeExt}}") {
				bad = append(bad, fmt.Sprintf("line %d names %s with no {{exeExt}}: %s", i+1, l[m[0]:m[1]], strings.TrimSpace(l)))
			}
		}
	}
	return bad
}

// ownSuffix is the executable suffix a plant spells by hand, which the check
// must refuse where exeExt should have written it.
const ownSuffix = ".ex" + "e"

// TestEveryToolPathOfTheTaskfileCarriesTheExeSuffix holds Taskfile.yml to that, and a planted call with no suffix, in
// each shape the Taskfile spells one, is red.
func TestEveryToolPathOfTheTaskfileCarriesTheExeSuffix(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	for _, b := range bareToolPaths(text) {
		t.Error(b)
	}
	for name, plant := range map[string][2]string{
		"bare":                 {"{{.TOOLS}}/changie{{exeExt}} latest", "{{.TOOLS}}/changie latest"},
		"in double quotes":     {`"{{.TOOLS}}/pnpm{{exeExt}}"`, `"{{.TOOLS}}/pnpm"`},
		"in single quotes":     {"'{{.TOOLS}}/goreleaser{{exeExt}}'", "'{{.TOOLS}}/goreleaser'"},
		"in a $()":             {"$({{.TOOLS}}/changie{{exeExt}} latest)", "$({{.TOOLS}}/changie latest)"},
		"by a shell variable":  {"", "      - cmd: '{{.TOOLS}}/$name{{exeExt}} --version'\n"},
		"a suffix of its own":  {"{{.TOOLS}}/gitleaks{{exeExt}}", "{{.TOOLS}}/gitleaks" + ownSuffix},
		"the suffix spaced":    {"{{.TOOLS}}/govulncheck{{exeExt}}", "{{.TOOLS}}/govulncheck {{exeExt}}"},
		"a new tool, appended": {"", "      - cmd: '{{.TOOLS}}/newtool run'\n"},
	} {
		planted := text + plant[1]
		if plant[0] != "" {
			planted = strings.Replace(text, plant[0], plant[1], 1)
		}
		if planted == text {
			t.Errorf("%s: the plant changed nothing; the Taskfile's text moved under it", name)
			continue
		}
		if len(bareToolPaths(planted)) == 0 {
			t.Errorf("%s: planted, and the check found nothing", name)
		}
	}
}

// namesTaskProblems holds the names task's text (gragas#1566) to one command: with NAMES_NOT_ASKED set, a line that
// says the list was not asked, and nothing else; otherwise the bare command of cmd/gragas-names, which reads the list
// from the settings file and never from a variable. Only the operator's Mac holds the list, so a release on a lent
// Linux or Windows session is not asked, and must say so rather than fail or pass without a word. It reads the
// rulebook folder: the root also holds the testdata books, and more than one rulebook.yaml is not ready (gragas#1741).
func namesTaskProblems(section string) []string {
	const want = "      - cmd: go run ./cmd/gragas-names -C rulebook{{if .NAMES_NOT_ASKED}} -not-asked{{end}}\n"
	var bad []string
	if !strings.Contains(section, want) || len(taskCmds(section)) != 1 {
		bad = append(bad, "it is not the one command that runs cmd/gragas-names on the rulebook folder, with -not-asked under NAMES_NOT_ASKED (which gragas-names refuses where the settings name a list)")
	}
	if strings.Contains(section, "NAMES_FILE") {
		bad = append(bad, "it names the list's variable, which gragas-names refuses")
	}
	return bad
}

// windowsPathProblems are the two shapes that broke the Taskfile on Windows (gragas#1566, gragas#1571): a mktemp that
// is not given its folder, which go-task's own mktemp takes as /tmp when TMPDIR is unset, and a name cut from $PWD,
// whose separators are backslashes there.
func windowsPathProblems(text string) []string {
	var bad []string
	mktemp := regexp.MustCompile(`\bmktemp\b`)
	for i, l := range strings.Split(text, "\n") {
		code, _, _ := strings.Cut(strings.TrimSpace(l), " #")
		if strings.HasPrefix(code, "#") {
			continue
		}
		for _, m := range mktemp.FindAllStringIndex(code, -1) {
			if !strings.HasPrefix(code[m[1]:], " -p ") && !strings.HasPrefix(code[m[1]:], " -d -p ") {
				bad = append(bad, fmt.Sprintf("line %d runs mktemp with no -p folder: %s", i+1, code))
			}
		}
		if strings.Contains(code, "${PWD") || strings.Contains(code, "$PWD") {
			bad = append(bad, fmt.Sprintf("line %d cuts a name from $PWD: %s", i+1, code))
		}
	}
	return bad
}

func TestNoMemberTakesATempFolderOrANameThatWindowsLacks(t *testing.T) {
	text := read(t, "Taskfile.yml")
	if bad := windowsPathProblems(text); len(bad) != 0 {
		t.Errorf("the Taskfile: %v", bad)
	}
	// No member runs mktemp since the secret scan moved into gragas-dev (gragas#1655, X171); the check stays for a
	// new one, so its plants are lines added to the text, beside a control that gives mktemp its folder.
	for name, line := range map[string]string{
		"a bare mktemp":    `      log=$(mktemp "$base")`,
		"a mktemp -d bare": `      dir=$(mktemp -d) log=$(mktemp -p "$dir")`,
	} {
		if len(windowsPathProblems(text+"\n"+line+"\n")) == 0 {
			t.Errorf("%s: planted, and the check found nothing", name)
		}
	}
	if bad := windowsPathProblems(text + "\n" + `      log=$(mktemp -p "$base")` + "\n"); len(bad) != 0 {
		t.Errorf("a mktemp given its folder: %v", bad)
	}
	for name, plant := range map[string][2]string{
		"a name cut from PWD": {`"$cache/$name.tsbuildinfo"`, `"$cache/${PWD##*/}.tsbuildinfo"`},
	} {
		planted := strings.Replace(text, plant[0], plant[1], 1)
		if planted == text {
			t.Errorf("%s: the plant changed nothing; the Taskfile's text moved under it", name)
			continue
		}
		if len(windowsPathProblems(planted)) == 0 {
			t.Errorf("%s: planted, and the check found nothing", name)
		}
	}
}

// ptyTaskProblems holds the pty member (gragas#1566) to asking the build: it lists its journeys, says it was not asked
// when the first is not listed, and otherwise runs exactly the journeys it listed. Each name must be a test of
// internal/journeys, so a rename cannot leave the member saying "not asked" on a system that builds them.
func ptyTaskProblems(section string, journeys map[string]bool) []string {
	var bad []string
	run := regexp.MustCompile(`run='\^\(([A-Za-z|]+)\)\$'`).FindStringSubmatch(section)
	if run == nil {
		return append(bad, "it sets no run='^(...)$' of journey names")
	}
	names := strings.Split(run[1], "|")
	for _, n := range names {
		if !journeys[n] {
			bad = append(bad, n+" is not a test of internal/journeys")
		}
	}
	for _, want := range []string{
		`listed=$(go test -timeout {{.TEST_TIMEOUT}} -list "$run" ./internal/journeys) || {`,
		`            *` + names[0] + `*) ;;`,
		`*) echo "pty: not asked: `,
		`go test -count=1 -timeout {{.TEST_TIMEOUT}} -run "$run" ./internal/journeys`,
	} {
		if !strings.Contains(section, want) {
			bad = append(bad, "it lacks "+want)
		}
	}
	if strings.Contains(section, "{{if") || strings.Contains(section, "platforms:") {
		bad = append(bad, "it decides by the system's name, not by what the build holds")
	}
	return bad
}

func TestThePtyMemberSaysNotAskedWhereItsJourneysAreNotBuilt(t *testing.T) {
	t.Parallel()
	journeys := map[string]bool{}
	files, err := filepath.Glob(filepath.Join(root, "internal", "journeys", "*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no test file of internal/journeys: %v", err)
	}
	for _, f := range files {
		for _, m := range regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`).FindAllStringSubmatch(read(t, "internal/journeys/"+filepath.Base(f)), -1) {
			journeys[m[1]] = true
		}
	}
	section := taskSection(t, "pty")
	if bad := ptyTaskProblems(section, journeys); len(bad) != 0 {
		t.Errorf("the pty task (%q): %v", section, bad)
	}
	for name, plant := range map[string][2]string{
		"a journey renamed":       {"TestTheNarrowPropertiesFindEachBreak)", "TestTheNarrowPropertiesFindEachBreakX)"},
		"not asked, said nothing": {`echo "pty: not asked: `, `true "`},
		"the list unread":         {`listed=$(go test -timeout {{.TEST_TIMEOUT}} -list "$run" ./internal/journeys) || {`, `listed=$(true) || {`},
		"by the system's name":    {`      - cmd: |`, `      - cmd: |` + "\n          {{if eq OS \"windows\"}}exit 0{{end}}"},
		"a run of something else": {`-run "$run" ./internal/journeys`, `-run "$run" ./internal/...`},
	} {
		planted := strings.Replace(section, plant[0], plant[1], 1)
		if planted == section {
			t.Errorf("%s: the plant changed nothing; the Taskfile's text moved under it", name)
			continue
		}
		if len(ptyTaskProblems(planted, journeys)) == 0 {
			t.Errorf("%s: planted, and the check found nothing", name)
		}
	}
}

// toolchainPin is GOTOOLCHAIN set to go.mod's toolchain line, for a child that runs the pinned task:
// the isolation gives every child GOTOOLCHAIN=local, and the Taskfile's guard refuses anything but
// the toolchain line, so a test that means to get past the guard names it.
func toolchainPin(t *testing.T) string {
	t.Helper()
	tc := regexp.MustCompile(`(?m)^toolchain (go[0-9.]+)$`).FindStringSubmatch(read(t, "go.mod"))
	if tc == nil {
		t.Fatal("go.mod has no toolchain line")
	}
	return "GOTOOLCHAIN=" + tc[1]
}
