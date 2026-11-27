package coreapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// checkGone is check in a working folder with no one rulebook (missing is
// ErrNoBook or ErrTwoBooks) and a base ref named: missing itself, the old
// check's, when the merge base holds no one rulebook either; a gate finding
// when it holds one; not ready when the merge base cannot be found.
func checkGone(ctx context.Context, p port.Ports, ask CheckAsk, missing error) (model.RulebookCheck, *model.Refusal, error) {
	out := model.RulebookCheck{BaseRef: ask.BaseRef, Unavailable: lifecycle.NotAvailable()}
	h, err := ask.OpenHistory()
	if r, ok := AsRefusal(err); ok {
		return out, r, nil
	}
	if err != nil {
		return out, nil, err
	}
	base, err := h.Base(ctx, ask.Root, ask.BaseRef, "HEAD")
	if r, ok := gitRefusal(err, ask.BaseRef); ok {
		return out, r, nil
	}
	if err != nil {
		return out, nil, err
	}
	at, err := newRevTree(ctx, p.Tree, h, ask.Root, base, false)
	if err != nil {
		return out, nil, err
	}
	book, err := format.Find(at, ask.Root) // its folder, "" for the root
	switch {
	case noOneBook(err):
		return model.RulebookCheck{}, nil, missing
	case err != nil:
		return out, nil, fmt.Errorf("the merge base's rulebook could not be found: %w", err)
	}
	out.Base = base
	out.Gate = lifecycle.Gate(lifecycle.GateIn{Gone: "the working folder " + holds(missing), GoneFile: join(book, format.IdentityFile)})
	return out, nil, ctx.Err()
}

// noOneBook says err is a folder or a tree with no rulebook.yaml, or more
// than one.
func noOneBook(err error) bool {
	return errors.Is(err, ErrNoBook) || errors.Is(err, ErrTwoBooks)
}

// holds says what a folder whose rulebook err could not find holds.
func holds(err error) string {
	if errors.Is(err, ErrTwoBooks) {
		return "holds more than one " + format.IdentityFile
	}
	return "holds no " + format.IdentityFile
}
