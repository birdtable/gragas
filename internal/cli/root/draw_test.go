package root

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestWithNoDrawTheTreeRunsOnCobraAloneAndTellsOnErrorOnce: the subset build
// and the tests hand the root no Draw; an error the tree returns reaches
// onError once, and cobra prints nothing of its own (A70).
func TestWithNoDrawTheTreeRunsOnCobraAloneAndTellsOnErrorOnce(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	tree := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return errors.New("no") }}
	tree.SetOut(&out)
	tree.SetErr(&out)
	told := 0
	err := draw(nil)(context.Background(), tree, func(io.Writer, error) { told++ })
	if err == nil || told != 1 || out.Len() != 0 {
		t.Errorf("err = %v, told %d times, cobra wrote %q; want the error, told once, nothing written", err, told, out.String())
	}
}

// TestADrawMainFillsIsTheOneThatRuns: a Draw handed to the root is used, not
// the plain run.
func TestADrawMainFillsIsTheOneThatRuns(t *testing.T) {
	t.Parallel()
	want := errors.New("drawn")
	got := draw(func(context.Context, *cobra.Command, func(io.Writer, error)) error { return want })(context.Background(), &cobra.Command{Use: "x"}, nil)
	if !errors.Is(got, want) {
		t.Errorf("err = %v; want the one the given Draw returned", got)
	}
}

// TestTheStopLineIsSaidOnlyWhenStoppedSaysASignalEndedTheRun: the root's
// first hook says it heard a signal when, and only when, the Stopped main
// fills from host/term says a signal ended the command's context; a nil
// Stopped is no signal. Each run's Draw ends that context with
// a planted cause, and Stopped is asked of that context, so a Stopped asked of
// another one finds no cause and the line is not said. Risk: a run that
// ends another way said to have stopped on a signal, or a signal's stop
// never said.
func TestTheStopLineIsSaidOnlyWhenStoppedSaysASignalEndedTheRun(t *testing.T) {
	t.Parallel()
	cause := errors.New("planted signal received")
	line := fmt.Sprintf(stopHeard, cause)
	for _, tc := range []struct {
		name    string
		stopped func(context.Context) bool
		said    bool
	}{
		{"Stopped says the planted cause is a signal's", func(ctx context.Context) bool { return errors.Is(context.Cause(ctx), cause) }, true},
		{"Stopped says no signal", func(context.Context) bool { return false }, false},
		{"no Stopped", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			planted := func(ctx context.Context, tree *cobra.Command, onError func(io.Writer, error)) error {
				c, cancel := context.WithCancelCause(ctx)
				cancel(cause)
				return draw(nil)(c, tree, onError)
			}
			_, said := executeSaid(t.Context(), t, Process{Args: []string{"version"}, Draw: planted, Stopped: tc.stopped})
			if got := strings.Contains(said, line); got != tc.said {
				t.Errorf("said the stop line: %v, want %v; the run said\n%s", got, tc.said, said)
			}
		})
	}
}
