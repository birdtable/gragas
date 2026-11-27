package internal

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// osFile is a Go file built for one system by its name.
var (
	osFile = regexp.MustCompile(`_(aix|android|darwin|dragonfly|freebsd|illumos|ios|js|linux|netbsd|openbsd|plan9|solaris|wasip1|windows|unix)(_[a-z0-9]+)?(_test)?\.go$`)
	// cmdMain is where a main package may be: a command, and the source of standin.wasm,
	// a program for the wasm target and not a command (gragas#2023).
	cmdMain = regexp.MustCompile(`^(cmd/[^/]+|internal/dev/standin)$`)
)

// adapterKinds are the kinds of layer a package that reaches the world may
// sit in: an adapter, the entry and the command tree, and test-only code.
func adapterKinds() []string {
	return []string{layers.KindHost, layers.KindEdge, layers.KindStandIn, layers.KindTests}
}

// layout is go.layout.1's template held against the tree: no pkg folder, a
// main package only in cmd/<name>/, no package at the root or outside cmd/
// and internal/, no adapter outside internal/host/, no file built for one
// system and no build constraint outside it (arch.ports.1), and no go.work
// (go.layout.3). A test-only package ships nothing, so it may hold a test
// for one system: the signal journeys are Unix-only (go.concurrency.4).
func layout(tb *layers.Table, w world) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range w.files {
		dir := path.Dir(f)
		for d := dir; d != "."; d = path.Dir(d) {
			if path.Base(d) == "pkg" && !seen[d] {
				seen[d] = true
				out = append(out, d+": a pkg folder; the template has cmd/ and internal/ only")
			}
		}
		if f == "go.work" {
			out = append(out, "go.work: a workspace file in the tree (go.layout.3)")
		}
		host := strings.HasPrefix(f, "internal/host/") || testOnly(tb, path.Dir(f))
		if strings.HasSuffix(f, ".go") && osFile.MatchString(f) && !host {
			out = append(out, f+": a file built for one system, outside internal/host/")
		}
	}
	for _, f := range w.constrained {
		if !strings.HasPrefix(f, "internal/host/") && !testOnly(tb, path.Dir(f)) {
			out = append(out, f+": a build constraint outside internal/host/")
		}
	}
	for _, p := range w.sorted() {
		if p.name == "main" && !cmdMain.MatchString(p.path) {
			out = append(out, p.path+": a main package outside cmd/<name>/")
		}
		if len(p.files) > 0 && p.path != "internal" && !strings.HasPrefix(p.path, "cmd/") && !strings.HasPrefix(p.path, "internal/") {
			out = append(out, p.path+": a package outside cmd/ and internal/")
		}
		out = append(out, adapterOutsideHost(tb, w, p)...)
	}
	return out
}

// testOnly is whether the package at dir is in a test-only row.
func testOnly(tb *layers.Table, dir string) bool {
	r := tb.RowOf(dir)
	return r != nil && r.Kind == layers.KindTests
}

// adapterOutsideHost is p when it reaches the world, an effect package or a
// library no pure package may take, from a layer that is not an adapter's.
func adapterOutsideHost(tb *layers.Table, w world, p *pkg) []string {
	if r := tb.RowOf(p.path); r != nil && slices.Contains(adapterKinds(), r.Kind) {
		return nil
	}
	var out []string
	for _, imp := range p.imports {
		if _, own := w.rel(imp); own {
			continue
		}
		e, owned := tb.Owner(imp)
		if pure := owned && e.Pure; tb.Effect(imp) || (!std(imp) && !pure) {
			out = append(out, p.path+" imports "+imp+": an adapter outside internal/host/")
		}
	}
	return out
}

// owners is arch.ports.6: each library is imported by its one owner, and a
// library the table does not list is imported nowhere. The tests of a
// test-only package are exempt, as the depguard owner rows are.
func owners(tb *layers.Table, w world) []string {
	var out []string
	for _, p := range w.sorted() {
		imps := p.imports
		if r := tb.RowOf(p.path); r == nil || r.Kind != layers.KindTests {
			imps = append(slices.Clone(imps), p.testImports...)
		}
		for _, imp := range imps {
			if _, own := w.rel(imp); own || std(imp) {
				continue
			}
			e, ok := tb.Owner(imp)
			switch {
			case !ok:
				out = append(out, p.path+" imports "+imp+", a library "+layers.File+" names no owner for")
			case e.Owner != p.path && !e.AlsoOwner(p.path) && !e.WithinPattern(p.path):
				out = append(out, p.path+" imports "+imp+", which only "+orNothing(e.Owner)+" may import")
			}
		}
	}
	return append(out, staleAlso(tb, w)...)
}

