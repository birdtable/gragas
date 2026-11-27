package internal

// The parallel-test linters of the shared preset (go.cycle.2, tests.runners.4):
// the preset shows a serial test red and one that says why green, and the
// packages still serial are the exclusion of this repository's lint, no more.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// parallelPlants is a module of the plants of paralleltest and tparallel, one
// package each: the serial test and the half-parallel one are red, and the
// parallel test and the serial one with its reason are green.
func parallelPlants(preset string) map[string]string {
	pkg := func(name, body string) string {
		return "// Package " + name + " is a plant of the parallel-test linters.\npackage " + name + "\n\nimport \"testing\"\n\n" + body
	}
	return map[string]string{
		".golangci.yml":       preset,
		"go.mod":              "module example.com/planted\n\ngo 1.23.0\n",
		"serial/x_test.go":    pkg("serial", "func TestAlone(t *testing.T) { t.Log(\"alone\") }\n"),
		"halfway/x_test.go":   pkg("halfway", "func TestParent(t *testing.T) {\n\tt.Parallel()\n\tt.Run(\"child\", func(t *testing.T) { t.Log(\"child\") })\n}\n"),
		"parallel/x_test.go":  pkg("parallel", "func TestTogether(t *testing.T) {\n\tt.Parallel()\n\tt.Run(\"child\", func(t *testing.T) {\n\t\tt.Parallel()\n\t\tt.Log(\"child\")\n\t})\n}\n"),
		"reasoned/x_test.go":  pkg("reasoned", "//nolint:paralleltest // serial: it sets an environment variable\nfunc TestAlone(t *testing.T) { t.Log(\"alone\") }\n"),
		"reasoned2/x_test.go": pkg("reasoned2", "func TestOne(t *testing.T) {\n\tt.Parallel()\n\tt.Log(\"one\")\n}\n"),
	}
}

// TestThePresetShowsASerialTestRedAndOneWithItsReasonGreen runs the pinned
// golangci-lint with presets/golangci-lint/.golangci.yml over a module of plants:
// a test with no t.Parallel is reported by paralleltest, a parent that is
// parallel over a subtest that is not by tparallel, and the parallel test, the
// one whose suppression of paralleltest gives a serial: reason and the second parallel test
// are reported by nothing, nolintlint included.
// Planted failure: the preset drops paralleltest or tparallel, or lets a
// serial test pass.
func TestThePresetShowsASerialTestRedAndOneWithItsReasonGreen(t *testing.T) {
	preset, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	bin := pinnedLint(t)
	dir := writeModule(t, parallelPlants(string(preset)))
	out, ok := lint(t, bin, dir, exec.CommandContext(t.Context(), "golangci-lint", "run",
		"--enable-only=paralleltest,tparallel,nolintlint", "--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./..."))
	if ok {
		t.Fatalf("the plants are not reported:\n%s", out)
	}
	for _, want := range []string{"serial/x_test.go", "(paralleltest)", "halfway/x_test.go", "(tparallel)"} {
		if !strings.Contains(out, want) {
			t.Errorf("the lint does not say %q:\n%s", want, out)
		}
	}
	for _, spared := range []string{"parallel/x_test.go", "reasoned/x_test.go", "reasoned2/x_test.go"} {
		for line := range strings.SplitSeq(out, "\n") {
			if strings.HasPrefix(line, spared) {
				t.Errorf("a control is reported: %s", line)
			}
		}
	}
}

