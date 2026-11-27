package trust

// The settings file and the tools file name the programs gragas runs, so
// each is read only when this account or root owns it and neither its group
// nor others can write it (cli.config.12). go-files' ReadOwned makes the
// check on the file itself, through the descriptor it reads, never on a path
// checked first and opened after, and never through a link at the name or
// on a pipe that would wait.

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"
)

// MaxOwned is the most a file ReadOwned reads may hold.
const MaxOwned = 1 << 20

// rootAccount is root as ReadOwned names an owner. On Windows no SID is
// "uid:0", so the second read is refused as the first was, and the first
// refusal is the one said.
const rootAccount = "uid:0"

// Owned is what ReadOwned returns for a file it will not read: another
// account owns it, its group or others can write it, it is not a regular
// file, or it is larger than MaxOwned. It matches go-files' refusal, as
// trust.ErrNotOwners.
type Owned struct {
	Path string
	// Why says what is wrong, as the end of "the settings file <path> ...".
	Why string
	// Fix is the line that fixes it, "" when there is none to type.
	Fix string
	// Reason is which refusal it is, told by go-files' error and never by its
	// words, for a reader that must not print Why or Fix, which name the path.
	Reason Reason
	err    error
}

// Reason is which refusal of the owned read an Owned is.
type Reason int

const (
	// Other is a refusal of none of the reasons below: a file replaced at
	// each try of its check, or one refused for the kernel's overflow account.
	Other Reason = iota
	// ByOther is a file an account but this one or root owns.
	ByOther
	// Writable is a file its group or another account can write.
	Writable
	// IsLink is a link at the name, which is never followed.
	IsLink
	// NotRegular is a folder, a pipe or a device at the name.
	NotRegular
	// TooLong is a file over MaxOwned, refused whole.
	TooLong
	// OnMount is a file on a disk whose owners go-files does not judge: a
	// Linux mount it does not judge, or a macOS volume that ignores ownership.
	OnMount
	// MountEntry is a folder of the mounts handed in that go-files refuses as
	// an entry, before the file is opened.
	MountEntry
)

// reasonOf is the Reason of go-files' refusal err of the file at path. A
// refusal that matches two is the narrower's: ErrMountEntry matches ErrMount,
// and ErrMount and ErrOwnersIgnored match ErrNotOwners.
func reasonOf(path string, err error) Reason {
	switch {
	case errors.Is(err, gotrust.ErrMountEntry):
		return MountEntry
	case errors.Is(err, gotrust.ErrMount), errors.Is(err, gotrust.ErrOwnersIgnored):
		return OnMount
	case errors.Is(err, gotrust.ErrNotOwners):
		return ByOther
	case errors.Is(err, gotrust.ErrOthersWrite):
		return Writable
	case errors.Is(err, gotrust.ErrNotPlain) && isLink(path):
		return IsLink
	case errors.Is(err, gotrust.ErrNotPlain):
		return NotRegular
	case errors.Is(err, gotrust.ErrTooLong):
		return TooLong
	}
	return Other
}

func (o *Owned) Error() string { return o.Path + " " + o.Why }

func (o *Owned) Unwrap() error { return o.err }

// Refusal is what is wrong and the line that fixes it, which config's
// reader reads without importing this package.
func (o *Owned) Refusal() (why, fix string) { return o.Why, o.Fix }

// ReadOwned reads the whole file at path when this account owns it, or
// root does, and neither its group nor others can write it. A missing file
// is an error that matches fs.ErrNotExist. Root's is read only once this
// account's read is refused for its owner alone, and each read checks the
// descriptor it reads. mounts are GRAGAS_MOUNTS's folders, as config hands
// them, on whose disks a file mounted nosuid is judged by its owners in
// place of refused; go-files judges each as an entry before
// it opens the file, and a refused one is the file's
// refusal, naming the mount. In a test binary it asks with Who.Overflow
// first, as walk does, since in a user namespace the names above an entry
// are root's as the overflow account.
func ReadOwned(path string, mounts []string) ([]byte, error) {
	me, err := gotrust.Account()
	if err != nil {
		return nil, err
	}
	return readOwned(path, me, rootAccount, mounts)
}

// readOwned is ReadOwned for the accounts me and root, which a test of this
// package names, so it sees another account's file refused and root's read
// with no second account.
func readOwned(path, me, root string, mounts []string) ([]byte, error) {
	data, err := read(path, me, mounts)
	if errors.Is(err, gotrust.ErrNotOwners) && !errors.Is(err, gotrust.ErrMount) && me != root {
		var again error
		data, again = read(path, root, mounts)
		if again == nil || !errors.Is(again, gotrust.ErrNotOwners) {
			err = again
		}
	}
	if err == nil {
		return data, nil
	}
	return nil, owned(path, me, err)
}

