package internal

import (
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// blockedMains lists the programs whose main starts the blocked child first
// (gragas#1759, #1769): go-proc's block leaves open a descriptor another
// goroutine makes without syscall.ForkLock, so sandbox.Run is called from a
// process that has started no such goroutine.
func blockedMains() []string { return []string{"cmd/gragas-dev", "cmd/gragas-release"} }

// blockedFirst holds the order of each blocked main (gragas#1769): the first
// statement of main that does work is the dispatch, an if whose init calls
// blockedChild. A defer, a declaration or an assignment with no call in it may
// come before; any other statement, or a call, may not. A main that has no
// dispatch, or no main at all, is a finding too. The source is read as a
// syntax tree, never as text.
func blockedFirst(all []source) []string {
	var out []string
	for _, pkg := range blockedMains() {
		var main *ast.FuncDecl
		for _, s := range all {
			if s.pkg != pkg || s.test {
				continue
			}
			for _, d := range s.file.Decls {
				if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "main" && fn.Body != nil {
					main = fn
				}
			}
		}
		if main == nil {
			out = append(out, pkg+" has no main to hold the blocked child's dispatch in")
			continue
		}
		if msg := dispatchOrder(main.Body.List); msg != "" {
			out = append(out, pkg+": main "+msg)
		}
		out = append(out, beforeMain(pkg, all)...)
	}
	return out
}

// pureInitCalls are the calls a package-level initialiser of a blocked main
// may make: each builds a value and starts nothing (no goroutine, no
// descriptor), as the sentinel errors and the compiled patterns do.
func pureInitCalls() []string { return []string{"errors.New", "regexp.MustCompile"} }

// beforeMain is what of pkg runs before main's dispatch (gragas#1769): every
// non-test file is walked for an init function, and for a package-level
// variable whose initialiser calls anything but a pureInitCalls call. Both run
// before main starts, so each would start work ahead of the blocked child.
func beforeMain(pkg string, all []source) []string {
	var out []string
	for _, s := range all {
		if s.pkg != pkg || s.test {
			continue
		}
		for _, d := range s.file.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" {
					out = append(out, s.path+": an init function runs before main, so before the blocked child's dispatch")
				}
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					vs, ok := sp.(*ast.ValueSpec)
					if !ok || d.Tok.String() != "var" {
						continue
					}
					for _, v := range vs.Values {
						if call := impureCall(v); call != "" {
							out = append(out, s.path+": the package-level variable "+vs.Names[0].Name+" is set by a call to "+call+" before main, so before the blocked child's dispatch")
						}
					}
				}
			}
		}
	}
	return out
}

// impureCall is the first call in n that is not a pureInitCalls call, as it
// is written, or "".
func impureCall(n ast.Node) string {
	bad := ""
	ast.Inspect(n, func(x ast.Node) bool {
		c, ok := x.(*ast.CallExpr)
		if !ok || bad != "" {
			return bad == ""
		}
		name := ""
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				name = id.Name + "." + sel.Sel.Name
			}
		}
		if name == "" || !slices.Contains(pureInitCalls(), name) {
			bad = name
			if bad == "" {
				bad = "a function"
			}
		}
		return bad == ""
	})
	return bad
}

// packageFiles is every non-test Go file of pkg as the tree has it.
func packageFiles(t *testing.T, pkg string) []source {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(pkg)))
	if err != nil {
		t.Fatal(err)
	}
	var out []source
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pkg), e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, parse(t, pkg, e.Name(), string(data)))
	}
	return out
}

// dispatchOrder is why list does not open with the blocked child's dispatch,
// or "" when it does.
func dispatchOrder(list []ast.Stmt) string {
	for _, st := range list {
		if isDispatch(st) {
			return ""
		}
		switch st.(type) {
		case *ast.DeferStmt:
			continue
		case *ast.DeclStmt, *ast.AssignStmt:
			if !holdsACall(st) {
				continue
			}
		}
		return "does work before it asks blockedChild: the blocked child must start before any other call of main runs"
	}
	return "never asks blockedChild"
}

