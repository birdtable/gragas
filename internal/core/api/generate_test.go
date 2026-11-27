package coreapi_test

import (
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// valid is a rulebook repository whose generated files are current: the
// rulebook domain's valid tree, with the files gen makes from it. Inputs
// read: testdata/valid/.
func valid(t *testing.T) fstest.MapFS {
	t.Helper()
	tree := fstest.MapFS{}
	src := os.DirFS("testdata/valid")
	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		tree[p] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return tree
}

// outputs is what gen makes of tree, every file whether stale or not.
func outputs(t *testing.T, tree fstest.MapFS) []coreapi.Output {
	t.Helper()
	empty := fstest.MapFS{}
	for p, f := range tree {
		if !generated(p) {
			empty[p] = f
		}
	}
	l, err := coreapi.Load(t.Context(), (&porttest.Fake{Repo: empty}).Ports(), ".", "")
	if err != nil || len(l.Findings) > 0 {
		t.Fatalf("Load: %v, findings %v", err, l.Findings)
	}
	outs, err := coreapi.Stale(l)
	if err != nil {
		t.Fatal(err)
	}
	return outs
}

// generated is whether p is a file gen writes.
func generated(p string) bool {
	for _, g := range []string{"rules.json", "bindings.json", "coverage.json", "ids.lock"} {
		if strings.HasSuffix(p, g) {
			return true
		}
	}
	return false
}

// TestOutputsMatchGolden: every file gen makes from the valid tree is the
// golden in testdata/valid, reviewed by hand and never rewritten by a test.
func TestOutputsMatchGolden(t *testing.T) {
	t.Parallel()
	tree := valid(t)
	outs := outputs(t, tree)
	var paths []string
	for _, o := range outs {
		paths = append(paths, o.Path)
	}
	want := []string{"rulebook/rules.json", "rulebook/bindings.json", "rulebook/coverage.json", "rulebook/ids.lock"}
	if !slices.Equal(paths, want) {
		t.Fatalf("outputs = %q, want %q", paths, want)
	}
	for _, o := range outs {
		t.Run(o.Path, func(t *testing.T) {
			t.Parallel()
			if golden := string(tree[o.Path].Data); string(o.Data) != golden {
				t.Errorf("%s differs from its golden:\n%s", o.Path, firstDiff(string(o.Data), golden))
			}
		})
	}
}

func TestStale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		edit  func(fstest.MapFS)
		stale []string
	}{
		{"current", func(fstest.MapFS) {}, nil},
		{"hand-edited rules.json", func(tree fstest.MapFS) {
			tree["rulebook/rules.json"] = &fstest.MapFile{Data: []byte("{}\n")}
		}, []string{"rulebook/rules.json"}},
		{"missing bindings.json", func(tree fstest.MapFS) {
			delete(tree, "rulebook/bindings.json")
		}, []string{"rulebook/bindings.json"}},
		{"rule changed, outputs not regenerated", func(tree fstest.MapFS) {
			f := tree["rulebook/lang/go/rules/errors.yaml"]
			f.Data = []byte(strings.Replace(string(f.Data), "fix: auto", "fix: never", 1))
		}, []string{"rulebook/rules.json", "rulebook/bindings.json"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tree := valid(t)
			tc.edit(tree)
			l, err := coreapi.Load(t.Context(), (&porttest.Fake{Repo: tree}).Ports(), ".", "")
			if err != nil {
				t.Fatal(err)
			}
			stale, err := coreapi.Stale(l)
			if err != nil {
				t.Fatalf("Stale: %v", err)
			}
			var got []string
			for _, o := range stale {
				got = append(got, o.Path)
			}
			if !slices.Equal(got, tc.stale) {
				t.Errorf("stale = %q, want %q", got, tc.stale)
			}
		})
	}
}

func TestOutputsAreDeterministic(t *testing.T) {
	t.Parallel()
	tree := valid(t)
	first := outputs(t, tree)
	for range 5 {
		again := outputs(t, tree)
		if !slices.EqualFunc(first, again, func(a, b coreapi.Output) bool { return a.Path == b.Path && string(a.Data) == string(b.Data) }) {
			t.Fatal("two runs over the same tree gave different outputs")
		}
	}
}

func firstDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return "line " + strconv.Itoa(i+1) + ":\n got  " + g[i] + "\n want " + w[i]
		}
	}
	return "lengths differ: got " + strconv.Itoa(len(g)) + " lines, want " + strconv.Itoa(len(w))
}

// table is a layer table of one row that renders into lint.yml.
const table = `module: example.com/m
renders: {doc: layers.md, lint: lint.yml}
about: A program.
layers:
  - {layer: port, kind: pure, packages: [internal/port], holds: ports, template: port, std: [io], why: "no"}
  - {layer: fake, kind: stand-in, packages: [internal/port/testing], holds: fake, template: testing, may: [internal/port], why: "no"}
effects: {why: "no", packages: [os]}
platform: {roots: [/work/scratch], tools: [systemctl], names: [linux]}
platform: {roots: [/work/scratch], tools: [systemctl], names: [linux]}
ports: {fake: internal/port/testing, list: []}
`

// TestARepositoryWithALayerTableGetsNoRendersFromGen: the layer table's two
// files are gragas-dev's. A stale document and lint file are not listed, a
// broken table does not fail the plan, and the rulebook's own stale files
// still are listed (the control).
func TestARepositoryWithALayerTableGetsNoRendersFromGen(t *testing.T) {
	t.Parallel()
	tree := valid(t)
	tree["layers.yaml"] = &fstest.MapFile{Data: []byte(table + "colour: red\n")}
	tree["layers.md"] = &fstest.MapFile{Data: []byte("by hand\n")}
	tree["lint.yml"] = &fstest.MapFile{Data: []byte("# >>> generated by gragas gen from layers.yaml: edit that, not this block\n# <<< generated by gragas gen\n")}
	l, err := coreapi.Load(t.Context(), (&porttest.Fake{Repo: tree}).Ports(), ".", "")
	if err != nil {
		t.Fatal(err)
	}
	stale, err := coreapi.Stale(l)
	if err != nil {
		t.Fatalf("a broken layer table failed gen: %v", err)
	}
	for _, o := range stale {
		if o.Path == "layers.md" || o.Path == "lint.yml" {
			t.Errorf("gen lists %s, which gragas-dev renders", o.Path)
		}
	}
	delete(tree, "rulebook/coverage.json")
	control, err := coreapi.Stale(l)
	if err != nil || len(control) == 0 {
		t.Errorf("the control: a missing generated file gave %d stale files, %v", len(control), err)
	}
}
