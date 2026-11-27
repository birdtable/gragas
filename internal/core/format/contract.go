package format

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// Foreign is a rulebook a book is composed of, its parent or an import, as
// coreapi hands it in: its name and release, its part slugs and its rules.
// Only the ids and aliases of the rules are read; their prose is display only
// (arch.ports.19). Format fetches nothing: the caller reads the book from the
// releases cache (arch.ports.8, A10).
type Foreign struct {
	Rulebook string
	Version  string
	Parts    []string
	Rules    []model.Rule
	// CheckerCode is the checker code the rulebook carries, by name and
	// publisher, which the importer declares to bring it (S3). The fetch of
	// a parent or an import fills it; a test hands it in until then.
	CheckerCode []model.CheckerArtifact
}

// Composed is what a book is composed of beyond itself, each one handed in
// when the caller holds it: the parent at the pin of extends, and the imports
// at theirs. A pinned book left out is not checked against, and says so.
type Composed struct {
	Parent  *Foreign
	Imports []Foreign
}

// ErrNotThePin is a book handed in that the identity does not pin: another
// name or another release, or a book with no pin at all. It is the caller's
// mistake, and no check is run against it.
var ErrNotThePin = errors.New("a rulebook handed in is not one the identity pins")

// positions are where the contract's subjects stand in the files read, kept
// by Load for Contracts.
type positions struct {
	// rules is the file and line of each rule.
	rules map[model.RuleID]ref
	// tightens are the bindings that name a rule they tighten.
	tightens []ref
	// imports is the line of each import's parts in the identity.
	imports []int
}

// Contracts checks the overlay and import contracts of a loaded book against
// the books it is composed of (S3, S4, S34):
//
//   - an overlay defines no rule under a parent's id or alias, and adds none
//     in a part the parent holds: the parent and the overlay are one chain,
//     one id space;
//   - an import names only parts the imported rulebook holds; an import is
//     namespaced by its rulebook's name, so the importer's own rule may share
//     an id with an imported one;
//   - a tightening, a binding's tightens or a bindings/ record's subject,
//     names a rule of the parent or of an imported part: not the book's own,
//     not one two of them hold, since the id cannot say which is meant;
//   - a bindings/ record names its subject on its first line.
//
// A pinned book not handed in is a notice naming what was not checked
// against it, a Finding of the kind Loaded.Notices holds: it never changes
// the closing word (cli.output.9: a check that did not run says so, and is
// not a pass). A book handed in that the identity does not pin is
// ErrNotThePin. A book whose identity did not load is not checked.
func Contracts(l Loaded, c Composed) ([]Finding, []Finding, error) {
	id := l.Book.Identity
	if id.Name == "" {
		return nil, nil, nil
	}
	idFile := join(l.Dir, IdentityFile)
	ck := contract{l: l, idFile: idFile, holders: map[model.RuleID][]string{}}
	if err := ck.pins(c); err != nil {
		return nil, nil, err
	}
	for _, r := range l.Book.Rules {
		for _, x := range append([]model.RuleID{r.ID}, r.Aliases...) {
			ck.hold(x, ck.holderOf(r))
		}
	}
	if ck.parent != nil {
		ck.foreign(*ck.parent, "the parent "+named(*ck.parent), nil)
		ck.overlay()
	}
	for i, im := range id.Imports {
		if f := ck.imports[i]; f != nil {
			ck.foreign(*f, "the import "+named(*f), im.Parts)
			ck.parts(i, im, *f)
		}
	}
	for _, t := range ck.subjects() {
		ck.subject(t)
	}
	return ck.findings, ck.notices, nil
}

// contract is one run of Contracts.
type contract struct {
	l        Loaded
	idFile   string
	parent   *Foreign
	imports  []*Foreign
	unread   bool
	holders  map[model.RuleID][]string
	findings []Finding
	notices  []Finding
}

