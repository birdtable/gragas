package tool_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// ranPaths is the path of each program tl started, in order.
func ranPaths(tl *tool.Tool) []string {
	var out []string
	for _, r := range tl.Ran() {
		if len(r.Argv) > 0 {
			out = append(out, r.Argv[0])
		}
	}
	return out
}

// TestAZeroScopeNeverTakesARepoLocalProgram: a Scope that says nothing of OwnPlace binds by the
// pinned find, so a program at the grant's own place below the project is
// never started, not even for its version: with the tools file pinning one,
// Bind takes that, and with the grant's place alone, Bind is not ready. With
// OwnPlace, the grant's place is taken. Planted: the zero value taking the
// grant's place, as Pinned's zero did (the two zero rows red).
func TestAZeroScopeNeverTakesARepoLocalProgram(t *testing.T) {
	t.Parallel()
	root := private(t)
	own := program(t, root+"/"+binDir+"/"+proc.LocalFileName("lint-a"), says("1.2.3"))
	pin := elsewhere(t, "1.2.3")
	m := atGrant()
	m.Tool = "lint-a"
	scope := func(ownPlace bool) plugin.Scope {
		return plugin.Scope{Manifest: m, Root: model.Root(root), OwnPlace: ownPlace, Floor: plugin.LevelIsolated, Note: (&notes{}).note,
			Scratch: model.Scratch{Dir: private(t), Level: model.Isolated, Seconds: 60}}
	}
	t.Run("a zero Scope with a pinned program takes the pinned one", func(t *testing.T) {
		t.Parallel()
		tl := over(nil, map[string]string{"lint-a@1.2.3": pin})
		r, err := tl.Bind(t.Context(), scope(false))
		if err != nil || r.Tool().Path != pin {
			t.Fatalf("Bind = %v; want the pinned %s", err, pin)
		}
		if ran := ranPaths(tl); slices.Contains(ran, own) {
			t.Errorf("a zero Scope started the repository's %s: %q", own, ran)
		}
	})
	t.Run("a zero Scope with the grant's place alone is not ready", func(t *testing.T) {
		t.Parallel()
		tl := over(nil, nil)
		_, err := tl.Bind(t.Context(), scope(false))
		if !errors.Is(err, port.ErrStepNotReady) {
			t.Errorf("Bind = %v, want port.ErrStepNotReady", err)
		}
		if ran := ranPaths(tl); len(ran) != 0 {
			t.Errorf("a zero Scope started %q", ran)
		}
	})
	t.Run("OwnPlace takes the grant's place", func(t *testing.T) {
		t.Parallel()
		tl := over(nil, map[string]string{"lint-a@1.2.3": pin})
		r, err := tl.Bind(t.Context(), scope(true))
		if err != nil || r.Tool().Path != own {
			t.Fatalf("Bind = %v; want the grant's %s", err, own)
		}
	})
}

