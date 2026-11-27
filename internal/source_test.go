package internal

// The checks that read the source itself, file by file: the port package's
// shape, the platform words, seams beside their users, nil tests on ports,
// tagged result types in the command tree, stand-in types in shipped code,
// goroutines, and the names of table cases.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/ordered"
)

// source is one parsed Go file of the tree.
type source struct {
	path string // slash path from the root
	pkg  string // its package's path
	test bool
	file *ast.File
	fset *token.FileSet
}

// sources parses every Go file of w.
func sources(t *testing.T, w world) []source {
	t.Helper()
	fset := token.NewFileSet()
	var out []source
	s := snap()
	for _, p := range w.sorted() {
		for _, name := range append(slices.Clone(p.files), p.tests...) {
			rel := path.Join(p.path, name)
			f, set := s.asts[rel], s.fset
			if f == nil {
				var err error
				if f, err = parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, parser.ParseComments); err != nil {
					t.Fatal(err)
				}
				set = fset
			}
			out = append(out, source{path: rel, pkg: p.path, test: strings.HasSuffix(name, "_test.go"), file: f, fset: set})
		}
	}
	return out
}

// parse is src, one file of package pkg at path, as a source.
func parse(t *testing.T, pkg, file, src string) source {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return source{path: path.Join(pkg, file), pkg: pkg, test: strings.HasSuffix(file, "_test.go"), file: f, fset: fset}
}

// imports is the import path each name of f refers to.
func imports(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if im.Name != nil {
			name = im.Name.Name
		}
		out[name] = p
	}
	return out
}

// literals is every string literal of f.
func literals(f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if b, ok := n.(*ast.BasicLit); ok && b.Kind == token.STRING {
			if s, err := strconv.Unquote(b.Value); err == nil {
				out = append(out, s)
			}
		}
		return true
	})
	return out
}

// wordOf is a pattern matching w as a word of its own, any case.
func wordOf(w string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^a-z0-9_-])` + regexp.QuoteMeta(w) + `($|[^a-z0-9_-])`)
}

// portShape is arch.ports.10, .15 and .5 over the port package: no function
// and no method, its interfaces exactly the ports and what they carry, a
// field of Ports for each port with its required tag, and no platform field.
func portShape(tb *layers.Table, port []source) []string {
	var out []string
	ifaces := map[string]bool{}
	var fields []string
	for _, s := range port {
		if s.test {
			continue
		}
		for _, d := range s.file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				out = append(out, s.path+": "+fn.Name.Name+" is a function in the port package, which holds no logic")
			}
		}
		ast.Inspect(s.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.TypeSpec:
				if _, ok := x.Type.(*ast.InterfaceType); ok && x.Name.IsExported() {
					ifaces[x.Name.Name] = true
				}
				if st, ok := x.Type.(*ast.StructType); ok && x.Name.Name == "Ports" {
					for _, f := range st.Fields.List {
						for _, n := range f.Names {
							fields = append(fields, n.Name)
							if f.Tag == nil || !strings.Contains(f.Tag.Value, `port:"required"`) && !strings.Contains(f.Tag.Value, `port:"optional"`) {
								out = append(out, "Ports."+n.Name+" does not say whether a platform must fill it")
							}
						}
					}
				}
			case *ast.Field:
				for _, n := range x.Names {
					if slices.Contains([]string{"Platform", "GOOS", "OS"}, n.Name) {
						out = append(out, s.path+": "+n.Name+" is a platform in a port (arch.ports.5)")
					}
				}
			}
			return true
		})
	}
	var want []string
	for _, p := range tb.Ports.List {
		want = append(want, p.Name)
		if !slices.Contains(fields, p.Name) {
			out = append(out, layers.File+" names the port "+p.Name+", which Ports has no field for")
		}
		delete(ifaces, p.Name)
	}
	for _, f := range fields {
		if !slices.Contains(want, f) {
			out = append(out, "Ports."+f+" is a port "+layers.File+" does not name (arch.ports.15)")
		}
	}
	for _, c := range tb.Ports.Carries {
		delete(ifaces, c)
	}
	for name := range ifaces {
		out = append(out, "port."+name+" is an interface "+layers.File+" names neither as a port nor as carried")
	}
	slices.Sort(out)
	return out
}

// platformWords is arch.ports.4 and .17: no non-test file of a pure package
// starts a string with a system root or names an OS tool, and no file of
// the port package or its fake names a system.
func platformWords(tb *layers.Table, all []source) []string {
	var out []string
	tools, names := map[string]*regexp.Regexp{}, map[string]*regexp.Regexp{}
	for _, w := range tb.Platform.Tools {
		tools[w] = wordOf(w)
	}
	for _, w := range tb.Platform.Names {
		names[w] = wordOf(w)
	}
	for _, s := range all {
		r := tb.RowOf(s.pkg)
		if s.test || r == nil {
			continue
		}
		portish := s.pkg == "internal/port" || s.pkg == tb.Ports.Fake
		for _, lit := range literals(s.file) {
			if r.Kind == layers.KindPure {
				if atRoot(lit, tb.Platform.Roots) {
					out = append(out, s.path+": "+strconv.Quote(lit)+" starts at a system root")
				}
				for _, home := range tb.Platform.Homes {
					if strings.Contains(strings.ToLower(filepath.ToSlash(lit)), strings.ToLower(home)) {
						out = append(out, s.path+": "+strconv.Quote(lit)+" names the system folder "+home)
					}
				}
				for _, tool := range tb.Platform.Tools {
					if tools[tool].MatchString(lit) {
						out = append(out, s.path+": "+strconv.Quote(lit)+" names the OS tool "+tool)
					}
				}
			}
			for _, name := range tb.Platform.Names {
				if portish && names[name].MatchString(lit) {
					out = append(out, s.path+": "+strconv.Quote(lit)+" names the system "+name)
				}
			}
		}
	}
	return out
}

// seams is arch.ports.10's layout check: an interface outside the port
// package whose methods carry a port's type or an effect package's is a port
// declared beside its user. An interface a domain declares over another
// domain's values (arch.ports.8) carries neither.
func seams(tb *layers.Table, module string, all []source) []string {
	var out []string
	for _, s := range all {
		if s.test || s.pkg == "internal/port" {
			continue
		}
		names := imports(s.file)
		ast.Inspect(s.file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			it, ok := spec.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			ast.Inspect(it, func(m ast.Node) bool {
				sel, ok := m.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); ok {
					if imp := names[x.Name]; imp == module+"/internal/port" || tb.Effect(imp) {
						out = append(out, s.path+": "+spec.Name.Name+" is an interface over "+imp+"."+sel.Sel.Name+"; a port lives in internal/port")
					}
				}
				return true
			})
			return true
		})
	}
	return out
}

// taggedResults is cli.output.2: no struct with json tags in a cli package;
// result types live in internal/core/api.
func taggedResults(all []source) []string {
	var out []string
	for _, s := range all {
		if s.test || !strings.HasPrefix(s.pkg, "internal/cli/") {
			continue
		}
		ast.Inspect(s.file, func(n ast.Node) bool {
			if f, ok := n.(*ast.Field); ok && f.Tag != nil && strings.Contains(f.Tag.Value, `json:"`) {
				out = append(out, s.path+": a json-tagged field in the command tree; result types live in internal/core/api")
			}
			return true
		})
	}
	return out
}

