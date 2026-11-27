package tool

import (
	"strings"

	"github.com/birdtable/go-proc/proc"
)

// The names env sets itself, beyond go-proc's: the search path and the
// language, and the three that keep the go command to the run's Home.
const (
	envPath        = "PATH"
	envLang        = "LANG"
	envGoEnv       = "GOENV"
	envGoToolchain = "GOTOOLCHAIN"
	envGoFlags     = "GOFLAGS"
)

// Reserved is the one list of the names a step's environment sets itself: env's own, and go-proc's, the names Needed keeps, the home
// names HomeEnv points below a Home, and the system names SystemEnv gives. A
// grant's cache_vars may name none of them: the plugin host refuses such a
// manifest at open, from this list and no copy of it, and env sets the
// cache names first, so a name of its own given later would win all the same.
// The names are go-proc's spelling, and the host compares them without
// regard to case on every system, as Windows reads a name. It is a new list
// at each call.
func Reserved() []string {
	names := []string{envPath, envLang, envGoEnv, envGoToolchain, envGoFlags}
	names = append(names, proc.Needed()...)
	for _, kv := range append(proc.HomeEnv(proc.NeutralDir()), proc.SystemEnv()...) {
		name, _, _ := strings.Cut(kv, "=")
		names = append(names, name)
	}
	return names
}
