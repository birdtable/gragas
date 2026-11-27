package root

import (
	"context"
	"errors"
	"fmt"
	"io"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/port"
)

// hookInput reads the hook's input from t to its end, a line at a time, as
// the agent's hook gives it on standard input: nothing is asked
// (cli.environment.5). More than coreapi.MaxHookInput bytes is an error, and
// nothing past it is kept. An input refused, for its length or a read that
// failed, is drained first (drain), so the host's write of it ends before
// the guard refuses: the block does not rest on how a host reads exit 2
// after its own write failed.
func hookInput(ctx context.Context, t port.Terminal) ([]byte, error) {
	in, err := readHook(ctx, t)
	if err != nil && ctx.Err() == nil {
		if derr := drain(ctx, t); derr != nil {
			err = fmt.Errorf("%w; the rest of it was not read away: %w", err, derr)
		}
	}
	return in, err
}

// readHook is hookInput's read, with no drain.
func readHook(ctx context.Context, t port.Terminal) ([]byte, error) {
	var in []byte
	for first := true; ; first = false {
		line, err := t.Line(ctx)
		if errors.Is(err, io.EOF) {
			return in, nil
		}
		if err != nil {
			return nil, fmt.Errorf("standard input could not be read: %w", err)
		}
		if !first {
			in = append(in, '\n')
		}
		if len(in)+len(line) > coreapi.MaxHookInput {
			return nil, fmt.Errorf("standard input is longer than the %d bytes the guard reads", coreapi.MaxHookInput)
		}
		in = append(in, line...)
	}
}

// drainer is a terminal that reads the rest of its input away, keeping none
// of it: the host's terminal is.
type drainer interface {
	Drain(ctx context.Context) error
}

// drain reads the rest of t's input away, and waits for it no longer than
// coreapi.HookDrain: past it the answer is the deadline's error. A terminal
// that cannot drain is left as it is.
func drain(ctx context.Context, t port.Terminal) error {
	d, ok := t.(drainer)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, coreapi.HookDrain)
	defer cancel()
	done := make(chan error, 1) // the read's answer, sent once it ends, waited for or not
	// A bound, not a speed-up: the read of a blocking pipe, as a host hands
	// standard input, cannot be woken, so the wait on the deadline is beside
	// it. The read ends with the input or with the process, which ends once
	// the refusal is written.
	// measured: guard edit's wait on a hook pipe whose writer never closes, from no end to 10 s (coreapi.HookDrain), TestARefusedHookInputIsDrainedForNoLongerThanHookDrain
	go func() { done <- d.Drain(ctx) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
