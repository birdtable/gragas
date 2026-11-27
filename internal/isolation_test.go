package internal

// go.style.6, the Go form of the sibling-hiding resolver: a package's tests
// link no package of the module its own row would not let it reach, and a
// suite that still passes with its package's code emptied is vacuous.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/layers"
)

// reach is every package of the module p may reach: its own, its row's and
// the stand-ins, and all they import in turn.
func reach(tb *layers.Table, w world, p string) map[string]bool {
	start := []string{p}
	if r := tb.RowOf(p); r != nil {
		start = append(append(start, r.May...), tb.StandIns()...)
	}
	seen := map[string]bool{}
	for len(start) > 0 {
		n := start[0]
		start = start[1:]
		if seen[n] {
			continue
		}
		seen[n] = true
		if q := w.pkgs[n]; q != nil {
			for _, imp := range q.imports {
				if r, ok := w.rel(imp); ok {
					start = append(start, r)
				}
			}
		}
	}
	return seen
}

// closures is the test-closure check: every package of the module a test
// binary links, by go list -deps -test, is one its package may reach. A
// test-only package's row is its tests' own.
func closures(tb *layers.Table, w world) []string {
	var out []string
	for _, p := range w.sorted() {
		ok := reach(tb, w, p.path)
		for _, dep := range w.testDeps[p.path] {
			if !ok[dep] {
				out = append(out, p.path+": its tests link "+dep+", which its row does not reach")
			}
		}
	}
	return out
}

func TestATestLinkingAnAdapterFromTheCoreFails(t *testing.T) {
	t.Parallel()
	tb, w := planted(t, pkg{path: "internal/core/rulebook", name: "rulebook", files: []string{"book.go"}, tests: []string{"book_test.go"}})
	w.testDeps = map[string][]string{"internal/core/rulebook": {"internal/port/legacy", "internal/host/fs"}}
	f := closures(tb, w)
	fires(t, f, "internal/core/rulebook: its tests link internal/host/fs")
	if len(f) != 1 {
		t.Errorf("findings = %q, want only the adapter", f)
	}
}

// vacuous is whether the tests of the package in dir, under the module at
// mod, compile and pass with every non-test file of it emptied to its
// package clause: go test -overlay.
func vacuous(t *testing.T, mod, dir, name string, files []string) bool {
	t.Helper()
	tmp := t.TempDir()
	replace := map[string]string{}
	empty := filepath.Join(tmp, "empty.go")
	if err := os.WriteFile(empty, []byte("package "+name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The overlay names a file by the path go sees, links resolved.
	list := exec.CommandContext(t.Context(), "go", "list", "-f", "{{.Dir}}", "./"+filepath.ToSlash(dir)) //nolint:gosec // go list of a package of this module
	list.Dir = mod
	list.Env = configtest.Inherited()
	out, err := list.Output()
	if err != nil {
		t.Fatalf("go list %s: %v", dir, err)
	}
	abs := strings.TrimSpace(string(out))
	for _, f := range files {
		replace[filepath.Join(abs, f)] = empty
	}
	overlay, err := json.Marshal(map[string]any{"Replace": replace})
	if err != nil {
		t.Fatal(err)
	}
	ov := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(ov, overlay, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-overlay="+ov, "./"+filepath.ToSlash(dir)) //nolint:gosec // go test of a package of this module
	cmd.Dir = mod
	cmd.Env = configtest.Inherited()
	return cmd.Run() == nil
}

// noVacuousSuite: every package's tests fail with its code emptied. A
// package with no tests or no code of its own has no suite to empty.
func noVacuousSuite(t *testing.T, w world) {
	t.Helper()
	n := 0
	for _, p := range w.sorted() {
		if len(p.tests) == 0 || len(p.files) == 0 {
			continue
		}
		n++
		t.Run(p.path, func(t *testing.T) {
			t.Parallel()
			if vacuous(t, root, p.path, p.name, p.files) {
				t.Errorf("%s: its tests pass with its code emptied", p.path)
			}
		})
	}
	if n == 0 {
		t.Fatal("no suite was run")
	}
}

// TestAVacuousSuiteIsSeen plants a package whose test asserts only on its
// own fixture, and its control, whose test calls the code.
func TestAVacuousSuiteIsSeen(t *testing.T) {
	t.Parallel()
	mod := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(mod, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/plant\n\ngo 1.27\n")
	write("a/a.go", "package a\n\n// F is one.\nfunc F() int { return 1 }\n")
	write("a/a_test.go", "package a_test\n\nimport \"testing\"\n\nfunc TestItsOwnFixture(t *testing.T) {\n\tif want := 1; want != 1 {\n\t\tt.Error(want)\n\t}\n}\n")
	write("b/b.go", "package b\n\n// F is one.\nfunc F() int { return 1 }\n")
	write("b/b_test.go", "package b_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/plant/b\"\n)\n\nfunc TestF(t *testing.T) {\n\tif b.F() != 1 {\n\t\tt.Error(b.F())\n\t}\n}\n")
	if !vacuous(t, mod, "a", "a", []string{"a.go"}) {
		t.Error("a suite that never calls its package was not seen as vacuous")
	}
	if vacuous(t, mod, "b", "b", []string{"b.go"}) {
		t.Error("a suite that calls its package was seen as vacuous")
	}
}
