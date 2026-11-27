package link

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// CheckerSchemaName is the schema a checker-code declaration is held to,
// under the Schema port's root.
const CheckerSchemaName = "v3/checker.schema.json"

// checkerPlaceRE is where a declaration stands: checkers/<name>/checker.yaml,
// relative to the rulebook's folder.
var checkerPlaceRE = regexp.MustCompile(`^` + model.CheckersDir + `/([a-z0-9]+(?:-[a-z0-9]+)*)/` + regexp.QuoteMeta(model.CheckerFile) + `$`)

// ReadDeclaration reads the one checker-code declaration data, which stands at
// path below the rulebook's folder, and checks it against the schema the
// Schema port s holds. It answers the declaration, or every reason it
// is refused, at most twenty, and never both: a document it cannot read is
// refused, never guessed at, since it is the rulebook's own file and a
// consumer's tools trust what it declares.
func ReadDeclaration(s port.Schema, path string, data []byte) (model.Checker, []Refusal) {
	r := &reader{path: path, kind: "a checker declaration", kinds: "checker declarations"}
	if len(data) > MaxDocument {
		r.refuse(0, CheckSize, "the document is %d bytes, over the cap of %d", len(data), MaxDocument)
		return model.Checker{}, r.out
	}
	place := checkerPlaceRE.FindStringSubmatch(path)
	if place == nil {
		r.refuse(0, CheckPlace, "a checker declaration stands at %s/<name>/%s, the name a lowercase slug", model.CheckersDir, model.CheckerFile)
		return model.Checker{}, r.out
	}
	v, err := ordered.ParseStrict(data)
	if err != nil {
		r.yaml(err)
		return model.Checker{}, r.out
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		r.refuse(1, CheckShape, "the document must be a mapping of the declaration's fields")
		return model.Checker{}, r.out
	}
	c := r.checker(m)
	if len(r.out) == 0 && c.Name != place[1] {
		r.refuse(m.KeyLine("name"), CheckPlace, "the name is %s, and the file stands in the folder %s", c.Name, place[1])
	}
	if len(r.out) == 0 {
		r.schema(s, CheckerSchemaName, v)
	}
	if len(r.out) > 0 {
		return model.Checker{}, r.out
	}
	return c, nil
}

// checker reads the declaration's fields.
func (r *reader) checker(m *ordered.Map) model.Checker {
	r.keys(m, "", "format", "name", "tool", "publisher", "tests", "licence", "from")
	if !r.format(m) {
		return model.Checker{}
	}
	c := model.Checker{
		Format: model.FormatVersion,
		Name:   r.word(m, "", "name", slugForm, slugSays, true),
		Tool:   r.word(m, "", "tool", slugForm, slugSays, true),
		Tests:  r.relative(m, "", "tests", false),
		From:   r.word(m, "", "from", slugForm, slugSays, false),
	}
	c.Publisher = r.plain(m, "publisher", true)
	c.Licence = r.plain(m, "licence", false)
	return c
}

// unseen is a character a listing cannot show as written: a control
// character (Cc), a format character (Cf) such as a zero-width space, a word
// joiner or a right-to-left override, which prints as nothing or turns the
// text around, and the line and paragraph separators, which start another
// line.
func unseen(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029'
}

// visible is a character a listing shows as something: graphic, no space, no
// combining mark, and none of the letters and symbols that print as nothing,
// the Hangul fillers and the blank braille pattern. A text with none prints
// as nothing, so it names no one.
func visible(r rune) bool {
	switch r {
	case 0x115F, 0x1160, 0x3164, 0xFFA0, 0x2800:
		return false
	}
	return unicode.IsGraphic(r) && !unicode.IsSpace(r) && !unicode.In(r, unicode.Mn, unicode.Me)
}

