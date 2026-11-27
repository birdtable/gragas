// Package configtest is the environment every gragas test runs in, and what
// it never inherits from the run that started it: each test package's
// TestMain is configtest.Main, and a test that starts a child hands it
// Inherited. Both are the call into go-testkit, the one package of the
// module that imports it, and nothing more: no environment work of gragas's
// own is kept beside it (tests.isolation.1), but the go
// setting of GOWORK, set before testenv.Main keeps it, so a test's go
// never fetches and reads no workspace. It is a test-only
// stand-in for config's environment and imports nothing of the module.
package configtest

import (
	"fmt"
	"os"

	testenv "github.com/birdtable/go-testkit"
	"github.com/birdtable/go-testkit/contain"
)

// Main is a test package's TestMain: testenv.Main with gragas's options. It
// runs the tests in the isolated environment and returns their exit code; an
// error of the isolation fails the run and is printed.
func Main(m interface{ Run() int }) int { return mainKeeping(m, nil) }

// MainCrash is Main for internal/journeys alone: it keeps the crash
// settings, CrashSeedVar and CrashPointsVar, besides the shared list, and
// nothing else; internal/testmain_test.go refuses it in any other TestMain.
func MainCrash(m interface{ Run() int }) int {
	return mainKeeping(m, nil, CrashSeedVar, CrashPointsVar)
}

// MainContained is Main with go-testkit's contain: the test binary is run
// again in a user and PID namespace that maps this account to itself and not
// root, so a / shows as the overflow account's and a signal a test sends
// reaches no process outside. It is for a package whose one purpose is a test
// that needs that namespace, internal/journeys/overflow: the whole package runs
// in it, and a test that needs the host's own / does not belong there.
// A host that refuses the namespace runs the tests uncontained,
// where such a test is red, never skipped.
func MainContained(m interface{ Run() int }) int {
	return mainKeeping(m, []testenv.Option{contain.Contain()})
}

// mainKeeping is Main, with the options extra beside gragas's own and the
// variables keep names let through to the package's tests besides the shared
// list. It sets GOWORK=off first, so testenv keeps that value whatever the
// run had.
func mainKeeping(m interface{ Run() int }, extra []testenv.Option, keep ...string) int {
	if err := os.Setenv(noWorkspaceVar, "off"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	code, err := testenv.Main(m, append(options(keep...), extra...)...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// Inherited is the environment for a child a test starts: the isolated
// process's, cleaned of what leaks.
func Inherited() []string { return testenv.Clean(os.Environ()) }
