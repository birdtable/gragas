package coreapi

import (
	"context"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ChainAsk is what resolve reads besides the rulebook: the releases cache the
// parent is read from, and the contract's schemas its bundle is read by.
type ChainAsk struct {
	// Cache is the releases cache folder, "" for none.
	Cache string
	// Contracts is the Schema port over the contract's schemas.
	Contracts port.Schema
}

// ResolveChain is the effective rulebook of the rulebook at root: a rulebook
// with no parent is the one layer; an overlay reads its parent's bundle from the
// releases cache at its pin as a value (A50), and one that is not there is a
// *ParentError, which is ErrNotReady, naming the pin. Nothing is fetched (A10).
func ResolveChain(ctx context.Context, p port.Ports, root model.Root, ask ChainAsk) (model.ResolveReport, error) {
	if err := ctx.Err(); err != nil {
		return model.ResolveReport{}, err
	}
	book, dir, _, _, err := rulebookHere(ctx, p, root)
	if err != nil {
		return model.ResolveReport{}, err
	}
	var parents []model.ChainLayer
	doc, err := parentBundle(p, ask.Contracts, ask.Cache, book)
	if err != nil {
		return model.ResolveReport{}, err
	}
	if doc != nil {
		parents = append(parents, parentLayer(book.Identity.Extends.Rulebook, doc))
	}
	r, err := link.Resolve(book, parents)
	if err != nil {
		return model.ResolveReport{}, err
	}
	return model.ResolveReport{Dir: dir, Resolved: r}, nil
}
