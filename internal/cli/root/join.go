package root

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/cli/legacy"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// builtCommand is a command package below internal/cli/cmd as the generated
// join (join_gen.go) names it: the row it declares and the cobra command it
// makes over the session of a run.
type builtCommand = kit.Package[*cobra.Command]

// kitSession is the session of a run as the command packages see it.
type kitSession struct{ s *session }

var _ kit.Session = kitSession{}

func (k kitSession) Say(line string)     { k.s.say(line) }
func (k kitSession) JSON() bool          { return k.s.json }
func (k kitSession) Document(v any)      { k.s.document(v) }
func (k kitSession) SARIF() bool         { return k.s.sarif }
func (k kitSession) Encoded(doc []byte)  { k.s.encoded(doc) }
func (k kitSession) Exit(code int) error { return exitErr(code) }

func (k kitSession) End(e kit.Ending) int {
	return k.s.end(ending{word: e.Word, what: e.What, next: e.Next}, e.Lead...)
}

func (k kitSession) Line(argv []string) (string, error) { return k.s.line(argv) }

func (k kitSession) Build() kit.Build {
	b := k.s.build
	return kit.Build{Version: b.Version, Commit: b.Commit, Go: b.Go, Line: b.String()}
}

// Validate opens the working folder and asks the core to validate the
// rulebook in it, with the names file the settings name.
func (k kitSession) Validate(ctx context.Context) (model.Validation, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return model.Validation{}, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	return coreapi.Validate(ctx, k.s.p.World, root, k.s.p.Config.NamesFile)
}

// CheckRulebook opens the working folder and asks the core to check the
// rulebook in it: validate's findings, then the gate over HEAD's history
// against the base ref the settings name, each history line replayed
// through the command tree as guard staged replays it.
func (k kitSession) CheckRulebook(ctx context.Context) (model.RulebookCheck, *model.Refusal, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return model.RulebookCheck{}, nil, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	ask := coreapi.CheckAsk{Root: root, BaseRef: k.s.p.Config.BaseRef, NamesFile: k.s.p.Config.NamesFile, Replay: k.s.replayer(),
		OpenHistory: func() (port.History, error) { return k.s.p.History("check") }}
	return coreapi.CheckRulebook(ctx, k.s.p.World, ask)
}

