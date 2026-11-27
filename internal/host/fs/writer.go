package fs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/write"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Writer is the adapter of port.Writer: it replaces a file whole and appends
// one line to one, inside the roots its Tree opened and nowhere else
// (cli.commands.4). Tree is the Tree whose roots it writes in, which the
// wiring always gives it (arch.ports.5).
type Writer struct {
	Tree *Tree
	// as, when a test of this package sets it, is the account Replace takes
	// this process to run as, in go-files' trust form ("uid:N" or a SID), so
	// a test sees another account's root refused without a second account.
	// It is never set outside a test binary.
	as func() string
}

// Replace makes the file at path in root hold data, through go-files'
// write.FileIn (gate/shared.yaml): the new file is written aside in the same
// folder, given the old file's owner and mode, flushed, renamed over the old
// one, and then the folder's record of the rename is flushed (cli.commands.23,
// store.state-files.2). A reader sees the old file or the new one, never half
// of either, and a run killed before the rename leaves the old file as it
// was. A link, a folder or anything that is not a file at path is refused,
// never written over.
//
// The path is walked with no link followed, and the file's folder is then
// opened by handle and checked to be the folder walked; FileIn works inside
// that handle, so a folder swapped on the way after the walk is never
// followed, inside the root or out of it. A root another account
// owns is refused before anything, as port.ErrNotOwner, and root running over
// another account's tree is no exception.
func (w Writer) Replace(root model.Root, path string, data []byte) (err error) {
	s, name, err := w.at(root, path)
	if err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	if err := mine("the root", s.root, w.as); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	old, err := s.walk("replace", path, name)
	if err != nil {
		return err
	}
	if old != nil && !old.Mode().IsRegular() {
		return fmt.Errorf("replace %s: it is a %s, and only a file is replaced", path, kindOf(old.Mode()))
	}
	if err := s.parent(name); err != nil {
		return s.refused("replace", path, name, err)
	}
	dir, base, err := s.folderOf("replace", path, name)
	if err != nil {
		return err
	}
	defer func() { err = errClosed(err, dir.Close()) }()
	err = write.FileIn(dir, base, data, options()...)
	if r := replaceRefused(path, err); r != nil {
		return r
	}
	if err != nil {
		return s.fail("replace", path, name, err)
	}
	return nil
}

// replaceRefusals are go-files' refusals of a Replace the store names, each
// with its words. ErrNoFileIndex is a volume, or a file, whose file index
// cannot tell the file written from one put in its place, so go-files cannot
// tell whether the file there is the one written; its own words say whether
// it is in place.
func replaceRefusals() []struct {
	is  error
	why string
} {
	return []struct {
		is  error
		why string
	}{
		{write.ErrNotDurable, "the file is in place, but its folder could not be flushed, so a power cut now could bring back the old one"},
		{write.ErrNoOwner, "the file there is another account's, and a new file would be this account's and lock that owner out, so nothing is written"},
		{write.ErrNoFileIndex, "the volume gives the file no ID that tells it from one put in its place, so whether the file there is the one written is not known; keep the store on a local disk whose files keep their own IDs"},
	}
}

// replaceRefused is go-files' refusal err of a Replace of path, in the
// store's words, or nil when err is none it names.
func replaceRefused(path string, err error) error {
	for _, r := range replaceRefusals() {
		if errors.Is(err, r.is) {
			return fmt.Errorf("replace %s: %s: %w", path, r.why, err)
		}
	}
	return nil
}

// options are FileIn's for a file of the store: the old file's mode, or a
// shared file's for a new one (go-files' Shared: 0644 on Unix, its folder's
// inherited access list on Windows), and the old file's owner and group, so a
// group a repository shares stays its group (go-files' KeepOwner). On Windows
// go-files keeps an owner that is the writer's own and refuses any other with
// ErrNoOwner, as a new file there is always its writer's.
func options() []write.Option {
	return []write.Option{write.KeepMode(), write.Shared(), write.KeepOwner()}
}

