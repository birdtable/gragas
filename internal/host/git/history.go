package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// The History methods that read objects. Each checks what the caller hands
// it on its parsed form before anything runs, reads the git's band once
// (history-version-bands), and runs only verbs of the table. A git that is missing is a
// *NotReady naming programs.git and one unsafe to run a *Refused, as Who's
// are; a git too old for the reads, or one before 2.44 in a repository with
// a promisor remote, is a *NotReady; a git that hangs, prints past its cap,
// prints what the verb does not print or fails is a *Failed with the run id.
// A read a shallow clone cannot answer is a *NotReady naming
// git fetch --unshallow. Nothing is fetched: a missing object is a failure.

// dirOf is root as the folder git runs in.
func dirOf(root model.Root) (string, error) {
	if !filepath.IsAbs(string(root)) {
		return "", fmt.Errorf("the root %q is not absolute", string(root))
	}
	return string(root), nil
}

// Base answers the full object name of the one merge base of a and b, each a
// tag, a branch or a commit. None in common, or more than one, is a
// *NoBase. A shallow clone is not ready before merge-base runs, since on a
// cut history it answers a wrong commit.
func (g *Git) Base(ctx context.Context, root model.Root, a, b string) (string, error) {
	dir, err := dirOf(root)
	if err != nil {
		return "", err
	}
	ra, err := parseRev(a)
	if err != nil {
		return "", err
	}
	rb, err := parseRev(b)
	if err != nil {
		return "", err
	}
	if err := g.ready(ctx, dir); err != nil {
		return "", err
	}
	if err := g.notShallow(ctx, dir); err != nil {
		return "", err
	}
	ca, err := g.resolve(ctx, dir, ra)
	if err != nil {
		return "", err
	}
	cb, err := g.resolve(ctx, dir, rb)
	if err != nil {
		return "", err
	}
	out, err := g.run(ctx, verbBase, dir, ca, cb)
	if ex, ok := errors.AsType[*exited](err); ok && ex.code == 1 && len(ex.stderr) == 0 {
		return "", &NoBase{A: a, B: b}
	}
	if err != nil {
		return "", g.lacking(ctx, dir, "git merge-base failed", err)
	}
	bases, err := parseBases(out)
	if err != nil {
		return "", g.failed("git merge-base printed what it does not print", err)
	}
	if len(bases) != 1 {
		return "", &NoBase{A: a, B: b, Bases: len(bases)}
	}
	return bases[0].text, nil
}

// onTop is nil when dir is the top of its repository, and not ready when git
// found the repository above it: a listing then names its paths from that top,
// so each would be read in dir under a name that is another file's, or none.
// The answer is git's own, whether the folder git ran in is where the
// repository begins (it is in a work tree, and --show-prefix prints nothing there: a root inside .git is in no work tree); no path git printed
// is compared with dir, so a link, a case or a short name in dir cannot
// change it.
func (g *Git) onTop(ctx context.Context, dir string) error {
	out, err := g.run(ctx, verbWorkTop, dir)
	if err != nil {
		return g.answer("git rev-parse --is-inside-work-tree failed", err)
	}
	if string(out) != "true\n\n" {
		return belowTop()
	}
	return nil
}

// Files answers the regular files and the links of the tree at rev, or of
// the index when rev is "", sorted by path, a link marked by git's own mode
// field. A submodule is not a file and is left out. A rev the repository does not have is a *Missing; an index with
// a merge in progress is not ready.
func (g *Git) Files(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
	files, err := g.files(ctx, root, rev)
	if err != nil {
		return nil, err
	}
	out := make([]model.Entry, len(files))
	for i, f := range files {
		out[i] = f.model()
	}
	return out, nil
}

