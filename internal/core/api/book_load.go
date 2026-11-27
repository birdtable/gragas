package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ErrNotReady is a rulebook the engine cannot read yet, whose message names
// what is needed: a file in a format newer than this engine reads ends "not
// ready", naming both versions (store.schemas.2). The root ends the run on the
// word, exit code 4.
var ErrNotReady = errors.New("not ready")

// ErrNoBook is a repository with no rulebook.yaml, and ErrTwoBooks one with
// more than one.
var (
	ErrNoBook   = format.ErrNoBook
	ErrTwoBooks = format.ErrTwoBooks
)

// BookReport is a rulebook read from a repository: where its files are, what
// was read and what is wrong with it.
type BookReport struct {
	// Dir is the folder of rulebook.yaml below the root, "" for the root.
	Dir string
	// Book is the identity and the structure; a file with a finding is the
	// zero value.
	Book model.Book
	// Findings are the problems, each entity "file:line" (or the file when
	// no line is known).
	Findings []model.Finding
	// Resolve returns the rule that has the id as its id or as an alias. It
	// is built once with the report; other contexts are handed it as an
	// argument and never import the loader.
	Resolve func(model.RuleID) (model.Rule, bool)
	// Notices are what the load says that is not a problem, such as a names
	// check that did not run and why, or a parent or an import the overlay
	// and import contracts were not checked against. A notice never changes
	// the closing word.
	Notices []model.Notice
	// Checkers are the checker-code declarations read from checkers/, the
	// rulebook's own and an import's by reference; a refused one is a
	// finding and not here.
	Checkers []model.Checker
	// Held is how many migrated rules the prose lint holds, each statement
	// still with the digest its conversion recorded.
	Held int
	// prose says, beside the finding of the same index, whether the prose
	// lint made it: the one finding migrate's read-back counts, not refuses.
	prose []bool
	// namesAt is the index in Findings of each finding the names check made.
	namesAt []int
	// NamesOff is why the names check did not run, "" when it ran: no list
	// was named, the list is not there, or it holds no name. A caller whose
	// whole job is the check ends not ready on it; the notice says the same.
	NamesOff string
	// names is the private list the load read, nil for none: the project check
	// hands the same list to its repository checks, and reads none again.
	names *format.Names
}

// LoadBook finds the one rulebook.yaml in the repository at root and reads its
// identity and structure through the Tree and Schema ports, and checks the
// overlay and import contracts. A file in a newer
// format is ErrNotReady; findings are in the report, not the error.
func LoadBook(ctx context.Context, p port.Ports, root model.Root) (BookReport, error) {
	return LoadBookNamed(ctx, p, root, "")
}

// LoadBookNamed is LoadBook with the names check on: namesFile is the absolute
// path of the private names list, which lies outside the repository and is read
// through the Tree's owned read. An empty path, a file that is not there, or
// one with no name in it turns the check off with a notice in the report. A
// pattern that does not compile, or a file another account owns or can write,
// ends ErrNotReady, since the check cannot say what it would have found.
func LoadBookNamed(ctx context.Context, p port.Ports, root model.Root, namesFile string) (BookReport, error) {
	return loadBookWith(ctx, p, root, namesFile, nil)
}

// loadBookWith is LoadBookNamed with the statement digests that hold a migrated
// rule from the prose lint: nil reads them from the history the
// load reads, as every caller but one does; migrate's read-back hands in the
// ones its conversion has just built, before its history line is written.
func loadBookWith(ctx context.Context, p port.Ports, root model.Root, namesFile string, statements map[model.RuleID]model.Digest) (BookReport, error) {
	// Nothing fetches a parent or an import yet, so none is handed in.
	return loadBookComposed(ctx, p, root, namesFile, statements, format.Composed{})
}