// Append adds lines and one newline to the file at path in root, in one
// write, ending a torn last line first (store.state-files.5). lines is one
// line, or several joined by a newline with none empty and none ending it, so
// a plan's several lines are one write and a cut leaves them all or the
// journal's roll-back takes them all. The file is made when it is missing,
// shared, as Replace makes one. A newline at either end, an empty line among several, or a
// path that is not a file, writes nothing. The file is opened by go-files'
// confine.Open, which follows no link at any component and refuses a file
// with a second name, a hard link. This is the one place in gragas that opens
// a file to append; a repository test counts the flag. A root another account
// owns is refused before anything, as port.ErrNotOwner, as Replace refuses it.
func (w Writer) Append(root model.Root, path string, line []byte) (err error) {
	if err := checkLines(line); err != nil {
		return fmt.Errorf("append %s: %w", path, err)
	}
	s, name, err := w.at(root, path)
	if err != nil {
		return fmt.Errorf("append: %w", err)
	}
	if err := mine("the root", s.root, w.as); err != nil {
		return fmt.Errorf("append %s: %w", path, err)
	}
	walked, err := s.walk("append", path, name)
	if err != nil {
		return err
	}
	if walked == nil {
		if err := s.parent(name); err != nil {
			return s.refused("append", path, name, err)
		}
	}
	// made is whether this open made the file, never a guess from before
	// it: a file another process made in between is walked again, opened as
	// it is, and keeps its mode.
	open := os.O_RDWR | os.O_APPEND
	f, err := confine.Open(s.root, name, open|os.O_CREATE|os.O_EXCL, 0o644)
	made := err == nil
	if errors.Is(err, fs.ErrExist) {
		if walked, err = s.walk("append", path, name); err != nil {
			return err
		}
		w.Tree.opened(path)
		f, err = confine.Open(s.root, name, open, 0)
	}
	if err != nil {
		return s.refused("append", path, name, err)
	}
	defer func() { err = errClosed(err, f.Close()) }()
	if err := same("append", path, f, walked, made); err != nil {
		return err
	}
	if err := s.appendTo(f, line, made); err != nil {
		return s.fail("append", path, name, err)
	}
	if made {
		if err := write.SyncFolder(s.root, filepath.Dir(name)); err != nil {
			return fmt.Errorf("append %s: the line is written, but the folder of the new file could not be flushed: %w", path, err)
		}
	}
	return nil
}

// Remove takes away the regular file at path in root, so a roll-back undoes
// a file its run made, and flushes the folder that held it so the removal
// survives a power cut. A file that is not there is no error. A link, a
// folder or anything that is not a file is refused and stays. The path is
// walked with no link followed, so a link to .git as a folder on the path
// never carries a removal into it, and the removal is made in
// the file's folder opened by handle, never by the path again. The folder
// stays, as Replace made it. The check and the removal are two calls, so an
// empty folder put at path between them would go; the root lock keeps
// gragas's own runs out of that gap. A root another account owns is refused
// before anything, as port.ErrNotOwner, as Replace refuses it.
func (w Writer) Remove(root model.Root, path string) (err error) {
	s, name, err := w.at(root, path)
	if err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	if err := mine("the root", s.root, w.as); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	old, err := s.walk("remove", path, name)
	switch {
	case err != nil:
		return err
	case old == nil:
		return nil
	case !old.Mode().IsRegular():
		return fmt.Errorf("remove %s: it is a %s, and only a file is removed", path, kindOf(old.Mode()))
	}
	dir, base, err := s.folderOf("remove", path, name)
	if err != nil {
		return err
	}
	defer func() { err = errClosed(err, dir.Close()) }()
	if err := dir.Remove(base); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return s.fail("remove", path, name, err)
	}
	if err := write.SyncFolder(dir, "."); err != nil {
		return fmt.Errorf("remove %s: the file is gone, but its folder could not be flushed, so a power cut now could bring it back: %w", path, err)
	}
	return nil
}