// staleAlso is each other owner of a library that imports it in neither its
// code nor its tests: a row that has outlived its move or never held, which
// the table's doc says the layer test fails (A70).
func staleAlso(tb *layers.Table, w world) []string {
	var out []string
	for _, e := range tb.External {
		for _, a := range e.Also {
			if p, ok := w.pkgs[a.Owner]; !ok || !importsLibrary(tb, w, p, e) {
				out = append(out, a.Owner+" is listed as another owner of "+e.Path+" and imports it in neither its code nor its tests: remove the also from "+layers.File)
			}
		}
	}
	return out
}

// importsLibrary is whether p imports the library e, in code or in tests.
func importsLibrary(tb *layers.Table, w world, p *pkg, e layers.External) bool {
	return slices.ContainsFunc(append(slices.Clone(p.imports), p.testImports...), func(imp string) bool {
		// A standard-library row (encoding/json/jsontext, A59) has other owners
		// too, so a std import is looked up like any other; one with no row
		// is no owner's.
		if _, own := w.rel(imp); own {
			return false
		}
		got, ok := tb.Owner(imp)
		return ok && got.Path == e.Path
	})
}

func orNothing(owner string) string {
	if owner == "" {
		return "nothing"
	}
	return owner
}

func TestAPkgFolderFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "pkg/util", name: "util", files: []string{"util.go"}})
	fires(t, layout(tb, w), "pkg: a pkg folder")
}

func TestAMainOutsideCmdFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: ".", name: "main", files: []string{"main.go"}})
	f := layout(tb, w)
	fires(t, f, ".: a main package outside cmd/<name>/")
	fires(t, f, ".: a package outside cmd/ and internal/")
}

func TestAnAdapterOutsideTheHostFolderFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/launchd", name: "launchd", files: []string{"launchd.go"}, imports: []string{"os/exec"}})
	fires(t, layout(tb, w), "internal/launchd imports os/exec: an adapter outside internal/host/")
	tb, w = planted(t, pkg{path: "internal/host/fs", name: "fs", files: []string{"repo.go"}, imports: []string{"os"}})
	if f := layout(tb, w); len(f) > 0 {
		t.Errorf("an adapter in the host folder was refused: %q", f)
	}
}

func TestAFileForOneSystemOutsideTheHostFolderFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/rulebook", name: "rulebook", files: []string{"book_linux.go"}})
	w.constrained = []string{"internal/core/rulebook/other.go"}
	f := layout(tb, w)
	fires(t, f, "internal/core/rulebook/book_linux.go: a file built for one system")
	fires(t, f, "internal/core/rulebook/other.go: a build constraint outside internal/host/")
}

func TestAWorkspaceFileFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "cmd/gragas", name: "main", files: []string{"main.go"}})
	w.files = append(w.files, "go.work")
	fires(t, layout(tb, w), "go.work: a workspace file")
}

func TestALibraryOutsideItsOwnerFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/api", name: "coreapi", files: []string{"api.go"},
		imports: []string{"github.com/spf13/cobra", "example.org/unknown"}})
	f := owners(tb, w)
	fires(t, f, "imports github.com/spf13/cobra, which only internal/cli/root may import")
	fires(t, f, "imports example.org/unknown, a library layers.yaml names no owner for")
}

// TestWazeroIsImportedByTheHostPluginAlone (gragas#2037): the one importer of
// the WebAssembly runtime is internal/host/plugin. Planted: the core, and
// another adapter, that import it.
func TestWazeroIsImportedByTheHostPluginAlone(t *testing.T) {
	t.Parallel()
	const lib = "github.com/tetratelabs/wazero"
	tb, w := tree(t)
	if got := owners(tb, w); len(got) != 0 {
		t.Fatalf("the tree: %q", got)
	}
	for _, at := range []string{"internal/core/api", "internal/host/fs"} {
		t.Run(at, func(t *testing.T) {
			tb, w := planted(t, pkg{path: at, name: "x", files: []string{"x.go"}, imports: []string{lib}})
			fires(t, owners(tb, w), "imports "+lib+", which only internal/host/plugin may import")
		})
	}
}

