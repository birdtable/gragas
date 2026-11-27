package internal

// The plants of the generated-presets member (gragas#2340): it holds the
// presets through the plugins tools.yaml pins, each approved by its pin in a
// state folder the member makes for its run. A clean copy of the files it
// reads is green: golangci-lint is approved and served, and its one file is
// the committed one. Each plant below is red, naming what is wrong: a pin
// whose digest is not the lock's, one byte of the committed bundle changed,
// and no lock, each before preset --check runs; the preset file the plugin
// makes taken away, and a line of it edited under the mark, by preset
// --check; and a line edited with the mark removed, by the member, after it.
//
// The member reads preset --check's report under --json and skips the preset
// of a pinned plugin whose tool the report does not list, naming it
// (gragas#2033). Of the three plants,
// "a bound tool with its file missing is red" and "a bound tool with its mark
// removed and a line edited is red" are the two golangci-lint rows below, over
// the real plugin; "an unbound pinned plugin with no file passes and is named"
// needs a second released plugin no rule binds, which this tree does not pin
// yet, so it is cmd/gragas-dev's TestAPinnedPluginNoRuleBindsIsSkippedAndNamed,
// with the other two beside it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
)

// pluginPresetCopy is a copy of the files generated-presets reads: tools.yaml, the
// plugins, the rulebook and the presets.
func pluginPresetCopy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"plugins", "rulebook", "presets"} {
		copyTree(t, d, dir)
	}
	b, err := os.ReadFile(filepath.Join(root, "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// plantedPresetRun runs generated-presets over dir with the file rel changed by plant,
// and puts the file back after.
func plantedPresetRun(t *testing.T, dir, rel string, plant func([]byte) []byte) (string, bool) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, plant(append([]byte(nil), b...)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Error(err)
		}
	})
	return runTask(t, "generated-presets", "PRESET_ROOT="+filepath.ToSlash(dir))
}

// flipLast changes the last byte of b to another.
func flipLast(b []byte) []byte {
	if len(b) > 0 {
		b[len(b)-1] ^= 0x01
	}
	return b
}

