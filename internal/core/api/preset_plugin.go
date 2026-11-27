package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// presetPlugins are the plugins that write the presets of root, by tool,
// opened: of each tool the one plugin chosenPlugins chooses (the same as the
// rank and the import choose), where its manifest has a preset. A tool whose
// chosen plugin has no preset has none: no other
// plugin of the tool writes it, and the book is not ready. A plugin that is
// refused or fails to open ends the run, for a refusal gives no manifest and
// cannot be told from a tampered plugin of this tool (fail
// closed). The caller closes each.
func presetPlugins(ctx context.Context, p port.Ports, root model.Root, tested *model.Tested) (map[string]port.Plugin, error) {
	chosen, err := chosenPlugins(ctx, p.Plugins, root, tested)
	if err != nil {
		return nil, err
	}
	for _, tool := range canon.SortedKeys(chosen) {
		if chosen[tool].Manifest().Preset != nil {
			continue
		}
		pl := chosen[tool]
		delete(chosen, tool)
		if err := pl.Close(); err != nil {
			return nil, errors.Join(fmt.Errorf("the plugin of %s could not be closed: %w", tool, err), closePresetPlugins(chosen))
		}
	}
	return chosen, nil
}

// closePresetPlugins closes every plugin of by and answers what failed.
func closePresetPlugins(by map[string]port.Plugin) error {
	var errs []error
	for _, tool := range canon.SortedKeys(by) {
		if err := by[tool].Close(); err != nil {
			errs = append(errs, fmt.Errorf("the plugin of %s could not be closed: %w", tool, err))
		}
	}
	return errors.Join(errs...)
}

// generatePresets is the preset files of book as a whole: the one place the
// presets are made, which presetFiles (the files a run writes) and presetsOf
// (what a rule change would write) both take their files from. Every file is
// written by the Preset of the plugin that serves its tool: a
// tool no plugin serves has none, and presetFiles refuses a book that binds
// one. The files are in the order of the tool and path, and the notes are those
// of the entries the plugins were not given (link.Shippable).
func generatePresets(ctx context.Context, p port.Ports, root model.Root, book model.Book, served map[string]port.Plugin) (link.Generated, error) {
	files, notes, err := pluginPresetFiles(ctx, p, root, book, served)
	if err != nil {
		return link.Generated{}, err
	}
	slices.SortFunc(files, func(a, b link.PresetFile) int {
		return canon.Compare(path.Join(a.Tool, a.File.Path), path.Join(b.Tool, b.File.Path))
	})
	slices.Sort(notes)
	return link.Generated{Files: files, Notes: notes}, nil
}

// shippableBy is link.Shippable over the book for the tools of served, with the
// targets each manifest names.
func shippableBy(book model.Book, served map[string]port.Plugin) (map[string][]link.Entry, []string) {
	targets := func(tool string) ([]model.TargetType, bool) {
		pl, ok := served[tool]
		if !ok {
			return nil, false
		}
		var ts []model.TargetType
		for _, t := range pl.Manifest().Targets {
			ts = append(ts, model.TargetType(t))
		}
		return ts, true
	}
	return link.Shippable(book, model.Chain{}, func(tool string) bool { _, ok := served[tool]; return ok }, targets)
}

// shippedBy is each binding the presets of served ship, with the file its
// tool's manifest names, in the order of tool, key and rule: what explain
// says of where an entry went.
func shippedBy(book model.Book, served map[string]port.Plugin) []link.Shipped {
	by, _ := shippableBy(book, served)
	var out []link.Shipped
	for _, tool := range canon.SortedKeys(by) {
		file := served[tool].Manifest().Preset.Path
		for _, e := range by[tool] {
			for _, r := range e.Rules {
				out = append(out, link.Shipped{Rule: r, Binding: e.Binding, File: file})
			}
		}
	}
	return out
}