// appendTo writes line to f, opened to append, as one write: a newline first
// when the last byte is not one, the line, its newline. It refuses a file
// that is not regular before it writes.
func (s *store) appendTo(f *os.File, line []byte, made bool) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("it is a %s, not a file", kindOf(info.Mode()))
	}
	// A file this append made is shared, 0644 whatever the umask, as Replace's
	// write.Shared makes one. go-files' write.Append would own the mode, but
	// it also holds the log and moves it aside at a limit, which this append
	// does not do yet.
	if made {
		if err := f.Chmod(0o644); err != nil {
			return err
		}
	}
	out := make([]byte, 0, len(line)+2)
	if size := info.Size(); size > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, size-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			out = append(out, '\n') // a torn last line is ended first
		}
	}
	out = append(append(out, line...), '\n')
	if n, err := f.Write(out); err != nil || n != len(out) {
		return errors.Join(fmt.Errorf("wrote %d of %d bytes", n, len(out)), err)
	}
	return f.Sync()
}

// checkLines holds what Append is given to one line or several joined by a
// newline: no newline at either end, and no two together.
func checkLines(lines []byte) error {
	if len(lines) > 0 && (lines[0] == '\n' || lines[len(lines)-1] == '\n' || bytes.Contains(lines, []byte("\n\n"))) {
		return errors.New("the lines hold a newline at an end or an empty line between two; several lines are joined by one newline")
	}
	return nil
}

// at is the store and the name inside it for a file's path. The root itself
// is no file.
func (w Writer) at(root model.Root, path string) (*store, string, error) {
	if path == "" {
		return nil, "", errors.New("a file is named by a path, and the empty path is the root")
	}
	// No writer writes a name a disk reads as another, .git among them: by name
	// on each component, as go-files' confine.Portable judges it, so .GIT, its
	// short name, the code points a disk ignores, a reserved name and a trailing
	// dot or space are refused alike. The
	// plan's vet and the roll-back refuse it too; this is the last place, for
	// whatever way another record reaches a write.
	s, name, err := w.Tree.at(root, path)
	if err != nil {
		return nil, "", err
	}
	// name is in the system's form (confine.Local), `sub\f` on Windows, so it is
	// split in the slash form: a \ inside a part is still refused as Portable
	// refuses it on Unix, and a separator is not.
	for seg := range strings.SplitSeq(filepath.ToSlash(name), "/") {
		if _, err := confine.Portable(seg); err != nil {
			return nil, "", fmt.Errorf("%s: %w", path, err)
		}
	}
	return s, name, nil
}

// parent makes each folder above name that is missing through go-files'
// confine.MkdirShared (gate/shared.yaml), a folder other accounts reach: on
// Unix confine.Mkdir at 0755 whatever the umask, with the folder that holds
// each new one flushed, so a file put in it is not lost with its folder
// (store.seams.3).
//
// Nothing goes by the path twice: Mkdir makes each folder in the
// handle of the one above it and opens it with no link followed, and refuses
// a link, anything that is not a folder, another account's folder or one that
// holds anything, swapped in on the way, as confine.ErrOutside, which matches
// port.ErrOutside here. The one swap it cannot tell, an empty folder of this
// account's renamed in, is go-files' to answer. On Windows each folder it makes
// inherits the access list of the folder above, and one that does not, or a
// folder above with no inheritable entry, is refused as confine.ErrOutside;
// nothing there is flushed.
func (s *store) parent(name string) error {
	dir := filepath.Dir(name)
	if dir == "." {
		return nil
	}
	made, err := confine.MkdirShared(s.root, dir)
	if err != nil {
		return err
	}
	return made.Close()
}

