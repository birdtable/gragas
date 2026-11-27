package internal

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// goStacks is the line go test prints when a test binary reaches its -timeout, before the stacks of
// every goroutine.
const goStacks = "panic: test timed out after"

// hungRun runs the pinned hatch tests with the Taskfile's own HATCH_TESTS_FLAGS, its --timeout set to
// hatchAfter, over a module whose one test never ends, passing go test -timeout goAfter. It returns
// what the run printed and whether it was green.
func hungRun(t *testing.T, hatchAfter, goAfter string) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	appendTo(t, dir, "go.mod", "module hung\n\ngo 1.21\n")
	appendTo(t, dir, "hung_test.go", `package hung

import (
	"testing"
	"time"
)

func TestHangs(t *testing.T) { time.Sleep(time.Hour) }
`)
	hatch := pinnedPath(t, "hatch")
	if _, err := os.Stat(hatch); err != nil {
		t.Fatalf("the pinned hatch is not installed: run task tools (%v)", err)
	}
	flags := strings.Fields(varLine(read(t, "Taskfile.yml"), "HATCH_TESTS_FLAGS"))
	args := []string{"tests"}
	for i := 0; i < len(flags); i++ {
		if flags[i] == "--timeout" {
			args = append(args, "--timeout", hatchAfter)
			i++
			continue
		}
		args = append(args, flags[i])
	}
	args = append(args, "--", "-count=1", "-timeout", goAfter, "./...")
	res, err := proc.Run(t.Context(), proc.Cmd{
		Path:      hatch,
		Args:      args,
		Dir:       dir,
		Env:       append(configtest.Inherited(), "GOWORK=off", "GOFLAGS="),
		Timeout:   3 * time.Minute,
		MaxOutput: 4 << 20,
		Drain:     10 * time.Second,
	})
	if err != nil {
		if _, ended := errors.AsType[proc.Ended](err); !ended {
			t.Fatalf("hatch tests did not run: %v", err)
		}
	}
	return string(res.Stdout) + string(res.Stderr), err == nil && res.Code == 0
}

// TestAHungTestEndsWithItsStacksUnderHatch holds the order of the two timeouts (gragas#1471): a test
// member's hung test is ended by go test's own -timeout, which prints every goroutine's stack, and not
// by hatch's --timeout, which kills the tree and prints none. It runs the pinned hatch the way the
// Taskfile does, with go test's timeout under hatch's, and the plant is the two at one value, where
// hatch counts the build too and always ends the run first.
func TestAHungTestEndsWithItsStacksUnderHatch(t *testing.T) {
	t.Parallel()
	out, ok := hungRun(t, "2m", "5s")
	if ok || !strings.Contains(out, goStacks) || !strings.Contains(out, "TestHangs") {
		t.Errorf("with hatch's timeout above go test's, the hung test did not end with its stacks (green=%v):\n%s", ok, out)
	}
	out, ok = hungRun(t, "5s", "5s")
	if ok || strings.Contains(out, goStacks) || !strings.Contains(out, "timed out after 5s") {
		t.Errorf("plant: with hatch's timeout at go test's, the run is not red without stacks (green=%v):\n%s", ok, out)
	}
}