// goroutines is go.concurrency.6 and .7: a go statement carries the
// measured gain on the line above it, its package's tests hold a leak check,
// and an adapter starts no goroutine, ticker or timer.
func goroutines(tb *layers.Table, all []source) []string {
	leakChecked := map[string]bool{}
	for _, s := range all {
		if s.test && leakCheck(s) {
			leakChecked[s.pkg] = true
		}
	}
	var out []string
	for _, s := range all {
		if s.test {
			continue
		}
		host := false
		if r := tb.RowOf(s.pkg); r != nil && r.Kind == layers.KindHost {
			host = true
		}
		names := imports(s.file)
		ast.Inspect(s.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.GoStmt:
				out = append(out, started(s, x, "a go statement", host, leakChecked[s.pkg])...)
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Go" && len(x.Args) == 1 {
					if _, lit := x.Args[0].(*ast.FuncLit); lit {
						out = append(out, started(s, x, "a Go call (WaitGroup.Go or errgroup's)", host, leakChecked[s.pkg])...)
					}
				}
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && host && names[id.Name] == "time" && slices.Contains([]string{"NewTicker", "NewTimer", "AfterFunc", "Tick"}, x.Sel.Name) {
					out = append(out, s.path+": time."+x.Sel.Name+" in an adapter")
				}
			}
			return true
		})
	}
	return out
}

// started is what go.concurrency.6 and .7 say of a goroutine started at n, by a go
// statement or by a Go call: no adapter starts one, it carries its measured line,
// and its package's tests hold a leak check. what names the start in a finding; the
// go statement keeps its words, which the guards read.
func started(s source, n ast.Node, what string, host, leakChecked bool) []string {
	var out []string
	if host {
		out = append(out, s.path+": a goroutine in an adapter")
	}
	if !commentAbove(s, n) {
		out = append(out, s.path+": "+what+" with no '// measured: what, gain, where' line above")
	}
	if !leakChecked {
		out = append(out, s.path+": "+what+" in a package whose tests have no leak check")
	}
	return out
}

// leakCheck is whether a test file holds a leak check: it imports goleak or
// a goroutineleak package, or reads the runtime's own goroutine-leak profile
// by runtime/pprof's Lookup("goroutineleak"), which needs no library.
func leakCheck(s source) bool {
	for _, lit := range []string{"goleak", "goroutineleak"} {
		if slices.ContainsFunc(s.file.Imports, func(im *ast.ImportSpec) bool { return strings.Contains(im.Path.Value, lit) }) {
			return true
		}
	}
	names := imports(s.file)
	found := false
	ast.Inspect(s.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return !found
		}
		pkg, ok := sel.X.(*ast.Ident)
		if ok && sel.Sel.Name == "NoLeaks" && names[pkg.Name] == "github.com/birdtable/gragas/internal/config/testing" {
			found = true
			return false
		}
		if len(call.Args) != 1 || sel.Sel.Name != "Lookup" {
			return !found
		}
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if ok && isLit && names[pkg.Name] == "runtime/pprof" && lit.Value == `"goroutineleak"` {
			found = true
		}
		return !found
	})
	return found
}

// measuredForm is the comment go.concurrency.7 asks for on the line above
// a go statement: what was measured, how much it gained (a number), and where
// the measurement is recorded.
var measuredForm = regexp.MustCompile(`^// measured: [^,]+, [^,]*[0-9][^,]*, \S.*$`)

// commentAbove is whether the line above n is a comment in the measured form.
func commentAbove(s source, n ast.Node) bool {
	line := s.fset.Position(n.Pos()).Line
	for _, cg := range s.file.Comments {
		last := cg.List[len(cg.List)-1]
		if s.fset.Position(last.End()).Line == line-1 && measuredForm.MatchString(last.Text) {
			return true
		}
	}
	return false
}

// badCase is a table case name that says nothing: empty, a number, or
// "case 3".
var badCase = regexp.MustCompile(`(?i)^\s*((case|test|tc)[ _-]?)?#?\s*\d*\s*$`)

// numbered is whether e makes a number into text: strconv's Itoa and
// Format, fmt's Sprint, or a Sprintf whose format says nothing but the
// number.
func numbered(s source, e ast.Expr) bool {
	c, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	switch imports(s.file)[x.Name] + "." + sel.Sel.Name {
	case "strconv.Itoa", "strconv.FormatInt", "strconv.FormatUint", "fmt.Sprint", "fmt.Sprintln":
		return true
	case "fmt.Sprintf":
		if len(c.Args) > 0 {
			if b, ok := c.Args[0].(*ast.BasicLit); ok {
				v, err := strconv.Unquote(b.Value)
				return err == nil && badCase.MatchString(strings.ReplaceAll(v, "%d", "1"))
			}
		}
	}
	return false
}

