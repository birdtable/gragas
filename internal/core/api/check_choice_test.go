package coreapi_test

import (
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestTheProjectCheckReadsAToolsConfigThroughThePluginThatServesTheTestedVersion:
// of two plugins of lint-a, a-lint serving <2.0.0 and b-lint serving 2.5.0 (second in List's order), the book tested at
// 2.5.0, the project check asks b-lint alone. A check that hands the audit no
// tested set keeps the first plugin of List's order, and asks a-lint.
// Risk: checkProject dropping the book's tested set at the audit's entries.
func TestTheProjectCheckReadsAToolsConfigThroughThePluginThatServesTheTestedVersion(t *testing.T) {
	t.Parallel()
	const tool = "lint-a"
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC))
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/sources.schema.json")
	book := w.Seed("/book", map[string]string{
		"rulebook.yaml":                 bookIdentity + "tested:\n  tools:\n    - {name: lint-a, version: 2.5.0}\n  min_engine: 0.1.0\n",
		"structure.yaml":                projStructure,
		"sources.yaml":                  projSources,
		"language/go/rules/errors.yaml": goRules(rule("go.errors.1", "active", tool, "k")),
		"language/ts/rules/style.yaml":  "format: 3\npart: ts\nconcern: style\nrules: []\n",
	})
	root := w.Seed("/proj", map[string]string{"a.json": "{}\n"})
	w.SeedTool(tool, model.Found{Path: "/tools/" + tool, Version: "2.5.0"})
	w.SeedStep(tool, plugin.Result{Kind: plugin.KindRun})
	asked := map[string]int{}
	for _, c := range []struct{ name, serves string }{{"a-lint", "<2.0.0"}, {"b-lint", ">=2.0.0 <3.0.0"}} {
		p := servedBy(w, root, c.name, "1.0.0", tool)
		p.Facts.ToolVersions = c.serves
		p.Facts.Programs = []plugin.Program{{Found: &plugin.ProgramFound{ID: tool, Versions: ">=1.0.0", VersionArgv: []string{"--version"}}}}
		p.OnPlan = func(plugin.PlanRequest) (plugin.PlanResponse, error) { return plugin.PlanResponse{Final: true}, nil }
		name := c.name
		p.OnEntries = func(plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			asked[name]++
			return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "k", On: true}}}, nil
		}
	}
	ports := w.Ports()
	ports.Sandbox = &scriptedSandbox{level: model.Isolated}
	ask := coreapi.ProjectAsk{
		Readers: coreapi.Readers{Rows: []coreapi.Reader{{Tool: tool, Files: []string{"a.json"}, Kind: "entries"}}},
		Plugins: coreapi.AuditPlugins{Plugins: ports.Plugins, Steps: ports.Steps, Sandbox: ports.Sandbox, Journal: ports.Journal, Clock: ports.Clock,
			Root: root, Command: "check", Engine: "v0.0.0-test"},
		Today: today, NamesFile: "/names",
	}
	l := project.Loaded{Root: root, Rulebook: book, PinPath: ".gragas/pin.yaml", Pin: model.Pin{Rulebook: "example", Source: "book", Parts: []string{"go"}, Format: 1}}
	_, err := checkIn(t, projWorld{w, book, root}, l, ask)
	if err != nil {
		t.Fatal(err)
	}
	if asked["a-lint"] != 0 || asked["b-lint"] != 1 {
		t.Errorf("asked %v; want b-lint alone", asked)
	}
}
