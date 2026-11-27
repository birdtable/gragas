package coreapi

import (
	"context"
	"maps"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/port"
)

// revCache holds, for the length of one merge, the tree of each rev the
// commits it brings in are judged over and the book read from it, so a rev is
// listed and read once: in a chain, a commit's tree after is the next one's
// tree before, and each was read twice (gragas#1205). A tree in it is never
// written: a judge that lays a replay's writes over a tree before takes a
// fork of it. A nil cache reads every tree anew, as a change that is no merge
// does.
type revCache struct {
	trees map[string]*revTree
	books map[string]loadedRev
}

// loadedRev is the book of a rev as format.Load answered it, or its error.
type loadedRev struct {
	l   format.Loaded
	err error
}

func newRevCache() *revCache {
	return &revCache{trees: map[string]*revTree{}, books: map[string]loadedRev{}}
}

// tree answers the tree of rev, listed once, and the book read from it. The
// tree is the cache's own: read it, never write it. A rev of "" is the index
// and a tree with none is no file; neither is a rev to remember, so they are
// read as newRevTree reads them.
func (c *revCache) tree(ctx context.Context, p port.Ports, ask StagedAsk, rev string, none bool) (t *revTree, l format.Loaded, loadErr, err error) {
	if c == nil || rev == "" || none {
		if t, err = newRevTree(ctx, p.Tree, p.History, ask.Root, rev, none); err != nil {
			return nil, format.Loaded{}, nil, err
		}
		l, loadErr = format.Load(t, p.Schema, ask.Root)
		return t, l, loadErr, nil
	}
	if t, err = c.listed(ctx, p, ask, rev); err != nil {
		return nil, format.Loaded{}, nil, err
	}
	b, ok := c.books[rev]
	if !ok {
		b.l, b.err = format.Load(t, p.Schema, ask.Root)
		c.books[rev] = b
	}
	return t, b.l, b.err, nil
}

// listed answers the tree of rev, listed once, with no book read from it: the
// merge's own trees, the merge base and the other side, are read as they are.
// The tree is the cache's own: read it, never write it.
func (c *revCache) listed(ctx context.Context, p port.Ports, ask StagedAsk, rev string) (*revTree, error) {
	if c == nil {
		return newRevTree(ctx, p.Tree, p.History, ask.Root, rev, false)
	}
	if t, ok := c.trees[rev]; ok {
		return t, nil
	}
	t, err := newRevTree(ctx, p.Tree, p.History, ask.Root, rev, false)
	if err != nil {
		return nil, err
	}
	c.trees[rev] = t
	return t, nil
}

// fork is a tree of the same rev and files that a replay may write over
// without the tree it was taken from seeing it. The bytes read so far are
// shared by value, and a file read later is read into the fork alone.
func (t *revTree) fork() *revTree {
	c := *t
	c.files, c.data = maps.Clone(t.files), maps.Clone(t.data)
	return &c
}

// treeBefore is the tree a judge may write over: a fork when the tree is the
// cache's own, and the tree itself when no cache holds it.
func (c *revCache) treeBefore(t *revTree) *revTree {
	if c == nil {
		return t
	}
	return t.fork()
}