// caseNames is go.style.5's half that a tool decides: a table case is named
// by its claim, never empty or by number ("", "3", "case 3", "#1"), as a
// keyed field, the first value of a row, the key of a map-keyed table or the
// name t.Run is given; and t.Run is never named by a number made into text.
func caseNames(all []source) []string {
	var out []string
	bad := func(s source, e ast.Expr) {
		if b, ok := e.(*ast.BasicLit); ok && b.Kind == token.STRING {
			if v, err := strconv.Unquote(b.Value); err == nil && badCase.MatchString(v) {
				out = append(out, s.path+": the table case "+strconv.Quote(v)+" is not named by its claim")
			}
		}
	}
	for _, s := range all {
		if !s.test {
			continue
		}
		ast.Inspect(s.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.KeyValueExpr:
				if k, ok := x.Key.(*ast.Ident); ok && (k.Name == "name" || k.Name == "Name") {
					bad(s, x.Value)
				}
			case *ast.CompositeLit:
				if _, keyed := x.Type.(*ast.MapType); keyed {
					for _, e := range x.Elts {
						if kv, ok := e.(*ast.KeyValueExpr); ok {
							bad(s, kv.Key)
						}
					}
				}
				if _, table := x.Type.(*ast.ArrayType); table {
					for _, e := range x.Elts {
						if c, ok := e.(*ast.CompositeLit); ok && len(c.Elts) > 1 {
							bad(s, c.Elts[0])
						}
					}
				}
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" && len(x.Args) == 2 {
					bad(s, x.Args[0])
					if numbered(s, x.Args[0]) {
						out = append(out, s.path+": t.Run is named by a number made into text, not by its claim")
					}
				}
			}
			return true
		})
	}
	return out
}

// sourceChecks is every source check over the tree, by name.
func sourceChecks(t *testing.T, tb *layers.Table, w world) map[string][]string {
	t.Helper()
	all := sources(t, w)
	var port []source
	for _, s := range all {
		if s.pkg == "internal/port" {
			port = append(port, s)
		}
	}
	return map[string][]string{
		"the port package":   portShape(tb, port),
		"platform words":     platformWords(tb, all),
		"seams":              seams(tb, w.module, all),
		"nil ports":          nilPorts(tb, all),
		"tagged results":     taggedResults(all),
		"stand-in types":     standInTypes(tb, all),
		"goroutines":         goroutines(tb, all),
		"clock reads":        clockReads(tb, all),
		"table case names":   caseNames(all),
		"pure calls":         pureCalls(tb, all),
		"domain types":       domainTypes(tb, w.module, all),
		"rule ids and tools": ruleAndToolWords(t, tb, all),
		"init functions":     initFuncs(tb, all),
		"blocked children":   blockedChildren(all),
		"drains named":       drainNamed(all),
		"start sites":        startSites(all),
		"private keys":       keyReads(all),
	}
}

func TestALogicalPortPackageFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/port", "port.go", `package port

type Tree interface{ Read() }
type Pigeon interface{ Fly() }

type Ports struct {
	Tree Tree
	Platform string `+"`port:\"required\"`"+`
}

func Helper() int { return 1 }
`)
	f := portShape(tb, []source{s})
	fires(t, f, "Helper is a function in the port package")
	fires(t, f, "port.Pigeon is an interface layers.yaml names neither")
	fires(t, f, "Ports.Tree does not say whether a platform must fill it")
	fires(t, f, "Platform is a platform in a port")
	fires(t, f, "names the port Writer, which Ports has no field for")
}

// procFile is a system root path the plants name, joined at run time so the
// check of operating-system code (oscode_test.go), which folds a constant
// "a" + "b", finds none in this file.
func procFile() string { return strings.Join([]string{"/pro", "c/meminfo"}, "") }

func TestPlatformWordsInTheCoreFail(t *testing.T) {
	t.Parallel()
	tb := table(t)
	procFile := procFile()
	core := parse(t, "internal/core/rulebook", "book.go", "package rulebook\n\nconst a, b = \""+procFile+"\", \"run systemctl restart\"\n")
	fake := parse(t, "internal/port/testing", "fake.go", "package porttest\n\nconst c = \"as on linux\"\n")
	host := parse(t, "internal/host/fs", "repo.go", "package fs\n\nconst d = \""+procFile+"\"\n")
	f := platformWords(tb, []source{core, fake, host})
	fires(t, f, `"`+procFile+`" starts at a system root`)
	fires(t, f, "names the OS tool systemctl")
	fires(t, f, "names the system linux")
	if len(f) != 3 {
		t.Errorf("findings = %q; the adapter's path is its own", f)
	}
}

func TestAPortBesideItsUserFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	module := tb.Module
	s := parse(t, "internal/cli/root", "seam.go", "package root\n\nimport (\n\t\"os\"\n\n\t\""+module+"/internal/port\"\n)\n\ntype opener interface{ Open() (*os.File, error) }\ntype writes interface{ W() port.Writer }\ntype fine interface{ Name() string }\n")
	f := seams(tb, module, []source{s})
	fires(t, f, "opener is an interface over os.File")
	fires(t, f, "writes is an interface over "+module+"/internal/port.Writer")
	if len(f) != 2 {
		t.Errorf("findings = %q", f)
	}
}

func TestANilTestOnAPortFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/core/api", "api.go", "package coreapi\n\nfunc f(p struct{ Lock any }) bool { return p.Lock == nil }\n")
	fires(t, nilPorts(tb, []source{s}), "a nil test on the port Lock")
	main := parse(t, "cmd/gragas", "main.go", "package main\n\nfunc f(p struct{ Locks any }) bool { return nil != p.Lock }\n")
	if f := nilPorts(tb, []source{main}); len(f) > 0 {
		t.Errorf("main's own test was refused: %q", f)
	}
}

func TestATaggedResultInTheCommandTreeFails(t *testing.T) {
	t.Parallel()
	s := parse(t, "internal/cli/root", "answer.go", "package root\n\ntype R struct {\n\tA int `json:\"a\"`\n}\n")
	fires(t, taggedResults([]source{s}), "a json-tagged field in the command tree")
}

func TestAStandInInShippedCodeFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/host/fs", "repo.go", "package fs\n\ntype FakeRepo struct{}\n")
	fires(t, standInTypes(tb, []source{s}), "FakeRepo is a stand-in in shipped code")
	fake := parse(t, "internal/port/testing", "fake.go", "package porttest\n\ntype Fake struct{}\n")
	if f := standInTypes(tb, []source{fake}); len(f) > 0 {
		t.Errorf("the stand-in package was refused: %q", f)
	}
}

func TestAnUnmeasuredGoroutineFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/host/fs", "watch.go", "package fs\n\nimport \"time\"\n\nfunc w() {\n\tgo func() {}()\n\t_ = time.NewTicker(1)\n}\n")
	f := goroutines(tb, []source{s})
	fires(t, f, "a goroutine in an adapter")
	fires(t, f, "a go statement with no '// measured: what, gain, where' line above")
	fires(t, f, "a go statement in a package whose tests have no leak check")
	fires(t, f, "time.NewTicker in an adapter")
	vague := parse(t, "internal/core/rulebook", "vague.go", "package rulebook\n\nfunc v() {\n\t// faster this way\n\tgo func() {}()\n}\n")
	fires(t, goroutines(tb, []source{vague}), "a go statement with no '// measured: what, gain, where' line above")
	core := parse(t, "internal/core/rulebook", "par.go", "package rulebook\n\nfunc p() {\n\t// measured: reading 1000 files, 4x faster, BenchmarkLoad in book_test.go\n\tgo func() {}()\n}\n")
	leak := parse(t, "internal/core/rulebook", "main_test.go", "package rulebook\n\nimport _ \"go.uber.org/goleak\"\n")
	if f := goroutines(tb, []source{core, leak}); len(f) > 0 {
		t.Errorf("a measured, leak-checked goroutine in the core was refused: %q", f)
	}
	profile := parse(t, "internal/core/rulebook", "main_test.go", "package rulebook\n\nimport \"runtime/pprof\"\n\nvar _ = pprof.Lookup(\"goroutineleak\")\n")
	if f := goroutines(tb, []source{core, profile}); len(f) > 0 {
		t.Errorf("a goroutine whose tests read the goroutine-leak profile was refused: %q", f)
	}
	// Planted: another profile, and a Lookup that is not pprof's, are no leak check.
	for _, body := range []string{
		"package rulebook\n\nimport \"runtime/pprof\"\n\nvar _ = pprof.Lookup(\"goroutine\")\n",
		"package rulebook\n\nimport pprof \"example.com/notpprof\"\n\nvar _ = pprof.Lookup(\"goroutineleak\")\n",
	} {
		fires(t, goroutines(tb, []source{core, parse(t, "internal/core/rulebook", "main_test.go", body)}), "a go statement in a package whose tests have no leak check")
	}
}

func TestATableCaseNamedByNumberFails(t *testing.T) {
	t.Parallel()
	s := parse(t, "internal/config", "config_test.go", `package config

func cases() {
	_ = []struct{ name string }{{name: "case 1"}, {name: "refuses a retired key"}}
	_ = []struct{ n string; v int }{{"2", 1}, {"", 2}}
	t.Run("", nil)
}
`)
	f := caseNames([]source{s})
	for _, want := range []string{`"case 1"`, `"2"`, `""`} {
		fires(t, f, "the table case "+want)
	}
	if len(f) != 4 {
		t.Errorf("findings = %q, want the four that say nothing", f)
	}
}

// pureCalls is the call half of arch.ports.1: a pure package calls none of
// the table's effect calls, though it may import their package.
func pureCalls(tb *layers.Table, all []source) []string {
	var out []string
	for _, s := range all {
		if r := tb.RowOf(s.pkg); s.test || r == nil || r.Kind != layers.KindPure {
			continue
		}
		names := imports(s.file)
		ast.Inspect(s.file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); ok && slices.Contains(tb.EffectCalls, names[x.Name]+"."+sel.Sel.Name) {
				out = append(out, s.path+": "+names[x.Name]+"."+sel.Sel.Name+" reaches the world from a pure package")
			}
			return true
		})
	}
	return out
}

func TestAnEffectCallInAPurePackageFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/config", "config.go", "package config\n\nimport \"path/filepath\"\n\nfunc f() { _, _ = filepath.Glob(\"*\"); _ = filepath.Join(\"a\") }\n")
	f := pureCalls(tb, []source{s})
	fires(t, f, "path/filepath.Glob reaches the world")
	if len(f) != 1 {
		t.Errorf("findings = %q; Join is pure", f)
	}
}

// TestAnAppdirsCallThatReachesTheDiskFailsInAPurePackage: appdirs is a pure
// library for the calls that read only what they are handed, as HereWith, so
// config, its pure owner, calls neither Here, which reads the home and the
// environment itself, nor Temp nor Free (gragas#1607). Planted: the three,
// beside a HereWith that passes.
func TestAnAppdirsCallThatReachesTheDiskFailsInAPurePackage(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/config", "config.go", "package config\n\nimport \"github.com/birdtable/go-files/appdirs\"\n\nfunc f() { _, _ = appdirs.Here(\"a\", appdirs.XDG); _, _ = appdirs.Temp(\"a\"); _, _ = appdirs.Free(\"/\"); _, _ = appdirs.HereWith(\"a\", appdirs.XDG, nil) }\n")
	f := pureCalls(tb, []source{s})
	fires(t, f, "appdirs.Here reaches the world")
	fires(t, f, "appdirs.Temp reaches the world")
	fires(t, f, "appdirs.Free reaches the world")
	if len(f) != 3 {
		t.Errorf("findings = %q; HereWith is pure", f)
	}
}

// domainTypes is arch.layers.1 and arch.ports.2 at the operations' surface:
// no exported type, field, function or method of the operations layer
// carries a type of a domain, so a command that imports the operations
// reaches no domain type, even through a field. An exported sentinel error
// a domain declares may be re-exported: its type is error.
func domainTypes(tb *layers.Table, module string, all []source) []string {
	var domains, ops []string
	for _, r := range tb.Rows {
		switch r.Layer {
		case "core", "legacy-core":
			domains = append(domains, r.Packages...)
		case "coreapi":
			ops = append(ops, r.Packages...)
		}
	}
	var out []string
	for _, s := range all {
		if s.test || !slices.Contains(ops, s.pkg) {
			continue
		}
		names := imports(s.file)
		leak := func(where string, e ast.Node) {
			if e == nil {
				return
			}
			ast.Inspect(e, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok {
						if r, own := strings.CutPrefix(names[x.Name], module+"/"); own && slices.Contains(domains, r) {
							out = append(out, s.path+": "+where+" carries "+r+"."+sel.Sel.Name)
						}
					}
				}
				return true
			})
		}
		for _, d := range s.file.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() && (d.Recv == nil || exportedRecv(d.Recv)) {
					leak(d.Name.Name, d.Type)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch sp := spec.(type) {
					case *ast.TypeSpec:
						if sp.Name.IsExported() {
							exportedType(sp, leak)
						}
					case *ast.ValueSpec:
						for i, n := range sp.Names {
							if !n.IsExported() {
								continue
							}
							leak(n.Name, sp.Type)
							if sp.Type == nil && i < len(sp.Values) && !strings.HasPrefix(n.Name, "Err") {
								leak(n.Name, sp.Values[i])
							}
						}
					}
				}
			}
		}
	}
	return out
}

