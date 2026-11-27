package coreapi_test

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// TestDiffPresetsKeepsTwoToolsFilesOfOneNameApart: two tools whose files share a name are two files, whichever
// tool sorts first. Risk: a diff keyed by the path alone, so one tool's change
// is read as the other's.
func TestDiffPresetsKeepsTwoToolsFilesOfOneNameApart(t *testing.T) {
	t.Parallel()
	file := func(tool, text string) link.PresetFile {
		return link.PresetFile{Tool: tool, File: model.File{Path: "same.yml", Data: []byte(text)}}
	}
	for name, tools := range map[string][2]string{"the changed tool sorts first": {"aaa", "zzz"}, "the changed tool sorts last": {"zzz", "aaa"}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			before := link.Generated{Files: []link.PresetFile{file(tools[0], "one\n"), file(tools[1], "two\n")}}
			after := link.Generated{Files: []link.PresetFile{file(tools[0], "one\n"), file(tools[1], "changed\n")}}
			d := link.DiffPresets(before, after)
			if len(d) != 1 || d[0].Tool != tools[1] || d[0].Kind != link.FileChanged || !slices.Equal(d[0].Added, []string{"changed"}) {
				t.Errorf("the diff = %+v, want one change of the tool %s", d, tools[1])
			}
			added := link.DiffPresets(link.Generated{Files: before.Files[:1]}, before)
			if len(added) != 1 || added[0].Tool != tools[1] || added[0].Kind != link.FileAdded {
				t.Errorf("a second tool's file of the same name = %+v, want it added", added)
			}
		})
	}
}
