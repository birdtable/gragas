package coreapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/birdtable/gragas/internal/core/generate"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// StaleGenerated is the generated files of the format-3 rulebook at root that
// are missing or differ from what its sources make: rules.json and ids.lock,
// in that order, each an Output, or none when both are what the sources make.
// The proof grid is not read, so a standing is the one a book with no proof
// has, as the coverage listing derives it. bindings.json and coverage.json
// have no reader and are not made (docs/format/README.md).
//
// A folder with no rulebook is ErrNoBook. A book with findings is the
// FindingsError and nothing is generated from a book that does not load clean.
// Nothing is written.
func StaleGenerated(ctx context.Context, p port.Ports, root model.Root) ([]Output, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rep, err := LoadBook(ctx, p, root)
	if err != nil {
		return nil, err
	}
	if n := len(rep.Findings); n > 0 {
		return nil, &FindingsError{Count: n}
	}
	held, err := link.Derive(rep.Book, model.Grid{}, model.Chain{})
	if err != nil {
		return nil, err
	}
	lockPath := path.Join(rep.Dir, "ids.lock")
	lock, err := readGenerated(p, root, lockPath)
	if err != nil {
		return nil, err
	}
	made := []Output{
		{Path: path.Join(rep.Dir, generate.RulesJSON), Data: generate.RulesJSONOf(rep.Book, held)},
		{Path: lockPath, Data: generate.IDsLock(rep.Book, lock)},
	}
	var stale []Output
	for _, o := range made {
		have, err := readGenerated(p, root, o.Path)
		if err != nil {
			return nil, err
		}
		if have == nil || !bytes.Equal(have, o.Data) {
			stale = append(stale, o)
		}
	}
	return stale, nil
}

// readGenerated is the file at name, nil when there is none.
func readGenerated(p port.Ports, root model.Root, name string) ([]byte, error) {
	data, err := p.Tree.Read(root, name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if data == nil {
		data = []byte{}
	}
	return data, nil
}
