package journeys

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// TestThePtyLauncherChecksFreshnessBeforeItStarts: the launcher asks whether
// the binary is fresh, and asks before it opens the pty; a check that fails
// stops it before the pty is opened. The plants are recording fakes: one whose
// check passes, to see the order, and one whose check fails, to see that
// nothing is opened. Risk: a journey that no longer asks leaves a golden held
// against a binary from other code (tests.journeys.3).
func TestThePtyLauncherChecksFreshnessBeforeItStarts(t *testing.T) {
	t.Parallel()
	var calls []string
	l := ptyLauncher{
		fresh: func() error { calls = append(calls, "fresh"); return nil },
		open: func(context.Context, *exec.Cmd, int, int) (*uitest.Pty, error) {
			calls = append(calls, "open")
			return nil, nil
		},
	}
	if _, err := l.launch(t.Context(), ptyJourney{cols: 80}, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "fresh,open" {
		t.Errorf("the launcher called %q, want fresh then open", got)
	}

	calls = nil
	stale := errors.New("a journey on a stale binary")
	l.fresh = func() error { calls = append(calls, "fresh"); return stale }
	if _, err := l.launch(t.Context(), ptyJourney{cols: 80}, nil); !errors.Is(err, stale) {
		t.Errorf("a failed check was answered %v, want it returned", err)
	}
	if got := strings.Join(calls, ","); got != "fresh" {
		t.Errorf("after a failed check the launcher called %q, want fresh alone", got)
	}
}

// TestEveryJourneyLauncherIsWiredToTheFreshnessCheck: the launcher the journeys
// use fails, before it opens a pty, when the binary's record is another tree's.
// The plant is a copy of the binary whose record has one digit changed.
func TestEveryJourneyLauncherIsWiredToTheFreshnessCheck(t *testing.T) {
	t.Parallel()
	bin := plantStale(t)
	p, err := newPtyLauncherFor(bin).launch(t.Context(), ptyJourney{cols: 80}, commandOf(t, bin, t.TempDir(), nil, "--version"))
	if err == nil || !strings.Contains(err.Error(), "stale binary") {
		t.Errorf("the journeys' launcher on a stale binary: %v, want a stale-binary failure", err)
	}
	if p != nil {
		p.Close()
		t.Error("a pty was opened for a stale binary")
	}
}
