package coreapi

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/readglob"
)

// A config's set is what the plugin's plan reads (gragas#1173): the scratch
// folder holds the config alone, so before a read step runs, the tracked files
// of the config's own folder that the step's glob matches are mirrored into
// the scratch folder, at the path the glob sees them at. A file no glob matches
// is never copied, so a .env beside the config stays in the project. The mirror
// holds the caps a read is held to (plugin.MaxFile, plugin.MaxFiles) and
// refuses, matching port.ErrRefused, a match past them.

// closing is the runner of a read's steps that mirrors the files a read step
// asks for before the host's runner reads them. A runner with no tree or
// writer mirrors nothing and runs as the host's does.
type closing struct {
	port.StepRunner
	tree    port.Tree
	writer  port.Writer
	root    model.Root // the project
	dir     string     // the config's folder in the project, "" for its root
	scratch model.Root // the scratch folder
}

func (c closing) Run(ctx context.Context, round, step int, s plugin.Step) (plugin.Result, error) {
	if s.Read != nil && c.tree != nil && c.writer != nil {
		if err := c.mirror(s.Read.Glob); err != nil {
			return plugin.Result{}, err
		}
	}
	return c.StepRunner.Run(ctx, round, step, s)
}

// mirror copies the tracked files of the config's folder that glob matches into
// the scratch folder. A glob outside the grammar mirrors nothing: the host's
// read step refuses it.
func (c closing) mirror(glob string) error {
	parts, err := readglob.Parts(glob)
	if err != nil {
		return nil
	}
	entries, err := c.tree.List(c.root, c.dir, model.Tracked)
	if err != nil {
		return fmt.Errorf("the folder of the config could not be listed: %w", err)
	}
	total := 0
	for _, e := range entries {
		if e.Dir || e.Link || e.Special {
			continue
		}
		rel := strings.TrimPrefix(e.Path, c.dir)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || !readglob.Match(parts, strings.Split(rel, "/")) {
			continue
		}
		if e.Size > plugin.MaxFile {
			return fmt.Errorf("the file %s the read matches holds %d bytes, more than the %d a file is given: %w", e.Path, e.Size, plugin.MaxFile, port.ErrRefused)
		}
		total += int(e.Size)
		if total > plugin.MaxFiles {
			return fmt.Errorf("the files the read matches hold more than the %d bytes a read gives: %w", plugin.MaxFiles, port.ErrRefused)
		}
		data, err := c.tree.ReadSoleWithin(c.root, e.Path, plugin.MaxFile)
		if err != nil {
			return fmt.Errorf("the file %s could not be read to mirror: %w", e.Path, err)
		}
		if err := c.writer.Replace(c.scratch, rel, data); err != nil {
			return fmt.Errorf("the file %s could not be mirrored: %w", e.Path, err)
		}
	}
	return nil
}

// resolveLink is the Prepare link of the manifest's resolve_from, the folder
// below the config's own folder the tool finds what the config imports in, and
// whether there is one. The folder is the project's, so the path is absolute
// and clean as the sandbox asks.
func resolveLink(m plugin.Manifest, root model.Root, dir string) (model.Link, bool) {
	if m.ResolveFrom == nil {
		return model.Link{}, false
	}
	return model.Link{Path: *m.ResolveFrom, Dir: path.Join(string(root), dir, *m.ResolveFrom)}, true
}
