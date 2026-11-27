package fs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-files/lock"

	"github.com/birdtable/gragas/internal/digest"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Lock is the adapter of port.Lock: a kernel advisory lock on a file in the
// engine's state folder: one file for each root's root lock, named by the
// digest of the root, and one for the machine's scratch lock (lockName), taken
// through go-files' lock (the shared
// module's lock.Take, gate/shared.yaml). The kernel drops the lock when the
// process ends, however it ends, so a killed holder leaves nothing held
// (store.consistency.1, go.concurrency.3).
//
// A lock file is not state and cannot be rebuilt from anything: the holder
// writes its process and the time it took the lock in the note
// beside it, <name>.holder, so that a run that finds it held can say since
// when, and nothing reads the note for any other purpose. The start time a
// Hold carries is the one written there, read back at once through
// lock.Holder, so the busy answer another run reads says the same second. Take
// never waits and never retries, so it has no deadline to keep: ctx is read
// once, before the take. Wait waits for another process's hold until ctx ends,
// through go-files' lock.Wait, and holds the adapter's mutex all the while
// (Wait). Dir is the state folder, set by the wiring, and Mounts the
// configured mounts (config.Config.Mounts) its walk trusts, as every other
// walk of the product does.
//
// The state folder is opened as an os.Root once a take, and held open until
// Release: the lock file go-files took is held to the file at its name in that
// root, and every write under the lock goes through it, never by the path
// again. The note is written and removed there, and the journal
// of the root, when Tree is the Journal's Tree and the Journal names this
// Lock, writes in it. A folder on the way to Dir swapped after the take, a
// link or a junction turned to another folder, takes none of those writes.
type Lock struct {
	Dir    string
	Mounts []string

	// Clock says since when a take holds the lock, in the note it writes:
	// go-files, given no note, reads no clock of its own.
	Clock port.Clock

	// Tree, when set, is the Journal's Tree: each state folder a take holds
	// is put among its roots until Release, so the Journal writes there. The
	// wiring sets it with Journal.Lock; nil holds the folder for the note
	// alone.
	Tree *Tree

	// Roots, when set, answers the folders a clone carries for this run.
	// Dir must lie outside each of them and outside the root
	// each take names, judged before anything is made or taken there, and it
	// is made with them held apart (runApart), so a take matches
	// port.ErrCarried there. The wiring sets the same func on the Journal and
	// the approvals. nil judges nothing (runApart).
	Roots func() ([]string, error)

	// as, when a test of this package sets it, is the account Take takes
	// this process to run as. It is never set outside a test binary.
	as func() string

	// taking, when a test of this package sets it, is called with the lock
	// file's path after its name is checked and before go-files opens it,
	// so a test can swap the state folder there, or learn that a Wait is
	// about to wait. It is never set outside a test binary.
	taking func(path string)

	// judged, when a test of this package sets it, is called after the
	// state folder is judged against the roots and before it is made, so a
	// test can make a link at a missing name there. It is never set outside a
	// test binary.
	judged func()

	mu   sync.Mutex
	held map[model.Hold]holding
}

// holding is a lock this adapter holds and the second its note says, the
// Since a Wait for it answers with at once; the state folder it was taken in,
// held open, and the note's name there; the name the folder has among Tree's
// roots, "" with no Tree; and, for the root lock, what the Journal has seen
// of the root's journal under this hold, which goes with it at Release.
type holding struct {
	lock  *lock.Lock
	since int64
	state *os.Root
	note  string
	root  model.Root
	seen  *seen
}

var errOrder = errors.New("the root lock is taken before the scratch lock, never after it")

// errNotALockFile is a lock file's name that holds a link or anything but a
// file. go-files opens the name with no link followed, and this refusal,
// made before it opens, says which name it was in gragas's own words.
var errNotALockFile = errors.New("the lock file's name holds a link or something that is not a file")

// Take takes the lock which on root, or fails at once. A lock held by another
// process, or by this one, matches port.ErrHeld and says in its text since
// when. Taking the root lock while this adapter holds a scratch lock is
// refused whatever the root: the order is root first, scratch second.
func (l *Lock) Take(ctx context.Context, root model.Root, which model.Lock) (model.Hold, error) {
	return l.hold(ctx, root, which, false)
}

