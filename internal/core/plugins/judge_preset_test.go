package plugins_test

import (
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// TestAPresetAnswerIsHeldToTheManifestsPath: the one file's path is the
// manifest's preset.path byte for byte; each answer below is refused as an
// AnswerError at the place that is wrong, and the exact one is not.
// Risk: a plugin that writes where the manifest never said, or a spelling of
// the path that a system reads as the same file, passes the review of the
// manifest and writes elsewhere.
func TestAPresetAnswerIsHeldToTheManifestsPath(t *testing.T) {
	t.Parallel()
	m := good()
	file := func(p string) plugin.PresetFile { return plugin.PresetFile{Path: p, Text: "x\n"} }
	for name, tc := range map[string]struct {
		files []plugin.PresetFile
		at    string
	}{
		"the exact path":    {[]plugin.PresetFile{file("lintel.config.mjs")}, ""},
		"a case":            {[]plugin.PresetFile{file("Lintel.config.mjs")}, "/files/0/path"},
		"a leading ./":      {[]plugin.PresetFile{file("./lintel.config.mjs")}, "/files/0/path"},
		"a .. back to it":   {[]plugin.PresetFile{file("sub/../lintel.config.mjs")}, "/files/0/path"},
		"elsewhere":         {[]plugin.PresetFile{file("other.mjs")}, "/files/0/path"},
		"below it":          {[]plugin.PresetFile{file("lintel.config.mjs/x")}, "/files/0/path"},
		"a trailing space":  {[]plugin.PresetFile{file("lintel.config.mjs ")}, "/files/0/path"},
		"no file":           {nil, "/files"},
		"two files":         {[]plugin.PresetFile{file("lintel.config.mjs"), file("lintel.config.mjs")}, "/files/1"},
		"the right and one": {[]plugin.PresetFile{file("lintel.config.mjs"), file("other.mjs")}, "/files/1"},
	} {
		err := plugins.JudgePreset(m, plugin.PresetResponse{Files: tc.files})
		var ae *plugin.AnswerError
		switch {
		case tc.at == "" && err != nil:
			t.Errorf("%s: refused: %v", name, err)
		case tc.at != "" && !errors.As(err, &ae):
			t.Errorf("%s: err = %v, want an AnswerError", name, err)
		case tc.at != "" && ae.Path != tc.at:
			t.Errorf("%s: the answer is refused at %q, want %q", name, ae.Path, tc.at)
		}
	}
	m.Preset = nil
	if err := plugins.JudgePreset(m, plugin.PresetResponse{Files: []plugin.PresetFile{{Path: "a"}}}); err == nil {
		t.Error("a manifest with no preset took an answer")
	}
}
