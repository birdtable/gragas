package plugins

import (
	"fmt"

	"github.com/birdtable/gragas/internal/model/plugin"
)

// JudgePreset holds a plugin's preset answer to its manifest: the answer is
// exactly one file, and its path is the manifest's preset.path byte for
// byte. A path that differs by case, by a leading ./,
// by a .. that resolves back to it, or that lands elsewhere is refused, as is
// an answer with no file or with two, and so is a manifest that names no
// preset. The refusal is an *plugin.AnswerError at the path, answered before
// anything is written: the host's writer stays confined to the root whatever
// it is handed, and this is the engine's own check of what the manifest
// promised.
func JudgePreset(m plugin.Manifest, r plugin.PresetResponse) error {
	if m.Preset == nil {
		return &plugin.AnswerError{Why: "the manifest names no preset, so there is no file to write"}
	}
	switch len(r.Files) {
	case 1:
	case 0:
		return &plugin.AnswerError{Path: "/files", Why: "the answer holds no file, and the manifest's preset path is " + plugin.Shown(m.Preset.Path)}
	default:
		return &plugin.AnswerError{Path: "/files/1", Why: fmt.Sprintf("the answer holds %d files, and the manifest's preset is the one at %s", len(r.Files), plugin.Shown(m.Preset.Path))}
	}
	if got := r.Files[0].Path; got != m.Preset.Path {
		return &plugin.AnswerError{Path: "/files/0/path", Why: fmt.Sprintf("the file is at %s, and the manifest's preset path is %s: the two are alike byte for byte", plugin.Shown(got), plugin.Shown(m.Preset.Path))}
	}
	return nil
}