// Wait takes the lock which on root as Take does, with the same checks and the
// same order, but waits for another process that holds it until ctx ends.
// When ctx ends first, the error is the HeldError, which
// matches port.ErrHeld and carries the holder's Since, joined with ctx's
// error, so errors.Is finds both. A lock this adapter already holds is never
// waited for, as only this adapter's Release could free it: Wait fails at
// once with the HeldError and the Since of its own hold.
//
// Wait holds the adapter's mutex while it waits, so another goroutine's Take,
// Wait or Release on this adapter waits with it, until the lock is taken or
// ctx ends. A caller that may release a lock while another goroutine waits
// uses a second Lock over the same Dir, as a second run would.
func (l *Lock) Wait(ctx context.Context, root model.Root, which model.Lock) (model.Hold, error) {
	return l.hold(ctx, root, which, true)
}

// hold is Take, or Wait when wait is set.
func (l *Lock) hold(ctx context.Context, root model.Root, which model.Lock, wait bool) (model.Hold, error) {
	if err := ctx.Err(); err != nil {
		return model.Hold{}, err
	}
	switch {
	case which != model.RootLock && which != model.ScratchLock:
		return model.Hold{}, fmt.Errorf("take: %q is neither the root lock nor the scratch lock", which)
	case root == "":
		return model.Hold{}, errors.New("take: no root")
	case l.Dir == "":
		return model.Hold{}, errors.New("take: the lock has no state folder")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if which == model.RootLock {
		for h := range l.held {
			if h.Which == model.ScratchLock {
				return model.Hold{}, fmt.Errorf("take the root lock of %s: this run holds the scratch lock of %s: %w", root, h.Root, errOrder)
			}
		}
	}
	key := model.Hold{Root: root, Which: which}
	if mine, ok := l.held[key]; wait && ok {
		return model.Hold{}, &HeldError{Root: root, Which: which, Since: mine.since}
	}
	taken, err := l.take(ctx, root, which, wait)
	if err != nil {
		return model.Hold{}, err
	}
	taken.root = l.Tree.adopt(taken.state)
	// Only the root lock keeps another gragas out of the root's journal, so
	// only its hold keeps what was seen of it; under the scratch lock alone
	// every intent and close reads.
	if which == model.RootLock {
		taken.seen = &seen{}
	}
	if l.held == nil {
		l.held = map[model.Hold]holding{}
	}
	l.held[key] = taken
	return model.Hold{Root: root, Which: which, Since: taken.since}, nil
}

// Release puts back the lock Take answered: its note is removed through the
// state folder it was taken in, the lock dropped, and the folder let go of. A
// hold that is not held, or the zero hold, is an error and changes nothing.
func (l *Lock) Release(h model.Hold) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := model.Hold{Root: h.Root, Which: h.Which}
	taken, ok := l.held[key]
	if h == (model.Hold{}) || !ok {
		return fmt.Errorf("release: the %s lock of %q is not held", h.Which, h.Root)
	}
	delete(l.held, key)
	if taken.root != "" {
		l.Tree.drop(taken.root)
	}
	if err := taken.put(); err != nil {
		return fmt.Errorf("release the %s lock of %s: %w", h.Which, h.Root, err)
	}
	return nil
}

// put removes h's note through its state folder, then drops the lock and
// closes the folder, all three whatever the others answer.
func (h holding) put() error {
	return errors.Join(unnote(h.state, h.note), h.lock.Release(), h.state.Close())
}

// folder is the name among tree's roots of the state folder the lock which on
// root was taken in, while this adapter holds it there and tree is its Tree,
// and what the Journal has seen of root's journal under that hold. It takes
// l.mu, so it waits while a Wait of this adapter waits.
func (l *Lock) folder(tree port.Tree, root model.Root, which model.Lock) (model.Root, *seen, bool) {
	if l == nil {
		return "", nil, false
	}
	mine, isTree := tree.(*Tree)
	l.mu.Lock()
	defer l.mu.Unlock()
	h, ok := l.held[model.Hold{Root: root, Which: which}]
	if !ok || h.root == "" || !isTree || l.Tree != mine {
		return "", nil, false
	}
	return h.root, h.seen, true
}

