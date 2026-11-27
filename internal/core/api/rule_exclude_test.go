package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// overlayIdentity is an overlay of fixture-parent 0.9.0, the pin the parent
// stand-in in testdata/parent answers.
const overlayIdentity = `format: 3
name: house
version: 1.0.0
kind: overlay
licences:
  rules: CC-BY-4.0
language: en
extends:
  rulebook: fixture-parent
  version: 0.9.0
owners: [team-a]
`

const (
	exclusionsPath = "book/exclusions.yaml"
	historyPath    = "book/history.jsonl"
	cacheDir       = "cache"
)

// parentPin is the pin of overlayIdentity.
func parentPin() model.Extends { return model.Extends{Rulebook: "fixture-parent", Version: "0.9.0"} }

// realContractSchemas are the contract's schema files as the Assets port
// holds them; ReadBundle reads its deprecations from them.
func realContractSchemas(t *testing.T) map[string]string {
	t.Helper()
	const dir = "../../host/assets/contract/schema/v1"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(path.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[contract.SchemaDir+"/v1/"+e.Name()] = string(b)
	}
	return out
}

// standIn is the bytes of the parent stand-in.
func standIn(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/parent/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// overlayWorld is ruleWorld's book as an overlay of fixture-parent 0.9.0,
// with exclusions as its exclusions.yaml ("" for none). cache is the files of
// the cache root, nil for a world with no cache root at all.
func overlayWorld(t *testing.T, exclusions string, cache map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(append(allSchemas(), "v3/exclusions.schema.json", "v1/document-bundle.schema.json")...)
	w.SeedAssets(realContractSchemas(t))
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	files := map[string]string{
		"book/rulebook.yaml": overlayIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": bookRules,
	}
	if exclusions != "" {
		files[exclusionsPath] = exclusions
	}
	root := w.Seed("repo", files)
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	if cache != nil {
		w.Seed(cacheDir, cache)
	}
	return w, root
}

// fetched is a cache that holds the parent stand-in at the pin.
func fetched(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{coreapi.ParentBundle(parentPin()): standIn(t)}
}

func cacheAsk(root model.Root, cmd string) model.ChangeAsk {
	a := changeAsk(root, cmd)
	a.Cache = cacheDir
	return a
}

func excludeReq(id string) model.ExcludeRule { return model.ExcludeRule{ID: id, Why: why()} }

func includeReq(id string) model.IncludeRule {
	return model.IncludeRule{ID: id, Why: model.Why{New: &model.NewDecision{Slug: "decide-back", Title: "Back", Text: "It holds again."}}}
}

// TestParentBundleNamesTheRulebookByItsDigestNeverItsText: a rulebook that
// is a location, or that climbs, is one folder of hex digits in the cache.
// Risk: a pin's text read as a path, so a parent named ../../x reads a file
// outside the releases.
func TestParentBundleNamesTheRulebookByItsDigestNeverItsText(t *testing.T) {
	t.Parallel()
	for _, rb := range []string{"fixture-parent", "https://example.org/a/b.git", "../../../etc", "a/../../b"} {
		p := coreapi.ParentBundle(model.Extends{Rulebook: rb, Version: "0.9.0"})
		parts := strings.Split(p, "/")
		if len(parts) != 5 || parts[0] != "releases" || parts[1] != "pinned" || len(parts[2]) != 16 || strings.Trim(parts[2], "0123456789abcdef") != "" || parts[3] != "0.9.0" || parts[4] != "bundle.json" {
			t.Errorf("%q: %s", rb, p)
		}
	}
	if coreapi.ParentBundle(parentPin()) == coreapi.ParentBundle(model.Extends{Rulebook: "fixture-parent2", Version: "0.9.0"}) {
		t.Error("two rulebooks share a folder")
	}
	// the journeys seed the cache at this path, written out (parentRelease)
	if got := coreapi.ParentBundle(parentPin()); got != "releases/pinned/932251d4ef17cc0b/0.9.0/bundle.json" {
		t.Errorf("fixture-parent 0.9.0 is read from %s; move the journeys' parentRelease with it", got)
	}
}

// TestExcludeWritesTheRecordAndAHistoryLineAndASecondRunHasNothingToDo is
// the done-when's first line over the parent stand-in.
func TestExcludeWritesTheRecordAndAHistoryLineAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	w, root := overlayWorld(t, "", fetched(t))
	ctx := context.Background()
	ask := cacheAsk(root, "gragas rule exclude std.errors.4")
	c, ref, err := coreapi.RuleExclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: ask, Contracts: w.Ports().Schema}, excludeReq("std.errors.4"))
	if err != nil || ref != nil {
		t.Fatalf("ref = %v, err = %v", ref, err)
	}
	shown, ok := c.Preview(ctx)
	if !ok || read(t, w, root, exclusionsPath) != "" {
		t.Fatalf("preview ok=%v, or the dry run wrote", ok)
	}
	if !slicesHas(shown.Plan.Sections, "projects that pin house see std.errors.4 as excluded") {
		t.Errorf("the plan does not say what a consumer sees: %+v", shown.Plan.Sections)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	ex := read(t, w, root, exclusionsPath)
	if !strings.HasPrefix(ex, "format: 3\nexclusions:\n  - overlay: house\n    rule: std.errors.4\n    number: 1\n    decision: decide-it\n") || !strings.Contains(ex, `when: "2026-11-08T10:00:00Z"`) {
		t.Errorf("exclusions:\n%s", ex)
	}
	h := read(t, w, root, historyPath)
	if rows(h) != 1 || !strings.Contains(h, `"kind":"exclude"`) || !strings.Contains(h, `"entity":"exclusion:house/std.errors.4/1"`) || strings.Contains(h, `"before"`) {
		t.Errorf("history:\n%s", h)
	}
	again, ref, err := coreapi.RuleExclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: runTwo(ask), Contracts: w.Ports().Schema}, model.ExcludeRule{ID: "std.errors.4"})
	if out := applied(t, again, ref, err); out.Word != model.Done || !out.Nothing {
		t.Errorf("the second run ended %q nothing=%v (%v)", out.Word, out.Nothing, out.Cause)
	}
	if read(t, w, root, exclusionsPath) != ex || read(t, w, root, historyPath) != h {
		t.Error("the second run wrote")
	}
}

