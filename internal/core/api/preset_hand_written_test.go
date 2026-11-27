package coreapi_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// handPresetWorld is the preset world whose go.errors.1 also binds each of
// tools, which no plugin serves, with the identity listing listed as the
// tools whose presets are hand-written (none when listed is empty). lint-a is
// served by the stand-in, as in every preset world.
func handPresetWorld(t *testing.T, listed string, tools ...string) (*porttest.World, model.Root) {
	t.Helper()
	more := ""
	for _, tool := range tools {
		more += "      - {tool: " + tool + ", target: config, key: entry, value: kept, severity: warning}\n"
	}
	rules := strings.Replace(presetRulesActive(), "        severity: error\n", "        severity: error\n"+more, 1)
	if rules == presetRulesActive() && len(tools) > 0 {
		t.Fatal("the extra bindings were not planted")
	}
	files := map[string]string{}
	if listed != "" {
		files["book/rulebook.yaml"] = bookIdentity + "hand_written_presets: [" + listed + "]\n"
	}
	return presetBook(t, rules, files)
}

// TestAToolTheBookListsAsHandWrittenIsNotJudgedAndIsNamed: lint-h, bound and listed in
// hand_written_presets, is served by no plugin, and preset --check neither
// ends not ready on it nor compares a file of it: lint-a's file is the one
// compared, and lint-h is answered as hand-written. The plan writes nothing
// of lint-h either. Planted failure: a check that weighs every unserved tool,
// which ends on lint-h with ErrNoPlugin.
func TestAToolTheBookListsAsHandWrittenIsNotJudgedAndIsNamed(t *testing.T) {
	t.Parallel()
	w, root := handPresetWorld(t, "lint-h", "lint-h")
	ctx := context.Background()
	rep, err := coreapi.PresetCheck(ctx, w.Ports(), root)
	if err != nil {
		t.Fatalf("check: %v, want lint-h left to its owners", err)
	}
	if !slices.Equal(rep.HandWritten, []string{"lint-h"}) {
		t.Errorf("HandWritten = %q, want [lint-h]", rep.HandWritten)
	}
	if rep.Compared != 1 || len(rep.Files) != 1 || rep.Files[0].Tool != "lint-a" {
		t.Errorf("compared %d files %+v, want lint-a's alone", rep.Compared, rep.Files)
	}
	c, ref, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
}

// TestAnUnlistedToolNoPluginServesEndsThePresetCheckNotReady: the list is no
// default. With lint-h listed, lint-g, bound, unlisted and served by no
// plugin, ends the check and the plan with ErrNoPlugin naming lint-g alone;
// with nothing listed both are named. Planted failure: a check that skips
// every unserved tool once the book lists one, which ends done.
func TestAnUnlistedToolNoPluginServesEndsThePresetCheckNotReady(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, listed, want string }{
		{"lint-h listed, lint-g named alone", "lint-h", "for lint-g,"},
		{"nothing listed, both named", "", "for lint-g, lint-h,"},
	} {
		want := c.want
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, root := handPresetWorld(t, c.listed, "lint-h", "lint-g")
			ctx := context.Background()
			_, err := coreapi.PresetCheck(ctx, w.Ports(), root)
			if !errors.Is(err, coreapi.ErrNoPlugin) || !strings.Contains(err.Error(), want) {
				t.Errorf("check: %v, want ErrNoPlugin %q", err, want)
			}
			if _, _, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset")); !errors.Is(err, coreapi.ErrNoPlugin) {
				t.Errorf("plan: %v, want ErrNoPlugin", err)
			}
		})
	}
}

// markedWorld is the preset world with the stand-in serving lint-a, asked for
// the mark, and copied once where extends is false, with have as the tree's
// copy of its file (none when have is empty). The file the book makes is the
// mark line and lint-a's one entry.
func markedWorld(t *testing.T, extends bool, have string) (*porttest.World, model.Root) {
	t.Helper()
	more := map[string]string{}
	if have != "" {
		more[pluginPresetFile] = have
	}
	w, root := presetBook(t, presetRulesActive(), more)
	p := porttest.NewStandinPlugin()
	p.Facts.Tool = "lint-a"
	p.Facts.Extends = extends
	p.Facts.Preset = &plugin.ManifestPreset{Path: "standin.conf", Mark: true}
	w.PluginsOf().Install(string(root), model.PluginRef{Name: "standin", Version: "1.0.0"}, p)
	return w, root
}

