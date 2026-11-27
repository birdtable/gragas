package journeys

import (
	"encoding/json"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// standinWithout is the stand-in's bundle under a manifest that leaves op out
// of provides, and without the cases of op: the module built to leave op out,
// so what C15 probes is a live module. It is a function, not a
// global.
func standinWithout(t *testing.T, op string) (dir, bundle string) {
	t.Helper()
	module := configtest.StandinWasmWithout(t, op)
	manifest := strings.Replace(standinManifest, "provides: [plan, entries, findings, rank, preset]", "provides: "+without(op), 1)
	if manifest == standinManifest {
		t.Fatal("the manifest has no provides line to change")
	}
	dir = t.TempDir()
	files := map[string]string{
		"plugins/standin/1.0.0/plugin.yaml":                       manifest + wasmLine(module),
		"plugins/standin/1.0.0/plugin.wasm":                       string(module),
		"plugins/standin/1.0.0/testdata/recordings/1.0.0/out.txt": "raw output",
	}
	for name, text := range standinCases() {
		if !strings.Contains(name, "-"+op+"/") {
			files["plugins/standin/1.0.0/testdata/cases/"+name] = text
		}
	}
	put(t, dir, files)
	return dir, "plugins/standin/1.0.0"
}

// without is the list of the five operations but op.
func without(op string) string {
	var out []string
	for _, o := range []string{"plan", "entries", "findings", "rank", "preset"} {
		if o != op {
			out = append(out, o)
		}
	}
	return "[" + strings.Join(out, ", ") + "]"
}

// TestC15ProbesALiveModuleForAnOperationTheManifestLeavesOut: the stand-in's module under a manifest that provides no rank is
// run by gragas plugin test, and C15 passes because the module answers
// unsupported to the well-formed rank the probe sends. Risk: C15 passing with
// nothing probed, or the probe's request being malformed so the SDK's own
// check answers bug and the check tests the SDK, not the plugin. Mutation:
// put back the probe's empty pairs and C15 fails with bug; make the stand-in
// answer rank and C15 fails with succeeded.
func TestC15ProbesALiveModuleForAnOperationTheManifestLeavesOut(t *testing.T) {
	t.Parallel()
	dir, bundle := standinWithout(t, "rank")
	r := pluginRun(t, dir, "80", bundle, "--json")
	var d struct {
		Checks []struct {
			N      int
			Status string
			Why    string
		}
	}
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
		t.Fatalf("--json: exit %d, %v\n%s%s", r.code, err, r.stdout, r.stderr)
	}
	t.Logf("%s", r.stdout)
	rows := 0
	for _, c := range d.Checks {
		if c.N != 15 {
			continue
		}
		rows++
		if c.Status != "passed" {
			t.Errorf("C15: %s, want passed\n%s", c.Status, r.stdout)
		}
	}
	if rows != 1 {
		t.Errorf("%d C15 rows, want exactly one\n%s", rows, r.stdout)
	}
	if r.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", r.code, r.stdout, r.stderr)
	}
}
