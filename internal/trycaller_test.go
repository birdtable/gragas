package internal

// The one caller of Plugins.Try (gragas#2039): Try runs a module no person approved, so only plugin test,
// which holds the module to the conformance checks and runs nothing it
// answers, may reach it. This test type-checks every non-test package of the
// module that depends on the port. A reach is counted by what it yields: any
// selection, on any receiver, of a method or func field whose results hold a
// port.Trial, so an interface that narrows port.Plugins to Try is read as the
// port is. Each reach outside the one function allowed, tryModule, fails, and
// so does each use of tryModule outside the one operation allowed,
// PluginTest. A test file may call Try: its module is a fake.

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// thePort is the package of port.Plugins, relative to the module.
const thePort = "internal/port"

const (
	// tryPkg is the package of the one function that may reach Plugins.Try.
	tryPkg = "internal/core/api"
	// tryFunc is that function: coreapi's tryModule.
	tryFunc = "tryModule"
	// tryCaller is tryFunc as a reach names it.
	tryCaller = tryPkg + "." + tryFunc
	// tryOperation is the one function that may use tryModule: the plugin
	// test operation.
	tryOperation = tryPkg + ".PluginTest"
)

// tryWalk is what the walk needs: the export data and the port's Trial.
type tryWalk struct {
	module string
	fset   *token.FileSet
	pkgs   map[string]exportedPkg
	imp    types.Importer
	trial  types.Type
}

// reaches is what the walk finds in a package, each as the function it is in,
// path.function, with where it is: each selection that yields a port.Trial,
// and each use of tryModule.
type reaches struct {
	try, viaCaller []string
}

// portUsers is each package of the module, relative to it, that depends on
// the port: one that does not cannot hold a port.Plugins.
func portUsers(module string) ([]string, error) {
	var out []string
	err := golist(func(dec *json.Decoder) error {
		var l struct {
			ImportPath string
			Deps       []string
		}
		if err := dec.Decode(&l); err != nil {
			return err
		}
		if slices.Contains(l.Deps, module+"/"+thePort) {
			out = append(out, strings.TrimPrefix(l.ImportPath, module+"/"))
		}
		return nil
	}, "-json=ImportPath,Deps", "./...")
	return out, err
}

// newTryWalk loads the export data of scope and the port, and the port's
// Trial.
func newTryWalk(t *testing.T, module string, scope []string) *tryWalk {
	t.Helper()
	pkgs, err := exported(module, append(slices.Clone(scope), thePort))
	if err != nil {
		t.Fatal(err)
	}
	w := &tryWalk{module: module, fset: token.NewFileSet(), pkgs: pkgs}
	w.imp = fromExports(w.fset, pkgs)
	port, err := w.imp.Import(module + "/" + thePort)
	if err != nil {
		t.Fatal(err)
	}
	obj := port.Scope().Lookup("Trial")
	if obj == nil {
		t.Fatal("the port has no Trial: the walk would hold nothing")
	}
	if _, ok := obj.Type().Underlying().(*types.Interface); !ok {
		t.Fatal("port.Trial is no interface")
	}
	w.trial = obj.Type()
	return w
}

// callers is each reach of Plugins.Try in files, and each use of tryModule,
// type-checked as the package rel.
func (w *tryWalk) callers(rel string, files []*ast.File) (reaches, error) {
	info := &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}, Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: w.imp}
	pkg, err := conf.Check(w.module+"/"+rel, w.fset, files, info)
	if err != nil {
		return reaches{}, err
	}
	var caller types.Object
	if rel == tryPkg {
		caller = pkg.Scope().Lookup(tryFunc)
	}
	var out reaches
	for _, f := range files {
		for _, d := range f.Decls {
			in := rel + "." + declName(d)
			ast.Inspect(d, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.SelectorExpr:
					if sel, ok := info.Selections[n]; ok && w.yieldsTrial(sel.Type()) {
						out.try = append(out.try, in+" at "+w.fset.Position(n.Pos()).String())
					}
				case *ast.Ident:
					if caller != nil && info.Uses[n] == caller {
						out.viaCaller = append(out.viaCaller, in+" at "+w.fset.Position(n.Pos()).String())
					}
				}
				return true
			})
		}
	}
	return out, nil
}

