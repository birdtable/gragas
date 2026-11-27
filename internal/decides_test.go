package internal

// The trust line (arch.ports.19, A46, architecture 4.3): data from a program
// or a repository the engine does not control reaches a decision only through
// a field its contract type marks "Decides:". gragas-generate writes that mark
// from a property's x-decides, and "Display only." on every other field. This
// test type-checks each deciding context and every other package of the
// module that selects a field of a contract type, and fails a condition, a
// switch, a type switch, a range, a comparison, a logical operator or a map
// key that reads a field of a contract type not marked as deciding. A package
// is walked unless displayOnly names it with its reason, so a new package
// that reads a contract value is walked without anyone listing it (#600). It
// follows no data flow past one statement: a value copied out of the field in
// an if's, a for's or a switch's init is read as part of the decision, but one
// copied in an earlier statement, or one a helper returns, is not followed,
// since the mark is at the type (A46). The walk is here, not in a shared
// helper, until #507's G13 makes internal/dev/goast.

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// decidingContexts are the packages that decide on data from outside: the
// four architecture 4.3 names and coreapi, and internal/core/contract, which
// reads every document first. A planned one is read once it exists.
func decidingContexts() []string {
	return []string{"internal/core/link", "internal/core/proof", "internal/core/project", "internal/core/release", "internal/core/api", "internal/core/contract"}
}

// displayOnly is each package that selects a field of a contract type and
// is not walked, with the reason it decides nothing on one. Every other such
// package is walked. A row must name a package that selects a contract field
// and is not a deciding context: a row with nothing to exempt is refused, so
// the list never holds an exemption in advance.
func displayOnly() map[string]string {
	return map[string]string{
		contractTypes: "the generated ToCanon and FromCanon (gragas#563) copy every field between the type and canon's tree, and compare a field only with its zero value to leave an absent one out as encoding/json's omitempty did; TestTheGeneratedGoDeclaresTypesAndConstantsOnly holds the generated files to types, constants and that codec, and gragas-generate --check (make gen-check) refuses any other non-test Go file in the package's folder",
	}
}

// contractTypes is the package of the types generated from the contract's
// schemas, relative to the module.
const contractTypes = "internal/model/contract"

// readers is each package of the module, relative to it, that may select a
// field of a contract type: the contract types' own package and every
// package that depends on it. A package that does not depend on it cannot
// name a contract type's field.
func readers(module string) ([]string, error) {
	var out []string
	err := golist(func(dec *json.Decoder) error {
		var l struct {
			ImportPath string
			Deps       []string
		}
		if err := dec.Decode(&l); err != nil {
			return err
		}
		if l.ImportPath == module+"/"+contractTypes || slices.Contains(l.Deps, module+"/"+contractTypes) {
			out = append(out, strings.TrimPrefix(l.ImportPath, module+"/"))
		}
		return nil
	}, "-json=ImportPath,Deps", "./...")
	return out, err
}

// exportedPkg is the part of go list's answer the walk reads.
type exportedPkg struct {
	ImportPath string
	Dir        string
	Export     string
	GoFiles    []string
	Error      *struct{ Err string }
}

// exported is go list -export over pkgs and every package they import, by
// import path: where each one's export data is, and the files of each.
func exported(module string, pkgs []string) (map[string]exportedPkg, error) {
	args := []string{"-export", "-deps", "-json=ImportPath,Dir,Export,GoFiles,Error"}
	for _, p := range pkgs {
		args = append(args, module+"/"+p)
	}
	out := map[string]exportedPkg{}
	err := golist(func(dec *json.Decoder) error {
		var l exportedPkg
		if err := dec.Decode(&l); err != nil {
			return err
		}
		out[l.ImportPath] = l
		return nil
	}, args...)
	return out, err
}

// fromExports is an importer that reads each package from the export data go
// list named, and refuses one it did not name.
func fromExports(fset *token.FileSet, pkgs map[string]exportedPkg) types.Importer {
	return importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		l, ok := pkgs[path]
		if !ok || l.Export == "" {
			return nil, errors.New("go list named no export data for " + path)
		}
		return os.Open(l.Export)
	})
}

// scopeOf is the deciding contexts that exist, and a problem for each that
// is neither a package nor planned in layers.yaml: a renamed context must
// never drop out of the walk unseen.
func scopeOf(scope, planned []string, exists func(string) bool) ([]string, []string) {
	var in, problems []string
	for _, p := range scope {
		switch {
		case exists(p):
			in = append(in, p)
		case slices.Contains(planned, p):
		default:
			problems = append(problems, p+" is a deciding context that is neither a package nor planned in layers.yaml")
		}
	}
	return in, problems
}

// marks is whether each field of the contract types, "Type.Field", decides,
// read from the generated doc comments; unmarked is each field whose doc says
// neither "Decides:" nor "Display only.", or says both.
func marks(dir string) (decides map[string]bool, unmarked []string, err error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.gen.go"))
	if err != nil || len(names) == 0 {
		return nil, nil, errors.Join(errors.New("no generated contract types in "+dir), err)
	}
	decides = map[string]bool{}
	fset := token.NewFileSet()
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			return nil, nil, err
		}
		for _, d := range f.Decls {
			unmarked = append(unmarked, markDecl(d, decides)...)
		}
	}
	return decides, unmarked, nil
}