// take takes the lock file of root in l's state folder through go-files,
// writes its note through the folder's root, and reads back the second the
// note says. A lock another process holds, or this one, is a HeldError. l.as
// is a test's account, or nil for this process's. go-files takes the lock by
// its local name below the checked state folder's own handle (lock.TakeIn),
// so a folder swapped at its path between the check and the take is never
// reached: the lock and its note are the checked folder's. l.taking is a
// test's step between them. With wait, go-files' lock.WaitIn waits for
// another holder until ctx ends, and a lock still held then is the HeldError
// joined with ctx's error. The folder stays open in the holding answered, and
// is closed on every refusal. The caller holds l.mu.
func (l *Lock) take(ctx context.Context, root model.Root, which model.Lock, wait bool) (holding, error) {
	fail := func(err error) error { return fmt.Errorf("take the %s lock of %s: %w", which, root, err) }
	apart, err := runApart(l.Dir, l.Roots, string(root))
	if err != nil {
		return holding{}, fail(err)
	}
	if l.judged != nil {
		l.judged()
	}
	state, name, err := lockFile(l.Dir, l.Mounts, apart, root, which, l.as)
	if err != nil {
		return holding{}, fail(err)
	}
	if l.taking != nil {
		l.taking(filepath.Join(l.Dir, name))
	}
	// go-files is given no note: it would write and remove one itself. The
	// note is written here, through state, and removed by Release the same
	// way.
	var taken *lock.Lock
	if wait {
		taken, err = lock.WaitIn(ctx, state, name, "")
	} else {
		taken, err = lock.TakeIn(state, name, "")
	}
	var held lock.Held
	switch {
	case errors.As(err, &held) && wait:
		return holding{}, closed(errors.Join(&HeldError{Root: root, Which: which, Since: heldSince(state, holderName(name))}, ctx.Err()), state)
	case errors.As(err, &held):
		return holding{}, closed(&HeldError{Root: root, Which: which, Since: heldSince(state, holderName(name))}, state)
	case err != nil:
		return holding{}, closed(fail(err), state)
	}
	h := holding{lock: taken, state: state, note: holderName(name)}
	if err := writeNote(state, h.note, l.Clock.Now()); err != nil {
		return holding{}, errors.Join(fail(fmt.Errorf("it could not write its note: %w", err)), h.put())
	}
	if h.since, err = noted(state, h.note); err != nil {
		return holding{}, errors.Join(fail(fmt.Errorf("it could not read back its start time: %w", err)), h.put())
	}
	return h, nil
}

// closed is err once state is closed, and the close's error joined to it only
// when there is one, so a HeldError alone stays the error itself.
func closed(err error, state *os.Root) error {
	if cerr := state.Close(); cerr != nil {
		return errors.Join(err, cerr)
	}
	return err
}

// lockFile makes the state folder when it is missing, opens it as a root and
// names the lock file of root in it. A state folder an account other than
// this run's owns is refused: go-files opens the lock file by its path, so
// the folder's owner could swap a name there after the check below.
// A name that holds a link, or anything but a
// file, is refused before go-files opens it.
func lockFile(dir string, mounts, apart []string, root model.Root, which model.Lock, as func() string) (*os.Root, string, error) {
	if err := privateDirApart(dir, mounts, apart); err != nil {
		return nil, "", err
	}
	state, err := os.OpenRoot(dir)
	if err != nil {
		return nil, "", err
	}
	if err := mine("the state folder", state, as); err != nil {
		return nil, "", errors.Join(err, state.Close())
	}
	name := lockName(root, which)
	switch info, err := state.Lstat(name); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, "", errors.Join(err, state.Close())
	case !info.Mode().IsRegular():
		return nil, "", errors.Join(fmt.Errorf("%s: %w", name, errNotALockFile), state.Close())
	}
	return state, name, nil
}

// lockName is the lock file's name in the state folder. The root lock is one
// for each root, named by the root's digest. The scratch lock is one for the
// machine, whatever the root (model.ScratchLock): the scratch folders are
// made in one folder for every root, so a clear from one root would otherwise
// remove the folder a run of another holds.
func lockName(root model.Root, which model.Lock) string {
	if which == model.ScratchLock {
		return string(which) + ".lock"
	}
	return digest.Hex([]byte(root)) + "." + string(which) + ".lock"
}

