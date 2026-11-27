package coreapi

import (
	"context"
	"errors"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// folder is what the listings of conflicts, coverage, presets, explanations
// and tools read first: the folder's rulebook if there is one, and its tested
// set. No adapter is read: a tool's own reading is its plugin's.
type folder struct {
	hasBook bool
	rep     BookReport
	tested  *model.Tested
}

// readBook asks the journal for an open run and reads the rulebook at root if
// there is one. A folder with no rulebook has no tested set; a rulebook that
// cannot be read at all is ErrNotReady.
func readBook(ctx context.Context, p port.Ports, root model.Root) (folder, error) {
	var f folder
	if err := refuseOpenRun(ctx, p, root); err != nil {
		return f, err
	}
	rep, err := LoadBook(ctx, p, root)
	switch {
	case errors.Is(err, ErrNoBook):
	case err != nil:
		return f, err
	default:
		f.hasBook, f.rep, f.tested = true, rep, rep.Book.Identity.Tested
	}
	return f, nil
}
