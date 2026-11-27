// Package trust is gragas's one adapter over go-files's trust walk
// (trust-walk): the check of a path before gragas runs or reads what it
// names, git, an enforcer and its interpreter. go-files walks the path from
// the top of its volume down, one name at a time, read without following it,
// and judges every folder, link and link target on the way; this package is the one place gragas calls it.
// It adds only what is gragas's: the group whose write it accepts, and the
// words a caller puts around a refusal. Each caller words its subject and
// turns the errors here into its own.
package trust

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"
)

// Kind is what the last name of a path must be.
type Kind = gotrust.Kind

// The kinds of the last name, as go-files names them.
const (
	// Program is a regular file its owner may execute.
	Program = gotrust.Program
	// Folder is a folder.
	Folder = gotrust.Folder
	// File is a regular file that is read and not run.
	File = gotrust.File
)

// Subject is how a caller names the path the walk checks, for a person.
type Subject struct {
	// Name is what a person calls it: "git", "the tool".
	Name string
	// File is what a person moves when it is in the wrong folder: "git",
	// "ssh", "the tool".
	File string
	// Last is the kind its last name must be.
	Last Kind
	// Point is the end of the line that moves it: what to set, and to what,
	// once it is moved: "set programs.git to its path there".
	Point string
	// Bare says what the walk saw alone, "/usr/bin can be written by its
	// group", where the walk would say "git at /usr/bin/git: ...".
	Bare bool
}

// Who is whom the walk trusts besides root: this account, unless RootOnly.
type Who struct {
	// RootOnly trusts root alone: not this account, no group, no sticky
	// folder and no link.
	RootOnly bool
	// NoLinks refuses a link anywhere on the path, the last name included.
	NoLinks bool
	// Apart is folders the path may not be, nor lie below, each absolute and
	// clean: go-files's Who.Apart, which reads each once through its links
	// and refuses a name on the way that is the same file, by its file ID,
	// whatever spelling reached it.
	Apart []string
	// Mounts is folders, each absolute and clean, on whose disk a name
	// mounted nosuid, or noexec without nosuid, is judged by its owners in
	// place of refused: go-files's Who.Mounts, which reads them on Linux
	// alone. go-files judges each as an entry at every call, before the
	// path, and a refused one is the walk's refusal, naming it.
	Mounts []string
}

// Unsafe is a name on the way the walk does not trust. Fix is the line that
// fixes it. Owned marks the refusal of a name an account the walk does not
// trust owns, whose Fix is the hand-over line: a caller for whom handing the name
// to root is not the fix, as git's fetch of another account's repository,
// gives its own line in its place. Inside is the name on the
// way that is a folder of Who.Apart, as the walk reached it, when that is
// the refusal, so a caller names it in its own words. Unjudged marks a
// name whose writers the walk cannot judge, not one it judged another
// account may write: on a mount whose owners go-files does not judge (9p,
// CIFS, a file system of a type it does not know, ZFS with an NFSv4 list),
// on a volume that ignores ownership, or with an access list it does not
// read.
type Unsafe struct {
	Observed, Fix string
	Owned         bool
	Inside        string
	Unjudged      bool
}

func (e *Unsafe) Error() string { return e.Observed }

// Failed is a name the walk could not read: not an answer about the path.
type Failed struct {
	// What is the sentence for a person: "the path of git could not be
	// read".
	What string
	// Err is what the read answered.
	Err error
}

func (e *Failed) Error() string { return e.What + ": " + e.Err.Error() }

func (e *Failed) Unwrap() error { return e.Err }

// walk is go-files's walk of set for who. In a test binary on Linux it asks
// first with Who.Overflow, which go-files grants only there and only in a
// user namespace that maps this account and not root, as the namespace
// member's, where root's / shows as the overflow account's.
// Where go-files refuses that with ErrOverflow, as in the first namespace,
// the walk is asked again without it. A program asks once, never with it.
func walk(set string, last Kind, who gotrust.Who) (string, error) {
	if testing.Testing() {
		in := who
		in.Overflow = true
		if path, err := gotrust.Path(set, last, in); !errors.Is(err, gotrust.ErrOverflow) {
			return path, err
		}
	}
	return gotrust.Path(set, last, who)
}

