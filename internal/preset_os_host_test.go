package internal

// The operating-system boundary of the shared preset (arch.ports.24): the
// preset shows an operating-system package, a read of runtime.GOOS and
// runtime.GOARCH red outside the host layer and green inside it, and the rows
// of gate/exemptions.yaml for the files outside the host layer that name an
// operating system are the tree's, no more.

import (
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/ordered"
)

// hostLayerSlot is the os-host row's files line as the preset keeps it, and
// the text a project that has a host layer puts in its place.
const (
	hostLayerSlot = "files: [\"$all\"]\n          # per-project: leave \"$all\": a product has no host layer. A folder that has not\n          # moved its operating-system code yet is spared as \"!**/<folder>/**\", and each file in it\n          # carries a row of the project's exemptions file, which only falls.\n          # slot os-host end"
	hostLayerOwn  = "files: [\"$all\", \"!**/host/**\"]\n          # slot os-host end"
)

// osHostPlants is a module of the plants of the operating-system boundary: a
// package outside the host layer, and one inside it, for each way to name a
// system. The three operating-system modules are stand-ins that a replace
// directive points at, so the plants build with no network.
func osHostPlants(preset string) map[string]string {
	stub := func(path, pkg string) map[string]string {
		dir := "stub/" + strings.ReplaceAll(path, "/", "_")
		return map[string]string{
			dir + "/go.mod": "module " + path + "\n\ngo 1.23.0\n",
			dir + "/p.go":   "// Package " + pkg + " is a stand-in.\npackage " + pkg + "\n\n// Name is a stand-in.\nconst Name = \"x\"\n",
		}
	}
	files := map[string]string{
		".golangci.yml": preset,
		"go.mod": "module example.com/planted\n\ngo 1.23.0\n\n" +
			"require (\n\tgithub.com/gofrs/flock v0.0.0\n\tgithub.com/shirou/gopsutil v0.0.0\n\tgolang.org/x/sys v0.0.0\n)\n\n" +
			"replace (\n\tgithub.com/gofrs/flock => ./stub/github.com_gofrs_flock\n\tgithub.com/shirou/gopsutil => ./stub/github.com_shirou_gopsutil\n\tgolang.org/x/sys => ./stub/golang.org_x_sys\n)\n",
	}
	for path, pkg := range map[string]string{"golang.org/x/sys": "sys", "github.com/gofrs/flock": "flock", "github.com/shirou/gopsutil": "gopsutil"} {
		maps.Copy(files, stub(path, pkg))
	}
	use := func(pkg, imp, name string) string {
		return "// Package " + pkg + " is a plant of the operating-system boundary.\npackage " + pkg + "\n\nimport \"" + imp + "\"\n\n// Use reads the stand-in.\nfunc Use() string { return " + name + ".Name }\n"
	}
	for _, where := range []string{"outside", "host/inside"} {
		pkg := filepath.Base(where)
		files[where+"/"+"sys/x.go"] = use(pkg+"sys", "golang.org/x/sys", "sys")
		files[where+"/flock/x.go"] = use(pkg+"flock", "github.com/gofrs/flock", "flock")
		files[where+"/psutil/x.go"] = use(pkg+"psutil", "github.com/shirou/gopsutil", "gopsutil")
		files[where+"/goos/x.go"] = "// Package " + pkg + "goos is a plant of the operating-system boundary.\npackage " + pkg + "goos\n\nimport \"runtime\"\n\n// Is reads the system.\nfunc Is() string { return runtime.GOOS + runtime.GOARCH }\n"
	}
	files["plain/x.go"] = "// Package plain is a control: it names no system.\npackage plain\n\n// Is names none.\nfunc Is() string { return \"none\" }\n"
	return files
}