// markDecl records the marks of the struct types d declares, and answers the
// fields it holds that are unmarked.
func markDecl(d ast.Decl, decides map[string]bool) []string {
	g, ok := d.(*ast.GenDecl)
	if !ok {
		return nil
	}
	var unmarked []string
	for _, s := range g.Specs {
		ts, ok := s.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, fl := range st.Fields.List {
			var says, shows bool
			for line := range strings.Lines(fl.Doc.Text()) {
				says = says || strings.HasPrefix(line, "Decides: ")
				shows = shows || strings.TrimSpace(line) == "Display only."
			}
			for _, id := range fl.Names {
				name := ts.Name.Name + "." + id.Name
				decides[name] = says && !shows
				if says == shows {
					unmarked = append(unmarked, name)
				}
			}
		}
	}
	return unmarked
}

// fieldsOf names each field of the contract types by "Type.Field", so a
// selection the type checker resolves can be looked up in the marks.
func fieldsOf(pkg *types.Package) map[*types.Var]string {
	out := map[*types.Var]string{}
	for _, name := range pkg.Scope().Names() {
		tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		if st, ok := tn.Type().Underlying().(*types.Struct); ok {
			for f := range st.Fields() {
				out[f] = name + "." + f.Name()
			}
		}
	}
	return out
}

// trust is what the walk reads a package against: the type checker's answers
// and the contract's fields with their marks.
type trust struct {
	fset    *token.FileSet
	info    *types.Info
	fields  map[*types.Var]string
	decides map[string]bool
	seen    map[token.Pos]bool
	out     []string
	at      []readAt // where each of out was read, in step with it
	fn      string   // the top-level function being read, "" outside one
}

// readAt is the function and the field of one recorded read.
type readAt struct{ fn, field string }

// admission is a display-only field read in one function where nothing is
// decided, so the walk does not count its reads: the package, the function, the
// field, the number of reads there are, and why they decide nothing. Only one
// kind of read may have a row: a display field compared with the deciding field
// it copies, where a difference refuses. A row is
// exact both ways: a read more than it names is red, and a row whose function
// or field is gone, or has fewer reads, is red, so it cannot outlive its code
// or be written in advance. It is named by function, not by file and line.
type admission struct {
	pkg, fn, field string
	reads          int
	reason         string
}

// admissions are the rows. There is one: ReadBundle holds a history step's
// display copy, HistoryEntry.Status, equal to after.status, the value that
// decides, and only refuses when they differ (the nil check and the comparison
// with after["status"], gragas#1980, #2096).
func admissions() []admission {
	return []admission{{
		pkg: "internal/core/contract", fn: "splitStatus", field: "HistoryEntry.Status", reads: 2,
		reason: "compared with after.status, the field it copies, and only refuses when they differ (gragas#2096)",
	}}
}

// settle is the reads of the package rel the rows do not name, and a line for
// each row of the package whose reads are fewer than it says.
func (tr *trust) settle(rel string) (reads, stale []string) {
	drop := map[int]bool{}
	for _, a := range admissions() {
		if a.pkg != rel {
			continue
		}
		var at []int
		for i, r := range tr.at {
			if r.fn == a.fn && r.field == a.field {
				at = append(at, i)
			}
		}
		switch {
		case len(at) == a.reads:
			for _, i := range at {
				drop[i] = true
			}
		case len(at) < a.reads:
			stale = append(stale, fmt.Sprintf("%s: the admission of %d reads of %s in %s finds %d: drop or fix the row", rel, a.reads, a.field, a.fn, len(at)))
		}
	}
	for i, r := range tr.out {
		if !drop[i] {
			reads = append(reads, r)
		}
	}
	return reads, stale
}

// displayReads is each place in files where a condition, a switch, a type
// switch or a case, a range, a comparison, a logical operator or a map key
// reads a contract field that does not decide, as "file:line: <where> reads
// Type.Field". An if's or a switch's init is read as part of what it decides.
func (tr *trust) displayReads(files []*ast.File) []string {
	for _, f := range files {
		for _, d := range f.Decls {
			tr.fn = ""
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				tr.fn = fd.Name.Name
			}
			tr.inspect(d)
		}
	}
	return tr.out
}

