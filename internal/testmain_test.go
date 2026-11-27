package internal

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// Go keys a package's test cache on what its tests open, stat and read from
// the environment, and records that only inside m.Run. A TestMain that reads
// the tree before m.Run reads it into no key: a change to a file only that
// read sees replays the package's results from the cache (gragas#1222). So a
// TestMain makes no call before m.Run but the ones below, and a shared read is
// a sync.Once called from a test's body. Nor does an init function or a
// package-level var initialiser of such a package call anything but the
// constructors of pureInit: Go runs both before TestMain (gragas#1273).

// beforeRun are the calls a TestMain may make. None reads the tree: the
// first six run the tests and exit (crashMain is Main keeping the crash
// settings, in crashMainFile alone, gragas#340), flag's two declare a flag,
// and the rest are a child's entries, which answer at once, from os.Args or the
// environment, that the run of the tests is not their child.
// crashMain is the TestMain that keeps the crash settings from the run that
// started the tests, and crashMainFile the one file whose TestMain may call
// it: any other would let them, or a name like them, through its isolation.
const (
	crashMain     = "configtest.MainCrash"
	crashMainFile = "internal/journeys/main_test.go"
	// crashMainOwner is the folder of configtest itself, which defines it:
	// that folder alone, not one below it.
	crashMainOwner = "internal/config/testing"
)

func beforeRun() []string {
	return []string{
		// configtest.MainContained is Main run again in go-testkit's contain, for
		// internal/journeys/overflow (gragas#1828): it reads nothing before m.Run
		"os.Exit", "configtest.Main", crashMain, "configtest.MainContained", "configtest.NoLeaks", "Main", "m.Run",
		"flag.Lookup", "flag.Bool",
		"blockedChild", "rowChild", "runAsHelper",
		// host/git's stand-in for git, this test binary rerun under another
		// name (gragas#1581): in a run of the tests, named git.test, it
		// answers from os.Args[0] alone and reads nothing
		"git.StandInMain",
		// gragas-dev's stand-ins for the clone's task and for itself, run by
		// its hook tests (gragas#1655): in a run of the tests, named
		// gragas-dev.test, it answers from os.Args[0] alone and reads nothing
		"standInChild",
	}
}

// earlyReads is each TestMain of files, by slash path, that can read before
// m.Run: a call in its body that is not one of beforeRun, and, in the Run
// method of a value it hands configtest.Main, any call before that method's
// m.Run. n is how many TestMains it read.
func earlyReads(fset *token.FileSet, files map[string]*ast.File) (found []string, n int) {
	allowed := beforeRun()
	byDir := map[string][]*ast.File{}
	for rel, f := range files {
		byDir[path.Dir(rel)] = append(byDir[path.Dir(rel)], f)
	}
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		for _, d := range files[rel].Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil {
				continue
			}
			n++
			at := func(p token.Pos) string { return rel + ":" + strconv.Itoa(fset.Position(p).Line) }
			ast.Inspect(fn.Body, func(x ast.Node) bool {
				switch x := x.(type) {
				case *ast.CallExpr:
					if name := callName(x.Fun); !slices.Contains(allowed, name) {
						found = append(found, at(x.Pos())+": TestMain calls "+name+" before m.Run")
					} else if name == crashMain && rel != crashMainFile {
						found = append(found, at(x.Pos())+": TestMain calls "+name+", which only "+crashMainFile+" may: it keeps the crash settings")
					}
				case *ast.CompositeLit:
					if id, ok := x.Type.(*ast.Ident); ok {
						found = append(found, runReads(fset, byDir[path.Dir(rel)], id.Name, at(x.Pos()))...)
					}
				}
				return true
			})
		}
	}
	return found, n
}

// pureInit are the calls an init function or a package-level var initialiser
// may make: constructors that read nothing, a flag declared, and the builtins.
// sync's three take a function and call it later, from a test's body. A call is
// on the list only when its name means what the list says (pureCall): a call
// of fmt's Sprintf or Errorf reads what a Stringer reads, so those two are on it
// only with constants for arguments.
func pureInit() []pureName {
	return []pureName{
		{"regexp", "MustCompile"}, {"regexp", "QuoteMeta"}, {"flag", "Bool"}, {"errors", "New"}, {"strings", "NewReplacer"},
		{"sync", "OnceValue"}, {"sync", "OnceValues"}, {"sync", "OnceFunc"},
		{"fmt", "Errorf"}, {"fmt", "Sprintf"},
	}
}

// pureName is a function of a standard package by the package's import path.
type pureName struct{ path, name string }

// pureBuiltins are the builtins an init function or var initialiser may call.
func pureBuiltins() []string { return []string{"make", "new", "len", "cap", "append"} }

// constantArgs are the functions that read nothing only with constants for
// arguments: a Stringer or an error an argument holds runs its own code.
func constantArgs() []pureName { return []pureName{{"fmt", "Errorf"}, {"fmt", "Sprintf"}} }

// declaredIn is what the files of one package declare at the top level: every
// func (not a method), var, const and type name, and which of them are consts.
type declaredIn struct{ names, consts map[string]bool }