// exportedRecv is whether a method's receiver is an exported type.
func exportedRecv(fl *ast.FieldList) bool {
	t := fl.List[0].Type
	if st, ok := t.(*ast.StarExpr); ok {
		t = st.X
	}
	id, ok := t.(*ast.Ident)
	return ok && id.IsExported()
}

// exportedType hands leak what an exported type shows: a struct's exported
// fields, or the whole type otherwise.
func exportedType(sp *ast.TypeSpec, leak func(string, ast.Node)) {
	st, ok := sp.Type.(*ast.StructType)
	if !ok {
		leak(sp.Name.Name, sp.Type)
		return
	}
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.IsExported() {
				leak(sp.Name.Name+"."+n.Name, f.Type)
			}
		}
		if len(f.Names) == 0 {
			leak(sp.Name.Name, f.Type)
		}
	}
}

// TestADomainTypeOnTheOperationsSurfaceFails plants the case the reviewer
// found: a Loaded that hands out the rulebook's Book.
func TestADomainTypeOnTheOperationsSurfaceFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	m := tb.Module
	s := parse(t, "internal/core/api", "api.go", "package coreapi\n\nimport (\n\t\""+m+"/internal/core/generate\"\n\t\""+m+"/internal/core/rulebook\"\n)\n\n"+
		"type Loaded struct {\n\tBook *rulebook.Book\n\tbook *rulebook.Book\n}\n\ntype Output = generate.Output\n\n"+
		"func Counts(b *rulebook.Book) int { return 0 }\n\nfunc (Loaded) Parts() []rulebook.Part { return nil }\n\n"+
		"var ErrNoParts = rulebook.ErrNoParts\n\nvar Default = rulebook.Options{}\n\nfunc counts(b *rulebook.Book) int { return 0 }\n")
	f := domainTypes(tb, m, []source{s})
	for _, want := range []string{"Loaded.Book carries internal/core/rulebook.Book", "Output carries internal/core/generate.Output",
		"Counts carries internal/core/rulebook.Book", "Parts carries internal/core/rulebook.Part", "Default carries internal/core/rulebook.Options"} {
		fires(t, f, want)
	}
	if len(f) != 5 {
		t.Errorf("findings = %q; unexported names and a sentinel error are not the surface", f)
	}
}

// nilPorts is arch.ports.5: no code but main tests a port of Ports for nil;
// every port is required, so the test would read a port's absence as a
// platform. A port copied to a variable or into a field first, l :=
// p.Locks or recorder{locks: p.Locks}, is the same port.
func nilPorts(tb *layers.Table, all []source) []string {
	var ports []string
	for _, p := range tb.Ports.List {
		ports = append(ports, p.Name)
	}
	isPort := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		return ok && slices.Contains(ports, sel.Sel.Name)
	}
	var out []string
	for _, s := range all {
		if s.test || strings.HasPrefix(s.pkg, "cmd/") {
			continue
		}
		aliases := map[string]bool{}
		ast.Inspect(s.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				for i, r := range x.Rhs {
					if id, ok := x.Lhs[i].(*ast.Ident); ok && i < len(x.Lhs) && isPort(r) {
						aliases[id.Name] = true
					}
				}
			case *ast.ValueSpec:
				for i, v := range x.Values {
					if i < len(x.Names) && isPort(v) {
						aliases[x.Names[i].Name] = true
					}
				}
			case *ast.KeyValueExpr:
				if k, ok := x.Key.(*ast.Ident); ok && isPort(x.Value) {
					aliases[k.Name] = true
				}
			}
			return true
		})
		named := func(e ast.Expr) (string, bool) {
			switch x := e.(type) {
			case *ast.SelectorExpr:
				return x.Sel.Name, slices.Contains(ports, x.Sel.Name) || aliases[x.Sel.Name]
			case *ast.Ident:
				return x.Name, aliases[x.Name]
			}
			return "", false
		}
		ast.Inspect(s.file, func(n ast.Node) bool {
			b, ok := n.(*ast.BinaryExpr)
			if !ok || (b.Op != token.EQL && b.Op != token.NEQ) {
				return true
			}
			for _, side := range [][2]ast.Expr{{b.X, b.Y}, {b.Y, b.X}} {
				id, isNil := side[1].(*ast.Ident)
				if name, port := named(side[0]); port && isNil && id.Name == "nil" {
					out = append(out, s.path+": a nil test on the port "+name+" (arch.ports.5)")
				}
			}
			return true
		})
	}
	return out
}

// standInWords are the words of a stand-in in a type's name, in any place
// and any case: FakeRepo, FilesFake, inMemoryRepo.
func standInWords() []string { return []string{"fake", "stub", "mock", "dummy", "inmemory", "spy"} }

// standInTypes is arch.layers.10: no stand-in kind of type in shipped code.
func standInTypes(tb *layers.Table, all []source) []string {
	var out []string
	for _, s := range all {
		if r := tb.RowOf(s.pkg); s.test || r == nil || r.Kind == layers.KindStandIn {
			continue
		}
		ast.Inspect(s.file, func(n ast.Node) bool {
			if spec, ok := n.(*ast.TypeSpec); ok {
				lower := strings.ToLower(spec.Name.Name)
				if slices.ContainsFunc(standInWords(), func(w string) bool { return strings.Contains(lower, w) }) {
					out = append(out, s.path+": "+spec.Name.Name+" is a stand-in in shipped code")
				}
			}
			return true
		})
	}
	return out
}

// TestANilTestOnACopiedPortFails plants the reviewer's case and a field
// copy: a port moved into a variable or a field is still a port.
func TestANilTestOnACopiedPortFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/cli/root", "recorder.go", "package root\n\nfunc f(p struct{ Lock, Journal any }) bool {\n\tl := p.Lock\n\tr := struct{ journal any }{journal: p.Journal}\n\treturn l == nil || nil != r.journal\n}\n")
	f := nilPorts(tb, []source{s})
	fires(t, f, "a nil test on the port l")
	fires(t, f, "a nil test on the port journal")
}

