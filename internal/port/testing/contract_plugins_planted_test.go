package porttest_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// pluginsPlants is, by label, a line the Plugins suite's failure must hold: a
// plant that fails for another reason is red and named (see passes).
func pluginsPlants() map[string]string {
	return map[string]string{
		"a list in the order installed":             "List = ",
		"a root with none listed as an empty slice": "want a nil list and no error",
		"an Open that ignores its ctx":              "Open = <nil>, want an error matching context deadline exceeded",
		"a call that ignores its ctx":               "Plan = the plugin answered unsupported at /target: the standin serves source only, want an error matching context deadline exceeded",
		"a Close that frees nothing":                "after Close answered no error",
		"a missing ref as a manifest refusal":       "want no plugin and ErrNotInstalled",
		"a digest refusal as a manifest one":        "Open = ",
		"a failed call answered as an empty answer": "answered no error, want one",
		"a refusal that opened the plugin":          "Open of a refused plugin answered one",
		"a failure that matches every sentinel":     "matches a sentinel of the port besides what the failure names",
		"a refusal that ran the plugin's code":      "a refused plugin ran 1 calls of its code",
		"a fixture that counts no calls":            "the fixture counts no calls of the plugin's code",
	}
}

// plantedPlugins wraps the stand-in's fixture, each plant breaking one promise.
type plantedPlugins struct {
	port.Plugins
	label string
	ran   *atomic.Int32
}

func (p plantedPlugins) List(ctx context.Context, root string) ([]model.PluginRef, error) {
	got, err := p.Plugins.List(ctx, root)
	switch {
	case p.label == "a list in the order installed" && len(got) > 1:
		got[0], got[len(got)-1] = got[len(got)-1], got[0]
	case p.label == "a root with none listed as an empty slice" && got == nil:
		got = []model.PluginRef{}
	}
	return got, err
}

func (p plantedPlugins) Open(ctx context.Context, root string, ref model.PluginRef) (port.Plugin, error) {
	if p.label == "an Open that ignores its ctx" {
		ctx = context.WithoutCancel(ctx)
	}
	got, err := p.Plugins.Open(ctx, root, ref)
	if p.label == "a refusal that ran the plugin's code" && err != nil {
		p.ran.Add(1)
	}
	switch {
	case p.label == "a missing ref as a manifest refusal" && err != nil && ref.Name == "absent":
		return nil, port.ErrManifest
	case p.label == "a digest refusal as a manifest one" && err != nil && errors.Is(err, port.ErrDigest):
		return nil, port.ErrManifest
	case p.label == "a refusal that opened the plugin" && err != nil:
		return &plantedPlugin{}, err
	case err != nil:
		return nil, err
	}
	return &plantedPlugin{Plugin: got, label: p.label}, nil
}

// plantedPlugin is the opened plugin with its plant.
type plantedPlugin struct {
	port.Plugin
	label string
}

func (p *plantedPlugin) Close() error {
	if p.label == "a Close that frees nothing" || p.Plugin == nil {
		return nil
	}
	return p.Plugin.Close()
}

func (p *plantedPlugin) Plan(ctx context.Context, in plugin.PlanRequest) (plugin.PlanResponse, error) {
	switch p.label {
	case "a call that ignores its ctx":
		ctx = context.WithoutCancel(ctx)
	}
	r, err := p.Plugin.Plan(ctx, in)
	if p.label == "a failure that matches every sentinel" && err != nil {
		err = errors.Join(err, port.ErrGrant, port.ErrStepNotReady, port.ErrTrap)
	}
	if p.label == "a failed call answered as an empty answer" && err != nil {
		return plugin.PlanResponse{}, nil
	}
	return r, err
}

// plantFixture is f with the plant of label: the adapter wrapped, or the
// fixture's own count taken out or made to see a run of code.
func plantFixture(f contracttest.PluginsFixture, label string) contracttest.PluginsFixture {
	ran := new(atomic.Int32)
	f.Plugins = plantedPlugins{Plugins: f.Plugins, label: label, ran: ran}
	switch label {
	case "a fixture that counts no calls":
		f.Ran = nil
	case "a refusal that ran the plugin's code":
		inner := f.Ran
		f.Ran = func(ref model.PluginRef) int { return inner(ref) + int(ran.Load()) }
	}
	return f
}

// TestAPluginsAdapterThatBreaksAPromiseFailsThePluginsSuite: the
// suite is red on a stand-in that lists in the order installed, lists none as an
// empty slice, ignores its ctx at Open or on a call, calls after Close, names a
// missing plugin as a manifest refusal, tells one refusal as another, answers a
// failed call with no error, or opens a plugin it refused. The control is the
// stand-in as it is.
func TestAPluginsAdapterThatBreaksAPromiseFailsThePluginsSuite(t *testing.T) {
	t.Parallel()
	for label, want := range pluginsPlants() {
		if passes(t, label, want, func(t *testing.T) {
			t.Helper()
			contracttest.Plugins(t, func(t *testing.T) contracttest.PluginsFixture {
				t.Helper()
				return plantFixture(standinFixture(t), label)
			})
		}) {
			t.Errorf("a plugins adapter with %s passed the suite", label)
		}
	}
	if !passes(t, control, "", func(t *testing.T) { t.Helper(); contracttest.Plugins(t, standinFixture) }) {
		t.Error("the control, the stand-in as it is, failed the suite")
	}
}