// plain reads a text field of a declaration and holds it to what the schema
// holds and a listing needs: present, it is not empty, since an empty
// publisher or licence names no one; and it holds no character a listing
// cannot show as written, which it would print as something else, a newline
// as another row, or as nothing, so that a publisher of one zero-width space
// names no one. The Go reader holds it whatever the Schema port does.
func (r *reader) plain(m *ordered.Map, k string, required bool) string {
	s, ok := r.text(m, "", k, maxText, required)
	switch {
	case !ok:
		return ""
	case strings.ContainsFunc(s, unseen):
		r.refuse(m.KeyLine(k), CheckShape, "%s holds a control character, or one that prints as nothing or as another line, which a listing would print as something else", k)
	case !strings.ContainsFunc(s, visible):
		r.refuse(m.KeyLine(k), CheckShape, "%s is empty or prints as nothing, and %s names it or leaves it out", k, r.an())
	default:
		return s
	}
	return ""
}

// Imported is the checker code a fetched rulebook carries: the parent's, or an
// import's under its rulebook name. Nothing fetches one yet; the fetch will
// fill it, and until then a test hands it in.
type Imported struct {
	Rulebook string
	Code     []model.CheckerArtifact
}

// CheckerCode is what the checker-code findings read: the listing of the
// rulebook's checkers/ folder, every file below it relative to the rulebook's
// folder; the content of each file that stands where a declaration does; the
// book whose bindings name them; and the checker code of the parent and the
// imports handed in, none until a fetch hands them in.
type CheckerCode struct {
	Listing []string
	Files   []model.File
	Book    model.Book
	Parent  *Imported
	Imports []Imported
}

// Checkers reads the declarations of in and holds the rulebook's checker code
// to them (S23, S3). It answers the declarations it read, the rulebook's
// own and an import's by reference, and the findings:
//   - checker code with no declaration: a file under checkers/<name>/ where no
//     declaration stands, or a file straight under checkers/;
//   - a declaration that is refused, at its line;
//   - the rulebook's own artifact with no tests, or with a tests path that
//     holds nothing; one declared by reference that holds code;
//   - an import's checker code the rulebook does not declare by its name and
//     publisher, and a declaration from a rulebook it neither extends nor
//     imports or one that does not carry it;
//   - a name more than one carries, as one finding: the parent and an
//     import, the rulebook's own artifact and an import's, or two imports;
//   - an overlay's declaration by a name its parent carries, its own or one
//     from an import, unless it is from the parent itself;
//   - a binding that names no declaration, or one of another tool.
//
// No fix declares anything by reference, from an import or from the parent,
// where the rulebook's own code sits, declared, undeclared or under a refused
// declaration: that code is renamed, or the others' artifacts are dropped.
// No two fixes send the author to each other, and each way out of each
// finding the closure test plants, followed, leaves no finding behind it.
//
// It reads no file and calls no port but the schema: nothing is loaded or
// run, and no digest is taken (that is the release's).
func Checkers(s port.Schema, in CheckerCode) ([]model.Checker, []model.Finding) {
	var decls []model.Checker
	var out []model.Finding
	for _, f := range in.Files {
		c, refused := ReadDeclaration(s, f.Path, f.Data)
		for _, rf := range refused {
			out = append(out, finding(rf.Path, rf.Line, string(rf.Check)+": "+rf.Msg, "write the declaration as docs/format/checkers.md says"))
		}
		if len(refused) == 0 {
			decls = append(decls, c)
		}
	}
	t := folders{listing: in.Listing, files: in.Files}
	byImport := func(name string) bool {
		return slices.ContainsFunc(in.Imports, func(im Imported) bool { _, ok := carries(&im, name); return ok })
	}
	out = append(out, undeclared(t, in.Parent, byImport)...)
	for _, c := range decls {
		out = append(out, contents(c, in.Listing)...)
	}
	out = append(out, imported(in.Book, decls, in.Parent, in.Imports, t)...)
	out = append(out, overlaid(in.Book, decls, in.Parent)...)
	out = append(out, bound(in.Book, decls, in.Parent, t, byImport)...)
	return decls, out
}

// declarationPath is where the declaration of the artifact name stands.
func declarationPath(name string) string {
	return model.CheckersDir + "/" + name + "/" + model.CheckerFile
}

// finding is a finding about file at line, with its fix.
func finding(file string, line int, msg, fix string) model.Finding {
	entity := file
	if line > 0 {
		entity = fmt.Sprintf("%s:%d", file, line)
	}
	return model.Finding{Entity: entity, Msg: msg, Fix: fix, File: file, Line: line}
}