func declarations(files []*ast.File) declaredIn {
	d := declaredIn{names: map[string]bool{}, consts: map[string]bool{}}
	for _, f := range files {
		for _, decl := range f.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil {
					d.names[decl.Name.Name] = true
				}
			case *ast.GenDecl:
				// A const group's spec with no type and no value inherits both from the one before it, so the
				// type to judge is the last one a spec of the group gave.
				var groupType ast.Expr
				for _, sp := range decl.Specs {
					switch sp := sp.(type) {
					case *ast.ValueSpec:
						if sp.Type != nil || len(sp.Values) > 0 {
							groupType = sp.Type
						}
						for _, id := range sp.Names {
							d.names[id.Name] = true
							d.consts[id.Name] = decl.Tok == token.CONST && plainType(groupType)
						}
					case *ast.TypeSpec:
						d.names[sp.Name.Name] = true
					}
				}
			}
		}
	}
	return d
}

// plainType is whether a const's declared type runs no code of its own when the const is formatted: none
// (an untyped const), or a builtin type's name. A type of the package or of an import can have a String method
// that reads, so a const of one is not a constant to Sprintf (gragas#1409).
func plainType(t ast.Expr) bool {
	if t == nil {
		return true
	}
	id, ok := t.(*ast.Ident)
	return ok && id.Obj == nil && slices.Contains(builtinTypes(), id.Name)
}

// builtinTypes are the names of Go's own types a conversion or a const may be of.
func builtinTypes() []string {
	return []string{"string", "byte", "rune", "bool", "error", "any",
		"int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128"}
}

// importsOf is the name each import of f goes by, to its path: the alias, or
// the last element of the path, without a major version's "v2".
func importsOf(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if _, err := strconv.Atoi(strings.TrimPrefix(name, "v")); err == nil && strings.HasPrefix(name, "v") {
			name = path.Base(path.Dir(p))
		}
		if im.Name != nil {
			name = im.Name.Name
		}
		out[name] = p
	}
	return out
}

// goFiles is the Go files of the tree by folder, which a name is resolved in: the
// folder a file is in, and the folder an import of the module's own names.
type goFiles struct {
	byDir map[string][]*ast.File
	decls map[string]declaredIn
}

// modulePath is the module's own path, whose imports are folders of the world.
const modulePath = "github.com/birdtable/gragas"

// declared is what the package of folder dir declares, read once.
func (w *goFiles) declared(dir string) declaredIn {
	if d, ok := w.decls[dir]; ok {
		return d
	}
	d := declarations(w.byDir[dir])
	w.decls[dir] = d
	return d
}

// pureCall is whether a call of f, a file of the package in folder dir, is one
// of pureInit, and a call of the builtin or the standard function it is named
// for: a builtin only where nothing the package declares, or the file's own
// scope, hides its name; a package's function only where the name before the dot
// is the file's import of that package and nothing hides that name; and
// Sprintf and Errorf only with arguments that run no code of the tree's own
// (pureArg), since a Stringer or an error an argument holds runs its own method
// there (gragas#1409).
func (w *goFiles) pureCall(c *ast.CallExpr, f *ast.File, dir string, depth int) bool {
	decl := w.declared(dir)
	switch fun := c.Fun.(type) {
	case *ast.Ident:
		return slices.Contains(pureBuiltins(), fun.Name) && fun.Obj == nil && !decl.names[fun.Name]
	case *ast.SelectorExpr:
		pkg, ok := fun.X.(*ast.Ident)
		if !ok || pkg.Obj != nil || decl.names[pkg.Name] {
			return false
		}
		want := pureName{importsOf(f)[pkg.Name], fun.Sel.Name}
		if !slices.Contains(pureInit(), want) {
			return false
		}
		if slices.Contains(constantArgs(), want) {
			return !slices.ContainsFunc(c.Args, func(a ast.Expr) bool { return !w.pureArg(a, f, dir, depth) })
		}
		return true
	}
	return false
}

// pureArg is whether an argument of Sprintf or Errorf runs no code of the
// tree's own: a constant, or a package-level var, of the package or of an import
// from this module, whose one initialiser is itself a pure call (an error made
// with errors.New, or wrapped by Errorf). The chain is read to a few levels and
// no further.
func (w *goFiles) pureArg(e ast.Expr, f *ast.File, dir string, depth int) bool {
	if isConstant(e, f, w.declared(dir)) {
		return true
	}
	if depth > 4 {
		return false
	}
	var at string
	var name string
	switch e := e.(type) {
	case *ast.Ident:
		// A name that resolves in the file's scope is the package-level var of the file only if its spec is one of
		// the file's top-level ones: a local of the same name is another value, with a method of its own.
		if e.Obj != nil {
			vs, ok := e.Obj.Decl.(*ast.ValueSpec)
			if e.Obj.Kind != ast.Var || !ok || !topLevel(f, vs) {
				return false
			}
		}
		at, name = dir, e.Name
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		if !ok || pkg.Obj != nil || w.declared(dir).names[pkg.Name] {
			return false
		}
		p, ok := importsOf(f)[pkg.Name]
		rest, in := strings.CutPrefix(p, modulePath+"/")
		if !ok || !in {
			return false
		}
		at, name = rest, e.Sel.Name
	default:
		return false
	}
	for _, g := range w.byDir[at] {
		for _, d := range g.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				if len(vs.Names) == 1 && vs.Names[0].Name == name && len(vs.Values) == 1 {
					call, ok := vs.Values[0].(*ast.CallExpr)
					return ok && isMaker(call, g) && w.pureCall(call, g, at, depth+1)
				}
			}
		}
	}
	return false
}

