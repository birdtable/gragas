package configtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoEnvironmentIsSetButThroughTestenv: no non-test file of this package
// sets or clears a variable of the process: every name comes from testenv,
// but GOWORK, which mainKeeping alone sets before testenv.Main keeps it.
// Risk: a second isolation path growing back beside testenv.
func TestNoEnvironmentIsSetButThroughTestenv(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if filepath.Ext(name) != ".go" || isTest(name) {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			in := ""
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				in = fn.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" {
					switch sel.Sel.Name {
					case "Setenv":
						if in != "mainKeeping" {
							t.Errorf("%s calls os.Setenv in %s: the environment is testenv's, but GOWORK in mainKeeping", name, in)
						}
					case "Unsetenv", "Clearenv", "Chdir":
						t.Errorf("%s calls os.%s: the environment is testenv's", name, sel.Sel.Name)
					}
				}
				return true
			})
		}
	}
}

func isTest(name string) bool {
	return len(name) > len("_test.go") && name[len(name)-len("_test.go"):] == "_test.go"
}

// TestNoFileLaunchesThroughUsrBinEnv: no file of this package names the
// launcher the old Rerun started its child by; testenv starts a child by the
// absolute program it finds on the system. Risk: the literal returning with a
// second launch path.
func TestNoFileLaunchesThroughUsrBinEnv(t *testing.T) {
	t.Parallel()
	launcher := strings.Join([]string{"", "usr", "bin", "env"}, "/")
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no files read: %v", err)
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), launcher) {
			t.Errorf("%s names %s", name, launcher)
		}
	}
}
