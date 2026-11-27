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

// rankOnlyAndPresetOnly installs for lint-a two plugins: a-lint, which has a
// preset and serves <2.0.0, and b-lint, which ranks and serves 2.5.0 (its
// manifest has no preset). The book is tested at 2.5.0, so b-lint is the one
// plugin of the tool, and it comes second in List's order, so a path that keeps
// the first plugin takes a-lint.
func rankOnlyAndPresetOnly(w *porttest.World, root model.Root) (a, b *porttest.StandinPlugin) {
	a = servedBy(w, root, "a-lint", "1.0.0", "lint-a")
	a.Facts.ToolVersions = "<2.0.0"
	a.Facts.Extends = true
	a.Facts.Preset = &plugin.ManifestPreset{Path: "standin.conf", Mark: true}
	b = servedBy(w, root, "b-lint", "1.0.0", "lint-a")
	b.Facts.ToolVersions = ">=2.0.0 <3.0.0"
	return a, b
}

// TestEveryPathTakesTheOnePluginThatServesTheTestedVersion: of a-lint
// (preset, serves <2.0.0) and b-lint (rank only, serves 2.5.0, second in
// List's order), the book tested at 2.5.0, the rank, the listing and the import
// each take b-lint, and a-lint is opened, closed and never asked. The preset is
// not written by a-lint in b-lint's place: the tool's preset ends unwritten,
// with the install line. The plant is a path that keeps the first plugin of
// List's order, or the first with a preset op, which asks or names a-lint.
func TestEveryPathTakesTheOnePluginThatServesTheTestedVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("rank", func(t *testing.T) {
		t.Parallel()
		w, root := listWorld(t, valueBook())
		a, b := rankOnlyAndPresetOnly(w, root)
		var aAsked, bAsked int
		a.OnRank = func(plugin.RankRequest) (plugin.RankResponse, error) {
			aAsked++
			return plugin.RankResponse{Words: []plugin.Word{plugin.WordSame}}, nil
		}
		b.OnRank = func(plugin.RankRequest) (plugin.RankResponse, error) {
			bAsked++
			return plugin.RankResponse{Words: []plugin.Word{plugin.WordSame}}, nil
		}
		if _, err := coreapi.ListConflicts(ctx, w.Ports(), root); err != nil || aAsked != 0 || bAsked != 1 {
			t.Errorf("rank: %v, a asked %d, b asked %d; want b-lint alone", err, aAsked, bAsked)
		}
	})

	t.Run("listing", func(t *testing.T) {
		t.Parallel()
		w, root := listWorld(t, boundBook("    - {name: lint-a, version: 2.5.0}\n"))
		rankOnlyAndPresetOnly(w, root)
		l, err := coreapi.ListTools(ctx, w.Ports(), root)
		if err != nil || len(l.Rows) == 0 {
			t.Fatalf("ListTools = %+v, %v", l, err)
		}
		for _, row := range l.Rows {
			if row.Tool == "lint-a" && (row.Plugin == nil || row.Plugin.Name != "b-lint") {
				t.Errorf("listing names %+v for lint-a; want b-lint", row.Plugin)
			}
		}
	})

	t.Run("preset", func(t *testing.T) {
		t.Parallel()
		files := map[string]string{
			"book/rulebook.yaml":                 bookIdentity + "tested:\n  tools:\n    - {name: lint-a, version: 2.5.0}\n  min_engine: 0.1.0\n",
			"book/language/go/rules/layout.yaml": "format: 3\npart: go\nconcern: layout\nrules: []\n",
		}
		w, root := presetWorldNoPlugin(t, presetRulesActive(), files)
		a, _ := rankOnlyAndPresetOnly(w, root)
		_, ref, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))
		if ref != nil || !errors.Is(err, coreapi.ErrNoPlugin) || !strings.Contains(err.Error(), "gragas plugin add <folder>") {
			t.Fatalf("preset: %v, %v; want not ready with the install line", ref, err)
		}
		if _, err := w.Ports().Tree.Read(root, presetFile); err == nil {
			t.Errorf("%s was written by a-lint in b-lint's place", presetFile)
		}
		for _, call := range a.Calls() {
			t.Errorf("a-lint was called: %v", call)
		}
	})

	t.Run("import", func(t *testing.T) {
		t.Parallel()
		const tool = "ast-grep"
		w, _ := ruleWorld(t)
		root := w.Seed("/repo", map[string]string{
			"book/rulebook.yaml":                 bookIdentity + "tested:\n  tools:\n    - {name: ast-grep, version: 2.5.0}\n  min_engine: 0.1.0\n",
			"book/structure.yaml":                ruleStructure,
			"book/language/go/rules/errors.yaml": bookRules, "book/language/go/guide.md": ruleGuide,
			"guide/console.md": "# Console\n\n## No console\n\nCode MUST NOT print to the console.\n",
			"sgconfig.yml":     "ruleDirs: [rules]\n",
		})
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
		ask := changeAsk(root, "import")
		ask.Run = "dry-run"
		_, _, err := coreapi.Import(ctx, ports, ask, model.ImportInput{Part: "go", Concern: "errors", GuideDir: "guide", Tool: tool, Target: model.TargetSource,
			Configs: []string{"sgconfig.yml"}, Guide: model.GuideRef{Slug: "g", Title: "G", URL: "https://example.org/g", Tag: "v1.0.0"}})
		if err != nil || asked["a-lint"] != 0 || asked["b-lint"] != 1 {
			t.Errorf("import: %v, asked %v; want b-lint alone", err, asked)
		}
	})
}

