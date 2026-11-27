package coreapi_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
	"github.com/birdtable/gragas/internal/rendered"
)

// presetRulesActive is presetRules with go.errors.1 active, so its binding
// makes lint-a's preset.
func presetRulesActive() string {
	return strings.Replace(presetRules, "strength: must\n    status: proposed", "strength: must\n    status: active", 1)
}

// presetBook is the preset world with the layout concern's file it lacks, so
// the book reads with no finding.
func presetBook(t *testing.T, rules string, more map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	files := map[string]string{"book/language/go/rules/layout.yaml": "format: 3\npart: go\nconcern: layout\nrules: []\n"}
	maps.Copy(files, more)
	return presetWorldFiles(t, rules, files)
}

// serveWith installs for lint-a the stand-in whose preset is standin.conf: the
// text of each input it names, then the key of each entry; extends says whether
// a project's copy is the engine's to write again.
func serveWith(w *porttest.World, root model.Root, extends bool, inputs ...string) {
	p := porttest.NewStandinPlugin()
	p.Facts.Tool = "lint-a"
	p.Facts.Extends = extends
	p.Facts.Preset = &plugin.ManifestPreset{Path: "standin.conf", Mark: true, Inputs: inputs}
	p.OnPreset = func(r plugin.PresetRequest) (plugin.PresetResponse, error) {
		text := ""
		for _, in := range r.Inputs {
			text += *in.Data.Text
		}
		for _, e := range r.Entries {
			text += strings.Join(e.Rules, ",") + " " + e.Key + "\n"
		}
		return plugin.PresetResponse{Files: []plugin.PresetFile{{Path: "standin.conf", Text: text}}}, nil
	}
	w.PluginsOf().Install(string(root), model.PluginRef{Name: "standin", Version: "1.0.0"}, p)
}

const presetFile = "presets/lint-a/standin.conf"

// TestPresetWritesTheFilesOnceAndASecondRunHasNothingToWrite: the apply writes
// the preset of lint-a, and a second plan has no change, a check is fresh and
// counts what it compared. Risk: a run that rewrites bytes it already holds.
func TestPresetWritesTheFilesOnceAndASecondRunHasNothingToWrite(t *testing.T) {
	t.Parallel()
	w, root := presetBook(t, presetRulesActive(), nil)
	ctx := context.Background()
	c, ref, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if read(t, w, root, presetFile) == "" {
		t.Fatal("the preset file was not written")
	}
	again, _, err := coreapi.Preset(ctx, w.Ports(), runTwo(changeAsk(root, "gragas preset")))
	if err != nil {
		t.Fatal(err)
	}
	if out := again.Apply(ctx, ""); out.Word != model.Done || !out.Nothing {
		t.Errorf("the second run ended %q, nothing to do %v, want done with nothing", out.Word, out.Nothing)
	}
	rep, err := coreapi.PresetCheck(ctx, w.Ports(), root)
	if err != nil || rep.Compared == 0 {
		t.Fatalf("check: %+v, %v", rep, err)
	}
	for _, f := range rep.Files {
		if f.State != coreapi.PresetSame {
			t.Errorf("%s is %s after the apply", f.Path, f.State)
		}
	}
}

// TestPresetCheckNamesAFileOneByteOff: a file one byte longer than the book
// makes is named as differing, and the check writes nothing. Risk: a check
// that compares sizes, or that fixes what it finds.
func TestPresetCheckNamesAFileOneByteOff(t *testing.T) {
	t.Parallel()
	w, root := presetBook(t, presetRulesActive(), nil)
	ctx := context.Background()
	c, _, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))
	if err != nil {
		t.Fatal(err)
	}
	c.Apply(ctx, "")
	made := read(t, w, root, presetFile)
	off, root2 := presetBook(t, presetRulesActive(), map[string]string{presetFile: made + "\n"})
	rep, err := coreapi.PresetCheck(ctx, off.Ports(), root2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Files) != 1 || rep.Files[0].State != coreapi.PresetDiffers || rep.Files[0].Path != presetFile {
		t.Errorf("check %+v, want %s differing", rep.Files, presetFile)
	}
	if got := read(t, off, root2, presetFile); got != made+"\n" {
		t.Errorf("the check wrote the file:\n%s", got)
	}
}

