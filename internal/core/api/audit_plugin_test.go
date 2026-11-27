package coreapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// auditWorld is a world with a stand-in plugin installed for ast-grep, and the
// AuditPlugins over its ports.
func auditWorld(t *testing.T, entries func(plugin.EntriesRequest) (plugin.EntriesResponse, error), plan func(plugin.PlanRequest) (plugin.PlanResponse, error)) (*porttest.World, coreapi.AuditPlugins, *porttest.StandinPlugin) {
	t.Helper()
	const tool = "ast-grep"
	w, _ := ruleWorld(t)
	root := w.Seed("/repo", map[string]string{"a.json": "{}\n"})
	p := porttest.NewStandinPlugin()
	p.Facts.Tool = tool
	p.Facts.Programs = []plugin.Program{{Found: &plugin.ProgramFound{ID: tool, Versions: ">=1.0.0", VersionArgv: []string{"--version"}}}}
	w.SeedTool(tool, model.Found{Path: "/tools/" + tool, Version: "1.1.0"})
	w.SeedStep(tool, plugin.Result{Kind: plugin.KindRun})
	if plan == nil {
		plan = func(plugin.PlanRequest) (plugin.PlanResponse, error) { return plugin.PlanResponse{Final: true}, nil }
	}
	p.OnPlan, p.OnEntries = plan, entries
	w.PluginsOf().Install(string(root), model.PluginRef{Name: tool, Version: "1.0.0"}, p)
	ports := w.Ports()
	ports.Sandbox = &scriptedSandbox{level: model.Isolated}
	return w, coreapi.AuditPlugins{Plugins: ports.Plugins, Steps: ports.Steps, Sandbox: ports.Sandbox, Journal: ports.Journal, Clock: ports.Clock,
		Root: root, Command: "check", Engine: "v0.0.0-test"}, p
}

func auditRequest(text string) plugin.PlanRequest {
	return plugin.PlanRequest{Config: []plugin.Config{{Path: "a.json", Data: &plugin.Bytes{Text: &text}}}, For: plugin.ForConfig, Target: plugin.TargetSource}
}

// TestTheAuditReadsAToolsConfigThroughItsInstalledPluginInARunOfItsOwn: the
// entries the check judges are the installed plugin's, and the read is one
// journal run, an intent first, the starts, and a close last, with the plugin
// opened once and closed once.
// Risk: a check that reads the config itself where a plugin is installed, or
// reads without a run.
func TestTheAuditReadsAToolsConfigThroughItsInstalledPluginInARunOfItsOwn(t *testing.T) {
	t.Parallel()
	w, a, p := auditWorld(t, someEntry, nil)
	call, _, why := coreapi.PluginAuditEntries(a, nil)("ast-grep")
	if call == nil || why != "" {
		t.Fatalf("no call: %q", why)
	}
	got, err := call(context.Background(), auditRequest("{}\n"))
	if err != nil || len(got.Entries) != 1 || got.Entries[0].Key != "k" {
		t.Fatalf("entries %+v, err %v, want the plugin's one entry", got.Entries, err)
	}
	if opened, closed := p.Opened(); opened != 1 || closed != 1 {
		t.Errorf("the plugin was opened %d and closed %d times, want once each", opened, closed)
	}
	recs, _, rerr := w.Ports().Journal.Records(context.Background(), a.Root)
	if rerr != nil || len(recs) < 3 || recs[0].Kind != model.KindIntent || recs[1].Kind != model.KindStart || recs[len(recs)-1].Kind != model.KindClose || recs[0].Run != recs[len(recs)-1].Run || recs[0].Command != "check" {
		t.Errorf("records %+v (%v), want one run of an intent, the starts and a close, for check", recs, rerr)
	}
}

// TestAnInterruptedCheckLeavesNoOpenRun: a check whose ctx is cancelled while
// the plugin plans fails, and the run's close is written all the same.
// Risk: an intent and its starts with no close after Ctrl+C, which refuses
// validate until recovered.
func TestAnInterruptedCheckLeavesNoOpenRun(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, a, _ := auditWorld(t, someEntry, func(plugin.PlanRequest) (plugin.PlanResponse, error) {
		cancel()
		return plugin.PlanResponse{}, errors.New("interrupted")
	})
	call, _, _ := coreapi.PluginAuditEntries(a, nil)("ast-grep")
	if _, err := call(ctx, auditRequest("{}\n")); err == nil {
		t.Fatal("the read of an interrupted check did not fail")
	}
	recs, _, rerr := w.Ports().Journal.Records(context.WithoutCancel(ctx), a.Root)
	if rerr != nil || len(recs) < 2 || recs[len(recs)-1].Kind != model.KindClose || !strings.Contains(string(recs[len(recs)-1].Detail), "failed") {
		t.Errorf("records %+v (%v), want the run closed as failed", recs, rerr)
	}
}

