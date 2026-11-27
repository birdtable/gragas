package internal

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/gitenv/gittest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// read is a file of the repository, read once for the whole run: the tests read the tree and never write it.
func read(t *testing.T, rel string) string {
	t.Helper()
	data, err := theTree.asked.of(nil, []string{"read", rel}, func() ([]byte, error) {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBothHooksAreCommittedExecutable(t *testing.T) {
	t.Parallel()
	// Whether git runs a hook is decided by the mode git stores for it, 100755
	// for an executable, not by the checked-out file's permission bits, which a
	// Windows checkout does not carry (gragas#1371). git ls-files --stage reads
	// the stored mode on every system.
	out, err := inRoot(nil, "git", "ls-files", "--stage", "--", "hooks/pre-commit", "hooks/pre-push").Output()
	if err != nil {
		t.Fatalf("git ls-files --stage: %v", err)
	}
	mode := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		// "<mode> <sha> <stage>\t<path>"
		if tab := strings.IndexByte(line, '\t'); tab > 0 {
			mode[line[tab+1:]] = strings.Fields(line[:tab])[0]
		}
	}
	for _, h := range []string{"hooks/pre-commit", "hooks/pre-push"} {
		switch mode[h] {
		case "100755":
		case "":
			t.Errorf("%s is not a tracked file", h)
		default:
			t.Errorf("%s is committed mode %s, not 100755, so git would skip it", h, mode[h])
		}
	}
	// Each hook is one line past its comments, which runs gragas-dev hook for its own tier from
	// the checkout's source (gragas#1655): the same Go on every system, which runs the clone's
	// .tools/bin/task (cmd/gragas-dev/hook_test.go holds what it does). GOFLAGS and GOWORK are
	// the Taskfile's own values, so a person's flags or workspace never change what the hook is
	// built from, and the task the hook runs inherits values its go-environment guard accepts
	// (an empty GOFLAGS it refuses).
	want := map[string]string{
		"pre-commit": "exec env GOFLAGS=-mod=readonly GOWORK=off go run -trimpath ./cmd/gragas-dev hook pre-commit",
		"pre-push":   `exec env GOFLAGS=-mod=readonly GOWORK=off go run -trimpath ./cmd/gragas-dev hook pre-push "$@"`,
	}
	for name, value := range map[string]string{"GOFLAGS": "-mod=readonly", "GOWORK": `"off"`} {
		if !strings.Contains(read(t, "Taskfile.yml"), "\n  "+name+": "+value+"\n") {
			t.Errorf("Taskfile.yml's env: no longer sets %s: %s, the value the hooks run with", name, value)
		}
	}
	for _, tier := range []string{"pre-commit", "pre-push"} {
		var code []string
		for line := range strings.SplitSeq(strings.TrimRight(read(t, "hooks/"+tier), "\n"), "\n") {
			if strings.HasPrefix(line, "#") {
				continue
			}
			code = append(code, line)
		}
		if !slices.Equal(code, []string{want[tier]}) {
			t.Errorf("the %s hook runs %q, want the one line %q", tier, code, want[tier])
		}
	}
}

// vetOS is go vet of a module of one file for one system, and what it said.
func vetOS(t *testing.T, goos, source string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"go.mod": "module example.com/core\n\ngo 1.24\n", "core.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.CommandContext(t.Context(), "go", "vet", "./...")
	cmd.Dir = dir
	cmd.Env = append(configtest.Inherited(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestThePerOSVetRunsForEverySystemInTheList(t *testing.T) {
	t.Parallel()
	text := read(t, "Taskfile.yml")
	list, err := systems()
	if err != nil || len(list) < 3 {
		t.Fatalf("the Taskfile has no list of systems: %v %v", list, err)
	}
	_, rest, ok := strings.Cut(text, "\n  vet:\n")
	if !ok {
		t.Fatal("the Taskfile has no vet task")
	}
	if i := strings.Index(rest, "\n  lint:\n"); i >= 0 {
		rest = rest[:i]
	}
	for _, want := range []string{"ref: .CROSS", "GOOS=", "go vet ./..."} {
		if !strings.Contains(rest, want) {
			t.Errorf("the vet task has no %q:\n%s", want, rest)
		}
	}
	if !strings.Contains(text, "      - task: vet\n") {
		t.Error("the per-OS vet is not a member of a tier (supply.release.7)")
	}
}

func TestAnOSSpecificCallInTheCoreFailsTheVetOfAnotherSystem(t *testing.T) {
	t.Parallel()
	const clean = "package core\n\nfunc Add(a, b int) int { return a + b }\n"
	const planted = "package core\n\nimport \"syscall\"\n\nfunc Stop(pid int) error { return syscall.Kill(pid, 0) }\n"
	if out, err := vetOS(t, "windows", clean); err != nil {
		t.Fatalf("the clean control fails the windows vet: %v\n%s", err, out)
	}
	if out, err := vetOS(t, "linux", planted); err != nil {
		t.Fatalf("the planted call is fine on the system it is for: %v\n%s", err, out)
	}
	out, err := vetOS(t, "windows", planted)
	if err == nil || !strings.Contains(out, "Kill") {
		t.Errorf("the planted syscall.Kill passed the windows vet: %v\n%s", err, out)
	}
}

// pipeBreakers are the commands that stop reading early and so can kill
// their writer with a broken pipe under pipefail (gates.command.13).
var pipeBreakers = regexp.MustCompile(`\|\s*(grep\s+(-[a-zA-Z]*q|--quiet)|head|read)\b`)

// pipefailBreaches is the files, by name with their text, that set pipefail
// and pipe into grep -q, head or read.
func pipefailBreaches(files map[string]string) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		text := files[name]
		if !strings.Contains(text, "pipefail") {
			continue
		}
		for i, l := range strings.Split(text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			if pipeBreakers.MatchString(l) {
				out = append(out, name+":"+strconv.Itoa(i+1)+": pipes into a command that stops early under pipefail (gates.command.13)")
			}
		}
	}
	return out
}
func TestNoScriptPipesIntoGrepQHeadOrReadUnderPipefail(t *testing.T) {
	t.Parallel()
	files := map[string]string{"Taskfile.yml": read(t, "Taskfile.yml")}
	for _, dir := range []string{"hooks", ".github/workflows", "scripts"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				files[dir+"/"+e.Name()] = read(t, dir+"/"+e.Name())
			}
		}
	}
	if len(files) < 3 {
		t.Fatalf("read %d scripts; the scan found too few to mean anything", len(files))
	}
	if b := pipefailBreaches(files); len(b) > 0 {
		t.Errorf("pipefail scripts:\n%s", strings.Join(b, "\n"))
	}
}

func TestAPipefailScriptThatPipesIntoGrepQHeadOrReadFails(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]string{
		"grep -q": "git log | grep -q fix",
		"head":    "git log | head -1",
		"read":    "git log | read first",
	} {
		b := pipefailBreaches(map[string]string{"s.sh": "set -o pipefail\n" + line + "\n"})
		fires(t, b, "s.sh:2: pipes into a command that stops early")
		if len(b) == 0 {
			t.Errorf("%s was not refused", name)
		}
	}
	if b := pipefailBreaches(map[string]string{"s.sh": "git log | grep -q fix\n"}); len(b) > 0 {
		t.Errorf("a script without pipefail was refused: %q", b)
	}
	if b := pipefailBreaches(map[string]string{"s.sh": "set -o pipefail\ngit log | grep fix\n# git log | head -1\n"}); len(b) > 0 {
		t.Errorf("a clean pipe or a comment was refused: %q", b)
	}
}

// A hook's line ends are \n on every system (gragas#1297): the pre-push hook compares its sources with the
// pushed commit's, and a shell script with a carriage return fails.
func TestTheHooksAreCheckedOutWithLineFeeds(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "git", "check-attr", "text", "eol", "--", "hooks/pre-commit", "hooks/pre-push")
	cmd.Dir = root
	cmd.Env = gittest.Git(t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git check-attr: %v\n%s", err, out)
	}
	for _, h := range []string{"hooks/pre-commit", "hooks/pre-push"} {
		for _, want := range []string{h + ": text: set", h + ": eol: lf"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s is not held to line feeds: want %q in\n%s", h, want, out)
			}
		}
	}
}