// pins matches each book handed in to the pin it is for, and gives a notice
// for each pin with none.
func (ck *contract) pins(c Composed) error {
	id := ck.l.Book.Identity
	switch e := id.Extends; {
	case e == nil && c.Parent != nil:
		return fmt.Errorf("%w: the parent %s, and %s extends no rulebook", ErrNotThePin, named(*c.Parent), ck.idFile)
	case e != nil && c.Parent == nil:
		ck.unread = true
		ck.notice("parent not fetched: %s %s, so the overlay contract against it was not checked: no rule is checked against the parent's ids and parts, and no tightening of the parent's rules is checked", e.Rulebook, e.Version)
	case e != nil:
		if c.Parent.Rulebook != e.Rulebook || c.Parent.Version != e.Version {
			return fmt.Errorf("%w: the parent %s, and %s extends %s %s", ErrNotThePin, named(*c.Parent), ck.idFile, e.Rulebook, e.Version)
		}
		ck.parent = c.Parent
	}
	ck.imports = make([]*Foreign, len(id.Imports))
	for i := range c.Imports {
		f := &c.Imports[i]
		at := slices.IndexFunc(id.Imports, func(im model.Import) bool { return im.Rulebook == f.Rulebook && im.Version == f.Version })
		if at < 0 || ck.imports[at] != nil {
			return fmt.Errorf("%w: the import %s, which %s does not pin once", ErrNotThePin, named(*f), ck.idFile)
		}
		ck.imports[at] = f
	}
	for i, im := range id.Imports {
		if ck.imports[i] == nil {
			ck.unread = true
			ck.notice("import not fetched: %s %s, so the import contract against it was not checked: its parts %s are not checked to exist, and no tightening of its rules is checked", im.Rulebook, im.Version, strings.Join(im.Parts, ", "))
		}
	}
	return nil
}

// foreign records the ids and aliases of f's rules, those in parts alone when
// parts is not nil, as held by who.
func (ck *contract) foreign(f Foreign, who string, parts []string) {
	for _, r := range f.Rules {
		if parts != nil && !slices.Contains(parts, partOf(r.ID)) {
			continue
		}
		for _, x := range append([]model.RuleID{r.ID}, r.Aliases...) {
			ck.hold(x, who)
		}
	}
}

// holderOf is who holds a rule the book carries: the book itself, or, for
// one it imported, the import whose parts hold the rule's part, named as
// foreign names it, so the import handed in is the same holder and not a
// second one. An imported rule is borrowed, and a record tightens it as it
// tightens a parent's (S34, S70); it is never the book's own.
func (ck *contract) holderOf(r model.Rule) string {
	if r.Origin != model.Imported {
		return "this rulebook"
	}
	for _, im := range ck.l.Book.Identity.Imports {
		if slices.Contains(im.Parts, partOf(r.ID)) {
			return "the import " + im.Rulebook + " " + im.Version
		}
	}
	return "an import"
}

func (ck *contract) hold(id model.RuleID, who string) {
	if !slices.Contains(ck.holders[id], who) {
		ck.holders[id] = append(ck.holders[id], who)
	}
}

// overlay is S4's first half: no rule of the book is a parent's, by id or
// alias, and none is added in a part the parent holds. Each rule has one
// finding at most, and a rule that has one is not the book's own to a
// tightening, so the one breach is not found again at each tightening of it.
func (ck *contract) overlay() {
	p := *ck.parent
	ids := map[model.RuleID]bool{}
	for _, r := range p.Rules {
		for _, x := range append([]model.RuleID{r.ID}, r.Aliases...) {
			ids[x] = true
		}
	}
	for _, r := range ck.l.Book.Rules {
		at := ck.at(r.ID)
		if x, ok := firstOf(append([]model.RuleID{r.ID}, r.Aliases...), ids); ok {
			ck.add(at, fmt.Sprintf("%s is the parent %s's rule %s, and an overlay never edits a parent's rule in place: tighten it with a binding whose tightens names it, or exclude it with a decision and add a rule of your own in a part of your own", r.ID, named(p), x))
			ck.disown(r)
			continue
		}
		if part := partOf(r.ID); slices.Contains(p.Parts, part) {
			ck.add(at, fmt.Sprintf("%s is added in the part %s, which the parent %s holds, and an overlay adds rules only in parts of its own", r.ID, part, named(p)))
			ck.disown(r)
		}
	}
}

// disown takes the ids of r from the book's own, r being a finding already.
func (ck *contract) disown(r model.Rule) {
	for _, x := range append([]model.RuleID{r.ID}, r.Aliases...) {
		ck.holders[x] = slices.DeleteFunc(ck.holders[x], func(w string) bool { return w == "this rulebook" })
	}
}

