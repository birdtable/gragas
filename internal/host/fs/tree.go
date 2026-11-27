// Package fs is the file system: the adapter of port.Tree and port.Writer,
// and until the old commands move, of legacy.Files and legacy.Writer. Each
// root is opened once, by one function that confines it under os.Root and
// sets the modes it makes files with and how its errors read
// (store.seams.3), so no path, link or .. reaches outside a root. A root
// follows a link that stays inside it, so every path from outside is first
// held to a clean local name by go-files' confine.Local, and Replace, Append,
// Remove and Read walk it by handle with confine.NoLinks, refusing a link or
// another reparse point at any component, the last included.
// Read and Append then open the file with confine.Open, which follows no
// link at any component and refuses a file with a second name opened to be
// written, a hard link, which no walk sees. Replace and Remove open the
// file's folder by handle, hold it to be the folder walked, and work inside
// it, so a folder swapped on the path after the walk is never followed,
// inside the root or out of it.
// Stat and List, on no written or checked path, follow such a
// link. The names file and the tools file are read outside every root, by an
// absolute path; the settings and tools files are read by host/trust's
// ReadOwned, only when their owner and mode allow. A Repo is a
// legacy.Source.
//
// The few calls go-files has no export for yet, the folder flush, an open
// that never waits and a file's or folder's owner, are os_unix.go's and
// os_other.go's, until go-files gives them.
//
// What it stores is not rebuilt: a root is a person's repository or project,
// or the engine's state folder, and the file there is the only copy
// (store.seams.6). Every write is durable when it returns: Replace flushes the
// new file and then its folder, and Append flushes the file, and its folder
// when it made the file. There is no checkpoint, since nothing is buffered
// here between writes; a run cut short between two writes will be found and
// rolled back from the run journal by the next run that writes (M0b-04,
// M0b-05), not by this package.
//
// A store makes a file 0644 and a folder 0755 whatever the umask, so others
// may read what it makes. No secret goes in a store, the run journal and
// history.jsonl included; a file that is there keeps its own
// mode, and a file aside is 0600 until it is in place.
package fs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/birdtable/go-files/confine"

	"github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// Repo is one repository's files, by slash-separated paths from its root.
// It holds the folder open, confined under os.Root, from Open until Close.
type Repo struct {
	root *os.Root
	fsys fs.FS
}

// Open is the repository at dir. dir must be a folder. Whoever opens it
// closes it: Windows refuses to remove a folder that is held open.
func Open(dir string) (Repo, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Repo{}, fmt.Errorf("open %s: %w", dir, err)
	}
	return Repo{root: root, fsys: root.FS()}, nil
}

// Close lets go of the folder. Nothing of r reads after it. The zero Repo
// holds none, and closes as nil.
func (r Repo) Close() error {
	if r.root == nil {
		return nil
	}
	return r.root.Close()
}

// Open opens the file at name, without waiting on a FIFO that has no writer.
func (r Repo) Open(name string) (fs.File, error) {
	f, err := r.open(name)
	if err != nil {
		return nil, err // never a nil *os.File inside a non-nil fs.File
	}
	return f, nil
}

// open is Open's file as an *os.File, opened through go-files' confine.Open:
// no link is followed at any component, the last included, so a file in the
// repository that is a link, or sits in a folder that is one, is refused even
// when it leads to another file of the repository, and the open never waits
// on a FIFO. The refusal names why, for audit's "not checked"
// says the reason as it is, and matches port.ErrOutside.
func (r Repo) open(name string) (*os.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	// Local refuses "." (its own spelling of the root is ""), which is a valid
	// io/fs name for the root.
	inRoot := "."
	if name != "." {
		var err error
		if inRoot, err = confine.Local(name); err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
		}
	}
	f, err := confine.Open(r.root, inRoot, os.O_RDONLY, 0)
	if errors.Is(err, confine.ErrOutside) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("it is a link, or a folder on the way to it is, and no link in the repository is followed: %w", offRoot{err})}
	}
	return f, err
}

// ReadFile reads the whole file at name. The file is opened without waiting
// and its open handle is statted, so a FIFO, a folder or a device is refused
// and never read, with no gap between a check and the open.
func (r Repo) ReadFile(name string) ([]byte, error) {
	f, err := r.open(name)
	if err != nil {
		return nil, err
	}
	data, err := readRegular(f, -1)
	if err != nil {
		return nil, &fs.PathError{Op: "read", Path: name, Err: err}
	}
	return data, nil
}

