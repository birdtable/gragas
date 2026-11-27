package tool_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// TestAZeroScopeRunsItsStepOnThePinnedPath: a step run under a zero Scope is
// given the pinned find's PATH,
// with every folder at or below the project left out, so a program the step
// starts by name is never one the project holds; under OwnPlace it is given
// the PATH whole. Planted: run's PATH taken as OwnPlace's under a zero Scope
// (the zero row red), and the other way (the OwnPlace row red).
func TestAZeroScopeRunsItsStepOnThePinnedPath(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		ownPlace bool
	}{{"a zero Scope", false}, {"OwnPlace", true}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := private(t)
			inside := filepath.FromSlash(root + "/node_modules/.bin")
			if err := os.MkdirAll(inside, 0o700); err != nil {
				t.Fatal(err)
			}
			outside := pathFolder()
			prog := program(t, private(t)+"/lint-a", answering(configtest.Vars("PATH")))
			tl := tool.New(tool.Config{
				Paths: map[string]map[string]string{root: {"lint-a@1.2.3": prog}},
				Path:  pathList(inside, outside), Lang: "C",
			})
			s := plugin.Scope{Manifest: stepManifest(), Root: model.Root(root), OwnPlace: c.ownPlace, Floor: plugin.LevelIsolated, Note: (&notes{}).note,
				Scratch: model.Scratch{Dir: private(t), Level: model.Isolated, Seconds: 60}}
			res, err := bind(t, tl, s).Run(t.Context(), 1, 1, runOf())
			if err != nil {
				t.Fatal(err)
			}
			got := filepath.SplitList(strings.TrimSpace(text(t, res.Stdout)))
			if slices.Contains(got, inside) != c.ownPlace || !slices.Contains(got, outside) {
				t.Errorf("the step's PATH is %q; want %s, which is below the project, in it: %v, and %s in it", got, inside, c.ownPlace, outside)
			}
		})
	}
}
