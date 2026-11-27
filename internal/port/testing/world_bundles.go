package porttest

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// bundlesPort is the fake of port.Bundles over the same world: the bundles a
// test seeded for a root, by the root's name as the port takes it.
type bundlesPort struct{ w *World }

// SeedBundles makes the bundles of root, the folder as the port takes it, what
// the lock of that root names: each ref with the bundle Read answers for it. A
// root seeded with none has no plugins, and Read of a ref not seeded matches
// port.ErrNotInstalled. Seeding a root again replaces it.
func (w *World) SeedBundles(root string, bundles map[model.PluginRef]model.PluginBundle) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.bundles == nil {
		w.bundles = map[string]map[model.PluginRef]model.PluginBundle{}
	}
	w.bundles[root] = bundles
}

// List is the refs seeded for root, ordered by name and then version as bytes.
func (b bundlesPort) List(ctx context.Context, root string) ([]model.PluginRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.w.mu.Lock()
	defer b.w.mu.Unlock()
	seeded, ok := b.w.bundles[root]
	if !ok {
		return nil, b.w.unseeded("Bundles.List", "no bundles were seeded for %s", root)
	}
	var out []model.PluginRef
	for ref := range seeded {
		out = append(out, ref)
	}
	slices.SortFunc(out, func(x, y model.PluginRef) int {
		if c := strings.Compare(x.Name, y.Name); c != 0 {
			return c
		}
		return strings.Compare(x.Version, y.Version)
	})
	return out, nil
}

// Read is the bundle seeded for ref under root.
func (b bundlesPort) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	if err := ctx.Err(); err != nil {
		return model.PluginBundle{}, err
	}
	b.w.mu.Lock()
	defer b.w.mu.Unlock()
	seeded, ok := b.w.bundles[root]
	if !ok {
		return model.PluginBundle{}, b.w.unseeded("Bundles.Read", "no bundles were seeded for %s", root)
	}
	got, ok := seeded[ref]
	if !ok {
		return model.PluginBundle{}, fmt.Errorf("%s@%s: %w", ref.Name, ref.Version, port.ErrNotInstalled)
	}
	return got, nil
}
