package contracttest

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// StepsMaker builds the Steps under test and a scope it binds: a manifest
// whose tool is a grant found here at a version the grant serves, an
// absolute clean root, a scratch folder that reached isolated, the floor
// isolated, and note as the scope's Note.
type StepsMaker func(t *testing.T, note func(context.Context, plugin.Noted) error) (port.Steps, plugin.Scope)

// errNote is the failure a note answers where a subtest makes it fail.
var errNote = errors.New("the journal is full")

// notes keeps every note a runner writes: got the steps' and asks the
// version asks', each in order. A step's note answers err, and a version
// ask's askErr.
type notes struct {
	mu          sync.Mutex
	got, asks   []plugin.Noted
	err, askErr error
}

func (n *notes) note(_ context.Context, x plugin.Noted) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if x.VersionAsk {
		n.asks = append(n.asks, x)
		return n.askErr
	}
	n.got = append(n.got, x)
	return n.err
}

// count is how many steps were noted.
func (n *notes) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.got)
}

// asked is the version asks noted.
func (n *notes) asked() []plugin.Noted {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.asks)
}

// Steps runs the Steps suite against the port build answers.
// It states what the port's doc comment promises of Bind and of a bound
// runner before anything starts: a refused scope, a grant missing, a level
// this machine lacks, a stdin past its cap and a note that fails. What a
// started step gives is the adapter's own tests'.
func Steps(t *testing.T, build StepsMaker) {
	t.Helper()
	t.Run("the bound runner knows its tool", func(t *testing.T) {
		t.Parallel()
		x, s := build(t, (&notes{}).note)
		r := bindOrFail(t, x, s)
		if f := r.Tool(); f.Path == "" || f.Version == "" {
			t.Errorf("Tool() = %+v, want the program's path and the version it printed", f)
		}
	})
	t.Run("Bind journals its version ask first", func(t *testing.T) { stepsAskNoted(t, build) })
	t.Run("a scope it cannot run under is refused", func(t *testing.T) { stepsRefusesScope(t, build) })
	t.Run("a manifest with no grant of its tool", func(t *testing.T) {
		t.Parallel()
		x, s := build(t, (&notes{}).note)
		s.Manifest.Tool = "no-such-grant"
		if _, err := x.Bind(t.Context(), s); !errors.Is(err, port.ErrGrant) {
			t.Errorf("Bind = %v, want port.ErrGrant", err)
		}
	})
	t.Run("granted looks at the grants", func(t *testing.T) { stepsGranted(t, build) })
	t.Run("nothing starts before its note, nor when the note fails", func(t *testing.T) { stepsNoteFirst(t, build) })
	t.Run("a context that ended", func(t *testing.T) {
		t.Parallel()
		x, s := build(t, (&notes{}).note)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := x.Bind(ctx, s); !errors.Is(err, context.Canceled) {
			t.Errorf("Bind with an ended context = %v, want context.Canceled", err)
		}
	})
	t.Run("a runner that kept no cache sweeps none", func(t *testing.T) {
		t.Parallel()
		x, _ := build(t, (&notes{}).note)
		if n, err := x.CachesLeft(); n != 0 || err != nil {
			t.Errorf("CachesLeft = %d, %v, want 0 and no error", n, err)
		}
		if n, err := x.SweepCaches(); n != 0 || err != nil {
			t.Errorf("SweepCaches = %d, %v, want 0 and no error", n, err)
		}
	})
}

// bindOrFail is x bound to s, or the test's end.
func bindOrFail(t *testing.T, x port.Steps, s plugin.Scope) port.StepRunner {
	t.Helper()
	r, err := x.Bind(t.Context(), s)
	if err != nil {
		t.Fatalf("Bind = %v", err)
	}
	return r
}

func stepsRefusesScope(t *testing.T, build StepsMaker) {
	t.Helper()
	t.Parallel()
	for name, change := range map[string]func(*plugin.Scope){
		"no note":               func(s *plugin.Scope) { s.Note = nil },
		"a relative root":       func(s *plugin.Scope) { s.Root = "project" },
		"an unclean root":       func(s *plugin.Scope) { s.Root += "/../x" },
		"a relative scratch":    func(s *plugin.Scope) { s.Scratch.Dir = "scratch" },
		"a scratch of no level": func(s *plugin.Scope) { s.Scratch.Level = "" },
		"a floor of no level":   func(s *plugin.Scope) { s.Floor = "loose" },
		"a deadline of 0":       func(s *plugin.Scope) { s.Scratch.Seconds = 0 },
		"a limit below 0":       func(s *plugin.Scope) { s.Scratch.MemoryMiB = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			x, s := build(t, (&notes{}).note)
			change(&s)
			if _, err := x.Bind(t.Context(), s); !errors.Is(err, port.ErrRefused) {
				t.Errorf("Bind = %v, want port.ErrRefused", err)
			}
		})
	}
}

