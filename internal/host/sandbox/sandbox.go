// Package sandbox is the Sandbox adapter: the isolated run level
// (architecture 4.2). Prepare makes a scratch folder under the engine's
// cache, never under a project, places each fixture and each file of the
// config set at its declared path, links the tool's resolve-from folder,
// and answers the limits a run in it is held to; Release removes it. The
// Steps adapter (package tool) runs the tool in the folder, with PATH and LANG alone, held
// to those limits.
//
// The memory limit at this level is per process and best-effort: the
// Steps adapter judges the largest single process of the run against it,
// never the sum of the processes that run at once, and on Linux backs it
// with a cap per process. Only the sandboxed level caps the whole run.
//
// An isolated run cannot deny a network socket or a write through the
// linked folder: only the sandboxed level does, through a container or the
// OS sandbox, and this build has neither (M4-04, M4-05, M4-09), so a
// sandboxed Prepare answers port.ErrUnavailable.
//
// The adapter reads no other port. The run that prepares a folder takes the
// scratch lock first, after the root lock (store.consistency.1), and
// releases the folder on every path, a cancel included.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-files/trust"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Seconds is the adapter's own deadline of a run, as the Steps adapter's own,
// for a Prepare that asks for 0. There is no memory limit of the adapter's
// own: a Prepare that asks for 0 MiB is held to none, and a run is held to a
// memory limit only where one is set.
const Seconds = 120

const (
	// Folder is the folder of the cache the scratch folders are made in:
	// the scratch kind of the cache command.
	Folder = "scratch"
	// prefix starts the name of every scratch folder, so Release removes
	// only one Prepare could have made.
	prefix = "run-"
)

// Config is what the adapter is made from.
type Config struct {
	// Dir is the engine's cache folder, absolute: config.Config's CacheDir.
	Dir string
	// Mounts are config.Config's Mounts: folders on whose disks the base,
	// mounted nosuid, or noexec without nosuid, is judged by its owners in
	// place of refused.
	Mounts []string
}

// Sandbox is the adapter. Make it with New.
type Sandbox struct{ cfg Config }

// New makes the adapter. It makes no folder until Prepare.
func New(cfg Config) *Sandbox { return &Sandbox{cfg: cfg} }

