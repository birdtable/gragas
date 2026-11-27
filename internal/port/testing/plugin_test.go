package porttest_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The in-memory stand-in of port.Plugins is driven through the port alone:
// each of the five operations, each of the four codes a plugin
// answers with, the host's failures at Open and on a call, a bad answer, and
// what a closed plugin does. A test of the core that needs a plugin installs one
// like these.

const root = "/plugins"

// ref is the plugin every test here installs.
func ref() model.PluginRef { return model.PluginRef{Name: "standin", Version: "1.0.0"} }

// installed is a Standin with one plugin that serves every operation with a
// fixed answer, and the plugin.
func installed(t *testing.T) (port.Plugin, *porttest.StandinPlugin) {
	t.Helper()
	p := &porttest.StandinPlugin{
		Facts:     plugin.Manifest{Name: "standin", Version: "1.0.0", Provides: []plugin.Op{plugin.OpPlan, plugin.OpEntries, plugin.OpFindings, plugin.OpRank, plugin.OpPreset}},
		OnPlan:    func(plugin.PlanRequest) (plugin.PlanResponse, error) { return plugin.PlanResponse{Final: true}, nil },
		OnEntries: func(plugin.EntriesRequest) (plugin.EntriesResponse, error) { return plugin.EntriesResponse{}, nil },
		OnFindings: func(plugin.FindingsRequest) (plugin.FindingsResponse, error) {
			return plugin.FindingsResponse{Ran: true}, nil
		},
		OnRank: func(in plugin.RankRequest) (plugin.RankResponse, error) {
			w := make([]plugin.Word, len(in.Pairs))
			for i := range w {
				w[i] = plugin.WordSame
			}
			return plugin.RankResponse{Words: w}, nil
		},
		OnPreset: func(plugin.PresetRequest) (plugin.PresetResponse, error) {
			return plugin.PresetResponse{Files: []plugin.PresetFile{{Path: "a", Text: "b"}}}, nil
		},
	}
	var s porttest.Standin
	s.Install(root, ref(), p)
	got, err := s.Open(context.Background(), root, ref())
	if err != nil {
		t.Fatal(err)
	}
	return got, p
}

// TestEveryOperationIsDrivenThroughThePort: the five operations answer what the
// test scripted, in the order called, and the manifest is read, never called.
func TestEveryOperationIsDrivenThroughThePort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	got, p := installed(t)
	if _, err := got.Plan(ctx, plugin.PlanRequest{}); err != nil {
		t.Errorf("Plan: %v", err)
	}
	if _, err := got.Entries(ctx, plugin.EntriesRequest{}); err != nil {
		t.Errorf("Entries: %v", err)
	}
	if r, err := got.Findings(ctx, plugin.FindingsRequest{}); err != nil || !r.Ran {
		t.Errorf("Findings: %v, %v", r, err)
	}
	if r, err := got.Rank(ctx, plugin.RankRequest{Pairs: make([]plugin.Pair, 2)}); err != nil || len(r.Words) != 2 {
		t.Errorf("Rank: %v, %v", r, err)
	}
	if r, err := got.Preset(ctx, plugin.PresetRequest{}); err != nil || len(r.Files) != 1 {
		t.Errorf("Preset: %v, %v", r, err)
	}
	if got.Manifest().Name != "standin" {
		t.Errorf("Manifest().Name = %q", got.Manifest().Name)
	}
	want := []plugin.Op{plugin.OpPlan, plugin.OpEntries, plugin.OpFindings, plugin.OpRank, plugin.OpPreset}
	if c := p.Calls(); len(c) != len(want) || c[0] != want[0] || c[4] != want[4] {
		t.Errorf("Calls = %v, want %v and no call for Manifest", c, want)
	}
}

// TestACodeOfThePluginIsAnErrorThatKeepsItsCodeAndPath: each of the four codes
// of the wire comes back as a *plugin.CallError that keeps
// its code and its path, and is never an empty answer.
func TestACodeOfThePluginIsAnErrorThatKeepsItsCodeAndPath(t *testing.T) {
	t.Parallel()
	for _, code := range []plugin.Code{plugin.CodeUnsupported, plugin.CodeUnreadable, plugin.CodeRefused, plugin.CodeBug} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			var s porttest.Standin
			s.Install(root, ref(), &porttest.StandinPlugin{OnPreset: func(plugin.PresetRequest) (plugin.PresetResponse, error) {
				return plugin.PresetResponse{}, &plugin.CallError{Code: code, Message: "no", Path: "/entries/3/options"}
			}})
			got, err := s.Open(context.Background(), root, ref())
			if err != nil {
				t.Fatal(err)
			}
			_, err = got.Preset(context.Background(), plugin.PresetRequest{})
			var pe *plugin.CallError
			if !errors.As(err, &pe) || pe.Code != code || pe.Path != "/entries/3/options" {
				t.Errorf("err = %v, want a CallError of %s at /entries/3/options", err, code)
			}
		})
	}
}

// TestAnOperationNotScriptedIsUnsupported: a plugin with no answer for an
// operation answers the wire's refusal, not an empty answer.
func TestAnOperationNotScriptedIsUnsupported(t *testing.T) {
	t.Parallel()
	var s porttest.Standin
	s.Install(root, ref(), &porttest.StandinPlugin{})
	got, err := s.Open(context.Background(), root, ref())
	if err != nil {
		t.Fatal(err)
	}
	_, err = got.Rank(context.Background(), plugin.RankRequest{})
	var pe *plugin.CallError
	if !errors.As(err, &pe) || pe.Code != plugin.CodeUnsupported {
		t.Errorf("err = %v, want unsupported", err)
	}
}

