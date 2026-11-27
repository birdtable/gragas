// Command gragas checks the Gragás rulebook and writes the files made from it.
//
// main is the one place that names both the commands and the adapters they
// reach only through the ports: it fills every port, by hand, runs the
// command tree and exits.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/birdtable/go-console/shell"

	"github.com/birdtable/gragas/internal/cli/root"
	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/core/wire"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/assets"
	"github.com/birdtable/gragas/internal/host/clock"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/host/jsonschema"
	hostplugin "github.com/birdtable/gragas/internal/host/plugin"
	"github.com/birdtable/gragas/internal/host/sandbox"
	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/host/toml"
	"github.com/birdtable/gragas/internal/host/tool"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// Stamped at build time by the Taskfile. Empty in a plain `go build`.
var (
	version = ""
	commit  = "" //nolint:gochecknoglobals // stamped by the linker, as version is; nothing writes it at run time
)

// ports are the adapters, each in the port the core asks for. The old lock's
// walk trusts the configured mounts, as every walk does, and
// its folder, the state folder, is judged against the folders a clone carries
// as world's lock is, with the root -C names, so the run
// recorder neither takes its lock nor begins its record in one.
func ports(cfg config.Config) legacy.Ports {
	return legacy.Ports{
		Files:   hostfs.Files{},
		Writer:  hostfs.LegacyWriter{},
		Schemas: jsonschema.Schemas{},
		Locks:   hostfs.LegacyLocks{Mounts: cfg.Mounts, Clock: clock.Clock{}, Roots: stateRoots(cfg, config.SystemHome, carriedRoots(&hostfs.Tree{Mounts: cfg.Mounts}, cfg.CacheDir))},
		Runs:    hostfs.Runs{},
		TOML:    toml.Reader{},
	}
}

// world is the adapters of the seventeen ports, as far as they are built. The
// rest are filled by the tasks that build them. Sandbox makes its scratch
// folders in the cache folder. Schema is compiled from the
// format schemas the assets hold, bytes read through the Assets port. The
// subset build fills Assets with an adapter that answers ErrUnavailable (A61).
func world(out port.Terminal, cfg config.Config) port.Ports {
	a := assets.New()
	tree := &hostfs.Tree{Mounts: cfg.Mounts}
	writer := hostfs.Writer{Tree: tree}
	var age time.Duration // zero is the journal's own thirty days, for a keep that did not parse
	if secs, err := cfg.Settings.KeepSeconds(); err == nil {
		age = time.Duration(secs) * time.Second
	}
	// The journal writes through the state folder the lock holds open, while
	// it holds it: the two share the tree.
	// The approvals, the lock and the journal judge the one state folder
	// against the same folders a clone carries; the lock and
	// the journal only one a variable moved.
	roots := carriedRoots(tree, cfg.CacheDir)
	judged := stateRoots(cfg, config.SystemHome, roots)
	lock := &hostfs.Lock{Dir: cfg.StateDir, Mounts: cfg.Mounts, Clock: clock.Clock{}, Tree: tree, Roots: judged}
	trust := hostfs.NewApprovals(writer, clock.Clock{}, cfg.StateDir, cfg.Mounts, roots)
	bundles := hostfs.NewBundles(tree, hostfs.DefaultLimits())
	plugins := pluginsOf(tree, trust, bundles)
	return port.Ports{
		Tree:      tree,
		Owned:     tree,
		Writer:    writer,
		Lock:      lock,
		Journal:   hostfs.Journal{Tree: tree, Writer: writer, Clock: clock.Clock{}, Dir: cfg.StateDir, Mounts: cfg.Mounts, Age: age, Lock: lock, Roots: judged},
		Sandbox:   sandbox.New(sandbox.Config{Dir: cfg.CacheDir, Mounts: cfg.Mounts}),
		Clock:     clock.Clock{},
		Schema:    jsonschema.Embedded(a, formatSchemas, contractSchemas),
		Assets:    a,
		Terminal:  out,
		FileNames: tree,
		Trust:     trust,
		Bundles:   bundles,
		Plugins:   plugins,
	}
}