// TestIncludeLiftsTheExclusionAndKeepsTheRecord: lifted is added under the
// record, every line of it stays, and the history line takes it from active
// to retired.
func TestIncludeLiftsTheExclusionAndKeepsTheRecord(t *testing.T) {
	t.Parallel()
	w, root := overlayWorld(t, "", fetched(t))
	ctx := context.Background()
	c, ref, err := coreapi.RuleExclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(root, "gragas rule exclude std.errors.4"), Contracts: w.Ports().Schema}, excludeReq("std.errors.4"))
	if out := applied(t, c, ref, err); out.Word != model.Done {
		t.Fatalf("exclude: %+v", out)
	}
	excluded := read(t, w, root, exclusionsPath)
	ask := cacheAsk(root, "gragas rule include std.errors.4")
	ask.Run = "run-3"
	c, ref, err = coreapi.RuleInclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: ask, Contracts: w.Ports().Schema}, includeReq("std.errors.4"))
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("include: %+v", out)
	}
	lifted := read(t, w, root, exclusionsPath)
	want := excluded + "    lifted:\n      decision: decide-back\n      who: " + testWho + "\n      when: \"2026-11-08T10:00:00Z\"\n"
	if lifted != want {
		t.Errorf("exclusions:\n%s\nwant\n%s", lifted, want)
	}
	h := read(t, w, root, historyPath)
	if rows(h) != 2 || !strings.Contains(h, `"kind":"include"`) || !strings.Contains(h, `"before":{"status":"active"}`) || !strings.Contains(h, `"after":{"status":"retired"}`) {
		t.Errorf("history:\n%s", h)
	}
}

// TestReasonFindsAParentNotReadyUnderAWrap: a parent error still matches
// ErrNotReady and keeps its bare sentence when a caller wraps it.
func TestReasonFindsAParentNotReadyUnderAWrap(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("plan: %w", &coreapi.ParentError{Msg: "parent not fetched: p 1.0.0"})
	if !errors.Is(err, coreapi.ErrNotReady) {
		t.Errorf("%v does not match ErrNotReady", err)
	}
	if got := coreapi.Reason(err); got != "parent not fetched: p 1.0.0" {
		t.Errorf("Reason = %q, want the parent's sentence alone", got)
	}
}

// TestWithoutTheParentInTheCacheTheRunIsNotReadyNamingThePin: no cache, a
// cache without the release, and a pin whose version is not one all end not
// ready, and nothing is planned. Seeding the release makes the same command
// succeed.
func TestWithoutTheParentInTheCacheTheRunIsNotReadyNamingThePin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, tc := range map[string]struct {
		cache map[string]string
		ask   func(model.Root) model.ChangeAsk
	}{
		"no cache folder":     {nil, func(r model.Root) model.ChangeAsk { return changeAsk(r, "gragas rule exclude std.errors.4") }},
		"no release in cache": {map[string]string{"releases/other": "x"}, func(r model.Root) model.ChangeAsk { return cacheAsk(r, "gragas rule exclude std.errors.4") }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root := overlayWorld(t, "", tc.cache)
			c, ref, err := coreapi.RuleExclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: tc.ask(root), Contracts: w.Ports().Schema}, excludeReq("std.errors.4"))
			if c != nil || ref != nil || !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "parent not fetched: fixture-parent 0.9.0") {
				t.Fatalf("change=%v ref=%v err=%v", c, ref, err)
			}
			if got := coreapi.Reason(err); !strings.HasPrefix(got, "parent not fetched: fixture-parent 0.9.0 ") {
				t.Errorf("Reason = %q, want the sentence without its closing word", got)
			}
			_, _, err = coreapi.RuleInclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: tc.ask(root), Contracts: w.Ports().Schema}, includeReq("std.errors.3"))
			if !errors.Is(err, coreapi.ErrNotReady) {
				t.Errorf("include: %v", err)
			}
		})
	}
	w, root := overlayWorld(t, "", fetched(t))
	c, ref, err := coreapi.RuleExclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(root, "gragas rule exclude std.errors.4"), Contracts: w.Ports().Schema}, excludeReq("std.errors.4"))
	if out := applied(t, c, ref, err); out.Word != model.Done {
		t.Errorf("seeded: %+v", out)
	}
}

