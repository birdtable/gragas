// Command gragas-subset is the subset build of gragas: the same command tree
// with the Assets port filled by an adapter that answers port.ErrUnavailable,
// so it links no embedded file (arch.layers.13, A61). The gate builds it in
// every run and checks its package list; nothing ships it.
package main

import (
	"context"
	"os"

	"github.com/birdtable/gragas/internal/cli/root"
	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/host/clock"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/host/toml"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// ports are cmd/gragas's old ports, the old lock's walk trusting the
// configured mounts (gragas#1803).
func ports(cfg config.Config) legacy.Ports {
	return legacy.Ports{
		Files:   hostfs.Files{},
		Writer:  hostfs.LegacyWriter{},
		Schemas: jsonschema.Schemas{},
		Locks:   hostfs.LegacyLocks{Mounts: cfg.Mounts, Clock: clock.Clock{}},
		Runs:    hostfs.Runs{},
		TOML:    toml.Reader{},
	}
}

// world is the ports as far as they are built, as cmd/gragas fills
// them, with Assets answering ErrUnavailable.
func world(out port.Terminal) port.Ports {
	tree := &hostfs.Tree{}
	return port.Ports{
		Tree:     tree,
		Owned:    tree,
		Clock:    clock.Clock{},
		Assets:   unavailable{},
		Terminal: out,
	}
}

func main() {
	// No signal is handled here: this build links no host/term, whose palette is
	// an embedded file (A61), so no kit stands in front of the tree and Ctrl+C
	// takes Go's default. It is built to be checked and never run or shipped
	// (gragas#917; cmd/gragas leaves its signals to the kit).
	ctx := context.Background()
	cfg, problem := config.Load(os.Getenv, root.ConfigFile(ctx, os.Args[1:]), hosttrust.ReadOwned)
	os.Exit(root.Execute(ctx, root.Process{
		Args:        os.Args[1:],
		Stdout:      os.Stdout,
		Config:      cfg,
		Problem:     problem,
		Ports:       ports(cfg),
		World:       world(noTerminal{}),
		Diagnostics: noTerminal{},
	}))
}
