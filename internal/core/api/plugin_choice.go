package coreapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// chosenPlugins opens the plugins installed under root and keeps one of each
// tool, by tool: the one place a tool's plugin is chosen, for the rankers,
// the listing, the preset, import and audit alike. The kept plugin is the one
// whose range serves the version of its tool the tested set names, else
// the first in the order List gives. Two that both serve the tested version are
// an error, for which one answers is not the engine's to pick. What a chosen
// plugin has no op for is unanswered: no other plugin of the tool takes its
// place. A plugin that is refused, fails to open or fails to close ends the
// read, with every one opened closed; the caller closes each of the map.
func chosenPlugins(ctx context.Context, plugins port.Plugins, root model.Root, tested *model.Tested) (map[string]port.Plugin, error) {
	by := map[string]port.Plugin{}
	if plugins == nil {
		return by, nil
	}
	fail := func(err error) (map[string]port.Plugin, error) {
		return nil, errors.Join(err, closePresetPlugins(by))
	}
	refs, err := plugins.List(ctx, string(root))
	if err != nil {
		return fail(fmt.Errorf("the plugins installed under %s could not be listed: %w", root, err))
	}
	for _, ref := range refs {
		pl, err := plugins.Open(ctx, string(root), ref)
		if errors.Is(err, port.ErrNotApproved) {
			return fail(&UnapprovedPlugin{Ref: ref, Err: err})
		}
		if err != nil {
			return fail(fmt.Errorf("the plugin %s %s could not be opened: %w", ref.Name, ref.Version, err))
		}
		m := pl.Manifest()
		kept, taken := by[m.Tool]
		if !taken {
			by[m.Tool] = pl
			continue
		}
		drop := pl
		var clash error
		switch held := servesTested(tested, kept.Manifest()); {
		case held && servesTested(tested, m):
			k := kept.Manifest()
			clash = fmt.Errorf("the plugins %s %s and %s %s both serve %s %s, the version the tested set names, so which of them answers for it is not known; keep one installed (gragas plugin list lists them)",
				k.Name, k.Version, m.Name, m.Version, m.Tool, testedVersionOf(tested, m.Tool))
		case !held && servesTested(tested, m):
			by[m.Tool], drop = pl, kept
		}
		if err := drop.Close(); err != nil {
			d := drop.Manifest()
			clash = errors.Join(clash, fmt.Errorf("the plugin %s %s could not be closed: %w", d.Name, d.Version, err))
		}
		if clash != nil {
			return fail(clash)
		}
	}
	return by, nil
}

// testedVersionOf is the version of tool the tested set names, "" for none.
func testedVersionOf(tested *model.Tested, tool string) string {
	if tested == nil {
		return ""
	}
	for _, v := range tested.Tools {
		if v.Name == tool {
			return v.Version
		}
	}
	return ""
}

// servesTested says whether the plugin of m serves the version of its tool the
// tested set names; false where the set names none or the range cannot be
// read, which a rank reports when it asks.
func servesTested(tested *model.Tested, m plugin.Manifest) bool {
	v := testedVersionOf(tested, m.Tool)
	if v == "" {
		return false
	}
	vr, err := link.ParseRange(m.ToolVersions)
	if err != nil {
		return false
	}
	in, err := link.InRange(vr, v)
	return err == nil && in
}
