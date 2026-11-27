// Package surface is the domain of the public surface listing: for each port
// and published package, every exported name with the kind it is, its
// signature, a stability mark and the first sentence of its contract comment
// (arch.shared.9, A30). It reads the Go sources it is handed, renders
// docs/surface.md from them, and says where the file and the code disagree,
// both ways, and whether the reason for a change is recorded.
//
// The listing is generated from the code and from docs/surface-marks.yaml, and
// is never edited. The exports and signatures are the code's. The marks are
// decisions, written by hand in the marks file with the reason for each; a new
// export starts as experimental. The marks file also holds the change record:
// each change of the listing with the reason it was made. The same listing is
// the declared-API-break check of the release (supply.release.2) and its
// exported-name count feeds the coupling report (arch.measures.4).
//
// The change record has two forms (gragas#825). In the old one each line is the
// digest of the whole listing a change made, and the newest must be the digest
// of the listing, so two pull requests that each change the surface cannot both
// land without a third line for the listing that holds them both. In the new
// one the listing has a sum: each export's line (folder, name, kind, signature
// and mark) is summed with SHA-256 and the sums are added as numbers, modulo
// 2^256, so the sum does not hang on the order of the exports or of the
// changes. A base line holds the listing's sum when the record moved to the
// new form, and each step line what one change added to it; the record holds
// when the base and every step add up to the listing's sum. Two changes made
// apart then add up, with no line for the two together, and a change no step
// explains still fails. Every step carries its reason, written by a person.
//
// The sum is no seal. An author who writes a step can write any step, as one
// who writes a digest can write any digest: what the record holds is that a
// change of the listing comes with a line a reviewer reads, and the sum only
// has to change when the listing does.
package surface

import (
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/ordered"
)

// File is the listing, from the repository root.
const File = "docs/surface.md"

// The marks. A mark is a decision, never a default but experimental.
const (
	// Stable is a name a consumer may rely on until a declared break.
	Stable = "stable"
	// Experimental is a name that may change in any release.
	Experimental = "experimental"
	// Internal is a name that is exported for the program's own wiring and
	// is no part of the contract.
	Internal = "internal"
)

// Marks are the marks an export may carry.
func Marks() []string { return []string{Stable, Experimental, Internal} }

// Source is the non-test Go files of one package, by file name.
type Source struct {
	// Dir is the package's folder, relative to the module root.
	Dir   string
	Files map[string][]byte
}

// Export is one exported name.
type Export struct {
	// Name is the name, or Type.Method for a method.
	Name string
	// Kind is type, func, method, const or var.
	Kind string
	// Signature is the name's type as the code writes it, on one line.
	Signature string
	// Mark is one of Marks.
	Mark string
	// Summary is the first sentence of the contract comment, "" when none.
	Summary string
}

// Section is one package's exports, sorted by name.
type Section struct {
	Dir     string
	Exports []Export
}

// Collect is the exports of each source, with no mark yet, in the order of
// the sources.
func Collect(srcs []Source) ([]Section, error) {
	var out []Section
	for _, s := range srcs {
		fset := token.NewFileSet()
		var exports []Export
		names := make([]string, 0, len(s.Files))
		for n := range s.Files {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			f, err := parser.ParseFile(fset, s.Dir+"/"+n, s.Files[n], parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.Dir, err)
			}
			for _, d := range f.Decls {
				exports = append(exports, declExports(d)...)
			}
		}
		slices.SortFunc(exports, func(a, b Export) int { return strings.Compare(a.Name, b.Name) })
		out = append(out, Section{Dir: s.Dir, Exports: exports})
	}
	return out, nil
}

// declExports is the exported names one top-level declaration makes.
func declExports(d ast.Decl) []Export {
	switch d := d.(type) {
	case *ast.FuncDecl:
		return funcExports(d)
	case *ast.GenDecl:
		var out []Export
		for _, spec := range d.Specs {
			switch sp := spec.(type) {
			case *ast.TypeSpec:
				out = append(out, typeExports(d, sp)...)
			case *ast.ValueSpec:
				out = append(out, valueExports(d, sp)...)
			}
		}
		return out
	}
	return nil
}

func funcExports(d *ast.FuncDecl) []Export {
	if !d.Name.IsExported() {
		return nil
	}
	sig := funcSig(d.Type, d.Type.TypeParams)
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return []Export{{Name: d.Name.Name, Kind: "func", Signature: sig, Summary: summary(d.Doc)}}
	}
	recv := recvName(d.Recv.List[0].Type)
	if !ast.IsExported(recv) {
		return nil
	}
	return []Export{{Name: recv + "." + d.Name.Name, Kind: "method", Signature: sig, Summary: summary(d.Doc)}}
}