// Prepare makes the scratch folder of p (port.Sandbox). Every path is
// checked before the folder is made, and a folder is removed again on any
// refusal or cancel past that point. The folder is made fresh and private
// by go-files' confine.MkdirFresh in the base held as a root, and removed
// through that root by confine.RemoveAll.
func (s *Sandbox) Prepare(ctx context.Context, p model.Prepare) (_ model.Scratch, err error) {
	if err := s.check(p); err != nil {
		return model.Scratch{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.Scratch{}, err
	}
	base, err := s.base(true)
	if err != nil {
		return model.Scratch{}, err
	}
	top, err := s.held(base)
	if err != nil {
		return model.Scratch{}, err
	}
	defer func() { err = errors.Join(err, top.Close()) }()
	made, name, err := confine.MkdirFresh(top, prefix)
	if err != nil {
		return model.Scratch{}, fmt.Errorf("the scratch folder could not be made in %s: %w", base, err)
	}
	dir := filepath.Join(base, name)
	if err := errors.Join(place(ctx, made, p), made.Close()); err != nil {
		if rm := confine.RemoveAll(top, name); rm != nil {
			return model.Scratch{}, errors.Join(err, fmt.Errorf("and the scratch folder %s could not be removed: %w", dir, rm))
		}
		return model.Scratch{}, err
	}
	return model.Scratch{
		Dir: filepath.ToSlash(dir), Level: model.Isolated,
		Seconds: or(p.Seconds, Seconds), MemoryMiB: p.MemoryMiB,
	}, nil
}

// Release removes a scratch folder Prepare made and every file in it
// (port.Sandbox). The linked folder is not followed: the link goes, and what
// it named stays. Any folder but one Prepare could have made is refused. It
// is removed by go-files' confine.RemoveAll through the base held as a root.
func (s *Sandbox) Release(sc model.Scratch) (err error) {
	if sc == (model.Scratch{}) {
		return refused("the zero scratch folder names no folder, so nothing is removed")
	}
	dir := filepath.FromSlash(sc.Dir)
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || filepath.Dir(dir) != s.basePath() || !strings.HasPrefix(filepath.Base(dir), prefix) {
		return refused(fmt.Sprintf("%q is no scratch folder this adapter makes, so nothing is removed", sc.Dir))
	}
	base, err := s.base(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	root, err := s.held(base)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := confine.RemoveAll(root, filepath.Base(dir)); err != nil {
		return fmt.Errorf("the scratch folder %s could not be removed: %w", sc.Dir, err)
	}
	return nil
}

// Sweep removes every scratch folder Prepare could have made (port.Sandbox):
// each entry of the base that starts with the prefix, whatever it holds. The
// caller holds the scratch lock, one for the machine, so no run of any root
// holds one. A link is removed, never followed, and an entry without the
// prefix is left where it is. The base is held as Prepare holds it, so a link
// planted in its place sweeps nothing.
func (s *Sandbox) Sweep() (int, error) { return s.left(true) }

// Left counts the scratch folders Sweep would remove now (port.Sandbox), by
// the same rule and over the same checked base, and removes nothing.
func (s *Sandbox) Left() (int, error) { return s.left(false) }

// left counts, and when remove is set removes, every entry of the base that
// starts with the prefix. The base is opened as a root and held to the folder
// base checked, so a base swapped for a link after the check is refused, and
// every removal is made through that handle: none follows a link, and none
// leaves the folder that was checked.
func (s *Sandbox) left(remove bool) (n int, err error) {
	if !filepath.IsAbs(s.cfg.Dir) {
		return 0, refused(fmt.Sprintf("the cache folder %q is not an absolute path", s.cfg.Dir))
	}
	base, err := s.base(false)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	root, err := s.held(base)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	dir, err := root.Open(".")
	if err != nil {
		return 0, fmt.Errorf("the scratch folders in %s could not be read: %w", base, err)
	}
	entries, err := dir.ReadDir(-1)
	if cerr := dir.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, fmt.Errorf("the scratch folders in %s could not be read: %w", base, err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if remove {
			if err := confine.RemoveAll(root, e.Name()); err != nil {
				return n, fmt.Errorf("the scratch folder %s could not be removed: %w", filepath.ToSlash(filepath.Join(base, e.Name())), err)
			}
		}
		n++
	}
	return n, nil
}

// held opens base, as base checked it, as a root, and refuses one that same
// refuses. The caller closes it.
func (s *Sandbox) held(base string) (*os.Root, error) {
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, fmt.Errorf("the scratch folders in %s could not be opened: %w", base, err)
	}
	if err := s.same(base, root); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	return root, nil
}

// same refuses a root opened at base that is not the folder at base now, as
// base checks it: not a link, this account's, and written by no one else.
// A link planted at base before the open is followed by it, and found here.
func (s *Sandbox) same(base string, root *os.Root) error {
	held, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("the scratch folder %s could not be read: %w", base, err)
	}
	at, err := s.base(false)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(at)
	if err != nil {
		return err
	}
	if !os.SameFile(held, fi) {
		return refused(fmt.Sprintf("%s, where scratch folders are made, was moved while it was opened", base))
	}
	return nil
}

