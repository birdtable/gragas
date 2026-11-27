package plugintest_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/plugintest"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// wasm is the smallest module there is: the magic number and version 1.
func wasm() []byte { return []byte("\x00asm\x01\x00\x00\x00") }

// engine is the engine this test holds a bundle to: contract 1, no features.
func engine() plugintest.Engine { return plugintest.Engine{Contract: 1, Features: []string{}} }

// manifest is a manifest that passes C1 to C4 for the bundle lint-a 2.0.0, with
// lines the test replaces to plant one fault.
func manifest(replace ...string) []byte {
	m := `contract: 1
needs: []
name: lint-a
version: 2.0.0
tool: lint-a
tool_versions: ">=10.13.0 <11.0.0"
targets: [source]
provides: [entries]
extends: false
warning: false
programs:
  - id: lint-a
    path: bin/lint-a
    version_argv: [--version]
    versions: ">=10.13.0 <11.0.0"
wasm: ` + digest.Prefixed(wasm()) + "\n"
	for i := 0; i+1 < len(replace); i += 2 {
		if !strings.Contains(m, replace[i]) {
			panic("the plant " + replace[i] + " is not in the manifest")
		}
		m = strings.Replace(m, replace[i], replace[i+1], 1)
	}
	return []byte(m)
}

const request = `{"body":{"config":[],"have":[],"scratch":"/s","target":"source","tool_version":"10.13.0"},"contract":1,"features":[],"op":"entries"}`

func good() plugintest.Bundle {
	return plugintest.Bundle{Name: "lint-a", Version: "2.0.0", Manifest: manifest(), Wasm: wasm(), HasWasm: true,
		Cases: []plugintest.Case{{Name: "1-entries", Request: []byte(request)}}}
}

func statuses(t *testing.T, b plugintest.Bundle) map[int]model.PluginCheck {
	t.Helper()
	out := map[int]model.PluginCheck{}
	for _, c := range plugintest.Run(t.Context(), engine(), b) {
		out[c.N] = c
	}
	if len(out) != plugintest.Checks {
		t.Fatalf("Run answered %d checks, want %d", len(out), plugintest.Checks)
	}
	return out
}

// TestAGoodBundlePassesWhatRunsAndSaysWhatDoesNot: C1 to C4 and C6 pass, and the
// checks that need the wasm host are not run, never passed.
func TestAGoodBundlePassesWhatRunsAndSaysWhatDoesNot(t *testing.T) {
	t.Parallel()
	got := statuses(t, good())
	for n := 1; n <= 13; n++ {
		want := model.CheckPassed
		if n == 5 || n >= 7 {
			want = model.CheckNotRun
		}
		if got[n].Status != want {
			t.Errorf("C%d is %s %v, want %s", n, got[n].Status, got[n].What, want)
		}
	}
}