// TestThePresetShowsAnOperatingSystemPackageOutsideTheHostLayerRed runs the
// pinned golangci-lint with presets/golangci-lint/.golangci.yml, a project's host layer
// set in the os-host slot and in an exclusion of forbidigo, over a module of
// plants: x/sys, flock, gopsutil and runtime.GOOS and runtime.GOARCH are
// reported in each package outside the layer, and in none inside it, and the
// package that names no system is reported by nothing.
// Planted failure: the preset drops the os-host row or the forbidigo pattern,
// or its files line spares the plants outside the layer.
func TestThePresetShowsAnOperatingSystemPackageOutsideTheHostLayerRed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), hostLayerSlot) {
		t.Fatalf("the preset's os-host row does not read\n%s", hostLayerSlot)
	}
	preset := strings.Replace(string(raw), hostLayerSlot, hostLayerOwn, 1) +
		"      - path: ^host/\n        linters: [forbidigo]\n        text: \"arch\\\\.ports\\\\.24:\"\n"
	bin := pinnedLint(t)
	dir := writeModule(t, osHostPlants(preset))
	out, ok := lint(t, bin, dir, exec.CommandContext(t.Context(), "golangci-lint", "run",
		"--enable-only=depguard,forbidigo", "--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./..."))
	if ok {
		t.Fatalf("the plants are not reported:\n%s", out)
	}
	for _, want := range []string{
		"outside/sys/x.go", "outside/flock/x.go", "outside/psutil/x.go", "outside/goos/x.go",
		"(depguard)", "(forbidigo)", "arch.ports.24",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the lint does not say %q:\n%s", want, out)
		}
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "no-os-packages") {
			continue // the whole-package row refuses x/sys in the host layer too
		}
		for _, spared := range []string{"host/inside/", "plain/"} {
			if strings.HasPrefix(line, spared) {
				t.Errorf("a control is reported: %s", line)
			}
		}
	}
}

// osRows is each path the live rows of gate/exemptions.yaml give arch.ports.24.
func osRows(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "gate", "exemptions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		t.Fatalf("gate/exemptions.yaml is no mapping: %v", err)
	}
	var rows []string
	for _, r := range ordered.Maps(doc.List("exemptions")) {
		if r.Str("rule") == "arch.ports.24" {
			rows = append(rows, r.Str("path"))
		}
	}
	return rows
}

// TestTheOsFilesOutsideTheHostLayerAreTheRows: gragas has no host layer
// (gragas#1605), so lint/project.yml's os-host row spares no
// folder, and every Go file that names an operating system, by the test
// gragas check makes (audit.NamesAnOS), internal/host included, has its
// arch.ports.24 row in gate/exemptions.yaml or its line in
// gate/os-code-allowlist.txt, the list, which only shrinks. The rows
// name each file once, and each file the os-host row spares alone has one.
// Planted failure: an operating-system file with neither, a row whose file is
// gone or names no system, a file listed twice, a file the lint spares with
// no row, or a folder the os-host row spares.
func TestTheOsFilesOutsideTheHostLayerAreTheRows(t *testing.T) {
	t.Parallel()
	rows := osRows(t)
	if len(slices.Compact(slices.Sorted(slices.Values(rows)))) != len(rows) {
		t.Error("gate/exemptions.yaml gives one file two arch.ports.24 rows")
	}
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "vendor" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		named, err := audit.NamesAnOS(d.Name(), body)
		if err != nil {
			return err
		}
		if named {
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	held := osCodeHeld(t)
	for _, f := range found {
		if !slices.Contains(rows, f) && !held[f] {
			t.Errorf("%s names an operating system and has no arch.ports.24 row in gate/exemptions.yaml nor a line in %s: the code belongs to a platform library", f, osCodeAllowlist)
		}
	}
	for _, f := range rows {
		if !slices.Contains(found, f) {
			t.Errorf("gate/exemptions.yaml's arch.ports.24 row for %s names a file that is gone or names no operating system: remove the row", f)
		}
	}
	project, err := os.ReadFile(filepath.Join(root, "lint", "project.yml"))
	if err != nil {
		t.Fatal(err)
	}
	spared := regexp.MustCompile(`"!\*\*/([^"*]+\.go)"`)
	_, slot, _ := strings.Cut(string(project), "\nos-host: |")
	slot, _, _ = strings.Cut(slot, "\n\n")
	for _, m := range spared.FindAllStringSubmatch(slot, -1) {
		if !slices.Contains(rows, m[1]) {
			t.Errorf("lint/project.yml's os-host row spares %s, which has no arch.ports.24 row in gate/exemptions.yaml", m[1])
		}
	}
	// The slot's folders are the host layer for depguard and for gragas check's
	// os-files alike, and gragas has none: no folder may import golang.org/x/sys,
	// flock or gopsutil, and none is skipped by this walk.
	if folders := regexp.MustCompile(`"!\*\*/([^"*]+)/\*\*"`).FindAllStringSubmatch(slot, -1); len(folders) != 0 {
		t.Errorf("lint/project.yml's os-host row spares the folders %v: gragas has no host layer (gragas#1605)", folders)
	}
}

// osCodeHeld is each path of gate/os-code-allowlist.txt, the list of
// the files that carry operating-system code today.
func osCodeHeld(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, osCodeAllowlist))
	if err != nil {
		t.Fatal(err)
	}
	held := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if path, _, _ := strings.Cut(line, " "); path != "" && !strings.HasPrefix(path, "#") {
			held[path] = true
		}
	}
	return held
}
