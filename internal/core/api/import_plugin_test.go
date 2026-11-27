package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// pluginImport is one import with a plugin installed for the tool: what it
// answered, the scratch folders the read asked for, the plugin, and the
// journal as the read left it.
type pluginImport struct {
	change  *coreapi.Change
	sandbox *scriptedSandbox
	plugin  *porttest.StandinPlugin
	records []model.Record
	err     error
}

// importThrough is a dry-run import of the guide in guide/ with the config
// named config (whose text is text) of ast-grep into the test book, through a
// stand-in plugin installed for the tool that plans as plan says (nil plans
// nothing) and answers each entries call as entries says. The sandbox reaches
// the level reached.
func importThrough(t *testing.T, config, text string, reached model.Level, plan func(plugin.PlanRequest) (plugin.PlanResponse, error), entries func(plugin.EntriesRequest) (plugin.EntriesResponse, error)) pluginImport {
	t.Helper()
	return importThroughCtx(t, context.Background(), [2]string{config, text}, reached, plan, entries)
}

// importThroughCtx is importThrough with the ctx the import is called with and
// the config as its name and its text.
func importThroughCtx(t *testing.T, ctx context.Context, file [2]string, reached model.Level, plan func(plugin.PlanRequest) (plugin.PlanResponse, error), entries func(plugin.EntriesRequest) (plugin.EntriesResponse, error)) pluginImport {
	t.Helper()
	return importRun(t, ctx, file, reached, standinAnswers{plan: plan, entries: entries})
}

// importThroughFacts is importThrough with the config as its name and its text,
// and facts given the stand-in's manifest to change before it is installed.
func importThroughFacts(t *testing.T, file [2]string, reached model.Level, facts func(*plugin.Manifest), plan func(plugin.PlanRequest) (plugin.PlanResponse, error), entries func(plugin.EntriesRequest) (plugin.EntriesResponse, error)) pluginImport {
	t.Helper()
	return importRun(t, context.Background(), file, reached, standinAnswers{facts: facts, plan: plan, entries: entries})
}

// standinAnswers is how the stand-in an import runs through is made: facts,
// when it is not nil, changes its manifest before it is installed; plan and
// entries answer its calls.
type standinAnswers struct {
	facts   func(*plugin.Manifest)
	plan    func(plugin.PlanRequest) (plugin.PlanResponse, error)
	entries func(plugin.EntriesRequest) (plugin.EntriesResponse, error)
}

// importRun is the import importThroughCtx and importThroughFacts describe.
func importRun(t *testing.T, ctx context.Context, file [2]string, reached model.Level, s standinAnswers) pluginImport {
	t.Helper()
	const tool = "ast-grep"
	facts, plan, entries := s.facts, s.plan, s.entries
	config, text := file[0], file[1]
	w, _ := ruleWorld(t)
	root := w.Seed("/repo", map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": bookRules, "book/language/go/guide.md": ruleGuide,
		"guide/console.md": "# Console\n\n## No console\n\nCode MUST NOT print to the console.\n",
		config:             text,
	})
	p := porttest.NewStandinPlugin()
	p.Facts.Tool = tool
	p.Facts.Programs = []plugin.Program{{Found: &plugin.ProgramFound{ID: tool, Versions: ">=1.0.0", VersionArgv: []string{"--version"}}}}
	if facts != nil {
		facts(&p.Facts)
	}
	w.SeedTool(tool, model.Found{Path: "/tools/" + tool, Version: "1.1.0"})
	w.SeedStep(tool, plugin.Result{Kind: plugin.KindRun})
	if plan == nil {
		plan = func(plugin.PlanRequest) (plugin.PlanResponse, error) { return plugin.PlanResponse{Final: true}, nil }
	}
	p.OnPlan = plan
	p.OnEntries = entries
	w.PluginsOf().Install(string(root), model.PluginRef{Name: tool, Version: "1.0.0"}, p)
	ports := w.Ports()
	sb := &scriptedSandbox{level: reached}
	ports.Sandbox = sb
	ask := changeAsk(root, "import")
	ask.Run = "dry-run"
	c, _, err := coreapi.Import(ctx, ports, ask,
		model.ImportInput{Part: "go", Concern: "errors", GuideDir: "guide", Tool: tool, Target: model.TargetSource, Configs: []string{config},
			Guide: model.GuideRef{Slug: "g", Title: "G", URL: "https://example.org/g", Tag: "v1.0.0"}})
	recs, _, rerr := ports.Journal.Records(context.WithoutCancel(ctx), root)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return pluginImport{change: c, sandbox: sb, plugin: p, records: recs, err: err}
}

func someEntry(plugin.EntriesRequest) (plugin.EntriesResponse, error) {
	return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "k", On: true}}}, nil
}