// inspect records the reads under one top-level declaration.
func (tr *trust) inspect(d ast.Decl) {
	{
		ast.Inspect(d, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.IfStmt:
				tr.init(n.Init, "an if condition")
				tr.check(n.Cond, "an if condition")
			case *ast.ForStmt:
				tr.init(n.Init, "a for condition")
				tr.check(n.Cond, "a for condition")
			case *ast.SwitchStmt:
				tr.init(n.Init, "a switch")
				tr.check(n.Tag, "a switch")
			case *ast.TypeSwitchStmt:
				tr.init(n.Init, "a type switch")
				tr.check(asserted(n.Assign), "a type switch")
			case *ast.RangeStmt:
				tr.check(n.X, "a range")
			case *ast.CaseClause:
				for _, e := range n.List {
					tr.check(e, "a case")
				}
			case *ast.BinaryExpr:
				switch {
				case compares(n.Op):
					tr.check(n.X, "a comparison")
					tr.check(n.Y, "a comparison")
				case n.Op == token.LAND || n.Op == token.LOR:
					tr.operand(n.X)
					tr.operand(n.Y)
				}
			case *ast.UnaryExpr:
				if n.Op == token.NOT {
					tr.operand(n.X)
				}
			case *ast.IndexExpr:
				if tr.isMap(n.X) {
					tr.check(n.Index, "a map key")
				}
			case *ast.CompositeLit:
				tr.mapKeys(n)
			}
			return true
		})
	}
}

// mapKeys checks each key of a map literal.
func (tr *trust) mapKeys(n *ast.CompositeLit) {
	if !tr.isMap(n) {
		return
	}
	for _, e := range n.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			tr.check(kv.Key, "a map key")
		}
	}
}

// init checks the right-hand sides of an if's, a for's or a switch's init as
// part of what it decides: `if v := d.Version; v == ""` reads the field for
// its condition in one statement. Each right-hand side is read by carried.
func (tr *trust) init(s ast.Stmt, where string) {
	as, ok := s.(*ast.AssignStmt)
	if !ok {
		return
	}
	for _, e := range as.Rhs {
		tr.carried(e, where)
	}
}

// carried checks each field whose value e carries on: the field itself, one
// a type assertion asks of, the operands of an arithmetic or a string
// operator (`d.Version + "x"`), the operand of a unary operator or a
// dereference, what is indexed or sliced and by what, and the arguments of a
// builtin (`len(d.Version)`) or a conversion. A comparison and a logical
// operator are read where they stand. A call to a function is its own, so
// `if err := write(d.Name); err != nil` decides on the error, not the field.
func (tr *trust) carried(e ast.Expr, where string) {
	switch e := ast.Unparen(e).(type) {
	case *ast.SelectorExpr:
		tr.check(e, where)
	case *ast.TypeAssertExpr:
		tr.check(e.X, where)
	case *ast.BinaryExpr:
		if !compares(e.Op) && e.Op != token.LAND && e.Op != token.LOR {
			tr.carried(e.X, where)
			tr.carried(e.Y, where)
		}
	case *ast.UnaryExpr:
		if e.Op != token.NOT {
			tr.carried(e.X, where)
		}
	case *ast.StarExpr:
		tr.carried(e.X, where)
	case *ast.IndexExpr:
		tr.carried(e.X, where)
		tr.carried(e.Index, where)
	case *ast.SliceExpr:
		for _, x := range []ast.Expr{e.X, e.Low, e.High, e.Max} {
			if x != nil {
				tr.carried(x, where)
			}
		}
	case *ast.CallExpr:
		if f := tr.info.Types[e.Fun]; f.IsBuiltin() || f.IsType() {
			for _, a := range e.Args {
				tr.carried(a, where)
			}
		}
	}
}

// asserted is what a type switch asks the type of: the x of "x.(type)",
// written alone or assigned.
func asserted(s ast.Stmt) ast.Expr {
	var e ast.Expr
	switch s := s.(type) {
	case *ast.ExprStmt:
		e = s.X
	case *ast.AssignStmt:
		if len(s.Rhs) == 1 {
			e = s.Rhs[0]
		}
	}
	if ta, ok := ast.Unparen(e).(*ast.TypeAssertExpr); ok {
		return ta.X
	}
	return nil
}

// operand checks one operand of &&, || or !. An operand that is itself a
// comparison or a logical operator is left to its own case, so each read is
// named by the nearest operator that decides on it.
func (tr *trust) operand(e ast.Expr) {
	switch u := ast.Unparen(e).(type) {
	case *ast.BinaryExpr:
		if compares(u.Op) || u.Op == token.LAND || u.Op == token.LOR {
			return
		}
	case *ast.UnaryExpr:
		if u.Op == token.NOT {
			return
		}
	}
	tr.check(e, "a logical operator")
}

func compares(op token.Token) bool {
	return slices.Contains([]token.Token{token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ}, op)
}

func (tr *trust) isMap(e ast.Expr) bool {
	t := tr.info.TypeOf(e)
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Map)
	return ok
}

// check records each read under e of a contract field that does not decide.
// A read inside a comparison inside an if is one read, recorded once.
func (tr *trust) check(e ast.Expr, where string) {
	if e == nil {
		return
	}
	ast.Inspect(e, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || tr.seen[sel.Sel.Pos()] {
			return true
		}
		s, ok := tr.info.Selections[sel]
		if !ok || s.Kind() != types.FieldVal {
			return true
		}
		v, ok := s.Obj().(*types.Var)
		if !ok {
			return true
		}
		if name, isContract := tr.fields[v]; isContract && !tr.decides[name] {
			tr.seen[sel.Sel.Pos()] = true
			p := tr.fset.Position(sel.Sel.Pos())
			file := strings.TrimPrefix(filepath.ToSlash(p.Filename), root+"/")
			tr.out = append(tr.out, file+":"+strconv.Itoa(p.Line)+": "+where+" reads "+name)
			tr.at = append(tr.at, readAt{tr.fn, name})
		}
		return true
	})
}

