// Package format is the format context: it finds a rulebook in a repository
// and reads its files into the model (docs/architecture.md section 7.1). This
// part reads the identity, rulebook.yaml, and the structure, structure.yaml;
// the rules, terms and guides come in later parts. It reaches files only
// through the Tree port and checks them only through the Schema port, imports
// no other context (arch.ports.8) and holds no host, address or path of its
// own (code.config.1): the docs base comes from the identity.
package format

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// NamesNoticeFile is the entity of the notice that says the names check did
// not run.
const NamesNoticeFile = "names check"

// The two files this part reads, and the schemas that hold them.
const (
	IdentityFile    = "rulebook.yaml"
	StructureFile   = "structure.yaml"
	identitySchema  = "v3/identity.schema.json"
	structureSchema = "v3/structure.schema.json"
)

// ErrNoBook is a repository with no rulebook.yaml anywhere in it.
var ErrNoBook = errors.New("no " + IdentityFile + " here")

// ErrTwoBooks is a repository with more than one rulebook.yaml. A rulebook may
// sit anywhere in a repository, but only one, so which is the book is never a
// guess.
var ErrTwoBooks = errors.New("more than one " + IdentityFile)

// NotReady is a file written in a format newer than this engine reads. The run
// ends "not ready", naming both versions, and reads nothing else of the file.
type NotReady struct {
	// File is the file, relative to the root, slash-separated.
	File string
	// Found is the format version the file says it is written in.
	Found int
	// Reads is the newest format version this engine reads.
	Reads int
}

func (e *NotReady) Error() string {
	return fmt.Sprintf("%s is written in format %d and this engine reads up to format %d; update the engine", e.File, e.Found, e.Reads)
}

// Finding is one problem in a file a person wrote: the file, relative to the
// root, the 1-based line (0 when none is known) and a sentence saying what is
// wrong and what to do.
type Finding struct {
	File string
	Line int
	Msg  string
	// Fix is the change to offer, "" when there is none to name.
	Fix string
	// Prose is whether the prose lint made it of a statement it does not
	// hold: the one finding migrate's read-back counts and does not refuse.
	Prose bool
	// Names is whether the names check made it (NamesIn): a caller tells a
	// names finding by this mark, never by its words.
	Names bool
}

func (f Finding) String() string {
	s := f.File + ": " + f.Msg
	if f.Line > 0 {
		s = fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Msg)
	}
	if f.Fix != "" {
		s += " (fix: " + f.Fix + ")"
	}
	return s
}

// Loaded is a book read from a repository: where it was found, what was read
// of it and what is wrong with it. A book with findings is partial: the
// identity or the structure that failed is the zero value.
type Loaded struct {
	// Dir is the folder of rulebook.yaml, relative to the root, "" for the
	// root itself, slash-separated.
	Dir string
	// Book is what was read.
	Book model.Book
	// Findings are the problems, in the order of the files read.
	Findings []Finding
	// Notices are what a check says that is not a problem: a check that did
	// not run, and why.
	Notices []Finding
	// Sites is where each rule is written, by entity (rule:<id>), so a delta
	// can be rendered into its file.
	Sites map[string]Site
	// Clashed is where the first rule of Book.Clashed with each id is written,
	// by that id: the rule a renumber gives a new id, which Sites does not
	// hold, since it names the rule that kept the id.
	Clashed map[model.RuleID]Site
	// Held is how many migrated rules the prose lint holds: each statement
	// that still has the digest its conversion recorded.
	Held int
	// at is where the subjects of the contract checks stand, for Contracts.
	at *positions
}

// Options are what a load is given beyond the repository.
type Options struct {
	// Names is the private list of names rule text must not hold; nil turns
	// the names check off, which the load says in a notice.
	Names *Names
	// Statements is the digest of each statement a conversion recorded, by
	// rule id, which holds a migrated rule from the prose lint until its
	// statement changes. Nil takes them from the migrate entries
	// of the history the load reads; only migrate's read-back hands them in,
	// the ones it has just built, since its history line is not written yet.
	// Nothing turns the prose lint or the undefined-term check off.
	Statements map[model.RuleID]model.Digest
}

// skipped is whether a file's path lies where the go tool does not look for
// packages: below a folder named testdata, or one whose name starts with "."
// or "_". A rulebook.yaml there is a fixture or a hidden copy, never the book.
func skipped(file string) bool {
	folders := strings.Split(file, "/")
	for _, f := range folders[:len(folders)-1] {
		if f == "testdata" || strings.HasPrefix(f, ".") || strings.HasPrefix(f, "_") {
			return true
		}
	}
	return false
}

