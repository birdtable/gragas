package coreapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ErrNoPlugin is a read of a tool's config for which no plugin of the tool is
// installed. Release 1 ships no built-in adapter, so such a read is
// refused, never answered from a file the engine holds: the person installs a
// plugin of the tool, and the refusal says how.
var ErrNoPlugin = errors.New("no plugin is installed")

// noPlugin is ErrNoPlugin for tool, naming the tool and how to install a
// plugin of it.
func noPlugin(tool string) error {
	return noPluginFor([]string{tool}, "its config cannot be read")
}

// noPluginFor is ErrNoPlugin for tools, with what cannot be done without one,
// and how to install a plugin.
func noPluginFor(tools []string, cannot string) error {
	return fmt.Errorf("%w for %s, so %s; install one with `gragas plugin add <folder>`", ErrNoPlugin, strings.Join(tools, ", "), cannot)
}

// noPresetPlugin is ErrNoPlugin for tools whose preset no installed plugin
// writes: none is installed, or the ones installed have no preset.
func noPresetPlugin(tools []string) error {
	return fmt.Errorf("%w that writes a preset for %s, so its preset cannot be written; install one with `gragas plugin add <folder>`", ErrNoPlugin, strings.Join(tools, ", "))
}

// pluginOf is the installed plugin that serves tool at root, opened, and
// whether there is one: the one chosenPlugins chooses for the tool, as the
// rank and the preset choose it. A plugin that fails to open is an error and
// no other answer is given in its place, and a refused plugin of any tool ends
// the read: a ref carries no tool and a refusal gives no manifest, so a
// refused plugin cannot be told from a tampered plugin of this tool (fail
// closed), and the person must hear of it. The rest are closed.
func pluginOf(ctx context.Context, plugins port.Plugins, root model.Root, tool string, tested *model.Tested) (port.Plugin, bool, error) {
	chosen, err := chosenPlugins(ctx, plugins, root, tested)
	if err != nil {
		return nil, false, err
	}
	pl, found := chosen[tool]
	delete(chosen, tool)
	if err := closePresetPlugins(chosen); err != nil {
		if found {
			err = errors.Join(err, pl.Close())
		}
		return nil, false, err
	}
	return pl, found, nil
}
