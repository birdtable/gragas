package internal

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// The two parsers of input the engine did not write, each with its one
// owner (tests.generated.1, arch.ports.6): YAML and Markdown.
func parserOwners() []struct{ library, owner, fuzz string } {
	return []struct{ library, owner, fuzz string }{
		{"go.yaml.in/yaml/v3", "internal/ordered", "FuzzParseYAML"},
		{"github.com/yuin/goldmark", "internal/markdown", "FuzzParse"},
	}
}

// importers is the packages of w that import a library, by path, in a file
// of the package or in one of its tests.
func importers(w world, library string) []string {
	var out []string
	for _, p := range w.sorted() {
		for _, imp := range append(slices.Clone(p.imports), p.testImports...) {
			if imp == library || strings.HasPrefix(imp, library+"/") {
				out = append(out, p.path)
				break
			}
		}
	}
	return out
}

// importedByOne is arch.ports.6 for one library: exactly one package of w
// imports it, and it is the owner.
func importedByOne(w world, library, owner string) []string {
	got := importers(w, library)
	if len(got) == 1 && got[0] == owner {
		return nil
	}
	if len(got) == 0 {
		return []string{library + ": imported by no package, and " + owner + " should"}
	}
	return []string{library + ": imported by " + strings.Join(got, ", ") + ", and only " + owner + " may"}
}

func TestTheMarkdownAndYAMLLibrariesAreEachImportedByOnePackage(t *testing.T) {
	t.Parallel()
	_, w := tree(t)
	for _, po := range parserOwners() {
		if got := importedByOne(w, po.library, po.owner); len(got) > 0 {
			t.Errorf("%v", got)
		}
	}
}

func TestAnotherPackageImportingTheMarkdownLibraryFails(t *testing.T) {
	t.Parallel()
	_, w := planted(t, pkg{path: "internal/core/rulebook", name: "rulebook", files: []string{"book.go"},
		imports: []string{"github.com/yuin/goldmark", "github.com/yuin/goldmark/ast"}})
	w.pkgs["internal/markdown"] = &pkg{path: "internal/markdown", name: "markdown", files: []string{"m.go"},
		imports: []string{"github.com/yuin/goldmark"}}
	fires(t, importedByOne(w, "github.com/yuin/goldmark", "internal/markdown"),
		"imported by internal/core/rulebook, internal/markdown, and only internal/markdown may")
}

func TestATestImportingTheYAMLLibraryFails(t *testing.T) {
	t.Parallel()
	_, w := planted(t, pkg{path: "internal/ordered", name: "ordered", files: []string{"y.go"},
		imports: []string{"go.yaml.in/yaml/v3"}})
	w.pkgs["internal/core/api"] = &pkg{path: "internal/core/api", name: "coreapi", files: []string{"a_test.go"},
		testImports: []string{"go.yaml.in/yaml/v3"}}
	fires(t, importedByOne(w, "go.yaml.in/yaml/v3", "internal/ordered"), "imported by internal/core/api, internal/ordered")
}

func TestALibraryNobodyImportsFails(t *testing.T) {
	t.Parallel()
	_, w := planted(t, pkg{path: "internal/ordered", name: "ordered", files: []string{"y.go"}})
	fires(t, importedByOne(w, "github.com/yuin/goldmark", "internal/markdown"), "imported by no package")
}

var fuzzFunc = regexp.MustCompile(`(?m)^func (Fuzz\w+)\(`)

// fuzzTargets is tests.generated.1 over a tree: every fuzz target of a test
// file has a folder of checked-in seeds at testdata/fuzz/<name> beside it
// with at least one seed in it. It returns the targets it found and what is
// wrong with them.
func fuzzTargets(fsys fs.FS) (found, problems []string) {
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "node_modules") {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		for _, m := range fuzzFunc.FindAllStringSubmatch(string(data), -1) {
			found = append(found, path.Dir(p)+"."+m[1])
			seeds, _ := fs.ReadDir(fsys, path.Join(path.Dir(p), "testdata", "fuzz", m[1]))
			if len(seeds) == 0 {
				problems = append(problems, path.Dir(p)+": "+m[1]+" has no checked-in seeds in testdata/fuzz/"+m[1])
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	return found, problems
}

func TestEveryFuzzTargetHasCheckedInSeeds(t *testing.T) {
	t.Parallel()
	found, problems := fuzzTargets(os.DirFS(root))
	if len(problems) > 0 {
		t.Errorf("%q", problems)
	}
	for _, po := range parserOwners() {
		want := po.owner + "." + po.fuzz
		if !slices.Contains(found, want) {
			t.Errorf("%s is not a fuzz target of the tree; found %q", want, found)
		}
	}
}

func TestAFuzzTargetWithNoSeedsFails(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"internal/p/p_test.go":                     {Data: []byte("package p\n\nfunc FuzzP(f *testing.F) {}\n")},
		"internal/q/q_test.go":                     {Data: []byte("package q\n\nfunc FuzzQ(f *testing.F) {}\n")},
		"internal/q/testdata/fuzz/FuzzQ/a-seed":    {Data: []byte("go test fuzz v1\n[]byte(\"\")\n")},
		"internal/q/testdata/fuzz/FuzzOther/other": {Data: []byte("x")},
	}
	found, problems := fuzzTargets(fsys)
	if len(found) != 2 {
		t.Fatalf("found %q, want both targets", found)
	}
	fires(t, problems, "internal/p: FuzzP has no checked-in seeds")
	for _, p := range problems {
		if strings.Contains(p, "FuzzQ") {
			t.Errorf("FuzzQ has a seed and was named: %s", p)
		}
	}
}