// ReadDir lists the folder at name, sorted by file name.
func (r Repo) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(r.fsys, name) }

// Stat describes the file at name.
func (r Repo) Stat(name string) (fs.FileInfo, error) { return fs.Stat(r.fsys, name) }

// Tree is the adapter of port.Tree: roots opened once under os.Root, so that
// no path, link or .. reaches outside one, and the files the settings name
// outside every root. The zero Tree is ready; it is safe for many goroutines.
// A Writer writes inside the roots its Tree opened.
type Tree struct {
	// Mounts are GRAGAS_MOUNTS's folders, as config hands them, on whose
	// disks ReadOwned judges a file mounted nosuid by its owners in place of
	// refusing it. The zero Tree holds none, the strict side. Set it before
	// the first read and never after.
	Mounts []string
	mu     sync.Mutex
	roots  map[model.Root]*store
	// adopted counts the folders adopt has put among roots, so each has a
	// name of its own.
	adopted int
	// opening, when a test of this package sets it, is called with the
	// path between the walk and the open of a Read or an Append, so the
	// test can swap the last component there. It is never called outside a
	// test binary.
	opening func(path string)
}

// opened calls opening, which only this package's tests set, through
// export_test.go; the shipped binary never links package testing.
func (t *Tree) opened(path string) {
	if t.opening != nil {
		t.opening(path)
	}
}

// store is one opened root: the folder confined under os.Root and, through
// fail, how its errors read. openStore is the one place a store is made from
// a path, and OpenIn the one place one is made inside another's folder. A
// file and a folder it makes are shared, other accounts reading them, in the
// library's terms: write.Shared for a file Replace makes,
// confine.MkdirShared for a folder a write makes for its file, and Append's
// own open (appendTo). A file replaced keeps its own mode.
type store struct {
	root *os.Root
	// held is a folder Tree.adopt put among the roots, which its caller
	// closes, not Tree.Close.
	held bool
}

// openStore opens the folder real, absolute and with no link in it, as a
// store, confined. Every root of a Tree that Open makes is made here.
func openStore(real string) (*store, error) {
	root, err := os.OpenRoot(real)
	if err != nil {
		return nil, err
	}
	return &store{root: root}, nil
}

// fail is how a store's error reads, the one mapping of them: what was done
// to which path, and which of four it is. A path that leaves the root matches
// port.ErrOutside; a missing one fs.ErrNotExist; one this account may not
// read or write fs.ErrPermission, said as forbidden; anything else is said as
// unreadable and matches neither.
func (s *store) fail(op, path, name string, err error) error {
	switch {
	case errors.Is(err, port.ErrOutside):
		return fmt.Errorf("%s %q: %w", op, path, err)
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s %s: it is missing: %w", op, path, err)
	case s.leaves(name):
		return fmt.Errorf("%s %s: %w: %w", op, path, port.ErrOutside, err)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%s %s: it is forbidden to this account: %w", op, path, err)
	}
	return fmt.Errorf("%s %s: it cannot be read or written: %w", op, path, err)
}

// leaves is whether name, inside the store, resolves to somewhere outside
// it, which is why os.Root refused it. The standard library has no value to
// match and an error's text is never read, so the name is resolved again and
// looked at.
func (s *store) leaves(name string) bool {
	return outside(s.root.Name(), filepath.Join(s.root.Name(), name))
}

// outside is whether p, an absolute and clean path, is outside base, with
// every link on it followed as the system follows them: go-files'
// confine.Below, which follows a junction too (filepath.EvalSymlinks has not
// since Go 1.23) and compares folders by file ID, never by spelling.
// The names below the last one that exists are kept as
// written, and a link to something missing leads where it points, so a write
// that would make a file or a folder out of the root is seen to leave it as
// well. A path past go-files' limit of links, as a loop is, is outside
// (confine.ErrLoop matches confine.ErrOutside). Any other answer, a folder
// this account may not look at among them, is not judged outside here.
func outside(base, p string) bool {
	_, err := confine.Below(base, p)
	return errors.Is(err, confine.ErrOutside)
}

