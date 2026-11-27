package coreapi_test

import (
	"context"
	"maps"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// promoteStaged is a repository whose HEAD holds the promote world's
// rulebook and whose index holds rule promote's change to it, its line
// appended, as the command wrote them; and the promote that wrote them,
// which a replayer makes again over the ports it is handed.
func promoteStaged(t *testing.T) (*porttest.World, model.Root, func(context.Context, port.Ports) (*coreapi.Change, *model.Refusal, error)) {
	t.Helper()
	return promoteStagedBy(t, testWho)
}

// promoteStagedBy is promoteStaged whose staged line was written with who as
// who made it; the promote handed back plans as a gragas of today does, with
// the command's own name.
func promoteStagedBy(t *testing.T, wrote string) (*porttest.World, model.Root, func(context.Context, port.Ports) (*coreapi.Change, *model.Refusal, error)) {
	t.Helper()
	w, root := promoteWorld(t)
	promote := func(ctx context.Context, p port.Ports) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.RulePromote(ctx, p, changeAsk(root, "gragas rule promote go.errors.2"), model.PromoteRule{ID: "go.errors.2", Why: why()})
	}
	files := func() map[string]string {
		es, err := w.Ports().Tree.List(root, "book", model.Every)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, e := range es {
			if e.Dir {
				continue
			}
			out[e.Path] = read(t, w, root, e.Path)
		}
		return out
	}
	head := files()
	was := changeAsk(root, "gragas rule promote go.errors.2")
	was.Who = wrote
	c, ref, err := coreapi.RulePromote(t.Context(), w.Ports(), was, model.PromoteRule{ID: "go.errors.2", Why: why()})
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("promote: %+v", out)
	}
	index := files()
	if maps.Equal(head, index) {
		t.Fatal("the promote changed no file")
	}
	w.SeedRepo(root, porttest.Repo{
		Who:   &model.Identity{Name: "Ada", Email: "ada@example.org"},
		Heads: map[string]porttest.Rev{"HEAD": {Commit: "1111111111111111111111111111111111111111", Files: head}},
		Index: index,
	})
	return w, root, promote
}

// TestAReplayRefusesEachPortAPlanHasNoUseFor: a replay hands
// its command the tree before it, a fixed clock and its history, and every
// other port that reaches the host refuses, naming itself. A line replayed
// with them refusing is allowed; a command whose plan takes a lock, finds or
// runs a tool, fetches, prepares a scratch folder, verifies or asks the
// host's accounts is refused, naming that port. Each plant reaches the port
// first and then plans as the line did, so with its stub taken out the
// fake answers and the staged change is allowed, or the fake fails the test.
// Risk: a replayed command runs a tool, takes a lock or reads the host's
// accounts while guard staged judges a commit.
func TestAReplayRefusesEachPortAPlanHasNoUseFor(t *testing.T) {
	t.Parallel()
	// the control: a plan that reaches no port is replayed
	const noPort = "a plan that reaches no port"
	plants := map[string]func(context.Context, port.Ports, model.Root) error{
		noPort: func(context.Context, port.Ports, model.Root) error { return nil },
		"Lock": func(ctx context.Context, p port.Ports, root model.Root) error {
			_, err := p.Lock.Take(ctx, root, model.RootLock)
			return err
		},
		"Steps": func(ctx context.Context, p port.Ports, root model.Root) error {
			_, err := p.Steps.Bind(ctx, plugin.Scope{Root: root})
			return err
		},
		"Remote": func(ctx context.Context, p port.Ports, root model.Root) error {
			_, err := p.Remote.Commit(ctx, root, []string{"book/rulebook.yaml"}, "m")
			return err
		},
		"History.Who": func(ctx context.Context, p port.Ports, root model.Root) error {
			_, err := p.History.Who(ctx, root)
			return err
		},
		"Sandbox": func(ctx context.Context, p port.Ports, _ model.Root) error {
			_, err := p.Sandbox.Prepare(ctx, model.Prepare{Level: model.Isolated})
			return err
		},
	}
	for name, reach := range plants {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root, promote := promoteStaged(t)
			replay := func(ctx context.Context, p port.Ports, _ string) (*coreapi.Change, *model.Refusal, error) {
				if err := reach(ctx, p, root); err != nil {
					return nil, nil, err
				}
				return promote(ctx, p)
			}
			_, ref, err := coreapi.GuardStaged(t.Context(), w.Ports(), coreapi.StagedAsk{Root: root, Replay: replay, Run: "guard-1", Engine: "v0.0.0-test"})
			if err != nil {
				t.Fatal(err)
			}
			if name == noPort {
				if ref != nil {
					t.Fatalf("the line replayed with the ports refusing is refused: %s", ref.Observed())
				}
				return
			}
			want := "reached for the " + name + " port, which a replay refuses"
			if ref == nil || !strings.Contains(ref.Observed(), want) {
				t.Fatalf("a plan that reaches the %s port: ref = %v, want a refusal saying %q", name, ref, want)
			}
		})
	}
}

// TestALineWrittenBeforeWhoWasTheCommandIsRefusedSayingSo: a line an older
// gragas wrote names the runner's name and address as who, which is part of
// its plan's digest, so its command plans another digest now. The refusal says the line is an older one and how to make it
// again, and not that the digest was forged; a line whose who is the
// command's own name and whose digest is wrong is still the forgery.
// Risk: a person told their line was edited by hand when it was only old.
func TestALineWrittenBeforeWhoWasTheCommandIsRefusedSayingSo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, wrote, want, notWant string
	}{
		{"an older line", "Ada <ada@example.org>", "written by an older gragas", "edited by hand"},
		{"a line of today whose digest is wrong", "gragas rule promote", "edited by hand", "an older gragas"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, root, promote := promoteStagedBy(t, tc.wrote)
			replay := func(ctx context.Context, p port.Ports, _ string) (*coreapi.Change, *model.Refusal, error) {
				if tc.wrote == "gragas rule promote" {
					// the plan the command makes today is not the one the line holds
					a := changeAsk(root, "gragas rule promote go.errors.2")
					a.Who = tc.wrote
					other := model.Why{New: &model.NewDecision{Slug: "decide-it", Title: "Decide it again", Text: "Because."}}
					return coreapi.RulePromote(ctx, p, a, model.PromoteRule{ID: "go.errors.2", Why: other})
				}
				return promote(ctx, p)
			}
			_, ref, err := coreapi.GuardStaged(t.Context(), w.Ports(), coreapi.StagedAsk{Root: root, Replay: replay, Run: "guard-1", Engine: "v0.0.0-test"})
			if err != nil {
				t.Fatal(err)
			}
			if ref == nil || !strings.Contains(ref.Observed(), tc.want) || strings.Contains(ref.Observed(), tc.notWant) {
				t.Fatalf("ref = %v, want a refusal saying %q and not %q", ref, tc.want, tc.notWant)
			}
		})
	}
}