// check refuses p before anything is made: a level this build cannot
// reach, a limit below 0, a file path that leaves the folder, and a linked
// folder that is not a folder of its own.
func (s *Sandbox) check(p model.Prepare) error {
	switch p.Level {
	case model.Isolated:
	case model.Sandboxed:
		return fmt.Errorf("the sandboxed level needs a container or the OS sandbox, and this build has neither: %w", port.ErrUnavailable)
	default:
		return refused(fmt.Sprintf("the level %q is neither isolated nor sandboxed", p.Level))
	}
	if p.Seconds < 0 || p.MemoryMiB < 0 {
		return refused(fmt.Sprintf("a limit of %d seconds or %d MiB is below none", p.Seconds, p.MemoryMiB))
	}
	if !filepath.IsAbs(s.cfg.Dir) {
		return refused(fmt.Sprintf("the cache folder %q is not an absolute path", s.cfg.Dir))
	}
	for _, f := range p.Fixtures {
		if err := inside(f.Path); err != nil {
			return err
		}
	}
	for _, f := range p.Config {
		if err := inside(f.Path); err != nil {
			return err
		}
	}
	if p.Resolve == (model.Link{}) {
		return nil
	}
	if err := inside(p.Resolve.Path); err != nil {
		return err
	}
	d := filepath.FromSlash(p.Resolve.Dir)
	if !filepath.IsAbs(d) || filepath.Clean(d) != d {
		return refused(fmt.Sprintf("the folder %q to link is not an absolute, clean path", p.Resolve.Dir))
	}
	fi, err := os.Lstat(d)
	switch {
	case err != nil:
		return fmt.Errorf("refused: the folder %s to link: %w", p.Resolve.Dir, err)
	case fi.Mode().Type() == fs.ModeSymlink:
		return refused(fmt.Sprintf("the folder %s to link is itself a link", p.Resolve.Dir))
	case !fi.IsDir():
		return refused(fmt.Sprintf("%s, the folder to link, is not a folder", p.Resolve.Dir))
	}
	return nil
}

// inside refuses a path that is not relative, slash-separated and clean, or
// that climbs out of the folder: such a file is not placed.
func inside(p string) error {
	switch {
	case p == "" || p == ".":
		return refused("a file of the run is named by no path")
	case strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return refused(fmt.Sprintf("the path %q holds a control character", p))
	case strings.ContainsAny(p, `\:`):
		return refused(fmt.Sprintf("the path %q holds a backslash or a colon", p))
	case path.IsAbs(p) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../"):
		return fmt.Errorf("refused: the path %q leaves the scratch folder, so it is not placed: %w", p, port.ErrOutside)
	}
	return nil
}

// basePath is the folder the scratch folders are made in.
func (s *Sandbox) basePath() string { return filepath.Join(filepath.Clean(s.cfg.Dir), Folder) }

// base is basePath, made first when made is set. It must be a folder and
// not a link: a link planted in its place would put every run, and every
// removal, elsewhere. It must be this account's, and writable by no one
// else: MkdirAll leaves a folder that is already there as it is, and an
// account that can write it could put a folder of its own, its config files
// in it, where a run of this account's is about to start.
//
// The base is held; the cache folder above it, and every folder up to the
// root, is first walked by host/trust. An account that may
// write the cache folder, or a folder above it, could rename the judged base
// away and put a folder of its own at the name, or, on Windows, swap in a
// junction the base's open and its Lstat both follow. With every folder
// above it trusted, neither swap is open to another account, so the base's
// Lstat-then-open needs nothing more.
func (s *Sandbox) base(made bool) (string, error) {
	base := s.basePath()
	if made {
		if err := s.make(base); err != nil {
			return "", err
		}
	}
	if err := s.cache(); err != nil {
		return "", err
	}
	fi, err := os.Lstat(base)
	switch {
	case err != nil:
		return "", err
	case fi.Mode().Type() == fs.ModeSymlink:
		return "", refused(fmt.Sprintf("%s, where scratch folders are made, is a link", base))
	case !fi.IsDir():
		return "", refused(fmt.Sprintf("%s, where scratch folders are made, is not a folder", base))
	}
	if err := owned(base, fi, s.cfg.Mounts); err != nil {
		return "", err
	}
	return base, nil
}

// cache walks the cache folder the base is made in, and every folder up to
// the root, as host/trust's walk does a program's: links
// followed, each name judged by its owner and who else may write it, the
// configured mounts as its Who.Mounts. A cache folder that is not there
// answers the read's error, which matches fs.ErrNotExist, so Release and
// Sweep find nothing; one the walk does not trust is refused by name, in
// the walk's words and with its fix. host/trust's refusal keeps go-files'
// words, not its error, so a name on a disk whose owners go-files does not
// judge, the one refusal whose line is MountFix, is joined to trust.ErrMount
// here, as the base's own judgement of such a disk is.
//
// On macOS a folder the admin group may write passes, on purpose: the walk
// hands go-files the admin group for a package manager's folders,
// and an admin is root by sudo, so a cache folder its group
// may write is refused for every group but that one.
func (s *Sandbox) cache() error { return s.walk(filepath.Clean(s.cfg.Dir)) }

