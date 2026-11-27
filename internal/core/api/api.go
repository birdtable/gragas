// Package coreapi is the operations every client calls, and the JSON result
// types they answer with (internal/core/api in the template of go.layout.1):
// what check and gen ask for, done once, the rulebook loaded through the
// ports, its generated files compared with the tree, and the stale ones
// written. It reaches the world only through the ports cmd/gragas fills. The
// folder is the template's; the package is not named api, which the lint's
// package-naming rule, arch.structure.7's check, refuses.
package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/generate"
	"github.com/birdtable/gragas/internal/core/rulebook"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// ErrNames is a names file that could not be read or holds a bad pattern.
var ErrNames = errors.New("names file")

// ErrNoParts is a folder that is not a rulebook repository: it has no
// rulebook/parts.yaml.
var ErrNoParts = rulebook.ErrNoParts

// Loaded is a rulebook read from a repository, as a command reads it: its
// problems, and how much was read. The book and the source it came from stay
// inside the operations; no domain type leaves this package. The source is
// held open until Close.
type Loaded struct {
	// Findings are the problems in the rulebook, each as a person reads it.
	Findings []string
	// Entries and Files are how many entries and concern files were read.
	Entries, Files int
	// Notice is why the names check did not run though a names file was
	// named and read: the new engine's notice of a list that holds no name,
	// "" otherwise. It names no path.
	Notice string

	book    *rulebook.Book
	src     legacy.Source
	schemas legacy.Checker
}

// Load reads and checks the rulebook in dir through ports. namesFile, when
// not empty, turns on the names check. Problems in the rulebook are
// Loaded.Findings; an error is a rulebook that could not be read:
// ErrNoParts, ErrNames, or the context's own error once it ends.
//
// A Loaded holds the repository open: the caller closes it once nothing it
// calls reads the repository any more, since Windows refuses to remove a
// folder that is held open. On an error nothing is held: Load
// closes what it opened.
func Load(ctx context.Context, ports legacy.Ports, dir, namesFile string) (Loaded, error) {
	names, err := readNames(ports.Files, namesFile)
	if err != nil {
		return Loaded{}, err
	}
	src, err := ports.Files.Repo(dir)
	if err != nil {
		return Loaded{}, fmt.Errorf("%w: %w", rulebook.ErrNoParts, err)
	}
	l, err := load(ctx, ports, src, names)
	if err != nil {
		return Loaded{}, errors.Join(err, Loaded{src: src}.Close())
	}
	if names != nil && names.Len() == 0 {
		l.Notice = namesEmpty
	}
	return l, nil
}

// Close lets go of the repository the rulebook was read from, when its
// source holds one open. Nothing that reads the repository is called with l
// after it. The zero Loaded holds none, and closes as nil.
func (l Loaded) Close() error {
	if c, ok := l.src.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// load reads and checks the rulebook in src, Load's work once the
// repository is open.
func load(ctx context.Context, ports legacy.Ports, src legacy.Source, names *format.Names) (Loaded, error) {
	if err := rulebook.Probe(src); err != nil {
		return Loaded{}, err
	}
	schemas, err := ports.Schemas.Compile(src, rulebook.SchemaDir, rulebook.SchemaFiles()...)
	if err != nil {
		return Loaded{}, fmt.Errorf("load schemas: %w", err)
	}
	b, err := rulebook.Load(ctx, src, schemas, rulebook.Options{Names: names})
	if err != nil {
		return Loaded{}, err
	}
	if b.Parts != nil {
		if err := b.CheckBaseline(); err != nil {
			return Loaded{}, err
		}
	}
	findings := make([]string, len(b.Findings))
	for i, f := range b.Findings {
		findings[i] = f.String()
	}
	return Loaded{Findings: findings, Entries: len(b.Rules), Files: len(b.Files), book: b, src: src, schemas: schemas}, nil
}

func readNames(files legacy.Files, path string) (*format.Names, error) {
	if path == "" {
		return nil, nil
	}
	f, err := files.Open(path)
	if errors.Is(err, port.ErrNotOwned) {
		return nil, fmt.Errorf("%w: %w", ErrNames, refusedFor(err))
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNames, namesFailure(err))
	}
	n, err := format.ReadNames(f)
	if cerr := f.Close(); err == nil && cerr != nil {
		err = cerr
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNames, namesFailure(err))
	}
	return n, nil
}

// Output is one generated file: its path, slash-separated from the
// repository root, and what it holds.
type Output struct {
	Path string
	Data []byte
}

// Stale is every generated file that is missing from the tree or differs
// from it: the files the loaded book makes, in the order generate.Outputs
// gives. The layer table's two files are gragas-dev's, not gen's.
func Stale(l Loaded) ([]Output, error) {
	outs := generate.Outputs(book{l.book})
	if err := checkRulesJSON(l, outs); err != nil {
		return nil, err
	}
	stale, err := generate.Stale(l.src, outs)
	if err != nil {
		return nil, err
	}
	out := make([]Output, len(stale))
	for i, o := range stale {
		out[i] = Output{Path: o.Path, Data: o.Data}
	}
	return out, nil
}

// ErrRulesJSON is a generated rules.json that does not match its schema.
var ErrRulesJSON = errors.New("rules.json does not match " + rulebook.SchemaRulesJSON)

// checkRulesJSON holds the rules.json about to be written to its schema, so
// its shape is a contract and not only what the code happens to write.
func checkRulesJSON(l Loaded, outs []generate.Output) error {
	for _, o := range outs {
		if o.Path != rulebook.Dir+"/"+generate.RulesJSON {
			continue
		}
		v, err := ordered.ParseYAML(o.Data)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrRulesJSON, err)
		}
		if bad := l.schemas.Validate(rulebook.SchemaRulesJSON, v); len(bad) > 0 {
			return fmt.Errorf("%w: line %d: %s", ErrRulesJSON, bad[0].Line, bad[0].Msg)
		}
	}
	return nil
}

// Apply writes each stale file into the repository at dir through w, in
// order, and tells written of each once it is in place. It stops at the
// first file it cannot write, or when ctx ends; the files written before stay
// written, each whole.
func Apply(ctx context.Context, w legacy.Writer, dir string, stale []Output, written func(path string)) error {
	for _, o := range stale {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.Write(dir, o.Path, o.Data); err != nil {
			return err
		}
		written(o.Path)
	}
	return nil
}

// Counts is the per-part report of a loaded book, in parts.yaml order,
// leaving out a part with no entries and no tools.
func Counts(l Loaded) []Count {
	b := l.book
	entries, enforced := map[string]int{}, map[string]int{}
	for _, r := range b.Rules {
		entries[r.Part]++
		if l.book.Standing(r.Data.Str("id")) == rulebook.Enforced {
			enforced[r.Part]++
		}
	}
	var out []Count
	for _, p := range b.Parts.List {
		if entries[p.Short] == 0 && len(b.Tools[p.Short]) == 0 {
			continue
		}
		out = append(out, Count{Part: p.Short, Name: p.Name, Entries: entries[p.Short], Enforced: enforced[p.Short], Tools: len(b.Tools[p.Short])})
	}
	return out
}