// pluginsOf is the plugin host over bundles, made with
// the engine's codec, features and judgement of a manifest, and the
// approvals of trust. world hands it host/fs's Bundles,
// the same the core's Bundles port reads, so plugin list and an Open read
// one bundle the same way. The host makes its runtime at the first Open, so
// a run that opens no plugin holds none, and the process's end lets the
// runtime go.
func pluginsOf(names port.FileNames, trust port.Trust, bundles port.Bundles) port.Plugins {
	host, err := hostplugin.New(hostplugin.Config{
		Bundles:  bundles,
		Trust:    trust,
		Judge:    plugins.Judge,
		Codec:    wire.New(names),
		Features: wire.Features(),
		Reserved: tool.Reserved(),
	})
	if err != nil {
		panic(err) // New refuses only a nil Judge or Codec, and both are set above
	}
	return host
}

// carriedRoots answers the folders whose files another person may have
// written, which the state folder must lie outside: the working folder, the repository around it,
// found as HoldsPin finds it, and gragas's cache folder, where every rulebook
// a run fetches is kept (releases/, config.CacheKinds), so a rulebook's
// author cannot plant a run there. A working folder with no .git above it
// has no repository root. A cache folder that is not there yet is a root by
// its absolute name, which carried compares as it will resolve once made, so
// the first run that makes it never finds its state folder made there. The Lock and the Journal add the root each call
// names. A clone outside the cache that the run neither works in nor writes
// is not among them: the wiring knows no such folder.
func carriedRoots(tree *hostfs.Tree, cache string) func() ([]string, error) {
	return func() ([]string, error) {
		here, err := tree.Open("")
		if err != nil {
			return nil, fmt.Errorf("the working folder could not be opened: %w", err)
		}
		roots := []string{string(here)}
		top, err := tree.Locate(string(here), ".git")
		switch {
		case err == nil:
			roots = append(roots, string(top))
		case !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
		if cache == "" {
			return roots, nil
		}
		at, err := tree.Open(cache)
		switch {
		case err == nil:
			roots = append(roots, string(at))
		case errors.Is(err, fs.ErrNotExist):
			// A cache no run has made yet is held apart by its name, so a
			// state folder in its place is not made there first.
			abs, err := filepath.Abs(cache)
			if err != nil {
				return nil, fmt.Errorf("the cache folder could not be found: %w", err)
			}
			roots = append(roots, abs)
		default:
			return nil, fmt.Errorf("the cache folder could not be opened: %w", err)
		}
		return roots, nil
	}
}

// stateRoots is roots for the Journal and the locks, or nil, which judges
// nothing, when the state folder is the account's own (config.DefaultState,
// the home as the system records it): no clone and no fetched rulebook
// chooses that folder, so a working folder of the home, or a home that is a
// repository, refuses no run. Only a state folder a variable
// moved is held against the folders a run works in. The approvals keep
// roots whatever the folder.
func stateRoots(cfg config.Config, home func() (string, error), roots func() ([]string, error)) func() ([]string, error) {
	if cfg.DefaultState(home) {
		return nil
	}
	return roots
}

// tools is the Steps adapter: the settings' tool-paths first in its locate
// order and the tools file's tools last, and PATH and LANG the whole of a
// run's environment. A plugin's step finds its grant's program as a tool is
// found. It reads no adapter table. Its fix lines
// to paste are written for d, the run's dialect, and with the zero Dialect
// are said in words.
func tools(cfg config.Config, d shell.Dialect) *tool.Tool {
	return tool.New(tool.Config{Paths: cfg.Settings.ToolPaths, Machine: cfg.Machine.Tools,
		Path: cfg.ChildPath, Lang: cfg.ChildLang, Mounts: cfg.Mounts, Dialect: d, Cache: config.ToolsDir(cfg.CacheDir)})
}

// terminals are standard output, where a person's answer goes, drawn as the
// terminal it is (its measured width, COLUMNS over it, and COLUMNS alone off
// a terminal) and the environment and the flags say, and standard error, where
// the diagnostics go, which are plain lines and never a screen. A menu is
// drawn under the process's environment, which main alone reads for it.
func terminals(cfg config.Config, args []string) (out, diag *term.Terminal) {
	tty, width := term.Measure(os.Stdout)
	l := cfg.Look(tty, width, root.Flags(args))
	return term.New(term.Options{Out: os.Stdout, In: os.Stdin, Err: os.Stderr, Env: os.Environ(),
			Columns: l.Columns, PipeColumns: cfg.Columns, ColorBits: l.ColorBits, Accessible: l.Accessible}),
		term.New(term.Options{Out: os.Stderr})
}

// endings is whether the table of endings the kit is given is whole, as
// go-run/exit checks it. Its kinds are internal/exit's and never change at
// run time, so this fails only on a fault there, which its test finds first;
// a binary built with one says so and stops before anything runs, on failed,
// rather than panicking in an init or ending later on a kind it lacks.
func endings() error {
	if _, err := exit.NewTable(); err != nil {
		return fmt.Errorf("gragas: the table of endings is not whole, so nothing ran: %w", err)
	}
	return nil
}

func main() {
	if err := endings(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exit.Failed)
	}
	// Ctrl+C and SIGTERM are the kit's: the tree runs under its Execute, which
	// cancels the run's context on either, on every system.
	ctx := context.Background()
	cfg, problem := config.Load(os.Getenv, root.ConfigFile(ctx, os.Args[1:]), hosttrust.ReadOwned)
	out, diag := terminals(cfg, os.Args[1:])
	w := world(out, cfg)
	d, noQuoting := dialect(cfg)
	steps := d
	if noQuoting != nil {
		steps = 0
	}
	w.Steps = tools(cfg, steps)
	code := root.Execute(ctx, root.Process{
		Args:        os.Args[1:],
		Stdout:      os.Stdout,
		Stamped:     version,
		Commit:      commit,
		Config:      cfg,
		Problem:     problem,
		Ports:       ports(cfg),
		World:       w,
		Contract:    jsonschema.Embedded(w.Assets, contractSchemas),
		Readers:     coreapi.LoadReaders(w.Assets),
		Tiers:       coreapi.LoadTiers(w.Assets),
		Diagnostics: diag,
		History:     historyFor(cfg),
		Draw:        out.Execute,
		Stopped:     term.Stopped,
		Dialect:     d,
		NoQuoting:   noQuoting,
	})
	os.Exit(code)
}