// walker reads packages against the contract's marks: one importer, so each
// package and the contract types are the same objects in every check.
type walker struct {
	module  string
	fset    *token.FileSet
	pkgs    map[string]exportedPkg
	imp     types.Importer
	fields  map[*types.Var]string
	decides map[string]bool
}

// newWalker loads the export data of scope and the contract types, and the
// marks from the generated files.
func newWalker(module string, scope []string) (*walker, []string, error) {
	pkgs, err := exported(module, append(slices.Clone(scope), contractTypes))
	if err != nil {
		return nil, nil, err
	}
	decides, unmarked, err := marks(filepath.Join(root, contractTypes))
	if err != nil {
		return nil, nil, err
	}
	w := &walker{module: module, fset: token.NewFileSet(), pkgs: pkgs, decides: decides}
	w.imp = fromExports(w.fset, pkgs)
	mc, err := w.imp.Import(module + "/" + contractTypes)
	if err != nil {
		return nil, nil, err
	}
	w.fields = fieldsOf(mc)
	return w, unmarked, nil
}

// pkgReads is what the walk read in one package: whether it selects a field
// of a contract type anywhere, and its display-only reads in decisions.
type pkgReads struct {
	selects bool
	reads   []string
	stale   []string // the rows of admissions that name more reads than there are
}

// read type-checks files as the package path and answers whether it selects
// a contract field and its display-only reads. A package that does not
// type-check is an error, never an empty list: what cannot be read cannot be
// said to hold. The contract types' own
// package, read from source, has its own objects for its fields, so its
// fields are named from what this check made of it.
func (w *walker) read(path string, files []*ast.File) (pkgReads, error) {
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	conf := types.Config{Importer: w.imp}
	pkg, err := conf.Check(path, w.fset, files, info)
	if err != nil {
		return pkgReads{}, errors.New(path + " does not type-check, so the trust line is not read in it: " + err.Error())
	}
	fields := w.fields
	if path == w.module+"/"+contractTypes {
		fields = fieldsOf(pkg)
	}
	var r pkgReads
	for _, s := range info.Selections {
		if v, ok := s.Obj().(*types.Var); ok && s.Kind() == types.FieldVal {
			if _, isContract := fields[v]; isContract {
				r.selects = true
				break
			}
		}
	}
	tr := &trust{fset: w.fset, info: info, fields: fields, decides: w.decides, seen: map[token.Pos]bool{}}
	tr.displayReads(files)
	r.reads, r.stale = tr.settle(strings.TrimPrefix(path, w.module+"/"))
	return r, nil
}

// verdict is what fails in the package rel, given what the walk read in it:
// each display-only read in a decision, unless displayOnly names the package.
// A row on a deciding context, and a row for a package that selects no
// contract field, are refused: a deciding context is always walked, and a
// row with nothing to exempt would exempt the next read unseen.
func verdict(rel string, r pkgReads, deciding []string, display map[string]string) []string {
	why, listed := display[rel]
	switch {
	case listed && slices.Contains(deciding, rel):
		return []string{rel + " is a deciding context, and displayOnly lists it (" + why + "): drop the row; a deciding context is always walked"}
	case listed && !r.selects:
		return []string{rel + " is listed display only (" + why + ") but selects no contract field: drop the row until it reads one"}
	case listed:
		return nil
	}
	var out []string
	for _, read := range r.reads {
		out = append(out, read+", which is display only: decide on a field whose schema marks it x-decides, mark this one with the one decision it drives, or list "+rel+" in displayOnly with the reason it decides nothing")
	}
	return out
}

