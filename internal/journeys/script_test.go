package journeys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestScripts runs each script of testdata/scripts with the built gragas on
// its PATH. testscript is imported directly: it
// runs its own commands, and the lint on script text (internal/txtar_lint_test.go)
// refuses a script that names a system, so a script is the same on every
// system. Risk: a command-line behaviour held only by a hand-written
// runner, or a script that tests one system and is read as a test of all.
func TestScripts(t *testing.T) {
	t.Parallel()
	bin := gragasBin(t)
	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "scripts"),
		Setup: func(env *testscript.Env) error {
			env.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+env.Getenv("PATH"))
			// The home is the script's own folder, placed by the variable the system
			// reads it from (envName), as commandOf places it: testscript's own is a
			// Unix path that is no home on a system whose home is another variable, and
			// gragas refuses a run with none. No state folder is planted: a script that
			// needs one sets it, and version reads none.
			env.Setenv(envName(homeKey), env.WorkDir)
			return nil
		},
	})
}