// TestAnAlsoOwnerThatNoLongerImportsItsLibraryFails: the also list only
// shrinks if a row that outlived its move fails. Planted: host/term without
// cobra, cli/legacy without it, and cli/root without the console's test driver
// in its tests.
func TestAnAlsoOwnerThatNoLongerImportsItsLibraryFails(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)
	if got := owners(tb, w); len(got) != 0 {
		t.Fatalf("the tree: %q", got)
	}
	for _, c := range []struct{ pkg, lib string }{
		{"internal/host/term", "github.com/spf13/cobra"},
		{"internal/cli/legacy", "github.com/spf13/cobra"},
		{"internal/cli/root", "github.com/birdtable/go-console/uitest"},
	} {
		t.Run(c.pkg, func(t *testing.T) {
			tb, w := tree(t)
			p := *w.pkgs[c.pkg]
			p.imports = slices.DeleteFunc(slices.Clone(p.imports), func(i string) bool { return strings.HasPrefix(i, c.lib) })
			p.testImports = slices.DeleteFunc(slices.Clone(p.testImports), func(i string) bool { return strings.HasPrefix(i, c.lib) })
			w.pkgs[c.pkg] = &p
			fires(t, owners(tb, w), c.pkg+" is listed as another owner of "+c.lib+" and imports it in neither")
		})
	}
}

var (
	// tagFlag is go's build tag flag in any spelling: -tags or --tags,
	// followed by a value with = or a space, wherever it stands on a line.
	tagFlag = regexp.MustCompile(`(^|[^A-Za-z0-9_-])--?tags([=\s"']|$)`)
	// gitCmd is a git command line, whose own --tags is no build tag.
	gitCmd = regexp.MustCompile(`(^|[\s;(|&{])git\s`)
	// cgoSetting is any setting of CGO_ENABLED, and its value.
	cgoSetting = regexp.MustCompile(`CGO_ENABLED\s*[:?+]?=\s*["']?([^"'\s;)]*)`)
	// goBuild is a go command that makes a binary.
	goBuild = regexp.MustCompile(`(^|[\s;(|&{])go\s+(build|install)\b`)
	// yamlTags is a tags key in a release config.
	yamlTags = regexp.MustCompile(`^\s*-?\s*tags\s*:`)
)

// releaseTags is arch.layers.10's last clause and go.layout.4, over every
// recipe of the tree: no build tag in any spelling (a flag, GOFLAGS, a
// release config's tags key), so a tag that exists for tests never reaches
// a release; no CGO_ENABLED but 0; and every go build or go install on a
// line that sets CGO_ENABLED=0, or, in a release config, a file that does.
// A git command's own --tags is not a build tag. files maps a recipe's
// path to its text.
func releaseTags(files map[string]string) []string {
	var out []string
	for name, text := range files {
		release := strings.Contains(name, "goreleaser")
		if release && !strings.Contains(text, "CGO_ENABLED=0") {
			out = append(out, name+": a release config that does not set CGO_ENABLED=0")
		}
		for line := range strings.Lines(text) {
			l := strings.TrimSpace(line)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			say := func(why string) { out = append(out, name+": "+l+": "+why) }
			if loc := tagFlag.FindStringIndex(l); loc != nil && !gitCmd.MatchString(l[:loc[0]]) {
				say("a build tag in a recipe")
			} else if strings.Contains(l, "GOFLAGS") && strings.Contains(l, "tags") || release && yamlTags.MatchString(l) {
				say("a build tag in a recipe")
			}
			for _, m := range cgoSetting.FindAllStringSubmatch(l, -1) {
				if m[1] != "0" {
					say("CGO_ENABLED set to " + strconv.Quote(m[1]) + ", not 0 (go.layout.4)")
				}
			}
			if goBuild.MatchString(l) && !strings.Contains(l, "CGO_ENABLED=0") {
				say("a build with cgo on (go.layout.4)")
			}
		}
	}
	slices.Sort(out)
	return out
}