// isConstant is whether e is a constant: a literal, true or false, a const the
// package declares, or an expression of those.
func isConstant(e ast.Expr, f *ast.File, decl declaredIn) bool {
	switch e := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.ParenExpr:
		return isConstant(e.X, f, decl)
	case *ast.UnaryExpr:
		return isConstant(e.X, f, decl)
	case *ast.BinaryExpr:
		return isConstant(e.X, f, decl) && isConstant(e.Y, f, decl)
	case *ast.Ident:
		if e.Obj != nil {
			vs, ok := e.Obj.Decl.(*ast.ValueSpec)
			switch {
			case !ok || e.Obj.Kind != ast.Con:
				return false
			case topLevel(f, vs):
				// Judged by the package's declarations, which read a const group whole: a spec with no type and
				// no value has the type of the one before it, which its own spec does not say.
				return decl.consts[e.Name]
			}
			return plainType(vs.Type)
		}
		return decl.consts[e.Name] || ((e.Name == "true" || e.Name == "false") && !decl.names[e.Name])
	}
	return false
}

// initReads is each call of an init function, and each call a package-level
// var initialiser makes outside a function literal, in a package of files that
// holds a TestMain, that is not one of pureInit by what its name means. Go runs
// both before TestMain is called, so a read in either is outside m.Run and in no
// cache key (gragas #1273, the way into #1222's hole that earlyReads does not
// walk). A type conversion is no call.
func initReads(fset *token.FileSet, files map[string]*ast.File) []string {
	withMain := map[string]bool{}
	byDir := map[string][]*ast.File{}
	for rel, f := range files {
		byDir[path.Dir(rel)] = append(byDir[path.Dir(rel)], f)
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "TestMain" {
				withMain[path.Dir(rel)] = true
			}
		}
	}
	w := &goFiles{byDir: byDir, decls: map[string]declaredIn{}}
	var found []string
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		if !withMain[path.Dir(rel)] {
			continue
		}
		f, dir := files[rel], path.Dir(rel)
		scan := func(n ast.Node, what string) {
			ast.Inspect(n, func(x ast.Node) bool {
				switch x := x.(type) {
				case *ast.FuncLit:
					return false
				case *ast.CallExpr:
					if conversion(x.Fun) {
						return true
					}
					if !w.pureCall(x, f, dir, 0) {
						found = append(found, rel+":"+strconv.Itoa(fset.Position(x.Pos()).Line)+": "+what+" calls "+callName(x.Fun)+", which runs before TestMain and is not one of pureInit by what its name means here")
					}
				}
				return true
			})
		}
		for _, d := range files[rel].Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" && d.Body != nil {
					scan(d.Body, "init")
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, sp := range d.Specs {
					for _, v := range sp.(*ast.ValueSpec).Values {
						scan(v, "a package-level var")
					}
				}
			}
		}
	}
	return found
}

// runReads is each call the Run method of the type name, among files, makes
// before the statement that calls its m.Run, and a finding of its own when it
// calls no m.Run at all.
func runReads(fset *token.FileSet, files []*ast.File, name, at string) []string {
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Name.Name != "Run" || fn.Body == nil || recvName(fn.Recv.List[0].Type) != name {
				continue
			}
			var found []string
			for _, st := range fn.Body.List {
				ran := false
				ast.Inspect(st, func(x ast.Node) bool {
					if c, ok := x.(*ast.CallExpr); ok && strings.HasSuffix(callName(c.Fun), ".m.Run") {
						ran = true
					}
					return true
				})
				if ran {
					return found
				}
				ast.Inspect(st, func(x ast.Node) bool {
					if c, ok := x.(*ast.CallExpr); ok {
						found = append(found, at+": "+name+".Run calls "+callName(c.Fun)+" before m.Run (line "+strconv.Itoa(fset.Position(c.Pos()).Line)+")")
					}
					return true
				})
			}
			return append(found, at+": "+name+".Run never calls m.Run")
		}
	}
	return nil
}

// conversion is whether the function of a call is a type, so the call is a
// conversion: a type literal, or a parenthesised pointer type as in (*T)(nil).
func conversion(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.ArrayType, *ast.MapType, *ast.InterfaceType, *ast.ChanType, *ast.FuncType, *ast.StructType, *ast.StarExpr:
		return true
	case *ast.ParenExpr:
		return conversion(e.X)
	}
	return false
}

