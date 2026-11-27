// Package legacy holds the five ports of the engine's first form: Files,
// Writer, Schemas, Locks and Runs, and the values they carry. The old
// commands (check, gen) still reach the world through them, filled by the
// adapters under internal/host/jsonschema and fs, until M0b-02 and
// M6 move them to the seventeen ports of internal/port. Nothing new uses this
// package, and it is deleted with the last of its users.
//
// What is said here of a port holds as it did: every effect is a method of an
// interface, and one package under internal/host fills it. internal/port/testing
// still fills these five in memory.
package legacy

import (
	"errors"
	"io"
	"io/fs"
)

// ErrHeld is what Locks.Take's error matches when another process holds the
// lock. The error itself says which process, and since when.
var ErrHeld = errors.New("the lock is held")

// ErrNotOwner is what Writer.Write's and Locks.Take's error matches when the
// folder they write in, the repository or the folder that holds the lock, is
// owned by another account than the one this process runs as, root included:
// they write by its path, which that folder's owner could swap on the way.
// Nothing was written, and the error says which folder and whose it is.
var ErrNotOwner = errors.New("another account owns the folder")

// Problem is one way a document fails a schema: the line of the place in the
// document that failed (0 when unknown) and a sentence saying how.
type Problem struct {
	Line int
	Msg  string
}

// Undo is how a run that wrote is put back, for a person to read in its
// record: the lines to type, and why they work.
type Undo struct {
	Lines []string
	Why   string
}

// Source is one repository's files, by slash-separated paths relative to its
// root. A missing file is an error that matches fs.ErrNotExist, and nothing
// outside the root is reachable.
type Source interface {
	fs.ReadFileFS
	fs.ReadDirFS
	fs.StatFS
}

// Files reads files: a repository, and a file outside every repository.
type Files interface {
	// Repo is the repository at dir, which must be a folder.
	Repo(dir string) (Source, error)
	// Open opens the file at path for reading: the private names file,
	// which lives outside every repository.
	Open(path string) (io.ReadCloser, error)
}

// Writer replaces files inside a repository.
type Writer interface {
	// Write replaces the file at path, slash-separated and relative to the
	// repository at dir, with data, making its folder when it is missing. A
	// reader sees the old file or the new one, never half of either. A path
	// that leaves the repository is refused. A repository another account owns
	// is refused, matching ErrNotOwner.
	Write(dir, path string, data []byte) error
}

// Schemas compiles the JSON Schemas a document's shape is checked against.
type Schemas interface {
	// Compile reads each named schema from dir in src and compiles it. A
	// schema may refer to another only by a name in the same list.
	Compile(src Source, dir string, names ...string) (Checker, error)
}

// Checker checks documents against compiled schemas.
type Checker interface {
	// Validate checks doc, a value as internal/ordered parses it, against the
	// schema file named schema (such as "rules.schema.json"). It returns nil
	// when doc is valid, else the problems sorted by place, at most ten. A
	// schema that was not compiled is one problem, never a panic.
	Validate(schema string, doc any) []Problem
}

// Locks keeps two runs that write from writing at once.
type Locks interface {
	// Take takes the lock at path, without waiting, for a run that writes
	// in root, the folder -C names ("" when the command names none). When
	// another process holds it, the error matches ErrHeld. A lock in a
	// folder another account owns is refused, matching ErrNotOwner, and one
	// in a folder a clone carries, root among them, matching
	// port.ErrCarried.
	Take(path, root string) (Lock, error)
}

// Lock is a lock that was taken.
type Lock interface {
	// Release drops the lock.
	Release() error
}

// Runs keeps the record of every run that writes.
type Runs interface {
	// Begin opens the record of a run in the log at path: the command line
	// as typed, and the build that runs it. A record that cannot be written
	// says so at Err, never by stopping the run.
	Begin(path string, argv []string, build string) Run
}

// Run is one run's record, open until Finish.
type Run interface {
	// ID is the run's id, for a person to quote.
	ID() string
	// Change records one change the run really made.
	Change(line string)
	// WayBack records how the run is put back.
	WayBack(u Undo)
	// Finish closes the record with the exit code and the word the run
	// ended on, and what it said.
	Finish(code int, word, what string)
	// Err is the first error writing the record met, nil when none.
	Err() error
}

// TOML reads a TOML document into maps and lists of the plain kinds
// (map[string]any, []any, string, bool, int64, float64): the one reader of
// the config checker, for .gitleaks.toml.
type TOML interface {
	Read(data []byte) (map[string]any, error)
}

// Ports is every port gragas reaches the world through, filled once by
// cmd/gragas and handed down. Every port is required, as its tag says: no
// adapter of gragas lacks one, so no code tests a port for nil, and a port
// is never a stub that pretends to work.
type Ports struct {
	Files   Files   `port:"required"`
	Writer  Writer  `port:"required"`
	Schemas Schemas `port:"required"`
	Locks   Locks   `port:"required"`
	Runs    Runs    `port:"required"`
	TOML    TOML    `port:"required"`
}
