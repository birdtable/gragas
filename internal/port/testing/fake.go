// Package porttest is the fake that fills every port of internal/port/legacy in
// memory (arch.ports.17), so the core and the command tree are tested with
// no disk, schema compiler, lock or run log. It is a test-only stand-in:
// only _test.go files import it, and it holds no word of any one system. It
// fills the five ports of internal/port/legacy for now; the seventeen of
// internal/port are World's, in world.go.
package porttest

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/port/legacy"
)

// Fake is one in-memory world. Its zero value is a world with an empty
// repository, every document valid, a free lock and an empty run log; a test
// sets the fields it needs before Ports.
type Fake struct {
	// Repo is the repository Files.Repo answers with, whatever dir is asked.
	Repo fstest.MapFS
	// RepoErr, when set, is Files.Repo's error.
	RepoErr error
	// Names is the text Files.Open answers with; NamesErr, when set, its error.
	Names    string
	NamesErr error
	// Problems is what every Checker.Validate answers with: nil is valid.
	Problems []legacy.Problem
	// WriteErr, when set, is every Writer.Write's error.
	WriteErr error
	// Held makes Locks.Take answer that another process holds the lock.
	Held bool

	// Wrote is every path written, slash-separated, in order.
	Wrote []string
	// Locked is how many times the lock was taken, and Released dropped.
	Locked, Released int
	// Took is the path of every lock taken, in order.
	Took []string
	// Runs is every run record opened, in order.
	Runs []*Run
}

// Ports is f as every port, each one filled.
func (f *Fake) Ports() legacy.Ports {
	return legacy.Ports{Files: files{f}, Writer: writer{f}, Schemas: schemas{f}, Locks: locks{f}, Runs: runs{f}, TOML: tomlReader{}}
}

type files struct{ f *Fake }

func (x files) Repo(string) (legacy.Source, error) {
	if x.f.RepoErr != nil {
		return nil, x.f.RepoErr
	}
	if x.f.Repo == nil {
		x.f.Repo = fstest.MapFS{}
	}
	return x.f.Repo, nil
}

func (x files) Open(string) (io.ReadCloser, error) {
	if x.f.NamesErr != nil {
		return nil, x.f.NamesErr
	}
	return io.NopCloser(strings.NewReader(x.f.Names)), nil
}

type writer struct{ f *Fake }

func (x writer) Write(_, path string, _ []byte) error {
	if x.f.WriteErr != nil {
		return x.f.WriteErr
	}
	x.f.Wrote = append(x.f.Wrote, path)
	return nil
}

type schemas struct{ f *Fake }

func (x schemas) Compile(legacy.Source, string, ...string) (legacy.Checker, error) { return x, nil }

func (x schemas) Validate(string, any) []legacy.Problem { return slices.Clone(x.f.Problems) }

type locks struct{ f *Fake }

// errHeld is the fake's holder, said as a lock that is held says it.
var errHeld = fmt.Errorf("held by another run: %w", legacy.ErrHeld)

func (x locks) Take(path, _ string) (legacy.Lock, error) {
	if x.f.Held {
		return nil, errHeld
	}
	x.f.Locked++
	x.f.Took = append(x.f.Took, path)
	return lock(x), nil
}

type lock struct{ f *Fake }

func (x lock) Release() error {
	x.f.Released++
	return nil
}

type runs struct{ f *Fake }

func (x runs) Begin(path string, argv []string, build string) legacy.Run {
	r := &Run{Path: path, Argv: argv, Build: build, id: fmt.Sprintf("run-%d", len(x.f.Runs)+1), Code: -1}
	x.f.Runs = append(x.f.Runs, r)
	return r
}

// Run is one run record the fake kept.
type Run struct {
	// Path is the run log the record went to.
	Path    string
	Argv    []string
	Build   string
	Changes []string
	Undo    *legacy.Undo
	// Code is the exit code Finish closed it with, -1 while it is open.
	Code       int
	Word, What string
	id         string
}

// ID is the run's id: run-1 for the first run of a Fake.
func (r *Run) ID() string { return r.id }

// Change keeps line.
func (r *Run) Change(line string) { r.Changes = append(r.Changes, line) }

// WayBack keeps u.
func (r *Run) WayBack(u legacy.Undo) { r.Undo = &u }

// Finish closes the record.
func (r *Run) Finish(code int, word, what string) { r.Code, r.Word, r.What = code, word, what }

// Err is nil: the fake's record is always written.
func (r *Run) Err() error { return nil }

// Missing is the ports of p that are nil, by field name: only the ones its
// tag says are required, or every one. The fake's test and cmd/gragas's hold
// their ports to it (arch.ports.17).
func Missing(p legacy.Ports, requiredOnly bool) []string {
	var out []string
	v := reflect.ValueOf(p)
	for f := range reflect.TypeFor[legacy.Ports]().Fields() {
		if requiredOnly && f.Tag.Get("port") != "required" {
			continue
		}
		if v.FieldByIndex(f.Index).IsNil() {
			out = append(out, f.Name)
		}
	}
	return out
}

// tomlReader is a reader no test has a TOML file for: it says so, and reads
// nothing, so a check that reaches it reports the file as unreadable.
type tomlReader struct{}

func (tomlReader) Read([]byte) (map[string]any, error) {
	return nil, errors.New("the fake reads no TOML")
}