// folders is what the rulebook's checkers folder holds: the listing, and the
// declarations handed in, read or refused.
type folders struct {
	listing []string
	files   []model.File
}

// written is whether a declaration of name was handed in, whether or not it
// was read.
func (t folders) written(name string) bool {
	return slices.ContainsFunc(t.files, func(f model.File) bool { return f.Path == declarationPath(name) })
}

// code is whether the folder of name holds anything beside its declaration.
func (t folders) code(name string) bool {
	dir := model.CheckersDir + "/" + name + "/"
	return slices.ContainsFunc(t.listing, func(p string) bool { return strings.HasPrefix(p, dir) && p != declarationPath(name) })
}

// renamed is the way out that keeps the rulebook's own code under a name of
// its own.
const renamed = "move it to " + model.CheckersDir + "/<new>/ with its declaration and tests, and bind your rules to the new name"

// undeclared finds checker code with no declaration: each folder under
// checkers/ whose declaration was not handed in, and each file straight under
// checkers/, named once each. Code under a name an import carries is held's
// to report, as the shared name it is; code under a name the parent carries
// is the overlay's own, so its one way out is a name of its own, never a
// declaration from the parent over it.
func undeclared(t folders, parent *Imported, byImport func(string) bool) []model.Finding {
	var out []model.Finding
	seen := map[string]bool{}
	for _, p := range t.listing {
		rest, ok := strings.CutPrefix(p, model.CheckersDir+"/")
		if !ok {
			continue
		}
		name, _, inFolder := strings.Cut(rest, "/")
		if !inFolder {
			out = append(out, finding(p, 0, "checker code stands straight under "+model.CheckersDir+"/, where no declaration can name it, so a consumer's tools would load code the rulebook does not declare",
				"move it into "+model.CheckersDir+"/<name>/ beside its declaration "+model.CheckerFile))
			continue
		}
		if seen[name] || t.written(name) || byImport(name) {
			continue
		}
		seen[name] = true
		msg := "checker code in " + model.CheckersDir + "/" + name + "/ has no declaration, so a consumer's tools would load code the rulebook does not declare"
		fix := "declare it in " + declarationPath(name) + " by its tool, publisher and tests (docs/format/checkers.md)"
		if _, ok := carries(parent, name); ok {
			msg += ", under the name of the parent " + parent.Rulebook + "'s checker code"
			fix = renamed
		}
		out = append(out, finding(model.CheckersDir+"/"+name, 0, msg, fix))
	}
	return out
}

// contents holds a declaration to its folder: the rulebook's own artifact
// names its tests, and they are in the folder; an artifact declared by
// reference, a parent's or an import's, holds no code of its own, since that
// rulebook publishes it.
func contents(c model.Checker, listing []string) []model.Finding {
	dir := model.CheckersDir + "/" + c.Name + "/"
	file := declarationPath(c.Name)
	var own []string
	for _, p := range listing {
		if strings.HasPrefix(p, dir) && p != file {
			own = append(own, p)
		}
	}
	switch {
	case c.From != "" && len(own) > 0:
		return []model.Finding{finding(file, 0, c.Name+" declares "+c.From+"'s artifact by reference and its folder holds code of its own, "+own[0]+", which no one publishes",
			"if the code is yours, "+renamed+", or if it is "+c.From+"'s, keep only "+model.CheckerFile+" in "+dir)}
	case c.From != "":
		return nil
	case c.Tests == "":
		return []model.Finding{finding(file, 0, c.Name+" declares no tests, and the rulebook's checker code has its own tests (S23)",
			"name the folder or file of its tests in tests:, relative to "+dir)}
	}
	tests := dir + path.Clean(c.Tests)
	if !slices.ContainsFunc(own, func(p string) bool { return p == tests || strings.HasPrefix(p, tests+"/") }) {
		return []model.Finding{finding(file, 0, c.Name+"'s tests are at "+c.Tests+", which holds nothing in "+dir,
			"put its tests there, or name where they are")}
	}
	return nil
}

// carries is the artifact by name of the code r carries, if it does.
func carries(r *Imported, name string) (model.CheckerArtifact, bool) {
	if r == nil {
		return model.CheckerArtifact{}, false
	}
	i := slices.IndexFunc(r.Code, func(a model.CheckerArtifact) bool { return a.Name == name })
	if i < 0 {
		return model.CheckerArtifact{}, false
	}
	return r.Code[i], true
}

