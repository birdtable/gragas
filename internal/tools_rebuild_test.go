package internal

import (
	"github.com/birdtable/go-files/filestest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/digest"
)

// TestToolsGoBuildsAgainAGoToolChangedPastItsBuildID (gragas#1480): go build reads the build ID inside an existing target
// and writes nothing when that is current, so a go tool with a byte appended,
// its build ID intact, stayed as it was and task tools never repaired it. The
// plant is the clone's own dupl with one byte appended, in a scratch copy of the
// Taskfile, tools.yaml and the tools module; go tool buildid reads the clean
// file's build ID from it. task tools-go, the go tools' part of task tools,
// removes the file that fails its pins before the build, so dupl is built
// again: the clean file, and one of the sha256s tools.yaml pins for dupl. The
// mutant without the rm keeps the plant and is red here.
func TestToolsGoBuildsAgainAGoToolChangedPastItsBuildID(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	gate := gateBinary(t, abs)
	pinned := pinnedTask(t)
	cleanPath := pinnedPath(t, "dupl")
	clean, err := os.ReadFile(cleanPath)
	if err != nil {
		t.Fatal("the pinned dupl is not installed: run task tools")
	}
	dir := t.TempDir()
	for _, name := range []string{"Taskfile.yml", "tools.yaml", "tools/go.mod", "tools/go.sum",
		"internal/journeys/smoke.txt", "internal/journeys/goldens.txt"} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(read(t, name)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(dir, ".tools", "bin", filepath.Base(cleanPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	filestest.WriteProgram(t, target, append(append([]byte{}, clean...), 0))
	buildID := func(p string) string {
		cmd := exec.CommandContext(t.Context(), "go")
		cmd.Args = append(cmd.Args, "tool", "buildid", p)
		cmd.Env = configtest.Inherited()
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go tool buildid %s: %v", p, err)
		}
		return strings.TrimSpace(string(out))
	}
	// Without a build ID in the target, go build always writes it, so the plant
	// would prove nothing: the mutant without the rm stays green on it.
	want := buildID(cleanPath)
	if want == "" {
		t.Fatalf("%s holds no build ID, so it is not the build task tools makes: run task tools", cleanPath)
	}
	if planted := buildID(target); planted != want {
		t.Fatalf("the plant's build ID is %q and the clean dupl's %q: the plant does not keep it, so it proves nothing", planted, want)
	}
	run := exec.CommandContext(t.Context(), "task")
	run.Path, run.Err = pinned, nil // the clone's own, set after the call, as runSecretsFrom does
	run.Args = append(run.Args, "tools-go", "GATE="+gate, "GO_TOOLS=dupl=github.com/mibk/dupl")
	run.Dir = dir
	run.Env = append(configtest.Inherited(), "PATH="+filepath.Dir(pinned)+string(os.PathListSeparator)+inheritedPath())
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("task tools-go over a planted dupl: %v\n%s", err, out)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if digest.Hex(got) != digest.Hex(clean) {
		t.Errorf("after task tools-go, dupl has sha256 %s, want the clean build's %s: the plant was kept", digest.Hex(got), digest.Hex(clean))
	}
	// The pin of any system: the rebuilt file is the clean build of this one, so it
	// must be one of dupl's pins, and this test names no system of its own.
	row := regexp.MustCompile(`(?s)\n  - id: dupl\n.*?\n\n`).FindString(read(t, "tools.yaml"))
	if !strings.Contains(row, "{sha256: "+digest.Hex(got)+"}") {
		t.Errorf("after task tools-go, dupl has sha256 %s, which is none of dupl's pins in tools.yaml", digest.Hex(got))
	}
}
