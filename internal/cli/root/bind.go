package root

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The bind family's three plans reach the core through the one form every rule
// change ends in (ruleChange). The words live in internal/cli/cmd/bind, rebind
// and unbind.

// Bind plans the addition of a binding to the rule the request names.
func (k kitSession) Bind(ctx context.Context, cmd kit.Cmd, req model.BindRule) int {
	return bound(ctx, k, cmd, req.ID, req, coreapi.Bind)
}

// Rebind plans the change of a binding of the rule the request names.
func (k kitSession) Rebind(ctx context.Context, cmd kit.Cmd, req model.RebindRule) int {
	return bound(ctx, k, cmd, req.ID, req, coreapi.Rebind)
}

// Unbind plans the removal of a binding of the rule the request names.
func (k kitSession) Unbind(ctx context.Context, cmd kit.Cmd, req model.UnbindRule) int {
	return bound(ctx, k, cmd, req.ID, req, coreapi.Unbind)
}

// bound plans req of the rule id with plan, the core's operation of the word.
func bound[R any](ctx context.Context, k kitSession, cmd kit.Cmd, id string, req R, plan func(context.Context, port.Ports, coreapi.OverlayAsk, R) (*coreapi.Change, *model.Refusal, error)) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the bind command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	return s.ruleChange(ctx, c, []string{id}, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		// a parent's rule is read from its bundle in the releases cache (A50)
		ask.Cache = s.p.Config.CacheDir
		return plan(ctx, w, coreapi.OverlayAsk{ChangeAsk: ask, Contracts: s.p.Contract}, req)
	})
}
