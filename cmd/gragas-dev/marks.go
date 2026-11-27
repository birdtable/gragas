package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/birdtable/gragas/internal/dev/surface"
)

// gitBaseMarks is the surface marks file as the base revision has it, or nil
// when the base has none.
func gitBaseMarks(ctx context.Context, environ []string, dir, ref string) ([]byte, error) {
	return gitBaseFile(ctx, environ, dir, ref, surface.MarksFile)
}

// marksKept holds the head's change record of the surface to the base's: once
// the base's has moved to sums, the head keeps its base number as it is
// (surface.BaseKept). A base number is never edited, so a change cannot hide in
// it with no step and no reason. A marks file the head breaks is a finding; one
// the base breaks fails the check, since nothing can be held to it.
func marksKept(ctx context.Context, m machine, root *os.Root, ref string) ([]string, error) {
	if m.baseMarks == nil {
		return nil, nil
	}
	data, err := m.baseMarks(ctx, root.Name(), ref)
	if err != nil || data == nil {
		return nil, err
	}
	main, err := surface.ParseMarks(data)
	if err != nil {
		return nil, fmt.Errorf("the base's %s, which the head's change record is held to: %w", surface.MarksFile, err)
	}
	var head *surface.MarkFile
	text, err := fs.ReadFile(root.FS(), surface.MarksFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if head, err = surface.ParseMarks(text); err != nil {
			return []string{surface.MarksFile + ": " + err.Error()}, nil
		}
	}
	if why := surface.BaseKept(main, head); why != "" {
		return []string{why}, nil
	}
	return nil, nil
}