// callName is a call's function as written: a name, or a selector chain.
func callName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return callName(e.X) + "." + e.Sel.Name
	case *ast.ParenExpr:
		return callName(e.X)
	}
	return "an expression"
}

// recvName is the type of a method's receiver, its pointer dropped.
func recvName(e ast.Expr) string {
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// TestNoTestMainReadsBeforeMRun: no TestMain of the tree calls anything before
// m.Run but beforeRun, nor hands configtest.Main a value whose Run does.
// Risk: a package whose checks read the tree outside their cache key, so a
// push replays them green after a change to what they read (gragas#1222).
func TestNoTestMainReadsBeforeMRun(t *testing.T) {
	t.Parallel()
	s := snap()
	if s.err != nil {
		t.Fatal(s.err)
	}
	found, n := earlyReads(s.fset, s.asts)
	found = append(found, initReads(s.fset, s.asts)...)
	for _, f := range found {
		t.Error(f)
	}
	if n < 30 {
		t.Errorf("read %d TestMains, and the tree holds one in nearly every package", n)
	}
}

// TestAnEarlyReadInATestMainIsFound plants a read in a TestMain and in the Run
// of a value it hands configtest.Main, and a Run that never runs the tests;
// the controls are the plain TestMain and a Run that cleans up after m.Run.
func TestAnEarlyReadInATestMainIsFound(t *testing.T) {
	t.Parallel()
	head := "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n"
	for name, c := range map[string]struct {
		src  string
		want string
	}{
		"plain":            {"func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }\n", ""},
		"cleans up after":  {"type w struct{ m *testing.M }\n\nfunc (x *w) Run() int {\n\tcode := x.m.Run()\n\t_ = os.RemoveAll(\"d\")\n\treturn code\n}\n\nfunc TestMain(m *testing.M) { os.Exit(configtest.Main(&w{m})) }\n", ""},
		"reads in main":    {"func TestMain(m *testing.M) {\n\t_, _ = os.ReadFile(\"go.mod\")\n\tos.Exit(configtest.Main(m))\n}\n", "TestMain calls os.ReadFile before m.Run"},
		"reads in its Run": {"type loaded struct{ m *testing.M }\n\nfunc (l loaded) Run() int {\n\treadTree()\n\treturn l.m.Run()\n}\n\nfunc TestMain(m *testing.M) { os.Exit(configtest.Main(loaded{m})) }\n", "loaded.Run calls readTree before m.Run"},
		"never runs":       {"type idle struct{ m *testing.M }\n\nfunc (i idle) Run() int { return 0 }\n\nfunc TestMain(m *testing.M) { os.Exit(configtest.Main(idle{m})) }\n", "idle.Run never calls m.Run"},
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p/main_test.go", head+c.src, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		found, n := earlyReads(fset, map[string]*ast.File{"p/main_test.go": f})
		if n != 1 {
			t.Fatalf("%s: read %d TestMains, want 1", name, n)
		}
		switch {
		case c.want == "" && len(found) > 0:
			t.Errorf("%s: the control was refused: %v", name, found)
		case c.want != "" && (len(found) != 1 || !strings.Contains(found[0], c.want)):
			t.Errorf("%s: found %v, want one finding naming %q", name, found, c.want)
		}
	}
}

// TestAReadInInitOrAPackageVarIsFound plants an init function that reads a
// file and a package-level var whose initialiser does, each red; the controls
// are the pure constructors, a conversion, a read inside a function literal
// that runs later, and a package with no TestMain, where nothing is held.
func TestAReadInInitOrAPackageVarIsFound(t *testing.T) {
	t.Parallel()
	head := "package p\n\nimport (\n\t\"os\"\n\t\"regexp\"\n\t\"sync\"\n\t\"testing\"\n)\n\n"
	main := "func TestMain(m *testing.M) { os.Exit(m.Run()) }\n\n"
	for name, c := range map[string]struct {
		src  string
		want string
	}{
		"constructors":       {main + "var re = regexp.MustCompile(`x`)\n\nvar _ = (*testing.M)(nil)\n\nvar b = []byte(\"x\")\n", ""},
		"a read made later":  {main + "var read = sync.OnceValue(func() []byte { b, _ := os.ReadFile(\"go.mod\"); return b })\n", ""},
		"an init that reads": {main + "func init() {\n\t_, _ = os.ReadFile(\"go.mod\")\n}\n", "init calls os.ReadFile"},
		"a var that reads":   {main + "var tree = readTree()\n\nfunc readTree() []byte { return nil }\n", "a package-level var calls readTree"},
		"no TestMain":        {"func init() {\n\t_, _ = os.ReadFile(\"go.mod\")\n}\n", ""},
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p/x_test.go", head+c.src, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		found := initReads(fset, map[string]*ast.File{"p/x_test.go": f})
		switch {
		case c.want == "" && len(found) > 0:
			t.Errorf("%s: the control was refused: %v", name, found)
		case c.want != "" && (len(found) != 1 || !strings.Contains(found[0], c.want)):
			t.Errorf("%s: found %v, want one finding naming %q", name, found, c.want)
		}
	}
}

// TestAnInitCallIsPureByWhatItsNameMeansHere (gragas#1409): initReads takes a
// call of pureInit only when its name means the builtin or the standard
// function the list names. Planted, each found by its call: a package's own len
// that reads a file; a var named errors, whose New reads one; fmt.Sprintf of a
// Stringer that reads one; an import of another package under the name errors;
// and fmt.Errorf of an error from this module that is made by a call that reads.
// The controls: len and errors.New as the language means them, an alias of the
// real errors package, Sprintf and Errorf of constants, and Errorf wrapping an
// error from this module that errors.New made.
func TestAnInitCallIsPureByWhatItsNameMeansHere(t *testing.T) {
	t.Parallel()
	head := func(imports ...string) string {
		s := "package p\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n\t\"os\"\n\t\"testing\"\n"
		for _, im := range imports {
			s += "\t" + im + "\n"
		}
		return s + ")\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n\n"
	}
	port := "package port\n\nimport \"errors\"\n\nvar ErrKept = errors.New(\"kept\")\n\nvar ErrReads = readIt()\n\nfunc readIt() error { return nil }\n"
	for name, c := range map[string]struct {
		src, other string // the test file of package p, and the file of internal/port when a row has one
		want       string // what the one finding names; "" for a control
	}{
		"len as the language means it":    {head() + "var n = len(\"p\")\n\nvar s = make([]int, 3)\n", "", ""},
		"errors as the package means it":  {head() + "var e = errors.New(\"p\")\n", "", ""},
		"an alias of the real package":    {head("e \"regexp\"") + "var r = e.MustCompile(`x`)\n", "", ""},
		"Sprintf and Errorf of constants": {head() + "const c = \"x\"\n\nvar a = fmt.Sprintf(\"%s %d\", c, 3)\n\nvar b = fmt.Errorf(\"n: %d\", 3+4)\n", "", ""},
		"Errorf wrapping a pure error":    {head("\"github.com/birdtable/gragas/internal/port\"") + "var w = fmt.Errorf(\"w: %w\", port.ErrKept)\n", port, ""},
		"a package's own len":             {head() + "func len(s string) int { _, _ = os.ReadFile(s); return 0 }\n\nvar x = len(\"p\")\n", "", "a package-level var calls len"},
		"a var named errors":              {head() + "type e struct{}\n\nfunc (e) New(string) error { _, _ = os.ReadFile(\"p\"); return nil }\n\nvar errors e\n\nvar x = errors.New(\"p\")\n", "", "a package-level var calls errors.New"},
		"Sprintf of a Stringer":           {head() + "type s struct{}\n\nfunc (s) String() string { _, _ = os.ReadFile(\"p\"); return \"\" }\n\nvar x = fmt.Sprintf(\"%v\", s{})\n", "", "a package-level var calls fmt.Sprintf"},
		"another package named errors":    {strings.Replace(head("errors \"example.com/other/errs\""), "\t\"errors\"\n", "", 1) + "var x = errors.New(\"p\")\n", "", "a package-level var calls errors.New"},
		"Errorf of an error that reads":   {head("\"github.com/birdtable/gragas/internal/port\"") + "var w = fmt.Errorf(\"w: %w\", port.ErrReads)\n", port, "a package-level var calls fmt.Errorf"},
	} {
		fset := token.NewFileSet()
		files := map[string]*ast.File{}
		for rel, src := range map[string]string{"p/x_test.go": c.src, "internal/port/e.go": c.other} {
			if src == "" {
				continue
			}
			f, err := parser.ParseFile(fset, rel, src, 0)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			files[rel] = f
		}
		found := initReads(fset, files)
		switch {
		case c.want == "" && len(found) > 0:
			t.Errorf("%s: the control was refused: %v", name, found)
		case c.want != "" && (len(found) != 1 || !strings.Contains(found[0], c.want)):
			t.Errorf("%s: found %v, want one finding naming %q", name, found, c.want)
		}
	}
}

// TestAnInitCallIsNotPureWhenAScopeTheCallsFileDoesNotShowHidesItsName (gragas#1409, the self-review of PR 1417):
// each guard of pureCall and pureArg is held by a plant that no other guard can catch alone. A builtin or a
// package name declared in another file of the package (the check that the package declares nothing of the
// name); a local that shadows a builtin, or an import, in the init function itself (the check that the name
// resolves to nothing in the file's scope); a local that shadows a pure package var as an argument; an
// argument var that is a new or a make of a type with a String method that reads; and a typed const of such a
// type. The controls are a typed const of a builtin type, an iota group of one, and an untyped const.
func TestAnInitCallIsNotPureWhenAScopeTheCallsFileDoesNotShowHidesItsName(t *testing.T) {
	t.Parallel()
	head := "package p\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n\n"
	reads := "type T struct{}\n\nfunc (T) String() string { _, _ = os.ReadFile(\"p\"); return \"\" }\n\n"
	for name, c := range map[string]struct {
		files map[string]string // by path; p/x_test.go is the file with the TestMain
		want  string            // what the one finding names; "" for a control
	}{
		"len declared in another file": {map[string]string{
			"p/x_test.go": head + "var n = len(\"p\")\n",
			"p/b.go":      "package p\n\nimport \"os\"\n\nfunc len(s string) int { _, _ = os.ReadFile(s); return 0 }\n"}, "a package-level var calls len"},
		"errors declared in another file": {map[string]string{
			"p/x_test.go": head + "var x = errors.New(\"p\")\n",
			"p/b.go":      "package p\n\nimport \"os\"\n\ntype e struct{}\n\nfunc (e) New(string) error { _, _ = os.ReadFile(\"p\"); return nil }\n\nvar errors e\n"}, "a package-level var calls errors.New"},
		"a local len in the init": {map[string]string{
			"p/x_test.go": head + "func init() {\n\tlen := func(s string) int { _, _ = os.ReadFile(s); return 0 }\n\t_ = len(\"p\")\n}\n"}, "init calls len"},
		"a local errors in the init": {map[string]string{
			"p/x_test.go": head + "type e struct{}\n\nfunc (e) New(string) error { _, _ = os.ReadFile(\"p\"); return nil }\n\nfunc init() {\n\terrors := e{}\n\t_ = errors.New(\"p\")\n}\n"}, "init calls errors.New"},
		"a local shadowing a pure var": {map[string]string{
			"p/x_test.go": head + reads + "var e = errors.New(\"x\")\n\nfunc init() {\n\te := T{}\n\t_ = fmt.Sprintf(\"%v\", e)\n}\n"}, "init calls fmt.Sprintf"},
		"an argument var made by new": {map[string]string{
			"p/x_test.go": head + reads + "var p = new(T)\n\nvar x = fmt.Sprintf(\"%v\", p)\n"}, "a package-level var calls fmt.Sprintf"},
		"an argument var made by make": {map[string]string{
			"p/x_test.go": head + reads + "var s = make([]T, 1)\n\nvar x = fmt.Sprintf(\"%v\", s)\n"}, "a package-level var calls fmt.Sprintf"},
		"a typed const of a Stringer": {map[string]string{
			"p/x_test.go": head + "type K int\n\nfunc (K) String() string { _, _ = os.ReadFile(\"p\"); return \"\" }\n\nconst c K = 1\n\nvar x = fmt.Sprintf(\"%v\", c)\n"}, "a package-level var calls fmt.Sprintf"},
		"an iota group of a Stringer": {map[string]string{
			"p/x_test.go": head + "type K int\n\nfunc (K) String() string { _, _ = os.ReadFile(\"p\"); return \"\" }\n\nconst (\n\ta K = iota\n\tb\n)\n\nvar x = fmt.Sprintf(\"%v\", b)\n"}, "a package-level var calls fmt.Sprintf"},
		"a typed const of a builtin type": {map[string]string{
			"p/x_test.go": head + "const c int = 3\n\nvar x = fmt.Sprintf(\"%d\", c)\n"}, ""},
		"an iota group of a builtin type": {map[string]string{
			"p/x_test.go": head + "const (\n\ta int = iota\n\tb\n)\n\nvar x = fmt.Sprintf(\"%d\", b)\n"}, ""},
		"an argument var made by errors.New": {map[string]string{
			"p/x_test.go": head + "var e = errors.New(\"x\")\n\nvar w = fmt.Errorf(\"w: %w\", e)\n"}, ""},
	} {
		fset := token.NewFileSet()
		files := map[string]*ast.File{}
		for rel, src := range c.files {
			f, err := parser.ParseFile(fset, rel, src, 0)
			if err != nil {
				t.Fatalf("%s: %s: %v", name, rel, err)
			}
			files[rel] = f
		}
		found := initReads(fset, files)
		switch {
		case c.want == "" && len(found) > 0:
			t.Errorf("%s: the control was refused: %v", name, found)
		case c.want != "" && (len(found) != 1 || !strings.Contains(found[0], c.want)):
			t.Errorf("%s: found %v, want one finding naming %q", name, found, c.want)
		}
	}
}

// TestAReadBeforeMRunIsInNoCacheKey is the hazard the check above guards, run
// in a module of the test's own: early reads a file in its TestMain, before
// m.Run, and late reads it once from a test's body. Both replay from the cache
// while the file is as it was (the control); after a change to it, early
// still replays and late runs again.
func TestAReadBeforeMRunIsInNoCacheKey(t *testing.T) {
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
	// data is dated in the past: Go does not cache a run that read a file
	// changed in its last seconds, which would make the control fail and the
	// plant pass. The date is fixed, not read from the clock (arch.ports.1).
	data := func(body string, at time.Time) {
		t.Helper()
		write("data.txt", body)
		if err := os.Chtimes(filepath.Join(mod, "data.txt"), at, at); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/plant\n\ngo 1.27\n")
	write("early/early_test.go", "package early\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nvar data []byte\n\n"+
		"func TestMain(m *testing.M) {\n\tdata, _ = os.ReadFile(\"../data.txt\")\n\tos.Exit(m.Run())\n}\n\n"+
		"func TestData(t *testing.T) {\n\tif len(data) == 0 {\n\t\tt.Fatal(\"no data\")\n\t}\n}\n")
	write("late/late_test.go", "package late\n\nimport (\n\t\"os\"\n\t\"sync\"\n\t\"testing\"\n)\n\n"+
		"var read = sync.OnceValue(func() []byte { b, _ := os.ReadFile(\"../data.txt\"); return b })\n\n"+
		"func TestData(t *testing.T) {\n\tif len(read()) == 0 {\n\t\tt.Fatal(\"no data\")\n\t}\n}\n")
	data("one\n", time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC))
	run := func() map[string]bool {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "go", "test", "./early", "./late")
		cmd.Dir = mod
		cmd.Env = append(configtest.Inherited(), "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go test: %v\n%s", err, out)
		}
		cached := map[string]bool{}
		for line := range strings.SplitSeq(string(out), "\n") {
			for _, p := range []string{"early", "late"} {
				if strings.Contains(line, "example.com/plant/"+p+"\t") || strings.HasSuffix(line, "example.com/plant/"+p) {
					cached[p] = strings.Contains(line, "(cached)")
				}
			}
		}
		if len(cached) != 2 {
			t.Fatalf("go test named not both packages:\n%s", out)
		}
		return cached
	}
	run()
	if got := run(); !got["early"] || !got["late"] {
		t.Fatalf("control: a second run with nothing changed was not from the cache: %v", got)
	}
	data("two, longer\n", time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC))
	got := run()
	if !got["early"] {
		t.Error("early ran again after the change: Go now keys a read before m.Run, and the check above can be relaxed")
	}
	if got["late"] {
		t.Error("late replayed from the cache after a change to the file its test read")
	}
}

