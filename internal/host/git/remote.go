package git

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-proc/gitenv"

	"github.com/birdtable/gragas/internal/digest"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
)

// The Remote methods. Each checks what the caller hands it on its
// parsed form before anything runs, and runs only verbs of the table, each
// with core.hooksPath at an empty folder of the run's own, so no hook runs.
// None of them pushes, and none stages a path that was not named.

// errNoChange is a commit whose named paths hold what HEAD holds.
var errNoChange = errors.New("the path holds what HEAD holds, so there is nothing to commit")

// Fetch fetches the tag of the repository at from into the releases cache
// and answers the folder holding the files of its commit. from is an
// absolute path to a repository or a bare mirror on this machine, or an
// https:// or ssh:// URL; any other form is refused. Before
// anything runs, a path's source and its git dir are held to this account
// or root (holdSource), and that git dir is what git is handed, with
// upload-pack --strict, so git opens it and no folder beside it; a host's
// helper or ssh, and its known_hosts, are walked (transport). The fetch
// runs with every protocol but its own refused, no git config of the
// account's, no credential helper but the one the settings name, every
// object checked, and no gc (run.go).
// The tag is fetched to the same name in a bare repository of the cache,
// which never moves a tag it already holds, and its tree is read through
// the listing, which refuses a path that climbs, names .git or is no plain
// name; a link or a submodule is not a file and is not written. The files
// go to a folder of their own, named by the commit, which is renamed into
// place whole, so a failure changes nothing in the cache and a tag fetched
// again is answered as it is.
func (g *Git) Fetch(ctx context.Context, from, tag string) (_ model.Root, err error) {
	src, err := parseSource(from)
	if err != nil {
		return "", err
	}
	t, err := parseTag(tag)
	if err != nil {
		return "", err
	}
	fetch, vals, err := g.transport(ctx, src)
	if err != nil {
		return "", err
	}
	if g.cfg.Releases == "" {
		return "", errors.New("the adapter was made with no releases cache, so nothing can be fetched")
	}
	rels, err := parsePath(g.cfg.Releases)
	if err != nil {
		return "", fmt.Errorf("the releases cache: %w", err)
	}
	sum := digest.Sum([]byte(src.text))
	name := hex.EncodeToString(sum[:16])
	repo := filepath.Join(rels.text, name+".git")
	cache, err := g.openCache(rels.text)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, cache.Close()) }()
	if err := private(cache, name+".git"); err != nil {
		return "", g.failed("the releases cache could not be made", err)
	}
	w, err := newWork(rels.text)
	if err != nil {
		return "", g.failed("the releases cache could not be written", err)
	}
	defer g.done(w, &err)
	if err := g.cache(ctx, repo, w); err != nil {
		return "", err
	}
	if _, err := g.runWith(ctx, fetch, repo, input{hooks: w.hooks, home: w.home}, append(vals, t)...); err != nil {
		if fetch == verbFetchSSH && hostKeyUnknown(err) {
			host, port := hostOf(src.text)
			return "", knownHosts(host, port).refused("ssh found that the account's known_hosts holds no key, or another key, for the host "+host+" (port "+cmp.Or(port, "22")+")", "")
		}
		return "", g.answer("git fetch of the tag "+tag+" failed", err)
	}
	ref, err := parseRev("refs/tags/" + t.text)
	if err != nil {
		return "", err
	}
	commit, err := g.resolve(ctx, repo, ref)
	if err != nil {
		return "", err
	}
	files, err := g.tree(ctx, repo, ref)
	if err != nil {
		return "", err
	}
	files = regular(files)
	folder := filepath.Join(rels.text, name, commit.text)
	if info, err := os.Lstat(folder); err == nil {
		if !info.IsDir() {
			return "", g.failed("the releases cache holds what it does not hold", fmt.Errorf("%s is not a folder", folder))
		}
		return model.Root(folder), nil
	}
	if err := g.unpack(ctx, cache, repo, name, folder, files); err != nil {
		return "", err
	}
	return model.Root(folder), nil
}

// parseSource checks where a release is fetched from: an absolute, clean
// path, or an https:// or ssh:// URL as parseURL holds one. Anything else,
// a transport helper's <name>::, http://, file://, a relative path or
// host:path, is refused.
func parseSource(from string) (value, error) {
	if strings.HasPrefix(from, "https://") || strings.HasPrefix(from, "ssh://") {
		return parseURL(from)
	}
	v, err := parsePath(from)
	if err != nil || strings.Contains(from, "::") {
		return value{}, fmt.Errorf("a release is fetched from an absolute path on this machine or an https:// or ssh:// URL, and %q is none", clip([]byte(from)))
	}
	return v, nil
}

