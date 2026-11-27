package coreapi_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// TestARankedRebindIsPlannedPreviewedAndAppliedWithTheRankersOfEachReading:
// a rebind of go.errors.1's
// lint-a value, true to false, that the plugin of lint-a ranks tighter or the
// same needs no decision at the first reading, at the dry run and at the
// apply, and the apply writes the value; each reading opens the plugin to
// rank, asks it once and closes it before the next. The weaker word wants a decision at
// the first reading. The plant is rankers made once for the request and
// closed after the first reading, which ranks the dry run's value unknown,
// so the dry run fails on a loosening.
func TestARankedRebindIsPlannedPreviewedAndAppliedWithTheRankersOfEachReading(t *testing.T) {
	t.Parallel()
	rules := strings.Replace(presetRules, "key: wrap-errors\n        value: true\n        severity: error\n",
		"key: wrap-errors\n        value: true\n        severity: error\n        rung: 1\n        run_point: commit\n", 1)
	for _, word := range []plugin.Word{plugin.WordTighter, plugin.WordSame} {
		w, root := presetWorldWith(t, rules)
		p, asked := ranking(w, root, word)
		ctx := context.Background()
		value := any(false)
		req := model.RebindRule{ID: "go.errors.1", Tool: "lint-a", Target: "source", Key: "wrap-errors", Value: &value}
		c, ref, err := coreapi.Rebind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: changeAsk(root, "gragas rebind go.errors.1")}, req)
		if err != nil || ref != nil {
			t.Fatalf("%s: rebind: %v, %v", word, ref, err)
		}
		// each reading opens the plugin twice, once for its preset
		// (presetPlugins) and once to rank, and closes both before it ends
		readings := func(at string, n int) {
			t.Helper()
			if opened, closed := p.Opened(); opened != 2*n || closed != 2*n || len(asked()) != n {
				t.Errorf("%s: after %s opened %d, closed %d, asked %d; want %d opened and closed and %d asked", word, at, opened, closed, len(asked()), 2*n, n)
			}
		}
		readings("the first reading", 1)
		shown, ok := c.Preview(ctx)
		if !ok {
			t.Fatalf("%s: the dry run: %+v", word, shown)
		}
		readings("the dry run", 2)
		out := c.Apply(ctx, "")
		if out.Word != model.Done || out.Nothing {
			t.Fatalf("%s: apply ended %q (%v)", word, out.Word, out.Cause)
		}
		if opened, closed := p.Opened(); opened != 2*len(asked()) || opened != closed || len(asked()) < 3 {
			t.Errorf("%s: after the apply opened %d, closed %d, asked %d; want the apply's readings each to open, ask and close", word, opened, closed, len(asked()))
		}
		if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); !strings.Contains(rules, "key: wrap-errors\n        value: false") {
			t.Errorf("%s: the value is not written:\n%s", word, rules)
		}
	}

	w, root := presetWorldWith(t, rules)
	ranking(w, root, plugin.WordWeaker)
	value := any(false)
	req := model.RebindRule{ID: "go.errors.1", Tool: "lint-a", Target: "source", Key: "wrap-errors", Value: &value}
	_, ref, err := coreapi.Rebind(context.Background(), w.Ports(), coreapi.OverlayAsk{ChangeAsk: changeAsk(root, "gragas rebind go.errors.1")}, req)
	if err != nil || ref == nil || !strings.Contains(ref.Observed(), "decision") {
		t.Errorf("a weaker value: %v, %v; want refused for a decision", ref, err)
	}
}