// TestAWalkReplaysFromTheCacheUntilAProbeMovesAFolderItLists is why the
// explorer's licence listing names pnpm's store (gragas#1512), in a module of
// the test's own: walk's test walks the module from its root, as the tests
// that read the whole checkout do, and ws stands for explorer/. A second run
// with nothing changed replays from the cache (the control). A file created and
// removed in ws, as pnpm's probe for a store does, leaves every file as it was
// and still makes the next run fresh: Go keys a listed folder on each entry's
// time, and the root's listing holds ws's.
func TestAWalkReplaysFromTheCacheUntilAProbeMovesAFolderItLists(t *testing.T) {
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
		// Dated in the past: Go does not cache a run that read a file changed in
		// its last seconds. The date is fixed, not read from the clock (arch.ports.1).
		at := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/plant\n\ngo 1.27\n")
	write("ws/package.json", "{}\n")
	write("walk/walk_test.go", "package walk\n\nimport (\n\t\"io/fs\"\n\t\"path/filepath\"\n\t\"testing\"\n)\n\n"+
		"func TestWalk(t *testing.T) {\n\tn := 0\n\terr := filepath.WalkDir(\"..\", func(_ string, _ fs.DirEntry, err error) error { n++; return err })\n"+
		"\tif err != nil || n == 0 {\n\t\tt.Fatal(n, err)\n\t}\n}\n")
	cached := func() bool {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "go", "test", "./walk")
		cmd.Dir = mod
		cmd.Env = append(configtest.Inherited(), "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go test: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "example.com/plant/walk") {
			t.Fatalf("go test named not the package:\n%s", out)
		}
		return strings.Contains(string(out), "(cached)")
	}
	cached()
	if !cached() {
		t.Fatal("control: a second run of the walk with nothing changed was not from the cache")
	}
	probe := filepath.Join(mod, "ws", "_tmp_probe")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	if cached() {
		t.Error("the walk replayed from the cache after a probe moved a folder it lists: Go no longer keys a listing on its entries' times, and the explorer's store setting (scripts/pnpm-env.ts) may be needless")
	}
}