// Open opens the folder at dir as a root. The root is named by the folder's
// absolute, link-free path, every link and junction on it followed by
// go-files' confine.Resolve, so the same folder is the same
// root each time.
func (t *Tree) Open(dir string) (model.Root, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", dir, err)
	}
	real, err := confine.Resolve(abs)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", dir, err)
	}
	name := model.Root(filepath.ToSlash(real))
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.roots[name]; ok {
		return name, nil
	}
	s, err := openStore(real)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", dir, err)
	}
	if t.roots == nil {
		t.roots = map[model.Root]*store{}
	}
	t.roots[name] = s
	return name, nil
}

// adopt puts dir, a folder its caller opened and holds open, among t's roots
// under a name of its own, never a path Open answers, until drop: the state
// folder a Lock holds, which the Journal writes through while the lock is held.
// Whoever adopts dir closes it, after drop; Close leaves it
// open. A nil t adopts nothing and answers "": a Lock given no Tree holds
// its folder for the note alone.
func (t *Tree) adopt(dir *os.Root) model.Root {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.adopted++
	name := model.Root(fmt.Sprintf("held %d: %s", t.adopted, filepath.ToSlash(dir.Name())))
	if t.roots == nil {
		t.roots = map[model.Root]*store{}
	}
	t.roots[name] = &store{root: dir, held: true}
	return name
}

// drop takes the folder adopt put under name out of t's roots, open.
func (t *Tree) drop(name model.Root) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.roots, name)
}

// Close lets go of every folder the tree opened. Windows refuses to remove a
// folder that is held open, so whoever makes a Tree for a folder that is then
// removed closes it first. A root is not usable after it; the zero Tree, or one
// already closed, closes as nil.
func (t *Tree) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	var err error
	for name, s := range t.roots {
		if !s.held {
			err = errors.Join(err, s.root.Close())
		}
		delete(t.roots, name)
	}
	return err
}

// Read answers the whole content of the file at path in root. It is the
// read a check before a write makes, and the read back after it, so a link
// at any component of path is refused as a write's is: the
// walk refuses one, and go-files' confine.Open follows none, so a link put
// on the path after the walk is refused at the open.
func (t *Tree) Read(root model.Root, path string) ([]byte, error) {
	return t.read(root, path, false, -1)
}

// ReadSoleWithin is Read when path is the file's one name and the file holds at
// most max bytes: go-files' confine.NamesFile counts the names on the handle
// Read opens, and the content is read through that same handle, closed only
// once the read is done. More than one name matches
// port.ErrManyNames; none, a file every name of which was removed while it
// was held, and a count the system will not give are refused too, never read
// as one name. It reads no more than max+1 from the handle and refuses a file
// that has more, so a file that
// grew after a listing is never read whole before the bound.
// The refusal matches ErrOverBound. A max outside [0, MaxInt64) is refused
// before anything is opened, as no bound: it is never read as unbounded, and
// max+1 never overflows.
func (t *Tree) ReadSoleWithin(root model.Root, path string, max int64) ([]byte, error) {
	if max < 0 || max == math.MaxInt64 {
		return nil, fmt.Errorf("read %s: the bound %d is outside [0, %d)", path, max, int64(math.MaxInt64))
	}
	return t.read(root, path, true, max)
}

// read is Read, and ReadSoleWithin when sole is set, of at most max bytes when max
// is not negative.
func (t *Tree) read(root model.Root, path string, sole bool, max int64) ([]byte, error) {
	s, name, err := t.at(root, path)
	if err != nil {
		return nil, err
	}
	walked, err := s.walk("read", path, name)
	if err != nil {
		return nil, err
	}
	if walked == nil {
		return nil, s.fail("read", path, name, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist})
	}
	t.opened(path)
	f, err := confine.Open(s.root, name, os.O_RDONLY, 0)
	if err != nil {
		return nil, s.refused("read", path, name, err)
	}
	if err := same("read", path, f, walked, false); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if sole {
		if err := soleName(path, f); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	data, err := readRegular(f, max)
	if err != nil {
		return nil, s.fail("read", path, name, err)
	}
	return data, nil
}

