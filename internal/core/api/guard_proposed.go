package coreapi

import (
	"path"
	"slices"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// proposed is the tree with one file as the edit would leave it: its read,
// its stat and its folder's listing answer the new content, and every other
// call goes to the tree.
type proposed struct {
	port.Tree
	root model.Root
	path string
	data []byte
}

func (t proposed) Read(root model.Root, p string) ([]byte, error) {
	if root == t.root && p == t.path {
		return slices.Clone(t.data), nil
	}
	return t.Tree.Read(root, p)
}

func (t proposed) Stat(root model.Root, p string) (model.Entry, error) {
	if root == t.root && p == t.path {
		return model.Entry{Path: p, Size: int64(len(t.data))}, nil
	}
	return t.Tree.Stat(root, p)
}

func (t proposed) List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error) {
	entries, err := t.Tree.List(root, dir, scope)
	if err != nil || root != t.root {
		return entries, err
	}
	prefix := ""
	if dir != "" && dir != "." {
		prefix = dir + "/"
	}
	if len(t.path) < len(prefix) || t.path[:len(prefix)] != prefix {
		return entries, nil
	}
	have := map[string]bool{}
	for _, e := range entries {
		have[e.Path] = true
	}
	for d := path.Dir(t.path); d != "." && len(d)+1 > len(prefix); d = path.Dir(d) {
		if !have[d] {
			entries = append(entries, model.Entry{Path: d, Dir: true})
		}
	}
	if have[t.path] {
		for i := range entries {
			if entries[i].Path == t.path {
				entries[i].Size = int64(len(t.data))
			}
		}
	} else if scope != model.Tracked {
		entries = append(entries, model.Entry{Path: t.path, Size: int64(len(t.data))})
	}
	slices.SortFunc(entries, byPath)
	return entries, nil
}

// byPath orders entries by path, byte by byte, as the tree lists them.
func byPath(a, b model.Entry) int {
	switch {
	case a.Path < b.Path:
		return -1
	case a.Path > b.Path:
		return 1
	}
	return 0
}
