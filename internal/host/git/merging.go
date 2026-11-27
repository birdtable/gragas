package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/gragas/internal/model"
)

// maxMergeHead is the cap of the MERGE_HEAD file: an object name a line, as
// many as an octopus merge brings in.
const maxMergeHead = 64 << 10

// Merging answers the commits the merge in progress in root's worktree
// brings in, from the MERGE_HEAD file git writes when a merge stops before
// its commit, and none when there is no such file. It reads the file
// itself, where git says the worktree keeps it, never a rev named
// MERGE_HEAD: with no file, rev-parse falls through to a tag or a branch of
// that name, and that is no merge. Each line names a commit
// the repository holds, or the answer is failed.
func (g *Git) Merging(ctx context.Context, root model.Root) ([]string, error) {
	dir, err := dirOf(root)
	if err != nil {
		return nil, err
	}
	if err := g.ready(ctx, dir); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, verbMergeHead, dir)
	if err != nil {
		return nil, g.answer("git rev-parse --git-path failed", err)
	}
	where, ok := bytes.CutSuffix(out, []byte{'\n'})
	// relative to the root for the main worktree, absolute for a linked
	// one; git for Windows prints forward slashes
	p := filepath.FromSlash(string(where))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if !ok || len(where) == 0 || bytes.ContainsAny(where, "\n\r\x00") || filepath.Base(p) != "MERGE_HEAD" {
		return nil, g.failed("git rev-parse --git-path printed what it does not print", fmt.Errorf("%q", clip(out)))
	}
	data, err := readMergeHead(p)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, g.failed("the MERGE_HEAD file could not be read", err)
	}
	heads, err := parseMergeHead(data)
	if err != nil {
		return nil, g.failed("the MERGE_HEAD file holds what git does not write", err)
	}
	for _, h := range heads {
		r, err := parseRev(h)
		if err != nil {
			return nil, g.failed("the MERGE_HEAD file holds what git does not write", err)
		}
		c, err := g.resolve(ctx, dir, r)
		if err != nil {
			return nil, err
		}
		if c.text != h {
			return nil, g.failed("the MERGE_HEAD file names what is not a commit", fmt.Errorf("%s peels to %s", h, c.text))
		}
	}
	return heads, nil
}

// readMergeHead reads the file at p, a regular file no larger than the
// cap, through a root at its folder, so no link at its name is followed.
func readMergeHead(p string) (_ []byte, err error) {
	r, err := os.OpenRoot(filepath.Dir(p))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	return confine.ReadCapped(r, filepath.Base(p), maxMergeHead)
}

// parseMergeHead reads the MERGE_HEAD file: one object name or more, each
// ended by a newline.
func parseMergeHead(data []byte) ([]string, error) {
	var heads []string
	for len(data) > 0 {
		line, rest, ok := bytes.Cut(data, []byte{'\n'})
		if !ok {
			return nil, errors.New("its last line is not ended by a newline")
		}
		v, err := parseOID(string(line))
		if err != nil {
			return nil, err
		}
		heads = append(heads, v.text)
		data = rest
	}
	if len(heads) == 0 {
		return nil, errors.New("it names no commit")
	}
	return heads, nil
}

// Commits answers the commits reachable from to and not from from, each
// with its parents, every commit after its parents; none is an empty list.
// A shallow clone is not ready: its first commits name no parent they have.
func (g *Git) Commits(ctx context.Context, root model.Root, from, to string) ([]model.Commit, error) {
	dir, err := dirOf(root)
	if err != nil {
		return nil, err
	}
	rf, err := parseRev(from)
	if err != nil {
		return nil, err
	}
	rt, err := parseRev(to)
	if err != nil {
		return nil, err
	}
	if err := g.ready(ctx, dir); err != nil {
		return nil, err
	}
	if err := g.notShallow(ctx, dir); err != nil {
		return nil, err
	}
	cf, err := g.resolve(ctx, dir, rf)
	if err != nil {
		return nil, err
	}
	ct, err := g.resolve(ctx, dir, rt)
	if err != nil {
		return nil, err
	}
	out, err := g.run(ctx, verbCommitsBetween, dir, ct, cf)
	if err != nil {
		return nil, g.lacking(ctx, dir, "git rev-list failed", err)
	}
	commits, err := parseParents(out)
	if err != nil {
		return nil, g.failed("git rev-list printed what it does not print", err)
	}
	return commits, nil
}

// parseParents reads what rev-list --parents prints: a line a commit, its
// object name and then its parents', one space apart. None is an empty list.
func parseParents(out []byte) ([]model.Commit, error) {
	commits := []model.Commit{}
	for len(out) > 0 {
		line, rest, ok := bytes.Cut(out, []byte{'\n'})
		if !ok {
			return nil, errors.New("a commit is not ended by a newline")
		}
		var c model.Commit
		for i, f := range strings.Split(string(line), " ") {
			v, err := parseOID(f)
			if err != nil {
				return nil, err
			}
			if i == 0 {
				c.ID = v.text
				continue
			}
			c.Parents = append(c.Parents, v.text)
		}
		commits = append(commits, c)
		out = rest
	}
	return commits, nil
}
