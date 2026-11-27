package census

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// uses is what a body or a value names: the bare names it uses, sorted, its
// own locals left out, and the names it selects off an imported package.
type uses struct {
	refs  []string
	qrefs []qref
}

// qref is a name selected off an imported package: its import path, as the
// file wrote it, and the name.
type qref struct{ path, name string }

// decl is one exported declaration of a file: its key (<name>,
// <type>.<method> or <type>.<field>), its text with no body and no comment,
// and what the sources read off it.
type decl struct {
	key, text string
	uses
	// syscall is whether it names the file's import of syscall.
	syscall bool
	// doc is the first sentence of its doc comment that matched, or "".
	doc string
}

// symDecl is one name of a file that a body or a value can reach by it: a
// func, a var or a const of either case by its own name, or a method of
// either case as .<name>. A type is none: a struct laid out per system is no behaviour.
type symDecl struct {
	name string
	uses
	syscall bool
}

// file is what one Go file declares: its exported declarations and its
// reachable names.
type file struct {
	decls []decl
	syms  []symDecl
}

// reader parses each file of the GOROOT once.
type reader struct {
	src   fs.FS
	doc   *regexp.Regexp
	cache map[string]*file
}

func newReader(src fs.FS, doc *regexp.Regexp) *reader {
	return &reader{src: src, doc: doc, cache: map[string]*file{}}
}

// sym is a reachable name of a package, every port's declarations of it
// joined: whether it differs by its own declarations, and what it names.
type sym struct {
	own bool
	uses
}

// marks are the syms that differ, each to the name it reaches, as
// <import path>.<name>: "" when it differs by its own declarations.
type marks map[*sym]string

// stop is whether the reach passes no further than the name of the package
// at path.
type stop func(path, name string) bool

// runtimeHelper is whether name is one of runtime's unexported helpers, a
// func or a method: the reach does not follow them. Runtime's files branch
// on the system for the port's own mechanics, so its exported names are
// judged on their own declarations.
func runtimeHelper(path, name string) bool {
	return path == "runtime" && !token.IsExported(strings.TrimPrefix(name, "."))
}

// pkgData is one package as read: the files each port builds, its exported
// declarations per port, a file each is declared in, and its reachable names.
type pkgData struct {
	path    string
	common  map[string]bool
	perPort []map[string]decl
	inFile  map[string]string // key -> a file it is declared in, per-system first
	syms    map[string]*sym
	names   []string // the keys of syms, sorted
}

// pkg reads the package at path; files[i] are the files port i builds.
func (r *reader) pkg(path string, files [][]string) (*pkgData, error) {
	d := &pkgData{path: path, common: commonFiles(files), perPort: make([]map[string]decl, len(files)), inFile: map[string]string{}, syms: map[string]*sym{}}
	for i, names := range files {
		d.perPort[i] = map[string]decl{}
		for _, n := range names {
			if err := r.port(d, n, !d.common[n], d.perPort[i]); err != nil {
				return nil, err
			}
		}
	}
	for n := range d.syms {
		d.names = append(d.names, n)
	}
	slices.Sort(d.names)
	return d, nil
}

// port reads the file n of the package for one port: each of its
// declarations goes in decls, and in inFile when no file holds it yet or n is
// per-system (perSystem), so a per-system file is the one named. Each
// reachable name joins the package's, and differs from the start when a
// per-system file declares it or it names syscall.
func (r *reader) port(d *pkgData, n string, perSystem bool, decls map[string]decl) error {
	f, err := r.file(d.path, n)
	if err != nil {
		return err
	}
	for _, x := range f.decls {
		decls[x.key] = x
		if perSystem || d.inFile[x.key] == "" {
			d.inFile[x.key] = n
		}
	}
	for _, s := range f.syms {
		have := d.syms[s.name]
		if have == nil {
			have = &sym{}
			d.syms[s.name] = have
		}
		have.own = have.own || perSystem || s.syscall
		have.refs = joinSorted(have.refs, s.refs)
		for _, q := range s.qrefs {
			if !slices.Contains(have.qrefs, q) {
				have.qrefs = append(have.qrefs, q)
			}
		}
	}
	return nil
}

// joinSorted is a and b joined, sorted, each once.
func joinSorted(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	out := append(slices.Clone(a), b...)
	slices.Sort(out)
	return slices.Compact(out)
}

// world is every package read, by import path.
type world map[string]*pkgData

// resolve is the package an import path names: the path itself, or the copy
// std vendors under vendor/.
func (w world) resolve(path string) *pkgData {
	if d := w[path]; d != nil {
		return d
	}
	return w["vendor/"+path]
}

