package fs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/birdtable/go-files/confine"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// Limits are the bounds a bundle is read within, each on the bytes read from
// the handle (bound plus one, refused past it), never on the size a listing
// gave.
type Limits = pluginbundle.Bounds

// Bundles is port.Bundles over a rulebook's plugins folder: the lock names the
// plugins and the folders hold them, so a folder the lock does not name is no
// plugin, and an entry with no folder is an error. It reads through the root
// and judges nothing of the manifest, as the port says; a bundle's add (Add)
// holds it to pluginbundle.Judge.
type Bundles struct {
	tree   *Tree
	limits Limits
}

var _ port.Bundles = Bundles{}

// NewBundles is the Bundles read through t within limits.
func NewBundles(t *Tree, limits Limits) Bundles {
	return Bundles{tree: t, limits: limits.Held()}
}

// lock is the lock of the rulebook at root, its entries by name and then
// version; no lock is none and no error. It is read by ReadSoleWithin, the
// lock's bound.
func (b Bundles) lock(root model.Root) ([]LockEntry, error) {
	raw, err := b.tree.ReadSoleWithin(root, PluginLockPath, b.limits.Lock)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return DecodePluginLock(raw)
}

// List is the plugins the lock of the rulebook at dir holds, by name and then
// version as bytes; none is a nil list and no error. Each is held to have its
// folder: one with none is an error that names it. A folder the lock does not
// name is not listed.
func (b Bundles) List(ctx context.Context, dir string) ([]model.PluginRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := b.tree.Open(dir)
	if err != nil {
		return nil, err
	}
	entries, err := b.lock(root)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	out := make([]model.PluginRef, 0, len(entries))
	for _, e := range entries {
		st, err := b.tree.Stat(root, folderOf(e.Ref))
		if err != nil || !st.Dir {
			return nil, fmt.Errorf("the lock names %s@%s, and its folder %s is missing: %w", e.Ref.Name, e.Ref.Version, folderOf(e.Ref), fs.ErrNotExist)
		}
		out = append(out, e.Ref)
	}
	return out, nil
}

// Find is the plugin of the rulebook at dir that goes by name alone. A name
// the lock holds in two versions is ambiguous: the refusal names both and none
// is picked. A name it does not hold matches port.ErrNotInstalled.
func (b Bundles) Find(ctx context.Context, dir, name string) (model.PluginRef, error) {
	if err := ctx.Err(); err != nil {
		return model.PluginRef{}, err
	}
	root, err := b.tree.Open(dir)
	if err != nil {
		return model.PluginRef{}, err
	}
	entries, err := b.lock(root)
	if err != nil {
		return model.PluginRef{}, err
	}
	var found []string
	var ref model.PluginRef
	for _, e := range entries {
		if e.Ref.Name == name {
			ref = e.Ref
			found = append(found, e.Ref.Name+"@"+e.Ref.Version)
		}
	}
	switch len(found) {
	case 0:
		return model.PluginRef{}, fmt.Errorf("the lock holds no plugin named %q: %w", name, port.ErrNotInstalled)
	case 1:
		return ref, nil
	}
	return model.PluginRef{}, fmt.Errorf("%q is ambiguous, the lock holds %s: %w", name, strings.Join(found, " and "), fs.ErrInvalid)
}

func folderOf(r model.PluginRef) string { return pluginbundle.FolderOf(r) }

