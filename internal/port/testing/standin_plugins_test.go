package porttest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestTheInMemoryStandinAnswersThePluginsSuite: the fake of
// port.Plugins answers the suite every adapter of it answers, so a core test
// that passes on the fake is held to what the wasm host promises too.
func TestTheInMemoryStandinAnswersThePluginsSuite(t *testing.T) {
	t.Parallel()
	contracttest.Plugins(t, standinFixture)
}

const pluginsRoot = "/plugins"

// planFails is a plugin whose Plan answers err; every other operation answers
// as the stand-in does.
func planFails(ref model.PluginRef, err error) *porttest.StandinPlugin {
	p := withFacts(ref)
	p.OnPlan = func(plugin.PlanRequest) (plugin.PlanResponse, error) { return plugin.PlanResponse{}, err }
	return p
}

// withFacts is the stand-in's plugin, named as ref.
func withFacts(ref model.PluginRef) *porttest.StandinPlugin {
	p := porttest.NewStandinPlugin()
	p.Facts.Name, p.Facts.Version = ref.Name, ref.Version
	return p
}

// standinFixture is the Standin with every plugin the suite names installed.
func standinFixture(t *testing.T) contracttest.PluginsFixture {
	t.Helper()
	var s porttest.Standin
	byRef := map[model.PluginRef]*porttest.StandinPlugin{}
	install := func(ref model.PluginRef, p *porttest.StandinPlugin) model.PluginRef {
		s.Install(pluginsRoot, ref, p)
		byRef[ref] = p
		return ref
	}
	f := contracttest.PluginsFixture{
		Plugins: &s, Root: pluginsRoot, Empty: "/nothing",
		Missing: model.PluginRef{Name: "absent", Version: "1.0.0"},
		Refused: map[error]model.PluginRef{},
		Call: func(ctx context.Context, p port.Plugin) error {
			_, err := p.Plan(ctx, plugin.PlanRequest{Target: plugin.TargetSource, For: plugin.ForRun})
			return err
		},
		Ran: func(ref model.PluginRef) int {
			if p := byRef[ref]; p != nil {
				return len(p.Calls())
			}
			return 0
		},
	}
	// Installed out of the order they list in, and 1.0.10 before 1.0.2 as bytes.
	for _, ref := range []model.PluginRef{{Name: "zeta", Version: "1.0.0"}, {Name: "standin", Version: "1.0.2"}, {Name: "standin", Version: "1.0.10"}} {
		f.Installed = append(f.Installed, install(ref, withFacts(ref)))
	}
	f.Installed[0], f.Installed[2] = f.Installed[2], f.Installed[0]
	for _, err := range []error{port.ErrManifest, port.ErrNotApproved, port.ErrDigest, port.ErrContract, port.ErrFeature, port.ErrModule} {
		ref := model.PluginRef{Name: fmt.Sprintf("refused-%d", len(f.Refused)), Version: "1.0.0"}
		p := withFacts(ref)
		p.OpenErr = fmt.Errorf("%s: %w", ref.Name, err)
		f.Refused[err] = install(ref, p)
	}
	for i, c := range []struct {
		name string
		err  error
		is   error
		as   any
	}{
		{"the grant", fmt.Errorf("step 1: %w", port.ErrGrant), port.ErrGrant, nil},
		{"a step not ready", fmt.Errorf("step 1: %w", port.ErrStepNotReady), port.ErrStepNotReady, nil},
		{"a trap", fmt.Errorf("call: %w", port.ErrTrap), port.ErrTrap, nil},
		{"the plugin's own error", &plugin.CallError{Code: plugin.CodeRefused, Path: "/config"}, nil, new(*plugin.CallError)},
		{"a limit", &plugin.LimitError{What: plugin.LimitTime}, nil, new(*plugin.LimitError)},
		{"an answer that is no answer", &plugin.AnswerError{Path: "/steps", Why: "unsorted"}, nil, new(*plugin.AnswerError)},
	} {
		ref := model.PluginRef{Name: fmt.Sprintf("failing-%d", i), Version: "1.0.0"}
		install(ref, planFails(ref, c.err))
		f.Failures = append(f.Failures, contracttest.PluginFailure{
			Name: c.name, Ref: ref, Is: c.is, As: c.as, Call: f.Call,
		})
	}
	return f
}