func TestTheGeneratedPresetsMemberHoldsThePinnedPluginsAndIsRedOnEachPlant(t *testing.T) {
	// The rulebook binds changie, goreleaser and gragas too, and no plugin of
	// theirs is released; the identity lists them in hand_written_presets, so
	// preset --check names each as hand-written and does not compare it, and
	// is not refused (gragas#2071).
	// golangci-lint is approved by its pin and served: its one file is
	// compared, and golangci-lint@1.0.1 makes the committed one byte for byte
	// (birdtable/gragas-plugins#33), so the member passes and is a member of
	// generated-files.
	t.Run("a clean copy", func(t *testing.T) {
		dir := pluginPresetCopy(t)
		out, ok := runTask(t, "generated-presets", "PRESET_ROOT="+filepath.ToSlash(dir))
		if !ok {
			t.Errorf("a clean copy: red, output %q, want a pass: the pinned plugin makes the committed preset", out)
		}
		for _, want := range []string{
			"changie: hand-written (hand_written_presets), so not compared",
			"goreleaser: hand-written (hand_written_presets), so not compared",
			"gragas: hand-written (hand_written_presets), so not compared",
			"1 preset file compared, none differs",
			"approved in a state folder of this run",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("a clean copy: output %q, want %q: the hand-written tools named and golangci-lint's one file compared", out, want)
			}
		}
		if strings.Contains(out, "no plugin is installed") {
			t.Errorf("a clean copy: output %q, want no tool refused for want of a plugin", out)
		}
	})
	t.Run("a pin whose digest is not the lock's", func(t *testing.T) {
		dir := pluginPresetCopy(t)
		out, ok := plantedPresetRun(t, dir, "tools.yaml", func(b []byte) []byte {
			s := string(b)
			i := strings.Index(s, `digest: "sha256:`)
			if i < 0 {
				t.Fatal("tools.yaml pins no plugin digest to plant over")
			}
			i += len(`digest: "sha256:`)
			c := byte('0')
			if s[i] == '0' {
				c = '1'
			}
			return []byte(s[:i] + string(c) + s[i+1:])
		})
		if ok || !strings.Contains(out, "and tools.yaml pins sha256:") {
			t.Errorf("a planted pin: ok %v, output %q, want a failure naming the lock's digest and the pin", ok, out)
		}
	})
	t.Run("one byte of the committed bundle", func(t *testing.T) {
		dir := pluginPresetCopy(t)
		out, ok := plantedPresetRun(t, dir, "plugins/golangci-lint/1.0.1/testdata/golden/preset.golden", flipLast)
		if ok || !strings.Contains(out, "was not approved by its pin") {
			t.Errorf("a changed bundle: ok %v, output %q, want a failure saying the plugin was not approved by its pin", ok, out)
		}
	})
	t.Run("no lock", func(t *testing.T) {
		dir := pluginPresetCopy(t)
		if err := os.Remove(filepath.Join(dir, "plugins", "lock.yaml")); err != nil {
			t.Fatal(err)
		}
		if out, ok := runTask(t, "generated-presets", "PRESET_ROOT="+filepath.ToSlash(dir)); ok || !strings.Contains(out, "the plugin lock could not be read") {
			t.Errorf("no lock: ok %v, output %q, want a failure saying the lock could not be read", ok, out)
		}
	})
	// The plugin's manifest does not extend (extends: false), so gragas writes
	// its preset once, and its manifest asks for the mark (mark: true): while
	// the file opens with the mark, preset --check holds it byte for byte, and
	// it is red where the file is not there. A project takes the file as its
	// own by removing the mark, and preset --check then reads it as the
	// project's; the member refuses that here, naming the file, so this
	// repository's preset is always what the plugin makes (gragas#2429). The plants are the
	// file taken away, a line edited with the mark kept, and a line edited with the mark removed.
	// a bound tool with its file missing is red
	t.Run("a preset the plugin makes, taken away", func(t *testing.T) {
		dir := pluginPresetCopy(t)
		if err := os.Remove(filepath.Join(dir, "presets", "golangci-lint", ".golangci.yml")); err != nil {
			t.Fatal(err)
		}
		if out, ok := runTask(t, "generated-presets", "PRESET_ROOT="+filepath.ToSlash(dir)); ok || !strings.Contains(out, "the presets are not what the installed plugins make") {
			t.Errorf("no preset: ok %v, output %q, want a failure saying the presets are not what the plugins make", ok, out)
		}
	})
	const preset = "presets/golangci-lint/.golangci.yml"
	// edited is the committed preset with its default linters turned off, the
	// edit a reviewer planted, and the mark removed when unmark is set.
	edited := func(t *testing.T, unmark bool) func([]byte) []byte {
		t.Helper()
		return func(b []byte) []byte {
			s := string(b)
			if !strings.Contains(s, "\n  default: standard") {
				t.Fatalf("%s holds no `default: standard` to plant over", preset)
			}
			s = strings.Replace(s, "\n  default: standard", "\n  default: none", 1)
			if unmark {
				first, rest, _ := strings.Cut(s, "\n")
				if !strings.Contains(first, link.Mark()) {
					t.Fatalf("%s's first line is %q, not the mark", preset, first)
				}
				s = rest
			}
			return []byte(s)
		}
	}
	t.Run("a line edited, the mark kept", func(t *testing.T) {
		dir := pluginPresetCopy(t)
		if out, ok := plantedPresetRun(t, dir, preset, edited(t, false)); ok || !strings.Contains(out, "the presets are not what the installed plugins make") {
			t.Errorf("an edit under the mark: ok %v, output %q, want preset --check red on the file", ok, out)
		}
	})
	// a bound tool with its mark removed and a line edited is red (2429's row)
	t.Run("the mark removed and a line edited", func(t *testing.T) {
		dir := pluginPresetCopy(t)
		out, ok := plantedPresetRun(t, dir, preset, edited(t, true))
		if ok || !strings.Contains(out, preset+" does not open with the mark, and golangci-lint@1.0.1's manifest asks for it") {
			t.Errorf("the mark removed: ok %v, output %q, want a failure naming the file and the mark its manifest asks for", ok, out)
		}
	})
}