// parsed is the non-test files of a exportedPkg package, named from the module's
// root.
func (w *walker) parsed(l exportedPkg, rel string) ([]*ast.File, error) {
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

// walkScope is the packages the walk reads: each deciding context that
// exists, then each reader not already among them, in order.
func walkScope(deciding, readers []string) []string {
	scope := slices.Clone(deciding)
	for _, p := range readers {
		if !slices.Contains(scope, p) {
			scope = append(scope, p)
		}
	}
	return scope
}

// strayRows is a problem for each row of display that names no package
// that may select a contract field: a renamed or removed package's row must
// not wait for the next package to take its name.
func strayRows(display map[string]string, readers []string) []string {
	var out []string
	for _, p := range slices.Sorted(maps.Keys(display)) {
		if !slices.Contains(readers, p) {
			out = append(out, p+" is listed display only ("+display[p]+") but is no package that depends on "+contractTypes+": drop the row")
		}
	}
	return out
}

// TestNoDecisionReadsAContractFieldThatIsDisplayOnly: in each deciding
// context and every other package that selects a contract field, unless
// displayOnly names it, no condition, switch, comparison or map key reads a
// field of a contract type unless the field's schema marks it x-decides;
// every field of the contract types is marked one way; every deciding
// context is a package or planned; and every displayOnly row names a package
// that selects a contract field and does not decide (arch.ports.19, A46,
// #600, gates.plants.1). Planted failure: a read of a display-only field in
// coreapi, or in a package on neither list.
func TestNoDecisionReadsAContractFieldThatIsDisplayOnly(t *testing.T) {
	t.Parallel()
	tb := table(t)
	var planned []string
	for _, p := range tb.Planned {
		planned = append(planned, p.Packages...)
	}
	deciding, problems := scopeOf(decidingContexts(), planned, func(p string) bool {
		names, err := filepath.Glob(filepath.Join(root, p, "*.go"))
		return err == nil && len(names) > 0
	})
	for _, p := range problems {
		t.Error(p)
	}
	rd, err := readers(tb.Module)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(rd, contractTypes) {
		t.Fatalf("go list named no package %s among the module's packages %v: the walk would read nothing", contractTypes, rd)
	}
	display := displayOnly()
	for _, p := range strayRows(display, rd) {
		t.Error(p)
	}
	scope := walkScope(deciding, rd)
	w, unmarked, err := newWalker(tb.Module, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range unmarked {
		t.Errorf("the contract field %s says neither \"Decides:\" nor \"Display only.\": run make gen", f)
	}
	for _, a := range admissions() {
		if !slices.Contains(scope, a.pkg) {
			t.Errorf("the admission of %s in %s names a package the walk does not read", a.field, a.pkg)
		}
	}
	selecting := 0
	for _, p := range scope {
		l, ok := w.pkgs[tb.Module+"/"+p]
		if !ok || l.Error != nil {
			t.Errorf("go list did not read %s: %+v", p, l.Error)
			continue
		}
		files, err := w.parsed(l, p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := w.read(l.ImportPath, files)
		if err != nil {
			t.Error(err)
			continue
		}
		if r.selects {
			selecting++
		}
		for _, f := range verdict(p, r, deciding, display) {
			t.Error(f)
		}
		for _, f := range r.stale {
			t.Error(f)
		}
	}
	t.Logf("read %d packages (%d deciding contexts, %d that select a contract field, %d display only) against %d contract fields", len(scope), len(deciding), selecting, len(display), len(w.fields))
}

// plantedReads is the display-only reads of one planted file, type-checked
// against the real contract types.
func plantedReads(t *testing.T, w *walker, body string) ([]string, error) {
	t.Helper()
	r, err := plantedPkg(t, w, body)
	return r.reads, err
}

// plantedPkg is what the walk reads in one planted file as a package.
func plantedPkg(t *testing.T, w *walker, body string) (pkgReads, error) {
	t.Helper()
	return plantedIn(t, w, "planted", body)
}

// plantedIn is plantedPkg with the package's path named, for a plant that stands
// in a real package's place.
func plantedIn(t *testing.T, w *walker, path, body string) (pkgReads, error) {
	t.Helper()
	src := "package planted\n\nimport mc \"" + w.module + "/" + contractTypes + "\"\n\n" + body
	f, err := parser.ParseFile(w.fset, "planted.go", src, 0)
	if err != nil {
		t.Fatalf("the plant does not parse: %v\n%s", err, src)
	}
	return w.read(path, []*ast.File{f})
}

// TestAPackageOnNeitherListIsWalked: a package that selects a contract field
// and is neither a deciding context nor listed display only is walked, so a
// decision on a display-only field in it fails, and so does one in a new
// package nobody gave a row; listed display only with its reason, it passes
// (the clean control). A row on a deciding context, a row for a package that
// selects no contract field, and a row for no package that depends on the
// contract types are each refused (#600). Planted failure: the walk scoped
// to the deciding contexts alone, or a row that exempts in advance.
func TestAPackageOnNeitherListIsWalked(t *testing.T) {
	t.Parallel()
	deciding := []string{"internal/core/contract"}
	fresh := "internal/core/fresh"
	if got := walkScope(deciding, []string{contractTypes, "internal/core/contract", fresh}); !slices.Equal(got, []string{"internal/core/contract", contractTypes, fresh}) {
		t.Errorf("walkScope = %v; want the deciding context, then the contract types and the new package nobody listed", got)
	}
	w, _, err := newWalker(table(t).Module, nil)
	if err != nil {
		t.Fatal(err)
	}
	decides, err := plantedPkg(t, w, "func f(d mc.SarifDocument) bool {\n\treturn d.Version == \"2.1.0\"\n}\n")
	if err != nil || !decides.selects || len(decides.reads) != 1 {
		t.Fatalf("the planted decision read gave %+v, %v; want one read and the package selecting", decides, err)
	}
	if got := verdict(fresh, decides, deciding, map[string]string{}); len(got) != 1 || !strings.Contains(got[0], "planted.go:6: a comparison reads SarifDocument.Version") || !strings.Contains(got[0], "list "+fresh+" in displayOnly") {
		t.Errorf("a decision read in a package on neither list gave %q; want it failed at its line", got)
	}
	listed := map[string]string{fresh: "prints a contract value and decides nothing on it"}
	if got := verdict(fresh, decides, deciding, listed); got != nil {
		t.Errorf("a package listed display only gave %q; want it passed", got)
	}
	if got := verdict(deciding[0], decides, deciding, map[string]string{deciding[0]: "a reason"}); len(got) != 1 || !strings.Contains(got[0], "is a deciding context") {
		t.Errorf("a row on a deciding context gave %q; want it refused", got)
	}
	idle, err := plantedPkg(t, w, "func f(d mc.SarifDocument) mc.SarifDocument {\n\treturn d\n}\n")
	if err != nil || idle.selects {
		t.Fatalf("a package that passes a contract value on without selecting a field gave %+v, %v; want it not selecting", idle, err)
	}
	if got := verdict(fresh, idle, deciding, listed); len(got) != 1 || !strings.Contains(got[0], "selects no contract field") {
		t.Errorf("a row for a package that selects no contract field gave %q; want it refused", got)
	}
	if got := strayRows(listed, []string{contractTypes}); len(got) != 1 || !strings.Contains(got[0], fresh) {
		t.Errorf("a row for a package that does not depend on the contract types gave %q; want it refused", got)
	}
	if got := strayRows(listed, []string{fresh}); got != nil {
		t.Errorf("a row for a reader gave %q; want none", got)
	}
}

// TestAPlantedReadOfADisplayOnlyFieldFails: each form of decision reading
// SarifDocument.Version or BundleDocument.Released, both display only, fails
// at its line, a flag read alone as well as in a comparison; reading
// Contract, which decides, and writing Version both pass (the clean
// controls). Planted failure: any form left out of the walk.
func TestAPlantedReadOfADisplayOnlyFieldFails(t *testing.T) {
	t.Parallel()
	w, _, err := newWalker(table(t).Module, nil)
	if err != nil {
		t.Fatal(err)
	}
	read := "SarifDocument.Version"
	for name, c := range map[string]struct {
		body string
		want []string
	}{
		"a display-only field in an if":                                          {"func f(d mc.SarifDocument) bool {\n\tif d.Version == \"2.1.0\" {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads " + read}},
		"a display-only field under a call in an if":                             {"func f(d mc.SarifDocument) bool {\n\tif len(d.Version) > 0 {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads " + read}},
		"a display-only field as a switch tag":                                   {"func f(d mc.SarifDocument) int {\n\tswitch d.Version {\n\tcase \"2.1.0\":\n\t\treturn 1\n\t}\n\treturn 0\n}\n", []string{"planted.go:6: a switch reads " + read}},
		"a display-only field in a case":                                         {"func f(d mc.SarifDocument) int {\n\tswitch {\n\tcase d.Version == \"\":\n\t\treturn 1\n\t}\n\treturn 0\n}\n", []string{"planted.go:7: a case reads " + read}},
		"a display-only field in a comparison outside any if":                    {"func f(d mc.SarifDocument) bool {\n\tok := d.Version != \"\"\n\treturn ok\n}\n", []string{"planted.go:6: a comparison reads " + read}},
		"a display-only field in a for condition":                                {"func f(d mc.SarifDocument) {\n\tfor d.Version == \"\" {\n\t\td.Version = \"x\"\n\t}\n}\n", []string{"planted.go:6: a for condition reads " + read}},
		"a display-only field as a map key":                                      {"func f(d mc.SarifDocument, m map[string]bool) bool {\n\treturn m[d.Version]\n}\n", []string{"planted.go:6: a map key reads " + read}},
		"a display-only field as a map literal's key":                            {"func f(d mc.SarifDocument) map[string]int {\n\treturn map[string]int{d.Version: 1}\n}\n", []string{"planted.go:6: a map key reads " + read}},
		"a display-only flag alone as an if condition":                           {"func f(d mc.BundleDocument) bool {\n\tif !d.Released {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads BundleDocument.Released"}},
		"a display-only flag alone as a for condition":                           {"func f(d mc.BundleDocument) {\n\tfor d.Released {\n\t\td.Released = false\n\t}\n}\n", []string{"planted.go:6: a for condition reads BundleDocument.Released"}},
		"a display-only flag alone as a case":                                    {"func f(d mc.BundleDocument) int {\n\tswitch {\n\tcase d.Released:\n\t\treturn 1\n\t}\n\treturn 0\n}\n", []string{"planted.go:7: a case reads BundleDocument.Released"}},
		"a display-only field in a type switch":                                  {"func f(e mc.HistoryEntry) int {\n\tswitch e.Before[\"k\"].(type) {\n\tcase error:\n\t\treturn 1\n\t}\n\treturn 0\n}\n", []string{"planted.go:6: a type switch reads HistoryEntry.Before"}},
		"a display-only field in a type switch that names its value":             {"func f(e mc.HistoryEntry) int {\n\tswitch v := e.Before[\"k\"].(type) {\n\tcase error:\n\t\treturn len(v.Error())\n\t}\n\treturn 0\n}\n", []string{"planted.go:6: a type switch reads HistoryEntry.Before"}},
		"a display-only field a range is driven by":                              {"func f(d mc.BundleDocument) int {\n\tn := 0\n\tfor range d.Terms {\n\t\tn++\n\t}\n\treturn n\n}\n", []string{"planted.go:7: a range reads BundleDocument.Terms"}},
		"a parent's rules ranged over, keyed by id and status, is clean":         {"func f(d mc.BundleDocument) map[mc.ID]bool {\n\tm := map[mc.ID]bool{}\n\tfor _, r := range d.Rules {\n\t\tm[r.ID] = r.Status == mc.StatusRetired\n\t}\n\treturn m\n}\n", nil},
		"a parent's rules ranged over, reading a display-only field":             {"func f(d mc.BundleDocument) int {\n\tn := 0\n\tfor _, r := range d.Rules {\n\t\tif r.Statement != \"\" {\n\t\t\tn++\n\t\t}\n\t}\n\treturn n\n}\n", []string{"planted.go:8: an if condition reads Rule.Statement"}},
		"a parent's bindings compared by the fields S70 holds them to, is clean": {"func f(d mc.BundleDocument) int {\n\tn := 0\n\tfor _, b := range d.Bindings {\n\t\tif b.Severity == mc.SeverityError && b.Rung > 0 && len(b.Options) == 0 {\n\t\t\tn++\n\t\t}\n\t}\n\treturn n\n}\n", nil},
		"a parent's binding's remedy read to decide":                             {"func f(b mc.Binding) bool {\n\treturn b.Remedy != \"\"\n}\n", []string{"planted.go:6: a comparison reads Binding.Remedy"}},
		"a parent rule's aliases ranged over":                                    {"func f(r mc.Rule) int {\n\tn := 0\n\tfor range r.Aliases {\n\t\tn++\n\t}\n\treturn n\n}\n", []string{"planted.go:7: a range reads Rule.Aliases"}},
		"a display-only flag and'ed outside any if":                              {"func f(d mc.BundleDocument, x bool) bool {\n\tok := d.Released && x\n\treturn ok\n}\n", []string{"planted.go:6: a logical operator reads BundleDocument.Released"}},
		"a display-only flag or'ed outside any if":                               {"func f(d mc.BundleDocument, x bool) bool {\n\treturn x || d.Released\n}\n", []string{"planted.go:6: a logical operator reads BundleDocument.Released"}},
		"a display-only flag negated outside any if":                             {"func f(d mc.BundleDocument) bool {\n\treturn !d.Released\n}\n", []string{"planted.go:6: a logical operator reads BundleDocument.Released"}},
		"a comparison and'ed is named as the comparison":                         {"func f(d mc.SarifDocument, x bool) bool {\n\treturn d.Version != \"\" && x\n}\n", []string{"planted.go:6: a comparison reads " + read}},
		"a display-only field read in an if's init":                              {"func f(d mc.SarifDocument) bool {\n\tif v := d.Version; v == \"\" {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads " + read}},
		"a display-only field asserted in an if's init":                          {"func f(e mc.HistoryEntry) bool {\n\tif _, ok := e.Before[\"k\"].(error); ok {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads HistoryEntry.Before"}},
		"a display-only field read in a switch's init":                           {"func f(d mc.SarifDocument) int {\n\tswitch v := d.Version; v {\n\tcase \"\":\n\t\treturn 1\n\t}\n\treturn 0\n}\n", []string{"planted.go:6: a switch reads " + read}},
		"a display-only field under a builtin in an if's init":                   {"func f(d mc.SarifDocument) bool {\n\tif n := len(d.Version); n > 0 {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads " + read}},
		"a display-only field in a sum in an if's init":                          {"func f(d mc.SarifDocument) bool {\n\tif v := d.Version + \"x\"; v == \"x\" {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads " + read}},
		"a display-only field read in a for's init":                              {"func f(d mc.SarifDocument) {\n\tfor v := d.Version; v == \"\"; v = \"x\" {\n\t}\n}\n", []string{"planted.go:6: a for condition reads " + read}},
		"a display-only field converted in an if's init":                         {"func f(d mc.SarifDocument) bool {\n\tif b := []byte(d.Version); b == nil {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads " + read}},
		"a display-only field sliced in a switch's init":                         {"func f(d mc.SarifDocument) int {\n\tswitch v := d.Version[1:]; v {\n\tcase \"\":\n\t\treturn 1\n\t}\n\treturn 0\n}\n", []string{"planted.go:6: a switch reads " + read}},
		"a display-only field indexed in an if's init":                           {"func f(c mc.BundleDocumentCoverage) bool {\n\tif v := c.RulesToBindings[0]; v == \"\" {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"planted.go:6: an if condition reads BundleDocumentCoverage.RulesToBindings"}},
		"a display-only field passed to a call in a for's init is clean":         {"func f(d mc.SarifDocument, g func(string) int) {\n\tfor n := g(d.Version); n > 0; n-- {\n\t}\n}\n", nil},
		"a display-only field passed to a call in an if's init is clean":         {"func f(d mc.SarifDocument, g func(string) error) bool {\n\tif err := g(d.Version); err != nil {\n\t\treturn true\n\t}\n\treturn false\n}\n", nil},
		"a deciding field in an if is clean":                                     {"func f(d mc.SarifDocument) bool {\n\tif d.Contract == \"1.0\" {\n\t\treturn true\n\t}\n\treturn false\n}\n", nil},
		"a display-only field written, not read, is clean":                       {"func f(d mc.SarifDocument) mc.SarifDocument {\n\td.Version = \"2.1.0\"\n\treturn d\n}\n", nil},
	} {
		got, err := plantedReads(t, w, c.body)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: reads %q, %v; want %q", name, got, err, c.want)
		}
	}
}