// folderOf opens the folder that holds name as a root of its own, once a
// walk with no link followed reaches it, and holds it to be the very folder
// walked: one swapped in between is refused as outside, and once it is open
// no swap on the way to it is followed, since nothing is reached by the path
// again. It answers the folder and name's last component.
func (s *store) folderOf(op, path, name string) (*os.Root, string, error) {
	dir, base := filepath.Dir(name), filepath.Base(name)
	walked, err := s.walk(op, path, dir)
	switch {
	case err != nil:
		return nil, "", err
	case walked == nil:
		return nil, "", s.fail(op, path, name, &fs.PathError{Op: op, Path: path, Err: fs.ErrNotExist})
	}
	folder, err := s.root.OpenRoot(dir)
	if err != nil {
		return nil, "", s.fail(op, path, name, err)
	}
	got, err := folder.Stat(".")
	if err == nil && !os.SameFile(got, walked) {
		err = fmt.Errorf("%s %s: its folder changed between the walk and the open, and nothing was done to it: %w", op, path, port.ErrOutside)
	}
	if err != nil {
		return nil, "", errors.Join(err, folder.Close())
	}
	return folder, base, nil
}

// mine refuses the folder dir, opened as a root, when an account other than
// this run's owns it, and names both, as what (the root, the state folder):
// the owner of the folder a write or a lock is made in could swap a name
// there. The owner is read by go-files' trust.OwnerOf
// on the folder opened, never by its path, and the account by its
// trust.Account, both in its form: "uid:N" on Unix, the SID on
// Windows, so a folder there that Administrators own is another account's
// and refused. as is a test's
// account, set through export_test.go. An account or an owner that cannot be
// read is refused too: nothing then says the folder is this run's.
func mine(what string, dir *os.Root, as func() string) error {
	me, err := account(as)
	if err != nil {
		return fmt.Errorf("%s %s: %w, so its owner cannot be compared and nothing is written: %w", what, dir.Name(), err, port.ErrNotOwner)
	}
	f, err := dir.Open(".")
	if err != nil {
		return err
	}
	owner, err := trust.OwnerOf(f)
	err = errClosed(err, f.Close())
	switch {
	case err != nil:
		return fmt.Errorf("%s %s: its owner cannot be read (%w), so nothing is written: %w", what, dir.Name(), err, port.ErrNotOwner)
	case owner == me:
		return nil
	}
	return fmt.Errorf("%s %s is owned by account %s, and this run is account %s: its owner could swap a name in it, so nothing is written: %w",
		what, dir.Name(), owner, me, port.ErrNotOwner)
}

// account is the account this process runs as, in go-files' trust form, or
// the one a test of this package set through export_test.go. One go-files
// cannot read, or reads beside an error, is an error: it fails closed.
func account(set func() string) (string, error) {
	if set != nil {
		return set(), nil
	}
	me, err := trust.Account()
	switch {
	case err != nil:
		return "", fmt.Errorf("this run's account cannot be read: %w", err)
	case me == "":
		return "", errors.New("this run's account cannot be read")
	}
	return me, nil
}

// writeNote writes the note of a lock this process has just taken, at name
// in state, through go-files' write.FileIn: a temp file in state and a rename
// there, at 0600 as go-files makes the lock file, so a link at the name is
// replaced and its target untouched, and a folder swapped on the way to state
// takes nothing. It says what go-files' Note writes and
// lock.Holder reads, the process and since when, one JSON line; Lock's take
// reads it back through lock.Holder, so a note go-files could not read is a
// refusal, never a lock held with no note. It is here, beside Replace, as
// this package's writes through write.FileIn are (arch.layers.2).
func writeNote(state *os.Root, name string, since time.Time) error {
	raw, err := json.Marshal(struct {
		PID   int       `json:"pid"`
		Since time.Time `json:"since"`
	}{os.Getpid(), since})
	if err != nil {
		return err
	}
	return write.FileIn(state, name, append(raw, '\n'))
}