// parts is the import contract: every part an import names is a part of the
// imported rulebook.
func (ck *contract) parts(i int, im model.Import, f Foreign) {
	line := 0
	if at := ck.l.at; at != nil && i < len(at.imports) {
		line = at.imports[i]
	}
	for _, part := range im.Parts {
		if !slices.Contains(f.Parts, part) {
			ck.findings = append(ck.findings, Finding{File: ck.idFile, Line: line, Msg: fmt.Sprintf("imports the part %s, which %s does not hold: name a part it has (%s)", part, named(f), strings.Join(f.Parts, ", "))})
		}
	}
}

// subjects are every tightening's subject: each binding's tightens, then each
// bindings/ record's. A record whose first line names no subject is a finding
// and has none.
func (ck *contract) subjects() []ref {
	var out []ref
	if at := ck.l.at; at != nil {
		out = append(out, at.tightens...)
	}
	for _, rec := range ck.l.Book.BindingRecords {
		file := join(ck.l.Dir, rec.Path)
		s, ok := SubjectOf(rec.Data)
		if !ok {
			ck.findings = append(ck.findings, Finding{File: file, Line: 1, Msg: "a binding record names its subject on its first line, as subject: <rule id>"})
			continue
		}
		out = append(out, ref{id: s, file: file, line: 1, what: "subject"})
	}
	return out
}

// subject checks one tightening's subject: one rule of the parent or of an
// imported part, and not the book's own. A subject held by none, while a
// pinned book was not handed in, may be that book's and is not judged: the
// notice already says so.
func (ck *contract) subject(t ref) {
	who := ck.holders[t.id]
	id := ck.l.Book.Identity
	switch {
	case slices.Contains(who, "this rulebook"):
		ck.add(t, fmt.Sprintf("%s names %s, this rulebook's own rule: a tightening names a rule of the parent or of an import, and an own rule is bound in place", t.what, t.id))
	case len(who) > 1:
		ck.add(t, fmt.Sprintf("%s names %s, which %s all hold, so it cannot say which is meant", t.what, t.id, strings.Join(who, " and ")))
	case len(who) == 1:
	case id.Extends == nil && len(id.Imports) == 0:
		ck.add(t, fmt.Sprintf("%s names %s, and this rulebook extends none and imports none, so it has no rule to tighten but its own", t.what, t.id))
	case !ck.unread:
		ck.add(t, fmt.Sprintf("%s names %s, which neither the parent nor an imported part holds", t.what, t.id))
	}
}

func (ck *contract) at(id model.RuleID) ref {
	if at := ck.l.at; at != nil {
		if r, ok := at.rules[id]; ok {
			return r
		}
	}
	return ref{file: ck.idFile}
}

func (ck *contract) add(at ref, msg string) {
	ck.findings = append(ck.findings, Finding{File: at.file, Line: at.line, Msg: msg})
}

func (ck *contract) notice(format string, args ...any) {
	ck.notices = append(ck.notices, Finding{File: ck.idFile, Msg: fmt.Sprintf(format, args...)})
}

// subjectLine is a record's first line: the key and a rule id as the format
// schema holds one (common.schema.json, id), and nothing else.
var subjectLine = regexp.MustCompile(`^subject: ([a-z]+\.[a-z0-9]+(?:-[a-z0-9]+)*\.[1-9][0-9]*)$`)

// SubjectOf reads the subject of a bindings/ record from its first line and
// nothing else, so the record stays opaque (link owns what it means). A first
// line that is not exactly "subject: <rule id>", ended by a newline or the
// end of the file, names none.
func SubjectOf(data []byte) (model.RuleID, bool) {
	line, _, _ := bytes.Cut(data, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'}) // a record written with CRLF ends its line as one with LF does
	m := subjectLine.FindSubmatch(line)
	if m == nil {
		return "", false
	}
	return model.RuleID(m[1]), true
}

// partOf is the part of a rule id, its first word.
func partOf(id model.RuleID) string {
	p, _, _ := strings.Cut(string(id), ".")
	return p
}

// firstOf is the first of ids in set.
func firstOf(ids []model.RuleID, set map[model.RuleID]bool) (model.RuleID, bool) {
	for _, x := range ids {
		if set[x] {
			return x, true
		}
	}
	return "", false
}

func named(f Foreign) string { return f.Rulebook + " " + f.Version }