// TestAPinThatIsNotAVersionIsNeverAPath: the version is a folder of the cache
// path, so one that climbs or is empty ends not ready before anything is read.
func TestAPinThatIsNotAVersionIsNeverAPath(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"../../x", "0.9", "0..9.0", ".0.9.0", "0.9.0.", "0.9.0/..", "", "1.2.3-rc.1"} {
		ident := strings.Replace(overlayIdentity, "version: 0.9.0", "version: \""+v+"\"", 1)
		w, root := overlayWorld(t, "", map[string]string{})
		w.Seed("repo", map[string]string{"book/rulebook.yaml": ident, "book/structure.yaml": ruleStructure, "book/language/go/rules/errors.yaml": bookRules})
		w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
		_, _, err := coreapi.RuleExclude(context.Background(), w.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(root, "gragas rule exclude std.errors.4"), Contracts: w.Ports().Schema}, excludeReq("std.errors.4"))
		if !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "which is not a version") {
			t.Errorf("version %q: %v", v, err)
		}
	}
}

// TestABundleThatIsNotOneIsAnErrorAndPlansNothing: bytes in the cache are
// less-trusted data; ReadBundle holds them to the contract before a rule of
// them is read.
func TestABundleThatIsNotOneIsAnErrorAndPlansNothing(t *testing.T) {
	t.Parallel()
	w, root := overlayWorld(t, "", map[string]string{coreapi.ParentBundle(parentPin()): "{not json"})
	c, ref, err := coreapi.RuleExclude(context.Background(), w.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(root, "gragas rule exclude std.errors.4"), Contracts: w.Ports().Schema}, excludeReq("std.errors.4"))
	if c != nil || ref != nil || err == nil || errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "the bundle of the parent fixture-parent 0.9.0 in the releases cache") {
		t.Errorf("change=%v ref=%v err=%v", c, ref, err)
	}
	fw, froot := overlayWorld(t, "", fetched(t))
	if _, _, err := coreapi.RuleExclude(context.Background(), fw.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(froot, "gragas rule exclude std.errors.4"), Contracts: nil}, excludeReq("std.errors.4")); err == nil || !strings.Contains(err.Error(), "reads no contract document") {
		t.Errorf("a build with no contract schemas read the bundle: %v", err)
	}
}

// TestTheRefusalsOfExcludeAndIncludeEndOnTheirWords: a base is refused, a
// rule the parent lacks is refused, include of a rule not excluded is usage,
// and include of a rule the parent retired is not ready.
func TestTheRefusalsOfExcludeAndIncludeEndOnTheirWords(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	open5 := "format: 3\nexclusions:\n  - overlay: house\n    rule: std.errors.5\n    number: 1\n    decision: decide-it\n    who: team-a\n    when: \"2026-11-01T10:00:00Z\"\n"
	w, root := overlayWorld(t, open5, fetched(t))
	for name, tc := range map[string]struct {
		run  func() (*coreapi.Change, *model.Refusal, error)
		word model.Word
		says string
	}{
		"exclude a rule the parent lacks": {func() (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleExclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(root, "x"), Contracts: w.Ports().Schema}, excludeReq("std.errors.9"))
		}, model.Refused, "not in the parent fixture-parent 0.9.0"},
		"include a rule not excluded": {func() (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleInclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(root, "x"), Contracts: w.Ports().Schema}, includeReq("std.errors.1"))
		}, model.Usage, "is not excluded by house"},
		"include a rule the parent retired": {func() (*coreapi.Change, *model.Refusal, error) {
			return coreapi.RuleInclude(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: cacheAsk(root, "x"), Contracts: w.Ports().Schema}, includeReq("std.errors.5"))
		}, model.NotReady, "retired in fixture-parent 0.9.0"},
	} {
		c, ref, err := tc.run()
		if err != nil || c != nil || ref == nil || ref.Word() != tc.word || ref.Hint() == "" || !strings.Contains(ref.Observed(), tc.says) {
			t.Errorf("%s: change=%v ref=%v err=%v", name, c, ref, err)
		}
	}
	bw, broot := ruleWorld(t)
	c, ref, err := coreapi.RuleExclude(ctx, bw.Ports(), coreapi.OverlayAsk{ChangeAsk: changeAsk(broot, "x"), Contracts: bw.Ports().Schema}, excludeReq("std.errors.1"))
	if err != nil || c != nil || ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), "only an overlay can exclude") {
		t.Errorf("a base: change=%v ref=%v err=%v", c, ref, err)
	}
}

func slicesHas(sections []model.Section, part string) bool {
	for _, s := range sections {
		for _, l := range s.Lines {
			if strings.Contains(l, part) {
				return true
			}
		}
	}
	return false
}
