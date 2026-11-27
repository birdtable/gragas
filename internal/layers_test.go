// Package internal holds the tests over the repository itself, as a sibling tool's
// internal/layers_test.go does: the layer table, layers.yaml, held against
// what go list says the tree is. Every check is a function of the table and
// of a world read once from the tree, so each one is also run once against
// a world with a breach planted in it, to show that it fires.
package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/layers"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// root is the module's root, from this package's folder.
const root = ".."

// pkg is one of the module's packages as go list reads it.
type pkg struct {
	// path is relative to the module: cmd/gragas, internal/core/api; the
	// module's own root package is ".".
	path string
	name string
	// files and tests are the base names of its non-test and its _test.go
	// files.
	files, tests []string
	// imports are the import paths of its non-test files; testImports those
	// of its tests, both kinds, without its own path.
	imports, testImports []string
}

// world is the tree as the checks read it.
type world struct {
	module string
	pkgs   map[string]*pkg
	// files is every file of the tree, by slash path from the root,
	// outside hidden folders and testdata.
	files []string
	// constrained are the .go files with a build constraint line.
	constrained []string
	// testDeps is the module packages each package's test links, from go
	// list -deps -test, by package.
	testDeps map[string][]string
}

// rel is an import path as the table writes it, when it is the module's.
func (w world) rel(imp string) (string, bool) {
	if imp == w.module {
		return ".", true
	}
	r, ok := strings.CutPrefix(imp, w.module+"/")
	return r, ok
}

// sorted is the world's packages in path order.
func (w world) sorted() []*pkg {
	out := make([]*pkg, 0, len(w.pkgs))
	for _, p := range w.pkgs {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b *pkg) int { return strings.Compare(a.path, b.path) })
	return out
}

// std is whether imp is a standard library path: no dot in its first element.
func std(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}

// table is the layer table, layers.yaml.
func table(t *testing.T) *layers.Table {
	t.Helper()
	tb, err := loadTable()
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// loadTable reads layers.yaml, each time its own table: a test may add to it.
func loadTable() (*layers.Table, error) {
	data, err := os.ReadFile(filepath.Join(root, layers.File))
	if err != nil {
		return nil, err
	}
	return layers.Parse(data)
}

// tree is the table and the world as go list and a walk of the tree read
// it, read once for the run by the first test that asks (snap). The world is
// a copy the test may plant a breach in. A walk that finds a Go file go list
// did not read stopped the read: the checks could not run.
func tree(t *testing.T) (*layers.Table, world) {
	t.Helper()
	s := snap()
	if s.err != nil {
		t.Fatal(s.err)
	}
	return table(t), s.world.clone()
}

// golist runs go list in the module's root with args and decodes each JSON
// object it prints into one of fn's calls.
func golist(fn func(dec *json.Decoder) error, args ...string) error {
	return golistOn(nil, fn, args...)
}

// listArgs is go list of every package of the module, what each one holds and imports.
func listArgs() []string {
	return []string{"-e", "-json=ImportPath,Name,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,IgnoredGoFiles,Imports,TestImports,XTestImports", "./..."}
}

// testListArgs is go list of the packages each test binary links.
func testListArgs() []string {
	return []string{"-e", "-deps", "-test", "-json=ImportPath,Imports", "./..."}
}

// runGoList runs go list in the module's root with env added to the
// environment, and answers what it prints.
func runGoList(env []string, args ...string) ([]byte, error) {
	cmd := inRoot(env, "go", append([]string{"list"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("go list: " + err.Error() + "\n" + stderr.String())
	}
	return out, nil
}

// golistOn is golist with env added to the environment: GOOS, GOARCH and
// CGO_ENABLED for one system of the list.
func golistOn(env []string, fn func(dec *json.Decoder) error, args ...string) error {
	out, err := goOut(env, args...)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		if err := fn(dec); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// systems is the list the core compiles for (arch.ports.4): the Taskfile's
// CROSS, as GOOS/GOARCH pairs. A file for one system must not hide an import
// from the checks on another.
func systems() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		return nil, err
	}
	for line := range strings.Lines(string(data)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "CROSS:"); ok {
			rest = strings.Trim(strings.TrimSpace(rest), "[]")
			if list := strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' }); len(list) > 0 {
				return list, nil
			}
		}
	}
	return nil, errors.New("could not run: the Taskfile names no CROSS list of systems")
}

