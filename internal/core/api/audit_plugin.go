package coreapi

import (
	"context"
	"fmt"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// AuditPlugins is the ports the check reads a tool's config through the tool's
// installed plugin with. A zero value, or one with no
// Root (a folder that could not be opened), reads no tool: every tool's rules
// are not checked, with the reason ErrNoPlugin gives. Plugins is never tested
// for nil (arch.ports.5): the entry points set it from host, always.
//
// Where a plugin of the tool is installed the check asks it; where none is,
// the rules of the tool are not checked, and the reason names the tool and
// how to install a plugin of it (release 1 ships no adapter that answers in
// its place). Each plugin read is a read run in the
// journal, intent then starts then close, the same run the import's read has.
type AuditPlugins struct {
	Plugins port.Plugins
	Steps   port.Steps
	Sandbox port.Sandbox
	Journal port.Journal
	Clock   port.Clock
	// Tree and Writer mirror the files a read step matches into the scratch
	// folder (gragas#1173); with neither, a read mirrors nothing.
	Tree   port.Tree
	Writer port.Writer
	// Root is the repository the plugins are installed under.
	Root model.Root
	// Command and Engine name the run in the journal.
	Command, Engine string
}

// entries is the check's audit.Entries: a tool's installed plugin where there
// is one, and the refusal ErrNoPlugin gives where there is none. tested is the
// tested set of the book the check reads, which picks the plugin of a tool as it
// picks it for the rank and the preset; nil where there is none.
func (a AuditPlugins) entries(tested *model.Tested) audit.Entries {
	if a.Root == "" {
		return func(tool string) (audit.EntriesCall, string, string) {
			return nil, "", noPlugin(tool).Error()
		}
	}
	return func(tool string) (audit.EntriesCall, string, string) {
		return func(ctx context.Context, in plugin.PlanRequest) (plugin.EntriesResponse, error) {
			p, found, err := pluginOf(ctx, a.Plugins, a.Root, tool, tested)
			if err != nil {
				return plugin.EntriesResponse{}, err
			}
			if !found {
				return plugin.EntriesResponse{}, noPlugin(tool)
			}
			return a.read(ctx, p, tool, in)
		}, "", ""
	}
}

// read is the plugin's entries over the request's config, in a read run of its
// own whose close is written whatever ends the read. The plugin is closed after.
func (a AuditPlugins) read(ctx context.Context, p port.Plugin, tool string, in plugin.PlanRequest) (out plugin.EntriesResponse, err error) {
	defer func() {
		if cerr := p.Close(); cerr != nil && err == nil {
			out, err = plugin.EntriesResponse{}, fmt.Errorf("the plugin of %s could not be closed: %w", tool, cerr)
		}
	}()
	name, data := "", []byte(nil)
	if len(in.Config) > 0 {
		c := in.Config[0]
		if c.Data == nil || c.Data.Text == nil {
			return plugin.EntriesResponse{}, fmt.Errorf("%s: the config holds no text", c.Path)
		}
		name, data = c.Path, []byte(*c.Data.Text)
	}
	run, err := beginReadRun(ctx, a.Journal, a.Clock, model.ChangeAsk{Root: a.Root, Command: a.Command, Engine: a.Engine}, tool)
	if err != nil {
		return plugin.EntriesResponse{}, err
	}
	defer func() {
		if cerr := run.end(context.WithoutCancel(ctx), err); cerr != nil {
			out, err = plugin.EntriesResponse{}, cerr
		}
	}()
	return pluginRead{sandbox: a.Sandbox, steps: a.Steps, note: run.note, root: a.Root, tree: a.Tree, writer: a.Writer}.entries(ctx, p, tool, in.Target, name, data)
}