// via is the first name u reaches that m marks and stop passes, as
// <import path>.<name>, or "": a bare name of the package's own, then a name
// of another package. self is left out, so a body that calls itself does not
// name itself.
func (w world) via(d *pkgData, u uses, self string, m marks, stop stop) string {
	for _, r := range u.refs {
		if s := d.syms[r]; r != self && s != nil && !stop(d.path, r) {
			if _, ok := m[s]; ok {
				return d.path + "." + r
			}
		}
	}
	for _, q := range u.qrefs {
		if o := w.resolve(q.path); o != nil && !stop(o.path, q.name) {
			if s := o.syms[q.name]; s != nil {
				if _, ok := m[s]; ok {
					return o.path + "." + q.name
				}
			}
		}
	}
	return ""
}

// reach marks every name that differs by its own declarations, then, to a
// fixed point, every reachable name whose body or value names a marked one
// that stop passes, in its own package or in another std package, internal
// and vendored ones too.
func (w world) reach(order []string, stop stop) marks {
	m := marks{}
	for _, path := range order {
		for _, s := range w[path].syms {
			if s.own {
				m[s] = ""
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, path := range order {
			d := w[path]
			for _, n := range d.names {
				s := d.syms[n]
				if _, ok := m[s]; ok {
					continue
				}
				if v := w.via(d, s.uses, n, m, stop); v != "" {
					m[s], changed = v, true
				}
			}
		}
	}
	return m
}

// selfOf is the name a declaration's body calls itself by: a method's own
// name after its type's.
func selfOf(key string) string {
	if _, name, ok := strings.CutLast(key, "."); ok {
		return "." + name
	}
	return key
}

// report is the package's exported identifiers that differ by system, the
// reach over found, and how many exported identifiers it declares for any
// port.
func (w world) report(d *pkgData, found marks) ([]Ident, int) {
	keys := unionKeys(d.perPort)
	var out []Ident
	for _, k := range keys {
		self := selfOf(k)
		why, via := reasons(k, d.perPort, !d.common[d.inFile[k]], func(u uses) string { return w.via(d, u, self, found, runtimeHelper) })
		if len(why) > 0 {
			out = append(out, Ident{Name: d.path + "." + k, Why: why, Doc: docOf(k, d.perPort), Via: via})
		}
	}
	return out, len(keys)
}

// docOf is the first port's doc sentence for key that matched, or "".
func docOf(key string, perPort []map[string]decl) string {
	for _, m := range perPort {
		if d := m[key]; d.doc != "" {
			return d.doc
		}
	}
	return ""
}

// commonFiles are the files every port builds.
func commonFiles(files [][]string) map[string]bool {
	out := map[string]bool{}
	if len(files) == 0 {
		return out
	}
	for _, n := range files[0] {
		every := true
		for _, other := range files[1:] {
			every = every && slices.Contains(other, n)
		}
		out[n] = every
	}
	return out
}

// unionKeys are the keys any port declares, sorted.
func unionKeys(perPort []map[string]decl) []string {
	var out []string
	for _, m := range perPort {
		for k := range m {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// reasons are why the identifier key differs, in the order of the Why
// constants, and the name it reaches when WhyReach is one: none when it does
// not differ. via answers what a declaration's uses reach.
func reasons(key string, perPort []map[string]decl, perSystemFile bool, via func(uses) string) ([]string, string) {
	var why []string
	add := func(ok bool, w string) {
		if ok && !slices.Contains(why, w) {
			why = append(why, w)
		}
	}
	text, reached := "", ""
	for i, m := range perPort {
		d, ok := m[key]
		add(!ok, WhyPartial)
		if !ok {
			continue
		}
		add(i > 0 && text != "" && d.text != text, WhyDefined)
		if text == "" {
			text = d.text
		}
		if !perSystemFile && reached == "" {
			reached = via(d.uses)
		}
		add(d.syscall, WhySyscall)
		add(d.doc != "", WhyDoc)
	}
	add(perSystemFile, WhyFile)
	add(reached != "", WhyReach)
	order := []string{WhyPartial, WhyDefined, WhyFile, WhyReach, WhySyscall, WhyDoc}
	slices.SortFunc(why, func(a, b string) int { return slices.Index(order, a) - slices.Index(order, b) })
	return why, reached
}

// file parses src/<path>/<name> once.
func (r *reader) file(path, name string) (*file, error) {
	at := "src/" + path + "/" + name
	if f, ok := r.cache[at]; ok {
		return f, nil
	}
	data, err := fs.ReadFile(r.src, at)
	if err != nil {
		return nil, fmt.Errorf("the GOROOT's %s: %w", at, err)
	}
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, at, data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("the GOROOT's %s does not parse: %w", at, err)
	}
	p := &fileParse{fset: fset, sys: importNames(af, "syscall"), imports: importPaths(af), doc: r.doc, out: &file{}}
	for _, d := range af.Decls {
		p.decl(d)
	}
	r.cache[at] = p.out
	return p.out, nil
}

// fileParse is the state of reading one file's declarations.
type fileParse struct {
	fset    *token.FileSet
	sys     map[string]bool
	imports map[string]string // the name a file gives an import -> its path
	doc     *regexp.Regexp
	out     *file
}

func (p *fileParse) decl(d ast.Decl) {
	switch d := d.(type) {
	case *ast.FuncDecl:
		p.fn(d)
	case *ast.GenDecl:
		for _, s := range d.Specs {
			p.spec(d, s)
		}
	}
}

func (p *fileParse) fn(d *ast.FuncDecl) {
	key, exported := d.Name.Name, d.Name.IsExported()
	var u uses
	if d.Body != nil {
		u = p.usesOf(d.Body, locals(d))
	}
	sys := p.reaches(d)
	if d.Recv != nil && len(d.Recv.List) > 0 {
		// A method is reached by its name as a selector, of any type of the
		// package: p.kill() names .kill and f.Close() .Close. The dot keeps it
		// apart from the package's own names, so time's dataIO.read is not its
		// per-system func read, nor its dataIO.byte the type byte. A selector
		// off a value of another package's type is not followed: that wants
		// the types, which the census does not check.
		p.out.syms = append(p.out.syms, symDecl{name: "." + key, uses: u, syscall: sys})
		recv, ok := receiver(d.Recv.List[0].Type)
		if !exported || !ok || !ast.IsExported(recv) {
			return // a method of an unexported type is no API of the package
		}
		key = recv + "." + key
	} else {
		p.out.syms = append(p.out.syms, symDecl{name: key, uses: u, syscall: sys})
		if !exported {
			return
		}
	}
	bare := *d
	bare.Body, bare.Doc = nil, nil
	p.out.decls = append(p.out.decls, decl{key: key, text: p.render(&bare), uses: u, syscall: sys, doc: p.docs(d.Doc)})
}

// locals are the names a func declares for itself: its receiver, its
// parameters and results, and what its body defines with := or var, which
// shadow a package's names of the same spelling.
func locals(d *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	for _, id := range fieldNames(d.Recv, d.Type.Params, d.Type.Results) {
		out[id.Name] = true
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		for _, id := range defined(n) {
			out[id.Name] = true
		}
		return true
	})
	return out
}

// defined is the names one node of a body defines: a func literal's
// parameters and results, the left of a := or a range :=, and a var's names.
func defined(n ast.Node) []*ast.Ident {
	switch x := n.(type) {
	case *ast.FuncLit:
		return fieldNames(x.Type.Params, x.Type.Results)
	case *ast.AssignStmt:
		if x.Tok == token.DEFINE {
			return idents(x.Lhs...)
		}
	case *ast.RangeStmt:
		if x.Tok == token.DEFINE {
			return idents(x.Key, x.Value)
		}
	case *ast.ValueSpec:
		return x.Names
	}
	return nil
}

// fieldNames is the names of the fields of each list, none for no list.
func fieldNames(lists ...*ast.FieldList) []*ast.Ident {
	var out []*ast.Ident
	for _, fl := range lists {
		if fl == nil {
			continue
		}
		for _, f := range fl.List {
			out = append(out, f.Names...)
		}
	}
	return out
}

// idents is the expressions that are bare names.
func idents(es ...ast.Expr) []*ast.Ident {
	var out []*ast.Ident
	for _, e := range es {
		if id, ok := e.(*ast.Ident); ok {
			out = append(out, id)
		}
	}
	return out
}

// usesOf is what n names, the names in skip left out of its bare names: a
// selector off an imported package is that package's name, and any other
// selector a method's, as .name; never a bare one.
func (p *fileParse) usesOf(n ast.Node, skip map[string]bool) uses {
	refs := map[string]bool{}
	var u uses
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && !skip[id.Name] {
				if path, ok := p.imports[id.Name]; ok {
					if q := (qref{path, x.Sel.Name}); !slices.Contains(u.qrefs, q) {
						u.qrefs = append(u.qrefs, q)
					}
					return false
				}
			}
			refs["."+x.Sel.Name] = true
			ast.Inspect(x.X, visit)
			return false
		case *ast.Ident:
			if !skip[x.Name] {
				refs[x.Name] = true
			}
		}
		return true
	}
	ast.Inspect(n, visit)
	for r := range refs {
		u.refs = append(u.refs, r)
	}
	slices.Sort(u.refs)
	return u
}

// receiver is the name of a method's receiver type, star and type
// arguments taken off.
func receiver(t ast.Expr) (string, bool) {
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch x := t.(type) {
	case *ast.IndexExpr:
		t = x.X
	case *ast.IndexListExpr:
		t = x.X
	}
	id, ok := t.(*ast.Ident)
	if !ok {
		return "", false
	}
	return id.Name, true
}

func (p *fileParse) spec(d *ast.GenDecl, s ast.Spec) {
	switch s := s.(type) {
	case *ast.TypeSpec:
		p.typeSpec(d, s)
	case *ast.ValueSpec:
		p.valueSpec(d, s)
	}
}

func (p *fileParse) typeSpec(d *ast.GenDecl, s *ast.TypeSpec) {
	if !s.Name.IsExported() {
		return
	}
	doc := s.Doc
	if doc == nil {
		doc = d.Doc
	}
	bare := *s
	bare.Doc, bare.Comment = nil, nil
	p.out.decls = append(p.out.decls, decl{key: s.Name.Name, text: p.render(&bare), syscall: p.reaches(s), doc: p.docs(doc)})
	st, ok := s.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.IsExported() {
				p.out.decls = append(p.out.decls, decl{key: s.Name.Name + "." + n.Name, text: p.render(f.Type), syscall: p.reaches(f.Type), doc: p.docs(f.Doc)})
			}
		}
	}
}