// recipes is every recipe of the tree, by path: Taskfiles and *.mk files,
// scripts, hooks, release configs and workflows.
func recipes(t *testing.T, w world) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range w.files {
		base := path.Base(f)
		if base == "Taskfile.yml" || strings.HasSuffix(base, ".mk") || strings.HasPrefix(f, "scripts/") || strings.HasPrefix(f, "hooks/") ||
			strings.HasPrefix(base, ".goreleaser.") || strings.HasPrefix(base, "goreleaser.") {
			out[f] = ""
		}
	}
	if entries, err := os.ReadDir(filepath.Join(root, ".github", "workflows")); err == nil {
		for _, e := range entries {
			out[".github/workflows/"+e.Name()] = ""
		}
	}
	for _, n := range []string{".goreleaser.yml", ".goreleaser.yaml"} {
		if _, err := os.Stat(filepath.Join(root, n)); err == nil {
			out[n] = ""
		}
	}
	for n := range out {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
		if err != nil {
			t.Fatal(err)
		}
		out[n] = string(data)
	}
	if out["Taskfile.yml"] == "" || out["hooks/pre-push"] == "" {
		t.Fatal("the Taskfile or the hook was not read")
	}
	return out
}

// TestABuildTagOrCgoInARecipeFails plants the reviewer's cases.
func TestABuildTagOrCgoInARecipeFails(t *testing.T) {
	t.Parallel()
	f := releaseTags(map[string]string{
		"Makefile": "build:\n\tCGO_ENABLED=0 go build --tags=x ./cmd/x\n\tCGO_ENABLED=0 go build \"-tags=x\" ./cmd/y\n" +
			"\tGOFLAGS=-tags=fake CGO_ENABLED=0 go build ./cmd/z\n\tCGO_ENABLED=1 go install ./cmd/w\n\tgo build ./cmd/v\n" +
			"VERSION := $(shell git describe --tags)\n\t# go build in a comment\n",
		"scripts/release.sh": "export GOFLAGS='-tags=stub'\n",
		".goreleaser.yml":    "builds:\n  - env:\n      - CGO_ENABLED=1\n    tags:\n      - fake\n",
	})
	for _, want := range []string{
		"Makefile: CGO_ENABLED=0 go build --tags=x ./cmd/x: a build tag",
		`Makefile: CGO_ENABLED=0 go build "-tags=x" ./cmd/y: a build tag`,
		"Makefile: GOFLAGS=-tags=fake CGO_ENABLED=0 go build ./cmd/z: a build tag",
		`Makefile: CGO_ENABLED=1 go install ./cmd/w: CGO_ENABLED set to "1"`,
		"Makefile: CGO_ENABLED=1 go install ./cmd/w: a build with cgo on",
		"Makefile: go build ./cmd/v: a build with cgo on",
		"scripts/release.sh: export GOFLAGS='-tags=stub': a build tag",
		`.goreleaser.yml: - CGO_ENABLED=1: CGO_ENABLED set to "1"`,
		".goreleaser.yml: tags:: a build tag",
		".goreleaser.yml: a release config that does not set CGO_ENABLED=0",
	} {
		fires(t, f, want)
	}
	if len(f) != 10 {
		t.Errorf("findings = %q; git describe --tags and a comment are no build", f)
	}
}

// templateFolders are the folders the Go program template names under
// internal/ (arch.structure.6), and the ones a CLI must have.
func templateFolders() (named, needed []string) {
	return []string{"core", "port", "host", "door", "config", "cli", "contract"},
		[]string{"internal/core", "internal/core/api", "internal/port", "internal/host", "internal/config", "internal/cli"}
}

// leafLayers are the rows whose packages are leaves: the leaf row, and canon,
// which has a row of its own for the standard packages only it needs (A59).
func leafLayers() []string { return []string{"leaf", "canon"} }

// shape is arch.structure.6: the tree has every folder the template gives a
// CLI, a command under cmd/<name>/, and no folder under internal/ the
// template does not name, but for a leaf or a test-only package the table
// places.
func shape(tb *layers.Table, w world) []string {
	named, needed := templateFolders()
	dirs := map[string]bool{}
	for _, f := range w.files {
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	var out []string
	for _, d := range needed {
		if !dirs[d] {
			out = append(out, d+": a folder the template gives every CLI is missing")
		}
	}
	cmds := 0
	for d := range dirs {
		if cmdMain.MatchString(d) {
			cmds++
		}
		top, ok := strings.CutPrefix(d, "internal/")
		if !ok || strings.Contains(top, "/") || slices.Contains(named, top) {
			continue
		}
		if r := tb.RowOf(d); r == nil || !slices.Contains(leafLayers(), r.Layer) && r.Kind != layers.KindTests {
			out = append(out, d+": a folder the template does not name, and no leaf")
		}
	}
	if cmds == 0 {
		out = append(out, "cmd/: no command under cmd/<name>/")
	}
	slices.Sort(out)
	return out
}

func TestAFolderOutsideTheTemplateFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/services", name: "services", files: []string{"s.go"}})
	f := shape(tb, w)
	fires(t, f, "internal/services: a folder the template does not name")
	fires(t, f, "internal/port: a folder the template gives every CLI is missing")
	fires(t, f, "cmd/: no command under cmd/<name>/")
}