// TestEachPlantedBundleFailsItsCheckAndNoOther: for each check this command
// runs, a bundle that fails it, red with its number, and the others as they
// were.
func TestEachPlantedBundleFailsItsCheckAndNoOther(t *testing.T) {
	t.Parallel()
	with := func(f func(*plugintest.Bundle)) plugintest.Bundle { b := good(); f(&b); return b }
	cases := []struct {
		name string
		n    int
		b    plugintest.Bundle
		said string
	}{
		{"an unknown key", 1, with(func(b *plugintest.Bundle) { b.Manifest = append(manifest(), "colour: red\n"...) }), "colour"},
		{"a key that is not a string", 1, with(func(b *plugintest.Bundle) { b.Manifest = manifest("extends: false", "extends: 3") }), "/extends"},
		{"a manifest that is not there", 1, with(func(b *plugintest.Bundle) { b.Manifest = nil }), "plugin.yaml"},
		{"a contract the engine does not speak", 2, with(func(b *plugintest.Bundle) { b.Manifest = manifest("contract: 1", "contract: 2") }), "contract 2"},
		{"a need the engine lacks", 2, with(func(b *plugintest.Bundle) { b.Manifest = manifest("needs: []", "needs: [describe]") }), "describe"},
		{"a name that is not the folder", 3, with(func(b *plugintest.Bundle) { b.Name = "lint-b" }), "lint-b"},
		{"a version that is not the folder", 3, with(func(b *plugintest.Bundle) { b.Version = "2.0.1" }), "2.0.1"},
		{"a digest that is not the module's", 3, with(func(b *plugintest.Bundle) { b.Wasm = []byte("\x00asm\x01\x00\x00\x01") }), "plugin.wasm is sha256:"},
		{"a module that is not there", 3, with(func(b *plugintest.Bundle) { b.Wasm, b.HasWasm = nil, false }), "not in the bundle"},
		{"a provides with nothing in it", 4, with(func(b *plugintest.Bundle) { b.Manifest = manifest("provides: [entries]", "provides: []") }), "provides is empty"},
		{"a grant id given twice", 4, with(func(b *plugintest.Bundle) {
			b.Manifest = append(manifest(), "  - id: lint-a\n    path: bin/other\n    version_argv: [-v]\n    versions: \">=1.0.0\"\n"...)
			b.Manifest = []byte(strings.Replace(string(b.Manifest), "wasm: "+digest.Prefixed(wasm())+"\n", "", 1) + "wasm: " + digest.Prefixed(wasm()) + "\n")
		}), `"lint-a" is given twice`},
		{"a request that is not an envelope", 6, with(func(b *plugintest.Bundle) { b.Cases[0].Request = []byte(`{"contract":1}`) }), "1-entries"},
		{"a request with a body that is not the operation's", 6, with(func(b *plugintest.Bundle) {
			b.Cases[0].Request = []byte(strings.Replace(request, `"target":"source",`, "", 1))
		}), "target"},
		{"a request that is not JSON", 6, with(func(b *plugintest.Bundle) { b.Cases[0].Request = []byte("{") }), "1-entries"},
		{"a request for another operation than the folder's", 6, with(func(b *plugintest.Bundle) { b.Cases[0].Name = "1-plan" }), "names the operation plan"},
		{"a case without its answer", 6, with(func(b *plugintest.Bundle) { b.Cases[0].Missing = "response.json" }), "response.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := statuses(t, c.b)
			for n := 1; n <= 6; n++ {
				if n == 5 {
					continue
				}
				if n == c.n {
					if got[n].Status != model.CheckFailed || !strings.Contains(strings.Join(got[n].What, "\n"), c.said) {
						t.Errorf("C%d is %s %v, want failed and saying %q", n, got[n].Status, got[n].What, c.said)
					}
				} else if got[n].Status == model.CheckFailed || (got[n].Status == model.CheckNotRun && (c.n != 1 || n < 2 || n > 4)) {
					t.Errorf("C%d is %s %v, want passed: the plant fails C%d alone", n, got[n].Status, got[n].What, c.n)
				}
			}
		})
	}
}

// TestAManifestThatCannotBeReadLeavesTheRestNotRun: C2 to C4 read what C1 read,
// and say so when it did not.
func TestAManifestThatCannotBeReadLeavesTheRestNotRun(t *testing.T) {
	t.Parallel()
	for name, m := range map[string]string{
		"anchors":   "a: &x 1\nb: *x\n",
		"duplicate": "name: a\nname: b\n",
		"a list":    "- 1\n",
	} {
		b := good()
		b.Manifest = []byte(m)
		got := statuses(t, b)
		if got[1].Status != model.CheckFailed {
			t.Errorf("%s: C1 is %s, want failed", name, got[1].Status)
		}
		for n := 2; n <= 4; n++ {
			if got[n].Status != model.CheckNotRun {
				t.Errorf("%s: C%d is %s, want not run", name, n, got[n].Status)
			}
		}
	}
}

// TestNoCasesLeavesTheRequestCheckNotRun: a bundle with no case has nothing for C6.
func TestNoCasesLeavesTheRequestCheckNotRun(t *testing.T) {
	t.Parallel()
	b := good()
	b.Cases = nil
	if got := statuses(t, b); got[6].Status != model.CheckNotRun {
		t.Errorf("C6 is %s, want not run", got[6].Status)
	}
}

// TestTheClosingWord: a failure is findings, a check not run is not ready, and
// only a run in which every check passed is done.
func TestTheClosingWord(t *testing.T) {
	t.Parallel()
	pass, fail, skip := model.PluginCheck{Status: model.CheckPassed}, model.PluginCheck{Status: model.CheckFailed}, model.PluginCheck{Status: model.CheckNotRun}
	thirteen := make([]model.PluginCheck, model.PluginChecks)
	for i := range thirteen {
		thirteen[i] = model.PluginCheck{N: i + 1, Status: model.CheckPassed}
	}
	twice := slices.Clone(thirteen)
	twice[len(twice)-1].N = 1
	for _, c := range []struct {
		checks []model.PluginCheck
		want   model.Word
	}{
		{thirteen, model.Done},
		{[]model.PluginCheck{pass, pass}, model.NotReady},
		{twice, model.NotReady},
		{thirteen[:len(thirteen)-1], model.NotReady},
		{[]model.PluginCheck{pass, skip}, model.NotReady},
		{[]model.PluginCheck{skip, fail, pass}, model.Findings},
	} {
		if got := (model.PluginTest{Checks: c.checks}).Word(); got != c.want {
			t.Errorf("%v ends %s, want %s", c.checks, got, c.want)
		}
	}
}