func (p *fileParse) valueSpec(d *ast.GenDecl, s *ast.ValueSpec) {
	doc := s.Doc
	if doc == nil {
		doc = d.Doc
	}
	sys := p.reaches(s)
	for i, n := range s.Names {
		var u uses
		if i < len(s.Values) {
			u = p.usesOf(s.Values[i], nil)
		}
		p.out.syms = append(p.out.syms, symDecl{name: n.Name, uses: u, syscall: sys})
		if !n.IsExported() {
			continue
		}
		text := d.Tok.String() + " " + n.Name
		if s.Type != nil {
			text += " " + p.render(s.Type)
		}
		switch {
		case i < len(s.Values):
			text += " = " + p.render(s.Values[i])
		case d.Tok == token.CONST:
			text += " = <implicit " + strconv.Itoa(i) + ">"
		}
		p.out.decls = append(p.out.decls, decl{key: n.Name, text: text, uses: u, syscall: sys, doc: p.docs(doc)})
	}
}

// render is a node's source on one line, comments left out.
func (p *fileParse) render(n ast.Node) string {
	var b bytes.Buffer
	if err := format.Node(&b, p.fset, n); err != nil {
		return "<" + err.Error() + ">"
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// reaches is whether n names the file's import of syscall.
func (p *fileParse) reaches(n ast.Node) bool {
	if len(p.sys) == 0 {
		return false
	}
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := se.X.(*ast.Ident); ok && p.sys[id.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

// sentence is one sentence of a doc comment.
var sentence = regexp.MustCompile(`[^.!?]*[.!?](\s|$)`)

// docs is the first sentence of the doc comment cg that matches, or "".
func (p *fileParse) docs(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	t := strings.Join(strings.Fields(cg.Text()), " ") + " "
	for _, s := range sentence.FindAllString(t, -1) {
		if p.doc.MatchString(s) {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// importNames are the names a file gives the package at importPath.
func importNames(f *ast.File, importPath string) map[string]bool {
	names := map[string]bool{}
	for name, path := range importPaths(f) {
		if path == importPath {
			names[name] = true
		}
	}
	return names
}

// importPaths are the names a file gives its imports, each to its path: the
// path's last element when the import names none. A blank or dot import
// gives no name.
func importPaths(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		switch {
		case im.Name == nil:
			out[p[strings.LastIndex(p, "/")+1:]] = p
		case im.Name.Name != "_" && im.Name.Name != ".":
			out[im.Name.Name] = p
		}
	}
	return out
}

// docPhrases are the words of a doc sentence that say behaviour depends on
// the system without naming one. Each GOOS of the dist list joins them.
func docPhrases() []string {
	return []string{"operating system", "operating systems", "system-dependent", "system dependent", "system-specific", "on some systems", "os-specific", "platform-specific", "platform-dependent", "GOOS"}
}

// terms are the GOOS values and docPhrases, sorted.
func terms(systems []string) []string {
	out := append(slices.Clone(systems), docPhrases()...)
	slices.Sort(out)
	return slices.Compact(out)
}

// letterDigit is the place between a letter and a digit, where a doc may
// put a space the GOOS does not hold.
var letterDigit = regexp.MustCompile(`([a-z])([0-9])`)

// docMatcher matches a sentence that holds a term as words, in any case.
func docMatcher(terms []string) *regexp.Regexp {
	alts := make([]string, 0, len(terms))
	for _, t := range terms {
		q := regexp.QuoteMeta(strings.ToLower(t))
		alts = append(alts, letterDigit.ReplaceAllString(q, "$1 ?$2"))
	}
	return regexp.MustCompile(`(?i)\b(` + strings.Join(alts, "|") + `)\b`)
}
