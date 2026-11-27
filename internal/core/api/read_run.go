package coreapi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// readRun is the journal run of one plugin read of configs. A read starts
// programs, so it has a run of its own with its own id, apart from the plan's:
// the intent is written before the first start and the close after the last,
// so a start is never read as torn by the host, a dry run that is never
// applied leaves no open run, and an apply's own run keeps the intent it writes.
// The import and the audit's plugin read are its two callers.
type readRun struct {
	j    port.Journal
	root model.Root
	id   string
	ask  model.ChangeAsk
	note func(ctx context.Context, n plugin.Noted) error
}

// beginReadRun opens the run of a read of the configs of tool and returns it
// with its intent written. A journal that refuses the intent ends the read.
func beginReadRun(ctx context.Context, j port.Journal, clock port.Clock, ask model.ChangeAsk, tool string) (*readRun, error) {
	r := &readRun{j: j, root: ask.Root, ask: ask,
		id: fmt.Sprintf("read-%d-%s", clock.Now().UnixNano(), digest.Hex([]byte(tool))[:8])}
	detail, err := json.Marshal(map[string]string{"read": tool, "for": ask.Command})
	if err != nil {
		return nil, fmt.Errorf("the read of %s could not be journalled: %w", tool, err)
	}
	if err := r.j.Append(ctx, ask.Root, model.Record{Run: r.id, Kind: model.KindIntent, Command: ask.Command, Engine: ask.Engine, Detail: detail}); err != nil {
		return nil, fmt.Errorf("the read of %s could not be journalled: %w", tool, err)
	}
	r.note = StartNote(j, ask.Root, r.id, ask.Command, ask.Engine)
	return r, nil
}

// end closes the run, with the outcome of the read that ended with err, and
// answers err, or the close's own error when the read had none.
func (r *readRun) end(ctx context.Context, err error) error {
	outcome := "done"
	if err != nil {
		outcome = "failed"
	}
	detail, merr := json.Marshal(map[string]string{"outcome": outcome})
	if merr == nil {
		merr = r.j.Append(ctx, r.root, model.Record{Run: r.id, Kind: model.KindClose, Command: r.ask.Command, Engine: r.ask.Engine, Detail: detail})
	}
	if merr != nil && err == nil {
		return fmt.Errorf("the read of the config could not be journalled: %w", merr)
	}
	return err
}