// list reads the module's packages once for the host and once for each
// system of the list, and holds the union: a file one system builds
// contributes its imports whichever system this run is on.
func (w *world) list() error {
	all, err := systems()
	if err != nil {
		return err
	}
	runs := [][]string{nil}
	for _, s := range all {
		goos, goarch, _ := strings.Cut(s, "/")
		runs = append(runs, []string{"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=0"})
	}
	for _, env := range runs {
		if err := w.listOn(env); err != nil {
			return err
		}
	}
	return nil
}

// addNew appends to dst each of src it does not hold.
func addNew(dst []string, src ...string) []string {
	for _, s := range src {
		if !slices.Contains(dst, s) {
			dst = append(dst, s)
		}
	}
	return dst
}

// listOn reads the module's packages for one system, env, into the union.
func (w *world) listOn(env []string) error {
	return golistOn(env, func(dec *json.Decoder) error {
		var lp struct {
			ImportPath, Name                             string
			GoFiles, CgoFiles, TestGoFiles, XTestGoFiles []string
			IgnoredGoFiles                               []string
			Imports, TestImports, XTestImports           []string
		}
		if err := dec.Decode(&lp); err != nil {
			return err
		}
		rel, ok := w.rel(lp.ImportPath)
		if !ok {
			return nil
		}
		p := &pkg{path: rel, name: lp.Name, files: append(lp.GoFiles, lp.CgoFiles...),
			tests: append(lp.TestGoFiles, lp.XTestGoFiles...), imports: lp.Imports}
		// A file another platform builds is listed as ignored. It is a test file or
		// not by its name, whichever platform this run is on.
		for _, f := range lp.IgnoredGoFiles {
			if strings.HasSuffix(f, "_test.go") {
				p.tests = append(p.tests, f)
			} else {
				p.files = append(p.files, f)
			}
		}
		for _, imp := range append(lp.TestImports, lp.XTestImports...) {
			if imp != lp.ImportPath && !slices.Contains(p.testImports, imp) {
				p.testImports = append(p.testImports, imp)
			}
		}
		if old, ok := w.pkgs[rel]; ok {
			old.files = addNew(old.files, p.files...)
			old.tests = addNew(old.tests, p.tests...)
			old.imports = addNew(old.imports, p.imports...)
			old.testImports = addNew(old.testImports, p.testImports...)
			return nil
		}
		w.pkgs[rel] = p
		return nil
	}, listArgs()...)
}

// listTests reads, for each package with tests, the module packages its
// test binary links: go list -deps -test, the closure the compiler sees.
func (w *world) listTests() error {
	graph := map[string][]string{}
	if err := golist(func(dec *json.Decoder) error {
		var lp struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&lp); err != nil {
			return err
		}
		graph[lp.ImportPath] = lp.Imports
		return nil
	}, testListArgs()...); err != nil {
		return err
	}
	w.testDeps = map[string][]string{}
	for _, p := range w.pkgs {
		if len(p.tests) == 0 {
			continue
		}
		main := w.module + "/" + p.path + ".test"
		if p.path == "." {
			main = w.module + ".test"
		}
		w.testDeps[p.path] = w.closure(graph, main, p.path)
	}
	return nil
}

// closure is every module package reachable from the node start of graph,
// but self, by path: a node "x [y.test]" is x built for y's test.
func (w *world) closure(graph map[string][]string, start, self string) []string {
	seen := map[string]bool{start: true}
	queue := []string{start}
	var out []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		plain, _, _ := strings.Cut(n, " ")
		if r, ok := w.rel(plain); ok && r != self && !strings.HasSuffix(plain, ".test") && !strings.HasSuffix(plain, "_test") && !slices.Contains(out, r) {
			out = append(out, r)
		}
		for _, next := range graph[n] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	slices.Sort(out)
	return out
}

// walk reads every file of the tree, and which .go files carry a build
// constraint.
func (w *world) walk() error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		w.files = append(w.files, rel)
		if !strings.HasSuffix(rel, ".go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for line := range strings.Lines(string(data)) {
			if strings.HasPrefix(line, "//go:build") || strings.HasPrefix(line, "// +build") {
				w.constrained = append(w.constrained, rel)
				break
			}
		}
		return nil
	})
}