// cacheSubject names the cache folder in the walks of cache and make.
func cacheSubject() hosttrust.Subject {
	return hosttrust.Subject{
		Name: "the cache folder", File: "the cache folder", Last: trust.Folder, Bare: true,
		Point: "point this account's cache folder there",
	}
}

// walk is cache's walk of the cache folder at, refused by its name.
func (s *Sandbox) walk(at string) error {
	dir := filepath.Clean(s.cfg.Dir)
	_, err := hosttrust.Walk(cacheSubject(), at, hosttrust.Who{Mounts: s.cfg.Mounts})
	return untrusted(dir, err)
}

// untrusted is the refusal of the cache folder dir that the walk refused
// with err, in its words and with its fix; the disk's, whose line is
// MountFix, matches trust.ErrMount. Any error that is not the
// walk's refusal is err as it is.
func untrusted(dir string, err error) error {
	u, ok := errors.AsType[*hosttrust.Unsafe](err)
	if !ok {
		return err
	}
	if u.Fix == hosttrust.MountFix {
		err = errors.Join(err, trust.ErrMount)
	}
	return refusedBy(fmt.Sprintf("%s, the cache folder scratch folders are made in, is not trusted: %s: %s", dir, u.Observed, u.Fix), err)
}

// make makes base, and each folder above it that is missing, once every
// name that is there is judged, by host/trust's Private, which
// is go-files' judged private make: the nearest folder that is there is
// walked first, as cache walks the cache folder, so nothing is made through a
// name the walk refuses, a link another account may change among them, and
// each folder below it is made private to this account whatever the umask,
// with no link followed and nothing made through one.
// host/fs's state folder and host/git's releases cache make theirs the same
// way (gate/shared.yaml private-dir). A base that is there, a
// link included, is left as it is for base to judge.
func (s *Sandbox) make(base string) error {
	if _, err := os.Lstat(base); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the scratch folder's parent %s could not be made: %w", base, err)
	}
	made, err := hosttrust.Private(base, cacheSubject(), hosttrust.Who{Mounts: s.cfg.Mounts})
	if _, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		return untrusted(filepath.Clean(s.cfg.Dir), err)
	}
	if err == nil {
		err = made.Close()
	}
	if err != nil {
		return fmt.Errorf("the scratch folder's parent %s could not be made: %w", base, err)
	}
	return nil
}