// recvName is the type name of a receiver, without its pointer or its type
// arguments.
func recvName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}

func typeExports(d *ast.GenDecl, sp *ast.TypeSpec) []Export {
	if !sp.Name.IsExported() {
		return nil
	}
	doc := sp.Doc
	if doc == nil {
		doc = d.Doc
	}
	sig := typeString(sp.Type)
	if sp.TypeParams != nil {
		sig = "[" + fieldsString(sp.TypeParams) + "] " + sig
	}
	if sp.Assign.IsValid() {
		sig = "= " + sig
	}
	out := []Export{{Name: sp.Name.Name, Kind: "type", Signature: sig, Summary: summary(doc)}}
	if it, ok := sp.Type.(*ast.InterfaceType); ok {
		for _, m := range it.Methods.List {
			if len(m.Names) == 0 || !m.Names[0].IsExported() {
				continue
			}
			ft, ok := m.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			out = append(out, Export{Name: sp.Name.Name + "." + m.Names[0].Name, Kind: "method", Signature: funcSig(ft, nil), Summary: summary(m.Doc)})
		}
	}
	return out
}

// typeString is a type as a consumer can name it, on one line: a struct keeps
// its exported fields and their tags (a tag is a wire name), an interface
// keeps what it embeds (its methods are listed one by one).
func typeString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StructType:
		return structString(x)
	case *ast.InterfaceType:
		var embeds []string
		for _, f := range x.Methods.List {
			if len(f.Names) == 0 {
				embeds = append(embeds, types.ExprString(f.Type))
			}
		}
		return "interface{" + strings.Join(embeds, "; ") + "}"
	}
	return types.ExprString(e)
}

// structString is a struct with its exported fields and their tags.
func structString(x *ast.StructType) string {
	var fields []string
	for _, f := range x.Fields.List {
		p, ok := fieldString(f)
		if ok {
			fields = append(fields, p)
		}
	}
	return "struct{" + strings.Join(fields, "; ") + "}"
}

// fieldString is one field of a struct as a consumer sees it; ok is false for a
// field it cannot name.
func fieldString(f *ast.Field) (string, bool) {
	var names []string
	for _, n := range f.Names {
		if n.IsExported() {
			names = append(names, n.Name)
		}
	}
	p := types.ExprString(f.Type)
	switch {
	case len(f.Names) == 0 && ast.IsExported(embeddedName(f.Type)):
	case len(names) > 0:
		p = strings.Join(names, ", ") + " " + p
	default:
		return "", false
	}
	if f.Tag != nil {
		p += " " + f.Tag.Value
	}
	return p, true
}

// embeddedName is the name an embedded field takes: the type's, or for a
// package-qualified one the selected name.
func embeddedName(e ast.Expr) string {
	if s, ok := e.(*ast.SelectorExpr); ok {
		return s.Sel.Name
	}
	return recvName(e)
}

func valueExports(d *ast.GenDecl, sp *ast.ValueSpec) []Export {
	kind := "var"
	if d.Tok == token.CONST {
		kind = "const"
	}
	doc := sp.Doc
	if doc == nil {
		doc = d.Doc
	}
	var out []Export
	for i, n := range sp.Names {
		if !n.IsExported() {
			continue
		}
		sig := ""
		if sp.Type != nil {
			sig = types.ExprString(sp.Type)
		}
		if kind == "const" && i < len(sp.Values) {
			sig = strings.TrimSpace(sig + " = " + types.ExprString(sp.Values[i]))
		}
		out = append(out, Export{Name: n.Name, Kind: kind, Signature: sig, Summary: summary(doc)})
	}
	return out
}

// funcSig is a function's signature on one line, type parameters included.
func funcSig(ft *ast.FuncType, tparams *ast.FieldList) string {
	s := types.ExprString(&ast.FuncType{Params: ft.Params, Results: ft.Results})
	if tparams != nil {
		s = "func[" + fieldsString(tparams) + "]" + strings.TrimPrefix(s, "func")
	}
	return s
}