// dialect is the shell the run's lines to paste are written for: the one
// GRAGAS_SHELL or the shell key names, else, for auto, the shell gragas was
// started from, as host/term finds it. auto is the
// one value that reads the parent processes. When host/term finds no dialect
// for that shell, the error is the run's NoQuoting, whatever wraps it: no
// line to paste is printed and every step is said in words, never quoted for
// a shell taken in its place.
func dialect(cfg config.Config) (shell.Dialect, error) {
	if d, here := cfg.Dialect(); !here {
		return d, nil
	}
	return term.HereDialect()
}

// formatSchemas is the folder of the Assets port that holds the format
// schemas, one folder per format version.
const formatSchemas = "format/schema"

// contractSchemas is the folder of the Assets port that holds the contract's
// schemas, one folder per contract major: `contract validate` checks a
// document against them through the same Schema port, and a bundle from
// outside is read by them.
const contractSchemas = "contract/schema"

// accountHome is the HOME to hand git: itself when git.HomeOK, else none, so a
// relative HOME never makes git read the account's config from the repository.
func accountHome(h string) string {
	if git.HomeOK(h) {
		return h
	}
	return ""
}

// historyFor makes the History port for a run from the git the settings name,
// and only from there: a program found on PATH skips the trust walk (A48,
// cli.config.12). Home is the account's, so git reads its own config.
func historyFor(cfg config.Config) func(run string) (port.History, error) {
	return func(run string) (port.History, error) {
		if cfg.Settings.Programs.Git == "" {
			return nil, root.ErrNoGit
		}
		return git.New(git.Config{Git: cfg.Settings.Programs.Git, Home: accountHome(config.AccountHome(os.Getenv)), Run: run, Mounts: cfg.Mounts})
	}
}
