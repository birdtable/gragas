package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// revTree is the Tree of one root as git holds it at a rev, the index for "",
// read through History and never from the disk (the index's blobs come
// from History, not the working tree), with the writes of the replays so far
// laid over it. Read, Stat and List of its root answer it; every other call,
// and every other root, goes to the Tree it wraps, which is how a parent
// fetched at its pin is still read. A tree with no rev, the first commit's
// before, holds no file.
type revTree struct {
	port.Tree
	ctx  context.Context
	h    port.History
	root model.Root
	rev  string
	// files are the rev's files by path, and the replays' writes among them.
	files map[string]model.Entry
	// data are the bytes read so far and the bytes the replays wrote.
	data map[string][]byte
	// sorted caches paths(); a write of a new path drops it.
	sorted []string
}

// newRevTree lists the files of rev once. none is a tree with no file, for a
// repository with no commit yet; rev is then not read at all.
func newRevTree(ctx context.Context, tree port.Tree, h port.History, root model.Root, rev string, none bool) (*revTree, error) {
	t := &revTree{Tree: tree, ctx: ctx, h: h, root: root, rev: rev, files: map[string]model.Entry{}, data: map[string][]byte{}}
	if none {
		return t, nil
	}
	entries, err := h.Files(ctx, root, rev)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.Dir {
			t.files[e.Path] = e
		}
	}
	return t, nil
}

// write lays data over the file at p, as a replay's plan leaves it.
func (t *revTree) write(p string, data []byte) {
	if _, ok := t.files[p]; !ok {
		t.sorted = nil
	}
	t.files[p] = model.Entry{Path: p, Size: int64(len(data))}
	t.data[p] = slices.Clone(data)
}

// paths are the tree's files, sorted. The slice is shared: do not change it.
func (t *revTree) paths() []string {
	if t.sorted == nil {
		out := make([]string, 0, len(t.files))
		for p := range t.files {
			out = append(out, p)
		}
		slices.Sort(out)
		t.sorted = out
	}
	return t.sorted
}

func (t *revTree) Read(root model.Root, p string) ([]byte, error) {
	if root != t.root {
		return t.Tree.Read(root, p)
	}
	p = revPath(p)
	if b, ok := t.data[p]; ok {
		return slices.Clone(b), nil
	}
	if _, ok := t.files[p]; !ok {
		if t.isDir(p) {
			return nil, &fs.PathError{Op: "read", Path: p, Err: errors.New("is a folder")}
		}
		return nil, &fs.PathError{Op: "read", Path: p, Err: fs.ErrNotExist}
	}
	if err := t.fetch(p); err != nil {
		return nil, err
	}
	return slices.Clone(t.data[p]), nil
}

// A read of a file brings in the commit's next unread files with it, in path
// order, as one read of the repository: a judge reads about 380
// files of a commit, and one git process for each file, or for each folder,
// made a commit's check the cost of hundreds of processes. The batch is
// bounded: as many files as fetchFiles and as many bytes as fetchBytes, none
// a link or one past a read's cap.
const (
	fetchFiles = 256
	fetchBytes = 4 << 20
	// fetchHeader is what each file of a batch takes of the history's cap
	// beyond its bytes (its header and newlines), counted for each sibling.
	fetchHeader = 128
)

// fetch reads the file at p and, with it, the next unread files of the commit
// from p on, into data. A neighbour is only a convenience: when the batch
// fails, p is read alone and only its own error is returned, and the others
// stay unread, each to be read on its own if a judge asks for it.
func (t *revTree) fetch(p string) error {
	names := []string{p}
	total := t.files[p].Size + fetchHeader
	all := t.paths()
	from, _ := slices.BinarySearch(all, p)
	for _, q := range all[from:] {
		if len(names) >= fetchFiles {
			break
		}
		e := t.files[q]
		if _, read := t.data[q]; read || q == p || e.Link || e.Dir || total+e.Size+fetchHeader > fetchBytes {
			continue
		}
		names = append(names, q)
		total += e.Size + fetchHeader
	}
	if len(names) > 1 && t.fill(names) == nil {
		return nil
	}
	return t.fill(names[:1])
}

// fill reads names in one read of the history into data, or none of them.
func (t *revTree) fill(names []string) error {
	got, err := t.h.Blobs(t.ctx, t.root, t.rev, names)
	if err != nil {
		return err
	}
	if len(got) != len(names) {
		return fmt.Errorf("the history answered %d files of the %d it was asked for", len(got), len(names))
	}
	for i, q := range names {
		t.data[q] = got[i]
	}
	return nil
}

func (t *revTree) Stat(root model.Root, p string) (model.Entry, error) {
	if root != t.root {
		return t.Tree.Stat(root, p)
	}
	p = revPath(p)
	if p == "" {
		return model.Entry{Dir: true}, nil
	}
	if e, ok := t.files[p]; ok {
		return e, nil
	}
	if t.isDir(p) {
		return model.Entry{Path: p, Dir: true}, nil
	}
	return model.Entry{}, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
}

// List answers the files below dir and the folders that hold them. Every file
// of a rev is one its history holds, so the untracked scope is empty.
func (t *revTree) List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error) {
	if root != t.root {
		return t.Tree.List(root, dir, scope)
	}
	if scope == model.Untracked {
		return []model.Entry{}, nil
	}
	dir = revPath(dir)
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
		if _, ok := t.files[dir]; ok {
			return nil, &fs.PathError{Op: "list", Path: dir, Err: errors.New("is not a folder")}
		}
	}
	out := []model.Entry{}
	seen := map[string]bool{}
	for _, p := range t.paths() {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		for d := path.Dir(p); d != "." && len(d) > len(dir); d = path.Dir(d) {
			if !seen[d] {
				seen[d] = true
				out = append(out, model.Entry{Path: d, Dir: true})
			}
		}
		out = append(out, t.files[p])
	}
	slices.SortFunc(out, byPath)
	return out, nil
}

// isDir is whether some file of the tree lies below p.
func (t *revTree) isDir(p string) bool {
	prefix := p + "/"
	if p == "" {
		return true
	}
	for f := range t.files {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// revPath is p as the tree keys it: no "." and no leading "./".
func revPath(p string) string {
	if p == "" || p == "." {
		return ""
	}
	return path.Clean(strings.TrimPrefix(p, "./"))
}