// fieldsString is a field list as written, comma separated.
func fieldsString(l *ast.FieldList) string {
	var parts []string
	for _, f := range l.List {
		var names []string
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		p := types.ExprString(f.Type)
		if len(names) > 0 {
			p = strings.Join(names, ", ") + " " + p
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

// summary is the first sentence of a comment, on one line.
func summary(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	t := strings.Join(strings.Fields(g.Text()), " ")
	if i := strings.Index(t, ". "); i >= 0 {
		t = t[:i+1]
	}
	return t
}

// Count is the exported names of each package, by folder: what the coupling
// report reads (arch.measures.4).
func Count(secs []Section) map[string]int {
	out := make(map[string]int, len(secs))
	for _, s := range secs {
		out[s.Dir] = len(s.Exports)
	}
	return out
}

// MarksFile is the authored file that holds the marks and the change record,
// from the repository root. The listing is generated from the code and this
// file and is never edited.
const MarksFile = "docs/surface-marks.yaml"

// MinReasonWords is the fewest words a reason has.
const MinReasonWords = 3

// ErrMarks is a marks file that cannot be read as one.
var ErrMarks = errors.New("the surface marks")

// Row is one mark and why it is that mark.
type Row struct {
	Mark   string
	Reason string
}

// Entry is one line of the change record and why it was made. Exactly one of
// Digest, Base and Step is set.
type Entry struct {
	// Digest is an old-form line's: the digest of the whole listing a change
	// made.
	Digest string
	// Base is the line where the record moved to the new form: the listing's
	// sum then, 64 hex digits.
	Base string
	// Step is a new-form line's: what one change added to the listing's sum,
	// modulo 2^256, 64 hex digits.
	Step   string
	Reason string
}

// MarkFile is the marks file as it stands. A nil *MarkFile is an empty one.
type MarkFile struct {
	// Rows are the marks, by package folder and name, separated by a space.
	Rows map[string]Row
	// Changes are the change record, oldest first.
	Changes []Entry
}

var (
	digestForm = regexp.MustCompile("^sha256:[0-9a-f]{64}$")
	sumForm    = regexp.MustCompile("^[0-9a-f]{64}$")
)

// changeKinds are the keys that name what a line of the change record is, one
// to a line.
func changeKinds() []string { return []string{"digest", "base", "step"} }

// ParseMarks reads the marks file. Every problem it finds is in the error,
// which matches ErrMarks. It reads with ordered.ParseStrict: git's union
// driver merges the file, and two changes that remove one export leave one
// step with two reasons, so a key written twice is refused with its line
// rather than read as the last value, which would drop the other reason. An
// anchor or an alias is refused too.
func ParseMarks(data []byte) (*MarkFile, error) {
	v, err := ordered.ParseStrict(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMarks, err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, fmt.Errorf("%w: not a mapping", ErrMarks)
	}
	var bad []string
	for _, k := range doc.Keys() {
		if k != "marks" && k != "changes" {
			bad = append(bad, "unknown key "+k)
		}
	}
	mf := &MarkFile{Rows: map[string]Row{}}
	for _, m := range ordered.Maps(doc.List("marks")) {
		bad = append(bad, mf.addRow(m)...)
	}
	for _, m := range ordered.Maps(doc.List("changes")) {
		bad = append(bad, mf.addChange(m)...)
	}
	bad = append(bad, mf.recordOrder()...)
	if len(bad) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrMarks, strings.Join(bad, "; "))
	}
	return mf, nil
}

// unknownKeys names the keys of m that are not in want, or are missing.
func unknownKeys(m *ordered.Map, at string, want []string) []string {
	var bad []string
	for _, k := range m.Keys() {
		if !slices.Contains(want, k) {
			bad = append(bad, at+": unknown key "+k)
		}
	}
	for _, k := range want {
		if m.Str(k) == "" {
			bad = append(bad, at+": no "+k)
		}
	}
	return bad
}

// wordsEnough is whether a reason says something.
func wordsEnough(r string) bool { return len(strings.Fields(r)) >= MinReasonWords }

func (mf *MarkFile) addRow(m *ordered.Map) []string {
	at := fmt.Sprintf("marks line %d", m.Line)
	bad := unknownKeys(m, at, []string{"package", "name", "mark", "reason"})
	if len(bad) > 0 {
		return bad
	}
	key := m.Str("package") + " " + m.Str("name")
	if !slices.Contains(Marks(), m.Str("mark")) {
		bad = append(bad, fmt.Sprintf("%s: %q is not a mark (%s)", at, m.Str("mark"), strings.Join(Marks(), ", ")))
	}
	if !wordsEnough(m.Str("reason")) {
		bad = append(bad, at+": the reason for "+key+" is at least three words")
	}
	if _, dup := mf.Rows[key]; dup {
		bad = append(bad, at+": "+key+" is marked twice")
	}
	mf.Rows[key] = Row{Mark: m.Str("mark"), Reason: strings.Join(strings.Fields(m.Str("reason")), " ")}
	return bad
}

func (mf *MarkFile) addChange(m *ordered.Map) []string {
	at := fmt.Sprintf("changes line %d", m.Line)
	var kinds []string
	for _, k := range changeKinds() {
		if _, ok := m.Get(k); ok {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) != 1 {
		return []string{at + ": a line is one of digest, base or step, with a reason"}
	}
	kind := kinds[0]
	bad := unknownKeys(m, at, []string{kind, "reason"})
	if len(bad) > 0 {
		return bad
	}
	value := m.Str(kind)
	switch {
	case kind == "digest" && !digestForm.MatchString(value):
		bad = append(bad, at+": the digest is sha256: and 64 hex digits")
	case kind != "digest" && !sumForm.MatchString(value):
		bad = append(bad, at+": the "+kind+" is 64 lower-case hex digits")
	case kind == "step" && value == strings.Repeat("0", 64):
		bad = append(bad, at+": a step of nothing is no change, so it has no line")
	}
	if !wordsEnough(m.Str("reason")) {
		bad = append(bad, at+": a reason is at least three words")
	}
	e := Entry{Reason: strings.Join(strings.Fields(m.Str("reason")), " ")}
	switch kind {
	case "digest":
		e.Digest = value
	case "base":
		e.Base = value
	default:
		e.Step = value
	}
	mf.Changes = append(mf.Changes, e)
	return bad
}

// recordOrder is what is wrong with the order of the change record: the old
// form stops at the base, which comes once, and every step is after it. A
// step may come twice: an export removed and then added back as it was is
// the same step twice, with its removal between, and a change counted twice
// with nothing between does not add up to the listing's sum.
func (mf *MarkFile) recordOrder() []string {
	var bad []string
	base := -1
	for i, e := range mf.Changes {
		switch {
		case e.Base != "" && base >= 0:
			bad = append(bad, "the change record has a second base, and moves to the new form once")
		case e.Base != "":
			base = i
		case e.Digest != "" && base >= 0:
			bad = append(bad, "the change record has a digest after its base, where only steps add up")
		case e.Step != "" && base < 0:
			bad = append(bad, "the change record has the step "+e.Step+" before any base, which it adds to")
		}
	}
	return bad
}

// Mark applies the marks of mf to the sections: an export the file does not
// hold starts as experimental.
func Mark(secs []Section, mf *MarkFile) {
	for i := range secs {
		for j := range secs[i].Exports {
			e := &secs[i].Exports[j]
			e.Mark = Experimental
			if mf != nil {
				if r, ok := mf.Rows[secs[i].Dir+" "+e.Name]; ok {
					e.Mark = r.Mark
				}
			}
		}
	}
}

// Digest is the identity of a listing: its names, kinds, signatures and marks.
// A contract comment is not in it, so rewording one asks for no reason.
func Digest(secs []Section) string {
	var b strings.Builder
	for _, s := range secs {
		for _, e := range s.Exports {
			b.WriteString(strings.Join([]string{s.Dir, e.Name, e.Kind, e.Signature, e.Mark}, "\x00") + "\n")
		}
	}
	return "sha256:" + sum(b.String())
}

func sum(s string) string {
	return digest.Hex([]byte(s))
}

// number is a 256-bit number, written big-endian.
type number [digest.Size]byte

// plus is a + b modulo 2^256, a byte at a time from the low end; the carry
// out of the top byte is dropped. A byte sum that wraps is the carry.
func (a number) plus(b number) number {
	var out number
	var carry byte
	for i := len(a) - 1; i >= 0; i-- {
		s := a[i] + b[i]
		t := s + carry
		if s < a[i] || t < s {
			carry = 1
		} else {
			carry = 0
		}
		out[i] = t
	}
	return out
}

// minus is a - b modulo 2^256: a plus the two's complement of b.
func (a number) minus(b number) number {
	var inv, one number
	for i := range b {
		inv[i] = ^b[i]
	}
	one[len(one)-1] = 1
	return a.plus(inv.plus(one))
}

func (a number) String() string { return hex.EncodeToString(a[:]) }

// parseNumber is a number from its 64 hex digits; ok is false for any other
// text.
func parseNumber(s string) (n number, ok bool) {
	if !sumForm.MatchString(s) {
		return n, false
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(n) {
		return n, false
	}
	copy(n[:], b)
	return n, true
}

// Sum is the listing's sum: the SHA-256 of each export's line (its folder,
// name, kind, signature and mark), added as numbers modulo 2^256, in 64 hex
// digits. It holds what Digest holds, in no order, so two changes made apart
// add up to the listing that holds them both.
func Sum(secs []Section) string {
	var total number
	for _, s := range secs {
		for _, e := range s.Exports {
			total = total.plus(digest.Sum([]byte(strings.Join([]string{s.Dir, e.Name, e.Kind, e.Signature, e.Mark}, "\x00"))))
		}
	}
	return total.String()
}

// moved says whether the change record of mf has moved to the new form.
func moved(mf *MarkFile) bool {
	return mf != nil && slices.ContainsFunc(mf.Changes, func(e Entry) bool { return e.Base != "" })
}

// missing is what the base and the steps of mf lack of the listing's sum, zero
// when they add up to it; ok is false when one of them is not a number, which
// a file ParseMarks read never holds.
func missing(secs []Section, mf *MarkFile) (n number, ok bool) {
	var recorded number
	for _, e := range mf.Changes {
		v := e.Base + e.Step
		if v == "" {
			continue
		}
		part, ok := parseNumber(v)
		if !ok || e.Base != "" && e.Step != "" {
			return n, false
		}
		recorded = recorded.plus(part)
	}
	total, _ := parseNumber(Sum(secs))
	return total.minus(recorded), true
}

// BaseKept is what is wrong with the change record of head against the one
// of main, the record a landing is judged against: once main's has moved to
// the new form, head holds main's base number as it is. An edit of the number
// would take a change into the base with no step and no reason for it; the
// base's reason is words, and may be reworded. Empty when head holds the
// number, or main's has not moved.
func BaseKept(main, head *MarkFile) string {
	was, ok := baseOf(main)
	if !ok {
		return ""
	}
	now, ok := baseOf(head)
	switch {
	case !ok:
		return MarksFile + ": the change record has no base, and main's has the base " + was.Base + ": a record moves to the new form once"
	case now.Base != was.Base:
		return MarksFile + ": the change record's base is not main's, the base " + was.Base + ": a base is written once, and a change after it is a step"
	}
	return ""
}

// baseOf is the base line of the change record of mf, and whether it has one.
func baseOf(mf *MarkFile) (Entry, bool) {
	if mf == nil {
		return Entry{}, false
	}
	for _, e := range mf.Changes {
		if e.Base != "" {
			return e, true
		}
	}
	return Entry{}, false
}

// Unrecorded says whether the change record of mf does not hold the listing
// of secs, whose marks are set. In the old form the newest line is not its
// digest; in the new form the base and the steps do not add up to its sum.
func Unrecorded(secs []Section, mf *MarkFile) bool {
	if mf == nil || len(mf.Changes) == 0 {
		return true
	}
	if moved(mf) {
		gap, ok := missing(secs, mf)
		return !ok || gap != number{}
	}
	return mf.Changes[len(mf.Changes)-1].Digest != Digest(secs)
}

// Why is the line the author adds to the change record of mf for the listing
// of secs: in the new form the step the record lacks, in the old the digest.
func Why(secs []Section, mf *MarkFile) string {
	if !moved(mf) {
		return "add under changes: in " + MarksFile + " the digest " + Digest(secs) + " with a reason"
	}
	gap, ok := missing(secs, mf)
	if !ok {
		return "the change record in " + MarksFile + " holds a base or a step that is not 64 hex digits"
	}
	return "add under changes: in " + MarksFile + " the step " + gap.String() + " with a reason"
}

// line is an export as the file writes it.
func (e Export) line() string {
	s := "- `" + e.Name + "` (" + e.Kind + ") **" + e.Mark + "**"
	if e.Signature != "" {
		s += ": " + codeSpan(e.Signature)
	}
	if e.Summary != "" {
		s += " - " + e.Summary
	}
	return s
}

// codeSpan is s as a Markdown code span, fenced by more backticks than s holds
// in a row.
func codeSpan(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if longest > 0 {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

const intro = `<!-- generated by gragas-dev surface (task gen) from the code and docs/surface-marks.yaml; edit those, not this file -->
# Public surface

Every port and published package, with each exported name, its kind, its
signature, a stability mark and the first sentence of its contract comment.
Go exports are listed now; the TypeScript packages join when they exist.

Marks: **stable** may be relied on until a declared break; **experimental** may
change in any release and is where a new export starts; **internal** is exported
for the program's own wiring and is no part of the contract. A mark and the
reason for it are written by hand in ` + "`" + MarksFile + "`" + `.

` + "`task generated-files`" + ` holds this file against the code both ways: an export not
listed and a listed export gone both fail, and so does a change of surface or
mark the change record in the marks file does not hold. A record of digests
holds the listing whose digest is its newest line; once it has a base, the
base and its steps hold the listing whose sum they add up to, in any order.
`

// Render is the listing file for sections whose marks are set and the change
// record of the marks file.
func Render(secs []Section, changes []Entry) []byte {
	var b strings.Builder
	b.WriteString(intro)
	b.WriteString("\nListing digest: `" + Digest(secs) + "`\n")
	b.WriteString("\nListing sum: `" + Sum(secs) + "`\n")
	for _, s := range secs {
		fmt.Fprintf(&b, "\n## %s\n\n%d exported names.\n\n", s.Dir, len(s.Exports))
		for _, e := range s.Exports {
			b.WriteString(e.line() + "\n")
		}
	}
	b.WriteString("\n## Change record\n\n")
	b.WriteString("From the marks file, oldest first. Each line is the digest of the listing a change made, the base the sums start from, or a step one change added to the sum, and why.\n\n")
	for _, e := range changes {
		switch {
		case e.Base != "":
			b.WriteString("- base `" + e.Base + "` " + e.Reason + "\n")
		case e.Step != "":
			b.WriteString("- step `" + e.Step + "` " + e.Reason + "\n")
		default:
			b.WriteString("- `" + e.Digest + "` " + e.Reason + "\n")
		}
	}
	return []byte(b.String())
}

var exportLine = regexp.MustCompile("^- `([A-Za-z0-9_.]+)` \\(([a-z]+)\\) \\*\\*([a-z]+)\\*\\*")

// listedLines are the export lines of a listing file, by package folder and
// name separated by a space.
func listedLines(have []byte) map[string]string {
	out := map[string]string{}
	dir := ""
	for line := range strings.SplitSeq(string(have), "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			dir = strings.TrimPrefix(line, "## ")
		case dir != "" && dir != "Change record":
			if m := exportLine.FindStringSubmatch(line); m != nil {
				out[dir+" "+m[1]] = line
			}
		}
	}
	return out
}

// Problems is everything wrong with the listing on disk against the code and
// the marks file, one line each: a mark on an export that is not there, an
// export not listed, a listed export gone or changed, a file stale in some
// other way, a change with no reason. have is the listing's bytes, nil when
// missing. secs need no marks set.
func Problems(secs []Section, mf *MarkFile, have []byte) []string {
	marked := slices.Clone(secs)
	for i := range marked {
		marked[i].Exports = slices.Clone(secs[i].Exports)
	}
	Mark(marked, mf)

	var out []string
	known := map[string]bool{}
	for _, s := range marked {
		for _, e := range s.Exports {
			known[s.Dir+" "+e.Name] = true
		}
	}
	if mf != nil {
		for _, key := range slices.Sorted(maps.Keys(mf.Rows)) {
			if !known[key] {
				out = append(out, "marked but not exported: "+key)
			}
		}
	}
	if have == nil {
		out = append(out, File+" is missing; run task gen")
	} else {
		out = append(out, listingProblems(marked, mf, have)...)
	}
	if Unrecorded(marked, mf) {
		out = append(out, "the surface or a mark changed and the change record does not say why: "+Why(marked, mf))
	}
	return out
}

func listingProblems(marked []Section, mf *MarkFile, have []byte) []string {
	var out []string
	lines := listedLines(have)
	want := map[string]bool{}
	for _, s := range marked {
		for _, e := range s.Exports {
			key := s.Dir + " " + e.Name
			want[key] = true
			switch got, ok := lines[key]; {
			case !ok:
				out = append(out, "not listed: "+key)
			case got != e.line():
				out = append(out, "changed: "+key)
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(lines)) {
		if !want[key] {
			out = append(out, "listed but gone: "+key)
		}
	}
	var changes []Entry
	if mf != nil {
		changes = mf.Changes
	}
	if len(out) == 0 && string(Render(marked, changes)) != string(have) {
		out = append(out, File+" is stale; run task gen")
	}
	return out
}
