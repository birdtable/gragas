package journeys

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// The push's golden tests: internal/journeys/goldens.txt lists every test that compares a
// command's output with a golden, one name to a line, and the push member journeys-goldens runs those and
// no other journey, so a branch that changes a command's output without its golden is red at push, not at
// review. This test builds the list from the sources: a test that reads a golden, directly or through a
// helper of this package, and is not on the list is red, and so is a line that names no such test.

// goldenReaders are the calls that read a golden: the shared module's uitest.Golden and this package's two.
func goldenReaders() []string { return []string{"Golden", "matchesGolden", "goldenOf"} }

// goldenTestsIn returns, sorted, the names of the Test functions in the source files (name to text) that
// read a golden, following calls to this package's own functions.
func goldenTestsIn(t *testing.T, sources map[string]string) []string {
	t.Helper()
	fset := gotoken.NewFileSet()
	calls := map[string]map[string]bool{} // function -> the names it calls
	for name, text := range sources {
		f, err := parser.ParseFile(fset, name, text, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil {
				continue
			}
			set := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					switch f := c.Fun.(type) {
					case *ast.Ident:
						set[f.Name] = true
					case *ast.SelectorExpr:
						set[f.Sel.Name] = true
					}
				}
				return true
			})
			calls[fn.Name.Name] = set
		}
	}
	reads := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for fn, set := range calls {
			if reads[fn] {
				continue
			}
			for c := range set {
				if reads[c] || slices.Contains(goldenReaders(), c) {
					reads[fn], changed = true, true
					break
				}
			}
		}
	}
	var out []string
	for fn := range reads {
		if strings.HasPrefix(fn, "Test") && fn != "TestMain" {
			out = append(out, fn)
		}
	}
	slices.Sort(out)
	return out
}

func TestTheGoldensListNamesEveryTestThatReadsAGolden(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_test.go") {
			b, err := os.ReadFile(e.Name())
			if err != nil {
				t.Fatal(err)
			}
			sources[e.Name()] = string(b)
		}
	}
	raw, err := os.ReadFile("goldens.txt")
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for l := range strings.SplitSeq(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			listed = append(listed, l)
		}
	}
	slices.Sort(listed)
	want := goldenTestsIn(t, sources)
	for _, w := range want {
		if !slices.Contains(listed, w) {
			t.Errorf("%s reads a golden and is not in goldens.txt: a drifted golden would reach review", w)
		}
	}
	for _, l := range listed {
		if !slices.Contains(want, l) {
			t.Errorf("goldens.txt lists %s, which is no test that reads a golden", l)
		}
	}
	if len(want) < 20 {
		t.Errorf("only %d golden tests found: the walk read nothing it should", len(want))
	}
}

// A test that reads a golden through a helper is found, and one that does not is not.
func TestTheGoldenWalkFollowsAHelperAndSkipsATestWithNoGolden(t *testing.T) {
	t.Parallel()
	got := goldenTestsIn(t, map[string]string{"a_test.go": `package p
func helper(t T) { goldenOf(t, "x") }
func TestDirect(t T) { uitest.Golden(t, "x", "y") }
func TestViaHelper(t T) { helper(t) }
func TestNone(t T) { run(t) }
func TestMain(m M) { helper(nil) }
`})
	if want := []string{"TestDirect", "TestViaHelper"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
