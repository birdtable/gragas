package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// put writes a file of the test's repository.
func put(t *testing.T, dir, rel, text string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestThePlannedPackagesAreWrittenAndTheProseAroundThemStays: the block is
// the planned: list of layers.yaml, and a change to the list is a drift that
// --check tells.
func TestThePlannedPackagesAreWrittenAndTheProseAroundThemStays(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	if code, _, errs := runIn(t, "-C", dir); code != 0 {
		t.Fatalf("generate = %d %q", code, errs)
	}
	arch := read(t, dir, archFile)
	if !strings.Contains(arch, "| `internal/core/format` | this repository | created in M1 |") {
		t.Errorf("the planned block is missing a package:\n%s", arch)
	}
	if !strings.Contains(arch, "Prose.") || !strings.Contains(arch, "## 17. v6") {
		t.Errorf("the prose around the block changed:\n%s", arch)
	}
	put(t, dir, layersFile, "planned:\n  - created: M2\n    layer: core\n    packages: [internal/core/other]\n")
	if code, _, errs := runIn(t, "-C", dir, "--check"); code == 0 || !strings.Contains(errs, archFile) {
		t.Errorf("--check after a change to layers.yaml = %d %q, want failed naming %s", code, errs, archFile)
	}
}

// TestAnArchitectureWithoutTheBlockIsTold: the marked block is what the
// generator replaces, and it does not guess where one would go.
func TestAnArchitectureWithoutTheBlockIsTold(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	put(t, dir, archFile, "# a\n")
	if code, _, errs := runIn(t, "-C", dir); code == 0 || !strings.Contains(errs, "has no block marked") {
		t.Errorf("exit %d: %q", code, errs)
	}
}
