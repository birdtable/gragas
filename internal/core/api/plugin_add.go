package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// A PluginAddAsk is a request to add a plugin's bundle to the rulebook at
// Ask.Root: the ref, where the files came from as the lock records it, every
// file of the bundle by path below its folder, and the sizes a bundle may be.
type PluginAddAsk struct {
	Ask    model.ChangeAsk
	Ref    model.PluginRef
	Source string
	Files  []model.File
	Bounds pluginbundle.Bounds
}

// PluginAdd is the change that adds a bundle to the rulebook: the dry run says
// each file and the lock entry, and --apply writes the files and then the lock
// through the one write path, so a run that stops early is rolled back by the
// journal and leaves no lock naming a half bundle. The plan is
// plugins.PlanAdd, made from what the tree holds when Plan runs; nothing to do
// is a plan with no target.
func PluginAdd(ctx context.Context, p port.Ports, ask PluginAddAsk) (*Change, *model.Refusal, error) {
	first, err := pluginAddPlan(p, ask)
	if err != nil {
		return nil, nil, err
	}
	var cur plugins.AddPlan
	paths := make([]string, 0, len(first.Targets))
	for _, t := range first.Targets {
		paths = append(paths, t.Path)
	}
	spec := Spec{
		Command: ask.Ask.Command, Applies: ask.Ask.Applies, Engine: ask.Ask.Engine, Run: ask.Ask.Run,
		Scope: paths,
		Plan: func(context.Context) (model.Plan, error) {
			m, err := pluginAddPlan(p, ask)
			if err != nil {
				return model.Plan{}, err
			}
			cur = m
			lines := []string{"nothing to write: the bundle is locked as these files are"}
			if !m.Nothing {
				lines = lines[:0]
				for _, c := range m.Delta.Changes {
					lines = append(lines, c.Entity+": "+c.After)
				}
				lines = append(lines, "this approves nothing: gragas plugin approve "+ask.Ref.Name+"@"+ask.Ref.Version+" shows what an approval covers")
			}
			return model.Plan{Command: ask.Ask.Command, Delta: m.Delta, Sections: []model.Section{{Title: "Plugin " + ask.Ref.Name + "@" + ask.Ref.Version, Lines: lines}}, Inputs: m.Inputs}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) { return cur.Targets, nil },
	}
	return NewChange(p, ask.Ask.Root, spec), nil, nil
}

// pluginAddPlan reads the lock and the version's folder through the Tree port
// and decides with plugins.PlanAdd.
func pluginAddPlan(p port.Ports, ask PluginAddAsk) (plugins.AddPlan, error) {
	in, err := pluginAddInput(p, ask)
	if err != nil {
		return plugins.AddPlan{}, err
	}
	return plugins.PlanAdd(in)
}

// pluginAddInput is what plugins.PlanAdd decides on: ask's bundle, the lock
// read whole within its bound, and the files below the version's folder.
func pluginAddInput(p port.Ports, ask PluginAddAsk) (plugins.AddInput, error) {
	root := ask.Ask.Root
	in := plugins.AddInput{Ref: ask.Ref, Source: ask.Source, Files: ask.Files, Bounds: ask.Bounds.Held()}
	if st, err := p.Tree.Stat(root, pluginbundle.PluginLockPath); err == nil && st.Size > in.Bounds.Lock {
		return in, fmt.Errorf("%s is %d bytes, over the %d it may be, and was not read: %w", pluginbundle.PluginLockPath, st.Size, in.Bounds.Lock, pluginbundle.ErrOverBound)
	}
	lock, err := p.Tree.ReadSoleWithin(root, pluginbundle.PluginLockPath, in.Bounds.Lock)
	switch {
	case err == nil:
		in.LockFile, in.LockDigest = lock, apply.DigestBytes(lock)
		if in.LockFile == nil {
			in.LockFile = []byte{}
		}
	case errors.Is(err, fs.ErrNotExist):
	default:
		return in, fmt.Errorf("%s could not be read: %w", pluginbundle.PluginLockPath, err)
	}
	es, err := p.Tree.List(root, pluginbundle.FolderOf(ask.Ref), model.Every)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return in, fmt.Errorf("%s could not be listed: %w", pluginbundle.FolderOf(ask.Ref), err)
	}
	for _, e := range es {
		if !e.Dir {
			in.Present = append(in.Present, e.Path)
		}
	}
	return in, nil
}