// topLevel is whether vs is one of the top-level var or const specs of f.
func topLevel(f *ast.File, vs *ast.ValueSpec) bool {
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && slices.ContainsFunc(gd.Specs, func(s ast.Spec) bool { return s == ast.Spec(vs) }) {
			return true
		}
	}
	return false
}

// isMaker is whether a call of f makes an error or a string with no method of its own: errors.New, fmt.Errorf
// or fmt.Sprintf, by the file's import of the package. A value an argument var holds is taken to run no code
// of the tree's only when it is one of these (a new or a make of a type of the tree is not: its String can
// read), so pureArg reads a var's initialiser as a maker and not as any pure call (gragas#1409).
func isMaker(c *ast.CallExpr, f *ast.File) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	switch (pureName{importsOf(f)[pkg.Name], sel.Sel.Name}) {
	case pureName{"errors", "New"}, pureName{"fmt", "Errorf"}, pureName{"fmt", "Sprintf"}:
		return true
	}
	return false
}

// TestOnlyTheJourneysKeepTheCrashSettings: configtest.MainCrash keeps
// CRASH_SEED and CRASH_POINTS through a package's isolation, so a TestMain
// that calls it anywhere but internal/journeys is a finding
// (tests.isolation.1); the control is the journeys' own TestMain.
func TestOnlyTheJourneysKeepTheCrashSettings(t *testing.T) {
	t.Parallel()
	src := "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(configtest.MainCrash(m)) }\n"
	for _, c := range []struct {
		rel  string
		want string
	}{
		{"internal/dev/join/main_test.go", "TestMain calls configtest.MainCrash, which only internal/journeys/main_test.go may"},
		{crashMainFile, ""},
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, c.rel, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		found, _ := earlyReads(fset, map[string]*ast.File{c.rel: f})
		switch {
		case c.want == "" && len(found) > 0:
			t.Errorf("%s: the control was refused: %v", c.rel, found)
		case c.want != "" && (len(found) != 1 || !strings.Contains(found[0], c.want)):
			t.Errorf("%s: found %v, want one finding naming %q", c.rel, found, c.want)
		}
	}
}