// fromParent is whether c declares by reference the parent's own artifact of
// its name, by the parent's publisher: a reference, never a shadow.
func fromParent(c model.Checker, parent *Imported) bool {
	a, ok := carries(parent, c.Name)
	return ok && c.From == parent.Rulebook && c.Publisher == a.Publisher
}

// parentName is the rulebook the book extends, "" for a base.
func parentName(b model.Book) string {
	if b.Identity.Extends == nil {
		return ""
	}
	return b.Identity.Extends.Rulebook
}

// imported holds the rulebook's imports' checker code to its declarations,
// and its declarations by reference to where they are from (S3). The parent
// and the imports are those handed in.
func imported(b model.Book, decls []model.Checker, parent *Imported, imports []Imported, t folders) []model.Finding {
	return append(brought(decls, parent, imports, t), sourced(b, decls, parent, imports)...)
}

// brought holds what each import handed in carries to the rulebook's
// declarations. A rulebook holds one artifact by a name, so each name an
// import carries is read once, with every other that carries it: the parent,
// the other imports, and the rulebook itself through its own declaration. An
// import's artifact the rulebook does not declare by its name and publisher
// is a finding, since an import brings checker code only when the importer
// declares it (S3). A name more than one of them carries is one finding of its
// own, whose fix keeps one: the parent's, through a declaration from the
// parent; the rulebook's own, renamed; or one import's, the others dropped.
// Where the rulebook's own code sits in the name's folder, declared, not
// declared, or under a declaration that is refused, no way out declares
// anything by reference there: the rulebook's own is renamed or the others'
// are dropped.
func brought(decls []model.Checker, parent *Imported, imports []Imported, t folders) []model.Finding {
	var out []model.Finding
	seen := map[string]bool{}
	for _, im := range imports {
		for _, a := range im.Code {
			if seen[a.Name] {
				continue
			}
			seen[a.Name] = true
			var by []carrier
			for _, other := range imports {
				if x, ok := carries(&other, a.Name); ok {
					by = append(by, carrier{other.Rulebook, x})
				}
			}
			out = append(out, heldFindings(a.Name, decls, parent, by, t)...)
		}
	}
	return out
}

// carrier is an import that carries an artifact, and the artifact.
type carrier struct {
	rulebook string
	artifact model.CheckerArtifact
}

// names is the carriers' rulebook names, joined for a sentence.
func names(by []carrier) string {
	out := make([]string, len(by))
	for i, c := range by {
		out[i] = c.rulebook
	}
	switch len(out) {
	case 1:
		return out[0]
	case 2:
		return out[0] + " and " + out[1]
	}
	return strings.Join(out[:len(out)-1], ", ") + " and " + out[len(out)-1]
}

// theirs is the artifacts of the carriers drop, for a fix line.
func theirs(drop []carrier) string {
	if len(drop) == 1 {
		return drop[0].rulebook + "'s artifact"
	}
	return "the artifacts of " + names(drop)
}