// TestAMarkedPresetIsHeldByteForByteAndAnUnmarkedCopyOnceIsTheProjects:
// while the file carries the plugin's mark, preset --check holds it byte for byte
// whatever extends says, naming the first line that differs, and the plan
// writes it again; once the mark is removed a copy-once file is the
// project's, and the check and the plan leave it alone, and the check names it
// and does not count it as compared; an untouched
// file is the same; the mark counts on the first line alone. Planted failures:
// a check that lets every copy-once file be (edited-and-marked ends same), one
// that holds every copy-once file (unmarked ends differs), one that finds the
// mark anywhere in the file (the quoted mark ends differs), and one that
// counts a project's file as compared (Compared 1, the file in Files).
func TestAMarkedPresetIsHeldByteForByteAndAnUnmarkedCopyOnceIsTheProjects(t *testing.T) {
	t.Parallel()
	made := "# " + link.Mark() + "\nwrap-errors error\n"
	// owned is the state of a row whose file is the project's: it is in
	// Owned, not in Files, and not counted
	const owned coreapi.PresetState = ""
	for _, c := range []struct {
		name    string
		extends bool
		have    string
		state   coreapi.PresetState
		first   string
	}{
		{"untouched, copied once", false, made, coreapi.PresetSame, ""},
		{"untouched, extending", true, made, coreapi.PresetSame, ""},
		{"marked and a line added, copied once", false, made + "extra\n", coreapi.PresetDiffers, `line 3 is not one the book makes: it holds "extra\n"`},
		{"marked and a line edited, copied once", false, "# " + link.Mark() + "\nwrap-errors warning\n", coreapi.PresetDiffers, `line 2 holds "wrap-errors warning\n", the book makes "wrap-errors error\n"`},
		{"marked and a line cut, extending", true, "# " + link.Mark() + "\n", coreapi.PresetDiffers, `line 2 is missing: the book makes "wrap-errors error\n"`},
		{"the mark removed, copied once", false, "wrap-errors warning\n", owned, ""},
		{"the mark removed and quoted on a later line, copied once", false, "wrap-errors warning\n# " + link.Mark() + "\n", owned, ""},
		{"the mark removed, extending", true, "wrap-errors warning\n", coreapi.PresetDiffers, `line 1 holds "wrap-errors warning\n", the book makes "# Generated by gragas from the rulebook. Regenerate it, neve"…`},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, root := markedWorld(t, c.extends, c.have)
			ctx := context.Background()
			rep, err := coreapi.PresetCheck(ctx, w.Ports(), root)
			if err != nil {
				t.Fatalf("check %+v, %v", rep, err)
			}
			if c.state == owned {
				if rep.Compared != 0 || len(rep.Files) != 0 || !slices.Equal(rep.Owned, []string{pluginPresetFile}) {
					t.Errorf("check = compared %d, files %+v, owned %q; want none compared and %s named as the project's", rep.Compared, rep.Files, rep.Owned, pluginPresetFile)
				}
			} else if rep.Compared != 1 || len(rep.Files) != 1 || len(rep.Owned) != 0 {
				t.Fatalf("check = compared %d, files %+v, owned %q; want the one file compared", rep.Compared, rep.Files, rep.Owned)
			} else if got := rep.Files[0]; got.State != c.state || got.First != c.first {
				t.Errorf("check = %s %q, want %s %q", got.State, got.First, c.state, c.first)
			}
			ch, _, err := coreapi.Preset(ctx, w.Ports(), changeAsk(root, "gragas preset"))
			if err != nil {
				t.Fatal(err)
			}
			if out := ch.Apply(ctx, ""); out.Word != model.Done {
				t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
			}
			want := made
			if c.state == coreapi.PresetSame || c.state == owned {
				want = c.have
			}
			if got := read(t, w, root, pluginPresetFile); got != want {
				t.Errorf("after the apply the file is %q, want %q", got, want)
			}
		})
	}
}

// TestAPresetFileThatIsNotThereIsNamedSo: a file the book makes that the tree
// lacks differs, and says it is not there.
func TestAPresetFileThatIsNotThereIsNamedSo(t *testing.T) {
	t.Parallel()
	w, root := markedWorld(t, false, "")
	rep, err := coreapi.PresetCheck(context.Background(), w.Ports(), root)
	if err != nil || len(rep.Files) != 1 || rep.Files[0].State != coreapi.PresetDiffers || rep.Files[0].First != "it is not there" {
		t.Errorf("check = %+v, %v, want the file named as not there", rep, err)
	}
}
