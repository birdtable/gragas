package coreapi

import (
	"context"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// handWrittenFindings are the findings at load of the identity's list of the
// tools whose presets are hand-written:
// a listed tool no active or trial rule binds, so no entry stays on the list
// once its rules are gone; and a listed tool an installed plugin serves, so
// the list never silences a plugin that can rank and write the tool. The
// second opens the installed plugins to read their manifests, and only for a
// book that lists a tool; when they cannot be read the list is not weighed
// against them, which is a notice, as for a read whose Plugins port answers
// ErrUnavailable.
func handWrittenFindings(ctx context.Context, p port.Ports, root model.Root, l format.Loaded) (found, notices []format.Finding) {
	listed := l.Book.Identity.HandWrittenPresets
	if len(listed) == 0 {
		return nil, nil
	}
	file := below(l.Dir, format.IdentityFile)
	bound := map[string]bool{}
	for _, r := range l.Book.Rules {
		if r.Status != model.Active && r.Status != model.Trial {
			continue
		}
		for _, b := range r.Bindings {
			bound[b.Tool] = true
		}
	}
	for _, tool := range listed {
		if !bound[tool] {
			found = append(found, format.Finding{File: file,
				Msg: "hand_written_presets lists " + tool + ", which no active or trial rule binds",
				Fix: "take " + tool + " off hand_written_presets"})
		}
	}
	served, err := servingPlugins(ctx, p.Plugins, root, l.Book.Identity.Tested)
	if err != nil {
		return found, append(notices, format.Finding{File: file,
			Msg: "hand_written_presets was not weighed against the installed plugins: " + err.Error()})
	}
	for _, tool := range listed {
		if pl, ok := served[tool]; ok {
			found = append(found, format.Finding{File: file,
				Msg: "hand_written_presets lists " + tool + ", which the installed plugin " + pl.Name + " " + pl.Version + " serves",
				Fix: "take " + tool + " off hand_written_presets, or remove the plugin"})
		}
	}
	return found, notices
}