// holderName is holder's name of the note of the lock file name, in the
// state folder.
func holderName(name string) string { return name + ".holder" }

// unnote removes the note at name in state, a link there as the link and
// never its target, through go-files' confine.Remove. A note already gone is
// no error.
func unnote(state *os.Root, name string) error {
	if err := confine.Remove(state, name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("its note could not be removed: %w", err)
	}
	return nil
}

// heldSince is the second the note at note below state says another holder
// took the lock, read by lock.HolderIn, and 0 for a note it cannot read: the
// note is for a message only, as go-files says, and is read, never written,
// here.
func heldSince(state *os.Root, note string) int64 {
	held, _, err := lock.HolderIn(state, note)
	if err != nil {
		return 0
	}
	return seconds(held.Since)
}

// noted is the second in the lock's note at note below state, which this
// process has just written: lock.HolderIn reads it, and a note that names
// another process, or none, or holds no time, is refused.
func noted(state *os.Root, note string) (int64, error) {
	held, _, err := lock.HolderIn(state, note)
	if err != nil {
		return 0, err
	}
	if held.By != os.Getpid() {
		return 0, errors.New("the lock's note does not name this process")
	}
	if seconds(held.Since) == 0 {
		return 0, errors.New("the lock's note holds no time")
	}
	return seconds(held.Since), nil
}

// seconds is t in seconds, UTC, and 0 for a time before 1970 or none at all:
// a time that could not be read is unread, never a guess.
func seconds(t time.Time) int64 {
	if s := t.Unix(); s > 0 {
		return s
	}
	return 0
}

// privateDirApart makes the folder dir, absolute, when it is missing, and each
// missing folder above it, this account's alone, once every name of it that
// is there is judged (as the sandbox's make), by
// host/trust's Private, which is go-files' judged private make: the nearest
// folder of dir that is there, dir itself when it is, is walked first, top
// down, links followed and each name judged by its owner and who else may
// write it, so nothing is made through a name the walk refuses. A state
// folder reached through a link in a folder another account may write is
// refused by its name, and neither it, the lock file nor the journal is made
// in the folder that account chose. The rest is made at 0700 with no link
// followed, one folder at a time; on Windows each has the owner-only access
// list, which os.MkdirAll would not give. A folder that is there keeps its
// mode, for its caller to judge its owner. mounts are the configured mounts
// the walk trusts, and the folders of apart, as apartOf answers them, are
// held apart by the walk (go-files's Who.Apart), so no folder of
// dir is made through a link into one. The approvals hand it the folders a
// clone carries, so a link made at a missing name of the state folder after
// their carried check leads nothing into a clone.
func privateDirApart(dir string, mounts, apart []string) error {
	made, err := hosttrust.Private(dir, stateSubject(), hosttrust.Who{Mounts: mounts, Apart: apart})
	if err != nil {
		return stateRefusal(dir, err)
	}
	return made.Close()
}

// stateFolder is the walk of the state folder dir that is there: host/trust's
// walk of it as a folder, whose refusal names dir and keeps the walk's
// *Unsafe, its fix line said. It trusts the configured mounts, as every other
// walk does: a state folder on a nosuid disk named there passes.
func stateFolder(dir string, mounts []string) error {
	_, err := hosttrust.Walk(stateSubject(), dir, hosttrust.Who{Mounts: mounts})
	return stateRefusal(dir, err)
}

// stateSubject names the state folder in the walks of stateFolder and
// privateDirApart.
func stateSubject() hosttrust.Subject {
	return hosttrust.Subject{
		Name: "the state folder", File: "the state folder", Last: hosttrust.Folder, Bare: true,
		Point: "point this account's state folder there",
	}
}

// stateRefusal words err, a refusal of the walk of the state folder dir, by
// its name and with its fix line, keeping the walk's *Unsafe; any other err
// is its own.
func stateRefusal(dir string, err error) error {
	if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		return fmt.Errorf("the state folder %s is not trusted, so nothing is made or taken in it: %w: %s", dir, err, u.Fix)
	}
	return err
}