// TestAToolWithNoPluginIsRefusedNamingItAndNoRunIsWritten: a tool
// no plugin is installed for is refused with ErrNoPlugin, which names the tool
// and how to install one, and the journal holds nothing.
// Risk: a built-in adapter answering where release 1 ships none.
func TestAToolWithNoPluginIsRefusedNamingItAndNoRunIsWritten(t *testing.T) {
	t.Parallel()
	w, a, _ := auditWorld(t, someEntry, nil)
	call, _, _ := coreapi.PluginAuditEntries(a, nil)("golangci-lint")
	_, err := call(context.Background(), auditRequest("{}\n"))
	if err == nil || !errors.Is(err, coreapi.ErrNoPlugin) || !strings.Contains(err.Error(), "golangci-lint") {
		t.Errorf("err = %v, want ErrNoPlugin naming the tool", err)
	}
	if recs, _, _ := w.Ports().Journal.Records(context.Background(), a.Root); len(recs) != 0 {
		t.Errorf("records %+v, want none", recs)
	}
}

// TestAReadWithNoConfigSendsNoneAndRunsAtTheIsolatedFloor: the check of a tool
// whose config is not there asks
// the plugin for its entries with no config, prepares a scratch folder holding
// none at the isolated level, and the plan's run step runs there. A config
// whose name is no data format is still sandboxed.
// Risk: the floor of a config-less read moved without a row to hold it, so a
// change back to sandboxed passes every test and ends every such check not
// ready, and a change to the floor of a named config goes unseen.
func TestAReadWithNoConfigSendsNoneAndRunsAtTheIsolatedFloor(t *testing.T) {
	t.Parallel()
	var sent plugin.EntriesRequest
	oneStep := func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		if len(in.Have) > 0 {
			return plugin.PlanResponse{Final: true}, nil
		}
		return plugin.PlanResponse{Steps: []plugin.Step{{Run: &plugin.Run{Program: "ast-grep", Argv: []string{"scan"}}}}}, nil
	}
	_, a, _ := auditWorld(t, func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		sent = in
		return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "k", On: true}}}, nil
	}, oneStep)
	call, _, _ := coreapi.PluginAuditEntries(a, nil)("ast-grep")
	got, err := call(context.Background(), plugin.PlanRequest{Config: []plugin.Config{}, For: plugin.ForConfig, Target: plugin.TargetSource})
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("entries %+v, err %v, want the plugin's one entry, its run step allowed at isolated", got.Entries, err)
	}
	if len(sent.Config) != 0 {
		t.Errorf("the entries call was sent %+v, want no config", sent.Config)
	}
	sb := a.Sandbox.(*scriptedSandbox)
	if len(sb.prepared) != 1 || sb.prepared[0].Level != model.Isolated || len(sb.prepared[0].Config) != 0 {
		t.Errorf("scratch folders %+v, want one at isolated holding no config", sb.prepared)
	}
	// a named config that is no data format is held at sandboxed, which the
	// stand-in sandbox does not reach: its run step is refused.
	_, a2, _ := auditWorld(t, someEntry, oneStep)
	call2, _, _ := coreapi.PluginAuditEntries(a2, nil)("ast-grep")
	if _, err := call2(context.Background(), auditRequest2("a.js", "x\n")); err == nil {
		t.Error("a read of a.js ran its step at isolated, want it refused")
	}
}

func auditRequest2(name, text string) plugin.PlanRequest {
	return plugin.PlanRequest{Config: []plugin.Config{{Path: name, Data: &plugin.Bytes{Text: &text}}}, For: plugin.ForConfig, Target: plugin.TargetSource}
}

// TestAnAuditWithNoRootReadsNoToolAndNamesIt: a folder the check could not
// open hands no Root, and the check then reads no tool through a plugin: it
// makes no call and its reason is ErrNoPlugin's, naming the tool. The guard is
// the Root, not the Plugins port, which the entry points always set
// (arch.ports.5): even with a plugin installed and every port wired, no Root
// asks none.
// Risk: the guard dropped or moved back to a nil test of Plugins, so a
// rootless audit asks a plugin under a root it has no folder for.
func TestAnAuditWithNoRootReadsNoToolAndNamesIt(t *testing.T) {
	t.Parallel()
	_, wired, _ := auditWorld(t, someEntry, nil)
	wired.Root = ""
	for name, a := range map[string]coreapi.AuditPlugins{"zero": {}, "wired but no root": wired} {
		call, _, why := coreapi.PluginAuditEntries(a, nil)("golangci-lint")
		if call != nil {
			t.Errorf("%s: a call was made for a rootless audit, want none", name)
		}
		if !strings.Contains(why, "golangci-lint") || !strings.Contains(why, coreapi.ErrNoPlugin.Error()) {
			t.Errorf("%s: reason %q, want ErrNoPlugin's, naming the tool", name, why)
		}
	}
}
