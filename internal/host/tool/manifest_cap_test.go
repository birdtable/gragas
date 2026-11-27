package tool_test

import (
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/tool"
)

// TestAManifestIsReadUpToItsCapAndNoFurther is the test of maxPackageJSON.
// boundary: maxPackageJSON.
func TestAManifestIsReadUpToItsCapAndNoFurther(t *testing.T) {
	t.Parallel()
	for size, wantErr := range map[int]bool{tool.MaxPackageJSON: false, tool.MaxPackageJSON + 1: true} {
		dir := t.TempDir()
		body := `{"x":"` + strings.Repeat("a", size-8) + `"}`
		body += strings.Repeat(" ", size-len(body))
		if err := os.WriteFile(dir+"/package.json", []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := tool.ManifestBytes(strings.ReplaceAll(dir, "\\", "/"), "package.json")
		if (err != nil) != wantErr {
			t.Errorf("a manifest of %d bytes: error = %v, want an error: %v", size, err, wantErr)
		}
	}
}
