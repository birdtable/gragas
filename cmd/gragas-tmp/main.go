// Command gragas-tmp makes and prints the stable temp folder the Taskfile's
// cached members hand Go's test cache as TMPDIR (gragas#910, RULED): the folder
// of internal/host/trust's StableTemp, one gragas makes itself at mode 0700
// under os.TempDir(), whose way from / down the trust walk has checked. A
// parent its group can write (a 775 ~/.cache on a host with a group for each
// user) fails here, with the walk's message, and not later as red tests.
//
// main is the only place that exits (go.errors.4).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

func main() {
	os.Exit(run(os.Stdout, os.Stderr, func() (string, error) { return hosttrust.StableTemp(apart()) }))
}

// apart is the folder the command runs in, the work tree, which the temp folder
// may not be inside (gragas#1042): the Taskfile runs it from the repository root.
func apart() []string {
	if wd, err := os.Getwd(); err == nil {
		return []string{wd}
	}
	return nil
}

// run prints the folder make gives and returns 0, or says why not on stderr
// and returns 1.
func run(stdout, stderr io.Writer, make func() (string, error)) int {
	dir, err := make()
	if err != nil {
		if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
			return say(stderr, 1, "gragas-tmp: the temp folder is not trusted: %s\n  fix: %s\n", u.Observed, u.Fix)
		}
		return say(stderr, 1, "gragas-tmp: %v\n", err)
	}
	return say(stdout, 0, "%s\n", dir)
}

// say writes the line and returns the exit code; a write that fails is a
// failure of its own.
func say(w io.Writer, code int, format string, args ...any) int {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return 1
	}
	return code
}
