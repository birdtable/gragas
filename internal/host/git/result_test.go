package git

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"
)

// What it costs when this fails: a run the caller cancelled is reported as
// one git itself ended (a deadline, a cap, a tree escaped), and the caller
// cannot tell its own cancel from one of git's own failures.
func TestResultOnACancelledContextAnswersTheContextsErrorBare(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(proc.Ended{Why: proc.WhyCancelled}, context.Cause(ctx))
	out, got := result(ctx, time.Second, proc.Result{Code: -1}, err)
	if out != nil {
		t.Errorf("result on a cancelled context: output %q, want nil", out)
	}
	if !errors.Is(got, context.Canceled) || errors.Is(got, proc.Ended{Why: proc.WhyCancelled}) {
		t.Errorf("result on a cancelled context: %v, want the bare context error, not git's own Ended wrapping it", got)
	}
}

// What it costs when this fails: a process a tree's kill did not reach
// still holding the run's output is read as the run's own exit or another
// ending, and errEscaped, which says a process outside the tree held it,
// never reaches the caller.
func TestResultOnAnEscapedProcessNamesWhatHeldTheOutput(t *testing.T) {
	t.Parallel()
	werr := errors.New("a sentinel the stand-in's run ended with")
	err := errors.Join(proc.Ended{Why: proc.WhyEscaped}, werr)
	out, got := result(context.Background(), time.Second, proc.Result{Code: -1}, err)
	if out != nil {
		t.Errorf("result on an escaped process: output %q, want nil", out)
	}
	if !errors.Is(got, errEscaped) || !errors.Is(got, werr) {
		t.Errorf("result on an escaped process: %v, want errEscaped wrapping %v", got, werr)
	}
}