// pureImports is arch.ports.1 held by an allow list: a non-test file of a
// pure package imports only the standard packages its row's std list names,
// each exactly, and no library but a pure one it owns.
func pureImports(tb *layers.Table, w world) []string {
	var out []string
	for _, p := range w.sorted() {
		for _, imp := range p.imports {
			if _, own := w.rel(imp); !own && !tb.PureMay(p.path, imp) {
				out = append(out, p.path+" imports "+imp+", which its row's std list does not name")
			}
		}
	}
	return out
}

// TestThePureLayersTakeOnlyTheirStandardPackages plants the reviewer's
// cases in the core: each is outside the core's list.
func TestThePureLayersTakeOnlyTheirStandardPackages(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/rulebook", name: "rulebook", files: []string{"book.go"},
		imports: []string{"net/smtp", "path/filepath", "text/template", "os", "fmt"}})
	f := pureImports(tb, w)
	for _, imp := range []string{"net/smtp", "path/filepath", "text/template", "os"} {
		fires(t, f, "internal/core/rulebook imports "+imp+",")
	}
	if len(f) != 4 {
		t.Errorf("findings = %q; fmt is on the list", f)
	}
}

// refusedNames is arch.structure.7: no package carries a name the source
// table refuses. The operations package is named coreapi because api is one.
func refusedNames(tb *layers.Table, w world) []string {
	var out []string
	for _, p := range w.sorted() {
		if slices.Contains(tb.Source.Refused, p.name) || slices.Contains(tb.Source.Refused, path.Base(p.path)) && p.name != "coreapi" {
			out = append(out, p.path+": a package named "+p.name+", which "+layers.File+" refuses")
		}
	}
	return out
}

// layerFiles is arch.layers.3: every Go file sits in a package of exactly one
// layer (the table refuses a package in two), and each layer that lists a
// package holds at least one file, so a check that loops over layers ran
// over every one of them.
func layerFiles(tb *layers.Table, w world) (count map[string]int, out []string) {
	count = map[string]int{}
	for _, p := range w.sorted() {
		n := len(p.files) + len(p.tests)
		if r := tb.RowOf(p.path); r != nil {
			count[r.Layer] += n
		} else if n > 0 {
			out = append(out, p.path+": "+fmt.Sprint(n)+" Go files in no layer of "+layers.File)
		}
	}
	for _, r := range tb.Rows {
		if len(r.Packages) > 0 && count[r.Layer] == 0 {
			out = append(out, "layer "+r.Layer+" holds no Go file, so no check ran over it")
		}
	}
	return count, out
}

// lineLimit is the source table's limit on the files of one layer: a file
// longer than it fails, and a table whose layer holds no file fails too.
func lineLimit(tb *layers.Table, w world, read func(rel string) ([]byte, error)) []string {
	var out []string
	checked := 0
	for _, p := range w.sorted() {
		r := tb.RowOf(p.path)
		if r == nil || r.Layer != tb.Source.LimitLayer {
			continue
		}
		for _, f := range p.files {
			rel := path.Join(p.path, f)
			data, err := read(rel)
			if err != nil {
				out = append(out, rel+": "+err.Error())
				continue
			}
			checked++
			if n := strings.Count(string(data), "\n"); n > tb.Source.LimitLines {
				out = append(out, rel+": "+fmt.Sprint(n)+" lines, over the "+fmt.Sprint(tb.Source.LimitLines)+" of layer "+r.Layer)
			}
		}
	}
	if checked == 0 {
		out = append(out, "layer "+tb.Source.LimitLayer+" has no file, so the line limit checked nothing")
	}
	return out
}

func lines(n int) []byte { return []byte(strings.Repeat("x\n", n)) }

func TestAFileInNoLayerOrALayerOfNoFilesFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/stray", name: "stray", files: []string{"s.go"}})
	_, f := layerFiles(tb, w)
	fires(t, f, "internal/stray: 1 Go files in no layer")
	fires(t, f, "holds no Go file, so no check ran over it")
	_, w = tree(t)
	count, f := layerFiles(tb, w)
	if len(f) != 0 || len(count) == 0 {
		t.Errorf("the tree: %q over %v, want none over some layers", f, count)
	}
}

func TestAnUtilPackageFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/helpers", name: "helpers", files: []string{"h.go"}})
	fires(t, refusedNames(tb, w), "a package named helpers")
	tb, w = planted(t, pkg{path: "internal/core/api", name: "coreapi", files: []string{"a.go"}})
	if f := refusedNames(tb, w); len(f) != 0 {
		t.Errorf("coreapi in a folder named api: %q", f)
	}
	tb, w = planted(t, pkg{path: "internal/core/other", name: "api", files: []string{"a.go"}})
	fires(t, refusedNames(tb, w), "a package named api")
}

func TestAFileOverTheLineLimitFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/api", name: "coreapi", files: []string{"big.go", "small.go"}})
	read := func(rel string) ([]byte, error) {
		if strings.HasSuffix(rel, "big.go") {
			return lines(tb.Source.LimitLines + 1), nil
		}
		return lines(tb.Source.LimitLines), nil
	}
	f := lineLimit(tb, w, read)
	if len(f) != 1 {
		t.Fatalf("findings = %q, want only big.go", f)
	}
	fires(t, f, "internal/core/api/big.go: ")
	_, w = planted(t, pkg{path: "internal/host/fs", name: "fs", files: []string{"a.go"}})
	fires(t, lineLimit(tb, w, read), "checked nothing")
}

// kitLook is every break of cli.look.1 and A62: a package other than host/term
// that imports the kit's look in non-test code, a command package that does
// (cli.* draws through the Terminal port and imports no kit look; the root is
// one, A70).
func kitLook(tb *layers.Table, w world) []string {
	const look = "github.com/birdtable/go-console/ui"
	e, ok := tb.Owner(look)
	if !ok || e.Owner != "internal/host/term" {
		return []string{look + " is not owned by internal/host/term in " + layers.File}
	}
	var out []string
	importers := map[string]bool{}
	for _, p := range w.sorted() {
		for _, imp := range p.imports {
			if imp != look && !strings.HasPrefix(imp, look+"/") {
				continue
			}
			importers[p.path] = true
			switch {
			case p.path == e.Owner:
			case strings.HasPrefix(p.path, "internal/cli/"):
				out = append(out, p.path+" is a command package and imports the kit's look: it draws through the Terminal port")
			default:
				out = append(out, p.path+" imports the kit's look, which only "+e.Owner+" may")
			}
		}
	}
	if !importers[e.Owner] {
		out = append(out, e.Owner+" does not import the kit's look")
	}
	return out
}

// TestHostTermIsTheOnlyImporterOfTheKitsLook: the tree holds A62. Planted: a
// command package that imports the look, the root, and another adapter that
// does.
func TestHostTermIsTheOnlyImporterOfTheKitsLook(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)
	if got := kitLook(tb, w); len(got) != 0 {
		t.Errorf("the kit's look: %q", got)
	}
	const look = "github.com/birdtable/go-console/ui"
	t.Run("a command package", func(t *testing.T) {
		tb, w := planted(t, pkg{path: "internal/cli/format", name: "format", files: []string{"f.go"}, imports: []string{look}})
		w.pkgs["internal/host/term"] = &pkg{path: "internal/host/term", name: "term", files: []string{"t.go"}, imports: []string{look}}
		fires(t, kitLook(tb, w), "internal/cli/format is a command package")
	})
	t.Run("another adapter", func(t *testing.T) {
		tb, w := planted(t, pkg{path: "internal/host/fs", name: "fs", files: []string{"f.go"}, imports: []string{look}})
		fires(t, kitLook(tb, w), "internal/host/fs imports the kit's look")
	})
	t.Run("the command tree", func(t *testing.T) {
		tb, w := planted(t, pkg{path: "internal/cli/root", name: "root", files: []string{"r.go"}, imports: []string{look}})
		w.pkgs["internal/host/term"] = &pkg{path: "internal/host/term", name: "term", files: []string{"t.go"}, imports: []string{look}}
		fires(t, kitLook(tb, w), "internal/cli/root is a command package")
	})
	t.Run("no owner at all", func(t *testing.T) {
		tb, w := planted(t, pkg{path: "internal/host/fs", name: "fs", files: []string{"f.go"}})
		fires(t, kitLook(tb, w), "internal/host/term does not import the kit's look")
	})
}