// heldFindings is the finding, if any, about the name that the imports by carry,
// against the parent and the rulebook's declaration of that name.
func heldFindings(name string, decls []model.Checker, parent *Imported, by []carrier, t folders) []model.Finding {
	i := slices.IndexFunc(decls, func(c model.Checker) bool { return c.Name == name })
	var d *model.Checker
	if i >= 0 {
		d = &decls[i]
	}
	p, byParent := carries(parent, name)
	if f, ok := ownHeld(name, d, parent, by, t); ok {
		return []model.Finding{f}
	}
	if byParent {
		if d != nil && fromParent(*d, parent) {
			return nil
		}
		return []model.Finding{finding(model.CheckersDir, 0, name+" is carried by the parent "+parent.Rulebook+" and by "+names(by)+"; a rulebook can hold one",
			"bind to the parent's (declare it from the parent in "+declarationPath(name)+", with from: "+parent.Rulebook+" and publisher: "+p.Publisher+
				"), or drop "+theirs(by)+" and any declaration of it")}
	}
	k := -1
	if d != nil {
		k = slices.IndexFunc(by, func(c carrier) bool { return c.rulebook == d.From && c.artifact.Publisher == d.Publisher })
	}
	switch {
	case k >= 0 && len(by) == 1:
		return nil
	case k >= 0:
		rest := slices.Delete(slices.Clone(by), k, k+1)
		return []model.Finding{finding(declarationPath(name), 0, name+" is declared from "+by[k].rulebook+" and is also "+theirs(rest)+"; a rulebook can hold one",
			"drop "+theirs(rest))}
	case len(by) == 1:
		a := by[0]
		return []model.Finding{finding(model.CheckersDir, 0, "the import "+a.rulebook+" brings the checker code "+name+" by "+a.artifact.Publisher+", and this rulebook does not declare it, so it is not a dependency a consumer may load (S3)",
			"declare it by reference in "+declarationPath(name)+" with from: "+a.rulebook+" and publisher: "+a.artifact.Publisher)}
	}
	return []model.Finding{finding(model.CheckersDir, 0, name+" is carried by "+names(by)+"; a rulebook can hold one",
		"declare it by reference in "+declarationPath(name)+" from one of them, with its publisher there, and drop the others' artifacts")}
}

// ownHeld is the shared-name finding where the rulebook's own code holds the
// name: its own declaration is read, or its folder holds code with no
// declaration read, none handed in or one refused. Its ways out keep the
// rulebook's code, renamed, with the other's taken in the folder it leaves,
// or drop the others' artifacts; where the declaration is refused, it is
// written first. None declares anything by reference over the rulebook's
// code. An overlay never keeps its own
// code under its parent's name, so there it is renamed alone.
func ownHeld(name string, d *model.Checker, parent *Imported, by []carrier, t folders) (model.Finding, bool) {
	p, byParent := carries(parent, name)
	declared := d != nil && d.From == ""
	if !declared && (d != nil || !t.code(name)) {
		return model.Finding{}, false
	}
	also, keep := by[0].rulebook+"'s", "declare "+by[0].rulebook+"'s by reference there, with from: "+by[0].rulebook+" and publisher: "+by[0].artifact.Publisher
	if len(by) > 1 {
		also, keep = "carried by "+names(by), "declare one of theirs by reference there and drop the others'"
	}
	drop := ", or drop " + theirs(by)
	if byParent {
		also = "carried by the parent " + parent.Rulebook + " and by " + names(by)
		keep = "declare the parent's from the parent there, with from: " + parent.Rulebook + " and publisher: " + p.Publisher + ", and drop " + theirs(by)
		drop = ""
	}
	msg, first := name+" is this rulebook's own and also "+also, ""
	switch {
	case declared:
	case t.written(name):
		msg = model.CheckersDir + "/" + name + "/ holds this rulebook's code, whose declaration is refused, and " + name + " is also " + also
		first = "write its declaration as docs/format/checkers.md says, then "
	default:
		msg = model.CheckersDir + "/" + name + "/ holds this rulebook's code with no declaration, and " + name + " is also " + also
		if drop != "" {
			drop += " and declare yours in " + declarationPath(name) + " by its tool, publisher and tests"
		}
	}
	return finding(declarationPath(name), 0, msg+"; a rulebook can hold one", first+"rename yours ("+renamed+") and "+keep+drop), true
}

