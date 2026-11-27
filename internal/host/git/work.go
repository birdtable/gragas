package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/gragas/internal/model"
)

// WorkObjects answers the object name git would give each of paths, a file
// of the work tree at root, if its bytes were staged as they are, in the
// order given: one run of hash-object --no-filters
// --stdin-paths, with no -w, over the paths that are regular files. No
// attribute is read for a path, so no end-of-line conversion and no clean
// program the repository names runs, and nothing is written: a file git
// would filter answers the name of its bytes, not of what git add would
// store, and so differs from its base.
//
// A path is handed to git only when go-files' confine.NoLinks finds a
// regular file there, reached through no link from root, and the path does
// not begin with a double quote, which --stdin-paths would unquote; any
// other path answers "": one that is absent, a link, a folder or another
// kind of file, one below a link, and one that is not a clean, relative,
// slash-separated path of a tree. A path that changes between the look and
// git's read is hashed as git finds it; that gap is the caller's, as for
// any read of a tree someone edits meanwhile. The paths are relative to
// root, as Files names them; root is the top of the repository, which Files
// checks.
//
// No path is an empty list, and nothing runs. A root that cannot be opened,
// a run that fails, or output that is not one object name a line for each
// path handed to git, is an error, and no name is answered with it.
func (g *Git) WorkObjects(ctx context.Context, root model.Root, paths []string) ([]string, error) {
	dir, err := dirOf(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(paths))
	if len(paths) == 0 {
		return out, nil
	}
	sent, err := regularFiles(dir, paths)
	if err != nil {
		return nil, g.failed("the work tree could not be opened", err)
	}
	if len(sent) == 0 {
		return out, nil
	}
	var names strings.Builder
	for _, i := range sent {
		names.WriteString(paths[i] + "\n")
	}
	raw, err := g.runWith(ctx, verbWork, dir, input{stdin: []byte(names.String())})
	if err != nil {
		return nil, g.answer("git hash-object failed", err)
	}
	ids, err := parseWork(raw, len(sent))
	if err != nil {
		return nil, g.failed("git hash-object printed what it does not print", err)
	}
	for k, i := range sent {
		out[i] = ids[k].text
	}
	return out, nil
}

// regularFiles answers the index in paths of each that is a regular file in
// dir, reached through no link, and a path git's --stdin-paths reads as it
// is: checked as a tree's path is, and not beginning with a double quote.
// A path NoLinks refuses or finds missing is left out; only dir that cannot
// be opened is an error.
func regularFiles(dir string, paths []string) (_ []int, err error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	var sent []int
	for i, p := range paths {
		if checkPath(p) != nil || strings.HasPrefix(p, `"`) {
			continue
		}
		name, err := confine.Local(p)
		if err != nil {
			continue
		}
		fi, err := confine.NoLinks(r, name)
		if err != nil || fi == nil || !fi.Mode().IsRegular() {
			continue
		}
		sent = append(sent, i)
	}
	return sent, nil
}

// parseWork reads what work-objects prints for n paths: n object names of
// one length, each ended by a newline, and nothing else.
func parseWork(out []byte, n int) ([]value, error) {
	ids, err := parseOIDLines(out, "an object name", "the work tree's ids")
	if err != nil {
		return nil, err
	}
	if len(ids) != n {
		return nil, fmt.Errorf("%d object names for %d paths", len(ids), n)
	}
	return ids, nil
}
