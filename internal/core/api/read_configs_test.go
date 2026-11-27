package coreapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// noRun is a runner that grants and runs nothing: the plugins below ask for no
// step.
type noRun struct{}

func (noRun) Granted(plugin.Step) error { return nil }
func (noRun) Run(context.Context, int, int, plugin.Step) (plugin.Result, error) {
	return plugin.Result{}, errors.New("no step is asked")
}

// entriesOver is the EntriesCall of a plugin the stand-in opens: the plan
// rounds and the one entries call, as the engine makes them.
func entriesOver(t *testing.T, p *porttest.StandinPlugin) link.EntriesCall {
	t.Helper()
	ref := model.PluginRef{Name: "standin", Version: "1.0.0"}
	var s porttest.Standin
	s.Install("root", ref, p)
	open, err := s.Open(t.Context(), "root", ref)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = open.Close() })
	return func(ctx context.Context, in plugin.PlanRequest) (plugin.EntriesResponse, error) {
		return plugins.Entries(ctx, open, noRun{}, in)
	}
}

func finalPlan(plugin.PlanRequest) (plugin.PlanResponse, error) {
	return plugin.PlanResponse{Final: true}, nil
}

// TestReadConfigsHoldsWhatAPluginSaysIsOnAtTheEnginesSeverity:
// one call is made for each file, so each entry is held by the file it came
// from; an entry the plugin says is off holds nothing, one with no severity
// stands at error, a string value is as the plugin printed it and any other
// value and the options are JSON on one line.
// Risk: an import holds a rule the config turned off, or one weaker than the
// config said.
func TestReadConfigsHoldsWhatAPluginSaysIsOnAtTheEnginesSeverity(t *testing.T) {
	t.Parallel()
	warn, info := plugin.SeverityWarning, plugin.SeverityInfo
	var asked []plugin.PlanRequest
	call := entriesOver(t, &porttest.StandinPlugin{
		OnPlan: finalPlan,
		OnEntries: func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			path := in.Config[0].Path
			asked = append(asked, plugin.PlanRequest{Config: in.Config, Target: in.Target})
			if path == "b.json" {
				return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "plain", On: true}}}, nil
			}
			return plugin.EntriesResponse{Entries: []plugin.Entry{
				{Key: "loud", On: true, Severity: &warn, Value: canon.String("strict"), Options: canon.Object{"a": canon.Int(1)}},
				{Key: "quiet", On: false, Severity: &info},
				{Key: "count", On: true, Severity: &info, Value: canon.Int(3)},
			}}, nil
		},
	})
	got, err := link.ReadConfigs(t.Context(), call, "1.0.0", model.TargetSource, []link.ConfigFile{
		{Path: "a.json", Data: []byte(`{}`)}, {Path: "b.json", Data: []byte(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.ImportEntry{
		{Config: "a.json", Key: "loud", Value: "strict", Options: `{"a":1}`, Severity: model.SeverityWarning},
		{Config: "a.json", Key: "count", Value: "3", Severity: model.SeverityInfo},
		{Config: "b.json", Key: "plain", Severity: model.SeverityError},
	}
	if len(got) != len(want) {
		t.Fatalf("imported %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d is %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(asked) != 2 || asked[0].Target != plugin.TargetSource {
		t.Errorf("the plugin was asked %d times at %v, want once for each file at source", len(asked), asked)
	}
}

// TestReadConfigsNamesTheFileAPluginFailsOnAndRefusesATargetOrTextItCannotSend:
// a plugin's failure ends the read naming the file; a target the contract has
// no word for and a config that is not UTF-8 text are refused before a call.
// Risk: a failed read is taken for an empty config, or a file of bytes is sent
// as a second spelling.
func TestReadConfigsNamesTheFileAPluginFailsOnAndRefusesATargetOrTextItCannotSend(t *testing.T) {
	t.Parallel()
	calls := 0
	call := entriesOver(t, &porttest.StandinPlugin{
		OnPlan: finalPlan,
		OnEntries: func(plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			calls++
			return plugin.EntriesResponse{}, errors.New("the tool is not here")
		},
	})
	files := []link.ConfigFile{{Path: "x.json", Data: []byte(`{}`)}}
	if _, err := link.ReadConfigs(t.Context(), call, "1", model.TargetSource, files); err == nil || !strings.Contains(err.Error(), "x.json") {
		t.Errorf("a failed call: err = %v, want it naming x.json", err)
	}
	before := calls
	if _, err := link.ReadConfigs(t.Context(), call, "1", model.TargetType("elsewhere"), files); err == nil {
		t.Error("a target the contract has no word for was read")
	}
	if _, err := link.ReadConfigs(t.Context(), call, "1", model.TargetSource, []link.ConfigFile{{Path: "bin", Data: []byte{0xff, 0xfe}}}); err == nil || !strings.Contains(err.Error(), "not UTF-8") {
		t.Errorf("a config of bytes: err = %v, want it refused as not UTF-8 text", err)
	}
	if calls != before {
		t.Errorf("%d calls were made for a read refused before it", calls-before)
	}
}