// TestTheRankingPluginOfAToolIsTheOneThatServesTheTestedVersion: of two plugins of lint-a, tested at
// 2.5.0, the one whose range serves 2.5.0 ranks, at 2.5.0, wherever List puts
// it, and the other is opened, closed and never asked; two that both serve
// 2.5.0 end the read, naming both. The plant is keeping the first in List's
// order, which asks a-lint at the end of its own range.
func TestTheRankingPluginOfAToolIsTheOneThatServesTheTestedVersion(t *testing.T) {
	t.Parallel()
	for _, served := range []string{"<2.0.0", ">=2.0.0 <3.0.0"} {
		w, root := listWorld(t, valueBook())
		a := servedBy(w, root, "a-lint", "1.0.0", "lint-a")
		a.Facts.ToolVersions = served
		var aAsked []plugin.RankRequest
		a.OnRank = func(in plugin.RankRequest) (plugin.RankResponse, error) {
			aAsked = append(aAsked, in)
			return plugin.RankResponse{Words: []plugin.Word{plugin.WordSame}}, nil
		}
		b := servedBy(w, root, "b-lint", "1.0.0", "lint-a")
		b.Facts.ToolVersions = ">=2.0.0 <3.0.0"
		var bAsked []plugin.RankRequest
		b.OnRank = func(in plugin.RankRequest) (plugin.RankResponse, error) {
			bAsked = append(bAsked, in)
			return plugin.RankResponse{Words: []plugin.Word{plugin.WordSame}}, nil
		}
		_, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
		if served == "<2.0.0" {
			if err != nil || len(aAsked) != 0 || len(bAsked) != 1 || bAsked[0].ToolVersion != "2.5.0" {
				t.Errorf("a-lint serves %s: %v, a asked %+v, b asked %+v; want b alone, at 2.5.0", served, err, aAsked, bAsked)
			}
		} else if err == nil || !strings.Contains(err.Error(), "the plugins a-lint 1.0.0 and b-lint 1.0.0 both serve lint-a 2.5.0") ||
			len(aAsked)+len(bAsked) != 0 {
			t.Errorf("both serve 2.5.0: %v, asked %d and %d; want the read ended, naming both, and neither asked", err, len(aAsked), len(bAsked))
		}
		for name, p := range map[string]interface{ Opened() (int, int) }{"a-lint": a, "b-lint": b} {
			if opened, closed := p.Opened(); opened != 1 || closed != 1 {
				t.Errorf("a-lint serves %s: %s opened %d, closed %d; want once each", served, name, opened, closed)
			}
		}
	}
}

// TestAMissingPluginDropsTheUnrankedValuesOfItsOwnToolAlone: with golangci-lint served by a plugin that
// gives no word on errorlint's value and lint-z served by none, the record
// that weakens errorlint's value is still a finding, while the one of lint-z
// is not compared and the notice names lint-z. The plant is dropping every
// unranked value once any tool is unserved, which loses the errorlint one.
func TestAMissingPluginDropsTheUnrankedValuesOfItsOwnToolAlone(t *testing.T) {
	t.Parallel()
	overlay := strings.Replace(bookIdentity, "kind: base", "kind: overlay", 1) + "extends:\n  rulebook: std-base\n  version: 2.1.0\n"
	lends := func(id, tool, key string) model.Rule {
		return model.Rule{ID: model.RuleID(id), Status: model.Active, Bindings: []model.Binding{{
			Tool: tool, Target: "source", Key: key, Value: "strict", Severity: "error", Rung: 1, RunPoint: "commit"}}}
	}
	parent := format.Composed{Parent: &format.Foreign{Rulebook: "std-base", Version: "2.1.0", Parts: []string{"std"},
		Rules: []model.Rule{lends("std.errors.4", "golangci-lint", "errorlint"), lends("std.errors.6", "lint-z", "zcheck")}}}
	record := func(id, tool, key string) string {
		return "subject: " + id + "\ntool: " + tool + "\ntarget: source\nkey: " + key + "\nvalue: warn\nseverity: error\nrung: 1\nrun_point: commit\n"
	}
	w, root := checkersWorld(t, overlay, map[string]string{
		"bindings/std-errors-4.yaml": record("std.errors.4", "golangci-lint", "errorlint"),
		"bindings/std-errors-6.yaml": record("std.errors.6", "lint-z", "zcheck"),
	})
	p := servedBy(w, root, "go-lint", "1.0.0", "golangci-lint")
	p.OnRank = func(plugin.RankRequest) (plugin.RankResponse, error) {
		return plugin.RankResponse{Words: []plugin.Word{plugin.WordUnknown}}, nil
	}
	r, err := coreapi.LoadBookComposed(t.Context(), w.Ports(), root, parent)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, f := range r.Findings {
		if strings.Contains(f.Msg, "at its value") {
			files = append(files, f.File)
		}
	}
	if !slices.Equal(files, []string{"book/bindings/std-errors-4.yaml"}) {
		t.Errorf("value findings in %v; want the served tool's alone: %+v", files, r.Findings)
	}
	if !slices.ContainsFunc(r.Notices, func(n model.Notice) bool {
		return strings.Contains(n.Msg, "the binding records were not all compared") && strings.Contains(n.Msg, "no plugin is installed for lint-z")
	}) {
		t.Errorf("no notice names lint-z: %+v", r.Notices)
	}
}