// TestTheSerialPackagesAreTheLintsExclusion: gate/serial-packages.txt, which
// the serial-packages baseline counts, lists each package once, sorted, with
// a test file in it, and the exclusion of paralleltest and tparallel in
// .golangci.yml spares those packages and no other.
// Planted failure: a package in the exclusion and not the list, or in the
// list and not the exclusion, or listed twice.
func TestTheSerialPackagesAreTheLintsExclusion(t *testing.T) {
	t.Parallel()
	list := serialPackages(t)
	if !slices.IsSorted(list) || len(slices.Compact(slices.Clone(list))) != len(list) {
		t.Error("gate/serial-packages.txt is not sorted, one package a line")
	}
	var spared []exclusion
	for _, r := range exclusions(t) {
		if slices.Contains(r.linters, "paralleltest") || slices.Contains(r.linters, "tparallel") {
			spared = append(spared, r)
			if !slices.Contains(r.linters, "paralleltest") || !slices.Contains(r.linters, "tparallel") || r.text != "" {
				t.Errorf("the exclusion %q names one of the two linters, or only some of their findings: it names both, whole", r.path)
			}
		}
	}
	if len(spared) != 1 {
		t.Fatalf("%d exclusions of paralleltest or tparallel: one rule holds the list", len(spared))
	}
	rule := regexp.MustCompile(spared[0].path)
	for _, pkg := range list {
		tests, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pkg), "*_test.go"))
		if len(tests) == 0 {
			t.Errorf("%s is listed as serial and has no test file", pkg)
		}
		if !rule.MatchString(pkg + "/x_test.go") {
			t.Errorf("%s is listed in gate/serial-packages.txt and not excluded from the lint", pkg)
		}
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") && path != root || d.Name() == "testdata" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || d.IsDir() || !strings.HasSuffix(rel, "_test.go") {
			return err
		}
		if pkg := filepath.ToSlash(filepath.Dir(rel)); rule.MatchString(filepath.ToSlash(rel)) && !slices.Contains(list, pkg) {
			t.Errorf("%s is excluded from the lint and not listed in gate/serial-packages.txt", pkg)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// parallelLinters are the two linters the preset turns on for go.cycle.2 and
// tests.runners.4.
func parallelLinters() []string { return []string{"paralleltest", "tparallel"} }

// absentLinters are the wanted linters that enable does not hold.
func absentLinters(enable, want []string) []string {
	return slices.DeleteFunc(slices.Clone(want), func(l string) bool { return slices.Contains(enable, l) })
}

// TestThePresetEnablesBothParallelLinters reads linters.enable of
// presets/golangci-lint/.golangci.yml, since the runner test's --enable-only turns the
// linters on whatever the preset says.
// Planted failure: either linter commented out of the preset's enable list.
func TestThePresetEnablesBothParallelLinters(t *testing.T) {
	t.Parallel()
	enable := ordered.Strings(listAt(lintDoc(t, "presets/golangci-lint/.golangci.yml"), "linters.enable"))
	for _, l := range absentLinters(enable, parallelLinters()) {
		t.Errorf("presets/golangci-lint/.golangci.yml does not enable %s", l)
	}
	if got := absentLinters([]string{"paralleltest"}, parallelLinters()); !slices.Equal(got, []string{"tparallel"}) {
		t.Errorf("a preset that drops tparallel is not seen: %v", got)
	}
}

// withoutSerialExclusion is the lint config with the rule that spares the
// serial packages taken out: the rule's line and the linters line under it.
func withoutSerialExclusion(cfg string) (string, bool) {
	lines := strings.Split(cfg, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "linters: [paralleltest, tparallel]" && strings.Contains(lines[i-1], "- path: ^(") {
			return strings.Join(slices.Concat(lines[:i-1], lines[i+1:]), "\n"), true
		}
	}
	return cfg, false
}

// findingPackage reads the package folder of a finding line of the lint.
var findingPackage = regexp.MustCompile(`(?m)^(\S+)/[^/\s]+_test\.go:\d+:\d+: `)

// staleSerial are the listed packages the lint, with the exclusion dropped,
// finds nothing in, and the packages it finds something in that are not listed.
func staleSerial(list []string, out string) (stale, unlisted []string) {
	found := map[string]bool{}
	for _, m := range findingPackage.FindAllStringSubmatch(out, -1) {
		found[m[1]] = true
		if !slices.Contains(list, m[1]) && !slices.Contains(unlisted, m[1]) {
			unlisted = append(unlisted, m[1])
		}
	}
	for _, pkg := range list {
		if !found[pkg] {
			stale = append(stale, pkg)
		}
	}
	return stale, unlisted
}

// TestAListedPackageStillHasASerialTest runs the pinned golangci-lint with
// paralleltest and tparallel alone over the whole tree, the exclusion dropped:
// a listed package it finds nothing in has become parallel and leaves
// gate/serial-packages.txt with the count lowered, and a package it finds
// something in is listed.
// Planted failure: t.Parallel in every test of a listed package, which stays
// listed.
func TestAListedPackageStillHasASerialTest(t *testing.T) {
	list := serialPackages(t)
	if stale, unlisted := staleSerial([]string{"a", "b"}, "a/x_test.go:3:1: Function TestA missing t.Parallel (paralleltest)\nc/y_test.go:4:1: x (tparallel)\n"); !slices.Equal(stale, []string{"b"}) || !slices.Equal(unlisted, []string{"c"}) {
		t.Fatalf("the check does not tell a stale package from an unlisted one: %v, %v", stale, unlisted)
	}
	cfg, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	bare, ok := withoutSerialExclusion(string(cfg))
	if !ok {
		t.Fatal(".golangci.yml has no exclusion rule of paralleltest and tparallel to drop")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	confDir := t.TempDir()
	conf := filepath.Join(confDir, "golangci.yml")
	if err := os.WriteFile(conf, []byte(bare), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := pinnedLint(t)
	module := regexp.MustCompile(`(?m)^module (\S+)$`).FindStringSubmatch(read(t, "go.mod"))
	if module == nil {
		t.Fatal("go.mod names no module")
	}
	out := serialLintOutput(t, bin, abs, module[1], conf)
	// The lint names a file by its absolute path, resolved through any link.
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	out = strings.ReplaceAll(out, filepath.ToSlash(real)+"/", "")
	stale, unlisted := staleSerial(list, out)
	for _, pkg := range stale {
		t.Errorf("%s is listed as serial and has no serial test: take it off gate/serial-packages.txt and its exclusion, and lower the serial-packages count", pkg)
	}
	for _, pkg := range unlisted {
		t.Errorf("%s has serial tests and is not listed: make them parallel, or give each a serial: reason", pkg)
	}
}

// serialPackages are the packages gate/serial-packages.txt lists.
func serialPackages(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "gate", "serial-packages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var list []string
	for l := range strings.SplitSeq(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			list = append(list, l)
		}
	}
	return list
}

// serialLintOutput is the findings of paralleltest and tparallel alone, the
// exclusion dropped (conf), over the module at abs, for the host. The lint of
// another system's files (scripts/os-packages, gragas#800, #802) went with the
// tier runner (gragas#911): a serial test in a file built for another system is
// judged by the runner on that system.
func serialLintOutput(t *testing.T, bin, abs, module, conf string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--enable-only=paralleltest,tparallel", "--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "--path-mode=abs", "./...")
	cmd.Args = append(cmd.Args, "--config", conf)
	out, _ := lint(t, bin, abs, cmd)
	return out
}
