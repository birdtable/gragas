package internal

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/filestest"
)

// TestCannotFailAgreesWithTheGoTaskThatRunsTheMember: the check says a tier
// member cannot fail from reading it, and go-task is what runs it, so each row
// of internal/core/audit/testdata/members.txt is run by the pinned go-task
// (tools.yaml's pin) and its exit status held to the row: 0 for green, not 0 for
// red, and nothing for unjudged, which the check says it did not read. The audit package holds CannotFail to the same rows, so the reading and
// the tool are tied through one table. go-task runs a member with errexit on and
// pipefail off, as the check reads it; a row marked p turns pipefail on in the
// task's own set:. `false; exit 0` exits 201, which is what a reading with
// errexit off got wrong.
func TestCannotFailAgreesWithTheGoTaskThatRunsTheMember(t *testing.T) {
	bin, says := pinnedProgram(t, "task")
	saysItsPin(t, bin, says, exec.CommandContext(t.Context(), "task", "--version"))
	rows := 0
	for line := range strings.SplitSeq(read(t, "internal/core/audit/testdata/members.txt"), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.SplitN(line, " ", 3)
		if len(f) != 3 || (f[0] != "green" && f[0] != "red" && f[0] != "unjudged") || (f[1] != "p" && f[1] != "-") {
			t.Fatalf("bad row %q", line)
		}
		rows++
		set := ""
		if f[1] == "p" {
			set = "    set: [pipefail]\n"
		}
		dir := t.TempDir()
		member := strings.ReplaceAll(strings.ReplaceAll(f[2], "{root}", filepath.ToSlash(dir)), "; ", "\n")
		file := "version: '3'\ntasks:\n  member:\n" + set + "    cmds:\n      - |\n        " + strings.ReplaceAll(member, "\n", "\n        ") + "\n"
		if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := runIn(t, bin, dir, exec.CommandContext(t.Context(), "task", "member"))
		if _, ok := errors.AsType[*exec.ExitError](err); err != nil && !ok {
			t.Fatalf("%q: go-task did not run: %v\n%s", f[2], err, out)
		}
		if f[0] == "unjudged" {
			continue // the check says it did not read it, whatever go-task does
		}
		if exited0 := err == nil; exited0 != (f[0] == "green") {
			t.Errorf("%q (pipefail %s): go-task exits 0 is %v, the row says %s\n%s", f[2], f[1], exited0, f[0], out)
		}
	}
	if rows < 20 {
		t.Errorf("%d rows: the table was emptied", rows)
	}
}

// TestAProgramNotBuiltFromTheToolsLockIsRefused: a source-built tool is held by
// its build info against tools/go.sum, never by a release sha (gragas#1288).
// Each row is its own plant, and the pinned program passes: a file with no build
// info, and a Go program that is not the tools module's (this test's own binary,
// whose main module has no sum), are refused, so the reader cannot be satisfied
// by a program that only says the right version.
func TestAProgramNotBuiltFromTheToolsLockIsRefused(t *testing.T) {
	// A plain file with no build info is read and never run, so the row holds
	// on every system, Windows included.
	script := filepath.Join(t.TempDir(), "task")
	filestest.WriteFile(t, script, []byte("3.54.0\n"))
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for name, bin := range map[string]string{"a file with no build info": script, "a Go program of another module": self} {
		if err := builtPinned(bin); err == nil {
			t.Errorf("%s: builtPinned accepted %s", name, bin)
		}
	}
	bin, _ := pinnedProgram(t, "task")
	if err := builtPinned(bin); err != nil {
		t.Errorf("the pinned program: %v", err)
	}
}
