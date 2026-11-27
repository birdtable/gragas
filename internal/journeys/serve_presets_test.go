package journeys

import (
	"os"
	"path/filepath"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// presetFile is the file the stand-in writes as golangci-lint's preset: release 1
// ships no built-in adapter, so a preset is written by an installed
// plugin, and the manifest below names this one file.
const presetFile = "standin.conf"

// presetOf is where the presets journeys look for golangci-lint's preset.
func presetOf(dir string) string {
	return filepath.Join(dir, "presets", "golangci-lint", presetFile)
}

// servePresets installs and approves, in the repository at dir, the stand-in as
// the plugin of golangci-lint that writes its preset, by do: each argument list
// is one command of the binary run in dir (a crash seed hands in its child, whose
// clock is fixed, so every root holds the same history). The bundle lies below
// the repository, so the lock names it by a relative path.
func servePresets(t *testing.T, dir string, do func(args ...string) result) {
	t.Helper()
	module := configtest.StandinWasm(t)
	manifest := "contract: 1\nneeds: []\nname: standin\nversion: 1.0.0\ntool: golangci-lint\ntool_versions: \">=1.0.0 <2.0.0\"\ntargets: [source]\nprovides: [preset]\nextends: false\nwarning: false\n" +
		manifestPrograms + "\n  - id: golangci-lint\n    path: bin/golangci-lint\n    version_argv: [--version]\n    versions: \">=1.0.0 <2.0.0\"\npreset:\n  path: " + presetFile + "\n  inputs: []\n  mark: true\nwasm: " + sha256Of(module) + "\n"
	folder := filepath.Join(dir, authorFolder)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, folder, map[string]string{"plugin.yaml": manifest, "plugin.wasm": string(module)})
	if r := do("plugin", "add", authorFolder, "--apply"); r.code != 0 {
		t.Fatalf("plugin add: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	shown := do("plugin", "approve", "standin@1.0.0")
	m := recordDigest.FindString(shown.stdout + shown.stderr)
	if m == "" {
		t.Fatalf("plugin approve shows no digest\n%s%s", shown.stdout, shown.stderr)
	}
	if r := do("plugin", "approve", "standin@1.0.0", "--digest", m, "--apply"); r.code != 0 {
		t.Fatalf("plugin approve: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// servedBy is servePresets run through the binary with env.
func servedBy(t *testing.T, dir string, env map[string]string) {
	t.Helper()
	servePresets(t, dir, func(args ...string) result { return run(t, dir, env, args...) })
}