// counted is the could-not-run check of arch.layers.3: go list saw every Go
// file the walk found, or the checks would pass files they never read.
func (w *world) counted() error {
	listed := 0
	for _, p := range w.pkgs {
		listed += len(p.files) + len(p.tests)
	}
	walked := 0
	for _, f := range w.files {
		if strings.HasSuffix(f, ".go") {
			walked++
		}
	}
	if walked == 0 || listed != walked {
		return errors.New("could not run: go list read " + strconv.Itoa(listed) + " Go files and the tree holds " + strconv.Itoa(walked))
	}
	return nil
}

// unlayered is a sibling tool's TestEveryPackageHasALayer: a package in no row of
// the table, and a row's package the tree does not have.
func unlayered(tb *layers.Table, w world) []string {
	var out []string
	for _, p := range w.sorted() {
		if tb.RowOf(p.path) == nil {
			out = append(out, p.path+": in no layer of "+layers.File)
		}
	}
	for _, r := range tb.Rows {
		for _, name := range r.Packages {
			if w.pkgs[name] == nil {
				out = append(out, layers.File+" lists "+name+", which is no package of the tree")
			}
		}
	}
	return out
}

// edges is every import of the module's packages no row allows: of a
// non-test file, and of a test, which obeys its package's row and may also
// take a stand-in (arch.layers.1, .8, .10, arch.ports.8). A test-only
// package holds no non-test file.
func edges(tb *layers.Table, w world) []string {
	var out []string
	for _, p := range w.sorted() {
		r := tb.RowOf(p.path)
		if r == nil {
			continue
		}
		if r.Kind == layers.KindTests && len(p.files) > 0 {
			out = append(out, p.path+" is test-only and holds "+strings.Join(p.files, ", "))
		}
		for _, imp := range p.imports {
			if to, ok := w.rel(imp); ok && !tb.May(p.path, to, false) {
				out = append(out, p.path+" -> "+to+": "+r.Why)
			}
		}
		for _, imp := range p.testImports {
			if to, ok := w.rel(imp); ok && !tb.May(p.path, to, true) {
				out = append(out, p.path+" (its tests) -> "+to+": "+r.Why)
			}
		}
	}
	return out
}

// TestEveryPackageHasALayer: every package of the tree sits in one row of
// layers.yaml, and every package the table lists is in the tree
// (go.layout.1, arch.layers.3).
func TestEveryPackageHasALayer(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)
	for _, f := range unlayered(tb, w) {
		t.Error(f)
	}
}

// TestTheTreeKeepsTheTable reads the tree once and holds it to every check
// the table and the template make: imports, tests included, on the edges
// the table allows (arch.layers.1, .8, .10, arch.ports.8); the layout
// (go.layout.1); each library's one owner (arch.ports.6); each test's
// closure and no vacuous suite (go.style.6); the source checks; and the
// build recipes (arch.layers.10).
func TestTheTreeKeepsTheTable(t *testing.T) {
	t.Parallel() // tree hands each caller its own copy of the snapshot's world (gragas#1194)
	tb, w := tree(t)
	checks := map[string][]string{
		"imports":       edges(tb, w),
		"layout":        layout(tb, w),
		"pure imports":  pureImports(tb, w),
		"owners":        owners(tb, w),
		"test closures": closures(tb, w),
		"build recipes": releaseTags(recipes(t, w)),
		"refused names": refusedNames(tb, w),
		"line limit":    lineLimit(tb, w, func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }),
	}
	maps.Copy(checks, sourceChecks(t, tb, w))
	_, checks["layer files"] = layerFiles(tb, w)
	checks["exceptions"] = exceptionChecks(t, sources(t, w))
	for name, findings := range checks {
		for _, f := range findings {
			t.Errorf("%s: %s", name, f)
		}
	}
	t.Run("no suite is vacuous", func(t *testing.T) { noVacuousSuite(t, w) })
}

// TestTheTableNamesThePackagesTheTreeHas is the could-not-run guard's
// negative test: a world that lost a file says so.
func TestTheTableNamesThePackagesTheTreeHas(t *testing.T) {
	t.Parallel()
	short := world{module: table(t).Module, pkgs: map[string]*pkg{"a": {path: "a", files: []string{"a.go"}}}, files: []string{"a/a.go", "a/b.go"}}
	if err := short.counted(); err == nil {
		t.Error("a Go file go list did not read was not seen")
	}
}