// configtestPath is the import path of the test isolation's package.
const configtestPath = "github.com/birdtable/gragas/internal/config/testing"

// crashMainRefs is every reference to configtest.MainCrash in files outside
// crashMainFile and configtest itself: through the import by its own name,
// an alias or a dot import, as a call or as a value. A wrapper a TestMain
// calls is such a reference too, so the crash settings reach no package but
// the journeys.
func crashMainRefs(fset *token.FileSet, files map[string]*ast.File) []string {
	var found []string
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		if rel == crashMainFile || path.Dir(rel) == crashMainOwner {
			continue
		}
		f := files[rel]
		names, dot := map[string]bool{}, false
		for _, imp := range f.Imports {
			if p, err := strconv.Unquote(imp.Path.Value); err != nil || p != configtestPath {
				continue
			}
			switch {
			case imp.Name == nil:
				names["configtest"] = true
			case imp.Name.Name == ".":
				dot = true
			case imp.Name.Name != "_":
				names[imp.Name.Name] = true
			}
		}
		if len(names) == 0 && !dot {
			continue
		}
		at := func(p token.Pos) string { return rel + ":" + strconv.Itoa(fset.Position(p).Line) }
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && names[id.Name] && x.Sel.Name == "MainCrash" {
					found = append(found, at(x.Pos())+": refers to configtest.MainCrash, which only "+crashMainFile+" may: it keeps the crash settings")
				}
			case *ast.Ident:
				if dot && x.Name == "MainCrash" {
					found = append(found, at(x.Pos())+": refers to configtest.MainCrash through a dot import, which only "+crashMainFile+" may")
				}
			}
			return true
		})
	}
	return found
}

