package apply

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Remade is a plan made again over a tree that is not the one on disk, and
// the bytes each file it writes would hold after the write.
type Remade struct {
	// Plan is the sealed plan, its digest set as Preview sets it.
	Plan model.Plan
	// Files are the files the plan writes, its record last, each with the
	// whole content the write path would leave in it.
	Files []model.File
}

// Remake is the plan s makes over p's tree, sealed by the one seal Preview and
// Run plan with, so its digest is the digest the yes accepted when the tree is
// the one the plan was first made over. It takes no lock, makes no open-record
// check, since the tree is not the root's own, and writes nothing: guard staged
// makes a history line's command again through it over the tree git holds
// (arch.ports.19). ok is false with the outcome when the plan ends
// refused or failed, or has nothing to do.
func Remake(ctx context.Context, p port.Ports, root model.Root, s Spec) (Remade, Outcome, bool) {
	r := &run{ctx: ctx, jctx: context.WithoutCancel(ctx), p: p, root: root, s: s, out: Outcome{Run: s.Run}}
	if s.Plan == nil || s.Render == nil || s.Run == "" {
		return Remade{}, r.fail(FailWiring, errors.New("a plan made again needs a run id, a plan function and a render function")), false
	}
	if out, ok := r.seal(); !ok {
		return Remade{}, out, false
	}
	if empty(r.plan) {
		r.out.Word, r.out.Nothing = model.Done, true
		return Remade{}, r.out, false
	}
	out := Remade{Plan: r.plan}
	for _, t := range r.files {
		var old []byte
		if t.Append {
			b, err := p.Tree.Read(root, t.Path)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return Remade{}, r.fail(FailRead, fmt.Errorf("%s could not be read: %w", t.Path, err)), false
			}
			old = b
		}
		out.Files = append(out.Files, model.File{Path: t.Path, Data: expected(old, t)})
	}
	return out, Outcome{}, true
}
