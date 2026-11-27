package internal

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// TestAnAlsoRowThatNamesTestFilesSparesNoProductFile: the row of go-files/trust/trusttest for cmd/gragas-dev covers its
// test files only (gragas#1765), so a product file of cmd/gragas-dev that imports trusttest is red, with the
// table's reason, and a test file of it that does the same stays green. The control is the module with
// the test file alone, so the lint is running and reading the row.
func TestAnAlsoRowThatNamesTestFilesSparesNoProductFile(t *testing.T) {
	tb := table(t)
	const lib = "github.com/birdtable/go-files"
	e, ok := tb.Owner(lib + "/trust/trusttest")
	if !ok {
		t.Fatalf("the table has no row for %s/trust/trusttest", lib)
	}
	covered := false
	for _, a := range e.Also {
		if a.Owner == "cmd/gragas-dev" {
			covered = len(a.Files) == 1 && a.Files[0] == "tests"
		}
	}
	if !covered {
		t.Fatalf("the also row of cmd/gragas-dev for %s/trust/trusttest does not name its test files only: %+v", lib, e.Also)
	}
	use := fmt.Sprintf("import \"%s/trust/trusttest\"\n\nvar _ = trusttest.One\n", lib)
	files := func(product bool) map[string]string {
		m := map[string]string{
			".golangci.yml":      lintConfig(t),
			"go.mod":             "module " + tb.Module + "\n\ngo 1.23.0\n\nrequire " + lib + " v0.0.0\n",
			"vendor/modules.txt": "# " + lib + " v0.0.0\n## explicit\n" + lib + "/trust/trusttest\n",
			"vendor/" + lib + "/trust/trusttest/trusttest.go": "package trusttest\n\n// One is a value.\nvar One = 1\n",
			"cmd/gragas-dev/main.go":                          "package main\n\nfunc main() {}\n",
			"cmd/gragas-dev/trusttest_test.go":                "package main\n\n" + use,
		}
		if product {
			m["cmd/gragas-dev/main.go"] = "package main\n\n" + use + "\nfunc main() {}\n"
		}
		return m
	}
	bin := pinnedLint(t)
	depguard := func() *exec.Cmd {
		return exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--default=none", "--enable=depguard", "./...")
	}
	if out, ok := lint(t, bin, writeModule(t, files(false)), depguard()); !ok {
		t.Fatalf("a test file of cmd/gragas-dev importing trusttest is refused:\n%s", out)
	}
	out, ok := lint(t, bin, writeModule(t, files(true)), depguard())
	if ok || !strings.Contains(out, "cmd/gragas-dev/main.go") || !strings.Contains(out, lib+"/trust/trusttest") || strings.Contains(out, "trusttest_test.go") {
		t.Errorf("a product file of cmd/gragas-dev importing trusttest gave ok=%v, want it refused and the test file spared:\n%s", ok, out)
	}
}
