package root

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// PluginAdd plans the add of the bundle in dir to the rulebook, and with
// --apply writes its files and then the lock through the one write path. The
// words live in internal/cli/cmd/plugin.
func (k kitSession) PluginAdd(ctx context.Context, cmd kit.Cmd, dir string) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the plugin add command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	return s.ruleChange(ctx, c, []string{dir}, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.PluginAddFrom(ctx, w, dir, ask)
	})
}
