package main

import (
	"context"
	"flag"
	"io"
	"path/filepath"
	"time"

	"github.com/birdtable/gragas/internal/cli/root"
	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/assets"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/host/sandbox"
	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/host/toml"
	"github.com/birdtable/gragas/internal/host/tool"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// The run mode (gragas#340): one writing command of the tree, wired as
// cmd/gragas wires it, with the writer and the journal counted, so the
// command is killed in its own code at a numbered boundary and not in a
// script of it. The clock is fixed and History is one fixed account, so two
// runs of one command line from one root end in the same files and the
// journey can hold every end to a golden; it is never a terminal and stamps
// no commit. Those are its differences from cmd/gragas.

const runUsage = `usage: gragas-crash run [-stop n] [-at unix] [-plant intent-late] -- <gragas arguments>

  run the gragas command line in this folder as cmd/gragas runs it, with the
  clock at -at; with -stop n, say "stopped n <stage>" on stderr at boundary
  n and wait to be killed; with no -stop, say each boundary and its stage on
  stderr once the command ends; -plant intent-late is a planted bug: the
  write path journals nothing before it writes, and the run says
  "stopped n planted"
  once its first file is written, and waits to be killed`

// fixedAt is the clock of a run when -at is not given: a moment in 2026.
const fixedAt = 1794132000

// fixedClock is a clock that answers one moment.
type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

// The two folders of the assets the command tree reads, as cmd/gragas names
// them.
const (
	formatSchemas   = "format/schema"
	contractSchemas = "contract/schema"
)

// runTree runs the command line after -- through the command tree with the
// writer and the journal counted; getenv is main's environment.
func runTree(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	errs := &say{w: stderr}
	fl := flag.NewFlagSet("gragas-crash run", flag.ContinueOnError)
	fl.SetOutput(stderr)
	at := fl.Int("stop", -1, "the boundary to stop at")
	when := fl.Int64("at", fixedAt, "the clock, in Unix seconds")
	plant := fl.String("plant", "", "a planted bug: intent-late")
	if err := fl.Parse(args); err != nil || fl.NArg() == 0 || (*plant != "" && *plant != "intent-late") {
		errs.line("%s", runUsage)
		return exit.Usage
	}
	line := fl.Args()
	clk := fixedClock{at: time.Unix(*when, 0).UTC()}
	b := &bounds{stop: *at, out: errs, record: recordPath, open: true, intentLate: *plant == "intent-late"}
	cfg, problem := config.Load(getenv, root.ConfigFile(ctx, line), hosttrust.ReadOwned)
	// Never a terminal, as the journey starts it: the look the environment and
	// the flags say, as cmd/gragas draws a pipe.
	l := cfg.Look(false, 0, root.Flags(line))
	out := term.New(term.Options{Out: stdout, Columns: l.Columns, ColorBits: l.ColorBits, Accessible: l.Accessible})
	w := world(out, cfg, clk, b)
	w.Steps = tool.New(tool.Config{Paths: cfg.Settings.ToolPaths, Machine: cfg.Machine.Tools,
		Path: cfg.ChildPath, Lang: cfg.ChildLang, Mounts: cfg.Mounts, Cache: config.ToolsDir(cfg.CacheDir)})
	code := root.Execute(ctx, root.Process{
		Args:        line,
		Stdout:      stdout,
		Stamped:     "v0.0.0-crash",
		Config:      cfg,
		Problem:     problem,
		Ports:       legacyPorts(cfg, clk),
		World:       w,
		Contract:    jsonschema.Embedded(w.Assets, contractSchemas),
		Readers:     coreapi.LoadReaders(w.Assets),
		Tiers:       coreapi.LoadTiers(w.Assets),
		Diagnostics: term.New(term.Options{Out: stderr}),
		History:     history,
		Draw:        out.Execute,
	})
	for i, s := range b.stages {
		errs.line("point %d %s", i, s)
	}
	if code == exit.Done && errs.err != nil {
		return exit.Failed
	}
	return code
}

// world is cmd/gragas's ports, with the clock fixed and the writer and the
// journal counted by b.
func world(out port.Terminal, cfg config.Config, clk port.Clock, b *bounds) port.Ports {
	a := assets.New()
	tree := &hostfs.Tree{}
	writer, lock := hostfs.Writer{Tree: tree}, &hostfs.Lock{Dir: cfg.StateDir, Mounts: cfg.Mounts, Clock: clk, Tree: tree}
	plugins, trust, bundles := pluginHost(tree, writer, clk, cfg.StateDir, cfg.Mounts)
	return port.Ports{
		Tree:     tree,
		Owned:    tree,
		Writer:   countingWriter{Writer: writer, b: b},
		Lock:     lock,
		Journal:  countingJournal{Journal: hostfs.Journal{Tree: tree, Writer: writer, Clock: clk, Dir: cfg.StateDir, Mounts: cfg.Mounts, Age: keepAge(cfg), Lock: lock}, b: b},
		Sandbox:  sandbox.New(sandbox.Config{Dir: cfg.CacheDir, Mounts: cfg.Mounts}),
		Clock:    clk,
		Schema:   jsonschema.Embedded(a, formatSchemas, contractSchemas),
		Assets:   a,
		Plugins:  plugins,
		Trust:    trust,
		Bundles:  bundles,
		Terminal: out, FileNames: tree,
	}
}

// legacyPorts are the old commands' ports, as cmd/gragas fills them; the
// writes through them are not counted, and the crash list names each
// command that writes only through them as outside the write path.
func legacyPorts(cfg config.Config, clk port.Clock) legacy.Ports {
	return legacy.Ports{
		Files:   hostfs.Files{},
		Writer:  hostfs.LegacyWriter{},
		Schemas: jsonschema.Schemas{},
		Locks:   hostfs.LegacyLocks{Mounts: cfg.Mounts, Clock: clk},
		Runs:    hostfs.Runs{},
		TOML:    toml.Reader{},
	}
}

// who is the History port of a run: one fixed account, so the history line
// a command records is the same on every run and the crash journey can hold
// every end to a golden, and no git is started, which the trust walk refuses
// inside the PID namespace the crash journeys run in (its / belongs to the
// overflow account; go-files' Who.Overflow, which trusts it there, is a test
// binary's alone and refused in this program, gragas#1607). A plan records its
// command's name as who (gragas#1977), so no crash command asks; any other
// question reaches the nil port and panics, which fails its run loudly.
type who struct{ port.History }

func (who) Who(context.Context, model.Root) (model.Identity, error) {
	return model.Identity{Name: "Test Person", Email: "test@example.test"}, nil
}

// history hands every run the fixed account.
func history(string) (port.History, error) { return who{}, nil }

// isRecord is whether path, from the root, is a record of the yes: the
// history file of a rulebook at the root or in a folder of it. The path is
// the writer's, a path of this system, so filepath names its last element.
func isRecord(p string) bool { return filepath.Base(p) == recordPath }

// keepAge is the journal's keep age from the settings, as cmd/gragas reads
// it: zero, the journal's own, for a keep that does not parse.
func keepAge(cfg config.Config) time.Duration {
	if secs, err := cfg.Settings.KeepSeconds(); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}