// sourced holds each declaration by reference to where it is from: a
// rulebook the book neither extends nor imports is a finding, and so is a
// parent or an import handed in that carries no such artifact by that
// publisher.
func sourced(b model.Book, decls []model.Checker, parent *Imported, imports []Imported) []model.Finding {
	sources := map[string]bool{}
	for _, im := range b.Identity.Imports {
		sources[im.Rulebook] = true
	}
	if p := parentName(b); p != "" {
		sources[p] = true
	}
	handed := map[string][]model.CheckerArtifact{}
	if parent != nil {
		handed[parent.Rulebook] = parent.Code
	}
	for _, im := range imports {
		handed[im.Rulebook] = im.Code
	}
	var out []model.Finding
	for _, c := range decls {
		if c.From == "" {
			continue
		}
		code, wasHanded := handed[c.From]
		switch {
		case !sources[c.From]:
			// Where an import handed in carries the name, the artifact is
			// its, and declaring it as the rulebook's own would be the shared
			// name.
			fix := "import " + c.From + " in rulebook.yaml, or declare the artifact as this rulebook's own"
			for _, im := range imports {
				if a, ok := carries(&im, c.Name); ok {
					fix = "import " + c.From + " in rulebook.yaml, or declare it from " + im.Rulebook + ", with publisher: " + a.Publisher
					break
				}
			}
			out = append(out, finding(declarationPath(c.Name), 0, c.Name+" is declared from "+c.From+", which the rulebook neither extends nor imports", fix))
		case wasHanded && !slices.ContainsFunc(code, func(a model.CheckerArtifact) bool { return a.Name == c.Name && a.Publisher == c.Publisher }):
			out = append(out, finding(declarationPath(c.Name), 0, c.Name+" by "+c.Publisher+" is declared from "+c.From+", which carries no such artifact by that publisher",
				"declare its artifact by the name and publisher it has there"))
		}
	}
	return out
}

// overlaid holds an overlay's declarations to its parent's names: a name the
// parent carries is the parent's, which the parent's release lists and the
// overlay republishes none of (S23). So a declaration under that name, the
// overlay's own or one by reference from an import, is the shadow and a
// finding; one by reference from the parent itself, by the parent's
// publisher, is the parent's artifact and none. The parent's code is read
// once a fetch hands it in.
func overlaid(b model.Book, decls []model.Checker, parent *Imported) []model.Finding {
	if b.Identity.Extends == nil || parent == nil {
		return nil
	}
	var out []model.Finding
	for _, c := range decls {
		a, ok := carries(parent, c.Name)
		if !ok || fromParent(c, parent) {
			continue
		}
		// The overlay's own code is renamed, never declared over from the
		// parent; a reference is declared
		// from the parent instead.
		fix := "declare it from the parent, with from: " + parent.Rulebook + " and publisher: " + a.Publisher
		switch c.From {
		case "":
			fix = "give yours a name of its own: " + renamed
		case parent.Rulebook:
		default:
			fix += ", or drop it and " + c.From + "'s artifact"
		}
		out = append(out, finding(declarationPath(c.Name), 0, c.Name+" is the parent "+parent.Rulebook+"'s checker code, and an overlay holds a name its parent carries only by reference from the parent", fix))
	}
	return out
}

// bound holds each binding that names checker code to a declaration of that
// name and of the binding's tool: the rulebook's own, or one by reference,
// from an import or from the parent, so an overlay's rule is checked by its
// parent's checker code through a declaration from the parent. Where the
// name's folder holds the rulebook's own code with no declaration read and
// the parent or an import carries the name, the fix renames that code and
// declares nothing over it.
func bound(b model.Book, decls []model.Checker, parent *Imported, t folders, byImport func(string) bool) []model.Finding {
	var out []model.Finding
	for _, r := range b.Rules {
		for _, bd := range r.Bindings {
			if bd.Checker == "" {
				continue
			}
			i := slices.IndexFunc(decls, func(c model.Checker) bool { return c.Name == bd.Checker })
			switch {
			case i < 0:
				fix := "declare it in " + declarationPath(bd.Checker) + ", or bind to a tool's own entry"
				a, byParent := carries(parent, bd.Checker)
				switch {
				case t.code(bd.Checker) && (byParent || byImport(bd.Checker)):
					fix = "rename yours: " + renamed
				case byParent:
					fix = "declare it from the parent in " + declarationPath(bd.Checker) + ", with from: " + parent.Rulebook + " and publisher: " + a.Publisher
				}
				out = append(out, model.Finding{Entity: string(r.ID), Msg: string(r.ID) + "'s " + bd.Tool + " binding names the checker code " + bd.Checker + ", which the rulebook does not declare", Fix: fix})
			case decls[i].Tool != bd.Tool:
				out = append(out, model.Finding{Entity: string(r.ID), Msg: string(r.ID) + "'s " + bd.Tool + " binding names the checker code " + bd.Checker + ", which is declared for " + decls[i].Tool,
					Fix: "bind it with " + decls[i].Tool + ", or name an artifact of " + bd.Tool})
			}
		}
	}
	return out
}
