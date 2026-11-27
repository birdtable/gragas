package coreapi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// StartNote is the Note a plugin's steps are journalled by: each program start
// is one model.KindStart record under run, its Detail the plugin.Noted, written
// by the Journal port before the start is made. When the journal refuses the
// record the Note answers that error and the step does not start, so no
// program runs that the journal has not heard of.
// A start is no effect on a file, so a rollback reads nothing from it.
//
// The run must be begun before its first start: its KindIntent is written by
// the caller (step 8 wires it). On the host's journal a start is an entry line
// only, and an entry line is torn until its run's intent or close begins the
// run, so a start written first is lost, and stays torn, if the process stops
// before the run's first record that begins it.
func StartNote(j port.Journal, root model.Root, run, command, engine string) func(ctx context.Context, n plugin.Noted) error {
	return func(ctx context.Context, n plugin.Noted) error {
		detail, err := json.Marshal(n)
		if err != nil {
			return fmt.Errorf("the start of %s could not be journalled: %w", n.Grant, err)
		}
		rec := model.Record{Run: run, Kind: model.KindStart, Command: command, Engine: engine, Detail: detail}
		if err := j.Append(ctx, root, rec); err != nil {
			return fmt.Errorf("the start of %s could not be journalled: %w", n.Grant, err)
		}
		return nil
	}
}
