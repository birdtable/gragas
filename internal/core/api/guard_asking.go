package coreapi

import (
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// asking is a tree that keeps every path a read, a stat or a listing of root
// is asked for: the names the loader reads, there or not.
type asking struct {
	port.Tree
	root  model.Root
	asked map[string]bool
}

func (t asking) Read(root model.Root, p string) ([]byte, error) {
	t.ask(root, p)
	return t.Tree.Read(root, p)
}

func (t asking) Stat(root model.Root, p string) (model.Entry, error) {
	t.ask(root, p)
	return t.Tree.Stat(root, p)
}

func (t asking) List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error) {
	t.ask(root, dir)
	return t.Tree.List(root, dir, scope)
}

func (t asking) ask(root model.Root, p string) {
	if root == t.root && p != "" && p != "." {
		t.asked[p] = true
	}
}
