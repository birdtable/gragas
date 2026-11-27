package tool_test

import (
	"encoding/json"
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// marking is a program that says its version for --version, leaving a mark
// beside itself, and otherwise prints its arguments: a version run of it
// leaves the file marked, so a test sees that it started.
func marking(at string) string {
	list, err := json.Marshal([]configtest.Step{configtest.Version("lint-a 1.2.3", at+".started"), configtest.PrintArgs()})
	if err != nil {
		panic(err) // a list of steps always marshals
	}
	return fakeBody + string(list)
}

// asked is whether the program at p was asked its version.
func asked(p string) bool {
	_, err := os.Stat(p + ".started")
	return err == nil
}

// at p: a system that reads no #! line (Windows) starts none, and the product
// judges the script alone there. A row that expects a refusal of the chain
// expects it only where go-files gives one: on Windows these rows prove only
// the no-chain leg, and the chain legs are proven on Linux and macOS.
func startsNothing(t *testing.T, p string) bool {
	t.Helper()
	chain, err := gotrust.Interpreters(filepath.FromSlash(p))
	return err == nil && len(chain) == 0
}

// notStarted holds what is true where go-files reads no chain for a script (startsNothing): the walk
// judged the script alone, and the system starts a text file by no interpreter, so the tool is refused as
// a file that is not a program, by go-proc's refusal of the start. That refusal is itself a recorded run
// (outcome records a refused start), so the claim is its words, not that no run is recorded. The legs that
// need a chain are proven where one is read.
func notStarted(t *testing.T, claim string, err error) {
	t.Helper()
	if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "is not a program file") {
		t.Errorf("%s: %v; want go-proc's refusal of the start, as a file that is not a program", claim, err)
	}
}

// version is the step of a built program that says lint-a's version to anything
// it is asked, as the program a script's first line names does when it runs
// the script.
func lintVersion() configtest.Step { return configtest.Out("lint-a 1.2.3\n") }

// built puts a built program at path, which does steps, its folders made 0700,
// and answers where it is. A script's first line names one by its absolute path:
// no test writes a shell script, so no test depends on a shell.
func built(t *testing.T, path string, steps ...configtest.Step) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(place(t, filepath.Dir(path), filepath.Base(path), steps...))
}

// viaEnv is the first line of a script that names the program name through env: the line's
// path to env is testdata/shebang_env.txt's, so no test writes a system's path in its own text.
func viaEnv(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/shebang_env.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + " " + name
}

// script writes a script at path whose first line is first: the program that
// line names, a built one, says its version 1.2.3 when it is run on the script.
func script(t *testing.T, path, first string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	filestest.WriteProgram(t, path, []byte(first+"\n"))
	return path
}

// TestAScriptsInterpreterIsHeldToTheSameWalkAsTheScript: a tool that is a
// script starting /usr/bin/env and a name runs the first program of that name
// on the PATH a run is given, so that program is judged
// like the script; one a group can write, an env option, a relative
// interpreter and a name on no folder of the PATH each refuse the tool by name,
// and a trusted interpreter is the control that finds it.
// Risk: a node planted first on the PATH runs every call of a Node tool, the
// chainOfScripts writes a tool of lint-a that is the first of scripts
// scripts, each starting the next through env and the last starting
// a built program by its absolute path, on a PATH of one folder, and answers the tool's path, the PATH and
// how many files of the chain start with #!, counted from the files.
func chainOfScripts(t *testing.T, root string, scripts int) (tool, path string, shebangs int) {
	t.Helper()
	leaf := built(t, root+"/leaf/sh", lintVersion())
	for i := 1; i < scripts; i++ {
		next := viaEnv(t, "c") + strconv.Itoa(i+1)
		if i == scripts-1 {
			next = "#!" + leaf
		}
		script(t, root+"/chain/c"+strconv.Itoa(i), next)
	}
	first := viaEnv(t, "c1")
	if scripts == 1 {
		first = "#!" + leaf
	}
	tool = script(t, root+"/tools/lint-a", first)
	files, err := filepath.Glob(root + "/*/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil && strings.HasPrefix(string(b), "#!") {
			shebangs++
		}
	}
	return tool, root + "/chain", shebangs
}

// TestAnInterpreterChainIsHeldAtItsBoundary (tests.generated.7): a tool that
// is the first of MaxHops scripts is found, and one that is the first of
// MaxHops+1 is refused as past the chain a walk follows. The chains are
// counted from the files written, so the figure the test holds is not the

// says is echo, saying version v for --version.
func says(v string) string { return strings.Replace(echo, "lint-a 1.2.3", "lint-a "+v, 1) }

// over is a Tool over paths as the settings' tool-paths and machine as the
// tools file's tools, on a PATH of one folder.
func over(paths map[string]map[string]string, machine map[string]string) *tool.Tool {
	return tool.New(tool.Config{Paths: paths, Machine: machine, Path: pathFolder(), Lang: "C"})
}

// node is a stand-in for node: it takes the script as its first argument, and
// says version 1.2.3 for --version and otherwise the script and each argument
// that follows, one to a line.
func node() string {
	return fakeSteps(configtest.Case("* --version*", configtest.Out("lint-a 1.2.3\n")), configtest.ScriptArgs())
}

// pkg is a project with no node_modules/.bin at all, a package lint-a below its
// node_modules whose package.json is body and whose script is bin/lint.js,
// and a folder with node alone, which is the PATH a run is given. It answers
// the project, and a Tool over that PATH.
func pkg(t *testing.T, body string) (string, *tool.Tool) {
	t.Helper()
	root := private(t)
	dir := root + "/node_modules/lint-a"
	if err := os.MkdirAll(dir+"/"+binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for p, b := range map[string]string{dir + "/package.json": body, dir + "/" + binDir + "/lint.js": "// a script\n", dir + "/" + binDir + "/lint": "// a script with no .js\n"} {
		if err := os.WriteFile(p, []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	program(t, root+"/runtime/node", node())
	return root, tool.New(tool.Config{Path: root + "/runtime", Lang: "C"})
}
