package main

import (
	gomaps "maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// plantModule writes files, by slash path, under a new folder and returns it.
func plantModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// onePlatformModule is a module whose shipped command imports a package on
// one platform only, with more laid over it.
func onePlatformModule(t *testing.T, more map[string]string) string {
	t.Helper()
	files := map[string]string{
		"go.mod":                    "module example.com/shipped\n\ngo 1.22\n",
		"cmd/gragas/main.go":        "package main\n\nimport _ \"example.com/shipped/internal/always\"\n\nfunc main() {}\n",
		"cmd/gragas/only_darwin.go": "package main\n\nimport _ \"example.com/shipped/internal/onlydarwin\"\n",
		"internal/always/a.go":      "package always\n",
		"internal/onlydarwin/a.go":  "package onlydarwin\n",
	}
	gomaps.Copy(files, more)
	return plantModule(t, files)
}

// TestAPlatformWhoseClosureCannotBeReadFailsTheRead: a closure one platform
// cannot read is no closure: the read of the shipped binary's modules fails,
// naming the platform, rather than joining the others' and leaving that
// platform's modules off the list. The control is the same module with
// nothing broken.
func TestAPlatformWhoseClosureCannotBeReadFailsTheRead(t *testing.T) {
	t.Parallel()
	if _, err := goLinkedModules(t.Context(), os.Environ(), onePlatformModule(t, nil)); err != nil {
		t.Fatalf("the control does not read: %v", err)
	}
	broken := onePlatformModule(t, map[string]string{
		"cmd/gragas/only_darwin.go": "package main\n\nimport _ \"example.com/shipped/internal/nowhere\"\n",
	})
	_, err := goLinkedModules(t.Context(), os.Environ(), broken)
	if err == nil || !strings.Contains(err.Error(), "the linked modules for darwin/") {
		t.Errorf("%v, want the read to fail naming a darwin platform", err)
	}
}
