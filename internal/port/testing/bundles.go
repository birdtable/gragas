package porttest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// Bundles is the in-memory stand-in of port.Bundles: the
// bundles a test puts under a root, each locked at its digest as put. Its
// digest is pluginbundle.Digest, the one a reader answers, so a bundle put
// here opens through pluginbundle.Judge as a bundle on disk does.
type Bundles struct {
	mu  sync.Mutex
	put map[string]map[model.PluginRef]model.PluginBundle
}

// Wasm is a manifest's wasm value for module, the digest pluginbundle.Judge
// holds the module to, so a test writes the manifest of a module it made.
func Wasm(module []byte) string { return digest.Prefixed(module) }

// sumOf is the digest of every file of b, as a reader takes it.
func sumOf(b model.PluginBundle) model.Digest {
	return pluginbundle.Digest(append([]model.File{{Path: pluginbundle.ManifestFile, Data: b.Manifest}, {Path: pluginbundle.ModuleFile, Data: b.Module}}, b.Other...))
}

// cloneFiles is files, each one's bytes a copy, in the order of its path as
// bytes; none is nil.
func cloneFiles(files []model.File) []model.File {
	if len(files) == 0 {
		return nil
	}
	out := make([]model.File, len(files))
	for i, f := range files {
		out[i] = model.File{Path: f.Path, Data: slices.Clone(f.Data)}
	}
	slices.SortFunc(out, func(a, b model.File) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// Put installs a bundle of manifest, module and the other files at root under
// ref, locked at its digest, in place of any bundle there.
func (s *Bundles) Put(root string, ref model.PluginRef, manifest, module []byte, other ...model.File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.put == nil {
		s.put = map[string]map[model.PluginRef]model.PluginBundle{}
	}
	if s.put[root] == nil {
		s.put[root] = map[model.PluginRef]model.PluginBundle{}
	}
	b := model.PluginBundle{Manifest: slices.Clone(manifest), Module: slices.Clone(module), Other: cloneFiles(other)}
	b.Locked = sumOf(b)
	b.Read = b.Locked
	s.put[root][ref] = b
}

// Change rewrites the module of the bundle at root under ref and leaves its
// lock as it was, as a module changed on disk after it was locked is. A ref
// nothing was put under is port.ErrNotInstalled, and nothing changes.
func (s *Bundles) Change(root string, ref model.PluginRef, module []byte) error {
	return s.edit(root, ref, func(b *model.PluginBundle) { b.Module = slices.Clone(module) })
}

// Add puts f among the other files of the bundle at root under ref, in place
// of any file at its path, and leaves its lock as it was, as a file added to
// the folder after its lock is. A ref nothing was put under is
// port.ErrNotInstalled, and nothing changes.
func (s *Bundles) Add(root string, ref model.PluginRef, f model.File) error {
	return s.edit(root, ref, func(b *model.PluginBundle) {
		other := slices.DeleteFunc(slices.Clone(b.Other), func(g model.File) bool { return g.Path == f.Path })
		b.Other = cloneFiles(append(other, f))
	})
}

// edit changes the bundle at root under ref by change, keeps its lock and
// takes its digest as read again.
func (s *Bundles) edit(root string, ref model.PluginRef, change func(*model.PluginBundle)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.put[root][ref]
	if !ok {
		return fmt.Errorf("%s %s: %w", ref.Name, ref.Version, port.ErrNotInstalled)
	}
	change(&b)
	b.Read = sumOf(b)
	s.put[root][ref] = b
	return nil
}

// List answers the refs put at root, by name and then version, as bytes.
func (s *Bundles) List(ctx context.Context, root string) ([]model.PluginRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []model.PluginRef
	for ref := range s.put[root] {
		out = append(out, ref)
	}
	slices.SortFunc(out, func(a, b model.PluginRef) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Version, b.Version)
	})
	return out, nil
}

// Read answers the bundle put at root under ref, its files a copy, the
// others in the order of their paths as bytes, and
// port.ErrNotInstalled for a ref nothing was put under.
func (s *Bundles) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	if err := ctx.Err(); err != nil {
		return model.PluginBundle{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.put[root][ref]
	if !ok {
		return model.PluginBundle{}, fmt.Errorf("%s %s: %w", ref.Name, ref.Version, port.ErrNotInstalled)
	}
	b.Manifest, b.Module, b.Other = slices.Clone(b.Manifest), slices.Clone(b.Module), cloneFiles(b.Other)
	return b, nil
}

var _ port.Bundles = (*Bundles)(nil)