// planted is the real table and a world of one package, at path, with the
// files, imports and test imports given.
func planted(t *testing.T, p pkg) (*layers.Table, world) {
	t.Helper()
	tb := table(t)
	w := world{module: tb.Module, pkgs: map[string]*pkg{p.path: &p}}
	for _, f := range p.files {
		w.files = append(w.files, path.Join(p.path, f))
	}
	return tb, w
}

func fires(t *testing.T, findings []string, want string) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(f, want) {
			return
		}
	}
	t.Errorf("no finding says %q; found %q", want, findings)
}

func TestAPackageMissingFromTheTableFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/util", name: "util", files: []string{"util.go"}})
	fires(t, unlayered(tb, w), "internal/util: in no layer")
}

func TestAnImportNoRowAllowsFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/generate", name: "generate", files: []string{"gen.go"},
		imports: []string{table(t).Module + "/internal/core/rulebook"}})
	fires(t, edges(tb, w), "internal/core/generate -> internal/core/rulebook")
}

func TestATestReachingASiblingItsPackageMayNotFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/rulebook", name: "rulebook", files: []string{"book.go"}, tests: []string{"book_test.go"},
		testImports: []string{table(t).Module + "/internal/host/fs", table(t).Module + "/internal/port/testing"}})
	f := edges(tb, w)
	fires(t, f, "internal/core/rulebook (its tests) -> internal/host/fs")
	if slices.ContainsFunc(f, func(s string) bool { return strings.Contains(s, "-> internal/port/testing") }) {
		t.Errorf("a test taking a stand-in was refused: %q", f)
	}
}

func TestAStandInInNonTestCodeFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/api", name: "coreapi", files: []string{"api.go"},
		imports: []string{table(t).Module + "/internal/port/testing"}})
	fires(t, edges(tb, w), "internal/core/api -> internal/port/testing")
}

func TestATestOnlyPackageWithAFileFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/journeys", name: "contract", files: []string{"helper.go"}})
	fires(t, edges(tb, w), "internal/journeys is test-only and holds helper.go")
}

func TestAContractSuiteInNonTestCodeFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/api", name: "coreapi", files: []string{"api.go"},
		imports: []string{table(t).Module + "/internal/port/contracttest"}})
	fires(t, edges(tb, w), "internal/core/api -> internal/port/contracttest")
}

// TestANewImportOfTheLegacyCommandsFails: only the root mounts the old check
// and gen (M0a-20); a core package, the entry or another command package that
// imports them breaks the table. The control: the root may.
func TestANewImportOfTheLegacyCommandsFails(t *testing.T) {
	t.Parallel()
	legacy := table(t).Module + "/internal/cli/legacy"
	for _, from := range []pkg{
		{path: "internal/core/api", name: "coreapi", files: []string{"api.go"}},
		{path: "cmd/gragas", name: "main", files: []string{"main.go"}},
	} {
		from.imports = []string{legacy}
		tb, w := planted(t, from)
		fires(t, edges(tb, w), from.path+" -> internal/cli/legacy")
	}
	tb, w := planted(t, pkg{path: "internal/cli/root", name: "root", files: []string{"root.go"}, imports: []string{legacy}})
	if f := edges(tb, w); slices.ContainsFunc(f, func(s string) bool { return strings.Contains(s, "-> internal/cli/legacy") }) {
		t.Errorf("the root was refused the old commands it mounts: %q", f)
	}
}

// TestAFileForOneSystemCannotHideItsImports: the tree is read for every
// system of the list, not only this host's, so a file named for one system
// (_linux.go) puts its imports to the owners check on every system. Each file
// whose name carries a GOOS of the list has every import it declares in its
// package's imports, whichever system this run is on. Risk: a Linux-only
// import of a library no row owns passes on the Mac gate and fails on Linux
// after the merge. A tree with no file named for a system passes: the census
// drives those files to zero and TestNoNewOsCode keeps them there, so the
// check proves itself on a planted fixture instead
// (TestTheSystemFileCheckSeesAnImportTheReadMissed, gragas#1563).
func TestAFileForOneSystemCannotHideItsImports(t *testing.T) {
	t.Parallel()
	_, w := tree(t)
	oses, err := systemNames()
	if err != nil {
		t.Fatal(err)
	}
	findings, err := systemFileImports(w, oses, func(p *pkg) string {
		if p.path == "." {
			return root
		}
		return filepath.Join(root, filepath.FromSlash(p.path))
	}, slices.Contains[[]string])
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		t.Error(f)
	}
}