func stepsGranted(t *testing.T, build StepsMaker) {
	t.Helper()
	t.Parallel()
	x, s := build(t, (&notes{}).note)
	r := bindOrFail(t, x, s)
	if err := r.Granted(plugin.Step{Read: &plugin.Read{Glob: "*.json"}}); err != nil {
		t.Errorf("Granted of a read = %v, want nil", err)
	}
	if err := r.Granted(plugin.Step{Run: &plugin.Run{Program: s.Manifest.Tool}}); err != nil {
		t.Errorf("Granted of a run of the tool's grant = %v, want nil", err)
	}
	if err := r.Granted(plugin.Step{Run: &plugin.Run{Program: "no-such-grant"}}); !errors.Is(err, port.ErrGrant) {
		t.Errorf("Granted of a run of no grant = %v, want port.ErrGrant", err)
	}
}

func stepsNoteFirst(t *testing.T, build StepsMaker) {
	t.Helper()
	t.Parallel()
	sandboxed := plugin.LevelSandboxed
	big := strings.Repeat("x", plugin.MaxStdin+1)
	for name, c := range map[string]struct {
		run  func(tool string) plugin.Run
		want error
	}{
		"a run of no grant":        {func(string) plugin.Run { return plugin.Run{Program: "no-such-grant"} }, port.ErrGrant},
		"a level above the folder": {func(p string) plugin.Run { return plugin.Run{Program: p, Level: &sandboxed} }, port.ErrStepNotReady},
		"a stdin past its cap":     {func(p string) plugin.Run { return plugin.Run{Program: p, Stdin: &plugin.Bytes{Text: &big}} }, port.ErrRefused},
		"neither a run nor a read": {nil, port.ErrRefused},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n := &notes{}
			x, s := build(t, n.note)
			r := bindOrFail(t, x, s)
			step := plugin.Step{}
			if c.run != nil {
				run := c.run(s.Manifest.Tool)
				step.Run = &run
			}
			if _, err := r.Run(t.Context(), 1, 1, step); !errors.Is(err, c.want) {
				t.Errorf("Run = %v, want %v", err, c.want)
			}
			if n.count() != 0 {
				t.Errorf("a step refused before it starts was journalled %d times, want none", n.count())
			}
		})
	}
	t.Run("a note that fails", func(t *testing.T) {
		t.Parallel()
		n := &notes{err: errNote}
		x, s := build(t, n.note)
		r := bindOrFail(t, x, s)
		_, err := r.Run(t.Context(), 2, 3, plugin.Step{Run: &plugin.Run{Program: s.Manifest.Tool, Argv: []string{"a"}}})
		if !errors.Is(err, errNote) {
			t.Errorf("Run with a failing note = %v, want the note's error", err)
		}
		if n.count() != 1 {
			t.Fatalf("the step was noted %d times, want once", n.count())
		}
		got := n.got[0]
		if got.Round != 2 || got.Step != 3 || got.Grant != s.Manifest.Tool || got.Program != r.Tool().Path || got.Level != plugin.LevelIsolated {
			t.Errorf("the note = %+v, want round 2, step 3, the grant %s, the program %s and the level isolated", got, s.Manifest.Tool, r.Tool().Path)
		}
	})
}

// stepsAskNoted: Bind asks the tool's grant its version, and that ask is a
// start, so it is journalled first: once,
// as a VersionAsk of round 0 and step 0 naming the grant and the program the
// runner then holds; and a note that fails ends Bind with its error.
func stepsAskNoted(t *testing.T, build StepsMaker) {
	t.Helper()
	t.Parallel()
	t.Run("the ask is noted", func(t *testing.T) {
		t.Parallel()
		n := &notes{}
		x, s := build(t, n.note)
		r := bindOrFail(t, x, s)
		got := n.asked()
		if len(got) != 1 || got[0].Round != 0 || got[0].Step != 0 || got[0].Grant != s.Manifest.Tool || got[0].Program != r.Tool().Path || n.count() != 0 {
			t.Errorf("Bind noted the asks %+v and %d steps, want one ask of %s at %s, round 0, step 0, and no step", got, n.count(), s.Manifest.Tool, r.Tool().Path)
		}
	})
	t.Run("an ask whose note fails ends Bind", func(t *testing.T) {
		t.Parallel()
		n := &notes{askErr: errNote}
		x, s := build(t, n.note)
		if _, err := x.Bind(t.Context(), s); !errors.Is(err, errNote) {
			t.Errorf("Bind with a failing note = %v, want the note's error", err)
		}
	})
}
