package internal

import (
	"github.com/birdtable/go-files/filestest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// hooksClone is a repository of the test's own that holds the two hooks of
// this repository and the real text of its hooks and armed tasks, in a
// Taskfile of nothing else, committed with the hooks as programs: git's own
// mode 100755, set by git update-index, not by a mode of the disk's. It
// returns the clone and a function that runs git or the pinned task in it.
func hooksClone(t *testing.T) (string, func(name string, args ...string) (string, error)) {
	t.Helper()
	dir := t.TempDir()
	env := append(configtest.Inherited(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Alex", "GIT_AUTHOR_EMAIL=alex@example.com",
		"GIT_COMMITTER_NAME=Alex", "GIT_COMMITTER_EMAIL=alex@example.com")
	run := func(name string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), name)
		cmd.Args = append(cmd.Args, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	must := func(name string, args ...string) {
		t.Helper()
		if out, err := run(name, args...); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	must("git", "init", "-q", "-b", "main")
	for _, h := range []string{"pre-commit", "pre-push"} {
		p := filepath.Join(dir, "hooks", h)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		filestest.WriteProgram(t, p, []byte(read(t, "hooks/"+h)))
	}
	taskfile := "version: '3'\n\ntasks:\n" + taskSection(t, "hooks") + "\n" + taskSection(t, "armed")
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o600); err != nil {
		t.Fatal(err)
	}
	must("git", "add", "-A")
	must("git", "update-index", "--chmod=+x", "hooks/pre-commit", "hooks/pre-push")
	must("git", "commit", "-q", "-m", "base")
	return dir, func(name string, args ...string) (string, error) {
		if name == "task" {
			return run(pinnedTask(t), args...)
		}
		return run(name, args...)
	}
}

// TestTaskHooksArmsAFreshCloneAndTaskArmedIsRedUntilItDoes (gragas#1220): in
// a clone of its own, task armed is red before task hooks, and names it;
// task hooks sets core.hooksPath to hooks, and task armed is green after. A
// second task hooks changes nothing.
func TestTaskHooksArmsAFreshCloneAndTaskArmedIsRedUntilItDoes(t *testing.T) {
	t.Parallel()
	_, run := hooksClone(t)
	out, err := run("task", "armed")
	if err == nil || !saidLine(out, "armed: ") || !strings.Contains(out, "Run: task hooks") {
		t.Fatalf("task armed in an unarmed clone was not red and did not name task hooks\n%s", out)
	}
	if out, err := run("task", "hooks"); err != nil {
		t.Fatalf("task hooks: %v\n%s", err, out)
	}
	if got, _ := run("git", "config", "--get", "core.hooksPath"); strings.TrimSpace(got) != "hooks" {
		t.Errorf("task hooks set core.hooksPath to %q, want hooks", strings.TrimSpace(got))
	}
	if out, err := run("task", "armed"); err != nil {
		t.Errorf("task armed after task hooks: %v\n%s", err, out)
	}
	if out, err := run("task", "hooks"); err != nil {
		t.Errorf("task hooks twice: %v\n%s", err, out)
	}
}

// TestAPlantedOtherHooksPathIsRefusedAndNeverReplaced (gragas#1220): a clone
// whose core.hooksPath is another folder is red under task armed, and task
// hooks leaves it as it is and says why: it arms to hooks alone and replaces
// no path a person set.
func TestAPlantedOtherHooksPathIsRefusedAndNeverReplaced(t *testing.T) {
	t.Parallel()
	_, run := hooksClone(t)
	if out, err := run("git", "config", "core.hooksPath", "/elsewhere"); err != nil {
		t.Fatalf("planting the path: %v\n%s", err, out)
	}
	if out, err := run("task", "armed"); err == nil || !saidLine(out, "armed: ") || !strings.Contains(out, "'/elsewhere'") {
		t.Errorf("task armed with another path was not red naming it\n%s", out)
	}
	out, err := run("task", "hooks")
	if err == nil || !saidLine(out, "hooks: ") || !strings.Contains(out, "never replaces another path") {
		t.Errorf("task hooks over another path was not refused\n%s", out)
	}
	if got, _ := run("git", "config", "--get", "core.hooksPath"); strings.TrimSpace(got) != "/elsewhere" {
		t.Errorf("task hooks changed the planted path to %q", strings.TrimSpace(got))
	}
}

// TestAHookNotCommittedAsAProgramIsRedUnderTaskArmedAndNothingIsArmedForIt:
// git skips a hook that is not executable, so a clone whose hooks are
// committed as 100644 is not armed by a path alone.
func TestAHookNotCommittedAsAProgramIsRedUnderTaskArmedAndNothingIsArmedForIt(t *testing.T) {
	t.Parallel()
	dir, run := hooksClone(t)
	if out, err := run("git", "update-index", "--chmod=-x", "hooks/pre-push"); err != nil {
		t.Fatalf("planting the mode: %v\n%s", err, out)
	}
	if out, err := run("git", "config", "core.hooksPath", "hooks"); err != nil {
		t.Fatalf("setting the path: %v\n%s", err, out)
	}
	out, err := run("task", "armed")
	if err == nil || !saidLine(out, "armed: hooks/pre-push is not committed as a program") {
		t.Errorf("task armed with a hook of mode 100644 was not red naming it\n%s", out)
	}
	if err := os.Remove(filepath.Join(dir, "hooks", "pre-commit")); err != nil {
		t.Fatal(err)
	}
	if out, err := run("git", "config", "--unset", "core.hooksPath"); err != nil {
		t.Fatalf("unsetting: %v\n%s", err, out)
	}
	if out, err := run("task", "hooks"); err == nil || !saidLine(out, "hooks: hooks/pre-commit is not a regular file") {
		t.Errorf("task hooks without a hook of the repository's own was not refused\n%s", out)
	}
	if got, _ := run("git", "config", "--get", "core.hooksPath"); strings.TrimSpace(got) != "" {
		t.Errorf("a refused task hooks left core.hooksPath %q", strings.TrimSpace(got))
	}
}

// TestTaskHooksNamesOnePathAndTakesNoVariable (gragas#1220): the sanctioned
// way to arm sets hooks and nothing a caller can change: one git config
// line, a literal, in a task with no vars, no requires and no template in
// that line, so no command line, environment or .env picks the path.
func TestTaskHooksNamesOnePathAndTakesNoVariable(t *testing.T) {
	t.Parallel()
	section := taskSection(t, "hooks")
	var writes []string
	for line := range strings.SplitSeq(section, "\n") {
		if strings.Contains(line, "git config") && !strings.Contains(line, "--get") && !strings.HasPrefix(strings.TrimSpace(line), "#") &&
			!strings.Contains(line, "echo") {
			writes = append(writes, strings.TrimSpace(line))
		}
	}
	if len(writes) != 1 || writes[0] != "git config --local core.hooksPath hooks" {
		t.Errorf("the hooks task writes git config with %q, want exactly [git config --local core.hooksPath hooks]", writes)
	}
	// Every read of the path is --local too: plain git config reads and writes
	// where GIT_CONFIG points.
	for _, name := range []string{"hooks", "armed"} {
		for line := range strings.SplitSeq(taskSection(t, name), "\n") {
			if strings.Contains(line, "git config") && !strings.Contains(line, "git config --local") && !strings.Contains(line, "echo") &&
				!strings.HasPrefix(strings.TrimSpace(line), "#") {
				t.Errorf("the %s task runs git config without --local: %q", name, strings.TrimSpace(line))
			}
		}
	}
	for _, key := range []string{"\n    vars:", "\n    requires:", "\n    env:", "\n    dotenv:", "\n    deps:", "\n    dir:"} {
		if strings.Contains(section, key) {
			t.Errorf("the hooks task has %q: it takes nothing from a caller", strings.TrimSpace(key))
		}
	}
	// No template anywhere in either task: a variable of the command line,
	// ROOT_DIR above all, must not steer where it runs or what it arms.
	for _, name := range []string{"hooks", "armed"} {
		if strings.Contains(taskSection(t, name), "{{") {
			t.Errorf("the %s task holds a template: a variable of the command line could steer it to another repository", name)
		}
	}
}

// hooksTaskIn runs the pinned task in dir with args and extra in the environment, through
// the helper the other task tests use, and returns what it printed and whether it succeeded.
func hooksTaskIn(t *testing.T, dir string, extra []string, args ...string) (string, bool) {
	t.Helper()
	return taskWith(t, dir, extra, args...)
}

// hooksPathOf is the core.hooksPath of the repository at dir, "" when unset.
func hooksPathOf(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git")
	cmd.Args = append(cmd.Args, "-C", dir, "config", "--get", "core.hooksPath")
	cmd.Env = append(configtest.Inherited(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// TestNoVariableOfTheCommandLineOrTheEnvironmentPointsTaskHooksOrArmedAtAnotherRepository
// task hooks ROOT_DIR=<another repo>
// armed that other repository while the task cd'd into a templated ROOT_DIR.
// The tasks hold no template now, and stop unless git's top level is the
// folder they run in. Planted, each in a clone of its own with another
// repository beside it: ROOT_DIR= on the command line (it arms the clone
// the Taskfile is in, never the other one), GIT_DIR in the environment (it
// is refused, and neither is armed), and a run from a folder below the
// clone's top (it is refused). task armed is red for the same three.
func TestNoVariableOfTheCommandLineOrTheEnvironmentPointsTaskHooksOrArmedAtAnotherRepository(t *testing.T) {
	t.Parallel()
	t.Run("ROOT_DIR names another repository", func(t *testing.T) {
		t.Parallel()
		here, _ := hooksClone(t)
		other, _ := hooksClone(t)
		if out, ok := hooksTaskIn(t, here, nil, "hooks", "ROOT_DIR="+other); !ok {
			t.Fatalf("task hooks ROOT_DIR=other failed\n%s", out)
		}
		if got := hooksPathOf(t, other); got != "" {
			t.Errorf("task hooks ROOT_DIR=%s armed the other repository: core.hooksPath is %q", other, got)
		}
		if got := hooksPathOf(t, here); got != "hooks" {
			t.Errorf("task hooks ROOT_DIR=other left the clone it ran in with core.hooksPath %q, want hooks", got)
		}
		// The other repository armed on its own: task armed run in an unarmed clone, naming it, is still red.
		unarmed, _ := hooksClone(t)
		if out, ok := hooksTaskIn(t, other, nil, "hooks"); !ok {
			t.Fatalf("task hooks in the other repository failed\n%s", out)
		}
		if out, ok := hooksTaskIn(t, unarmed, nil, "armed", "ROOT_DIR="+other); ok {
			t.Errorf("task armed ROOT_DIR=<an armed repository> was green in an unarmed clone\n%s", out)
		}
	})
	t.Run("GIT_DIR names another repository", func(t *testing.T) {
		t.Parallel()
		here, _ := hooksClone(t)
		other, _ := hooksClone(t)
		env := []string{"GIT_DIR=" + filepath.Join(other, ".git")}
		if out, ok := hooksTaskIn(t, here, env, "hooks"); ok || !saidLine(out, "hooks: GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR") {
			t.Errorf("task hooks with GIT_DIR set was not refused naming it\n%s", out)
		}
		if a, b := hooksPathOf(t, here), hooksPathOf(t, other); a != "" || b != "" {
			t.Errorf("a refused task hooks armed a repository: here %q, other %q", a, b)
		}
		if out, ok := hooksTaskIn(t, here, env, "armed"); ok {
			t.Errorf("task armed with GIT_DIR set was green\n%s", out)
		}
	})
	// git config reads and writes where GIT_CONFIG points, and takes more from
	// GIT_CONFIG_PARAMETERS or GIT_CONFIG_COUNT with its KEY_n and VALUE_n. Each
	// plant carries the value that would arm, "hooks" (a value that task armed is
	// red for anyway proves nothing), into an unarmed clone:
	// task hooks is refused and arms nothing, and task armed stays red, by the
	// task's own refusal line. For GIT_CONFIG, the other repository is armed on
	// its own first, so its config does say hooks.
	for name, env := range map[string]func(other string) []string{
		"GIT_CONFIG names another repository's config": func(other string) []string {
			return []string{"GIT_CONFIG=" + filepath.Join(other, ".git", "config")}
		},
		"GIT_CONFIG_COUNT carries the path": func(string) []string {
			return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=hooks"}
		},
		"GIT_CONFIG_PARAMETERS carries the path": func(string) []string {
			return []string{"GIT_CONFIG_PARAMETERS='core.hookspath=hooks'"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			here, _ := hooksClone(t)
			other, _ := hooksClone(t)
			if out, ok := hooksTaskIn(t, other, nil, "hooks"); !ok {
				t.Fatalf("task hooks in the other repository failed\n%s", out)
			}
			e := env(other)
			if out, ok := hooksTaskIn(t, here, e, "hooks"); ok || !saidLine(out, "hooks: GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR, GIT_CONFIG") {
				t.Errorf("task hooks with %v set was not refused by its own line\n%s", e, out)
			}
			if got := hooksPathOf(t, here); got != "" {
				t.Errorf("a refused task hooks armed the clone: core.hooksPath %q", got)
			}
			// The control: the clone is unarmed, so task armed without the variable is red.
			if out, ok := hooksTaskIn(t, here, nil, "armed"); ok {
				t.Errorf("task armed in an unarmed clone was green\n%s", out)
			}
			if out, ok := hooksTaskIn(t, here, e, "armed"); ok || !saidLine(out, "armed: GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR, GIT_CONFIG") {
				t.Errorf("task armed with %v set, in an unarmed clone, was not refused by its own line\n%s", e, out)
			}
		})
	}
	t.Run("run from below the top level", func(t *testing.T) {
		t.Parallel()
		here, _ := hooksClone(t)
		sub := filepath.Join(here, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		args := []string{"--taskfile", filepath.Join(here, "Taskfile.yml"), "--dir", sub}
		if out, ok := hooksTaskIn(t, here, nil, append(args, "hooks")...); ok || !saidLine(out, "hooks: git's top level here") {
			t.Errorf("task hooks from a folder below the top was not refused\n%s", out)
		}
		if got := hooksPathOf(t, here); got != "" {
			t.Errorf("a refused task hooks armed the clone: core.hooksPath %q", got)
		}
		if out, ok := hooksTaskIn(t, here, nil, append(args, "armed")...); ok {
			t.Errorf("task armed from a folder below the top was green\n%s", out)
		}
	})
}

// saidLine is whether a line of out begins with prefix. A task's own message
// comes on a line of its own ("hooks: …", "armed: …"); go-task also echoes the
// whole script before it runs, and that echo holds each message's text inside
// an echo command, so a Contains on the text is met by the echo whether or not
// the task refused. No echoed line begins with a
// message: each begins with the script's own code.
func saidLine(out, prefix string) bool {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