// Walk resolves set, the path of s, and answers it with no link left in it
// when go-files's walk passes every name on the way for who. A name that is
// not there answers the read's own error, which matches fs.ErrNotExist; one
// the walk does not trust an *Unsafe in the words of s; and one it cannot
// read a *Failed. A set that is not absolute and clean is Unsafe before any
// name is read.
func Walk(s Subject, set string, who Who) (string, error) {
	admin, err := group()
	if err != nil {
		return "", &Failed{What: "the admin group could not be read, so the path of " + s.Name + " is not trusted", Err: err}
	}
	path, err := walk(set, s.Last, gotrust.Who{RootOnly: who.RootOnly, NoLinks: who.NoLinks, Group: admin, Apart: who.Apart, Mounts: who.Mounts})
	var u *gotrust.Unsafe
	switch {
	case err == nil:
		return path, nil
	case errors.As(err, &u):
		return "", s.unsafe(set, u, err)
	case errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	return "", &Failed{What: "the path of " + s.Name + " could not be read", Err: err}
}

// unsafe is go-files's refusal u of set, in the words of s. A refusal with
// no fix of go-files's own is a name right in an open sticky folder, as
// /tmp, or one of the wrong kind: only moving it fixes it, so the line is
// the one that moves the file of s and points its setting there.
//
// What the walk saw starts with the name it judged. When that is set
// itself, the path is said once, "the tool at X can be written by its
// group"; when it is a folder on the way, both are said, "git at
// /usr/bin/git: /usr/bin can be written by its group". go-files's Unsafe
// carries the judged name only in its words, so the prefix is what is read.
//
// err is the whole refusal that holds u, and what it matches is judged on
// err, not u alone, as owned judges a read's: a refusal that wraps u beside
// ErrMount is the disk's, with MountFix, on every system, though go-files
// gives ErrMount only on Linux.
func (s Subject) unsafe(set string, u *gotrust.Unsafe, err error) *Unsafe {
	if errors.Is(err, gotrust.ErrMountEntry) {
		return mountEntry(u)
	}
	observed := u.Observed
	switch {
	case s.Bare:
	case strings.HasPrefix(observed, set+" "):
		observed = s.Name + " at " + observed
	default:
		observed = s.Name + " at " + set + ": " + observed
	}
	fix := fixLine(u.Fix)
	owned := errors.Is(err, gotrust.ErrNotOwners) && fix != ""
	if fix == "" {
		fix = s.moveLine(err)
	}
	r := &Unsafe{Observed: observed, Fix: fix, Owned: owned, Unjudged: unjudged(err)}
	if errors.Is(err, gotrust.ErrInside) {
		r.Inside = u.Path
	}
	return r
}

// MountFix is the fix line of a name on a mount whose owners go-files does
// not judge (ErrMount): a file system it does not judge, or a disk mounted
// nosuid, or noexec without nosuid, whose owners it takes once Who.Mounts
// names a folder on it. Another folder on that disk is
// refused too, so the line does not send the user there. It is exported for
// the one judgement outside the walk, the sandbox's trust.Held of its base,
// so both say one line.
const MountFix = "the way the disk under it is mounted is why, not the folder, so a folder on that disk is refused too: when root mounted that disk for this machine, name the folder it is mounted on in " + MountsVar + " or the " + MountsKey + " setting; else use a folder on a disk whose owners are judged"

// MountsVar and MountsKey are the environment variable and the settings key
// that name the mounts gragas trusts, as config reads them.
// The fix lines say them; this package reads neither.
const (
	MountsVar = "GRAGAS_MOUNTS"
	MountsKey = "mounts"
)

// MountEntryFix is the fix line of a mount go-files refuses as an entry
// (ErrMountEntry): one it cannot lend a disk's pass to.
const MountEntryFix = "name the folder the disk is mounted on itself, as root made it, with no link on the way and no folder above it another account can write, in " + MountsVar + " or the " + MountsKey + " setting"

// mountEntry is go-files's refusal u of a folder of Who.Mounts, which it
// judges at every call before the path: not absolute and
// clean, below a name an account but root may change, a link, or no
// mount's root. go-files names the entry by its field; this is the same
// refusal in the words of the setting a person set, "the mount /mnt, which
// is no mount's root: ...", with MountEntryFix. It is never Unjudged, nor
// MountFix's: ErrMountEntry wraps ErrMount, but the disk is not why, the
// name the person gave is.
func mountEntry(u *gotrust.Unsafe) *Unsafe {
	return &Unsafe{Observed: "the mount " + u.Path + entryWords(u), Fix: MountEntryFix}
}

// entryWords is what go-files says of the entry u refuses, after its name:
// ", which is no mount's root: ..." where go-files's words start with what
// the field stands for and the quoted entry (go-files v0.10.0 says "the mounts
// named as trusted hold", where v0.9.0 named the field, Who.Mounts), else ": "
// and its words whole.
func entryWords(u *gotrust.Unsafe) string {
	if rest, ok := strings.CutPrefix(u.Observed, "the mounts named as trusted hold "+strconv.Quote(u.Path)); ok {
		return rest
	}
	return ": " + u.Observed
}

// Held judges f, a file or folder already open, as go-files's trust.Held
// judges it for last, with mounts as its Who.Mounts, each judged as an
// entry before f is. In a test binary it asks first with
// Who.Overflow, as walk does, since in a user namespace the names above an
// entry are root's as the overflow account. A refused entry is an *Unsafe
// naming it, with MountEntryFix; any other refusal is go-files's error as
// Held gives it, for its caller to word.
func Held(f *os.File, last Kind, mounts []string) error {
	err := held(f, last, mounts)
	if u, ok := errors.AsType[*gotrust.Unsafe](err); ok && errors.Is(err, gotrust.ErrMountEntry) {
		return mountEntry(u)
	}
	return err
}

// held is go-files's Held of f for last and mounts, asked with Who.Overflow
// first in a test binary, as walk asks Path.
func held(f *os.File, last Kind, mounts []string) error {
	if testing.Testing() {
		if err := gotrust.Held(f, last, gotrust.Who{Overflow: true, Mounts: mounts}); !errors.Is(err, gotrust.ErrOverflow) {
			return err
		}
	}
	return gotrust.Held(f, last, gotrust.Who{Mounts: mounts})
}

// moveLine is the fix line of a refusal err that go-files gives no fix of
// its own: the mount's line, or the line that moves the file of s and points
// its setting there, or none when s names no setting.
func (s Subject) moveLine(err error) string {
	switch {
	case errors.Is(err, gotrust.ErrMount):
		return MountFix
	case s.Point != "":
		return "move " + s.File + " to a folder of this account's, and " + s.Point
	}
	return ""
}

// unjudged is whether go-files refused with err because it cannot judge who
// may write the name: ErrMount, ErrOwnersIgnored, or ErrUnsupported for a
// list it does not read. Its ErrUnsupported of Who.Overflow off Linux is a
// setting refused, not a name.
func unjudged(err error) bool {
	switch {
	case errors.Is(err, gotrust.ErrMount), errors.Is(err, gotrust.ErrOwnersIgnored):
		return true
	}
	return errors.Is(err, errors.ErrUnsupported) && !errors.Is(err, gotrust.ErrOverflow)
}

// group is the group whose write the walk accepts: on macOS
// the admin group, which owns the folders of a package manager every
// administrator shares, as Homebrew's /opt/homebrew; elsewhere none, so a name
// its group can write is refused as any other. go-files names the group
// (trust.AdminGroup) and it is read once per process. A failed lookup is an
// error, never a guess: the walk refuses by name.
var group = sync.OnceValues(func() (func(gid uint32) bool, error) {
	return groupFrom(gotrust.AdminGroup)
})

// groupFrom is group over the lookup of the admin group.
func groupFrom(lookup func() (uint32, bool, error)) (func(gid uint32) bool, error) {
	gid, ok, err := lookup()
	switch {
	case err != nil:
		return nil, err
	case !ok:
		return nil, nil
	}
	return func(g uint32) bool { return g == gid }, nil
}

// Quote is s as one word for a POSIX shell: a value named in prose, never a
// line to paste. A line to paste is shell.Quote's, for the run's dialect.
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
