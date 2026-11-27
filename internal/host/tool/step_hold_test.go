package tool_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// The Steps-only helpers: the rows of step_*_test.go
// that hold what the Tool port, now deleted, and the Steps port shared, through
// Bind and a bound runner alone, with no adapter. They
// are over a tool.Config of the grant and the PATH alone.

// stepOn is a Tool with the PATH path, the settings' tool-paths paths and the
// tools file's tools machine, and no adapter.
func stepOn(path string, paths map[string]map[string]string, machine map[string]string) *tool.Tool {
	return tool.New(tool.Config{Paths: paths, Machine: machine, Path: path, Lang: "C"})
}

// stepOver is stepOn with a PATH of one folder outside every project.
func stepOver(paths map[string]map[string]string, machine map[string]string) *tool.Tool {
	return stepOn(pathFolder(), paths, machine)
}

// stepScope is the scope of a read of lint-a in the project root, its
// scratch folder a new private one: pinned by the zero Scope, or its own place
// with ownPlace.
func stepScope(t *testing.T, root string, ownPlace bool) plugin.Scope {
	t.Helper()
	return plugin.Scope{Manifest: stepManifest(), Root: model.Root(root), OwnPlace: ownPlace, Floor: plugin.LevelIsolated, Note: (&notes{}).note,
		Scratch: model.Scratch{Dir: private(t), Level: model.Isolated, Seconds: 60}}
}

// stepRunOf is the run of one step of lint-a with argv, in round 1, step 1.
func stepRunOf(t *testing.T, r port.StepRunner, argv ...string) (plugin.Result, error) {
	t.Helper()
	return r.Run(t.Context(), 1, 1, runOf(argv...))
}
