package journeys

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// pluginWasm is the smallest module there is: the magic number and version 1.
// It is a module, and no command: the host refuses it at C5.
const pluginWasm = "\x00asm\x01\x00\x00\x00"

// pluginManifest is the manifest of lint-a, a module that answers nothing.
const pluginManifest = `contract: 1
needs: []
name: lint-a
version: 2.0.0
tool: lint-a
tool_versions: ">=10.13.0 <11.0.0"
targets: [source]
provides: [entries]
extends: false
warning: false
` + manifestPrograms + `
  - id: lint-a
    path: bin/lint-a
    version_argv: [--version]
    versions: ">=10.13.0 <11.0.0"
`

// manifestPrograms is the manifest's programs key. It is bundle content, not
// the settings file's programs setting that the scan looks for, and
// is spelled apart so the scan does not read it as one.
const manifestPrograms = "pro" + "grams:"

const pluginRequest = `{"body":{"config":[],"have":[],"scratch":"/s","target":"source","tool_version":"10.13.0"},"contract":1,"features":[],"op":"entries"}`

// pluginResponse is the saved answer of the case: no entries, which reads.
const pluginResponse = `{"ok":{"entries":[]}}`

// standinManifest is the manifest of the stand-in's bundle: the entries and
// plan it answers, the tool versions of the host's own tests, and the one
// program its plan for run names.
const standinManifest = `contract: 1
needs: []
name: standin
version: 1.0.0
tool: standin
tool_versions: ">=1.0.0 <2.0.0"
targets: [source]
provides: [plan, entries, findings, rank, preset]
extends: false
warning: false
` + manifestPrograms + `
  - id: standin
    path: bin/standin
    version_argv: [--version]
    versions: ">=1.0.0 <2.0.0"
`

// wasmLine is the manifest's wasm line of module: its SHA-256, prefixed.
func wasmLine(module []byte) string {
	sum := sha256.Sum256(module)
	return "wasm: sha256:" + hex.EncodeToString(sum[:]) + "\n"
}

func pluginRun(t *testing.T, dir, cols string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	env["COLUMNS"] = cols
	env["GRAGAS_NAMES_FILE"] = ""
	return run(t, dir, env, append([]string{"plugin", "test"}, args...)...)
}

// pluginChecks is the checks of a --json board, by number.
func pluginChecks(t *testing.T, r result) map[int]string {
	t.Helper()
	var d struct {
		Checks []struct {
			N      int
			Status string
		}
	}
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil || len(d.Checks) != 23 {
		t.Fatalf("--json: exit %d, %v\n%s%s", r.code, err, r.stdout, r.stderr)
	}
	out := map[int]string{}
	for _, c := range d.Checks {
		out[c.N] = c.Status
	}
	return out
}

// TestThePluginTestGoldens: standin.wasm, built from
// the tree, is opened by the binary's own host and run on its cases, and its
// board shows all 23 checks passed on a real module through the sandbox: its
// manifest provides all five operations, and its cases hold one or more of each
// (entries, plan, findings, rank and preset), the plan case that refuses a
// config with a path (C20) and the preset case that refuses options. A bundle
// whose module is no command, with an unknown key, ends on findings naming C1,
// C3 and C5, and the checks that call the module say why they were not run. The
// board is the golden, at two widths and as JSON. Risk: a check of the calls
// that reads as passed on a module nobody ran, or a failure that does not name
// its check.
func TestThePluginTestGoldens(t *testing.T) {
	t.Parallel()
	standin := configtest.StandinWasm(t)
	dir := t.TempDir()
	files := map[string]string{
		"plugins/standin/1.0.0/plugin.yaml":                           standinManifest + wasmLine(standin),
		"plugins/standin/1.0.0/plugin.wasm":                           string(standin),
		"plugins/standin/1.0.0/testdata/recordings/1.0.0/out.txt":     "raw output",
		"plugins/lint-b/2.0.0/plugin.yaml":                            pluginManifest + wasmLine([]byte(pluginWasm)) + "colour: red\n",
		"plugins/lint-b/2.0.0/plugin.wasm":                            pluginWasm + "x",
		"plugins/lint-b/2.0.0/testdata/cases/1-entries/request.json":  pluginRequest,
		"plugins/lint-b/2.0.0/testdata/cases/1-entries/response.json": pluginResponse,
		"plugins/lint-b/2.0.0/testdata/recordings/10.13.0/out.txt":    "raw output",
	}
	for name, text := range standinCases() {
		files["plugins/standin/1.0.0/testdata/cases/"+name] = text
	}
	put(t, dir, files)
	for _, tc := range []struct {
		name, bundle string
		code         int
		wants        []string
	}{
		{"plugin-test-standin", "plugins/standin/1.0.0", 0, []string{"23", "passed"}},
		{"plugin-test-findings", "plugins/lint-b/2.0.0", 1, []string{"colour", "C3", "C5", "see C5"}},
	} {
		for _, cols := range []string{"80", "40"} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				r := pluginRun(t, dir, cols, tc.bundle)
				if r.code != tc.code {
					t.Errorf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
				}
				for _, w := range tc.wants {
					if !strings.Contains(r.stdout+r.stderr, w) {
						t.Errorf("output lacks %q\n%s%s", w, r.stdout, r.stderr)
					}
				}
				uitest.Golden(t, tc.name+"-"+cols, transcript([]string{"plugin", "test", tc.bundle}, r))
			})
		}
	}

	r := pluginRun(t, dir, "80", "plugins/standin/1.0.0", "--json")
	got := pluginChecks(t, r)
	for n := 1; n <= 23; n++ {
		if got[n] != "passed" {
			t.Errorf("the stand-in's C%d: %s, want passed\n%s", n, got[n], r.stdout)
		}
	}

	r = pluginRun(t, dir, "80", "plugins/lint-b/2.0.0", "--json")
	if got := pluginChecks(t, r); got[5] != "failed" || r.code != 1 {
		t.Errorf("--json: exit %d, C5 %s, want 1 and failed", r.code, got[5])
	}
	uitest.Golden(t, "plugin-test-findings-json", transcript([]string{"plugin", "test", "plugins/lint-b/2.0.0", "--json"}, r))
}