// Blob answers the whole content of the regular file at path as it is at
// rev, or in the index when rev is "". The path is checked before anything
// runs, and never enters git's argv: the file is found in the listing and
// read by its object name. A path the rev lacks, or one that is a link or a
// submodule, matches fs.ErrNotExist; a file past the cap is failed before it
// is read; content of another length than the listing's is failed.
func (g *Git) Blob(ctx context.Context, root model.Root, rev, path string) ([]byte, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	files, err := g.files(ctx, root, rev)
	if err != nil {
		return nil, err
	}
	i, found := slices.BinarySearchFunc(files, path, func(e entry, p string) int { return strings.Compare(e.path, p) })
	if !found || files[i].link {
		return nil, &fs.PathError{Op: "blob", Path: path, Err: fs.ErrNotExist}
	}
	f := files[i]
	if f.size > maxListing {
		return nil, g.failed("the file is past the cap", fmt.Errorf("%w: %q is %d bytes", errPastCap, path, f.size))
	}
	dir := string(root)
	out, err := g.run(ctx, verbBlob, dir, f.oid)
	if err != nil {
		return nil, g.lacking(ctx, dir, "git cat-file failed", err)
	}
	if int64(len(out)) != f.size {
		return nil, g.failed("git cat-file printed what it does not print",
			fmt.Errorf("%q is %d bytes, and the listing says %d", path, len(out), f.size))
	}
	// a copy, never nil: an empty file is an empty slice
	return append([]byte{}, out...), nil
}

// blobHeader is what a blob's header may take of a run's cap beyond its
// content: the object's name, " blob ", its size and the newline, and the
// newline that ends the content.
const blobHeader = 96

// Blobs answers the content of each of paths at rev, in the order given, as
// Blob answers each one alone: the paths are checked and found
// in one listing, and their objects are read by cat-file --batch with the
// names on stdin, a run for each cap's worth of bytes, one run for the files
// a commit reads. The first path Blob would refuse is the error, and no
// content is returned with it. An object git answers missing for, though the
// listing names it, is failed and never read as empty; a header whose size
// is not the listing's, or not the bytes that follow, is failed; a file past
// the cap is failed before anything runs.
func (g *Git) Blobs(ctx context.Context, root model.Root, rev string, paths []string) ([][]byte, error) {
	for _, p := range paths {
		if err := checkPath(p); err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 {
		return [][]byte{}, nil
	}
	files, err := g.files(ctx, root, rev)
	if err != nil {
		return nil, err
	}
	picked, err := g.pick(files, paths)
	if err != nil {
		return nil, err
	}
	dir := string(root)
	out := make([][]byte, 0, len(paths))
	for len(picked) > 0 {
		n, total := 0, int64(0)
		for n < len(picked) && (n == 0 || total+picked[n].size+blobHeader <= maxListing) {
			total += picked[n].size + blobHeader
			n++
		}
		var names strings.Builder
		for _, f := range picked[:n] {
			names.WriteString(f.oid.text + "\n")
		}
		raw, err := g.runWith(ctx, verbBlobs, dir, input{stdin: []byte(names.String())})
		if err != nil {
			return nil, g.lacking(ctx, dir, "git cat-file failed", err)
		}
		got, err := parseBlobs(raw, picked[:n])
		if err != nil {
			return nil, g.failed("git cat-file printed what it does not print", err)
		}
		out = append(out, got...)
		picked = picked[n:]
	}
	return out, nil
}

// pick finds each of paths among the listed files, as Blob does: a path that
// is absent or a link is fs.ErrNotExist, a file past the cap is failed.
func (g *Git) pick(files []entry, paths []string) ([]entry, error) {
	picked := make([]entry, len(paths))
	for i, p := range paths {
		at, found := slices.BinarySearchFunc(files, p, func(e entry, p string) int { return strings.Compare(e.path, p) })
		if !found || files[at].link {
			return nil, &fs.PathError{Op: "blob", Path: p, Err: fs.ErrNotExist}
		}
		if files[at].size > maxListing {
			return nil, g.failed("the file is past the cap", fmt.Errorf("%w: %q is %d bytes", errPastCap, p, files[at].size))
		}
		picked[i] = files[at]
	}
	return picked, nil
}

// Tags answers every tag, by name, with the commit it names, a tag of a tag
// peeled to its commit. None is an empty list. A tag that names no commit is
// failed, naming it.
func (g *Git) Tags(ctx context.Context, root model.Root) ([]model.Tag, error) {
	dir, err := dirOf(root)
	if err != nil {
		return nil, err
	}
	if err := g.ready(ctx, dir); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, verbTags, dir)
	if err != nil {
		return nil, g.lacking(ctx, dir, "git for-each-ref failed", err)
	}
	tags, err := parseTags(out)
	if err != nil {
		return nil, g.failed("git for-each-ref printed what it does not print", err)
	}
	slices.SortFunc(tags, func(a, b model.Tag) int { return strings.Compare(a.Name, b.Name) })
	if tags == nil {
		tags = []model.Tag{}
	}
	return tags, nil
}

