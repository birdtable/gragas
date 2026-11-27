package coreapi_test

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const presetRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
    pairs:
      - number: 1
        target: source
    bindings:
      - tool: lint-a
        target: source
        key: wrap-errors
        value: true
        severity: error
  - id: go.errors.2
    statement: Log an error once.
    strength: may
    status: proposed
    origin: authored
    sources:
      - ref: effective-go
        backs: rule
`

func presetWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	return presetWorldWith(t, presetRules)
}

func presetWorldWith(t *testing.T, rules string) (*porttest.World, model.Root) {
	t.Helper()
	return presetWorldFiles(t, rules, nil)
}

// presetWorldFiles is the world with more files in the repository.
func presetWorldFiles(t *testing.T, rules string, more map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w, root := presetWorldNoPlugin(t, rules, more)
	// lint-a is served by the stand-in, whose preset is the one file
	// standin.conf: release 1 has no adapter to write it.
	p := porttest.NewStandinPlugin()
	p.Facts.Tool = "lint-a"
	p.Facts.Extends = true
	p.Facts.Preset = &plugin.ManifestPreset{Path: "standin.conf", Mark: true}
	w.PluginsOf().Install(string(root), model.PluginRef{Name: "standin", Version: "1.0.0"}, p)
	return w, root
}

// presetWorldNoPlugin is that world with no plugin installed at all.
func presetWorldNoPlugin(t *testing.T, rules string, more map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	files := map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": rules, "book/sources.yaml": promoteSources,
	}
	maps.Copy(files, more)
	root := w.Seed("repo", files)
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

func toolEntries(t *testing.T, c *coreapi.Change) string {
	t.Helper()
	shown, ok := c.Preview(context.Background())
	if !ok {
		t.Fatal("no preview")
	}
	for _, s := range shown.Plan.Sections {
		if s.Title == "Tool entries" {
			return strings.Join(s.Lines, "\n")
		}
	}
	t.Fatalf("no Tool entries section in %+v", shown.Plan.Sections)
	return ""
}

// TestAPromoteShowsTheEntryGoingLiveAndAPlanWithoutABindingShowsNone: a promote
// of a rule with a binding names the preset file and the entry it changes, and
// the apply writes that file; a promote of a rule with no binding shows no
// Tool entries section and writes none. Risk: a plan that hides the preset change it
// causes, or writes a preset file it did not show.
func TestAPromoteShowsTheEntryGoingLiveAndAPlanWithoutABindingShowsNone(t *testing.T) {
	t.Parallel()
	w, root := presetWorld(t)
	ctx := context.Background()
	c, ref, err := coreapi.RulePromote(ctx, w.Ports(), changeAsk(root, "gragas rule promote go.errors.1"), model.PromoteRule{ID: "go.errors.1", Why: why()})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	got := toolEntries(t, c)
	if !strings.Contains(got, "standin.conf") || !strings.Contains(got, "wrap-errors") {
		t.Errorf("the tool entries do not name the preset file and the entry:\n%s", got)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if p := read(t, w, root, "presets/lint-a/standin.conf"); !strings.Contains(p, "wrap-errors") {
		t.Errorf("the preset file is not written:\n%s", p)
	}
	c, ref, err = coreapi.RulePromote(ctx, w.Ports(), runTwo(changeAsk(root, "gragas rule promote go.errors.2")), model.PromoteRule{ID: "go.errors.2", Why: model.Why{Cite: "decide-it"}})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	// a promote that moves no entry is the plan it was: no section, so its digest is main's
	if titles := sectionTitles(t, c); slices.Contains(titles, "Tool entries") {
		t.Errorf("a rule with no binding shows a Tool entries section: %v", titles)
	}
}

func sectionTitles(t *testing.T, c *coreapi.Change) []string {
	t.Helper()
	shown, ok := c.Preview(context.Background())
	if !ok {
		t.Fatal("no preview")
	}
	var out []string
	for _, s := range shown.Plan.Sections {
		out = append(out, s.Title)
	}
	return out
}

// TestBindShowsStandingProofAndPresetThenWritesOneRecordAndOneHistoryLine: a
// bind of an active rule shows the entry, the preset change, the standing
// before and after and the proof to run; the apply writes the binding, the
// preset and one history line; a second run has nothing to do; unbind takes
// the record out again. Risk: a binding written twice, a plan with no proof
// line, or an unbind that leaves the record.
func TestBindShowsStandingProofAndPresetThenWritesOneRecordAndOneHistoryLine(t *testing.T) {
	t.Parallel()
	w, root := presetWorldWith(t, strings.Replace(presetRules, "strength: may\n    status: proposed", "strength: may\n    status: active", 1))
	ctx := context.Background()
	req := model.BindRule{ID: "go.errors.2", Binding: model.Binding{Tool: "lint-a", Target: "source", Key: "log-once", Value: true, Severity: model.SeverityError, Rung: 1, RunPoint: "commit"}}
	c, ref, err := coreapi.Bind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: changeAsk(root, "gragas bind go.errors.2")}, req)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	got := strings.Join(sectionTitles(t, c), "|")
	if want := "What changes|Tool entries|Standing before and after|Proof to run|What is written where"; got != want {
		t.Errorf("sections = %s, want %s", got, want)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	rules := read(t, w, root, "book/language/go/rules/errors.yaml")
	if strings.Count(rules, "key: log-once") != 1 {
		t.Errorf("the binding is not written once:\n%s", rules)
	}
	if h := read(t, w, root, "book/history.jsonl"); strings.Count(h, `"kind":"bind"`) != 1 {
		t.Errorf("history:\n%s", h)
	}
	if p := read(t, w, root, "presets/lint-a/standin.conf"); !strings.Contains(p, "log-once") {
		t.Errorf("the preset does not hold the entry:\n%s", p)
	}
	again, ref, err := coreapi.Bind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: runTwo(changeAsk(root, "gragas bind go.errors.2"))}, req)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := again.Apply(ctx, ""); out.Word != model.Done || !out.Nothing {
		t.Errorf("a second bind ended %q nothing=%v (%v)", out.Word, out.Nothing, out.Cause)
	}
	ask := changeAsk(root, "gragas unbind go.errors.2")
	ask.Run = "run-3"
	un, ref, err := coreapi.Unbind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: ask}, model.UnbindRule{ID: "go.errors.2", Tool: "lint-a", Target: "source", Key: "log-once"})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := un.Apply(ctx, ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("unbind ended %q (%v)", out.Word, out.Cause)
	}
	if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); strings.Contains(rules, "log-once") {
		t.Errorf("the binding is still in the file:\n%s", rules)
	}
}

// TestAHandEditedPresetIsListedAndNeverOverwrittenByABind: a preset file that
// is neither what the book generates before the change nor after it is listed
// by the plan as hand-edited, is not among the files the apply writes, and
// holds the same bytes after it; the binding itself is still written. Risk: a
// bind that replaces a person's edit with the generated file.
func TestAHandEditedPresetIsListedAndNeverOverwrittenByABind(t *testing.T) {
	t.Parallel()
	const mine = "# written by hand\nlinters: {}\n"
	w, root := presetWorldFiles(t, strings.Replace(presetRules, "strength: may\n    status: proposed", "strength: may\n    status: active", 1),
		map[string]string{"presets/lint-a/standin.conf": mine})
	ctx := context.Background()
	req := model.BindRule{ID: "go.errors.2", Binding: model.Binding{Tool: "lint-a", Target: "source", Key: "log-once", Value: true, Severity: model.SeverityError, Rung: 1, RunPoint: "commit"}}
	c, ref, err := coreapi.Bind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: changeAsk(root, "gragas bind go.errors.2")}, req)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if got := toolEntries(t, c); !strings.Contains(got, "hand-edited") {
		t.Errorf("the plan does not list the hand edit:\n%s", got)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if got := read(t, w, root, "presets/lint-a/standin.conf"); got != mine {
		t.Errorf("the hand edit was overwritten:\n%s", got)
	}
	if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); strings.Count(rules, "key: log-once") != 1 {
		t.Errorf("the binding is not written once:\n%s", rules)
	}
}

// bindThenUnbind binds go.errors.2 to lint-a log-once and applies it, so the
// generated preset of lint-a is on disk, and plans the unbind of that entry:
// the only entry of lint-a, so the new render no longer makes the file. With
// edited set, the unbind runs in a second world that holds the bound rule and
// that text as the preset file, as a hand edit leaves it.
func bindThenUnbind(t *testing.T, edited string) (*porttest.World, model.Root, *coreapi.Change) {
	t.Helper()
	w, root := presetWorldFiles(t, strings.Replace(presetRules, "strength: may\n    status: proposed", "strength: may\n    status: active", 1), nil)
	ctx := context.Background()
	bound := model.Binding{Tool: "lint-a", Target: "source", Key: "log-once", Value: true, Severity: model.SeverityError, Rung: 1, RunPoint: "commit"}
	c, ref, err := coreapi.Bind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: changeAsk(root, "gragas bind go.errors.2")}, model.BindRule{ID: "go.errors.2", Binding: bound})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("bind ended %q (%v)", out.Word, out.Cause)
	}
	if edited != "" {
		w, root = presetWorldFiles(t, read(t, w, root, "book/language/go/rules/errors.yaml"), map[string]string{"presets/lint-a/standin.conf": edited})
	}
	c, ref, err = coreapi.Unbind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: runTwo(changeAsk(root, "gragas unbind go.errors.2"))}, model.UnbindRule{ID: "go.errors.2", Tool: "lint-a", Target: "source", Key: "log-once"})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	return w, root, c
}

// TestAPresetFileTheRenderDropsIsDeletedWhenItIsTheOneGragasWrote: the unbind
// of the only entry of a tool plans the deletion of its preset file, names it,
// and apply takes it away; the rule's file loses its binding in the same run.
// Risk: a dropped file left to rot, or one removed that gragas did not write.
func TestAPresetFileTheRenderDropsIsDeletedWhenItIsTheOneGragasWrote(t *testing.T) {
	t.Parallel()
	w, root, c := bindThenUnbind(t, "")
	if _, there := w.File(root, "presets/lint-a/standin.conf"); !there {
		t.Fatal("the bind wrote no preset file to drop")
	}
	if got := toolEntries(t, c); !strings.Contains(got, "the file is deleted") {
		t.Errorf("the plan does not say the file is deleted:\n%s", got)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if _, there := w.File(root, "presets/lint-a/standin.conf"); there {
		t.Error("the dropped preset file is still there")
	}
	if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); strings.Contains(rules, "key: log-once") {
		t.Errorf("the binding is still in the rule's file:\n%s", rules)
	}
}

// TestAHandEditedDroppedPresetFileIsKeptAndNamed: the same unbind with the
// preset file changed by hand lists it as hand-edited, never deletes it, and
// still writes the rule's file. Risk: a person's file removed because the
// render no longer makes it.
func TestAHandEditedDroppedPresetFileIsKeptAndNamed(t *testing.T) {
	t.Parallel()
	const mine = "# written by hand\nlinters: {}\n"
	w, root, c := bindThenUnbind(t, mine)
	if got := toolEntries(t, c); !strings.Contains(got, "hand-edited") || strings.Contains(got, "the file is deleted") {
		t.Errorf("the plan does not keep and name the hand edit:\n%s", got)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if got := read(t, w, root, "presets/lint-a/standin.conf"); got != mine {
		t.Errorf("the hand edit was removed or changed:\n%s", got)
	}
	if rules := read(t, w, root, "book/language/go/rules/errors.yaml"); strings.Contains(rules, "key: log-once") {
		t.Errorf("the binding is still in the rule's file:\n%s", rules)
	}
}
