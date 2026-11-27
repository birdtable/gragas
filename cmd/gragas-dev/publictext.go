package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/scan"
)

// publicText is `scan public-text`: the tracked tree against the count in
// scan.PublicTextFile. With apply it lowers the counts that have fallen, and
// it starts the count when there is no file; it never raises one.
// With list it prints every hit, not only those over a count.
func (h *scanHost) publicText(ctx context.Context, apply, list bool, stdout, stderr io.Writer) (code int) {
	talk := &say.Speaker{W: stderr}
	fail := func(err error) int {
		talk.Text("gragas-dev scan public-text: " + err.Error() + "\n")
		return exitFailed
	}
	root, err := os.OpenRoot(h.root)
	if err != nil {
		return fail(err)
	}
	defer func() {
		if err := root.Close(); err != nil && code == exitDone {
			code = fail(err)
		}
	}()
	return h.publicTextIn(ctx, root, apply, list, stdout, stderr)
}

// publicTextIn is publicText with the repository's root open.
func (h *scanHost) publicTextIn(ctx context.Context, root *os.Root, apply, list bool, stdout, stderr io.Writer) int {
	talk, out := &say.Speaker{W: stderr}, &say.Speaker{W: stdout}
	fail := func(err error) int {
		talk.Text("gragas-dev scan public-text: " + err.Error() + "\n")
		return exitFailed
	}
	data, err := root.ReadFile(scan.PublicTextFile)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return fail(err)
	}
	base, err := scan.ParseTextBaseline(data)
	if err != nil {
		return fail(err)
	}
	m := h.machine()
	lines, v, err := scan.PublicText(ctx, m, base)
	if apply && missing && v.Today != nil {
		// A tree with no count file is a start: the counts are today's. A
		// file that is there, however few counts it holds, is only lowered.
		base.Counts = v.Today
		lines, v, err = scan.PublicText(ctx, m, base)
	}
	for _, l := range lines {
		out.Text(l + "\n")
	}
	if list {
		for _, hit := range v.Hits {
			out.Text(fmt.Sprintf("  %s:%d: %s\n", hit.Path, hit.Line, hit.Rule))
		}
	}
	if apply {
		next := v.Lowered.Format()
		if !bytes.Equal(next, data) {
			werr := root.MkdirAll(path.Dir(scan.PublicTextFile), 0o755)
			if werr == nil {
				werr = replaceIn(root, scan.PublicTextFile, next)
			}
			if werr != nil {
				return fail(werr)
			}
			out.Text("public-text: " + scan.PublicTextFile + " written\n")
		}
	}
	if err != nil {
		talk.Text("gragas-dev scan public-text: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}