// TestOnlyTheJourneysReferToMainCrash holds the tree to crashMainRefs, with
// plants beside the journeys' control: a wrapper a TestMain calls, an aliased
// import taken as a function value, and a dot import.
func TestOnlyTheJourneysReferToMainCrash(t *testing.T) {
	t.Parallel()
	s := snap()
	if s.err != nil {
		t.Fatal(s.err)
	}
	if _, ok := s.asts[crashMainFile]; !ok {
		t.Fatalf("the tree read holds no %s, so the test files were not read and the check sees nothing", crashMainFile)
	}
	for _, f := range crashMainRefs(s.fset, s.asts) {
		t.Error(f)
	}
	for _, c := range []struct {
		claim, rel, src string
		want            bool
	}{
		{"a wrapper a TestMain calls", "internal/dev/join/main_test.go", "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\tconfigtest \"" + configtestPath + "\"\n)\n\nfunc Main(m *testing.M) int { return configtest.MainCrash(m) }\n\nfunc TestMain(m *testing.M) { os.Exit(Main(m)) }\n", true},
		{"an alias taken as a value", "internal/dev/join/main_test.go", "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\tct \"" + configtestPath + "\"\n)\n\nvar keep = ct.MainCrash\n\nfunc TestMain(m *testing.M) { os.Exit(keep(m)) }\n", true},
		{"a dot import", "internal/dev/join/main_test.go", "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t. \"" + configtestPath + "\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(MainCrash(m)) }\n", true},
		{"a folder below configtest's own", "internal/config/testing/sub/main_test.go", "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\tconfigtest \"" + configtestPath + "\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(configtest.MainCrash(m)) }\n", true},
		{"configtest's own folder", "internal/config/testing/isolate_test.go", "package configtest_test\n\nimport configtest \"" + configtestPath + "\"\n\nvar _ = configtest.MainCrash\n", false},
		{"the journeys' own", crashMainFile, "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\tconfigtest \"" + configtestPath + "\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(configtest.MainCrash(m)) }\n", false},
	} {
		t.Run(c.claim, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, c.rel, c.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := crashMainRefs(fset, map[string]*ast.File{c.rel: f}); (len(got) > 0) != c.want {
				t.Errorf("found %v, want a finding %t", got, c.want)
			}
		})
	}
}
