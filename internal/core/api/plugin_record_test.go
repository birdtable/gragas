package coreapi_test

import (
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// recordWorld is a stand-in world with a plugin for the tool "t" installed
// under /repo, its author's folder at /repo/author/p/1.0.0 holding the module
// the installed bundle holds, and the tool seeded at 1.4.2. onPlan is what the
// plugin plans, and each of facts edits its manifest before it is installed.
func recordWorld(t *testing.T, onPlan func(plugin.PlanRequest) (plugin.PlanResponse, error), facts ...func(*plugin.Manifest)) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	root := w.Seed("/repo", map[string]string{})
	w.Seed("/repo/author/p/1.0.0", map[string]string{"plugin.wasm": "MODULE"})
	ref := model.PluginRef{Name: "p", Version: "1.0.0"}
	w.SeedBundles(string(root), map[model.PluginRef]model.PluginBundle{ref: {Module: []byte("MODULE")}})
	p := porttest.NewStandinPlugin()
	p.Facts.Tool = "t"
	p.Facts.ToolVersions = ">=1.0.0 <2.0.0"
	p.Facts.Programs = []plugin.Program{{Found: &plugin.ProgramFound{ID: "t", Versions: ">=1.0.0 <2.0.0", VersionArgv: []string{"--version"}}}}
	p.OnPlan = onPlan
	for _, f := range facts {
		f(&p.Facts)
	}
	w.SeedTool("t", model.Found{Path: "/tools/t", Version: "1.4.2"})
	w.PluginsOf().Install(string(root), ref, p)
	return w, root
}

// TestARecordingRefusesAProgramThePluginIsNotGranted: a plugin
// whose plan runs a program its manifest does not grant ends the recording in
// a refusal that names the grant, and no change is made. The standin plugin of
// the journeys cannot ask for one, so the world's does. Risk: a recording that
// runs a program outside the grants, or ends in an error that is no refusal.
func TestARecordingRefusesAProgramThePluginIsNotGranted(t *testing.T) {
	t.Parallel()
	w, root := recordWorld(t, func(plugin.PlanRequest) (plugin.PlanResponse, error) {
		return plugin.PlanResponse{Final: true, Steps: []plugin.Step{{Run: &plugin.Run{Program: "other", Argv: []string{"--list"}}}}}, nil
	})
	ask := coreapi.PluginRecordAsk{Ask: changeAsk(root, "gragas plugin record author/p/1.0.0"), Dir: "/repo/author/p/1.0.0", Op: "entries", Apply: true}
	ch, ref, err := coreapi.PluginRecord(t.Context(), w.Ports(), ask)
	if err != nil {
		t.Fatalf("PluginRecord: %v, want a refusal", err)
	}
	if ch != nil || ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), "granted") {
		t.Fatalf("PluginRecord = %v, %+v, want a refusal naming the grant", ch, ref)
	}
	if _, err := w.Ports().Tree.Read("/repo/author/p/1.0.0", "testdata/cases/1-entries/request.json"); err == nil {
		t.Error("a refused recording wrote a case")
	}
}

// TestARecordingNumbersItsCaseAfterTheLastOne: without --case the
// case folder takes the next free number after those the folder holds, and a
// --case that is no whole number is an error. Risk: a recording that overwrites
// the last case, or files under a name no number leads.
func TestARecordingNumbersItsCaseAfterTheLastOne(t *testing.T) {
	t.Parallel()
	w, root := recordWorld(t, func(plugin.PlanRequest) (plugin.PlanResponse, error) {
		return plugin.PlanResponse{Final: true}, nil
	})
	w.Seed("/repo/author/p/1.0.0", map[string]string{"plugin.wasm": "MODULE", "testdata/cases/4-entries/request.json": "{}"})
	ask := coreapi.PluginRecordAsk{Ask: changeAsk(root, "gragas plugin record author/p/1.0.0"), Dir: "/repo/author/p/1.0.0", Op: "entries"}
	ch, ref, err := coreapi.PluginRecord(t.Context(), w.Ports(), ask)
	if err != nil || ref != nil {
		t.Fatalf("PluginRecord: %v, %+v", err, ref)
	}
	out, ok := ch.Preview(t.Context())
	if !ok || !strings.Contains(planText(out.Plan), "cases/5-entries/request.json") {
		t.Errorf("the plan = %+v, want the case 5", out)
	}
	ask.Case = "x"
	bad, _, err := coreapi.PluginRecord(t.Context(), w.Ports(), ask)
	if err != nil {
		t.Fatalf("PluginRecord: %v", err)
	}
	if out, ok := bad.Preview(t.Context()); ok || out.Word != model.Failed {
		t.Errorf("--case x: %+v, want failed, the plan not made", out)
	}
}

// TestARecordingOfAnOpTheRoundsDoNotReadIsAnError: only entries
// and findings are read by the rounds, so any other --op is an error before
// the plugin is opened. Risk: a recording that opens a plugin and runs a tool
// for a call no round makes.
func TestARecordingOfAnOpTheRoundsDoNotReadIsAnError(t *testing.T) {
	t.Parallel()
	w, root := recordWorld(t, func(plugin.PlanRequest) (plugin.PlanResponse, error) {
		return plugin.PlanResponse{Final: true}, nil
	})
	ask := coreapi.PluginRecordAsk{Ask: changeAsk(root, "gragas plugin record author/p/1.0.0"), Dir: "/repo/author/p/1.0.0", Op: "rank", Apply: true}
	ch, ref, err := coreapi.PluginRecord(t.Context(), w.Ports(), ask)
	if err == nil || ch != nil || ref != nil || !strings.Contains(err.Error(), "entries or findings") {
		t.Fatalf("PluginRecord = %v, %+v, %v, want an error saying entries or findings", ch, ref, err)
	}
}

func planText(p model.Plan) string {
	var b strings.Builder
	for _, c := range p.Delta.Changes {
		b.WriteString(c.Entity + "\n")
	}
	return b.String()
}