// TestTheSystemFileCheckSeesAnImportTheReadMissed holds the check of
// TestAFileForOneSystemCannotHideItsImports on a planted package, since the
// real tree may name no file for a system (gragas#1563): x_linux.go imports
// os/user, which the package's imports, as the tree was read, do not hold, and
// the check says so. The controls: x.go, named for no system, is not its to
// judge, and the same file passes when the imports hold it. The plant: with
// the check's holding test made always true, the fixture gives no finding, so
// the red above is the check's own.
func TestTheSystemFileCheckSeesAnImportTheReadMissed(t *testing.T) {
	t.Parallel()
	oses, err := systemNames()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(oses, "linux") {
		t.Fatalf("the list of systems %q names no linux, which the fixture's file is named for", oses)
	}
	dir := t.TempDir()
	for name, text := range map[string]string{
		"x.go":       "package x\n\nimport _ \"encoding/csv\"\n",
		"x_linux.go": "package x\n\nimport _ \"os/user\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	at := func(*pkg) string { return dir }
	fixture := func(imports ...string) world {
		return world{pkgs: map[string]*pkg{"internal/x": {path: "internal/x", name: "x", files: []string{"x.go", "x_linux.go"}, imports: imports}}}
	}
	got, err := systemFileImports(fixture(), oses, at, slices.Contains[[]string])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/x/x_linux.go (for linux) imports os/user, which the tree read on this host does not hold"}
	if !slices.Equal(got, want) {
		t.Errorf("the check on the planted package found %q, want %q", got, want)
	}
	if got, err := systemFileImports(fixture("os/user"), oses, at, slices.Contains[[]string]); err != nil || len(got) != 0 {
		t.Errorf("the check refused a file whose import the read holds: %q, %v", got, err)
	}
	always := func([]string, string) bool { return true }
	if got, err := systemFileImports(fixture(), oses, at, always); err != nil || len(got) != 0 {
		t.Errorf("with the holding test made always true the fixture still gave %q, %v: its red is not the check's", got, err)
	}
}

// systemNames is the GOOS of each system of the list.
func systemNames() ([]string, error) {
	list, err := systems()
	if err != nil {
		return nil, err
	}
	var oses []string
	for _, s := range list {
		goos, _, _ := strings.Cut(s, "/")
		oses = append(oses, goos)
	}
	return oses, nil
}

// systemFileImports is a finding for each import that a file of w named for
// one of oses declares and that holds does not find in its package's imports.
// dir is the folder of a package's files; the file is parsed for its imports
// alone.
func systemFileImports(w world, oses []string, dir func(*pkg) string, holds func([]string, string) bool) ([]string, error) {
	var findings []string
	for _, p := range w.sorted() {
		for _, f := range p.files {
			base := strings.TrimSuffix(f, ".go")
			var goos string
			for _, o := range oses {
				if strings.HasSuffix(base, "_"+o) {
					goos = o
				}
			}
			if goos == "" {
				continue
			}
			af, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir(p), f), nil, parser.ImportsOnly)
			if err != nil {
				return nil, err
			}
			for _, imp := range af.Imports {
				v, _ := strconv.Unquote(imp.Path.Value)
				if !holds(p.imports, v) {
					findings = append(findings, fmt.Sprintf("%s/%s (for %s) imports %s, which the tree read on this host does not hold", p.path, f, goos, v))
				}
			}
		}
	}
	return findings, nil
}

// inRoot is name run with args in the module's root, the environment inherited and env added: a read only
// run of a tool of this repository, a go list.
func inRoot(env []string, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...) //nolint:gosec,noctx // go list of this module, read only, once for the run from the first test that asks
	cmd.Dir = root
	cmd.Env = append(configtest.Inherited(), env...)
	return cmd
}