// Messages answers the message of each commit of head that base lacks,
// sorted by commit. base and head are checked on their parsed form before
// anything runs: each is a full object name, never a ref name or an option,
// and each must name a commit of the repository by exactly that name, so a
// name of the other hash's length, a short name or a tag's object is
// refused. They reach git only after --end-of-options. Each message is
// read from its commit's object as stored, never through git log, which
// re-codes it from the encoding its author's header names and cuts it at a
// NUL. Only UTF-8 messages are read: a commit whose header names another
// encoding than UTF-8 is errHidden naming it and the encoding, and one whose
// message holds a NUL is errHidden naming it. A Latin-1 commit made with
// i18n.commitEncoding is refused, never re-coded. A range
// past a verb's cap is failed, never cut.
func (g *Git) Messages(ctx context.Context, root model.Root, base, head string) ([]model.Message, error) {
	dir, err := dirOf(root)
	if err != nil {
		return nil, err
	}
	b, err := fullName(base)
	if err != nil {
		return nil, err
	}
	h, err := fullName(head)
	if err != nil {
		return nil, err
	}
	if err := g.ready(ctx, dir); err != nil {
		return nil, err
	}
	if err := g.notShallow(ctx, dir); err != nil {
		return nil, err
	}
	for _, v := range []value{b, h} {
		if err := g.isCommit(ctx, dir, v); err != nil {
			return nil, err
		}
	}
	out, err := g.run(ctx, verbRange, dir, b, h)
	if err != nil {
		return nil, g.lacking(ctx, dir, "git rev-list failed", err)
	}
	ids, err := parseRange(out, len(b.text))
	if err != nil {
		return nil, g.failed("git rev-list printed what it does not print", err)
	}
	if len(ids) == 0 {
		return []model.Message{}, nil
	}
	var names strings.Builder
	for _, id := range ids {
		names.WriteString(id.text + "\n")
	}
	raw, err := g.runWith(ctx, verbCommits, dir, input{stdin: []byte(names.String())})
	if err != nil {
		return nil, g.lacking(ctx, dir, "git cat-file failed", err)
	}
	msgs, err := parseCommits(raw, ids)
	switch {
	case errors.Is(err, errHidden):
		return nil, err
	case err != nil:
		return nil, g.failed("git cat-file printed what it does not print", err)
	}
	slices.SortFunc(msgs, func(x, y model.Message) int { return strings.Compare(x.Commit, y.Commit) })
	return msgs, nil
}

// fullName checks an end of a range on its parsed form: a full object name
// and nothing else, so a ref name, a short name or an option is refused
// before anything runs.
func fullName(s string) (value, error) {
	v, err := parseOID(s)
	if err != nil {
		return value{}, fmt.Errorf("%w: an end of a range is a full object name: %w", errBadRev, err)
	}
	return v, nil
}

// files is the regular files and links at rev, or in the index for "", sorted by path.
func (g *Git) files(ctx context.Context, root model.Root, rev string) ([]entry, error) {
	dir, err := dirOf(root)
	if err != nil {
		return nil, err
	}
	var r value
	if rev != "" {
		if r, err = parseRev(rev); err != nil {
			return nil, err
		}
	}
	if err := g.ready(ctx, dir); err != nil {
		return nil, err
	}
	if err := g.onTop(ctx, dir); err != nil {
		return nil, err
	}
	var files []entry
	if rev == "" {
		files, err = g.index(ctx, dir)
	} else {
		files, err = g.tree(ctx, dir, r)
	}
	if err != nil {
		return nil, err
	}
	// a submodule is not a file: only Commit reads one
	files = slices.DeleteFunc(files, func(e entry) bool { return e.gitlink })
	slices.SortFunc(files, func(a, b entry) int { return strings.Compare(a.path, b.path) })
	for i := 1; i < len(files); i++ {
		if files[i].path == files[i-1].path {
			return nil, g.failed("git printed one path twice", fmt.Errorf("%q", files[i].path))
		}
	}
	if files == nil {
		files = []entry{}
	}
	return files, nil
}

