package root

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Undo plans the entry that takes back the last change of the history, through
// the one form every rule change ends in (ruleChange). The word lives in
// internal/cli/cmd/undo. The apply line names the entry (--entry), and an
// apply that does not, or that names another than undo would take now, is
// refused by the plan.
func (k kitSession) Undo(ctx context.Context, cmd kit.Cmd) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the undo command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	entry, err := c.Flags().GetString("entry")
	if err != nil {
		return s.die("undo has no --entry flag: %v", err)
	}
	// a replay of guard staged keeps the entry as typed: the line it replays
	// stores the command as it was typed, and the plan digest covers it
	if !s.applied && s.capture == nil {
		root, found := s.folder()
		if !found {
			return exit.Failed
		}
		// the dry run's closing line carries the digest of the entry it showed, whatever
		// was typed: a number is input only, never printed back (the entry typed still
		// goes to Undo below, so one that is not the target is refused first)
		if ref, has := coreapi.UndoEntry(s.p.World, root, s.p.Config.NamesFile); has {
			if err := c.Flags().Set("entry", ref); err != nil {
				return s.die("undo could not name the entry: %v", err)
			}
		}
	}
	return s.ruleChange(ctx, c, nil, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.Undo(ctx, w, ask, entry, s.applied)
	})
}