// Find answers the folder, relative to root, that holds the one rulebook.yaml
// of the repository, "" when it is the root's own. A rulebook.yaml below a
// testdata folder, or a folder starting with "." or "_", is not a book (the
// set the go tool skips). Of what is left the shallowest wins, the one with
// the fewest path segments: a deeper one is an embedded fixture, not a book,
// and nothing reports it. None is ErrNoBook, and two at the shallowest depth
// are ErrTwoBooks naming both.
func Find(tree port.Tree, root model.Root) (string, error) {
	entries, err := tree.List(root, "", model.Every)
	if err != nil {
		return "", fmt.Errorf("list the repository: %w", err)
	}
	var found []string
	depth := 0
	for _, e := range entries {
		if e.Dir || path.Base(e.Path) != IdentityFile || skipped(e.Path) {
			continue
		}
		d := strings.Count(e.Path, "/")
		switch {
		case len(found) == 0 || d < depth:
			found, depth = []string{e.Path}, d
		case d == depth:
			found = append(found, e.Path)
		}
	}
	switch len(found) {
	case 0:
		return "", ErrNoBook
	case 1:
		dir := path.Dir(found[0])
		if dir == "." {
			dir = ""
		}
		return dir, nil
	}
	return "", fmt.Errorf("%w: %s and %s; keep one", ErrTwoBooks, found[0], found[1])
}

// Load finds the rulebook in the repository at root and reads its identity and
// structure. An error is a repository that cannot be read as a rulebook at all
// (ErrNoBook, ErrTwoBooks, *NotReady, a port failure); a file with something
// wrong in it is a finding, and the other file is still read.
func Load(tree port.Tree, schema port.Schema, root model.Root) (Loaded, error) {
	return LoadWith(tree, schema, root, Options{})
}

// LoadWith is Load with the options. With a names list, every line of every
// file the load read, and each one's path, is held to it (NamesIn): a load
// that ends in an error has read no rulebook, so it holds nothing.
func LoadWith(tree port.Tree, schema port.Schema, root model.Root, opts Options) (Loaded, error) {
	if opts.Names == nil {
		return loadBook(tree, schema, root, opts)
	}
	s := seeing(tree)
	out, err := loadBook(s, schema, root, opts)
	if err != nil {
		return out, err
	}
	literals := Literals(out.Book.Terms)
	for _, f := range s.read() {
		out.Findings = append(out.Findings, namesInBook(opts.Names, literals, join(out.Dir, BindingsDir), f.Path, f.Data)...)
	}
	return out, nil
}

// loadBook is LoadWith past the names check: it reads the files.
func loadBook(tree port.Tree, schema port.Schema, root model.Root, opts Options) (Loaded, error) {
	dir, err := Find(tree, root)
	if err != nil {
		return Loaded{}, err
	}
	out := Loaded{Dir: dir, at: &positions{rules: map[model.RuleID]ref{}}}
	idFile, stFile := join(dir, IdentityFile), join(dir, StructureFile)
	id, found, err := readFile(tree, schema, root, idFile, identitySchema, migrations())
	if err != nil {
		return out, err
	}
	out.Findings = append(out.Findings, found...)
	if id != nil && len(found) == 0 {
		out.Book.Identity = identityOf(id)
		for _, im := range ordered.Maps(id.List("imports")) {
			out.at.imports = append(out.at.imports, im.KeyLine("parts"))
		}
	}
	st, found, err := readFile(tree, schema, root, stFile, structureSchema, migrations())
	if err != nil {
		return out, err
	}
	if st != nil {
		found = slugFindings(stFile, st)
	}
	out.Findings = append(out.Findings, found...)
	if st == nil || len(found) > 0 {
		return out, nil
	}
	out.Book.Structure = structureOf(st)
	out.Sites = map[string]Site{}
	out.Clashed = map[model.RuleID]Site{}
	r := reader{tree: tree, schema: schema, root: root, dir: dir, out: &out, ids: map[model.RuleID]string{}, refs: &[]ref{}, slugs: &[]slugRef{}, prose: &[]proseItem{}, trans: &[]transEntry{}, opts: opts}
	if opts.Names == nil {
		out.Notices = append(out.Notices, Finding{File: NamesNoticeFile, Msg: "the names check did not run: no names list was given; set the names-file key or GRAGAS_NAMES_FILE"})
	}
	return out, r.loadRest(out.Book.Structure)
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}
