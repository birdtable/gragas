package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// goEXE is what go env GOEXE prints on this system: ".exe" on Windows,
// nothing elsewhere.
func goEXE(t *testing.T) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "env", "GOEXE")
	cmd.Env = configtest.Inherited()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env GOEXE: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// pinnedPath is the clone's own copy of a tool task tools places, by the name go builds it under here
// (gragas#1370): a test that opens it by its bare name finds nothing on Windows, where it is <tool>.exe.
func pinnedPath(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(abs, ".tools", "bin", name+goEXE(t))
}

// pinnedTask is the clone's own task, by the name go builds it under here.
func pinnedTask(t *testing.T) string {
	t.Helper()
	task := pinnedPath(t, "task")
	if _, err := os.Stat(task); err != nil {
		t.Fatal("the pinned task is not installed: run task tools")
	}
	return task
}

// fakeTool is one program under three names, told apart by the name it runs
// as. As go it answers go env GOEXE with ".exe" and go version -m FILE with
// FILE.info, and does nothing else; as node it prints FAKE_NODE as node
// --version prints on Windows, with a carriage return; as pnpm it does
// nothing. Each call is a file of the folder FAKE_LOG, named by its pid.
const fakeTool = `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	args := os.Args[1:]
	call := filepath.Join(os.Getenv("FAKE_LOG"), strconv.Itoa(os.Getpid()))
	_ = os.WriteFile(call, []byte(name+" "+strings.Join(args, " ")+"\n"), 0o600)
	switch {
	case name == "node":
		fmt.Print("v" + os.Getenv("FAKE_NODE") + "\r\n")
	case name == "go" && len(args) == 2 && args[0] == "env" && args[1] == "GOEXE":
		fmt.Println(".exe")
	case name == "go" && len(args) == 3 && args[0] == "version" && args[1] == "-m":
		b, err := os.ReadFile(args[2] + ".info")
		if err != nil {
			os.Exit(1)
		}
		os.Stdout.Write(b)
	}
}
`

// The task tools half of gragas#1088 (W-2 A, B and C) is held in Go since the node check and
// the go tools' loop left the Taskfile (gragas#1655): cmd/gragas-dev's
// TestToolsCheckHoldsNodeToNodeVersionWhateverTheLineEnds and
// TestToolsGoHoldsAWindowsToolByItsExeNameWithoutAnExecuteBit.

// bareToolLookups are the lines of a test file that open the repository's own pinned tool by a bare name, which
// task tools places as <tool>.exe on Windows (gragas#1370): a join of the root below .tools/bin ending in a
// literal name, and a ./.tools/bin/ path. The helper pinnedPath is the one place the name gets its suffix; a
// scratch folder's own fake (a join of dir, not of the root) is no lookup of the real tool.
func bareToolLookups(text string) []string {
	bare := regexp.MustCompile(`(?:abs|root)\b[^\n]*"\.tools", "bin", "[A-Za-z-]+"\)|"\./\.tools/bin/`)
	var out []string
	for line := range strings.SplitSeq(text, "\n") {
		if bare.MatchString(line) {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

// TestNoTestOpensAPinnedToolByItsBareName holds (gragas#1370) that the tests of internal look a pinned tool
// up through pinnedPath. Planted: a join of the root ending in "node", and a ./.tools/bin/task path, are
// named; the helper's call and a scratch folder's fake are not.
func TestNoTestOpensAPinnedToolByItsBareName(t *testing.T) {
	t.Parallel()
	for _, plant := range []string{
		`os.Stat(filepath.Join(abs, ".tools", "bin", "node"))`,
		`os.Stat(filepath.Join(root, ".tools", "bin", "gotestsum"))`,
		`exec.CommandContext(t.Context(), "./.tools/bin/task")`,
	} {
		if len(bareToolLookups(plant)) != 1 {
			t.Errorf("the bare lookup %q is not named", plant)
		}
	}
	for _, fine := range []string{
		`os.Stat(pinnedPath(t, "node"))`,
		`bin := filepath.Join(dir, ".tools", "bin", "task")`,
		`return filepath.Join(abs, ".tools", "bin", name+goEXE(t))`,
	} {
		if got := bareToolLookups(fine); len(got) != 0 {
			t.Errorf("%q is named: %v", fine, got)
		}
	}
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "pinned_tools_shape_test.go" {
			continue
		}
		for _, line := range bareToolLookups(read(t, "internal/"+f)) {
			t.Errorf("%s opens a pinned tool by its bare name, which Windows has as <tool>.exe (use pinnedPath): %s", f, line)
		}
	}
}
