package internal

import (
	"github.com/birdtable/go-files/filestest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// warmLists are the go list runs of warm-members, each as its flags, and what else the task's
// text runs that could start a test (gragas#1663).
func warmLists(section string) (flags [][]string, bad []string) {
	for line := range strings.SplitSeq(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		for _, runs := range []string{"go test", "go build", "go run", "go vet", "hatch tests --", "hatch tests {{", "hatch{{exeExt}} tests --", "hatch{{exeExt}} tests {{"} {
			if strings.Contains(line, runs) {
				bad = append(bad, "warm-members runs "+runs+": the compile must link and run nothing")
			}
		}
		_, rest, ok := strings.Cut(line, "go list ")
		if !ok {
			continue
		}
		var f []string
		for word := range strings.FieldsSeq(rest) {
			if !strings.HasPrefix(word, "-") {
				break
			}
			f = append(f, word)
		}
		flags = append(flags, f)
	}
	return flags, bad
}

// warmShapeProblems holds the text of warm-members to three go list runs: -export -test -deps with
// -race and without, and -export -deps -trimpath of ./cmd/gragas, the build the journeys' TestMain
// makes; and nothing that links or runs a test.
func warmShapeProblems(section string) []string {
	flags, bad := warmLists(section)
	seen := map[string]int{}
	for _, f := range flags {
		key := strings.Join(slices.Sorted(slices.Values(f)), " ")
		switch key {
		case "-deps -export -race -test", "-deps -export -test", "-deps -export -trimpath":
			seen[key]++
		default:
			bad = append(bad, "warm-members runs go list "+strings.Join(f, " ")+", want -export -test -deps with or without -race, or -export -deps -trimpath")
		}
	}
	for key, what := range map[string]string{
		"-deps -export -race -test": "the tests with -race, as test's race run",
		"-deps -export -test":       "the tests without -race",
		"-deps -export -trimpath":   "the journeys' build of gragas with -trimpath",
	} {
		if seen[key] != 1 {
			bad = append(bad, "warm-members compiles "+what+" "+strconv.Itoa(seen[key])+" times, want once")
		}
	}
	if !strings.Contains(section, "go list -export -deps -trimpath ./cmd/gragas >") {
		bad = append(bad, "warm-members's -trimpath compile is not of ./cmd/gragas alone, the package the journeys build")
	}
	return bad
}

// TestWarmCompilesEveryTestPackageAndRunsNone holds the warm member
// (gragas#1663): the compile of full's tests is its own cost under its own budget line, so full's
// line times their run. Its plants are the Taskfile text with a run of the tests in it, or a
// compile gone, which is red. Then each of its go list runs, with the Taskfile's own flags, warms a
// module of one package whose TestMain leaves a mark: the mark is absent after warm, so no test
// ran, and what full makes after it (go test, or go build -trimpath for the journeys' build)
// compiles nothing of the package, so all that is left to full is the link and the run. A control
// without warm compiles it, so the count sees a compile when there is one, and the tests' warm
// alone leaves a -trimpath build cold, which is why warm has a line for it.
func TestWarmCompilesEveryTestPackageAndRunsNone(t *testing.T) {
	t.Parallel()
	section := taskSection(t, "warm-members")
	for _, p := range warmShapeProblems(section) {
		t.Error(p)
	}
	for name, planted := range map[string]string{
		"a go test of no test":      strings.Replace(section, "go list -export -test -deps -race", "go test -race -run '^$'", 1),
		"go test -c":                section + "          go test -c ./...\n",
		"hatch tests":               section + "          {{.TOOLS}}/hatch{{exeExt}} tests {{.HATCH_TESTS_FLAGS}} -- -run '^$' ./...\n",
		"the race compile gone":     strings.Replace(section, " -race ", " ", 1),
		"-export dropped":           strings.Replace(section, "go list -export ", "go list ", 1),
		"-test dropped, no test":    strings.Replace(section, " -test -deps {{.WARM_PLAIN}}", " -deps {{.WARM_PLAIN}}", 1),
		"the trimpath compile gone": strings.Replace(section, "go list -export -deps -trimpath ./cmd/gragas", "true", 1),
		"-trimpath dropped":         strings.Replace(section, " -deps -trimpath ./cmd/gragas", " -deps ./cmd/gragas", 1),
		"another package trimmed":   strings.Replace(section, "-trimpath ./cmd/gragas", "-trimpath ./cmd/gragas-dev", 1),
	} {
		if len(warmShapeProblems(planted)) == 0 {
			t.Errorf("warm-members with %s is not red", name)
		}
	}
	if !strings.Contains(taskSection(t, "warm"), "    internal: true\n") {
		t.Error("warm is not internal: run alone, it would call a hatch no tools-check has held to its pin")
	}
	// the release calls warm inside its budgeted body (gragas#1779)
	for _, s := range []string{"landing", "release-body"} {
		if !strings.Contains(taskSection(t, s), "\n      - task: warm\n") {
			t.Errorf("%s does not call warm", s)
		}
	}
	if !strings.Contains(taskSection(t, "warm"), "BUDGET}} --tier warm ") {
		t.Error("warm has no budget line of its own")
	}

	flags, _ := warmLists(section)
	env := append(configtest.Inherited(), "GOWORK=off", "GOFLAGS=-mod=mod")
	module := func(t *testing.T) (dir, mark string) {
		t.Helper()
		dir = t.TempDir()
		mark = filepath.Join(t.TempDir(), "ran")
		// The nonce, the temporary folder's random path, makes the package new to the build cache, which the run
		// shares with the host.
		nonce := dir
		for name, text := range map[string]string{
			"go.mod": "module example.com/warmplant\n\ngo 1.24\n",
			"a.go":   "package warmplant\n\nconst Nonce = " + strconv.Quote(nonce) + "\n\nfunc N() int { return len(Nonce) }\n",
			"a_test.go": "package warmplant\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
				"func TestMain(m *testing.M) {\n\t_ = os.WriteFile(" + strconv.Quote(mark) + ", nil, 0o600)\n\tos.Exit(m.Run())\n}\n\n" +
				"func TestN(t *testing.T) {\n\tif N() == 0 {\n\t\tt.Fatal(\"no nonce\")\n\t}\n}\n",
		} {
			filestest.WriteFile(t, filepath.Join(dir, name), []byte(text))
		}
		return dir, mark
	}
	// compiles counts the plant's compiles in what full makes after a warm of flags: the journeys'
	// go build -trimpath after the -trimpath warm, go test (with -race after a -race warm) after the others.
	compiles := func(t *testing.T, dir string, flags []string) int {
		t.Helper()
		args := []string{"test", "-x", "-count=1"}
		if slices.Contains(flags, "-trimpath") {
			// The plant is no main package, so go build compiles it and keeps nothing.
			args = []string{"build", "-x", "-trimpath"}
		}
		if slices.Contains(flags, "-race") {
			args = append(args, "-race")
		}
		out, err := goCmd(t, dir, env, append(args, "./...")...).CombinedOutput()
		if err != nil {
			t.Fatalf("go %s of the plant: %v\n%s", args[0], err, out)
		}
		// A line of -x names the program it runs first, spelled as the system spells a path, and with the
		// system's executable suffix: count it by that name's base, not by a slash and a space around it.
		exe, n := goEXE(t), 0
		for line := range strings.SplitSeq(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			// go -x quotes a program whose path holds a space (as a home folder's may), and a split at
			// the space would name half of it: a quoted program is read to its closing quote (gragas#1831).
			first, lead := fields[0], strings.TrimLeft(line, " \t")
			if q := lead[0]; q == '"' || q == '\'' {
				if end := strings.IndexByte(lead[1:], q); end >= 0 {
					first = lead[1 : 1+end]
				}
			}
			prog := filepath.Base(strings.Trim(first, `"'`))
			if strings.TrimSuffix(prog, exe) == "compile" && strings.Contains(line, " -p example.com/warmplant ") {
				n++
			}
		}
		return n
	}
	t.Run("control", func(t *testing.T) {
		t.Parallel()
		dir, mark := module(t)
		if n := compiles(t, dir, nil); n == 0 {
			t.Error("go test of a package new to the cache showed no compile: the count sees nothing")
		}
		if _, err := os.Stat(mark); err != nil {
			t.Error("the plant's TestMain left no mark when its test ran: the mark proves nothing")
		}
	})
	t.Run("the tests' warm leaves a trimmed build cold", func(t *testing.T) {
		t.Parallel()
		dir, _ := module(t)
		if out, err := goCmd(t, dir, env, "list", "-export", "-test", "-deps", "./...").CombinedOutput(); err != nil {
			t.Fatalf("go list: %v\n%s", err, out)
		}
		if n := compiles(t, dir, []string{"-trimpath"}); n == 0 {
			t.Error("go build -trimpath compiled nothing after a warm without -trimpath: the trimpath line is not needed, or the count sees nothing")
		}
	})
	for _, f := range flags {
		t.Run(strings.Join(f, ""), func(t *testing.T) {
			t.Parallel()
			dir, mark := module(t)
			if out, err := goCmd(t, dir, env, append(append([]string{"list"}, f...), "./...")...).CombinedOutput(); err != nil {
				t.Fatalf("go list %s: %v\n%s", strings.Join(f, " "), err, out)
			}
			if _, err := os.Stat(mark); err == nil {
				t.Errorf("go list %s ran the plant's test", strings.Join(f, " "))
			}
			if n := compiles(t, dir, f); n != 0 {
				t.Errorf("full's go after go list %s compiled the package %d times: warm left the compile to full", strings.Join(f, " "), n)
			}
		})
	}
}

// goCmd is go in dir with env. A host where -race does not build fails full's test as well, so the race
// leg runs wherever full does.
func goCmd(t *testing.T, dir string, env []string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go")
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd
}
