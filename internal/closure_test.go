package internal

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// closurePlatforms mirrors internal/dev/gate's ShippedPlatforms(): the systems
// the shipped binary is built for, whose linked modules gate/closure.txt is the
// union of, read with cgo off and joined, so the list is the same on every host
// (A67, gragas#909). The repository's tests may not link the gate
// (arch.layers.1), so they hold the same list here; a change to
// gate.ShippedPlatforms() moves gate/closure.txt, and the union this reads no
// longer matching it fails this test until the list is brought back in step.
func closurePlatforms() []string {
	return []string{"darwin/amd64", "darwin/arm64", "freebsd/amd64", "freebsd/arm64", "linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"}
}

// The shipped binary has a declared closure, and the engine's own generators
// are outside it (A30, supply.intake.2, arch.layers.13): cmd/gragas-dev and
// everything under internal/dev are tooling run from the Makefile, never
// linked into gragas. A hidden dev command in the shipped binary is what
// A30 rejects.

// devPackages are the module-relative packages only gragas-dev links: the
// packages of the dev and dev-entry rows of layers.yaml.
func devPackages(tb *layers.Table) []string {
	var out []string
	for _, r := range tb.Rows {
		if r.Layer == "dev" || r.Layer == "dev-entry" {
			out = append(out, r.Packages...)
			for _, pat := range r.Patterns {
				out = append(out, layers.Folder(pat))
			}
		}
	}
	return out
}

// leaked is the packages of closure that are dev code.
func leaked(tb *layers.Table, closure []string) []string {
	var out []string
	for _, p := range closure {
		for _, d := range devPackages(tb) {
			if p == d || strings.HasPrefix(p, d+"/") {
				out = append(out, p)
			}
		}
	}
	return out
}

// shippedGraph is go list -deps of the shipped binary, by package: what each
// imports, in the build the binary is made from (no tests).
func shippedGraph(t *testing.T) map[string][]string {
	t.Helper()
	return graphOf(t, shippedArgs()...)
}

// graphOf is go list of args, by package: what each imports.
func graphOf(t *testing.T, args ...string) map[string][]string {
	t.Helper()
	graph := map[string][]string{}
	err := golist(func(dec *json.Decoder) error {
		var lp struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&lp); err != nil {
			return err
		}
		graph[lp.ImportPath] = lp.Imports
		return nil
	}, args...)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

// shippedClosure is the module's packages the shipped binary links, by
// path from the module, its own main included.
func shippedClosure(w world, graph map[string][]string) []string {
	main := w.module + "/cmd/gragas"
	return append(w.closure(graph, main, "cmd/gragas"), "cmd/gragas")
}

// TestTheShippedBinaryLinksNoDevCode: gragas's package list holds no
// internal/dev and no cmd/gragas-dev, the control that gragas-dev itself does
// link them, and a planted import of internal/dev/layers from the command
// tree fails it. Risk: the layer table's renderer growing back into the
// shipped binary through any importer.
func TestTheShippedBinaryLinksNoDevCode(t *testing.T) {
	t.Parallel()
	tb := table(t)
	w := world{module: tb.Module}
	if len(devPackages(tb)) == 0 {
		t.Fatal("layers.yaml names no dev package, so nothing could leak")
	}
	graph := shippedGraph(t)
	got := shippedClosure(w, graph)
	for _, want := range []string{"cmd/gragas", "internal/cli/root", "internal/core/api", "internal/ordered"} {
		if !slices.Contains(got, want) {
			t.Fatalf("the closure %v lacks %s, so it proves nothing", got, want)
		}
	}
	if bad := leaked(tb, got); len(bad) != 0 {
		t.Errorf("the shipped binary links dev code: %v", bad)
	}

	// The control: gragas-dev does link the renderer, so the walk can see it.
	dev := graphOf(t, devArgs()...)
	devClosure := w.closure(dev, w.module+"/cmd/gragas-dev", "cmd/gragas-dev")
	if !slices.Contains(devClosure, "internal/dev/layers") {
		t.Errorf("gragas-dev links %v: the check cannot see dev code", devClosure)
	}

	// The plant: the command tree imports the renderer.
	planted := map[string][]string{}
	for k, v := range graph {
		planted[k] = slices.Clone(v)
	}
	root := w.module + "/internal/cli/root"
	planted[root] = append(planted[root], w.module+"/internal/dev/layers")
	planted[w.module+"/internal/dev/layers"] = nil
	if bad := leaked(tb, shippedClosure(w, planted)); !slices.Equal(bad, []string{"internal/dev/layers"}) {
		t.Errorf("a planted import of internal/dev/layers gave %v, want it named", bad)
	}
}