// pluginPresetFiles is the preset file of each tool of served that the book
// binds, written by the plugin's Preset over the bindings that ship: the
// entries link.Shippable gives, the mark the manifest asks for, and the
// inputs it names, read from the tool's folder in the rulebook's presets. The
// answer is exactly the one file the manifest names (plugins.JudgePreset), and
// is written at presets/<tool>/<path>, copied once where the manifest does not
// extend.
func pluginPresetFiles(ctx context.Context, p port.Ports, root model.Root, book model.Book, served map[string]port.Plugin) ([]link.PresetFile, []string, error) {
	shipped, notes := shippableBy(book, served)
	var out []link.PresetFile
	for _, tool := range canon.SortedKeys(shipped) {
		pl, m := served[tool], served[tool].Manifest()
		req, err := presetRequest(p, root, tool, m, shipped[tool])
		if err != nil {
			return nil, nil, err
		}
		got, err := pl.Preset(ctx, req)
		if err != nil {
			return nil, nil, fmt.Errorf("%s could not write its preset: %w", tool, err)
		}
		if err := plugins.JudgePreset(m, got); err != nil {
			return nil, nil, fmt.Errorf("%s answered a preset that is refused: %w", tool, err)
		}
		// copied once where the manifest does not extend, as an adapter's
		// preset is (generate.go, copyOnce: !ad.Extends); the mark, where the
		// plugin was asked for one, holds a copy to these bytes while it is
		// there
		f := got.Files[0]
		pf := link.PresetFile{File: model.File{Path: f.Path, Data: []byte(f.Text)}, Tool: tool, CopyOnce: !m.Extends}
		if m.Preset.Mark {
			pf.Mark = link.Mark()
		}
		out = append(out, pf)
	}
	return out, notes, nil
}

// presetRequest is what a plugin's Preset is asked: the tool version at the
// lower bound of the manifest's tool_versions, the mark where the manifest
// asks for one, the inputs it names, and one entry for each binding.
func presetRequest(p port.Ports, root model.Root, tool string, m plugin.Manifest, es []link.Entry) (plugin.PresetRequest, error) {
	vr, err := model.ParseRange(m.ToolVersions)
	if err != nil {
		return plugin.PresetRequest{}, fmt.Errorf("%s: the manifest's tool_versions is not a range: %w", tool, err)
	}
	version := vr.Lower.Version
	if version == "" {
		return plugin.PresetRequest{}, fmt.Errorf("%s: the manifest's tool_versions %q has no lower bound, so no version is named to write the preset for", tool, m.ToolVersions)
	}
	req := plugin.PresetRequest{ToolVersion: version}
	if m.Preset.Mark {
		mark := link.Mark()
		req.Mark = &mark
	}
	if req.Inputs, err = presetInputs(p, root, tool, m); err != nil {
		return plugin.PresetRequest{}, err
	}
	for _, e := range es {
		pe, err := presetEntry(tool, e)
		if err != nil {
			return plugin.PresetRequest{}, err
		}
		req.Entries = append(req.Entries, pe)
	}
	return req, nil
}

// presetInputs is each input the manifest names, read from the tool's folder in
// the rulebook's presets; one that is not there is ErrSettingsInputMissing.
func presetInputs(p port.Ports, root model.Root, tool string, m plugin.Manifest) ([]plugin.File, error) {
	out := []plugin.File{}
	for _, in := range m.Preset.Inputs {
		rel := path.Join(presetsDir, tool, in)
		b, err := p.Tree.Read(root, rel)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("%s: %s is not there: %w", tool, rel, ErrSettingsInputMissing)
		case err != nil:
			return nil, fmt.Errorf("%s: %s could not be read: %w", tool, rel, err)
		}
		text := string(b)
		out = append(out, plugin.File{Path: in, Data: plugin.Bytes{Text: &text}})
	}
	return out, nil
}

// presetEntry is the entry a plugin is given for e: its key, target, rules, trial
// and severity, and its value and options as canon values.
func presetEntry(tool string, e link.Entry) (pe plugin.PresetEntry, err error) {
	pe = plugin.PresetEntry{Key: e.Binding.Key, Target: plugin.Target(e.Binding.Target), Trial: e.Trial}
	for _, r := range e.Rules {
		pe.Rules = append(pe.Rules, string(r))
	}
	if e.Binding.Severity != "" {
		s := plugin.Severity(e.Binding.Severity)
		pe.Severity = &s
	}
	if e.Binding.Value != nil {
		if pe.Value, err = link.Canon(e.Binding.Value); err != nil {
			return pe, fmt.Errorf("%s: the value of %s is not one a plugin can be given: %w", tool, e.Binding.Key, err)
		}
	}
	if len(e.Binding.Options) > 0 {
		if pe.Options, err = link.Canon(e.Binding.Options); err != nil {
			return pe, fmt.Errorf("%s: the options of %s are not ones a plugin can be given: %w", tool, e.Binding.Key, err)
		}
	}
	return pe, nil
}
