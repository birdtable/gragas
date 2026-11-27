package internal

import (
	"os/exec"
	"strings"
	"testing"
)

// TestAGateReaderThatTakesADuplicateKeyIsRefused: gragas#1733, class 4. In the
// gate registry, the repo checks and the dev tool, a call of ordered.ParseYAML,
// which takes a key twice, is refused by the pinned forbidigo, naming ParseStrict; the same call
// in another folder, and ParseStrict in the gate, stay green. The control is the
// green file in the same module, so the lint is running and reading the row.
func TestAGateReaderThatTakesADuplicateKeyIsRefused(t *testing.T) {
	tb := table(t)
	const stub = "package ordered\n\n// ParseYAML reads a document.\nfunc ParseYAML([]byte) (any, error) { return nil, nil }\n\n// ParseStrict reads a document.\nfunc ParseStrict([]byte) (any, error) { return nil, nil }\n"
	use := func(pkg, fn string) string {
		return "package " + pkg + "\n\nimport \"" + tb.Module + "/internal/ordered\"\n\n// Read reads.\nfunc Read() error {\n\t_, err := ordered." + fn + "(nil)\n\treturn err\n}\n"
	}
	bin := pinnedLint(t)
	forbidigo := func() *exec.Cmd {
		return exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--default=none", "--enable=forbidigo", "./...")
	}
	base := func() map[string]string {
		return map[string]string{
			".golangci.yml":                  lintConfig(t),
			"go.mod":                         "module " + tb.Module + "\n\ngo 1.23.0\n",
			"internal/ordered/ordered.go":    stub,
			"internal/core/reader/reader.go": use("reader", "ParseYAML"),
			"internal/dev/gate/strict.go":    use("gate", "ParseStrict"),
		}
	}
	if out, ok := lint(t, bin, writeModule(t, base()), forbidigo()); !ok {
		t.Fatalf("ParseYAML outside the gate and ParseStrict inside it are refused:\n%s", out)
	}
	for _, dir := range []string{"internal/dev/gate", "internal/repo/rulebook", "cmd/gragas-dev"} {
		files := base()
		files[dir+"/plant/plant.go"] = use("plant", "ParseYAML")
		out, ok := lint(t, bin, writeModule(t, files), forbidigo())
		if ok || !strings.Contains(out, dir+"/plant/plant.go") || !strings.Contains(out, "ordered.ParseStrict") {
			t.Errorf("a ParseYAML call in %s gave ok=%v, want it refused and naming ParseStrict:\n%s", dir, ok, out)
		}
	}
}
