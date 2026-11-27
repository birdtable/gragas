package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// recorder is a go that only records its arguments and the GOPROXY it was given, one line to a call
// in a file of its own, named by its pid, in the folder RECORD_LOG: built, so it is the same on every system.
const recorder = `package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	line := strings.Join(os.Args[1:], " ") + "\t" + os.Getenv("GOPROXY") + "\n"
	_ = os.WriteFile(filepath.Join(os.Getenv("RECORD_LOG"), strconv.Itoa(os.Getpid())), []byte(line), 0o600)
}
`

// commitCalls runs task pre-commit in a scratch folder holding taskfile and a go that only records
// its arguments and the GOPROXY it was given, one line to a call. Every member that runs go is
// reached through the tier, so each line is a go command of the commit's members.
func commitCalls(t *testing.T, taskfile, task string) []string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls")
	if err := os.MkdirAll(log, 0o750); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	for name, text := range map[string]string{"go.mod": "module example.com/recorder\n\ngo 1.24\n", "main.go": recorder} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	build := exec.CommandContext(t.Context(), "go")
	build.Args = append(build.Args, "build", "-o", filepath.Join(bin, "go"+goEXE(t)), ".")
	build.Dir = src
	build.Env = append(configtest.Inherited(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build of the recorder: %v\n%s", err, out)
	}
	// The Taskfile's own variables read these lists before any task runs.
	// The tier's own call of its members, with the setting as a call's var (a command-line one would reach every task
	// and hide a member that forwards nothing); the budget around them is hatch's, which this folder has not got (gragas#1779).
	taskfile += "\n  commit-under-test:\n    cmds:\n      - task: pre-commit-members\n        vars: {NO_NETWORK: \"1\"}\n"
	files := map[string]string{"Taskfile.yml": taskfile}
	for _, n := range []string{"internal/journeys/smoke.txt", "internal/journeys/goldens.txt"} {
		files[n] = read(t, n)
	}
	for name, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The Taskfile is staged, an input of every staged-only member, so each runs.
	gitOut(t, dir, "init", "-q")
	gitOut(t, dir, "add", "Taskfile.yml")
	// GATE is the binary gate-build builds (gragas#2367), and this folder has none: the gate's own calls answer nothing,
	// as the recorder does for go, so a member that runs the gate does not stop the run before the members that run go.
	cmd := exec.CommandContext(t.Context(), task, "commit-under-test", "GATE=true")
	cmd.Dir = dir
	// A test's own GOPROXY=off (configtest) is left out: go-task
	// lets a variable it inherits win over the file's env:, so it would hold
	// every member offline and hide one that does not forward the setting.
	env := slices.DeleteFunc(configtest.Inherited(), func(kv string) bool { return strings.HasPrefix(kv, "GOPROXY=") })
	cmd.Env = append(env, "RECORD_LOG="+log, "PATH="+bin+string(os.PathListSeparator)+inheritedPath())
	out, _ := cmd.CombinedOutput() // the members are red here, the recorder answering nothing: only their calls are read
	entries, err := os.ReadDir(log)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no member ran go: %v\n%s", err, out)
	}
	var lines []string
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(log, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, strings.TrimSpace(string(b)))
	}
	return lines
}

// TestEveryGoCommandOfTheCommitRunsWithTheNetworkOff (gragas#1313): go-task 3.54.0 gives a called task
// the file's env:, never the calling task's, so a GOPROXY set in pre-commit's own env: never reached
// format, generated-files or tidy. The setting is a var each call forwards, and the file's env: reads it.
func TestEveryGoCommandOfTheCommitRunsWithTheNetworkOff(t *testing.T) {
	task := pinnedTask(t)
	taskfile := read(t, "Taskfile.yml")
	off := func(t *testing.T, lines []string) (bad []string) {
		t.Helper()
		for _, l := range lines {
			if !strings.HasSuffix(l, "\toff") {
				bad = append(bad, l)
			}
		}
		return bad
	}
	t.Run("the commit's members run go with GOPROXY off", func(t *testing.T) {
		t.Parallel()
		lines := commitCalls(t, taskfile, task)
		for _, want := range []string{"cmd/gofmt", "mod tidy"} {
			seen := false
			for _, l := range lines {
				seen = seen || strings.Contains(l, want)
			}
			if !seen {
				t.Errorf("no member ran go %s: the plant would show nothing", want)
			}
		}
		if bad := off(t, lines); len(bad) > 0 {
			t.Errorf("go ran with the network on at the commit:\n%s", strings.Join(bad, "\n"))
		}
	})
	t.Run("a member that does not forward the setting is red", func(t *testing.T) {
		t.Parallel()
		from := "      - task: tidy\n        vars: {STAGED_ONLY: \"{{.STAGED_ONLY}}\", NO_NETWORK: \"{{.NO_NETWORK}}\"}\n"
		if strings.Count(taskfile, from) != 1 {
			t.Fatalf("the Taskfile has the tidy call %d times, want once", strings.Count(taskfile, from))
		}
		planted := strings.Replace(taskfile, from, "      - task: tidy\n        vars: {STAGED_ONLY: \"{{.STAGED_ONLY}}\"}\n", 1)
		if bad := off(t, commitCalls(t, planted, task)); len(bad) == 0 {
			t.Error("tidy ran with the network on and nothing was red")
		}
	})
	t.Run("a push, which needs the network, leaves it on", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(taskfile, "GOPROXY: '{{if eq .NO_NETWORK \"1\"}}off{{else}}https://proxy.golang.org,direct{{end}}'") {
			t.Error("the file's GOPROXY is not off for NO_NETWORK 1 and the proxy otherwise")
		}
	})
	t.Run("every call of the commit's own tasks forwards the setting", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"pre-commit", "pre-commit-members", "commit-checks", "secrets-staged"} {
			lines := strings.Split(taskSection(t, name), "\n")
			for i, l := range lines {
				if strings.HasPrefix(strings.TrimSpace(l), "- task:") && (i+1 >= len(lines) || !strings.Contains(lines[i+1], "NO_NETWORK")) {
					t.Errorf("%s: %q passes no NO_NETWORK to the task it calls", name, strings.TrimSpace(l))
				}
			}
		}
	})
}
