package coreapi

import (
	"context"
	"io/fs"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// MaxRead is the most a repository run reads of one tracked file, for the
// tests of the capped read.
const MaxRead = maxRead

// Shown is the guard's show function, for the table test of it.
func Shown(v any) string { return shown(v) }

// LoadBookComposed is the book load over the parent and the imports handed
// in, which a test hands in until a fetch does.
func LoadBookComposed(ctx context.Context, p port.Ports, root model.Root, c format.Composed) (BookReport, error) {
	return loadBookComposed(ctx, p, root, "", nil, c)
}

// LiveRowsAt is the per-file lookup of the exemptions file's live rows, which
// gragas check's os-files reader waives by.
func LiveRowsAt(src fs.ReadFileFS, today string) func(rule, file string) (string, bool) {
	return liveRowsAt(src, today)
}

// PartsIn is the part names of a parts file's text, for the test of its separators.
func PartsIn(raw string) []string { return partsIn(raw) }

// ReadFSOf is src as the repository checks of the project check are handed
// it (check_project_repo.go): the view wrapped in readFS.
func ReadFSOf(src fs.FS) fs.ReadFileFS { return readFS{src} }

// PluginAuditEntries is the check's audit.Entries over a, for the tests of the
// plugin read.
func PluginAuditEntries(a AuditPlugins, tested *model.Tested) audit.Entries { return a.entries(tested) }

// ReadRevTree reads each of paths, in order, through the Tree of root at rev
// that the guard judges a commit by, for the test of how it reads the history.
func ReadRevTree(ctx context.Context, tree port.Tree, h port.History, root model.Root, rev string, paths []string) ([][]byte, error) {
	t, err := newRevTree(ctx, tree, h, root, rev, false)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, len(paths))
	for _, p := range paths {
		b, err := t.Read(root, p)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// EmptiedTreeListing lists the tree of rev first, as the merge guard's
// format.Find does, then lists the tree emptied from it: what the emptied tree
// lists and what it says of the folder named by stat.
func EmptiedTreeListing(ctx context.Context, tree port.Tree, h port.History, root model.Root, rev, stat string) ([]model.Entry, error, error) {
	t, err := newRevTree(ctx, tree, h, root, rev, false)
	if err != nil {
		return nil, err, nil
	}
	if _, err := t.List(root, "", model.Every); err != nil {
		return nil, err, nil
	}
	e := t.empty()
	got, err := e.List(root, "", model.Every)
	_, serr := e.Stat(root, stat)
	return got, err, serr
}