// isDispatch is whether st is `if code, ok := blockedChild(); ... {`.
func isDispatch(st ast.Stmt) bool {
	ifs, ok := st.(*ast.IfStmt)
	if !ok {
		return false
	}
	as, ok := ifs.Init.(*ast.AssignStmt)
	if !ok || len(as.Rhs) != 1 {
		return false
	}
	c, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := c.Fun.(*ast.Ident)
	return ok && id.Name == "blockedChild"
}

// holdsACall is whether a call is anywhere in n.
func holdsACall(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(x ast.Node) bool {
		if _, ok := x.(*ast.CallExpr); ok {
			found = true
		}
		return !found
	})
	return found
}

// withFile is all with the file at path swapped for planted.
func withFile(all []source, planted source) []source {
	var out []source
	for _, s := range all {
		if s.path == planted.path {
			s = planted
		}
		out = append(out, s)
	}
	return out
}

// TestEachBlockedMainStartsTheBlockedChildFirst (gragas#1769): the real
// commands hold, and each of these is red in a copy of either: a call put
// before the dispatch in main, a main with no dispatch or none, a dispatch
// after a non-call statement, an init function, and a package-level variable
// set by a call that is not a pure one. A defer or a plain variable before the
// dispatch holds, and so does a variable set by errors.New or
// regexp.MustCompile.
func TestEachBlockedMainStartsTheBlockedChildFirst(t *testing.T) {
	t.Parallel()
	var real []source
	for _, pkg := range blockedMains() {
		real = append(real, packageFiles(t, pkg)...)
	}
	if got := blockedFirst(real); len(got) > 0 {
		t.Errorf("the real commands: %q, want none", got)
	}
	const open = "func main() {\n"
	for _, pkg := range blockedMains() {
		var mainFile source
		for _, s := range real {
			if s.path == pkg+"/main.go" {
				mainFile = s
			}
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pkg), "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if mainFile.file == nil || !strings.Contains(text, open) {
			t.Fatalf("%s: no main.go with %q to plant after", pkg, open)
		}
		for _, ins := range []string{
			"\tsetup()\n",
			"\tif x := setup(); x {\n\t}\n",
			"\tgo setup()\n",
			"\tvar v = setup()\n\t_ = v\n",
		} {
			planted := parse(t, pkg, "main.go", strings.Replace(text, open, open+ins, 1))
			fires(t, blockedFirst(withFile(real, planted)), "does work before it asks blockedChild")
		}
		quiet := parse(t, pkg, "main.go", strings.Replace(text, open, open+"\tvar v int\n\t_ = v\n\tdefer func() {}()\n", 1))
		if got := blockedFirst(withFile(real, quiet)); len(got) > 0 {
			t.Errorf("%s with a plain variable and a defer before the dispatch: %q, want none", pkg, got)
		}
		for claim, extra := range map[string]struct{ src, want string }{
			"an init that starts a goroutine":  {"package main\n\nfunc init() {\n\tgo func() {}()\n}\n", "an init function runs before main"},
			"a variable set by a call":         {"package main\n\nvar started = start()\n\nfunc start() int { return 1 }\n", "is set by a call to a function before main"},
			"a variable set by a package call": {"package main\n\nimport \"os\"\n\nvar here, _ = os.Getwd()\n", "is set by a call to os.Getwd before main"},
		} {
			got := blockedFirst(append(slices.Clone(real), parse(t, pkg, "zz_plant.go", extra.src)))
			if len(got) == 0 {
				t.Errorf("%s in %s: no finding, want one saying %q", claim, pkg, extra.want)
				continue
			}
			fires(t, got, extra.want)
		}
		pure := parse(t, pkg, "zz_pure.go", "package main\n\nimport (\n\t\"errors\"\n\t\"regexp\"\n)\n\nvar (\n\terrX = errors.New(\"x\")\n\tformX = regexp.MustCompile(`^x$`)\n)\n")
		if got := blockedFirst(append(slices.Clone(real), pure)); len(got) > 0 {
			t.Errorf("%s with errors.New and regexp.MustCompile initialisers: %q, want none", pkg, got)
		}
	}
	other := packageFiles(t, blockedMains()[1])
	none := parse(t, blockedMains()[0], "main.go", "package main\n\nfunc main() {\n}\n")
	fires(t, blockedFirst(append(slices.Clone(other), none)), "never asks blockedChild")
	fires(t, blockedFirst(other), "cmd/gragas-dev has no main")
}