// yieldsTrial is whether t is a func whose results hold a port.Trial: the
// type of a selection of Try on the port, on the host, on any other
// implementation or on an interface that narrows the port to Try, as a call,
// a method value, a method expression or a func field.
func (w *tryWalk) yieldsTrial(t types.Type) bool {
	sig, ok := t.Underlying().(*types.Signature)
	if !ok {
		return false
	}
	for v := range sig.Results().Variables() {
		if types.Identical(v.Type(), w.trial) {
			return true
		}
	}
	return false
}

// declName is the name of a declaration's function, with its receiver's
// type; "" for one that is no function.
func declName(d ast.Decl) string {
	f, ok := d.(*ast.FuncDecl)
	if !ok {
		return ""
	}
	if f.Recv == nil || len(f.Recv.List) == 0 {
		return f.Name.Name
	}
	typ := f.Recv.List[0].Type
	if s, ok := typ.(*ast.StarExpr); ok {
		typ = s.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name + "." + f.Name.Name
	}
	return f.Name.Name
}

// parsedFiles is the non-test files of the package rel.
func (w *tryWalk) parsedFiles(rel string) ([]*ast.File, error) {
	l := w.pkgs[w.module+"/"+rel]
	var files []*ast.File
	for _, name := range l.GoFiles {
		f, err := parser.ParseFile(w.fset, filepath.Join(root, rel, name), nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// TestOnlyPluginTestReachesTry: across the module, Plugins.Try is reached
// from coreapi's tryModule and from no other function, and tryModule is used
// by PluginTest and by no other function. Each is reached where it is
// allowed, so a rename does not leave the guard holding nothing. Risk:
// another operation tries a module, and so runs code no person approved.
func TestOnlyPluginTestReachesTry(t *testing.T) {
	t.Parallel()
	module := table(t).Module
	scope, err := portUsers(module)
	if err != nil {
		t.Fatal(err)
	}
	w := newTryWalk(t, module, scope)
	var found reaches
	for _, rel := range scope {
		files, err := w.parsedFiles(rel)
		if err != nil {
			t.Fatal(err)
		}
		r, err := w.callers(rel, files)
		if err != nil {
			t.Fatalf("%s does not type-check, so its calls of Try are not read: %v", rel, err)
		}
		found.try = append(found.try, r.try...)
		found.viaCaller = append(found.viaCaller, r.viaCaller...)
	}
	for _, e := range outside(found) {
		t.Error(e)
	}
	if !slices.ContainsFunc(found.try, inFunc(tryCaller)) {
		t.Errorf("%s reaches Plugins.Try nowhere: the guard holds nothing; name the one caller again", tryCaller)
	}
	if !slices.ContainsFunc(found.viaCaller, inFunc(tryOperation)) {
		t.Errorf("%s uses %s nowhere: the guard holds nothing; name the one operation again", tryOperation, tryCaller)
	}
	t.Logf("read %d packages that depend on the port; Try is reached %d times and %s used %d times", len(scope), len(found.try), tryFunc, len(found.viaCaller))
}

// outside is each reach of r that is not where it is allowed, as the error
// that says so.
func outside(r reaches) []string {
	var out []string
	for _, at := range r.try {
		if !inFunc(tryCaller)(at) {
			out = append(out, fmt.Sprintf("%s reaches Plugins.Try, which runs a module no person approved: only %s may (gragas#2039)", at, tryCaller))
		}
	}
	for _, at := range r.viaCaller {
		if !inFunc(tryOperation)(at) {
			out = append(out, fmt.Sprintf("%s uses %s, which runs a module no person approved: only %s may (gragas#2039)", at, tryCaller, tryOperation))
		}
	}
	return out
}

// inFunc is whether a reach is in the function fn.
func inFunc(fn string) func(string) bool {
	return func(at string) bool { return strings.HasPrefix(at, fn+" at ") }
}

// TestAPlantedReachOfTryIsFound: each way of reaching Try, planted in a
// package of its own, is found: a call through the port, a method value, a
// method expression, a call on the host itself, a call through an interface
// that narrows the port to Try, a func field that holds Try, and a call from
// coreapi in a function that is not tryModule. So is each use of tryModule
// outside PluginTest: a call from another coreapi function, and tryModule
// held as a value. Risk: a walk that reads only one spelling.
func TestAPlantedReachOfTryIsFound(t *testing.T) {
	t.Parallel()
	module := table(t).Module
	w := newTryWalk(t, module, []string{thePort, "internal/host/plugin"})
	head := "package planted\n\nimport (\n\t\"context\"\n\n\tport \"" + module + "/" + thePort + "\"\n\thostplugin \"" + module + "/internal/host/plugin\"\n\t\"" + module + "/internal/model/plugin\"\n)\n\nvar _ = hostplugin.New\nvar _ plugin.Manifest\nvar _ context.Context\nvar _ port.Ports\n\n"
	// theCaller is tryModule as coreapi has it, for the plants that use it.
	const theCaller = "func tryModule(ctx context.Context, p port.Ports) (port.Trial, error) { return p.Plugins.Try(ctx, plugin.Manifest{}, nil) }\n\n"
	for _, c := range []struct {
		name, path, body string
		want             int
	}{
		{"a call through the port", "internal/planted", "func f(ctx context.Context, p port.Ports) { _, _ = p.Plugins.Try(ctx, plugin.Manifest{}, nil) }", 1},
		{"a method value", "internal/planted", "func f(p port.Plugins) any { return p.Try }", 1},
		{"a method expression", "internal/planted", "var g = port.Plugins.Try", 1},
		{"a call on the host", "internal/planted", "func f(ctx context.Context, h *hostplugin.Host) { _, _ = h.Try(ctx, plugin.Manifest{}, nil) }", 1},
		{"a call through a narrowing interface", "internal/planted", "type tryer interface {\n\tTry(context.Context, plugin.Manifest, []byte) (port.Trial, error)\n}\n\nfunc f(ctx context.Context, p port.Ports) { var t tryer = p.Plugins; _, _ = t.Try(ctx, plugin.Manifest{}, nil) }", 1},
		{"a func field that holds Try", "internal/planted", "type s struct {\n\ttry func(context.Context, plugin.Manifest, []byte) (port.Trial, error)\n}\n\nfunc f(ctx context.Context, p port.Ports) { v := s{try: p.Plugins.Try}; _, _ = v.try(ctx, plugin.Manifest{}, nil) }", 2},
		{"coreapi outside tryModule", tryPkg, "func other(ctx context.Context, p port.Ports) { _, _ = p.Plugins.Try(ctx, plugin.Manifest{}, nil) }", 1},
		{"another coreapi function calls tryModule", tryPkg, theCaller + "func other(ctx context.Context, p port.Ports) { _, _ = tryModule(ctx, p) }", 1},
		{"tryModule held as a value", tryPkg, theCaller + "var g = tryModule", 1},
	} {
		f, err := parser.ParseFile(w.fset, "planted.go", head+c.body, 0)
		if err != nil {
			t.Fatalf("%s: the plant does not parse: %v", c.name, err)
		}
		found, err := w.callers(c.path, []*ast.File{f})
		if err != nil {
			t.Fatalf("%s: the plant does not type-check: %v", c.name, err)
		}
		if got := outside(found); len(got) != c.want {
			t.Errorf("%s: found %v outside, of %+v; want %d", c.name, got, found, c.want)
		}
	}
	// The clean control: tryModule in coreapi is the one reach allowed,
	// PluginTest the one use of it allowed, and a Try that yields no
	// port.Trial is no reach.
	clean := theCaller + "type other struct{}\n\nfunc (other) Try() {}\n\nfunc PluginTest(ctx context.Context, p port.Ports) { _, _ = tryModule(ctx, p); other{}.Try() }"
	f, err := parser.ParseFile(w.fset, "planted.go", head+clean, 0)
	if err != nil {
		t.Fatal(err)
	}
	found, err := w.callers(tryPkg, []*ast.File{f})
	if err != nil {
		t.Fatal(err)
	}
	if got := outside(found); len(got) != 0 || len(found.try) != 1 || len(found.viaCaller) != 1 {
		t.Errorf("the clean control: found %+v, %v outside; want one reach in %s and one use in %s", found, got, tryCaller, tryOperation)
	}
}
