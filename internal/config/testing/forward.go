package configtest

import (
	"context"
	"testing"

	testenv "github.com/birdtable/go-testkit"
)

// What testenv answers, handed on under the names the tests already call, so
// this package stays the one importer of go-testkit (arch.ports.6) and
// holds no code of its own for any of it.

// Git is the absolute path of the git this host puts on its PATH:
// testenv.GitPath. A host with no git fails the test.
func Git(t testing.TB) string {
	t.Helper()
	return testenv.GitPath(t)
}

// NoLeaks is testenv.NoLeaks: a test package that starts goroutines ends its
// TestMain with os.Exit(configtest.NoLeaks(configtest.Main(m))), and a leak
// fails the run (go.concurrency.6).
func NoLeaks(code int) int { return testenv.NoLeaks(code) }

// Helper is testenv.Helper: what Rerun gave this process, and whether it is
// a child of a rerun at all.
func Helper() ([]string, bool) { return testenv.Helper() }

// Ready is testenv.Ready: a child of RerunInterrupted says it can take its
// interrupt.
func Ready() error { return testenv.Ready() }

// Interrupted is testenv.Interrupted: a child of RerunInterrupted takes its
// interrupt as a context that ends at it, and says it is ready once the
// handler is in, so no test imports os/signal.
func Interrupted(ctx context.Context) (context.Context, func(), error) {
	return testenv.Interrupted(ctx)
}
