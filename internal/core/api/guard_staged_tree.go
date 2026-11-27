package coreapi

import (
	"bytes"
	"errors"
	"io/fs"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The helpers guard staged reads a tree with: the book of a tree, a file or
// none, a path below a folder, and a tree with the old layout or with no
// file. They are a move out of guard_staged.go, which holds the judgement.

// loadBook is the book of tree, an empty one where the tree holds none.
func loadBook(tree port.Tree, schema port.Schema, root model.Root) (model.Book, error) {
	l, err := format.Load(tree, schema, root)
	if errors.Is(err, ErrNoBook) {
		return model.Book{}, nil
	}
	return l.Book, err
}

// lineAt is the line of data that starts at off, from 1.
func lineAt(data []byte, off int) int {
	return bytes.Count(data[:off], []byte("\n")) + 1
}

// readOr is the file at p in tree, nil when it holds none.
func readOr(tree port.Tree, root model.Root, p string) ([]byte, error) {
	b, err := tree.Read(root, p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

func orRoot(dir string) string {
	if dir == "" {
		return "the root"
	}
	return dir
}

// join is dir and name as a path below the root.
func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// holdsV2 is whether t holds the old layout, in its root or in its rulebook/
// folder, which migrate moves to the format.
func (t *revTree) holdsV2() bool {
	entries, err := t.List(t.root, "", model.Every)
	if err != nil {
		return false
	}
	dir, _ := format.V2Folder(entries)
	return dir != "-"
}

// empty is a tree of t's root that holds no file.
func (t *revTree) empty() *revTree {
	c := *t
	c.files, c.data, c.sorted = map[string]model.Entry{}, map[string][]byte{}, nil
	return &c
}