// TestAStandInNamedAnyWayFails plants the reviewer's names.
func TestAStandInNamedAnyWayFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/host/fs", "repo.go", "package fs\n\ntype FilesFake struct{}\ntype inMemoryRepo struct{}\ntype Repo struct{}\n")
	f := standInTypes(tb, []source{s})
	fires(t, f, "FilesFake is a stand-in")
	fires(t, f, "inMemoryRepo is a stand-in")
	if len(f) != 2 {
		t.Errorf("findings = %q", f)
	}
}

// TestATableCaseNamedByNumberAnyWayFails plants the reviewer's cases: a
// map-keyed table, t.Run(strconv.Itoa(i)) and a name like "#1".
func TestATableCaseNamedByNumberAnyWayFails(t *testing.T) {
	t.Parallel()
	s := parse(t, "internal/config", "more_test.go", `package config

import (
	"fmt"
	"strconv"
)

func cases() {
	_ = map[string]int{"#1": 1, "3": 2, "refuses a retired key": 3}
	for i := range 3 {
		t.Run(strconv.Itoa(i), nil)
		t.Run(fmt.Sprintf("case %d", i), nil)
		t.Run(fmt.Sprintf("%s holds", "x"), nil)
	}
	_ = []struct{ name string }{{name: "#2"}}
}
`)
	f := caseNames([]source{s})
	for _, want := range []string{`the table case "#1"`, `the table case "3"`, `the table case "#2"`, "t.Run is named by a number"} {
		fires(t, f, want)
	}
	if len(f) != 5 {
		t.Errorf("findings = %q, want the three literals and the two numbered names", f)
	}
}

// pathSeparators is the separators a path is written with on any system the
// source table names a root of, one a line in testdata/path_separators.txt.
func pathSeparators() []string {
	b, err := os.ReadFile("testdata/path_separators.txt")
	if err != nil {
		panic(err)
	}
	return strings.Fields(string(b))
}

// atRoot is whether lit starts at one of roots, in any case: the root
// itself, or a path below it, with a separator after a root such as /etc,
// and anything after a root that ends in one, such as a drive's.
func atRoot(lit string, roots []string) bool {
	l := strings.ToLower(lit)
	for _, r := range roots {
		r = strings.ToLower(r)
		if l == r {
			return true
		}
		for _, sep := range pathSeparators() {
			if strings.HasSuffix(r, sep) && strings.HasPrefix(l, r) || strings.HasPrefix(l, r+sep) {
				return true
			}
		}
	}
	return false
}

// TestAHomeFolderOrADriveInTheCoreFails plants the reviewer's cases.
func TestAHomeFolderOrADriveInTheCoreFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	s := parse(t, "internal/core/rulebook", "paths.go", "package rulebook\n\nconst (\n\ta = \"Library/LaunchAgents/x.plist\"\n\tb = \"~/.config/systemd/user\"\n\tc = \"c"+":\\\\Program Files\"\n\td = \"rulebook/etc/parts.yaml\"\n)\n")
	f := platformWords(tb, []source{s})
	fires(t, f, "names the system folder Library/LaunchAgents")
	fires(t, f, "names the system folder .config/systemd")
	fires(t, f, `"c`+`:\\Program Files" starts at a system root`)
	if len(f) != 3 {
		t.Errorf("findings = %q; a path inside the repository is no root", f)
	}
}

// inScope is whether the package at pkg is in a folder the source table
// names.
func inScope(tb *layers.Table, pkg string) bool {
	return slices.ContainsFunc(tb.Source.Scope, func(dir string) bool { return pkg == dir || strings.HasPrefix(pkg, dir+"/") })
}