// TestTheHostsFailuresMatchTheirSentinels: Open's four refusals, a limit, a bad
// answer, a trap and a grant, each matching its sentinel through errors.Is, a
// limit and a bad answer carrying their field.
func TestTheHostsFailuresMatchTheirSentinels(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"not approved": {port.ErrNotApproved, port.ErrNotApproved},
		"digest":       {port.ErrDigest, port.ErrDigest},
		"contract":     {port.ErrContract, port.ErrContract},
		"feature":      {port.ErrFeature, port.ErrFeature},
		"manifest":     {port.ErrManifest, port.ErrManifest},
		"module":       {port.ErrModule, port.ErrModule},
	} {
		t.Run("open "+name, func(t *testing.T) {
			t.Parallel()
			var s porttest.Standin
			p := &porttest.StandinPlugin{OpenErr: tc.err}
			s.Install(root, ref(), p)
			got, err := s.Open(context.Background(), root, ref())
			if got != nil || !errors.Is(err, tc.want) {
				t.Errorf("Open = %v, %v, want no plugin and %v", got, err, tc.want)
			}
			if o, _ := p.Opened(); o != 0 {
				t.Errorf("the plugin was opened %d times though Open failed", o)
			}
		})
	}
	for name, tc := range map[string]struct {
		err error
		is  func(error) bool
	}{
		"limit": {&plugin.LimitError{What: plugin.LimitTime}, func(err error) bool {
			var e *plugin.LimitError
			return errors.As(err, &e) && e.What == plugin.LimitTime
		}},
		"answer": {&plugin.AnswerError{Path: "/entries/0", Why: "out of order"}, func(err error) bool {
			var e *plugin.AnswerError
			return errors.As(err, &e) && e.Path == "/entries/0"
		}},
		"trap":  {port.ErrTrap, func(err error) bool { return errors.Is(err, port.ErrTrap) }},
		"grant": {port.ErrGrant, func(err error) bool { return errors.Is(err, port.ErrGrant) }},
	} {
		t.Run("call "+name, func(t *testing.T) {
			t.Parallel()
			var s porttest.Standin
			s.Install(root, ref(), &porttest.StandinPlugin{OnEntries: func(plugin.EntriesRequest) (plugin.EntriesResponse, error) {
				return plugin.EntriesResponse{}, tc.err
			}})
			got, _ := s.Open(context.Background(), root, ref())
			_, err := got.Entries(context.Background(), plugin.EntriesRequest{})
			var own *plugin.CallError
			if !tc.is(err) || errors.As(err, &own) {
				t.Errorf("err = %v, want the host's failure and not a plugin's own", err)
			}
		})
	}
}

// TestListAnswersByNameAndVersionAndOpenOfNothingIsNotInstalled.
func TestListAnswersByNameAndVersionAndOpenOfNothingIsUnsupported(t *testing.T) {
	t.Parallel()
	var s porttest.Standin
	for _, r := range []model.PluginRef{{Name: "b", Version: "1.0.0"}, {Name: "a", Version: "2.0.0"}, {Name: "a", Version: "1.0.0"}} {
		s.Install(root, r, &porttest.StandinPlugin{})
	}
	got, err := s.List(context.Background(), root)
	want := []model.PluginRef{{Name: "a", Version: "1.0.0"}, {Name: "a", Version: "2.0.0"}, {Name: "b", Version: "1.0.0"}}
	if err != nil || len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("List = %v, %v, want %v", got, err, want)
	}
	if l, _ := s.List(context.Background(), "/elsewhere"); len(l) != 0 {
		t.Errorf("List of a root with none = %v", l)
	}
	_, err = s.Open(context.Background(), root, model.PluginRef{Name: "none", Version: "1.0.0"})
	var pe *plugin.CallError
	if !errors.Is(err, port.ErrNotInstalled) || errors.As(err, &pe) {
		t.Errorf("Open of nothing = %v, want ErrNotInstalled and no plugin answer", err)
	}
}

// TestACallOnAClosedPluginFailsAndCloseTwiceIsNoError, and calls at once are safe.
func TestACallOnAClosedPluginFailsAndCloseTwiceIsNoError(t *testing.T) {
	t.Parallel()
	got, p := installed(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := got.Plan(context.Background(), plugin.PlanRequest{}); err != nil {
				t.Errorf("Plan: %v", err)
			}
		})
	}
	wg.Wait()
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
	if err := got.Close(); err != nil {
		t.Errorf("a second Close = %v", err)
	}
	if _, err := got.Plan(context.Background(), plugin.PlanRequest{}); !errors.Is(err, porttest.ErrClosed) {
		t.Errorf("Plan after Close = %v, want ErrClosed", err)
	}
	if o, c := p.Opened(); o != 1 || c != 1 {
		t.Errorf("opened %d closed %d, want 1 and 1", o, c)
	}
	if n := len(p.Calls()); n != 8 {
		t.Errorf("%d calls, want the 8 made before Close", n)
	}
}