// owned holds the folder at path, which fi describes from its Lstat, to
// judge. The folder is opened, the handle held to fi, and its owner, mode
// and access list read on the handle, never by the name, so a folder swapped
// in at the name after the Lstat is refused, not judged. Where the Lstat's
// file id is read by the name only when it is compared (Windows), a plain
// folder swapped in and left there passes the compare, and is the folder the
// handle judges: what is judged is what is at the name.
//
// The owner must be this account (judge); who else may write the folder is
// go-files' trust.Held's to judge on the handle, by the mode bits, a macOS
// access list, a Linux one's mask and a Windows one, and the volume it is
// on, through host/trust's Held, with mounts as its
// Who.Mounts, each judged by go-files as an entry first, as the walk judges
// them.
func owned(path string, fi fs.FileInfo, mounts []string) (err error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return fmt.Errorf("the scratch folder %s could not be opened: %w", path, err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	f, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("the scratch folder %s could not be opened: %w", path, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	held, err := f.Stat()
	if err != nil {
		return fmt.Errorf("the scratch folder %s could not be read: %w", path, err)
	}
	if !os.SameFile(held, fi) {
		return refused(fmt.Sprintf("%s, where scratch folders are made, was moved while it was opened", path))
	}
	owner, err := trust.OwnerOf(f)
	if err != nil {
		return refusedBy(fmt.Sprintf("the owner of %s, where scratch folders are made, cannot be read", path), err)
	}
	me, err := trust.Account()
	if err != nil || me == "" {
		return refusedBy("this account cannot be read, so no folder can be held to it", err)
	}
	if err := judge(path, owner, me); err != nil {
		return err
	}
	if err := hosttrust.Held(f, trust.Folder, mounts); err != nil {
		if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
			return refusedBy(fmt.Sprintf("%s, where scratch folders are made, is judged with a mount gragas cannot trust: %s: %s", path, u.Observed, u.Fix), err)
		}
		return unheld(path, err)
	}
	return nil
}

// unheld is the refusal of the base at path that trust.Held refused with
// err. A base on a mount whose owners go-files does not judge (ErrMount: a
// disk mounted nosuid, or noexec without nosuid, a file system it does not
// judge) gets host/trust's line for it, as the walk's refusals do: the disk
// is why, so another folder on it is refused too, and the line names the
// setting that lets such a disk be judged. Any other refusal
// says who may write it.
func unheld(path string, err error) error {
	if errors.Is(err, trust.ErrMount) {
		return refusedBy(fmt.Sprintf("%s, where scratch folders are made, is on a disk whose owners are not judged: %s", path, hosttrust.MountFix), err)
	}
	return refusedBy(fmt.Sprintf("%s, where scratch folders are made, can be written by another account, or its owner cannot be judged where it is", path), err)
}

// judge refuses the folder at path, owned by owner, for the account me, both
// in the form trust.Account gives: one owned by another account, root and
// the system's own included, which trust.Held would pass as root's. A SID is
// judged as uid:N is: the access list a Windows owner's mode bits say
// nothing of is Held's, where until go-files judged it the
// isolated level was shut on Windows.
func judge(path, owner, me string) error {
	if owner != me {
		return refused(fmt.Sprintf("%s, where scratch folders are made, is owned by account %s, not by this one (%s)", path, owner, me))
	}
	return nil
}

// place writes every file of p into root, the fresh scratch folder, so no
// file lands outside it, each file new: two files at one path, or at a path
// a folder or the link takes, are refused. The link is placed last.
func place(ctx context.Context, root *os.Root, p model.Prepare) error {
	for _, f := range append(append([]model.File(nil), p.Fixtures...), p.Config...) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := write(root, f); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Resolve == (model.Link{}) {
		return nil
	}
	if err := parent(root, p.Resolve.Path); err != nil {
		return err
	}
	// check held the target to an absolute, clean path of a folder that is no
	// link. LinkFolder makes a symbolic link, or on Windows a junction, and
	// refuses a link at any folder on the way to it.
	if err := confine.LinkFolder(root, filepath.FromSlash(p.Resolve.Dir), filepath.FromSlash(p.Resolve.Path)); err != nil {
		return refusedBy(fmt.Sprintf("the folder %s could not be linked at %s", p.Resolve.Dir, p.Resolve.Path), err)
	}
	return nil
}

// write places one file, new, readable and writable by this account alone,
// opened through confine.Open: no link followed at any component, and no
// wait on a pipe at the name.
func write(root *os.Root, f model.File) error {
	if err := parent(root, f.Path); err != nil {
		return err
	}
	w, err := confine.Open(root, filepath.FromSlash(f.Path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return refusedBy(fmt.Sprintf("the file %s is not placed", f.Path), err)
	}
	_, err = w.Write(f.Data)
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	return err
}

// parent makes the folders above p, level by level through go-files'
// confine.MkdirPrivate: each private to this account as it is made, with no
// link followed and nothing made through one.
func parent(root *os.Root, p string) error {
	d := path.Dir(p)
	if d == "." {
		return nil
	}
	made, err := confine.MkdirPrivate(root, filepath.FromSlash(d))
	if err != nil {
		return refusedBy(fmt.Sprintf("the folder %s is not made", d), err)
	}
	return made.Close()
}

// refused is a refusal that matches port.ErrRefused.
func refused(why string) error { return fmt.Errorf("refused: %s: %w", why, port.ErrRefused) }

// refusedBy is a refusal for the reason err, which it keeps.
func refusedBy(why string, err error) error {
	return fmt.Errorf("refused: %s: %w", why, errors.Join(port.ErrRefused, err))
}

// or is v, or bound for a v of 0.
func or(v, bound int) int {
	if v == 0 {
		return bound
	}
	return v
}
