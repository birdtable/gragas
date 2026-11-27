package gocode

// The one place a link is made: a test plants one through
// testenv.Link (go-testkit), which skips with one message where the system refuses
// this account a link (tests.admission.5), and shipped code makes one
// through go-files' confine.LinkFolder, a folder's link, which makes a
// junction on Windows, and nothing of gragas's own makes one (arch.ports.4).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// linkMaker is the one call shipped code makes a link with.
const linkMaker = "go-files' confine.LinkFolder"

// isLinkCall is whether name makes a link: os.Symlink, os.Root's Symlink,
// and the system calls under them. A method of the same name on another type
// is refused too; the rule only narrows.
func isLinkCall(name string) bool {
	switch name {
	case "Symlink", "Symlinkat", "CreateSymbolicLink":
		return true
	}
	return false
}

// linkBreaches is every Go source, by slash path from the module's root,
// that names a call that makes a link outside its one place: a test file
// anywhere, as no test plants a link but through testenv.Link, and a file
// that is not a test anywhere, as shipped code links through linkMaker, which
// no source here holds. A use that is not a
// call, a function value, is refused as well.
func linkBreaches(sources map[string]string) []string {
	var out []string
	fset := token.NewFileSet()
	for _, name := range sortedKeys(sources) {
		test := strings.HasSuffix(name, "_test.go")
		f, err := parser.ParseFile(fset, name, sources[name], parser.SkipObjectResolution)
		if err != nil {
			out = append(out, name+": cannot be read: "+err.Error())
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !isLinkCall(sel.Sel.Name) {
				return true
			}
			where := fset.Position(sel.Pos()).Line
			if test {
				out = append(out, name+":"+strconv.Itoa(where)+": makes a link with "+sel.Sel.Name+"; a test plants one with testenv.Link (tests.admission.5)")
			} else {
				out = append(out, name+":"+strconv.Itoa(where)+": makes a link with "+sel.Sel.Name+"; shipped code makes one with "+linkMaker+" (arch.ports.4)")
			}
			return true
		})
	}
	return out
}

// allGoSources reads every Go file under cmd/ and internal/, tests included,
// outside testdata.
func allGoSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestALinkIsMadeInItsOnePlace: no test file makes a link but through
// testenv.Link, and no shipped file makes one but through go-files'
// confine.LinkFolder (tests.admission.5, arch.ports.4).
// Risk: a test that plants a link itself fails on a Windows account that may
// make none instead of skipping by name, and a shipped link made but through
// confine is a symlink Windows refuses where a junction was owed.
func TestALinkIsMadeInItsOnePlace(t *testing.T) {
	t.Parallel()
	src := allGoSources(t)
	for _, must := range []string{"internal/host/sandbox/sandbox.go", "internal/host/fs/links_test.go", "cmd/gragas-new/check.go"} {
		if _, ok := src[must]; !ok {
			t.Fatalf("%s was not read; the walk does not reach what it holds", must)
		}
	}
	if b := linkBreaches(src); len(b) > 0 {
		t.Errorf("links made outside their one place:\n%s", strings.Join(b, "\n"))
	}
}

// TestAPlantedLinkFailsByFileName: a link a test makes itself, one shipped
// code makes itself, in any package, through os.Root, or held as a function
// value, each fails by the file's name; the test helper passes.
// Risk: a check that reads none of the forms it names and passes everything.
func TestAPlantedLinkFailsByFileName(t *testing.T) {
	t.Parallel()
	b := linkBreaches(map[string]string{
		"internal/core/x/x_test.go":  "package x\nimport \"os\"\nfunc f() { _ = os.Symlink(\"a\", \"b\") }\n",
		"internal/host/fs/y_test.go": "package fs\nimport \"os\"\nfunc f() { _ = os.Symlink(\"a\", \"b\") }\n",
		"cmd/gragas-new/z.go":        "package main\nimport \"os\"\nfunc f() { _ = os.Symlink(\"a\", \"b\") }\n",
		"internal/host/sandbox/r.go": "package sandbox\nimport \"os\"\nfunc f(r *os.Root) { _ = r.Symlink(\"a\", \"b\") }\n",
		"internal/host/tool/v.go":    "package tool\nimport \"os\"\nvar mk = os.Symlink\n",
		"internal/host/tool/u.go":    "package tool\nimport \"golang.org/x/sys/unix\"\nfunc f() { _ = unix.Symlinkat(\"a\", 0, \"b\") }\n",
		"internal/host/fs/w.go":      "package fs\nimport \"os\"\nfunc f() { _ = os.Symlink(\"a\", \"b\") }\n",
	})
	fires(t, b, "internal/core/x/x_test.go:3: makes a link with Symlink; a test plants one with testenv.Link")
	fires(t, b, "internal/host/fs/y_test.go:3: makes a link with Symlink; a test plants one")
	fires(t, b, "cmd/gragas-new/z.go:3: makes a link with Symlink; shipped code makes one with go-files' confine.LinkFolder")
	fires(t, b, "internal/host/fs/w.go:3: makes a link with Symlink")
	fires(t, b, "internal/host/sandbox/r.go:3: makes a link with Symlink")
	fires(t, b, "internal/host/tool/v.go:3: makes a link with Symlink")
	fires(t, b, "internal/host/tool/u.go:3: makes a link with Symlinkat")
	if len(b) != 7 {
		t.Errorf("found %d breaches, want the 7 planted: %q", len(b), b)
	}
}
