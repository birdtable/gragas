package internal

import (
	"os/exec"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestEveryCommandPackageTakesTheUpdateFlag: go test -update over
// internal/cli runs in each of its packages, so the pattern a person writes
// the root's goldens with is not stopped by a package that keeps none
// (#658). -run '^$' runs no test, so nothing is written, whatever a package
// keeps. Risk: a package under internal/cli added with no -update, which
// fails the whole run.
func TestEveryCommandPackageTakesTheUpdateFlag(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-run", "^$", "./internal/cli/...", "-update")
	cmd.Dir = root
	cmd.Env = configtest.Inherited()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test -run '^$' ./internal/cli/... -update: %v\n%s", err, out)
	}
}
