package plugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// interpreterPkg is the package of wazero's interpreter engine, at the
// version go.mod pins; a wazero that moves it turns
// TestTheRuntimeIsTheInterpreterWithNoCompilationCache red, never green.
const interpreterPkg = "github.com/tetratelabs/wazero/internal/engine/interpreter"

// writesNoFile is the mode of TestTheHelperRunsOnlyWhenRerun that compiles
// and calls a plugin and reports every file that appeared.
const writesNoFile = "writes-no-file"

// openAndCall is a rig over echo, opened (which compiles its module) and
// called once.
func openAndCall(t *testing.T) (*rig, port.Plugin) {
	t.Helper()
	r := newRig(t, manifestOf(t, echo().bytes()), echo().bytes())
	p, err := r.open(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Entries(t.Context(), plugin.EntriesRequest{}); err != nil {
		t.Fatal(err)
	}
	return r, p
}

// TestTheRuntimeIsTheInterpreterWithNoCompilationCache: the runtime a
// Host compiles and calls a plugin in is RuntimeConfig's, wazero's interpreter
// with no compilation cache, so it keeps no compiled code and maps no
// executable memory. The lint's arch.ports.6 rows
// refuse every call that would make another; this row reads the one runtime
// made, by its fields at the wazero go.mod pins.
func TestTheRuntimeIsTheInterpreterWithNoCompilationCache(t *testing.T) {
	t.Parallel()
	r, _ := openAndCall(t)
	rt := reflect.ValueOf(r.host.rt).Elem()
	if cache := rt.FieldByName("cache"); !cache.IsValid() || !cache.IsNil() {
		t.Error("the host's runtime has a compilation cache; RuntimeConfig sets none")
	}
	engine := rt.FieldByName("store").Elem().FieldByName("Engine").Elem().Type()
	if engine.Kind() == reflect.Pointer {
		engine = engine.Elem()
	}
	if engine.PkgPath() != interpreterPkg {
		t.Errorf("the host's runtime runs on %s.%s; RuntimeConfig is wazero's interpreter, %s", engine.PkgPath(), engine.Name(), interpreterPkg)
	}
}

// TestACompileAndACallWriteNoFile: a host that opens a
// plugin, which compiles its module, calls it and closes writes no file, under
// the person's home, configuration, cache, state or temp folder or anywhere
// else the test process's isolation set. It runs in a child of
// configtest.Rerun, where it is the only test, so a file that appears there is
// the host's.
func TestACompileAndACallWriteNoFile(t *testing.T) {
	t.Parallel()
	code, out, err := configtest.Rerun(t.Context(), writesNoFile)
	if err != nil || code != 0 {
		t.Errorf("the child that compiled and called a plugin ended %d (%v):\n%s", code, err, out)
	}
}

// TestTheHelperRunsOnlyWhenRerun is a second process, run only as a child of
// configtest.Rerun: writes-no-file lists every path beneath the folder the
// test isolation made the child's home, configuration, cache, state and temp
// folders in (testkit: one folder of the process's own, the temp folder among
// them), opens and calls a plugin, closes the host, and fails on each path that
// appeared; deepest-frame measures what a call to the deepest frame allocates
// (stack_test.go). In a run of the tests it does nothing.
func TestTheHelperRunsOnlyWhenRerun(t *testing.T) {
	t.Parallel() // in a rerun child this is the only test that runs
	args, ok := configtest.Helper()
	if !ok {
		return
	}
	switch {
	case len(args) == 1 && args[0] == writesNoFile:
		compileAndCallWritingNoFile(t)
	case len(args) == 1 && args[0] == deepestFrame:
		measureDeepestFrame(t)
	default:
		t.Fatalf("the helper was rerun with %q; want %s or %s", args, writesNoFile, deepestFrame)
	}
}

// compileAndCallWritingNoFile is writes-no-file's run, made in its child: it
// lists every path beneath the isolation's folder, opens and calls a plugin,
// closes the host, and fails on each path that appeared.
func compileAndCallWritingNoFile(t *testing.T) {
	t.Helper()
	own := filepath.Dir(os.TempDir())
	before := pathsUnder(t, own)
	if len(before) < 2 {
		t.Fatalf("the isolation's folder %s holds %q; want the temp folder and the homes beside it", own, before)
	}
	r, p := openAndCall(t)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, path := range pathsUnder(t, own) {
		if !slices.Contains(before, path) {
			t.Errorf("a compile and a call wrote %s; the runtime writes no file", path)
		}
	}
}

// pathsUnder is every path beneath root, relative to it.
func pathsUnder(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
