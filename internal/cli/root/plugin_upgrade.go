package root

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// PluginUpgrade plans the upgrade of the installed plugin ref to the bundle
// the folder source holds, and with --apply writes it through the one write
// path. The words live in internal/cli/cmd/plugin.
func (k kitSession) PluginUpgrade(ctx context.Context, cmd kit.Cmd, source, ref string) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the plugin upgrade command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	return s.ruleChange(ctx, c, []string{source, ref}, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.PluginUpgradeFrom(ctx, w, source, ref, ask)
	})
}
