package configtest

import (
	"os"
	"path/filepath"
	"testing"

	testenv "github.com/birdtable/go-testkit"
)

// TestAGoModThatIsADanglingLinkDoesNotAnchorTheModuleRoot: the
// build of standin.wasm finds the module's root by go-files' confine.Nearest,
// which counts a go.mod that is a link to nothing; moduleRootFrom gives the
// answer of the os.Stat walk before it and goes on above such a go.mod to the
// real one. Planted failure: the os.Stat check of the found folder taken out
// answers the folder of the dangling link.
func TestAGoModThatIsADanglingLinkDoesNotAnchorTheModuleRoot(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "top")
	deep := filepath.Join(dir, "near", "deep")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/top\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, "gone", filepath.Join(dir, "near", "go.mod"))
	if got, err := moduleRootFrom(deep); err != nil || got != dir {
		t.Errorf("moduleRootFrom = %q, %v; want %q, above the dangling go.mod", got, err, dir)
	}
}