// closureFile is the shrink-only list of external modules the shipped binary
// may link (supply.intake.2), one a line, # for a comment.
const closureFile = "gate/closure.txt"

// shippedModules are the external modules the shipped binary links, as go list
// reads them for each of closurePlatforms with cgo off, joined, so the set is
// the same on every host and holds a module one platform links and another does
// not (A67): mousetrap on Windows, termios on Unix (gragas#1371), the flock
// library go-files' lock takes on Windows (gragas#909). It reads the same
// platforms, the same way, as gragas-dev's own gate check (cmd/gragas-dev
// closureHosts reads gate.ShippedPlatforms()), so the two never disagree on the
// list.
func shippedModules(t *testing.T) []string {
	t.Helper()
	var mods []string
	for _, p := range closurePlatforms() {
		goos, goarch, ok := strings.Cut(p, "/")
		if !ok {
			t.Fatalf("closurePlatforms names %q, which is not a system and an architecture", p)
		}
		err := golistOn([]string{"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=0"}, func(dec *json.Decoder) error {
			var lp struct {
				Standard bool
				Module   *struct {
					Path string
					Main bool
				}
			}
			if err := dec.Decode(&lp); err != nil {
				return err
			}
			if !lp.Standard && lp.Module != nil && !lp.Module.Main && !slices.Contains(mods, lp.Module.Path) {
				mods = append(mods, lp.Module.Path)
			}
			return nil
		}, modulesArgs()...)
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(mods)
	return mods
}

// listedModules is the modules of the closure list.
func listedModules(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(closureFile)))
	if err != nil {
		t.Fatal(err)
	}
	return moduleLines(string(data))
}

func moduleLines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

// closureFindings holds the linked modules to the list both ways: a module
// the list does not name fails until a reason is recorded, and a name the
// binary no longer links fails until the list is lowered. An empty list is
// no list.
func closureFindings(linked, listed []string) []string {
	var out []string
	if len(listed) == 0 {
		out = append(out, closureFile+" lists no module, so it holds nothing")
	}
	for _, m := range linked {
		if !slices.Contains(listed, m) {
			out = append(out, "the shipped binary links "+m+", which "+closureFile+" does not list: record a reason and add it")
		}
	}
	for _, m := range listed {
		if !slices.Contains(linked, m) {
			out = append(out, closureFile+" lists "+m+", which the shipped binary no longer links: lower the list")
		}
	}
	return out
}

// TestTheShippedBinaryLinksOnlyTheModulesTheListNames: the real closure holds
// to the list; a planted extra module fails and is named; the control passes;
// and an empty list is refused. Risk: a new library arriving with a feature.
func TestTheShippedBinaryLinksOnlyTheModulesTheListNames(t *testing.T) {
	t.Parallel()
	linked, listed := shippedModules(t), listedModules(t)
	if len(linked) == 0 {
		t.Fatal("go list found no external module in the shipped binary, so the check proves nothing")
	}
	if f := closureFindings(linked, listed); len(f) != 0 {
		t.Errorf("the closure and %s differ: %q", closureFile, f)
	}
	fires(t, closureFindings(append(slices.Clone(linked), "example.org/extra"), listed), "links example.org/extra, which "+closureFile+" does not list")
	fires(t, closureFindings(linked[1:], listed), "lower the list")
	fires(t, closureFindings(linked, nil), "lists no module")
}

