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

// StaleRulesJSON is the rules.json of the format-3 rulebook at root when the
// file in the tree is missing or differs from it: one Output, or none when the
// file is what the sources make. The proof grid is not read, so a standing is
// the one a book with no proof has, as the coverage listing derives it.
//
// A folder with no rulebook is ErrNoBook. A book with findings is the
// FindingsError and nothing is generated from a book that does not load clean.
// Nothing is written.
func StaleRulesJSON(ctx context.Context, p port.Ports, root model.Root) ([]Output, error) {
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
	out := Output{Path: path.Join(rep.Dir, generate.RulesJSON), Data: generate.RulesJSONOf(rep.Book, held)}
	have, err := p.Tree.Read(root, out.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return []Output{out}, nil
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", out.Path, err)
	case bytes.Equal(have, out.Data):
		return nil, nil
	}
	return []Output{out}, nil
}