// transport answers the verb that fetches from src and the values of its
// slots before the tag, once every path it runs or reads is checked, and
// before anything runs. A path is held as holdSource holds it. An https://
// host is fetched with no credential, or with the helper programs.credential
// names and no other; an ssh:// host through the ssh programs.ssh names,
// with the account's known_hosts. A helper or ssh path is walked as git is,
// and refused when its setting or the path it resolves to cannot go in one
// pair of single quotes.
func (g *Git) transport(ctx context.Context, src value) (string, []value, error) {
	switch {
	case src.kind == kindPath:
		gitDir, err := g.holdSource(ctx, src.text)
		if err != nil {
			return "", nil, err
		}
		held, err := parsePath(gitDir)
		if err != nil {
			return "", nil, fmt.Errorf("the git dir of the source: %w", err)
		}
		return verbFetch, []value{held}, nil
	case strings.HasPrefix(src.text, "https://") && g.cfg.Credential == "":
		return verbFetchHTTPS, []value{src}, nil
	case strings.HasPrefix(src.text, "https://"):
		helper, err := g.shellPath(ctx, credentialHelper(), g.cfg.Credential)
		if err != nil {
			return "", nil, err
		}
		line, err := helperLine(helper)
		if err != nil {
			return "", nil, credentialHelper().refused(err.Error(), "")
		}
		return verbFetchHelper, []value{line, src}, nil
	}
	ssh, err := g.shellPath(ctx, sshProgram(), g.cfg.SSH)
	if err != nil {
		return "", nil, err
	}
	s := knownHosts(hostOf(src.text))
	kh, err := g.knownHostsFile(ctx, s)
	if err != nil {
		return "", nil, err
	}
	line, err := sshLine(ssh, kh)
	if err != nil {
		return "", nil, s.refused(err.Error(), "")
	}
	return verbFetchSSH, []value{line, src}, nil
}

// shellPath answers set, the path s names, walked as git is, when it
// and the path it resolves to can each go in one pair of single quotes.
func (g *Git) shellPath(ctx context.Context, s subject, set string) (string, error) {
	if unquotable(set) {
		return "", s.refused(s.setting+" is "+strconv.Quote(set)+", which holds a single quote or a control character, and git hands it to its shell", "")
	}
	path, err := g.trusted(s, set)
	if err != nil {
		return "", err
	}
	if _, err := quoted(path); err != nil {
		return "", s.refused(s.setting+" resolves to "+strconv.Quote(path)+", which holds a single quote or a control character, and git hands it to its shell", "")
	}
	return path, nil
}

// knownHostsFile answers the account's known_hosts, ~/.ssh/known_hosts in
// the home the adapter was given, walked as a file that is read: root's or
// this account's, and written by no one else. None is not ready, with the
// line that adds the host's key by hand, and never by the fetch.
func (g *Git) knownHostsFile(ctx context.Context, s subject) (string, error) {
	if g.cfg.Home == "" {
		return "", s.missing("cannot be found, since the adapter was given no home folder")
	}
	return g.walkFor(s, filepath.Join(g.cfg.Home, ".ssh", "known_hosts"))
}

// hostOf is the host of a URL parseURL holds, and its port, "" when it
// names none.
func hostOf(u string) (host, port string) {
	_, rest, _ := strings.Cut(u, "://")
	authority, _, _ := strings.Cut(rest, "/")
	if _, after, has := strings.Cut(authority, "@"); has {
		authority = after
	}
	host, port, _ = strings.Cut(authority, ":")
	return host, port
}

// hostKeyUnknown is whether err is git's exit with ssh's own line for a host
// key that known_hosts does not hold, or holds another key for. That exact
// line may decide which words the fetch's refusal says, and nothing else:
// the fetch has ended either way.
func hostKeyUnknown(err error) bool {
	ex, ok := errors.AsType[*exited](err)
	if !ok {
		return false
	}
	for line := range strings.SplitSeq(string(ex.stderr), "\n") {
		if strings.TrimSuffix(line, "\r") == "Host key verification failed." {
			return true
		}
	}
	return false
}

// cache makes the bare repository of the cache at repo, once.
func (g *Git) cache(ctx context.Context, repo string, w work) error {
	if _, err := os.Lstat(filepath.Join(repo, "HEAD")); errors.Is(err, fs.ErrNotExist) {
		if _, err := g.runWith(ctx, verbInit, repo, input{hooks: w.hooks}); err != nil {
			return g.answer("git init of the releases cache failed", err)
		}
	}
	return g.ready(ctx, repo)
}