// TestAnImportBindsTheEntriesOfTheToolsPluginWithTheirSeverity: the entries
// of a config are the entries call of the plugin installed for the tool, over
// a scratch folder holding that config and nothing else, and a rule whose key
// names a draft of the guide is grandfathered at the severity the plugin gave
// it. An entry the plugin says is off is not imported. The plugin is opened
// once and closed once.
// Risk: an import that reads a config itself where a plugin is installed, or
// binds a rule at another severity.
func TestAnImportBindsTheEntriesOfTheToolsPluginWithTheirSeverity(t *testing.T) {
	t.Parallel()
	warn := plugin.SeverityWarning
	var sent plugin.EntriesRequest
	got := importThrough(t, "sgconfig.yml", "ruleDirs: [rules]\n", model.Isolated, nil, func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		sent = in
		return plugin.EntriesResponse{Entries: []plugin.Entry{
			{Key: "no-console", On: true, Severity: &warn},
			{Key: "deep-one", On: true},
			{Key: "off-one", On: false},
		}}, nil
	})
	if got.err != nil {
		t.Fatal(got.err)
	}
	out, _ := got.change.Preview(context.Background())
	screen := fmt.Sprintf("%+v", out.Plan)
	for _, want := range []string{"ast-grep", "no-console", "warning", "deep-one"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the plan lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "off-one") {
		t.Errorf("the plan holds off-one, an entry the plugin says is off:\n%s", screen)
	}
	if len(sent.Config) != 1 || sent.Config[0].Path != "sgconfig.yml" || sent.Config[0].Data == nil || *sent.Config[0].Data.Text != "ruleDirs: [rules]\n" {
		t.Errorf("the entries call was sent %+v, want the one config as text", sent.Config)
	}
	if sb := got.sandbox; len(sb.prepared) != 1 || len(sb.released) != 1 || len(sb.prepared[0].Config) != 1 {
		t.Errorf("scratch folders %+v released %d, want one holding the config alone", sb.prepared, len(sb.released))
	}
	if opened, closed := got.plugin.Opened(); opened != 1 || closed != 1 {
		t.Errorf("the plugin was opened %d and closed %d times, want once each", opened, closed)
	}
}

// TestTheFloorOfAConfigReadIsHeldByTheStepRunner: the level asked of
// the sandbox is not the guard, because a prepared folder may reach a lower
// one; the floor bound to the steps is. A plugin plans one step over each
// config, and the sandbox reaches isolated only: a step over a config that
// may be code (a.js, no extension) is refused, one over data (a.json, a.YAML)
// runs.
// Risk: a read of a config the tool loads as code that runs at the level of data.
func TestTheFloorOfAConfigReadIsHeldByTheStepRunner(t *testing.T) {
	t.Parallel()
	oneStep := func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		if len(in.Have) > 0 {
			return plugin.PlanResponse{Final: true}, nil
		}
		return plugin.PlanResponse{Steps: []plugin.Step{{Run: &plugin.Run{Program: "ast-grep", Argv: []string{"scan"}}}}}, nil
	}
	for name, runs := range map[string]bool{"a.json": true, "a.YAML": true, "a.js": false, "noext": false} {
		got := importThrough(t, name, "x\n", model.Isolated, oneStep, someEntry)
		switch {
		case runs && got.err != nil:
			t.Errorf("%s: err = %v, want the step to run at isolated", name, got.err)
		case !runs && (got.err == nil || !strings.Contains(got.err.Error(), "scratch folder reached isolated")):
			t.Errorf("%s: err = %v, want the step refused: it runs at sandboxed and the folder reached isolated", name, got.err)
		}
	}
}

