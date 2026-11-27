package configtest

import (
	"context"
	"time"

	testenv "github.com/birdtable/go-testkit"
)

// rerunLimit and rerunOutput are gragas's own bounds on a child of a rerun,
// not a library default: a helper that hangs is ended at the
// limit, and one that floods is ended at the cap of each stream.
const (
	rerunLimit  = 5 * time.Minute
	rerunOutput = 4 << 20
)

// Rerun is testenv.Rerun under gragas's bounds: a child that runs only the
// package's TestTheHelperRunsOnlyWhenRerun, with args for it to read with
// testenv.Helper, ended after five minutes or four MiB of output. Its results
// are testenv's.
func Rerun(ctx context.Context, args ...string) (code int, out []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, rerunLimit)
	defer cancel()
	return testenv.Rerun(ctx, rerunOutput, args...)
}

// RerunInterrupted is testenv.RerunInterrupted under the same bounds: the
// child is sent the system's interrupt once it calls testenv.Ready.
func RerunInterrupted(ctx context.Context, args ...string) (code int, out []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, rerunLimit)
	defer cancel()
	return testenv.RerunInterrupted(ctx, rerunOutput, args...)
}

// RerunVia is testenv.RerunVia under the same bounds: the child runs behind
// wrapper, a program by its absolute path, as Rerun's does alone.
func RerunVia(ctx context.Context, wrapper []string, args ...string) (code int, out []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, rerunLimit)
	defer cancel()
	return testenv.RerunVia(ctx, rerunOutput, wrapper, args...)
}