// unpack writes files, read from repo by object name, to a new folder beside
// folder, fresh by confine.MkdirFresh, in the private folder name of the
// cache, and moves it to folder
// with confine.RenameNoReplace, which never replaces. A folder
// the race made first is kept: fs.ErrExist is the fetch done. A system that
// cannot move without replacing (errors.ErrUnsupported) is passed on as it
// is, never tried again as a rename that may replace.
func (g *Git) unpack(ctx context.Context, cache *os.Root, repo, name, folder string, files []entry) (err error) {
	parent, err := confine.MkdirPrivate(cache, name)
	if err != nil {
		return g.failed("the releases cache could not be made", err)
	}
	defer func() { err = errors.Join(err, parent.Close()) }()
	made, tmp, err := confine.MkdirFresh(parent, ".fetch-")
	if err != nil {
		return g.failed("the releases cache could not be written", err)
	}
	// after the rename there is no tmp, and removing it is no error
	defer func() { err = errors.Join(err, confine.RemoveAll(parent, tmp)) }()
	err = g.writeFiles(ctx, repo, made, files)
	if err = errors.Join(err, made.Close()); err != nil {
		return err
	}
	if err := g.rename(parent, tmp, filepath.Base(folder)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return g.failed("the fetched files could not be put in the cache", err)
	}
	return nil
}

// rename moves from to to, both in root, with confine.RenameNoReplace, or
// through g.renaming when a test of this package set it.
func (g *Git) rename(root *os.Root, from, to string) error {
	if g.renaming != nil && testing.Testing() {
		return g.renaming(root, from, to)
	}
	return confine.RenameNoReplace(root, from, to)
}

// openCache answers the releases cache dir as a root, made by privateRoot:
// a name the walk refuses is a *Refused in the cache's words, with the walk's
// observation and fix line; any other error is failed.
func (g *Git) openCache(dir string) (*os.Root, error) {
	cache, err := privateRoot(dir, g.cfg.Mounts)
	if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		return nil, releasesCache().refused("the releases cache "+dir+" is not trusted: "+u.Observed, u.Fix)
	}
	if err != nil {
		return nil, g.failed("the releases cache could not be made", err)
	}
	return cache, nil
}

// privateRoot answers dir, the releases cache, as a root, once every name of
// it that is there is judged and what is missing is made (as the
// sandbox's make): host/trust's Private walks the nearest folder
// of dir that is there, dir itself when it is, as releasesCache's folder,
// top down, links followed, before anything is made or opened, so no folder
// and no repository is made through a link in a folder another account may
// write. Each folder that is missing is then made private, with no link
// followed in what is made. A folder that was there keeps its
// mode, as for the sandbox and host/fs's privateDirApart. The walk trusts
// mounts, the configured ones, as walkFor's does.
func privateRoot(dir string, mounts []string) (*os.Root, error) {
	s := releasesCache()
	return hosttrust.Private(dir, hosttrust.Subject{Name: s.name, File: s.file, Last: s.last, Bare: true}, hosttrust.Who{Mounts: mounts})
}

// private makes the folder name in root, private, by go-files'
// confine.MkdirPrivate: owner only whatever the umask, and on Windows an
// access list only this account and SYSTEM reach.
func private(root *os.Root, name string) error {
	made, err := confine.MkdirPrivate(root, name)
	if err != nil {
		return err
	}
	return made.Close()
}

// writeFiles writes each file, through root, the fresh folder that no name
// leaves, made new and never opened twice, so no link is met or followed.
func (g *Git) writeFiles(ctx context.Context, repo string, root *os.Root, files []entry) error {
	for _, f := range files {
		if f.size > maxListing {
			return g.failed("a fetched file is past the cap", fmt.Errorf("%w: %q is %d bytes", errPastCap, f.path, f.size))
		}
		data, err := g.run(ctx, verbBlob, repo, f.oid)
		if err != nil {
			return g.answer("git cat-file of a fetched file failed", err)
		}
		if int64(len(data)) != f.size {
			return g.failed("git cat-file printed what it does not print", fmt.Errorf("%q is %d bytes, and the listing says %d", f.path, len(data), f.size))
		}
		name := filepath.FromSlash(f.path)
		if d := filepath.Dir(name); d != "." {
			if err := root.MkdirAll(d, 0o755); err != nil {
				return g.failed("the releases cache could not be written", err)
			}
		}
		// each mode is the open's own permission, never a helper's argument;
		// the file must not be there
		var out *os.File
		var werr error
		if f.exec {
			out, werr = root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		} else {
			out, werr = root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		}
		if werr == nil {
			werr = writeAll(out, data)
		}
		if werr != nil {
			return g.failed("the releases cache could not be written", werr)
		}
	}
	return nil
}