// regular is files without its links and submodules: what Fetch writes is
// regular files alone, as before links were listed.
func regular(files []entry) []entry {
	return slices.DeleteFunc(files, func(e entry) bool { return e.link || e.gitlink })
}

// tree is the regular files, links and submodules of the commit rev names.
func (g *Git) tree(ctx context.Context, dir string, rev value) ([]entry, error) {
	c, err := g.resolve(ctx, dir, rev)
	if err != nil {
		return nil, err
	}
	// c is the commit's full object name, checked in dir just now, so its
	// listing is the one this value read before, if it did.
	if files, ok := g.trees[c.text]; ok {
		return slices.Clone(files), nil
	}
	out, err := g.run(ctx, verbTree, dir, c)
	if err != nil {
		return nil, g.lacking(ctx, dir, "git ls-tree failed", err)
	}
	files, err := parseTree(out)
	if err != nil {
		return nil, g.failed("git ls-tree printed what it does not print", err)
	}
	if g.trees == nil {
		g.trees = map[string][]entry{}
	}
	g.trees[c.text] = slices.Clone(files)
	return files, nil
}

// index is the regular files, links and submodules of the index.
func (g *Git) index(ctx context.Context, dir string) ([]entry, error) {
	out, err := g.run(ctx, verbIndex, dir)
	if err != nil {
		return nil, g.lacking(ctx, dir, "git ls-files failed", err)
	}
	files, unmerged, err := parseIndex(out)
	if err != nil {
		return nil, g.failed("git ls-files printed what it does not print", err)
	}
	if unmerged {
		return nil, mergeInProgress()
	}
	return files, nil
}

// resolve is the full object name of the commit rev names. A rev that names
// none (rev-parse -q exits 1, saying nothing) is a *Missing, or not ready
// in a shallow clone, where it may be past the cut.
func (g *Git) resolve(ctx context.Context, dir string, rev value) (value, error) {
	out, err := g.run(ctx, verbResolve, dir, rev)
	if ex, ok := errors.AsType[*exited](err); ok && ex.code == 1 && len(ex.stderr) == 0 {
		if err := g.notShallow(ctx, dir); err != nil {
			return value{}, err
		}
		return value{}, &Missing{Rev: rev.text}
	}
	if err != nil {
		return value{}, g.answer("git rev-parse failed", err)
	}
	c, err := parseOIDLine(out)
	if err != nil {
		return value{}, g.failed("git rev-parse printed what it does not print", err)
	}
	return c, nil
}

// notShallow is nil in a repository that is not a shallow clone, and the
// not ready of one that is.
func (g *Git) notShallow(ctx context.Context, dir string) error {
	out, err := g.run(ctx, verbShallow, dir)
	if err != nil {
		return g.answer("git rev-parse --is-shallow-repository failed", err)
	}
	switch string(out) {
	case "false\n":
		return nil
	case "true\n":
		return shallowClone("the commits asked for")
	}
	return g.failed("git rev-parse --is-shallow-repository printed what it does not print", fmt.Errorf("%q", clip(out)))
}

// lacking is the answer of a read that failed: in a shallow clone, where git
// exits 128 on an object past the cut, not ready; anywhere else, the
// failure of what, the run id beside it.
func (g *Git) lacking(ctx context.Context, dir, what string, err error) error {
	if ex, ok := errors.AsType[*exited](err); ok && ex.code == 128 {
		if serr := g.notShallow(ctx, dir); serr != nil {
			if _, nr := errors.AsType[*NotReady](serr); nr {
				return serr
			}
		}
	}
	return g.answer(what, err)
}