// read is go-files's ReadOwned of path for owner, with mounts as its
// Who.Mounts, asked with Who.Overflow first in a test binary, as walk asks
// Path; where go-files refuses that with ErrOverflow, as off Linux or in the
// first namespace, it is asked again without it. A program asks once.
func read(path, owner string, mounts []string) ([]byte, error) {
	if testing.Testing() {
		data, err := gotrust.ReadOwned(path, owner, false, MaxOwned, gotrust.Who{Overflow: true, Mounts: mounts})
		if !errors.Is(err, gotrust.ErrOverflow) {
			return data, err
		}
	}
	return gotrust.ReadOwned(path, owner, false, MaxOwned, gotrust.Who{Mounts: mounts})
}

// owned is go-files' refusal err of the file at path, read for the account
// me, in the words config puts after "the settings file <path>", with the
// line that fixes it. An error that is no refusal, a missing file among
// them, is answered as it is.
func owned(path, me string, err error) error {
	var u *gotrust.Unsafe
	if !errors.As(err, &u) || errors.Is(err, fs.ErrNotExist) {
		return err
	}
	o := &Owned{Path: path, Why: strings.TrimPrefix(u.Observed, path+" "), Fix: fixLine(u.Fix), Reason: reasonOf(path, err), err: err}
	switch {
	case errors.Is(err, gotrust.ErrMountEntry):
		// go-files judged a folder of the variable before it opened the
		// file: the mount the person named is why, not the
		// file, so its words name the mount and the variable.
		o.Why, o.Fix = "is read with the mounts "+MountsVar+" names, and "+mountEntry(u).Observed, readMountEntryFix
	case errors.Is(err, gotrust.ErrMount):
		// ErrMount wraps ErrNotOwners, and no owner is why: the disk is. The
		// mounts setting is in the file this refuses, so only the variable
		// can let it be read.
		o.Fix = readMountFix
	case errors.Is(err, gotrust.ErrNotOwners):
		// go-files names only the owner it was asked for, this account.
		by, _, _ := strings.Cut(o.Why, ", not by ")
		o.Why, o.Fix = notMine(by, me), fixLine(gotrust.Fix{Path: path, Owner: me})
	case errors.Is(err, gotrust.ErrNotPlain) && isLink(path):
		// go-files opened the name with no link followed; the name is told
		// apart from a pipe or a folder by its own entry, for the words only.
		o.Fix = unlink
	}
	return o
}

// administrators is the SID of Windows' Administrators group, which owns a
// file an administrator makes from an elevated shell. gragas reads root's
// file as "uid:0", which no SID is, so such a file stays refused, failing
// closed, and its refusal says how to hand it to this account.
const administrators = "S-1-5-32-544"

// notMine is why a file owned as by says, "is owned by uid:0", is refused to
// the account me. A file of Administrators says so, and on Windows the words
// say the fix wants an elevated shell.
func notMine(by, me string) string {
	why := by + ", not by this account or root"
	if strings.HasSuffix(by, " "+administrators) {
		why = by + ", the Administrators group, as a file made from an elevated shell is, not by this account or root"
	}
	if strings.HasPrefix(me, "S-1-") {
		why += "; only an elevated shell can give it to this account"
	}
	return why
}

// readMountFix is the fix of a settings or tools file on a disk whose
// owners go-files does not judge: the mounts setting is read from the
// settings file, so only GRAGAS_MOUNTS lets either be read.
const readMountFix = "when root mounted that disk for this machine, set " + MountsVar + " to the folder it is mounted on; the " + MountsKey + " setting is in the settings file, so it cannot let that file be read; else move the file to a disk whose owners are judged"

// readMountEntryFix is the fix of a mount in GRAGAS_MOUNTS that go-files
// refuses as an entry (ErrMountEntry), MountEntryFix for the variable alone.
const readMountEntryFix = "set " + MountsVar + " to the folder the disk is mounted on itself, as root made it, with no link on the way and no folder above it another account can write"

// unlink is the fix of a settings or tools file that is a link: gragas
// follows none, so only the file itself at the name is read.
const unlink = "replace the link with the file itself"

// isLink is whether the name path is a link itself.
func isLink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().Type() == fs.ModeSymlink
}