// writeAll writes data to out and closes it.
func writeAll(out *os.File, data []byte) (err error) {
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = out.Write(data)
	return err
}

// work is a run's own folder of a write: hooks, an empty folder git
// takes as core.hooksPath, so no hook runs, home, an empty folder a verb
// with no home takes as its home, and index, the path of an index of the
// run's own.
type work struct {
	dir                string
	hooks, home, index value
}

// newWork makes a work folder in parent, "" being the system's, fresh and
// private by go-files' confine.MkdirFresh, and its two empty folders in it
// private by confine.MkdirPrivate.
func newWork(parent string) (_ work, err error) {
	top, err := os.OpenRoot(cmp.Or(parent, os.TempDir()))
	if err != nil {
		return work{}, err
	}
	defer func() { err = errors.Join(err, top.Close()) }()
	root, name, err := confine.MkdirFresh(top, ".gragas-")
	if err != nil {
		return work{}, err
	}
	dir := filepath.Join(top.Name(), name)
	hooks, herr := parsePath(filepath.Join(dir, "hooks"))
	home, merr := parsePath(filepath.Join(dir, "home"))
	index, ierr := parsePath(filepath.Join(dir, "index"))
	err = errors.Join(herr, merr, ierr)
	if err == nil {
		err = errors.Join(private(root, "hooks"), private(root, "home"))
	}
	if err = errors.Join(err, root.Close()); err != nil {
		return work{}, errors.Join(err, confine.RemoveAll(top, name))
	}
	return work{dir: dir, hooks: hooks, home: home, index: index}, nil
}