// loadBookComposed is loadBookWith over the parent and the imports handed in:
// a test hands them in until a fetch does.
func loadBookComposed(ctx context.Context, p port.Ports, root model.Root, namesFile string, statements map[model.RuleID]model.Digest, composed format.Composed) (BookReport, error) {
	names, notice, err := readBookNames(p.Owned, namesFile)
	if err != nil {
		return BookReport{}, err
	}
	l, err := format.LoadWith(p.Tree, p.Schema, root, format.Options{Names: names, Statements: statements})
	if nr, ok := errors.AsType[*format.NotReady](err); ok {
		return BookReport{Dir: l.Dir}, maskErr(names, fmt.Errorf("%w: %w", ErrNotReady, nr))
	}
	if err != nil {
		return BookReport{}, maskErr(names, err)
	}
	r := BookReport{Dir: l.Dir, Book: l.Book, Held: l.Held, Resolve: format.NewTable(l.Book.Rules).Resolve, names: names}
	// A parent or an import not handed in is a notice that names the checks
	// it skipped.
	found, notices, err := format.Contracts(l, composed)
	if err != nil {
		return BookReport{}, maskErr(names, err)
	}
	weak, unweighed := borrowedFindings(ctx, p, root, l, composed.Parent)
	found, notices = append(found, weak...), append(notices, unweighed...)
	listed, unlisted := handWrittenFindings(ctx, p, root, l)
	found, notices = append(found, listed...), append(notices, unlisted...)
	for _, f := range slices.Concat(l.Findings, found) {
		r.add(f)
		r.prose = append(r.prose, f.Prose)
	}
	code, err := checkerCode(p, root, l.Dir, l.Book, composed)
	if err != nil {
		return BookReport{}, maskErr(names, err)
	}
	for _, f := range checkerNames(names, l, code) {
		r.add(f)
	}
	var held []model.Finding
	r.Checkers, held = link.Checkers(p.Schema, code)
	r.Findings = append(r.Findings, inDir(l.Dir, held)...)
	for _, n := range slices.Concat(l.Notices, notices) {
		msg := n.Msg
		if n.File == format.NamesNoticeFile {
			if notice != "" {
				msg = notice
			}
			r.NamesOff = msg
		}
		if n.Fix != "" {
			msg += " (fix: " + n.Fix + ")"
		}
		file, line := placeOf(n)
		r.Notices = append(r.Notices, model.Notice{Entity: entityOf(n), Msg: msg, File: file, Line: line})
	}
	masked(names, &r)
	return r, nil
}

// placeOf is the file and line f is about, from its fields: "" and 0 for the
// names check, which is about no file.
func placeOf(f format.Finding) (string, int) {
	if f.File == format.NamesNoticeFile {
		return "", 0
	}
	return f.File, max(f.Line, 0)
}

func entityOf(f format.Finding) string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

// checkerCode is what link reads of the rulebook's checker code: the listing
// of checkers/ below the rulebook's folder dir, every file in every scope,
// since a tool loads an untracked file as readily as a tracked one; the
// content of each file that stands where a declaration does; and the checker
// code of the parent and the imports handed in. A rulebook with no checkers/
// folder has none. Nothing is loaded or run.
func checkerCode(p port.Ports, root model.Root, dir string, b model.Book, composed format.Composed) (link.CheckerCode, error) {
	in := link.CheckerCode{Book: b}
	if composed.Parent != nil {
		in.Parent = &link.Imported{Rulebook: composed.Parent.Rulebook, Code: composed.Parent.CheckerCode}
	}
	for _, f := range composed.Imports {
		in.Imports = append(in.Imports, link.Imported{Rulebook: f.Rulebook, Code: f.CheckerCode})
	}
	base := path.Join(dir, model.CheckersDir)
	if _, err := p.Tree.Stat(root, base); errors.Is(err, fs.ErrNotExist) {
		return in, nil
	} else if err != nil {
		return in, err
	}
	entries, err := p.Tree.List(root, base, model.Every)
	if err != nil {
		return in, err
	}
	for _, e := range entries {
		if e.Dir {
			continue
		}
		rel := strings.TrimPrefix(e.Path, dir+"/")
		if dir == "" {
			rel = e.Path
		}
		in.Listing = append(in.Listing, rel)
		if path.Base(rel) != model.CheckerFile || strings.Count(rel, "/") != 2 {
			continue
		}
		data, err := p.Tree.Read(root, e.Path)
		if err != nil {
			return in, err
		}
		in.Files = append(in.Files, model.File{Path: rel, Data: data})
	}
	return in, nil
}

// inDir is each finding with its file below the rulebook's folder dir, as a
// finding's file is relative to the repository's root.
func inDir(dir string, found []model.Finding) []model.Finding {
	out := make([]model.Finding, 0, len(found))
	for _, f := range found {
		if f.File != "" && dir != "" {
			f.File = path.Join(dir, f.File)
			f.Entity = f.File
			if f.Line > 0 {
				f.Entity = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
		}
		out = append(out, f)
	}
	return out
}
