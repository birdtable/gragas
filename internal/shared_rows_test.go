package internal

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// stdUnder is the first standard package at or below imp, the one a plant
// imports: a row may deny a whole family by its prefix (compress), which is
// no package of its own.
func stdUnder(t *testing.T, imp string) string {
	t.Helper()
	var first string
	err := golist(func(dec *json.Decoder) error {
		var p struct{ ImportPath string }
		if err := dec.Decode(&p); err != nil {
			return err
		}
		if first == "" {
			first = p.ImportPath
		}
		return nil
	}, "-json=ImportPath", "--", imp+"/...")
	if err != nil || first == "" {
		t.Fatalf("no standard package is at or below %s: %v", imp, err)
	}
	return first
}

// mainFile is a file of package main that imports imp and uses nothing of it.
func mainFile(imp string, withMain bool) string {
	body := "package main\n\nimport _ \"" + imp + "\"\n"
	if withMain {
		body += "\nfunc main() {}\n"
	}
	return body
}

// TestEachSharedImportRowIsRedOutsideItsOwner: for each row of gate/shared.yaml
// that is banned and denies an import, a product file of a package that is
// neither the owner nor allowed, importing it, is refused with the row's
// message. The owner's own file and each allowed package's file pass, and a
// test file passes where the row spares test files (gragas#1752). The control
// is the same module without the plant, so the lint is running and reading
// the rows. Risk: a family whose ban renders and stops nothing, or one that
// stops its owner.
func TestEachSharedImportRowIsRedOutsideItsOwner(t *testing.T) {
	s, tb := sharedRegistry(t)
	var rows []layers.Capability
	for _, c := range s.Rows {
		for _, pr := range c.Primitives {
			if c.Banned() && pr.Import != "" {
				rows = append(rows, c)
				break
			}
		}
	}
	if len(rows) == 0 {
		t.Fatal("no banned row of gate/shared.yaml denies an import, so nothing is planted")
	}
	bin := pinnedLint(t)
	depguard := func() *exec.Cmd {
		return exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--default=none", "--enable=depguard", "./...")
	}
	const outside = "cmd/gragas-docs"
	for _, c := range rows {
		for _, pr := range c.Primitives {
			if pr.Import == "" {
				continue
			}
			imp := stdUnder(t, pr.Import)
			t.Run(c.Name+" "+imp, func(t *testing.T) {
				if c.Owner == outside || slices.ContainsFunc(c.Allow, func(a layers.Allowance) bool { return a.Package == outside }) {
					t.Fatalf("%s owns or is allowed %s, so it cannot stand outside", outside, c.Name)
				}
				files := map[string]string{
					".golangci.yml":      lintConfig(t),
					"go.mod":             "module " + tb.Module + "\n\ngo 1.23.0\n",
					c.Owner + "/own.go":  mainFile(imp, true),
					outside + "/main.go": "package main\n\nfunc main() {}\n",
				}
				for _, a := range c.Allow {
					files[a.Package+"/allowed.go"] = mainFile(imp, true)
				}
				if c.Tests != nil {
					files[outside+"/spared_test.go"] = mainFile(imp, false)
				}
				if out, ok := lint(t, bin, writeModule(t, files), depguard()); !ok {
					t.Fatalf("the owner, the allowed packages and the spared test file importing %s are refused:\n%s", imp, out)
				}
				files[outside+"/plant.go"] = mainFile(imp, false)
				out, ok := lint(t, bin, writeModule(t, files), depguard())
				says := layers.SharedRule + ": " + c.Name + " belongs to " + c.Owner
				if ok || !strings.Contains(out, outside+"/plant.go") || !strings.Contains(out, says) || strings.Contains(out, "spared_test.go") {
					t.Errorf("a product file of %s importing %s gave ok=%v, want it refused saying %q and the test file spared:\n%s", outside, imp, ok, says, out)
				}
			})
		}
	}
}