// camelWords splits an identifier into its lower-case words: RuleID is rule
// and id, golangci_lint is golangci and lint.
func camelWords(id string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	runes := []rune(id)
	for i, r := range runes {
		switch {
		case r == '_' || r >= '0' && r <= '9':
			flush()
		case r >= 'A' && r <= 'Z':
			lowerNext := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if len(cur) > 0 && (runes[i-1] < 'A' || runes[i-1] > 'Z' || lowerNext) {
				flush()
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// namesTool is whether the words of an identifier hold a tool's words, in
// order and side by side: eslint, or golangci and lint.
func namesTool(words []string, tool string) bool {
	want := strings.Split(strings.ToLower(tool), "-")
	for i := 0; i+len(want) <= len(words); i++ {
		if slices.Equal(words[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

// sourceWords is arch.ports.2: no non-test file in the source table's
// folders holds a rule id or the name of a tool or of its config format in a
// string, and none names a tool in an identifier. Rules and tools are data in
// adapters, run by host/tool. Comments are prose and are not read.
func sourceWords(tb *layers.Table, all []source) []string {
	var out []string
	id := regexp.MustCompile(tb.Source.RuleID)
	tools := map[string]*regexp.Regexp{}
	for _, tool := range tb.Source.Tools {
		tools[tool] = wordOf(tool)
	}
	for _, s := range all {
		if s.test || !inScope(tb, s.pkg) {
			continue
		}
		for _, lit := range literals(s.file) {
			if m := id.FindString(lit); m != "" {
				out = append(out, s.path+": "+strconv.Quote(lit)+" holds the rule id "+m)
			}
			for _, tool := range tb.Source.Tools {
				if tools[tool].MatchString(lit) {
					out = append(out, s.path+": "+strconv.Quote(lit)+" names the tool "+tool)
				}
			}
			for _, format := range tb.Source.Formats {
				if strings.Contains(strings.ToLower(lit), strings.ToLower(format)) {
					out = append(out, s.path+": "+strconv.Quote(lit)+" names the config format "+format)
				}
			}
		}
		ast.Inspect(s.file, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				for _, tool := range tb.Source.Tools {
					if namesTool(camelWords(ident.Name), tool) {
						out = append(out, s.path+": the name "+ident.Name+" names the tool "+tool)
					}
				}
			}
			return true
		})
	}
	return out
}

// initFuncs is the init functions outside package main, and a count above or
// below the table's: the count only falls, and a fall is written down.
func initFuncs(tb *layers.Table, all []source) []string {
	var out []string
	n := 0
	for _, s := range all {
		if s.file.Name.Name == "main" {
			continue
		}
		for _, d := range s.file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" {
				n++
				out = append(out, s.path+": an init function outside main")
			}
		}
	}
	switch {
	case n > tb.Source.Inits:
		return append(out, "init functions outside main number "+strconv.Itoa(n)+", over the "+strconv.Itoa(tb.Source.Inits)+" "+layers.File+" counts: the count only falls")
	case n < tb.Source.Inits:
		return []string{"init functions outside main number " + strconv.Itoa(n) + ": lower inits in " + layers.File + " from " + strconv.Itoa(tb.Source.Inits)}
	}
	return nil
}

const idSrc = "package rulebook\n\nconst a = \"see arch.ports.2 for why\"\n"

func TestARuleIdOrAToolInTheCoreOrTheCommandTreeFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	ruleID := parse(t, "internal/core/rulebook", "id.go", idSrc)
	tool := parse(t, "internal/cli/root", "lint.go", "package root\n\nconst a = \"run golangci-lint now\"\nconst b = \"writes .eslintrc\"\n")
	name := parse(t, "internal/core/api", "name.go", "package coreapi\n\nvar eslintPath, GolangciLintBin string\n")
	f := sourceWords(tb, []source{ruleID, tool, name})
	fires(t, f, "holds the rule id arch.ports.2")
	fires(t, f, "names the tool golangci-lint")
	fires(t, f, "names the config format .eslintrc")
	fires(t, f, "the name eslintPath names the tool eslint")
	fires(t, f, "the name GolangciLintBin names the tool golangci-lint")
}

// TestTheSourceWordsPassWhatTheyDoNotRefuse is the clean control and the
// refusal's other side: a tool in host/tool, in a test, in a comment or as
// part of a longer word is fine, and a version number is no rule id.
func TestTheSourceWordsPassWhatTheyDoNotRefuse(t *testing.T) {
	t.Parallel()
	tb := table(t)
	host := parse(t, "internal/host/tool", "run.go", "package tool\n\nconst a = \"eslint\"\n")
	test := parse(t, "internal/core/rulebook", "id_test.go", idSrc)
	test.test = true
	comment := parse(t, "internal/cli/root", "c.go", "package root\n\n// eslint and arch.ports.2 in prose.\nconst a = \"v1.2.3 of the tool, buffer.go\"\nvar ruffle, eslinted, rule, id string\n")
	if f := sourceWords(tb, []source{host, test, comment}); len(f) != 0 {
		t.Errorf("findings = %q, want none", f)
	}
	if len(tb.Source.Tools) == 0 || len(tb.Source.Scope) == 0 {
		t.Error("the table gives the source test no tools or no scope, so it would pass on nothing")
	}
}

func TestAnInitFunctionOutsideMainFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	lib := parse(t, "internal/core/api", "i.go", "package coreapi\n\nfunc init() {}\n")
	cmd := parse(t, "cmd/gragas", "m.go", "package main\n\nfunc init() {}\n")
	f := initFuncs(tb, []source{lib, cmd})
	fires(t, f, "internal/core/api/i.go: an init function outside main")
	fires(t, f, "the count only falls")
	if f := initFuncs(tb, []source{cmd}); len(f) != 0 {
		t.Errorf("an init in main: %q", f)
	}
	low := *tb
	low.Source.Inits = 2
	fires(t, initFuncs(&low, []source{lib}), "lower inits")
}

// exempt is the paths gate/exemptions.yaml waives rule from: an exception to
// the source test is a recorded row with an owner and an expiry, never a
// widened test (gates.exemptions.2).
func exempt(t *testing.T, rule string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "gate", "exemptions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ordered.ParseYAML(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		t.Fatalf("gate/exemptions.yaml cannot be read: %v", err)
	}
	var out []string
	for _, row := range ordered.Maps(doc.List("exemptions")) {
		if row.Str("rule") == rule {
			out = append(out, row.Str("path"))
		}
	}
	return out
}

// waived drops the findings under a waived path, and says each waiver that
// waived nothing, since a row nothing needs is to be deleted.
func waived(findings, paths []string) (kept, unused []string) {
	for _, p := range paths {
		if !slices.ContainsFunc(findings, func(f string) bool { return strings.HasPrefix(f, p) }) {
			unused = append(unused, "gate/exemptions.yaml waives "+p+", where nothing fires: delete the row")
		}
	}
	for _, f := range findings {
		if !slices.ContainsFunc(paths, func(p string) bool { return strings.HasPrefix(f, p) }) {
			kept = append(kept, f)
		}
	}
	return kept, unused
}

func TestAWaiverHidesOnlyItsPathAndIsDeletedWhenUnused(t *testing.T) {
	t.Parallel()
	found := []string{"internal/core/rulebook/a.go: x", "internal/cli/root/b.go: y"}
	kept, unused := waived(found, []string{"internal/core/rulebook/"})
	if !slices.Equal(kept, found[1:]) || len(unused) != 0 {
		t.Errorf("kept %q, unused %q", kept, unused)
	}
	_, unused = waived(found, []string{"internal/core/gone/"})
	fires(t, unused, "delete the row")
}

// ruleAndToolWords is sourceWords less what gate/exemptions.yaml waives for
// arch.ports.2, and the waivers that waive nothing.
func ruleAndToolWords(t *testing.T, tb *layers.Table, all []source) []string {
	t.Helper()
	paths := exempt(t, "arch.ports.2")
	kept, unused := waived(sourceWords(tb, all), paths)
	return append(kept, unused...)
}

// clockReads is arch.ports.1: only the package that fills the Clock port
// calls time.Now, and no other file does, a test of the core included: those
// ask the fake Clock. The adapter's own folder is read from layers.yaml.
func clockReads(tb *layers.Table, all []source) []string {
	adapter := tb.ClockAdapter()
	var out []string
	for _, s := range all {
		if s.pkg == adapter {
			continue
		}
		names := imports(s.file)
		ast.Inspect(s.file, func(n ast.Node) bool {
			x, ok := n.(*ast.SelectorExpr)
			if !ok || x.Sel.Name != "Now" {
				return true
			}
			if id, ok := x.X.(*ast.Ident); ok && names[id.Name] == "time" {
				out = append(out, s.path+": time.Now outside "+adapter+", the Clock adapter")
			}
			return true
		})
	}
	return out
}

func TestATimeNowOutsideTheClockAdapterFails(t *testing.T) {
	t.Parallel()
	tb := table(t)
	core := parse(t, "internal/core/rulebook", "book_test.go", "package rulebook\n\nimport \"time\"\n\nvar _ = time.Now()\n")
	aliased := parse(t, "internal/cli/root", "root.go", "package root\n\nimport t \"time\"\n\nvar _ = t.Now\n")
	adapter := parse(t, tb.ClockAdapter(), "clock.go", "package clock\n\nimport \"time\"\n\nvar _ = time.Now()\n")
	other := parse(t, "internal/host/fs", "tree.go", "package fs\n\nimport \"time\"\n\nvar _ = time.Now().Add(time.Second)\nvar _ = time.Unix(0, 0)\n")
	f := clockReads(tb, []source{core, aliased, adapter, other})
	fires(t, f, "internal/core/rulebook/book_test.go: time.Now outside "+tb.ClockAdapter())
	fires(t, f, "internal/cli/root/root.go: time.Now outside")
	fires(t, f, "internal/host/fs/tree.go: time.Now outside")
	if len(f) != 3 {
		t.Errorf("findings = %q; the adapter and time.Unix are not reads of the clock", f)
	}
}

// A goroutine started by WaitGroup.Go or errgroup's Go is held to what a go statement is: the
// measured line above it, a leak check in its package's tests, and no start in an adapter. The
// leak check may be configtest.NoLeaks, and a NoLeaks of another package is none (gragas#663).
//
// What it costs when this fails: a goroutine started through Go has no measured gain and no leak
// check, and the rule that holds a go statement to both is silent about it.
func TestAGoCallIsHeldToWhatAGoStatementIs(t *testing.T) {
	t.Parallel()
	tb := table(t)
	const pre = "package rulebook\n\nimport \"sync\"\n\nfunc p() {\n\tvar wg sync.WaitGroup\n"
	bare := parse(t, "internal/core/rulebook", "wg.go", pre+"\twg.Go(func() {})\n}\n")
	f := goroutines(tb, []source{bare})
	fires(t, f, "a Go call (WaitGroup.Go or errgroup's) with no '// measured: what, gain, where' line above")
	fires(t, f, "a Go call (WaitGroup.Go or errgroup's) in a package whose tests have no leak check")
	eg := parse(t, "internal/core/rulebook", "eg.go", "package rulebook\n\nfunc p(g interface{ Go(func() error) }) {\n\tg.Go(func() error { return nil })\n}\n")
	fires(t, goroutines(tb, []source{eg}), "a Go call (WaitGroup.Go or errgroup's) with no '// measured")
	host := parse(t, "internal/host/fs", "wg.go", strings.Replace(pre, "rulebook", "fs", 1)+"\twg.Go(func() {})\n}\n")
	fires(t, goroutines(tb, []source{host}), "a goroutine in an adapter")
	// A Go method that starts nothing (no function literal to run) is not a goroutine.
	other := parse(t, "internal/core/rulebook", "m.go", "package rulebook\n\nfunc p(m interface{ Go() int }) { _ = m.Go() }\n")
	if f := goroutines(tb, []source{other}); len(f) > 0 {
		t.Errorf("a Go method with no function was taken for a goroutine: %q", f)
	}
	good := parse(t, "internal/core/rulebook", "wg.go", "package rulebook\n\nimport \"sync\"\n\nfunc p() {\n\tvar wg sync.WaitGroup\n\t// measured: reading 1000 files, 4x faster, BenchmarkLoad in book_test.go\n\twg.Go(func() {})\n}\n")
	noLeaks := parse(t, "internal/core/rulebook", "main_test.go", "package rulebook\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\tconfigtest \"github.com/birdtable/gragas/internal/config/testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(configtest.NoLeaks(configtest.Main(m))) }\n")
	if f := goroutines(tb, []source{good, noLeaks}); len(f) > 0 {
		t.Errorf("a measured Go call whose tests end in configtest.NoLeaks was refused: %q", f)
	}
	notOurs := parse(t, "internal/core/rulebook", "main_test.go", "package rulebook\n\nimport other \"example.com/other\"\n\nvar _ = other.NoLeaks(0)\n")
	fires(t, goroutines(tb, []source{good, notOurs}), "a Go call (WaitGroup.Go or errgroup's) in a package whose tests have no leak check")
}

// keyReads holds the engine to holding no private key (no-secrets in
// docs/architecture.md, cli.config.4): no shipped file imports an SSH or
// X.509 key parser or names the armor a private key's content opens with.
// gragas signs nothing (release-pin-by-hash, gragas#1556), so no package
// has a reason to.
func keyReads(all []source) []string {
	parsers := []string{"golang.org/x/crypto/ssh", "golang.org/x/crypto/ssh/agent", "crypto/x509", "encoding/pem"}
	out := []string{}
	for _, s := range all {
		if s.test {
			continue
		}
		for _, imp := range imports(s.file) {
			if slices.Contains(parsers, imp) {
				out = append(out, s.path+": imports "+imp+", which parses keys, and the engine reads no private key")
			}
		}
		for _, lit := range literals(s.file) {
			if strings.Contains(lit, "PRIVATE KEY") {
				out = append(out, s.path+": names the armor of a private key's content, which the engine never reads")
			}
		}
	}
	return out
}

// TestAReadOfAPrivateKeyFails plants a key parser imported in a shipped
// file and the private key armor named in one. A test file is not shipped.
func TestAReadOfAPrivateKeyFails(t *testing.T) {
	parser := parse(t, "internal/core/release", "sum.go", "package release\n\nimport _ \"golang.org/x/crypto/ssh\"\n")
	armor := parse(t, "internal/host/fs", "tree.go", "package fs\n\nconst head = \"-----BEGIN OPENSSH PRIVATE KEY-----\"\n")
	test := parse(t, "internal/host/git", "remote_ssh_test.go", "package git\n\nimport _ \"golang.org/x/crypto/ssh\"\n\nconst head = \"PRIVATE KEY\"\n")
	f := keyReads([]source{parser, armor, test})
	fires(t, f, "internal/core/release/sum.go: imports golang.org/x/crypto/ssh")
	fires(t, f, "internal/host/fs/tree.go: names the armor of a private key's content")
	if len(f) != 2 {
		t.Errorf("findings = %q; a test file is not shipped", f)
	}
}
