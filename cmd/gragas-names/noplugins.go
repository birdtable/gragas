package main

import (
	"context"
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// noPlugins is the Plugins port of the names gate, which runs no plugin:
// every method answers port.ErrUnavailable and nothing pretends to work
// (arch.ports.5, arch.ports.17). A book that lists hand_written_presets is
// then not weighed against the installed plugins, which the load gives as a
// notice; the gate reads names only (gragas#2071).
type noPlugins struct{}

var _ port.Plugins = noPlugins{}

func (noPlugins) List(_ context.Context, root string) ([]model.PluginRef, error) {
	return nil, fmt.Errorf("list the plugins under %q: the names gate runs no plugin: %w", root, port.ErrUnavailable)
}

func (noPlugins) Open(_ context.Context, root string, ref model.PluginRef) (port.Plugin, error) {
	return nil, fmt.Errorf("open the plugin %s %s under %q: the names gate runs no plugin: %w", ref.Name, ref.Version, root, port.ErrUnavailable)
}

func (noPlugins) Try(_ context.Context, m plugin.Manifest, _ []byte) (port.Trial, error) {
	return nil, fmt.Errorf("try the plugin %s %s: the names gate runs no plugin: %w", m.Name, m.Version, port.ErrUnavailable)
}
