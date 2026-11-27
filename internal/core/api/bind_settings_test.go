package coreapi_test

import (
	"context"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// settingsMark is a line of the settings input that only a read of the input
// can bring into a preset file.
const settingsMark = "# held by the settings input"

// settingsWorld is presetWorldFiles' book with lint-a's adapter as clean.yaml
// writes it, naming the settings input settings.yml, and that input at the root's
// presets/lint-a/, where presetSettings reads it.
func settingsWorld(t *testing.T) (ctx context.Context, explain func(id string) error, bind func() (string, error)) {
	t.Helper()
	input := "linters:\n  " + settingsMark + "\n  # lint:begin\n  # lint:end\n"
	// Both rules active: go.errors.1's lint-a binding is held, so explain needs the
	// input before the bind, and go.errors.2 takes the bind.
	w, root := presetWorldFiles(t, strings.ReplaceAll(presetRules, "status: proposed", "status: active"),
		map[string]string{"presets/lint-a/settings.yml": input,
			// the layout file the structure names, so explain reads the book whole, as completed's does
			"book/language/go/rules/layout.yaml": "format: 3\npart: go\nconcern: layout\nrules: []\n"})
	// the stand-in's manifest names the settings input, and its preset is that
	// input's text with each entry's key after it
	p := porttest.NewStandinPlugin()
	p.Facts.Tool = "lint-a"
	p.Facts.Extends = true
	p.Facts.Preset = &plugin.ManifestPreset{Path: "standin.conf", Mark: true, Inputs: []string{"settings.yml"}}
	p.OnPreset = func(r plugin.PresetRequest) (plugin.PresetResponse, error) {
		text := ""
		for _, in := range r.Inputs {
			text += *in.Data.Text
		}
		for _, e := range r.Entries {
			text += e.Key + "\n"
		}
		return plugin.PresetResponse{Files: []plugin.PresetFile{{Path: "standin.conf", Text: text}}}, nil
	}
	w.PluginsOf().Install(string(root), model.PluginRef{Name: "standin", Version: "1.0.0"}, p)
	ctx = context.Background()
	explain = func(id string) error {
		_, err := coreapi.ExplainRule(ctx, w.Ports(), root, id)
		return err
	}
	bind = func() (string, error) {
		req := model.BindRule{ID: "go.errors.2", Binding: model.Binding{Tool: "lint-a", Target: "source", Key: "log-once", Value: true, Severity: model.SeverityError, Rung: 1, RunPoint: "commit"}}
		c, ref, err := coreapi.Bind(ctx, w.Ports(), coreapi.OverlayAsk{ChangeAsk: changeAsk(root, "gragas bind go.errors.2")}, req)
		if err != nil {
			return "", err
		}
		if ref != nil {
			t.Fatalf("bind refused: %s", ref.Observed())
		}
		if out := c.Apply(ctx, ""); out.Cause != nil {
			return "", out.Cause
		}
		return read(t, w, root, presetFile), nil
	}
	return ctx, explain, bind
}

// TestBindAndExplainReadTheSettingsInputTheAdapterNames: with lint-a's adapter naming a settings input, a bind generates the
// preset from that input, read at the root's presets/lint-a/, so the preset it
// writes holds the input's own line beside the entry bound, and explain reads the
// same input to say what ships. Planted failure: presetSettings reading nothing
// (or another folder), which leaves Generate no input the adapter names, so the
// bind and the explain are refused, not only preset's own tests.
func TestBindAndExplainReadTheSettingsInputTheAdapterNames(t *testing.T) {
	t.Parallel()
	_, explain, bind := settingsWorld(t)
	if err := explain("go.errors.1"); err != nil {
		t.Errorf("explain with the settings input at the root: %v", err)
	}
	got, err := bind()
	if err != nil {
		t.Fatalf("bind with the settings input at the root: %v", err)
	}
	if !strings.Contains(got, settingsMark) || !strings.Contains(got, "log-once") {
		t.Errorf("the preset the bind wrote holds no line of the settings input, or no entry bound:\n%s", got)
	}
}
