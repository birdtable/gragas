// Package overflow is the one journey that needs a / the overflow account
// owns, run in go-testkit's contain, which runs this package's test binary
// again in a user and PID namespace that maps this account and not root. It is
// a package of its own because contain wraps a whole test binary, and inside
// that namespace every program the program starts finds / owned by the
// overflow account and is refused, as the program is meant to refuse it: the
// rest of internal/journeys runs the program with git and could not run there.
// It holds no more than the built binary run and its refusal
// read, tests.journeys.2 as internal/journeys and its sub-packages hold it.
package overflow

import (
	"os"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestMain runs the tests in the containment, and in the isolated environment
// every gragas test runs in.
func TestMain(m *testing.M) {
	os.Exit(configtest.MainContained(m))
}