// TestTwoPluginsThatBothServeTheTestedVersionEndEveryPathInTheSameWords: where a-lint and b-lint both serve the tested 2.5.0, the rank, the
// listing, the preset and the audit entries each refuse, naming both, and no
// plugin is asked. The plant is a path that picks one of the two.
func TestTwoPluginsThatBothServeTheTestedVersionEndEveryPathInTheSameWords(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const want = "the plugins a-lint 1.0.0 and b-lint 1.0.0 both serve lint-a 2.5.0"
	both := func(w *porttest.World, root model.Root) {
		for _, name := range []string{"a-lint", "b-lint"} {
			p := servedBy(w, root, name, "1.0.0", "lint-a")
			p.Facts.ToolVersions = ">=2.0.0 <3.0.0"
			p.Facts.Extends = true
			p.Facts.Preset = &plugin.ManifestPreset{Path: "standin.conf", Mark: true}
		}
	}
	tested := &model.Tested{Tools: []model.Version{{Name: "lint-a", Version: "2.5.0"}}}

	w, root := listWorld(t, valueBook())
	both(w, root)
	_, rankErr := coreapi.ListConflicts(ctx, w.Ports(), root)

	w, root = listWorld(t, boundBook("    - {name: lint-a, version: 2.5.0}\n"))
	both(w, root)
	_, listErr := coreapi.ListTools(ctx, w.Ports(), root)

	w, root = presetWorldNoPlugin(t, presetRulesActive(), map[string]string{
		"book/rulebook.yaml":                 bookIdentity + "tested:\n  tools:\n    - {name: lint-a, version: 2.5.0}\n  min_engine: 0.1.0\n",
		"book/language/go/rules/layout.yaml": "format: 3\npart: go\nconcern: layout\nrules: []\n",
	})
	both(w, root)
	_, _, presetErr := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))

	call, _, _ := coreapi.PluginAuditEntries(coreapi.AuditPlugins{Plugins: w.Ports().Plugins, Root: root}, tested)("lint-a")
	_, auditErr := call(ctx, plugin.PlanRequest{})

	for name, err := range map[string]error{"rank": rankErr, "listing": listErr, "preset": presetErr} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v; want the words %q", name, err, want)
		}
	}
	if auditErr == nil || !strings.Contains(auditErr.Error(), want) {
		t.Errorf("audit: %v; want the words %q", auditErr, want)
	}
}