// TestEveryStartIsNotedFirst:
// a grant's version ask is a start, so it is journalled before it starts, as
// a step is: Bind's ask of the tool's grant as round 0, step 0, and the first
// find of any other grant as the step that names it. A note that fails
// starts nothing and ends Bind, or the step, with its error. Planted: the
// ask started with no note (the noted rows red), and the note's error
// dropped (the failing rows red).
func TestEveryStartIsNotedFirst(t *testing.T) {
	t.Parallel()
	full := errors.New("the journal is full")
	// two is a scope whose tool is lint-a and that grants lint-b too, each
	// named in the settings, with the note n.
	two := func(t *testing.T, n *notes) (plugin.Scope, *tool.Tool, string, string) {
		t.Helper()
		s, _, a := steps(t, model.Isolated, plugin.LevelIsolated, n)
		b := program(t, private(t)+"/lint-b", says("1.2.3"))
		s.Manifest.Programs = append(s.Manifest.Programs, plugin.Program{Found: &plugin.ProgramFound{ID: "lint-b", Versions: inRange, VersionArgv: []string{"--version"}}})
		return s, over(map[string]map[string]string{string(s.Root): {"lint-a@1.2.3": a, "lint-b@1.2.3": b}}, nil), a, b
	}
	t.Run("Bind's version ask is noted before it starts", func(t *testing.T) {
		t.Parallel()
		n := &notes{}
		s, tl, a, _ := two(t, n)
		before := -1
		s.Note = func(ctx context.Context, x plugin.Noted) error {
			before = len(tl.Ran())
			return n.note(ctx, x)
		}
		bind(t, tl, s)
		want := plugin.Noted{Grant: "lint-a", Program: a, Words: []string{"--version"}, VersionAsk: true}
		if got := n.asks(); len(got) != 1 || !same(got[0], want) || before != 0 || len(tl.Ran()) != 1 {
			t.Errorf("noted %+v with %d started before it and %d after; want %+v, noted before the one start", got, before, len(tl.Ran()), want)
		}
	})
	t.Run("a version ask whose note fails starts nothing", func(t *testing.T) {
		t.Parallel()
		n := &notes{failAsk: full}
		s, tl, _, _ := two(t, n)
		if _, err := tl.Bind(t.Context(), s); !errors.Is(err, full) || len(tl.Ran()) != 0 {
			t.Errorf("Bind = %v after %d starts, want the note's error and none", err, len(tl.Ran()))
		}
	})
	t.Run("another grant's first find is noted at its step", func(t *testing.T) {
		t.Parallel()
		n := &notes{}
		s, tl, _, b := two(t, n)
		r := bind(t, tl, s)
		if _, err := r.Run(t.Context(), 3, 4, plugin.Step{Run: &plugin.Run{Program: "lint-b"}}); err != nil {
			t.Fatal(err)
		}
		want := plugin.Noted{Round: 3, Step: 4, Grant: "lint-b", Program: b, Words: []string{"--version"}, VersionAsk: true}
		if got := n.asks(); len(got) != 2 || !same(got[1], want) {
			t.Errorf("asks noted %+v, want the second %+v", got, want)
		}
	})
	t.Run("another grant's version ask whose note fails starts nothing", func(t *testing.T) {
		t.Parallel()
		n := &notes{}
		s, tl, _, _ := two(t, n)
		r := bind(t, tl, s)
		n.mu.Lock()
		n.failAsk = full
		n.mu.Unlock()
		_, err := r.Run(t.Context(), 3, 4, plugin.Step{Run: &plugin.Run{Program: "lint-b"}})
		for _, x := range tl.Ran() {
			if x.Tool == "lint-b" {
				t.Errorf("lint-b started with its note refused: %+v", x)
			}
		}
		if !errors.Is(err, full) || len(n.all()) != 0 {
			t.Errorf("Run = %v with steps noted %+v, want the note's error and no step noted", err, n.all())
		}
	})
}

// TestABindToAFolderWithNoDeadlineOrABelowZeroLimitIsRefused: a prepared
// folder's deadline is never 0 and
// its memory limit never below 0 (model.Scratch), and the deleted Tool
// port's held() refused both, so Bind refuses a scope with either, matching port.ErrRefused,
// before it starts anything, the tool's version included. The control, the
// same scope at 1 s and no limit, binds. Planted: the floor dropped (the three
// refused rows red: a deadline of 0 or below ran at the host's own wait and
// -1 MiB with no limit).
func TestABindToAFolderWithNoDeadlineOrABelowZeroLimitIsRefused(t *testing.T) {
	t.Parallel()
	root := private(t)
	pin := elsewhere(t, "1.2.3")
	for _, c := range []struct {
		name         string
		seconds, mib int
		refused      bool
	}{
		{"a deadline of 0", 0, 0, true},
		{"a deadline below 0", -1, 0, true},
		{"a limit below 0", 60, -1, true},
		{"1 s and no limit", 1, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tl := stepOver(nil, map[string]string{"lint-a@1.2.3": pin})
			s := stepScope(t, root, false)
			s.Scratch.Seconds, s.Scratch.MemoryMiB = c.seconds, c.mib
			_, err := tl.Bind(t.Context(), s)
			if !c.refused {
				if err != nil {
					t.Fatalf("Bind = %v, want it bound", err)
				}
				return
			}
			if !errors.Is(err, port.ErrRefused) {
				t.Errorf("Bind = %v, want port.ErrRefused", err)
			}
			if ran := ranPaths(tl); len(ran) != 0 {
				t.Errorf("a refused Bind started %q", ran)
			}
		})
	}
}
