package coreapi

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// configFloor is the level a read of the config named name runs at, which the
// engine chooses and the plugin cannot lower: a
// config that is JSON or YAML, of any case of its extension, is data and runs
// isolated; every other name may be code the tool loads, and runs only
// sandboxed. A read with no config (name "", the tool on its defaults) is the
// one other that runs isolated: its scratch folder holds no file of the
// repository, so there is nothing the tool could load as code, and release 1
// has no sandboxed adapter to run it at.
// What it does not stop: a tool that searches its working folder and every
// parent for its config can see one in the cache folder's parents, as an a.json
// read can. Those parents belong to the user and the system, never to the
// repository, and host/sandbox refuses a cache folder or a parent that another
// account can write.
// A data config can still load code: a plugins key, say. The engine reads no
// tool's keys, so the plugin says so instead. A manifest whose
// config_loads_code is true (loadsCode) raises every config of the tool to
// sandboxed, whatever its extension. The flag only tightens: left
// out, it is false and the floor is as above. A read with no config stays
// isolated even then, as its scratch folder holds no file the tool could load.
// The name is the repository's, so the extension is read as the
// file system gives it and nothing is trusted but the last dot.
func configFloor(name string, loadsCode bool) (plugin.Level, model.Level) {
	if name == "" {
		return plugin.LevelIsolated, model.Isolated // no config: nothing to load
	}
	if loadsCode {
		return plugin.LevelSandboxed, model.Sandboxed
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".json", ".yml", ".yaml":
		return plugin.LevelIsolated, model.Isolated
	}
	return plugin.LevelSandboxed, model.Sandboxed
}

// loadsCode is the manifest's config_loads_code, false when it is left out.
func loadsCode(m plugin.Manifest) bool {
	return m.ConfigLoadsCode != nil && *m.ConfigLoadsCode
}

// configsFloor is the highest floor configFloor gives over names: the one level
// a call that reads several configs at once runs at, so a record and an entries
// call hold the same rule. No name is the floor of no config.
// loadsCode is the manifest's, passed to configFloor for each name, so a record
// of a plugin whose configs load code is held to the floor its entries call is.
func configsFloor(names []string, loadsCode bool) (plugin.Level, model.Level) {
	floor, level := configFloor("", loadsCode)
	for _, n := range names {
		if f, l := configFloor(n, loadsCode); f == plugin.LevelSandboxed {
			// the first that loads code decides: no later name lowers it
			floor, level = f, l
		}
	}
	return floor, level
}

// pluginRead is the ports a read of one config through an opened plugin is made
// with: a scratch folder holding the config, the plan's rounds run by the
// host's step runner bound to the manifest's grants, and the one entries call.
type pluginRead struct {
	sandbox port.Sandbox
	steps   port.Steps
	note    func(ctx context.Context, n plugin.Noted) error
	root    model.Root
	// tree and writer mirror the files a read step matches into the scratch
	// folder (gragas#1173); a read with neither mirrors nothing.
	tree   port.Tree
	writer port.Writer
}

// entries is the entries call over the config named name, whose content is
// data, with the floor configFloor gives the name. The scratch folder holds that
// copy and nothing of the repository, and is released whatever happens; a
// folder that did not reach the level the name asks is refused by the step
// runner when a step asks to run.
func (r pluginRead) entries(ctx context.Context, p port.Plugin, tool string, target plugin.Target, name string, data []byte) (out plugin.EntriesResponse, err error) {
	floor, level := configFloor(name, loadsCode(p.Manifest()))
	dir := path.Dir(path.Clean(name))
	if dir == "." || dir == "/" {
		dir = ""
	}
	var resolve model.Link
	if name != "" {
		// A dependency folder holds code the tool loads, so a read with one
		// runs at the sandboxed level only; below it the step runner refuses,
		// naming the levels (gragas#1173).
		if l, ok := resolveLink(p.Manifest(), r.root, dir); ok {
			resolve, floor, level = l, plugin.LevelSandboxed, model.Sandboxed
		}
	}
	var files []model.File
	cfg := []plugin.Config{}
	if name != "" {
		base := path.Base(name)
		text := string(data)
		files, cfg = []model.File{{Path: base, Data: data}}, []plugin.Config{{Path: base, Data: &plugin.Bytes{Text: &text}}}
	}
	sc, err := r.sandbox.Prepare(ctx, model.Prepare{Level: level, Seconds: lintersSeconds, Config: files, Resolve: resolve})
	if err != nil {
		return plugin.EntriesResponse{}, fmt.Errorf("the scratch folder for %s could not be made: %w", tool, err)
	}
	defer func() {
		if rerr := r.sandbox.Release(sc); rerr != nil && err == nil {
			out, err = plugin.EntriesResponse{}, fmt.Errorf("the scratch folder for %s could not be released: %w", tool, rerr)
		}
	}()
	steps, err := r.steps.Bind(ctx, plugin.Scope{Manifest: p.Manifest(), Root: r.root, Scratch: sc, Floor: floor, Note: r.note})
	if err != nil {
		if errors.Is(err, port.ErrStepNotReady) {
			return plugin.EntriesResponse{}, fmt.Errorf("%s is not ready on this machine: %w", tool, err)
		}
		return plugin.EntriesResponse{}, fmt.Errorf("%s could not be started: %w", tool, err)
	}
	var run plugins.Runner = steps
	if name != "" {
		run = closing{StepRunner: steps, tree: r.tree, writer: r.writer, root: r.root, dir: dir, scratch: model.Root(sc.Dir)}
	}
	got, err := plugins.Entries(ctx, p, run, plugin.PlanRequest{
		ToolVersion: steps.Tool().Version,
		Scratch:     &sc.Dir,
		Config:      cfg,
		Target:      target,
	})
	if err != nil {
		return plugin.EntriesResponse{}, fmt.Errorf("%s could not read the config: %w", tool, err)
	}
	return got, nil
}

// lintersSeconds is the deadline of a plugin's config read. It sets no memory
// limit: gragas sets none of its own, so a host that holds no run's memory runs
// it.
const lintersSeconds = 60
