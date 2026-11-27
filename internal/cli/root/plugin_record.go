package root

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// PluginRecord plans, and with --apply runs, the recording of the plugin in
// the author's folder dir. It never asks at a terminal: the plan is not what
// --apply runs (a tool), so --apply alone says yes. The words live in
// internal/cli/cmd/plugin.
func (k kitSession) PluginRecord(ctx context.Context, cmd kit.Cmd, dir string) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the plugin record command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	f, err := recordFlagsOf(c)
	if err != nil {
		return s.die("the plugin record command's flags could not be read: %v", err)
	}
	apply := applyGiven(c)
	return s.changeWith(ctx, c, []string{dir}, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.PluginRecord(ctx, w, coreapi.PluginRecordAsk{Ask: ask, Dir: dir, Op: f.op, Config: f.config, Fixtures: f.fixtures, Case: f.caseName, Apply: apply})
	}, false)
}

// recordFlags are the flags of plugin record that name what is recorded.
type recordFlags struct {
	op, caseName     string
	config, fixtures []string
}

func recordFlagsOf(c *cobra.Command) (f recordFlags, err error) {
	if f.op, err = c.Flags().GetString("op"); err != nil {
		return f, err
	}
	if f.config, err = c.Flags().GetStringArray("config"); err != nil {
		return f, err
	}
	if f.fixtures, err = c.Flags().GetStringArray("fixture"); err != nil {
		return f, err
	}
	f.caseName, err = c.Flags().GetString("case")
	return f, err
}