// Read is the bundle of ref in the rulebook at dir, every file read through
// the root, each by ReadSoleWithin, its bound on the bytes read: a link, a
// second name, a file that is not regular, a name a disk reads as another and
// a file over its bound, or a bundle over the total, are refused with the
// file named. plugin.yaml and plugin.wasm are answered apart, the other
// files by their path in the order of their path as bytes, and both digests:
// the lock's, and pluginbundle.Digest of the very files read.
func (b Bundles) Read(ctx context.Context, dir string, ref model.PluginRef) (model.PluginBundle, error) {
	if err := ctx.Err(); err != nil {
		return model.PluginBundle{}, err
	}
	root, err := b.tree.Open(dir)
	if err != nil {
		return model.PluginBundle{}, err
	}
	entries, err := b.lock(root)
	if err != nil {
		return model.PluginBundle{}, err
	}
	i := slices.IndexFunc(entries, func(e LockEntry) bool { return e.Ref == ref })
	if i < 0 {
		return model.PluginBundle{}, fmt.Errorf("the lock does not name %s@%s: %w", ref.Name, ref.Version, port.ErrNotInstalled)
	}
	folder := folderOf(ref)
	if st, err := b.tree.Stat(root, folder); err != nil || !st.Dir {
		return model.PluginBundle{}, fmt.Errorf("the folder %s of %s@%s is missing: %w: %w", folder, ref.Name, ref.Version, port.ErrNotInstalled, fs.ErrNotExist)
	}
	listed, err := b.tree.List(root, folder, model.Every)
	if err != nil {
		return model.PluginBundle{}, err
	}
	if err := b.withinTotal(folder, listed); err != nil {
		return model.PluginBundle{}, err
	}
	out, all, err := b.readFiles(ctx, root, folder, listed)
	if err != nil {
		return model.PluginBundle{}, err
	}
	out.Locked = entries[i].Digest
	out.Read = pluginbundle.Digest(all)
	return out, nil
}

// withinTotal refuses a folder whose files, by their listed sizes, hold more
// than the total, before a byte of any is read.
func (b Bundles) withinTotal(folder string, listed []model.Entry) error {
	var n int64
	for _, e := range listed {
		if !e.Special {
			n += e.Size
		}
	}
	if n > b.limits.Total {
		return fmt.Errorf("%s holds %d bytes, over the %d a bundle may hold: %w", folder, n, b.limits.Total, ErrOverBound)
	}
	return nil
}

// readFiles reads each file of listed, each by ReadSoleWithin, its bound, and
// answers the bundle's parts and every file read.
func (b Bundles) readFiles(ctx context.Context, root model.Root, folder string, listed []model.Entry) (model.PluginBundle, []model.File, error) {
	var out model.PluginBundle
	var all []model.File
	var total int64
	for _, e := range listed {
		if e.Dir {
			continue
		}
		if err := ctx.Err(); err != nil {
			return model.PluginBundle{}, nil, err
		}
		rel := strings.TrimPrefix(e.Path, folder+"/")
		bound, err := b.boundOf(e, rel, total)
		if err != nil {
			return model.PluginBundle{}, nil, err
		}
		data, err := b.tree.ReadSoleWithin(root, e.Path, bound)
		if err != nil {
			if errors.Is(err, ErrOverBound) && bound < b.limits.Own(rel) {
				return model.PluginBundle{}, nil, fmt.Errorf("%s: the bundle holds more than the %d bytes a bundle may hold: %w", e.Path, b.limits.Total, err)
			}
			return model.PluginBundle{}, nil, err
		}
		total += int64(len(data))
		all = append(all, model.File{Path: rel, Data: data})
		switch rel {
		case pluginbundle.ManifestFile:
			out.Manifest = slices.Clone(data)
		case pluginbundle.ModuleFile:
			out.Module = slices.Clone(data)
		default:
			out.Other = append(out.Other, model.File{Path: rel, Data: slices.Clone(data)})
		}
	}
	slices.SortFunc(out.Other, func(a, b model.File) int { return strings.Compare(a.Path, b.Path) })
	return out, all, nil
}

// boundOf is the most bytes the file e, below the bundle's folder as rel, may
// be read for: its own bound, and no more than what is left of the total
// after the bytes already read. A file that is not one the reader reads as
// the bundle's own bytes is refused here, before it is opened.
func (b Bundles) boundOf(e model.Entry, rel string, total int64) (int64, error) {
	for seg := range strings.SplitSeq(rel, "/") {
		if _, err := confine.Portable(seg); err != nil {
			return 0, fmt.Errorf("%s: %w", e.Path, err)
		}
	}
	if e.Special {
		return 0, fmt.Errorf("%s is a link or not a regular file: %w", e.Path, fs.ErrInvalid)
	}
	return min(b.limits.Own(rel), b.limits.Total-total), nil
}

// DefaultLimits are the bounds the product reads a bundle within, which the
// add plan holds a bundle to as well: pluginbundle.DefaultBounds.
func DefaultLimits() Limits { return pluginbundle.DefaultBounds() }