// goModReplaces is the replace lines of the go.mod at file (absolute, or from
// the module's root, where the go tool runs), as the go tool
// reads them (go mod edit -json), each as "old => new" with a version where the
// line gives one. A file the go tool cannot read is an error, never no lines.
func goModReplaces(file string) ([]string, error) {
	cmd := inRoot([]string{"GOWORK=off", "GOTOOLCHAIN=local"}, "go", "mod", "edit", "-json", file)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("go mod edit -json " + file + ": " + err.Error() + "\n" + stderr.String())
	}
	type version struct{ Path, Version string }
	var mod struct{ Replace []struct{ Old, New version } }
	if err := json.Unmarshal(out, &mod); err != nil {
		return nil, err
	}
	at := func(v version) string {
		if v.Version == "" {
			return v.Path
		}
		return v.Path + "@" + v.Version
	}
	var lines []string
	for _, r := range mod.Replace {
		lines = append(lines, at(r.Old)+" => "+at(r.New))
	}
	return lines, nil
}

// replaceFindings refuses every replace line: a replace swaps the source of a
// module under the path gate/closure.txt names, so the list would hold while
// the code it stands for changed, a fork or a local folder in its place
// (supply.intake.2, go.layout.3, gragas#918).
func replaceFindings(replaces []string) []string {
	var out []string
	for _, r := range replaces {
		out = append(out, "go.mod replaces "+r+": a replace swaps a module's source under the path "+closureFile+" names; take the module's own release instead")
	}
	return out
}

// TestTheModuleFileReplacesNoModule: the real go.mod has no replace line.
// Planted: a fixture go.mod that replaces a module with another version of
// itself and one with a local folder, each red and named; a replace block is
// read as its lines. The clean control is a fixture with the same require and
// no replace. The refusal case: a go.mod the go tool cannot read is an error,
// not a pass. Risk: a fork or a folder linked under a module's own name, which
// the closure list cannot see.
func TestTheModuleFileReplacesNoModule(t *testing.T) {
	t.Parallel()
	real, err := goModReplaces("go.mod") // the go tool runs in the module's root
	if err != nil {
		t.Fatal(err)
	}
	if f := replaceFindings(real); len(f) != 0 {
		t.Errorf("go.mod: %q", f)
	}
	head := "module example.org/fixture\n\ngo 1.27\n\nrequire (\n\tgolang.org/x/text v0.44.0\n\texample.org/a v1.0.0\n)\n"
	for name, tc := range map[string]struct {
		body string
		says []string
	}{
		"a line each": {"replace golang.org/x/text => golang.org/x/text v0.43.0\nreplace example.org/a => ../a\n",
			[]string{"replaces golang.org/x/text => golang.org/x/text@v0.43.0", "replaces example.org/a => ../a"}},
		"a block": {"replace (\n\texample.org/a v1.0.0 => example.org/fork v1.0.1\n)\n",
			[]string{"replaces example.org/a@v1.0.0 => example.org/fork@v1.0.1"}},
		"the control": {"", nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			file := filepath.Join(t.TempDir(), "go.mod")
			if err := os.WriteFile(file, []byte(head+"\n"+tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := goModReplaces(file)
			if err != nil {
				t.Fatal(err)
			}
			f := replaceFindings(got)
			if len(f) != len(tc.says) {
				t.Errorf("findings %q, want %d", f, len(tc.says))
			}
			for _, want := range tc.says {
				fires(t, f, want)
			}
		})
	}
	bad := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(bad, []byte("module\nreplace =>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := goModReplaces(bad); err == nil {
		t.Errorf("a go.mod the go tool cannot read: %q, want an error", got)
	}
}