// TestAManifestWhoseConfigLoadsCodeRaisesEveryConfigReadToSandboxed: a data config can load code (a plugins key), which the engine
// cannot see, so a plugin whose manifest says config_loads_code: true has every
// config of its tool read at sandboxed, whatever the extension. The sandbox is
// asked for sandboxed, and a step over a.json is refused at a folder that reached
// isolated, as one over a.js is. A folder that reached sandboxed runs it. The flag
// set to false is the flag left out (the floor test above).
// Risk: a data config whose plugins key loads code at the level of data.
func TestAManifestWhoseConfigLoadsCodeRaisesEveryConfigReadToSandboxed(t *testing.T) {
	t.Parallel()
	oneStep := func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		if len(in.Have) > 0 {
			return plugin.PlanResponse{Final: true}, nil
		}
		return plugin.PlanResponse{Steps: []plugin.Step{{Run: &plugin.Run{Program: "ast-grep", Argv: []string{"scan"}}}}}, nil
	}
	loads := func(v bool) func(*plugin.Manifest) {
		return func(m *plugin.Manifest) { m.ConfigLoadsCode = &v }
	}
	for _, name := range []string{"a.json", "a.YAML", "a.yml", "a.js", "noext"} {
		got := importThroughFacts(t, [2]string{name, "x\n"}, model.Isolated, loads(true), oneStep, someEntry)
		if got.err == nil || !strings.Contains(got.err.Error(), "scratch folder reached isolated") {
			t.Errorf("%s, config_loads_code: err = %v, want the step refused: it runs at sandboxed and the folder reached isolated", name, got.err)
		}
		if sb := got.sandbox; len(sb.prepared) != 1 || sb.prepared[0].Level != model.Sandboxed {
			t.Errorf("%s, config_loads_code: the sandbox was asked %+v, want one folder at sandboxed", name, sb.prepared)
		}
		if got := importThroughFacts(t, [2]string{name, "x\n"}, model.Sandboxed, loads(true), oneStep, someEntry); got.err != nil {
			t.Errorf("%s, config_loads_code: err = %v at a folder that reached sandboxed, want the step to run", name, got.err)
		}
	}
	for _, name := range []string{"a.json", "a.YAML"} {
		got := importThroughFacts(t, [2]string{name, "x\n"}, model.Isolated, loads(false), oneStep, someEntry)
		if got.err != nil {
			t.Errorf("%s, config_loads_code false: err = %v, want the step to run at isolated, as with the flag left out", name, got.err)
		}
		if sb := got.sandbox; len(sb.prepared) != 1 || sb.prepared[0].Level != model.Isolated {
			t.Errorf("%s, config_loads_code false: the sandbox was asked %+v, want one folder at isolated", name, sb.prepared)
		}
	}
}

// TestAReadOfAConfigIsARunOfItsOwnThatBeginsBeforeItsFirstStart: a dry-run
// import over a plugin journals its intent, then each start, then its close,
// all under one real run id and not the label "dry-run", so the host reads no
// start as torn and no run stays open. A failed read closes its run too.
// Risk: a start written before the run's intent, or an open run left by a plan
// nobody applied.
func TestAReadOfAConfigIsARunOfItsOwnThatBeginsBeforeItsFirstStart(t *testing.T) {
	t.Parallel()
	oneStep := func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		if len(in.Have) > 0 {
			return plugin.PlanResponse{Final: true}, nil
		}
		return plugin.PlanResponse{Steps: []plugin.Step{{Run: &plugin.Run{Program: "ast-grep", Argv: []string{"scan"}}}}}, nil
	}
	for name, entries := range map[string]func(plugin.EntriesRequest) (plugin.EntriesResponse, error){
		"a read": someEntry,
		"a failed read": func(plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			return plugin.EntriesResponse{}, errors.New("the config does not load")
		},
	} {
		got := importThrough(t, "a.json", "{}\n", model.Isolated, oneStep, entries)
		var kinds []string
		for _, r := range got.records {
			kinds = append(kinds, r.Kind)
			if r.Run == "dry-run" || r.Run != got.records[0].Run {
				t.Errorf("%s: a record of kind %s is of run %q, want one real run id (%q)", name, r.Kind, r.Run, got.records[0].Run)
			}
		}
		if len(kinds) < 3 || kinds[0] != model.KindIntent || kinds[len(kinds)-1] != model.KindClose || !strings.Contains(strings.Join(kinds, " "), model.KindStart) {
			t.Errorf("%s: the run's records are %v, want an intent, at least one start, and a close", name, kinds)
		}
		if name == "a failed read" && (got.err == nil || !strings.Contains(string(got.records[len(got.records)-1].Detail), "failed")) {
			t.Errorf("%s: err = %v, close %s, want the read failed and closed so", name, got.err, got.records[len(got.records)-1].Detail)
		}
	}
}

// TestAnInterruptedReadStillClosesItsRun: a read whose ctx
// is cancelled while the plugin plans fails, and its close is written all the
// same, since the host's journal refuses a record on a cancelled ctx and an
// open run refuses validate until it is recovered.
// Risk: an intent and its starts with no close after Ctrl+C.
func TestAnInterruptedReadStillClosesItsRun(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plan := func(plugin.PlanRequest) (plugin.PlanResponse, error) {
		cancel()
		return plugin.PlanResponse{}, errors.New("interrupted")
	}
	got := importThroughCtx(t, ctx, [2]string{"a.json", "{}\n"}, model.Isolated, plan, someEntry)
	if got.err == nil || len(got.records) < 2 || got.records[len(got.records)-1].Kind != model.KindClose {
		t.Errorf("err = %v, records %v, want the read failed and its close written last", got.err, got.records)
	}
}