// soleName is nil when f, opened at path, has that one name.
func soleName(path string, f *os.File) error {
	n, err := confine.NamesFile(f)
	switch {
	case err != nil:
		return fmt.Errorf("read %s: its names cannot be counted: %w", path, err)
	case n > 1:
		return fmt.Errorf("read %s: %w: %d names", path, port.ErrManyNames, n)
	case n < 1:
		return fmt.Errorf("read %s: it has no name left", path)
	}
	return nil
}

// Stat answers the entry at path in root; an empty path is the root.
func (t *Tree) Stat(root model.Root, path string) (model.Entry, error) {
	s, name, err := t.at(root, path)
	if err != nil {
		return model.Entry{}, err
	}
	info, err := s.root.Stat(name)
	if err != nil {
		return model.Entry{}, s.fail("stat", path, name, err)
	}
	e := s.entry(path, info)
	e.Exec = info.Mode().IsRegular() && s.runnable(name)
	return e, nil
}

// List answers the files and folders below dir, sorted by path. The disk
// cannot say which files a repository's history holds, so only model.Every
// is served here; History answers the other scopes. It reads the folders
// alone and opens no file, so every entry's Exec is false: Stat answers it.
func (t *Tree) List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error) {
	if scope != model.Every {
		return nil, fmt.Errorf("list %q: the disk lists only %s files, not %s ones", dir, model.Every, scope)
	}
	s, name, err := t.at(root, dir)
	if err != nil {
		return nil, err
	}
	slash := filepath.ToSlash(name)
	out := []model.Entry{}
	err = fs.WalkDir(s.root.FS(), slash, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == slash {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, s.entry(p, info))
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) && name != "." {
		// A folder that is not there has nothing in it.
		return []model.Entry{}, nil
	}
	if err != nil {
		return nil, s.fail("list", dir, name, err)
	}
	return out, nil
}

// ReadOwned answers the whole content of the file at the absolute, clean
// path when this account or root owns it and neither its group nor others
// can write it: the one owned read, host/trust's, on t's mounts, which
// checks the descriptor it reads and follows no link at the name. Its
// refusal matches port.ErrNotOwned and the port's error of its reason, and
// keeps trust.Owned's Refusal.
func (t *Tree) ReadOwned(path string) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("read %q: the path must be absolute and clean", path)
	}
	data, err := trust.ReadOwned(path, t.Mounts)
	if o, ok := errors.AsType[*trust.Owned](err); ok {
		return nil, notOwned(o)
	}
	if err != nil {
		return nil, fmt.Errorf("read owned: %w", err)
	}
	return data, nil
}

