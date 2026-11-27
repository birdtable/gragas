package gocode

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// exitOwner is the one package that declares an exit code (cli.exit.2).
const exitOwner = "internal/exit"

// exitName is a name that declares an exit code: ExitDone, exitCode, exit_6.
var exitName = regexp.MustCompile(`^(?i)exit`)

// exitBreaches is go.errors.4 and cli.exit.2 held against Go sources, by
// slash path from the module's root with their text: only a main package
// under cmd/ calls os.Exit, and an integer literal is never an exit code
// outside internal/exit, whether it is given to os.Exit or declared under a
// name that starts with exit. Test
// files are not read: a TestMain hands os.Exit the code the test runner
// computed.
func exitBreaches(sources map[string]string) []string {
	var out []string
	fset := token.NewFileSet()
	for _, name := range sortedKeys(sources) {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, sources[name], 0)
		if err != nil {
			out = append(out, name+": cannot be read: "+err.Error())
			continue
		}
		dir := path.Dir(name)
		mainOK := f.Name.Name == "main" && strings.HasPrefix(dir, "cmd/")
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if isOSExit(n) {
					if !mainOK {
						out = append(out, name+": calls os.Exit; only main exits (go.errors.4)")
					}
					if len(n.Args) == 1 && isIntLit(n.Args[0]) {
						out = append(out, name+": gives os.Exit a literal; the code comes from "+exitOwner+" (cli.exit.2)")
					}
				}
			case *ast.ValueSpec:
				if dir == exitOwner {
					return true
				}
				for i, id := range n.Names {
					if exitName.MatchString(id.Name) && i < len(n.Values) && isIntLit(n.Values[i]) {
						out = append(out, name+": declares "+id.Name+" as a literal exit code; "+exitOwner+" is the one place (cli.exit.2)")
					}
				}
			}
			return true
		})
	}
	return out
}

func isOSExit(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Exit" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "os"
}

func isIntLit(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.INT
}

func sortedKeys(m map[string]string) []string {
	return slices.Sorted(maps.Keys(m))
}

// goSources reads every non-test Go file under cmd/ and internal/ of the
// tree, outside testdata.
func goSources(t *testing.T) map[string]string {
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
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
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

func TestOnlyMainExitsAndOnlyExitDeclaresACode(t *testing.T) {
	t.Parallel()
	src := goSources(t)
	if len(src) < 10 {
		t.Fatalf("read %d source files; the walk found too few to mean anything", len(src))
	}
	if _, ok := src["cmd/gragas/main.go"]; !ok {
		t.Fatal("cmd/gragas/main.go was not read")
	}
	if _, ok := src[exitOwner+"/exit.go"]; !ok {
		t.Fatal(exitOwner + "/exit.go was not read")
	}
	if b := exitBreaches(src); len(b) > 0 {
		t.Errorf("exit codes and exits:\n%s", strings.Join(b, "\n"))
	}
}

func TestAnOSExitOutsideMainFails(t *testing.T) {
	t.Parallel()
	b := exitBreaches(map[string]string{
		"internal/core/api/x.go": "package coreapi\nimport \"os\"\nfunc f() { os.Exit(code()) }\nfunc code() int { return 0 }\n",
		"cmd/gragas/other.go":    "package other\nimport \"os\"\nfunc f() { os.Exit(code()) }\nfunc code() int { return 0 }\n",
	})
	fires(t, b, "internal/core/api/x.go: calls os.Exit; only main exits")
	fires(t, b, "cmd/gragas/other.go: calls os.Exit; only main exits")
}

func TestAnExitLiteralOutsideTheExitPackageFails(t *testing.T) {
	t.Parallel()
	b := exitBreaches(map[string]string{
		"cmd/gragas/main.go":      "package main\nimport \"os\"\nfunc main() { os.Exit(7) }\n",
		"internal/cli/root/r.go":  "package root\nconst exitDeclined = 6\n",
		"internal/core/api/ok.go": "package coreapi\nconst limit = 6\n",
	})
	fires(t, b, "cmd/gragas/main.go: gives os.Exit a literal")
	fires(t, b, "internal/cli/root/r.go: declares exitDeclined as a literal exit code")
	for _, line := range b {
		if strings.Contains(line, "ok.go") {
			t.Errorf("a constant that is not an exit code was refused: %s", line)
		}
	}
}

func TestMainMayExitAndTheExitPackageMayDeclareCodes(t *testing.T) {
	t.Parallel()
	b := exitBreaches(map[string]string{
		"cmd/gragas/main.go":    "package main\nimport \"os\"\nfunc main() { os.Exit(run()) }\nfunc run() int { return 0 }\n",
		"internal/exit/exit.go": "package exit\nconst Done = 0\nconst ExitSix = 6\n",
	})
	if len(b) > 0 {
		t.Errorf("a legal exit was refused: %q", b)
	}
}

// TestNoCommandDeclaresItsOwnCodes: a command that names its codes takes
// them from internal/exit; gragas-dev's three literals went with the landing
// judge they served. The plants: gragas-dev's old three, and a
// code in another command.
// Risk: a second place that declares a code, which a block bump would leave behind.
func TestNoCommandDeclaresItsOwnCodes(t *testing.T) {
	t.Parallel()
	if b := exitBreaches(map[string]string{
		"cmd/gragas-dev/main.go": "package main\nimport \"x/internal/exit\"\nconst (\n\texitDone   = exit.Done\n\texitFailed = exit.Failed\n)\n",
	}); len(b) > 0 {
		t.Errorf("codes taken from internal/exit were refused: %q", b)
	}
	b := exitBreaches(map[string]string{
		"cmd/gragas-dev/main.go":      "package main\nconst (\n\texitDone   = 0\n\texitFailed = 1\n\texitUsage  = 2\n)\n",
		"cmd/gragas-generate/main.go": "package main\nconst exitBusy = 3\n",
	})
	fires(t, b, "cmd/gragas-dev/main.go: declares exitDone as a literal exit code")
	fires(t, b, "cmd/gragas-dev/main.go: declares exitUsage as a literal exit code")
	fires(t, b, "cmd/gragas-generate/main.go: declares exitBusy as a literal exit code")
}