// TestWhatTheWalkCannotReadIsRefused: a deciding context that is neither a
// package nor planned, a plant that does not type-check, and a contract field
// with no mark are each refused, never passed over (the refusal cases).
// Planted failure: any of the three let through as clean.
func TestWhatTheWalkCannotReadIsRefused(t *testing.T) {
	t.Parallel()
	in, problems := scopeOf([]string{"internal/core/api", "internal/core/proof", "internal/core/renamed"}, []string{"internal/core/proof"}, func(p string) bool { return p == "internal/core/api" })
	if !slices.Equal(in, []string{"internal/core/api"}) || len(problems) != 1 || !strings.Contains(problems[0], "internal/core/renamed") {
		t.Errorf("scopeOf = %v, %v; want api read, proof waiting as planned, and renamed refused", in, problems)
	}
	w, _, err := newWalker(table(t).Module, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := plantedReads(t, w, "func f(d mc.SarifDocument) bool {\n\treturn d.NoSuchField == \"\"\n}\n"); err == nil {
		t.Errorf("a plant that does not type-check gave %q and no error, want it refused", got)
	}
	decides := map[string]bool{}
	src := "package p\n\ntype A struct {\n\t// Decides: b.\n\tB int\n\t// Display only.\n\tC int\n\t// A field.\n\tD int\n\t// Decides: e.\n\t// Display only.\n\tE int\n}\n"
	f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	unmarked := markDecl(f.Decls[0], decides)
	if !slices.Equal(unmarked, []string{"A.D", "A.E"}) || !decides["A.B"] || decides["A.C"] || decides["A.E"] {
		t.Errorf("unmarked %v, decides %v; want A.D and A.E unmarked, and only A.B deciding", unmarked, decides)
	}
}

// TestTheAdmissionOfTheStatusComparisonIsExactBothWays: ReadBundle's two reads
// of HistoryEntry.Status in splitStatus (the nil check and the comparison with
// after.status, gragas#2096) are the one admission, and it is exact: those two
// reads in that function pass (the clean control); a read of Status in an if in
// another function of the package, a third read inside splitStatus, and a read
// of another display-only field beside them are red; and the row left in with
// the comparison taken out of the code, or the function renamed, is stale.
// Planted failure: the admission widened to the package, to any number of
// reads, or to a field, or a row that can outlive its code.
func TestTheAdmissionOfTheStatusComparisonIsExactBothWays(t *testing.T) {
	t.Parallel()
	w, _, err := newWalker(table(t).Module, nil)
	if err != nil {
		t.Fatal(err)
	}
	pkg := w.module + "/internal/core/contract"
	split := "func splitStatus(h mc.HistoryEntry, moved string) bool {\n\tif h.Status == nil {\n\t\treturn false\n\t}\n\tif string(*h.Status) != moved {\n\t\treturn true\n\t}\n"
	for name, c := range map[string]struct {
		body         string
		reads, stale int
	}{
		"the two reads in their function":  {split + "\treturn false\n}\n", 0, 0},
		"a second function reading Status": {split + "\treturn false\n}\n\nfunc other(h mc.HistoryEntry) bool {\n\tif h.Status != nil {\n\t\treturn true\n\t}\n\treturn false\n}\n", 1, 0},
		"a third read in the function":     {split + "\tif h.Status != nil {\n\t\treturn true\n\t}\n\treturn false\n}\n", 3, 0},
		"another field beside them":        {split + "\tif h.Who != \"\" {\n\t\treturn true\n\t}\n\treturn false\n}\n", 1, 0},
		"the row with the comparison out":  {"func splitStatus(h mc.HistoryEntry, moved string) bool {\n\tif h.Status == nil {\n\t\treturn false\n\t}\n\treturn false\n}\n", 1, 1},
		"the row with its function gone":   {"func gone(h mc.HistoryEntry) bool { return false }\n", 0, 1},
	} {
		got, err := plantedIn(t, w, pkg, c.body)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(got.reads) != c.reads || len(got.stale) != c.stale {
			t.Errorf("%s: %d reads %q and %d stale %q, want %d and %d", name, len(got.reads), got.reads, len(got.stale), got.stale, c.reads, c.stale)
		}
	}
	elsewhere, err := plantedIn(t, w, w.module+"/internal/core/other", split+"\treturn false\n}\n")
	if err != nil || len(elsewhere.reads) != 2 {
		t.Errorf("the same two reads in another package gave %q, %v; want both red", elsewhere.reads, err)
	}
}