// removeAll removes dir and everything beneath it by go-files'
// confine.RemoveAll, in a root of the folder above, so a link met in it is
// removed as the link and never walked into. A dir that is not
// there is nil.
func removeAll(dir string) error {
	top, err := os.OpenRoot(filepath.Dir(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.Join(confine.RemoveAll(top, filepath.Base(dir)), top.Close())
}

// done removes the work folder of w, joining a failure to *err: a folder
// of the run's own left behind is a fault to say, though the write it
// served was made.
func (g *Git) done(w work, err *error) {
	if rerr := removeAll(w.dir); rerr != nil {
		*err = errors.Join(*err, g.failed("the work folder "+w.dir+" could not be removed", rerr))
	}
}

// change is one named path of a commit: its line of index info, and
// whether it is a removal.
type change struct {
	path, mode string
	oid        value
	gone       bool
}

// Commit commits the named paths of root, the top of its repository, on
// HEAD, with message, and answers the new commit. Each path is checked on
// its parsed form; none enters an argv. Each file is read once, through a
// root no name leaves, and refused when any folder on its way or the file
// itself is a link or is no regular file; a path with no file is its
// removal. A path HEAD holds as a link or a submodule, or one below a folder
// HEAD holds as either, is refused, as nothing is committed over one.
// The commit is made by plumbing alone: the blobs are stored with
// no filter, the tree is HEAD's with the named paths set in an index of
// the run's own, so nothing else staged or changed is in it, and HEAD is
// moved only if it still names the parent. No hook runs. Then the
// repository's index is set for the named paths alone. No path, a path
// named twice, a path whose content is HEAD's, or an empty message is an
// error and makes no commit.
func (g *Git) Commit(ctx context.Context, root model.Root, paths []string, message string) (_ string, err error) {
	dir, err := dirOf(root)
	if err != nil {
		return "", err
	}
	if err := checkMessage(message); err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", errors.New("a commit names at least one path")
	}
	for i, p := range paths {
		if err := checkPath(p); err != nil {
			return "", err
		}
		if slices.Contains(paths[:i], p) {
			return "", fmt.Errorf("the path %q is named twice", p)
		}
	}
	if _, err := g.Who(ctx, root); err != nil {
		return "", err
	}
	if err := g.ready(ctx, dir); err != nil {
		return "", err
	}
	if err := g.atTop(ctx, dir); err != nil {
		return "", err
	}
	headRev, err := parseRev("HEAD")
	if err != nil {
		return "", err
	}
	head, err := g.resolve(ctx, dir, headRev)
	if err != nil {
		return "", err
	}
	// before keeps HEAD's links: a file at a path HEAD holds as one is
	// refused by its mode, 120000
	before, err := g.tree(ctx, dir, headRev)
	if err != nil {
		return "", err
	}
	w, err := newWork("")
	if err != nil {
		return "", g.failed("the commit's folder could not be made", err)
	}
	defer g.done(w, &err)
	changes, err := g.changes(ctx, dir, w, paths, before, len(head.text))
	if err != nil {
		return "", err
	}
	return g.commitOn(ctx, dir, w, head, changes, message)
}

// commitOn makes the commit of changes on head and moves HEAD to it.
func (g *Git) commitOn(ctx context.Context, dir string, w work, head value, changes []change, message string) (string, error) {
	info := indexInfo(changes, len(head.text))
	own := input{index: w.index, hooks: w.hooks}
	steps := []struct {
		verb, what string
		in         input
		vals       []value
	}{
		{verbReadTree, "git read-tree failed", own, []value{head}},
		{verbIndexInfo, "git update-index failed", input{index: w.index, hooks: w.hooks, stdin: info}, nil},
	}
	for _, s := range steps {
		if _, err := g.runWith(ctx, s.verb, dir, s.in, s.vals...); err != nil {
			return "", g.answer(s.what, err)
		}
	}
	out, err := g.runWith(ctx, verbWriteTree, dir, own)
	if err != nil {
		return "", g.answer("git write-tree failed", err)
	}
	tree, err := parseOIDLine(out)
	if err != nil {
		return "", g.failed("git write-tree printed what it does not print", err)
	}
	msg := strings.TrimRight(message, "\n") + "\n"
	out, err = g.runWith(ctx, verbCommitTree, dir, input{stdin: []byte(msg), hooks: w.hooks}, head, tree)
	if err != nil {
		return "", g.answer("git commit-tree failed", err)
	}
	commit, err := parseOIDLine(out)
	if err != nil {
		return "", g.failed("git commit-tree printed what it does not print", err)
	}
	if _, err := g.runWith(ctx, verbUpdateRef, dir, input{hooks: w.hooks}, commit, head); err != nil {
		return "", g.answer("git update-ref failed, so HEAD is as it was", err)
	}
	if _, err := g.runWith(ctx, verbIndexReal, dir, input{stdin: info, hooks: w.hooks}); err != nil {
		return commit.text, g.failed("the commit "+commit.text+" is made, but the index could not be set to it for the named paths", err)
	}
	return commit.text, nil
}

// atTop is nil when dir is the top of its repository: a named path is
// relative to it, as HEAD's tree is.
func (g *Git) atTop(ctx context.Context, dir string) error {
	out, err := g.run(ctx, verbPrefix, dir)
	if err != nil {
		return g.answer("git rev-parse --show-prefix failed", err)
	}
	if string(out) != "\n" {
		return fmt.Errorf("the root %s is not the top of its repository, so its paths are not the tree's", dir)
	}
	return nil
}

// changes reads each named path once and stores it as a blob, or notes its
// removal, and refuses a path whose content and mode are HEAD's. The mode
// of a file is go-proc's gitenv.CommitMode, git's own rule, of the entry
// read, the repository's core.fileMode and HEAD's mode for the path,
// which refuses a file at a path HEAD holds as a link or a
// submodule. No commit changes the kind of what HEAD holds: a path below
// what HEAD holds as a link, a submodule or a file is refused before it is
// read, and a file where HEAD holds a folder is
// refused once it is read. before is HEAD's files, links and submodules, in
// git's order, which parseTree holds it to.
func (g *Git) changes(ctx context.Context, dir string, w work, paths []string, before []entry, width int) (_ []change, err error) {
	fileMode, err := g.fileMode(ctx, dir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, g.failed("the root could not be opened", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	out := make([]change, 0, len(paths))
	for _, p := range paths {
		c, err := g.changeOf(ctx, named{dir: dir, w: w, root: root, fileMode: fileMode, width: width}, before, p)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// named is what changes reads every path with: the repository's folder, the
// run's work, the root no name leaves, core.fileMode and the width of an
// object name.
type named struct {
	dir      string
	w        work
	root     *os.Root
	fileMode bool
	width    int
}

// changeOf is the one change of the named path p, as changes says.
func (g *Git) changeOf(ctx context.Context, n named, before []entry, p string) (change, error) {
	if err := underHeld(before, p); err != nil {
		return change{}, err
	}
	i, had := slices.BinarySearchFunc(before, p, byPath)
	data, info, err := g.readNamed(n.root, p)
	if err != nil {
		return change{}, err
	}
	if info == nil {
		// a link or a submodule HEAD holds and the tree does not is no
		// change, never its removal
		if !had || before[i].link || before[i].gitlink {
			return change{}, fmt.Errorf("commit %s: %w", p, errNoChange)
		}
		return change{path: p, gone: true}, nil
	}
	if err := overHeld(before, p); err != nil {
		return change{}, err
	}
	if err := foldedOnto(n.root, before, p); err != nil {
		return change{}, err
	}
	oid, err := g.hash(ctx, n.dir, n.w, data, n.width)
	if err != nil {
		return change{}, err
	}
	head := ""
	if had {
		head = before[i].fileMode()
	}
	// the mode is judged on info, the entry checked and found the same
	// file as the one opened, never by a second lookup of the name, which
	// would follow a link swapped in after the check
	mode, err := gitenv.CommitMode(info, n.fileMode, head)
	if err != nil {
		return change{}, fmt.Errorf("commit %s: %w", p, err)
	}
	if had && before[i].oid.text == oid.text && head == mode {
		return change{}, fmt.Errorf("commit %s: %w", p, errNoChange)
	}
	return change{path: p, mode: mode, oid: oid}, nil
}

// underHeld refuses p when a folder above it is one HEAD holds as a link
// (120000), a submodule (160000) or a file. git's index takes the new file
// in place of what HEAD holds there and drops that, unnamed: a commit of
// ld/y where HEAD's ld is a link replaces the link with a folder, one of
// sub/y puts a file of this repository where the submodule was,
// and one of f/y where HEAD's f is a file drops f.
// The check reads HEAD's tree listing, before, in git's
// order, and nothing on the disk.
func underHeld(before []entry, p string) error {
	for i := range len(p) {
		if p[i] != '/' {
			continue
		}
		j, ok := slices.BinarySearchFunc(before, p[:i], byPath)
		switch {
		case !ok:
		case before[j].link || before[j].gitlink:
			return fmt.Errorf("commit %s: HEAD holds %s as %s, and no file is committed over one or below it", p, p[:i], heldAs(before[j]))
		default:
			return fmt.Errorf("commit %s: HEAD holds %s as a file, %s, and no folder is committed where it stands: remove %s first, by its own name", p, p[:i], before[j].fileMode(), p[:i])
		}
	}
	return nil
}

// overHeld refuses a file at p where HEAD holds a folder, so where HEAD
// holds paths below p. git's index would drop each of them, unnamed, the
// links and submodules among them. The first one, in git's
// order, is named. Paths below p/ stand together in before, from where p/
// would be.
func overHeld(before []entry, p string) error {
	k, _ := slices.BinarySearchFunc(before, p+"/", byPath)
	if k < len(before) && strings.HasPrefix(before[k].path, p+"/") {
		return fmt.Errorf("commit %s: HEAD holds %s as a folder, with %s in it, and no file is committed where it stands: remove what it holds first, each by its own name", p, p, before[k].path)
	}
	return nil
}

// foldedOnto refuses p when a name on its way, a folder above it or its own,
// is spelled otherwise than a name HEAD holds in the same folder that this
// volume reads as the same file, as a disk that folds case reads LD and ld.
// git's index compares by bytes and would take both, so LD/y would commit
// beside HEAD's link ld, and the tree would then hold one folder under two
// spellings, one of them the link. Which names are one is the
// volume's and the folder's to say, so go-files' confine.Collide judges it,
// one name against one name in the folder that holds both. Each
// name of p is on the disk when it runs, as p was just read through each,
// so Collide makes no probe. A folder below one HEAD holds in no spelling
// is not read, as HEAD holds nothing in it.
func foldedOnto(root *os.Root, before []entry, p string) (err error) {
	parts := strings.Split(p, "/")
	dir := root
	defer func() {
		if dir != root {
			err = errors.Join(err, dir.Close())
		}
	}()
	for k, name := range parts {
		held := heldIn(before, strings.Join(parts[:k], "/"))
		if len(held) == 0 {
			return nil
		}
		if err := spelledApart(dir, p, parts[:k], name, held); err != nil {
			return err
		}
		if k == len(parts)-1 {
			return nil
		}
		below, err := openBelow(dir, name)
		if err != nil {
			return fmt.Errorf("commit %s: %w", p, err)
		}
		if dir != root {
			if err := dir.Close(); err != nil {
				return errors.Join(fmt.Errorf("commit %s: %w", p, err), below.Close())
			}
		}
		dir = below
	}
	return nil
}

// spelledApart refuses name, a name of p in the folder dir that the names
// above holds, when the folder reads it as one of held, HEAD's names there,
// spelled otherwise, naming HEAD's spelling.
func spelledApart(dir *os.Root, p string, above []string, name string, held []string) error {
	at := func(n string) string { return strings.Join(append(slices.Clip(above), n), "/") }
	for _, h := range held {
		if h == name {
			continue
		}
		same, err := confine.Collide(dir, name, h)
		if err != nil {
			return fmt.Errorf("commit %s: %w", p, err)
		}
		if same {
			return fmt.Errorf("commit %s: HEAD holds %s, a name this folder reads as %s, and no second spelling of one name is committed beside it: name it as HEAD spells it", p, at(h), at(name))
		}
	}
	return nil
}

// heldIn is the names HEAD holds in the folder under, "" for the top: the
// first part of each path below it, each once, in git's order.
func heldIn(before []entry, under string) []string {
	prefix := ""
	if under != "" {
		prefix = under + "/"
	}
	var names []string
	k, _ := slices.BinarySearchFunc(before, prefix, byPath)
	for _, e := range before[k:] {
		rest, ok := strings.CutPrefix(e.path, prefix)
		if !ok {
			break
		}
		name, _, _ := strings.Cut(rest, "/")
		if n := len(names); n == 0 || names[n-1] != name {
			names = append(names, name)
		}
	}
	return names
}

// openBelow opens the folder name in dir as a root of its own, refusing a
// link or no folder there, and one changed between the look and the open,
// as readNamed opens a file.
func openBelow(dir *os.Root, name string) (*os.Root, error) {
	seen, err := dir.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !seen.IsDir() {
		return nil, fmt.Errorf("%s is a link or no folder, and no link is followed on a named path", name)
	}
	below, err := dir.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := below.Stat(".")
	if err == nil && !os.SameFile(seen, opened) {
		err = fmt.Errorf("%s changed between the check and the open", name)
	}
	if err != nil {
		return nil, errors.Join(err, below.Close())
	}
	return below, nil
}

// byPath orders an entry against a path as git's listing does, by bytes.
func byPath(e entry, p string) int { return strings.Compare(e.path, p) }

// heldAs names what HEAD holds e as, with git's mode for it.
func heldAs(e entry) string {
	if e.gitlink {
		return "a submodule, 160000"
	}
	return "a link, 120000"
}

// hash stores data as a blob and answers its object id, which is width hex
// digits long, the repository's hash's.
func (g *Git) hash(ctx context.Context, dir string, w work, data []byte, width int) (value, error) {
	o, err := g.runWith(ctx, verbHash, dir, input{stdin: data, hooks: w.hooks})
	if err != nil {
		return value{}, g.answer("git hash-object failed", err)
	}
	oid, err := parseOIDLine(o)
	if err != nil || len(oid.text) != width {
		return value{}, g.failed("git hash-object printed what it does not print", errors.Join(err, fmt.Errorf("%q", clip(o))))
	}
	return oid, nil
}

// fileMode is the repository's core.fileMode as git reads it: true or
// false, and true when it is not set, git's default, as git config exits 1
// and prints nothing.
func (g *Git) fileMode(ctx context.Context, dir string) (bool, error) {
	out, err := g.run(ctx, verbFileMode, dir)
	if ex, ok := errors.AsType[*exited](err); ok && ex.code == 1 && len(ex.stderr) == 0 {
		return true, nil
	}
	if err != nil {
		return false, g.answer("git config could not be read", err)
	}
	switch string(out) {
	case "true\n":
		return true, nil
	case "false\n":
		return false, nil
	}
	return false, g.failed("git config printed what it does not print", fmt.Errorf("%q", clip(out)))
}

// readNamed reads the regular file at the slash path p in root, and answers
// its entry: each folder on the way and the file are read with Lstat, and a
// link or anything but a folder or a file refuses it; the file opened must
// be the one read. A path with nothing at it is no entry and no error.
func (g *Git) readNamed(root *os.Root, p string) (data []byte, _ fs.FileInfo, err error) {
	parts := strings.Split(p, "/")
	var info fs.FileInfo
	for i := range parts {
		name := filepath.Join(parts[:i+1]...)
		got, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("commit %s: %w", p, err)
		}
		switch last := i == len(parts)-1; {
		case !last && !got.IsDir():
			return nil, nil, fmt.Errorf("commit %s: %s is a link or no folder, and no link is followed on a named path", p, filepath.ToSlash(name))
		case last && !got.Mode().IsRegular():
			return nil, nil, fmt.Errorf("commit %s: it is a link or no regular file, and no link is followed on a named path", p)
		}
		info = got
	}
	if g.opening != nil && testing.Testing() {
		g.opening(filepath.FromSlash(p))
	}
	f, err := root.Open(filepath.FromSlash(p))
	if err != nil {
		return nil, nil, fmt.Errorf("commit %s: %w", p, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	opened, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("commit %s: %w", p, err)
	}
	if !os.SameFile(opened, info) {
		return nil, nil, fmt.Errorf("commit %s: it changed between the check and the open", p)
	}
	if info.Size() > maxListing {
		return nil, nil, fmt.Errorf("commit %s: %w: it is %d bytes", p, errPastCap, info.Size())
	}
	data, err = io.ReadAll(io.LimitReader(f, maxListing+1))
	if err != nil {
		return nil, nil, fmt.Errorf("commit %s: %w", p, err)
	}
	return data, info, nil
}

// indexInfo is the index lines of changes, NUL-ended (update-index -z
// --index-info): mode, object and path; a removal is mode 0.
func indexInfo(changes []change, width int) []byte {
	var b strings.Builder
	for _, c := range changes {
		if c.gone {
			b.WriteString("0 " + strings.Repeat("0", width) + "\t" + c.path + "\x00")
			continue
		}
		b.WriteString(c.mode + " " + c.oid.text + "\t" + c.path + "\x00")
	}
	return []byte(b.String())
}

// checkMessage holds a commit's or a tag's message to one text of lines:
// UTF-8, not blank, and no control character but a newline or a tab.
func checkMessage(m string) error {
	if strings.TrimSpace(m) == "" || !utf8.ValidString(m) {
		return errors.New("a message is not empty and is UTF-8")
	}
	for _, r := range m {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return fmt.Errorf("a message holds no control character, and this one holds %q", r)
		}
	}
	return nil
}

// Tag makes the annotated tag name on commit, holding message. It is never
// signed: a release is pinned by its content hash (release-pin-by-hash), and
// tag.gpgSign and tag.forceSignAnnotated are set false on the
// command line, so no config of the account's or the repository's signs it.
// The commit must be a commit's full object name. A tag of that name
// already there is an error, and it is left as it is. A tag is always
// annotated: there is no form that makes a lightweight one. No hook runs.
func (g *Git) Tag(ctx context.Context, root model.Root, name, commit, message string) (err error) {
	dir, err := dirOf(root)
	if err != nil {
		return err
	}
	t, err := parseTag(name)
	if err != nil {
		return err
	}
	c, err := parseOID(commit)
	if err != nil {
		return err
	}
	if err := checkMessage(message); err != nil {
		return err
	}
	if _, err := g.Who(ctx, root); err != nil {
		return err
	}
	if err := g.ready(ctx, dir); err != nil {
		return err
	}
	if err := g.isCommit(ctx, dir, c); err != nil {
		return fmt.Errorf("%w, so no tag is made on it", err)
	}
	tags, err := g.Tags(ctx, root)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(tags, func(m model.Tag) bool { return m.Name == t.text }) {
		return fmt.Errorf("the tag %s is already there, and is left as it is", t.text)
	}
	w, err := newWork("")
	if err != nil {
		return g.failed("the tag's folder could not be made", err)
	}
	defer g.done(w, &err)
	msg := strings.TrimRight(message, "\n") + "\n"
	if _, err := g.runWith(ctx, verbTag, dir, input{stdin: []byte(msg), hooks: w.hooks}, t, c); err != nil {
		return g.answer("git tag failed, so no tag is made", err)
	}
	return nil
}

// isCommit is nil when the repository at dir has a commit of exactly the
// name c, and refuses one it resolves to another name: a name of the other
// hash's length, or a tag's object, which peels to its commit.
func (g *Git) isCommit(ctx context.Context, dir string, c value) error {
	r, err := parseRev(c.text)
	if err != nil {
		return err
	}
	got, err := g.resolve(ctx, dir, r)
	if err != nil {
		return err
	}
	if got.text != c.text {
		return fmt.Errorf("%w: %s names no commit of this repository by that name", errBadRev, c.text)
	}
	return nil
}
