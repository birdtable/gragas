package coreapi

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// LocalSource is what the lock records for a bundle added from a folder
// outside the project: where it lay on one person's machine is not a thing a
// project commits.
const LocalSource = "local"

// PluginAddFrom is PluginAdd of the bundle in the folder dir. The plugin is
// named by the layout of the folder the Tree opened, <name>/<version>, as
// plugin test reads it, so the host's own spelling of dir is never parsed. The
// lock records the source as "file:" and the folder's slash path relative to
// the project (ask.Root), or LocalSource for a folder outside it: never an
// absolute path, which would put one person's home folder in a file the
// project commits. The folder is read through the Tree port, each file only
// after its size is known to be within the bound its name has
// (pluginbundle.Bounds.Own) and the bundle's total, and then by a read that
// takes no more than that bound and one byte, so a file that grows between the
// check and the read is refused, not read whole. A link, a hard-linked file or
// a file that is not regular is refused by the port or here, never followed.
// The add approves nothing: the plan says so and names plugin approve.
func PluginAddFrom(ctx context.Context, p port.Ports, dir string, ask model.ChangeAsk) (*Change, *model.Refusal, error) {
	root, err := p.Tree.Open(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("the bundle %s could not be opened: %w", dir, err)
	}
	folder := path.Clean(string(root))
	ref := model.PluginRef{Name: path.Base(path.Dir(folder)), Version: path.Base(folder)}
	if ref.Name == "." || ref.Name == "/" || ref.Version == "." || ref.Version == "/" {
		return nil, nil, fmt.Errorf("the folder %s is not <name>/<version>: name the version's folder of the plugin", dir)
	}
	source, err := bundleSource(p, ask.Root, folder)
	if err != nil {
		return nil, nil, err
	}
	bounds := pluginbundle.DefaultBounds().Held()
	files, err := readBundleFolder(p, root, dir, bounds)
	if err != nil {
		return nil, nil, err
	}
	return PluginAdd(ctx, p, PluginAddAsk{Ask: ask, Ref: ref, Source: source, Files: files, Bounds: bounds})
}

// bundleSource is what the lock records of the folder: "file:" and its path
// relative to the project in slash form, or LocalSource outside it.
func bundleSource(p port.Ports, project model.Root, folder string) (string, error) {
	if folder == path.Clean(string(project)) {
		return "file:.", nil
	}
	rel, err := p.Tree.Within(project, folder)
	switch {
	case err == nil:
		return "file:" + rel, nil
	case errors.Is(err, port.ErrOutside):
		return LocalSource, nil
	}
	return "", fmt.Errorf("where the bundle %s lies in the project could not be told: %w", folder, err)
}

// readBundleFolder reads every file of the opened folder, by path below it, sorted.
func readBundleFolder(p port.Ports, root model.Root, dir string, b pluginbundle.Bounds) ([]model.File, error) {
	return readBundleBelow(p, root, "", dir, b)
}

// readBundleBelow reads every file below the folder sub of root ("" being
// root itself), each by its path below sub, sorted, as readBundleFolder does:
// dir names the folder in a refusal. A sub that is not there is the List
// port's error, which matches fs.ErrNotExist.
func readBundleBelow(p port.Ports, root model.Root, sub, dir string, b pluginbundle.Bounds) ([]model.File, error) {
	es, err := p.Tree.List(root, sub, model.Every)
	if err != nil {
		return nil, fmt.Errorf("the bundle %s could not be listed: %w", dir, err)
	}
	var files []model.File
	var total, listed int64
	for _, e := range es {
		if e.Dir {
			continue
		}
		rel := e.Path
		if sub != "" {
			var ok bool
			if rel, ok = strings.CutPrefix(e.Path, sub+"/"); !ok {
				return nil, fmt.Errorf("%s was listed below %s and is not in it", e.Path, dir)
			}
		}
		if e.Special || e.Link {
			return nil, fmt.Errorf("%s of %s is not a regular file, and is not read", rel, dir)
		}
		if e.Size > b.Own(rel) {
			return nil, fmt.Errorf("%s is %d bytes, over the %d it may hold, and was not read: %w", rel, e.Size, b.Own(rel), pluginbundle.ErrOverBound)
		}
		if listed += e.Size; listed > b.Total {
			return nil, fmt.Errorf("the bundle holds over the %d bytes a bundle may, and was not read further: %w", b.Total, pluginbundle.ErrOverBound)
		}
		data, err := p.Tree.ReadSoleWithin(root, e.Path, b.Own(rel))
		if err != nil {
			return nil, fmt.Errorf("%s of %s could not be read: %w", rel, dir, err)
		}
		if total += int64(len(data)); total > b.Total {
			return nil, fmt.Errorf("the bundle holds over the %d bytes a bundle may, and was not read further: %w", b.Total, pluginbundle.ErrOverBound)
		}
		files = append(files, model.File{Path: rel, Data: data})
	}
	slices.SortFunc(files, func(a, c model.File) int { return strings.Compare(a.Path, c.Path) })
	return files, nil
}