// HoldsRulebook is whether the working folder itself holds a rulebook.yaml:
// one stat of the folder, nothing listed below it.
func (k kitSession) HoldsRulebook(context.Context) (bool, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return false, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	_, err = k.s.p.World.Tree.Stat(root, "rulebook.yaml")
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// HoldsPin is whether a project's pin lies in the working folder or above it,
// up to the repository's root: a pin above the folder that holds .git is
// another project's, and is not looked for. A working folder with no .git
// above it looks up to the top.
func (k kitSession) HoldsPin(context.Context) (bool, error) {
	tree := k.s.p.World.Tree
	here, err := tree.Open("")
	if err != nil {
		return false, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	pin, err := tree.Locate(string(here), pinPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	top, err := tree.Locate(string(here), ".git")
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	return pin == top || strings.HasPrefix(string(pin), string(top)+"/"), nil
}

// pinPath is the project's pin, relative to the folder that holds .gragas/.
const pinPath = ".gragas/pin.yaml"

// CheckProject is check below a pin: the project is found from the working
// folder, and held to the rulebook its pin names (coreapi.CheckProject). A pin
// or a book not ready to read is the error; a tool that could not be vouched
// for is the refusal.
func (k kitSession) CheckProject(ctx context.Context) (model.ProjectCheck, *model.Refusal, error) {
	here, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return model.ProjectCheck{}, nil, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	got, err := coreapi.CheckProjectAt(ctx, k.s.p.World, string(here), pinPath, k.projectAsk(string(here)))
	if ref, ok := coreapi.AsRefusal(err); ok {
		return got, ref, nil
	}
	return got, nil, err
}

// projectAsk is what the project check is asked with: the readers and the tiers,
// the plugins of the world, for the folder here.
func (k kitSession) projectAsk(here string) coreapi.ProjectAsk {
	ask := coreapi.ProjectAsk{Readers: k.s.p.Readers, Tiers: k.s.p.Tiers, TOML: k.s.p.Ports.TOML.Read, Today: k.s.today, NamesFile: k.s.p.Config.NamesFile,
		BaseRef: k.s.p.Config.BaseRef, OpenHistory: func() (port.History, error) { return k.s.p.History("check") }}
	ask.Plugins = k.s.auditPlugins(here)
	return ask
}

// OldCheck is the old check over the host and the world it reads (cmd/check.Old).
func (k kitSession) OldCheck() *cobra.Command { return legacy.Check(host(k), k.s.legacyDeps()) }

// legacyDeps are what the old commands read from the world.
func (s *session) legacyDeps() legacy.Deps {
	return legacy.Deps{Ports: s.p.Ports, NamesFile: s.p.Config.NamesFile, Today: s.today, Readers: s.p.Readers, Tiers: s.p.Tiers, Audit: s.auditPlugins}
}

// auditPlugins is the ports the check reads a tool's config through its
// installed plugin with, for the project at dir: the world's, with the run's
// engine and the command it is journalled for. A folder that cannot be opened
// has none, and the check reads by the adapter alone.
func (s *session) auditPlugins(dir string) coreapi.AuditPlugins {
	w := s.p.World
	root, err := w.Tree.Open(dir)
	if err != nil {
		return coreapi.AuditPlugins{}
	}
	return coreapi.AuditPlugins{Plugins: w.Plugins, Steps: w.Steps, Sandbox: w.Sandbox, Journal: w.Journal, Clock: w.Clock, Tree: w.Tree, Writer: w.Writer, Root: root, Command: "check", Engine: s.build.String()}
}

// today is the date by the world's clock, YYYY-MM-DD in UTC, asked when a
// command runs: the root is built before every run reads its settings.
func (s *session) today() string {
	return s.p.World.Clock.Now().UTC().Format(time.DateOnly)
}

// listed opens the working folder and asks the core operation list about it,
// the one way the listings reach the core.
func listed[T any](ctx context.Context, k kitSession, list func(context.Context, port.Ports, model.Root) (T, error)) (T, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		var none T
		return none, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	return list(ctx, k.s.p.World, root)
}

// Migrate plans the move of the working folder to the format and runs it
// through the form of s.plan: the plan reads the files, and the screen and the
// JSON document carry its count. A form the move refuses ends usage.
func (k kitSession) Migrate(ctx context.Context, cmd kit.Cmd, form model.InitForm) int {
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return k.s.die("a plan was given a command it cannot run")
	}
	record, line, err := k.s.planLines(commandArgv(c, "gragas migrate", nil, nil))
	if err != nil {
		// no line to paste, not even a stand-in: the reason names the value
		return k.s.end(ending{word: model.Usage, what: err.Error(), next: helpLine(c)})
	}
	change, err := k.migration(form, coreapi.Lines{Command: record, Applies: line, Who: c.CommandPath()})
	if err != nil {
		return k.s.end(ending{word: model.Usage, what: err.Error(), next: "gragas migrate --name my-rules --owner me"})
	}
	return k.s.plan(ctx, planRun{cmd: c, change: change, next: line, ask: k.s.canAsk()})
}

// migration makes the migrate change over the working folder.
func (k kitSession) migration(form model.InitForm, lines coreapi.Lines) (changer, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return nil, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	run := k.s.rec.id()
	if run == "" {
		run = dryRun
	}
	world := k.s.p.World
	h, err := k.s.p.History(run)
	if err != nil {
		return nil, fmt.Errorf("migrate needs git to say who migrated the rulebook: set programs.git in the settings file to the absolute path of git (%w)", err)
	}
	world.History = h
	ch, err := coreapi.Migrate(world, root, form, coreapi.MigrateRun{Lines: lines, Run: run, Engine: k.s.build.Version}, func(m coreapi.Migration) { k.s.data = m })
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// Tools lists the tools the rulebook of the working folder binds rules to.
func (k kitSession) Tools(ctx context.Context) (model.ToolsListing, error) {
	return listed(ctx, k, coreapi.ListTools)
}

// Attention lists what needs the maintainer of the working folder's rulebook.
func (k kitSession) Attention(ctx context.Context) (model.AttentionListing, error) {
	return listed(ctx, k, coreapi.Attention)
}

// Conflicts lists the entry conflicts of the rulebook of the working folder.
func (k kitSession) Conflicts(ctx context.Context) (model.ConflictsListing, error) {
	return listed(ctx, k, coreapi.ListConflicts)
}

// Coverage lists the coverage of the rulebook of the working folder.
func (k kitSession) Coverage(ctx context.Context) (model.Coverage, error) {
	return listed(ctx, k, coreapi.ListCoverage)
}

// Aside writes line on standard error, under --json too: the hook hands it to
// the agent whose edit is refused, beside the answer on stdout.
func (k kitSession) Aside(line string) { k.s.diag("", line) }

// GuardEdit reads the proposed edit from standard input and asks the core to
// judge it in the working folder. A refusal is journalled under the run's
// record, or a run of its own when this run writes none.
func (k kitSession) GuardEdit(ctx context.Context) (model.Guard, *model.Refusal, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return model.Guard{}, nil, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	in, unread := hookInput(ctx, k.s.p.World.Terminal)
	ask := coreapi.GuardAsk{Root: root, Input: in, Unread: unread, Run: k.s.rec.id(), Engine: k.s.build.String()}
	// From here a crash of the guard has a root, so the root's crash path
	// journals its refusal.
	k.s.crashJournal = func() error {
		return coreapi.JournalGuardCrash(context.WithoutCancel(ctx), k.s.p.World, ask, coreapi.GuardCommand)
	}
	return coreapi.GuardEdit(ctx, k.s.p.World, ask)
}

// ContractValidate checks one document against the contract's schemas.
func (k kitSession) ContractValidate(_ context.Context, file, kind string) (model.ContractCheck, error) {
	return coreapi.ContractValidate(k.s.p.World, filepath.ToSlash(file), kind)
}

// ContractFields lists one entity's fields with their versions.
func (k kitSession) ContractFields(_ context.Context, entity string) (model.ContractFields, error) {
	return coreapi.ContractFields(k.s.p.World, entity)
}

// ContractFixture writes the fixture's documents into out, or lists them.
func (k kitSession) ContractFixture(ctx context.Context, out string, apply bool) (model.ContractFixture, error) {
	k.s.applied = apply
	return coreapi.ContractFixture(ctx, k.s.p.World, filepath.ToSlash(out), apply)
}

// ExplainRule says what the rulebook of the working folder holds of a rule.
func (k kitSession) ExplainRule(ctx context.Context, asked string) (model.Explained, error) {
	return listed(ctx, k, func(ctx context.Context, p port.Ports, r model.Root) (model.Explained, error) {
		return coreapi.ExplainRule(ctx, p, r, asked)
	})
}

// ExplainEntry says which rules of the working folder's rulebook hold an entry.
func (k kitSession) ExplainEntry(ctx context.Context, tool, key string) (model.Explained, error) {
	return listed(ctx, k, func(ctx context.Context, p port.Ports, r model.Root) (model.Explained, error) {
		return coreapi.ExplainEntry(ctx, p, r, tool, key)
	})
}

// Resolve lists the effective rulebook of the working folder's chain.
func (k kitSession) Resolve(ctx context.Context) (model.ResolveReport, error) {
	ask := coreapi.ChainAsk{Cache: k.s.p.Config.CacheDir, Contracts: k.s.p.Contract}
	return listed(ctx, k, func(ctx context.Context, p port.Ports, r model.Root) (model.ResolveReport, error) {
		return coreapi.ResolveChain(ctx, p, r, ask)
	})
}

// PluginTest holds one plugin bundle to checks C1 to C23 of the plugin design.
func (k kitSession) PluginTest(ctx context.Context, dir string) (model.PluginTest, error) {
	return coreapi.PluginTest(ctx, k.s.p.World, filepath.ToSlash(dir))
}

// PluginList lists the plugins the working folder's lock names.
func (k kitSession) PluginList(ctx context.Context) (model.PluginsListing, error) {
	return listed(ctx, k, coreapi.PluginList)
}

// PluginApproval says what an approval of one installed plugin covers.
func (k kitSession) PluginApproval(ctx context.Context, ref string) (model.PluginApproval, error) {
	return listed(ctx, k, withRef(ref, coreapi.PluginApproval))
}

// withRef is f with its plugin reference given, in the shape listed takes.
func withRef[T any](ref string, f func(context.Context, port.Ports, model.Root, string) (T, error)) func(context.Context, port.Ports, model.Root) (T, error) {
	return func(ctx context.Context, p port.Ports, r model.Root) (T, error) {
		return f(ctx, p, r, ref)
	}
}

// PluginApprove records the approval of one installed plugin, for the digest
// shown.
func (k kitSession) PluginApprove(ctx context.Context, ref string, d model.Digest) (model.PluginApproval, error) {
	return listed(ctx, k, func(ctx context.Context, p port.Ports, r model.Root) (model.PluginApproval, error) {
		return coreapi.PluginApprove(ctx, p, r, ref, d)
	})
}