// TestPresetCheckNamesADeletedFile: a file the book makes that is not there
// is named as differing. Risk: a missing file counted as nothing to compare.
func TestPresetCheckNamesADeletedFile(t *testing.T) {
	t.Parallel()
	w, root := presetBook(t, presetRulesActive(), nil)
	rep, err := coreapi.PresetCheck(context.Background(), w.Ports(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Files) != 1 || rep.Files[0].State != coreapi.PresetDiffers || rep.Compared != 1 {
		t.Errorf("check %+v, want the one file differing", rep)
	}
}

// TestPresetCheckWithNoRenderedFileIsNotReady: a book that binds nothing makes
// no file, and a check over no file is not green. Risk: zero files compared
// reading as fresh.
func TestPresetCheckWithNoRenderedFileIsNotReady(t *testing.T) {
	t.Parallel()
	w, root := presetBook(t, presetRules, nil) // every rule proposed: no binding is held
	_, err := coreapi.PresetCheck(context.Background(), w.Ports(), root)
	if !errors.Is(err, rendered.ErrNoFile) {
		t.Errorf("err %v, want the compare's ErrNoFile", err)
	}
}

// TestPresetRefusesAMissingSettingsInput: a plugin whose manifest names an
// input the rulebook does not hold refuses, in the plan and the check.
// Risk: a preset written without the linter list it is meant to carry.
func TestPresetRefusesAMissingSettingsInput(t *testing.T) {
	t.Parallel()
	w, root := presetBook(t, presetRulesActive(), nil)
	serveWith(w, root, true, "settings.yml")
	ctx := context.Background()
	if _, err := coreapi.PresetCheck(ctx, w.Ports(), root); !errors.Is(err, coreapi.ErrSettingsInputMissing) {
		t.Errorf("check: %v, want ErrSettingsInputMissing", err)
	}
	if _, _, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset")); !errors.Is(err, coreapi.ErrSettingsInputMissing) {
		t.Errorf("plan: %v, want ErrSettingsInputMissing", err)
	}
}

// TestPresetNeverOverwritesACopyOnceFile: a tool with no extends has its file
// copied once; a project's edit stays through the plan, and the check names it
// as the project's and does not compare it. Risk: a rewritten file a project
// owns.
func TestPresetNeverOverwritesACopyOnceFile(t *testing.T) {
	t.Parallel()
	const mine = "# mine\n"
	w, root := presetBook(t, presetRulesActive(), map[string]string{presetFile: mine})
	serveWith(w, root, false)
	ctx := context.Background()
	c, _, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))
	if err != nil {
		t.Fatal(err)
	}
	c.Apply(ctx, "")
	if got := read(t, w, root, presetFile); got != mine {
		t.Errorf("the copied file was overwritten:\n%s", got)
	}
	rep, err := coreapi.PresetCheck(ctx, w.Ports(), root)
	if err != nil || rep.Compared != 0 || len(rep.Files) != 0 || !slices.Equal(rep.Owned, []string{presetFile}) {
		t.Errorf("check %+v, %v, want the owned copy named as the project's and not compared", rep, err)
	}
}

// TestThePresetsFolderIsTheRepositorysAndNotTheBooks: the book stands in book/,
// and the settings input is read from presets/ at the root, where projects
// import the presets, and never from book/presets/. A missing input is refused
// with the root's path. Risk: presets written, or an input read, under the
// book's folder, where no importer looks.
func TestThePresetsFolderIsTheRepositorysAndNotTheBooks(t *testing.T) {
	t.Parallel()
	const input = "linters:\n  # lint:begin\n  # lint:end\n"
	ctx := context.Background()
	w, root := presetBook(t, presetRulesActive(), map[string]string{"book/presets/lint-a/settings.yml": input})
	serveWith(w, root, true, "settings.yml")
	_, err := coreapi.PresetCheck(ctx, w.Ports(), root)
	if !errors.Is(err, coreapi.ErrSettingsInputMissing) || !strings.Contains(err.Error(), "presets/lint-a/settings.yml") || strings.Contains(err.Error(), "book/presets") {
		t.Fatalf("the input below the book's folder was read, or the refusal does not name the root's path: %v", err)
	}
	w, root = presetBook(t, presetRulesActive(), map[string]string{"presets/lint-a/settings.yml": input})
	serveWith(w, root, true, "settings.yml")
	c, _, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))
	if err != nil {
		t.Fatalf("the input at the root is not read: %v", err)
	}
	c.Apply(ctx, "")
	if got := read(t, w, root, presetFile); !strings.Contains(got, "go.errors.1") || !strings.Contains(got, input) {
		t.Errorf("the preset is not written at the root's presets/:\n%s", got)
	}
	if _, there := w.File(root, "book/"+presetFile); there {
		t.Error("a preset was written below the book's folder")
	}
}
