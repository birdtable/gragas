package configtest

import (
	"strings"

	testenv "github.com/birdtable/go-testkit"
)

// The crash journey's two settings (store.recovery.2): the seed
// its kill points are drawn from, and how many to draw, which task crash
// sets to the release count. internal/journeys alone keeps them, through
// MainCrash, so they reach it from the run that started it and no other
// package's tests see them.
const (
	CrashSeedVar   = "CRASH_SEED"
	CrashPointsVar = "CRASH_POINTS"
)

// options is what every test package's TestMain hands testenv.Main beyond its
// fixed allow list: the variables gragas's tests keep from the run that
// started them, each by name, and the names one package keeps besides
// (MainCrash).
// Main passes it to testenv.Main, and a TestMain ends in
// os.Exit(configtest.Main(m)), or in os.Exit(testenv.NoLeaks(configtest.Main(m)))
// when the package starts goroutines (tests.isolation.1).
//
// The list is closed: testenv drops every other name, LANG, TERM and the LC_
// ones among them, and a test that needs one gets it kept here by name.
// GOWORK is kept with the value mainKeeping sets, and
// testenv.GoOffline sets the go settings that stop a fetch.
// No credential name, GOAUTH, NETRC or GOFLAGS, is ever kept, nor GOPRIVATE:
// a test's go never fetches (supply.install.10).
func options(keep ...string) []testenv.Option {
	names := append([]string{noWorkspaceVar}, keep...)
	return []testenv.Option{testenv.Keep(names...), testenv.GoOffline()}
}

// noWorkspaceVar is the one go setting gragas keeps beside testenv.GoOffline,
// set to off before testenv.Main : no workspace, so a
// go.work beside the clone never changes the build. testenv.GoOffline's
// GOPROXY=off makes a module missing from the cache red, never fetched, and
// testenv's GOENV=off keeps the person's go env file, and its GOPRIVATE, out.
const noWorkspaceVar = "GOWORK"

// coldCache is the fix a go a test started says when it wanted a module the
// cache lacks: under GOPROXY=off go names the setting and stops.
const coldCache = "the module cache lacks a module this test's go needs, and a test's go never fetches (GOPROXY=off): run task tools, which fills the cache, and run the tests again"

// GoSaid is what a go a test started printed, out, with the fix of a cold
// module cache after it when go stopped on GOPROXY=off: a test that fails
// on a go it ran reports GoSaid(out), so the run names task tools rather
// than a search for a variable to pass.
func GoSaid(out string) string {
	if strings.Contains(out, "GOPROXY=off") {
		return out + "\n" + coldCache
	}
	return out
}
