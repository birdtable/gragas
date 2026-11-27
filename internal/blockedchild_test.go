package internal

import (
	"go/ast"
	"slices"
	"testing"
)

// sandboxImport is go-proc's block, which the blocked child sets on itself
// before it execs the program it was given (gragas#1455).
const sandboxImport = "github.com/birdtable/go-proc/sandbox"

// blockedChildren holds the kind of the block's self-start: a package whose
// code imports go-proc's sandbox builds the blocked child's argv, and starts
// its own program, as os.Executable names it, as that child. Under go test
// that program is the package's test binary, so its TestMain must be the
// blocked child first, by calling blockedChild before anything else, or the
// "blocked" member runs the package's tests in its place (PR 639, round 3).
// Each such package with no TestMain, or one whose first statement does not
// call blockedChild, is a finding.
func blockedChildren(all []source) []string {
	starts := map[string]bool{}
	for _, s := range all {
		if s.test {
			continue
		}
		for _, imp := range imports(s.file) {
			if imp == sandboxImport {
				starts[s.pkg] = true
			}
		}
	}
	dispatches := map[string]bool{}
	for _, s := range all {
		if !s.test || !starts[s.pkg] {
			continue
		}
		for _, d := range s.file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil || len(fn.Body.List) == 0 {
				continue
			}
			if callsBlockedChild(fn.Body.List[0]) {
				dispatches[s.pkg] = true
			}
		}
	}
	var out []string
	for pkg := range starts {
		if !dispatches[pkg] {
			out = append(out, pkg+" imports go-proc's sandbox, so it starts its own program as the blocked child, and its TestMain does not call blockedChild first: under go test the blocked member would run the package's tests in its place")
		}
	}
	slices.Sort(out)
	return out
}

// callsBlockedChild is whether stmt calls blockedChild anywhere in it.
func callsBlockedChild(stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "blockedChild" {
				found = true
			}
		}
		return !found
	})
	return found
}

// TestATestBinaryThatCanBeTheBlockedChildDispatchesIt: a package that
// imports go-proc's sandbox with no TestMain, or with one that runs the tests
// before it asks blockedChild, fails; one that asks it first holds, and a
// package that does not import the sandbox needs no TestMain.
func TestATestBinaryThatCanBeTheBlockedChildDispatchesIt(t *testing.T) {
	t.Parallel()
	code := parse(t, "cmd/x", "main.go", "package main\n\nimport \"github.com/birdtable/go-proc/sandbox\"\n\nvar _ = sandbox.Run\n")
	plain := parse(t, "cmd/y", "main.go", "package main\n\nfunc main() {}\n")
	rows := []struct {
		claim string
		all   []source
		fails bool
	}{
		{"no TestMain", []source{code, parse(t, "cmd/x", "x_test.go", "package main\n")}, true},
		{"a TestMain that runs the tests first", []source{code, parse(t, "cmd/x", "x_test.go",
			"package main\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) {\n\tcode := m.Run()\n\tif c, ok := blockedChild(); ok {\n\t\tos.Exit(c)\n\t}\n\tos.Exit(code)\n}\n")}, true},
		{"a TestMain that asks blockedChild first", []source{code, parse(t, "cmd/x", "x_test.go",
			"package main\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) {\n\tif c, ok := blockedChild(); ok {\n\t\tos.Exit(c)\n\t}\n\tos.Exit(m.Run())\n}\n")}, false},
		{"a package that does not import the sandbox", []source{plain, parse(t, "cmd/y", "y_test.go", "package main\n")}, false},
	}
	for _, r := range rows {
		got := blockedChildren(r.all)
		if r.fails {
			fires(t, got, "its TestMain does not call blockedChild first")
		} else if len(got) > 0 {
			t.Errorf("%s: %q, want no finding", r.claim, got)
		}
	}
}