// OpenIn opens the folder at dir in root as a root of its own: os.Root's
// OpenRoot on root's store, so the system's own walk refuses a dir that
// leaves root, by .. or through a link that leads out, and follows a link
// that stays inside. The new root is named as Open names one,
// by its folder's absolute, link-free path, and that path is held to be the
// folder opened, so a link swapped between the open and the naming is
// refused, never named.
func (t *Tree) OpenIn(root model.Root, dir string) (model.Root, error) {
	s, name, err := t.at(root, dir)
	if err != nil {
		return "", fmt.Errorf("open %q in %s: %w", dir, root, err)
	}
	if name == "." {
		return root, nil
	}
	sub, err := s.root.OpenRoot(name)
	if err != nil {
		if info, serr := s.root.Stat(name); serr == nil && !info.IsDir() {
			return "", fmt.Errorf("open %s: it is a %s, not a folder", dir, kindOf(info.Mode()))
		}
		return "", s.fail("open", dir, name, err)
	}
	named, err := held(sub)
	if err != nil {
		return "", errors.Join(fmt.Errorf("open %s: %w", dir, err), sub.Close())
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.roots[named]; ok {
		return named, sub.Close()
	}
	t.roots[named] = &store{root: sub}
	return named, nil
}

// held is the name of the folder sub holds open: its path with every link
// followed by go-files' confine.Resolve, as Open names a root, once that
// path is seen to be the same folder as the one held.
func held(sub *os.Root) (model.Root, error) {
	real, err := confine.Resolve(sub.Name())
	if err != nil {
		return "", err
	}
	opened, err := sub.Stat(".")
	if err != nil {
		return "", err
	}
	there, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !os.SameFile(opened, there) {
		return "", fmt.Errorf("it changed between the open and its naming: %w", port.ErrOutside)
	}
	return model.Root(filepath.ToSlash(real)), nil
}

// FS answers root's files as the read-only view the old check reads a
// repository through, a Repo's reads without its Close:
// it lists from os.Root's own view, the one List walks,
// and opens each file through go-files' confine.Open, so a link at any step
// of a path opened is refused, inside the root or out of it,
// and audit reads through the view what it reads today. The folder held is
// first seen to be the one at the root's name, so a root whose folder was
// removed or replaced is an error, never an empty view.
func (t *Tree) FS(root model.Root) (fs.FS, error) {
	s, _, err := t.at(root, "")
	if err != nil {
		return nil, err
	}
	opened, err := s.root.Stat(".")
	if err != nil {
		return nil, s.fail("view", string(root), ".", err)
	}
	there, err := os.Stat(s.root.Name())
	if err != nil {
		return nil, s.fail("view", string(root), ".", err)
	}
	if !os.SameFile(opened, there) {
		return nil, fmt.Errorf("view %s: the folder was replaced after it was opened", root)
	}
	return view{Repo{root: s.root, fsys: s.root.FS()}}, nil
}

// Portable makes the Tree the adapter of port.FileNames too: it answers
// name's key, or refuses the name, as go-files' confine.Portable does, the one
// table of what a disk does with a name, which gragas holds none of.
// It reads no disk and takes no root.
func (t *Tree) Portable(name string) (string, error) {
	return confine.Portable(name)
}

// view is a store's files as a Repo reads them, with no Close: the folder
// is the Tree's to let go of, at its own Close.
type view struct{ r Repo }

func (v view) Open(name string) (fs.File, error)          { return v.r.Open(name) }
func (v view) ReadFile(name string) ([]byte, error)       { return v.r.ReadFile(name) }
func (v view) ReadDir(name string) ([]fs.DirEntry, error) { return v.r.ReadDir(name) }
func (v view) Stat(name string) (fs.FileInfo, error)      { return v.r.Stat(name) }

// local is the one check every path from outside passes before it names
// anything in a root: a flag's, a config value's, an environment variable's,
// a rulebook's (store.state-files.8). It is go-files' confine.Local, which
// answers the name inside the root in this system's form, "." for the empty
// path, and refuses a path that is not clean, relative and slash-separated,
// or holds a backslash, a colon, a NUL or a file's fork, so a path means the
// same on every system; the refusal matches port.ErrOutside here.
func local(p string) (string, error) {
	name, err := confine.Local(p)
	if err != nil {
		return "", offRoot{err}
	}
	return name, nil
}

// walk walks name, inside s, by handle with go-files' confine.NoLinks, and
// refuses it when any component, the last included, is a link or another
// reparse point, or one before the last is not a folder. A root
// follows a link that stays inside it, so without this a rulebook that
// commits state -> .git would carry a write into .git. It answers what the
// last component is, or nil when the walk stopped at a missing one: the
// folders and the file a write makes from there are made by Mkdir, an
// exclusive open or a rename, none of which follows a link at its new name.
func (s *store) walk(op, path, name string) (fs.FileInfo, error) {
	info, err := confine.NoLinks(s.root, name)
	if err != nil {
		return nil, s.refused(op, path, name, err)
	}
	return info, nil
}

// refused is how an error of go-files' confine reads, for op on path: a
// link, a swap or a second name, which confine.ErrOutside matches, is the
// walk's *fs.PathError naming the component, so a caller finds it with
// errors.As and never by the text, and matches port.ErrOutside as well; the
// walk's refusal of a component that is not a folder is said as it is; any
// other error is fail's.
func (s *store) refused(op, path, name string, err error) error {
	if errors.Is(err, confine.ErrOutside) {
		return fmt.Errorf("%s %s: %w", op, path, offRoot{err})
	}
	if pe, ok := errors.AsType[*fs.PathError](err); ok && pe.Op == "walk" {
		return fmt.Errorf("%s %s: %w", op, path, err)
	}
	return s.fail(op, path, name, err)
}

// offRoot is a refusal of go-files' confine, said as it is, which matches
// port.ErrOutside too: the one error the ports know a path that leaves its
// root by.
type offRoot struct{ error }

func (o offRoot) Is(target error) bool { return target == port.ErrOutside }

func (o offRoot) Unwrap() error { return o.error }

// same refuses f, just opened at path, unless it is the file the walk
// found there: a file put in place of the walked one between the walk and
// the open is caught here, before a byte is read or written. walked nil is
// a file the walk found missing, which only an exclusive create may open;
// any other open of it is refused.
func same(op, path string, f *os.File, walked fs.FileInfo, made bool) error {
	if made {
		return nil
	}
	if walked == nil {
		return fmt.Errorf("%s %s: it was made between the check and the open, and nothing was done to it: %w", op, path, port.ErrOutside)
	}
	got, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(got, walked) {
		return fmt.Errorf("%s %s: it changed between the check and the open, and nothing was done to it: %w", op, path, port.ErrOutside)
	}
	return nil
}

// errClosed joins an error with the close's, and is err itself when the
// close was clean, so a caller still sees the error's own type.
func errClosed(err, closeErr error) error {
	if closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return err
}

// readRegular reads all of f, which it closes, when f is a regular file.
// Anything else, a folder, a FIFO or a device, is refused before a read can
// block on it.
func readRegular(f *os.File, max int64) (data []byte, err error) {
	defer func() { err = errClosed(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("it is a %s, not a file", kindOf(info.Mode()))
	}
	if max < 0 {
		return io.ReadAll(f)
	}
	data, err = io.ReadAll(io.LimitReader(f, max+1))
	if err == nil && int64(len(data)) > max {
		return nil, fmt.Errorf("it is over the %d bytes allowed: %w", max, ErrOverBound)
	}
	return data, err
}

// ErrOverBound is what a bounded read of a file with more bytes than the
// bound matches.
var ErrOverBound = pluginbundle.ErrOverBound

// kindOf names what a mode that is not a regular file's is.
func kindOf(m fs.FileMode) string {
	switch {
	case m.IsDir():
		return "folder"
	case m&fs.ModeSymlink != 0:
		return "link"
	case m&fs.ModeNamedPipe != 0:
		return "FIFO"
	case m&fs.ModeSocket != 0:
		return "socket"
	case m&fs.ModeDevice != 0:
		return "device"
	}
	return "special file"
}

// folder is the folder root names, as the *os.Root t holds open for it, from
// Open, OpenIn or adopt until Close or drop. The Journal hands it to go-run's
// runlog as Log.Root, so the journal is opened through the
// folder held, never by its path. The caller does not close it.
func (t *Tree) folder(root model.Root) (*os.Root, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.roots[root]
	if !ok {
		return nil, fmt.Errorf("root %q was not opened by this tree", root)
	}
	return s.root, nil
}

// at is the open store and the name inside it for a slash path, confined.
func (t *Tree) at(root model.Root, path string) (*store, string, error) {
	t.mu.Lock()
	s, ok := t.roots[root]
	t.mu.Unlock()
	if !ok {
		return nil, "", fmt.Errorf("root %q was not opened by this tree", root)
	}
	name, err := local(path)
	if err != nil {
		return nil, "", err
	}
	return s, name, nil
}

// entry is the model's entry for the file or folder at the slash path in s,
// from info alone: it opens nothing, so Exec is left false (List's answer;
// Stat judges it with runnable).
func (s *store) entry(path string, info fs.FileInfo) model.Entry {
	if path == "." {
		path = ""
	}
	e := model.Entry{Path: path, Dir: info.IsDir()}
	if !e.Dir {
		e.Special = !info.Mode().IsRegular()
		e.Size = info.Size()
	}
	return e
}

// runnable is whether the regular file at name, local to s, is one this system
// would run, as confine.IsRunnable judges it: from the file
// itself, opened inside the tree's root, following no link or junction and
// never waiting, by the exec bit for this account on Unix and by PATHEXT on
// Windows. An answer that fails is not runnable. Stat follows a link inside the
// root, and IsRunnable refuses the link at any component, so a name reached
// through a link is never runnable. It opens the file, so List never calls it
// (one open per file of a whole-root walk cost minutes).
func (s *store) runnable(name string) bool {
	ok, err := confine.IsRunnable(s.root, name)
	return ok && err == nil
}
