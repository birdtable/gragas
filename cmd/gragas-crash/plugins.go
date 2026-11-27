package main

import (
	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/core/wire"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	hostplugin "github.com/birdtable/gragas/internal/host/plugin"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// pluginHost is the plugin host as cmd/gragas wires it: the bundles of tree,
// the approvals of the state folder dir, the engine's codec, features and
// judgement of a manifest, and the step runner's own names a cache_vars is
// held to. Its runtime is made at the first Open, so a run over
// a folder with no plugin installed lists none and opens none
// (gragas#2026, gragas#2238). It answers the approvals and the bundles it was
// made over too, which a plugin command reads through the Trust and Bundles
// ports, as cmd/gragas fills them.
func pluginHost(tree *hostfs.Tree, w hostfs.Writer, clk port.Clock, dir string, mounts []string) (port.Plugins, port.Trust, port.Bundles) {
	bundles := hostfs.NewBundles(tree, hostfs.DefaultLimits())
	trust := hostfs.NewApprovalsOfNoClone(w, clk, dir, mounts)
	host, err := hostplugin.New(hostplugin.Config{
		Bundles:  bundles,
		Trust:    trust,
		Judge:    plugins.Judge,
		Codec:    wire.New(tree),
		Features: wire.Features(),
		Reserved: tool.Reserved(),
	})
	if err != nil {
		panic(err) // New refuses only a nil Judge or Codec, and both are set above
	}
	return host, trust, bundles
}
