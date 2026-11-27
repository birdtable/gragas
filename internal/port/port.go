// Package port is the contract between gragas's core and the world outside
// it (arch.ports.1, arch.ports.10): every effect a run asks for, a file read
// or replaced, a line journalled, a tag read, a tool run, the time, a
// question asked, is a method of an interface here, and one package under
// internal/host fills it. There are seventeen ports: the sixteen declared in
// this file, which include Owned, FileNames, Trust, Steps and Bundles (Tool
// went, its finds and runs being Steps'), and Plugins, declared in plugin.go.
// Each is one
// subject, named by a singular noun, narrow, in the core's verbs. A method
// says nothing about how an adapter does it, and no field or name here is one
// platform's (arch.ports.5).
//
// The package holds the interfaces, the one set main fills and the typed
// failures a caller branches on, and nothing that runs. The values that
// cross a port are types in internal/model. Ports is filled by hand in
// cmd/gragas (arch.ports.11); internal/port/testing is the fake. The ports
// of the engine's first form, which the old commands still use until M6,
// are in internal/port/legacy and are not counted here.
//
// Every context is held to the same reading of an error: a missing file
// matches fs.ErrNotExist, a path that leaves its root matches ErrOutside, a
// host path Within or Written is handed that is not clean matches
// ErrNotClean, one whose last part is a link matches ErrLinked, a file
// ReadSoleWithin finds with more than one name matches ErrManyNames, a
// lock that is held matches ErrHeld, a folder written by its path that
// another account owns matches ErrNotOwner, and a port its platform cannot
// serve matches ErrUnavailable. Anything else is the adapter's own failure and
// says what failed in words a person can read.
package port

import (
	"context"
	"errors"
	"io/fs"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// ErrHeld is what Lock.Take's error matches when another process holds the
// lock. The error itself says which process, and since when.
var ErrHeld = errors.New("the lock is held")

// ErrOutside is what an error matches when a path names something outside its
// root, by climbing, by an absolute path or through a link. Nothing outside a
// root is ever read or written.
var ErrOutside = errors.New("the path leaves its root")

// ErrNotClean is what Tree.Within's and Tree.Written's error matches when the
// host path it is given is not clean, with a part that is one dot or two, or
// a separator doubled or at the end. Such a path is never placed, since a
// link before a .. may lead the kernel to one file and a tool that cleans the
// spelling to another.
var ErrNotClean = errors.New("the path is not clean")

// ErrLinked is what Tree.Within's error matches when the last part of the
// host path it is given is a link, and the error names the file the link
// leads to. Such a path is never placed, since a tool that writes the file in
// place writes the one the link leads to, and a tool that writes a new file
// and renames it over replaces the link itself.
var ErrLinked = errors.New("the path's last part is a link")

// ErrManyNames is what Tree.ReadSoleWithin's error matches when the file at the
// path has more than one name, by hard links. Such a file is changed under
// every name it has, so a judge that read it by one name would let an edit
// through the others go unjudged.
var ErrManyNames = errors.New("the file has more than one name")

// ErrNoRev is what History's error matches when the revision asked for does
// not exist: a repository with no commit has no HEAD, and one with no merge
// in progress no MERGE_HEAD. It is never a path the revision lacks, which
// matches fs.ErrNotExist.
var ErrNoRev = errors.New("the revision does not exist")

// ErrNoBase is what History.Base's error matches when two revisions have no
// one merge base: none in common, or more than one, as after a criss-cross
// merge, where none is picked.
var ErrNoBase = errors.New("no one merge base")

// ErrNotOwner is what Writer.Replace's and Lock.Take's error matches when the
// folder they write in by its path, the root or the state folder, is owned by
// another account than the one this process runs as, root included: its
// owner could swap a folder on that path while the write goes, and carry it
// out of the folder. Nothing was written. The error says which folder and
// whose it is.
var ErrNotOwner = errors.New("another account owns the folder")

// ErrUnavailable is what every method of a port answers, matching this, when
// the platform cannot serve that port, such as no sandbox. It is the
// only way a port is absent. The Ports field is still filled, so no code
// tests a port for nil (arch.ports.5).
var ErrUnavailable = errors.New("this platform cannot serve that")

// ErrRefused is what a run's error matches, a step's through Steps or a
// folder Sandbox would not prepare, when what was asked does not fit, and so
// nothing was started.
var ErrRefused = errors.New("the call does not fit its shape")

// ErrNotHeld is what a step's run matches, beside ErrRefused, when the
// step is held to a memory limit that this system can neither hold the run's
// tree to while it runs nor read the run's peak against after it: Linux with
// no cgroup delegated to the program. Nothing was
// started, and no setting of the tool's fixes it: the host's does.
var ErrNotHeld = errors.New("this system can neither hold nor read the run's memory")

// ErrNotOwned is a file outside the roots that the owned read refuses:
// another account owns it or can write it, or it is not a regular file this
// account may read whole.
var ErrNotOwned = errors.New("another account owns the file or can write it")

// ErrCarried is what each method of Trust, Journal and Lock answers, matching
// this, when the state folder it keeps its records in lies inside a folder
// whose files another person may have written: the working folder, the
// repository around it, the root the call names, or gragas's cache folder,
// where every rulebook a run fetches is kept. An approval, a journal or a kept
// copy there would come with the clone to every machine it is cloned to, or be
// the rulebook author's, and an open run planted there would be rolled back
// into the root by the next run that writes, so nothing is read, made or taken
// there. The Journal and the Lock judge only a state folder a variable moved:
// the account's own, under the home the system records, no clone and no
// rulebook chooses.
var ErrCarried = errors.New("the state folder lies in a folder this run works in or fetches rulebooks into")

// The reasons of an owned read's refusal, each matched beside ErrNotOwned by
// a refusal for it, so a reader that must not print the refusal's own words,
// which name the path, says which it was and its fix from words of its own.
// A refusal for none of them, a file replaced at each try of
// its check among them, matches ErrNotOwned alone.
var (
	// ErrOwnedByOther is a file an account but this one or root owns.
	ErrOwnedByOther = errors.New("another account owns the file")
	// ErrOwnedWritable is a file its group or another account can write.
	ErrOwnedWritable = errors.New("another account can write the file")
	// ErrOwnedLink is a link at the path, which the owned read never follows.
	ErrOwnedLink = errors.New("the file is a link")
	// ErrOwnedNotRegular is a folder, a pipe or a device at the path.
	ErrOwnedNotRegular = errors.New("the file is not a regular file")
	// ErrOwnedTooLong is a file over the most the owned read reads, 1 MiB,
	// refused whole.
	ErrOwnedTooLong = errors.New("the file holds more than the owned read reads")
	// ErrOwnedMount is a file on a disk whose owners are not judged.
	ErrOwnedMount = errors.New("the file is on a disk whose owners are not judged")
	// ErrOwnedMountEntry is a mount the file is read through that is refused
	// as an entry, before the file is opened.
	ErrOwnedMountEntry = errors.New("a mount the file is read through is refused")
)

// Tree reads a root's files, read-only. A root is opened once and then named
// by its model.Root.
type Tree interface {
	// Open opens the folder at dir, absolute or relative to the working
	// folder, as a root, confined so that nothing outside it is reachable. It
	// answers the same root each time for the same folder. An empty dir is the
	// working folder. A path that is not a folder is an error matching
	// fs.ErrNotExist or an adapter failure.
	Open(dir string) (model.Root, error)
	// Locate opens the nearest folder at or above start, an absolute
	// folder, that holds a file named marker, walking up once. When none
	// holds it, the error matches fs.ErrNotExist.
	Locate(start, marker string) (model.Root, error)
	// Read answers the whole content of the file at path, slash-separated and
	// relative to root. A missing file matches fs.ErrNotExist, a path that
	// leaves the root ErrOutside, and a folder is an error. An empty file is
	// a nil or empty slice, never an error.
	Read(root model.Root, path string) ([]byte, error)
	// ReadSoleWithin answers the whole content of the file at path as Read
	// does, when that path is the file's one name and the file holds at most
	// max bytes. It counts the file's names through the handle it then reads
	// by, and reads no more than max+1 bytes from it, so the bytes answered are
	// those of the file counted and a file that grew past its bound is never
	// read whole. A file with more than one name, by hard links, matches
	// ErrManyNames; one with more than max bytes matches
	// pluginbundle.ErrOverBound; each answers no bytes. A count the system will
	// not give is an error, never a sole name. A name made after the count is
	// not seen. A max outside [0, MaxInt64) is no
	// bound and is refused, with no bytes, before the file is opened: never read
	// as unbounded, and max+1 never overflows.
	ReadSoleWithin(root model.Root, path string, max int64) ([]byte, error)
	// Stat answers the entry at path, slash-separated and relative to root,
	// as Read reads it: a size in bytes, never nil. A missing path matches
	// fs.ErrNotExist, and an empty path is the root itself.
	Stat(root model.Root, path string) (model.Entry, error)
	// List answers the files below dir, "" being the root, one entry per file
	// and per folder, sorted by path, in the scope asked. A folder with
	// nothing in it is an empty list, never an error, and so is a folder that
	// is not there when each of its parents is a folder or is not there
	// either: such a missing dir is no fs.ErrNotExist. A dir
	// through a file, or one that names a file, is not promised either way.
	// It opens no file, so no entry's Exec is set; Stat
	// answers whether a file is runnable.
	List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error)
	// Within answers where the file at abs, an absolute path as the host spells
	// it, lies in root: slash-separated and relative to root, once every link
	// in abs is followed as far as the path exists. An abs that is not clean
	// as the host spells a path matches ErrNotClean and is placed nowhere. A
	// path that lies outside root matches ErrOutside, and so does one that
	// lies inside through a link that leads out. A path whose last part is a
	// link that leads inside matches ErrLinked, naming where it leads, and is
	// placed nowhere. A relative or empty abs is an error. It reads no file.
	Within(root model.Root, abs string) (string, error)
	// Written answers where the file at abs lies in root as abs spells it:
	// slash-separated and relative to root, the part of abs the host reads as
	// root taken as root, and every part below it kept as written, no link
	// followed. A host that answers Within in the disk's own spelling answers
	// here the edit's. An abs that is not clean matches ErrNotClean, as for
	// Within. A path no part of which is root matches ErrOutside; a relative or
	// empty abs is an error.
	Written(root model.Root, abs string) (string, error)
	// OpenIn opens the folder at dir, slash-separated and relative to root,
	// as a root of its own, and answers the same root each time for the same
	// folder. An empty dir is root itself. A dir that leaves root, by .. or
	// through a link that leads out, matches ErrOutside; a link that stays
	// inside is followed. A missing dir matches fs.ErrNotExist, and a file is
	// an error. It is the one way a root is opened from a path a file names:
	// no host path is built.
	OpenIn(root model.Root, dir string) (model.Root, error)
	// FS answers the files of root as the standard read-only fs.FS, the view
	// a check that walks a tree takes, each file by its slash-separated path
	// relative to root and read as its whole content. Its listing is what
	// List shows with model.Every. A file is opened as Read opens one: a
	// link at any step of its path, inside root or leading out, is refused,
	// and so is a path with .., as the old check's reads refuse them.
	// A root that was not opened, or whose folder is gone, is
	// an error, never an empty view.
	FS(root model.Root) (fs.FS, error)
}

// Owned reads the one kind of file that lies outside every root: a file the
// settings name by an absolute path, read whole, only when this account or
// root owns it and no one else can write it. It is one subject, not
// Tree's: Tree answers for the files of a root it opened, and Owned for a
// path no root holds, which the trust walk judges before a byte is read.
type Owned interface {
	// ReadOwned answers the whole content of the file at path, an absolute,
	// clean path the settings name and outside every root, when this account
	// or root owns the file and neither its group nor others can write it.
	// A missing file matches fs.ErrNotExist. A file another account owns or
	// can write is refused: the error matches ErrNotOwned and says why and
	// the line that fixes it through Refusal() (why, fix string), and it
	// matches the ErrOwned error of its reason, where it has one. A relative
	// or empty path is refused, and an empty file is an empty slice.
	ReadOwned(path string) ([]byte, error)
}

// FileNames answers what a disk does with a file's name: the key a disk that
// folds names gives it, or a refusal for a name no system can hold. It reads
// no disk and takes no root, so every host and the fake answer alike. go-files'
// confine.Portable fills it, and nothing else does.
type FileNames interface {
	// Portable answers whether name, one part of a slash-separated path, means
	// one file, and the same one, on every system there is, and its key: the
	// form two names share when a folder that folds case or a canonical normal
	// form reads them as one. A path is asked a part at a time: "/" is refused
	// in a name. An empty name, ".", "..", a name no system takes (a character
	// Windows reserves, an end in a dot or a space, a device name, an 8.3
	// short name, a code point HFS+ ignores), and .git in any case are
	// refused: the error matches fs.ErrInvalid and says why in the host's
	// words, which no code matches on. It looks at nothing on disk and takes
	// no root, so every host and the fake answer alike.
	Portable(name string) (key string, err error)
}

// Trust is the approvals of this machine: the plugins a person said yes to,
// each by its bundle's digest as the lock names it, kept outside every
// repository, so a clone carries none. It holds the
// digest it is handed and never computes one. It fails closed: whenever an
// error is answered, the answer is no, and a record that cannot be read whole
// and trusted is never a yes.
type Trust interface {
	// Approved answers whether the bundle whose digest is d, "sha256:" and
	// sixty-four lower-case hexadecimal digits, was approved on this machine,
	// digest for digest: one whose module or grant changed has another digest,
	// and is not. No record, on a machine that never approved one, is false
	// and no error, as an empty record is: it is not a refusal.
	// A record another account owns or can write is refused as the owned read
	// refuses it, matching ErrNotOwned; a record kept in a folder a clone
	// carries matches ErrCarried; a record that is not whole JSON of its
	// form, or of a newer version, is refused. Each answers false with its
	// error, never true. A d not in the digest's form is an error. ctx ending
	// first is its own error.
	Approved(ctx context.Context, d model.Digest) (bool, error)
	// Approve records a, its digest, reference and grants' digest as they
	// were shown at the yes, and the time in seconds, UTC, by the Clock. It
	// waits for another writer of the record until ctx ends, so two at once
	// both hold, and replaces the record whole, so a write cut short leaves
	// the old one. Approving a digest again replaces its entry. An a whose
	// digests are not in the digest's form, or with an empty Ref, is refused
	// and writes nothing; so is a record Approved would refuse, which is
	// never written over, and one kept in a folder a clone carries (ErrCarried).
	Approve(ctx context.Context, a model.Approval) error
	// Revoke removes the approval of the bundle whose digest is d, in the
	// digest's text form Approved takes, waiting
	// and replacing as Approve does. A d that is not approved, and no record
	// at all, is not an error and writes nothing. A record Approved would
	// refuse is refused and stays as it is, and so does one kept in a folder a
	// clone carries (ErrCarried). ctx ending first is its own error.
	Revoke(ctx context.Context, d model.Digest) error
	// Prior answers the newest approval, by the time recorded, whose reference
	// names the plugin name (the text of Ref before its "@", or all of it), with
	// the grants it was shown with by id, sorted (model.Approval.Held, nil for
	// one recorded before they were kept). It reads no digest and says nothing of
	// whether the bundle in hand is approved: it is the earlier yes a later one
	// is compared with. No record, and none for name, match
	// fs.ErrNotExist, a first approval; a record the other methods refuse is
	// refused alike, matching ErrNotOwned, ErrCarried or the record error, and
	// answers no approval. ctx ending first is its own error.
	Prior(ctx context.Context, name string) (model.Approval, error)
}

// Steps runs what a plugin's plan asks run or read, and only what its
// manifest grants. The plugin runs nothing itself: every program
// a step names is a grant's, found by the grant's id as the host finds a
// tool, never by what the project says the tool is.
type Steps interface {
	// Bind answers the runner of one read's steps under s: its manifest, the
	// root the grants' programs are found for, the prepared scratch folder
	// and the level it reached, the engine's floor, and the note each step is
	// journalled by before it starts. Before it answers it finds the program
	// of the grant whose id is the manifest's tool and reads its version, so
	// the version is known before any plugin call. Asking a version starts
	// the program, so the ask is journalled by the Note first, as a
	// VersionAsk of round 0 and step 0, and so is the first find of any
	// other grant, at the step that names it; a note that fails starts
	// nothing and is the error Bind, or the step, ends in. A scope with a nil Note,
	// a root or scratch folder that is not absolute and clean, or a level
	// that is neither isolated nor sandboxed, or a scratch folder whose
	// deadline is below 1 s or whose memory limit is below 0 (one the Sandbox
	// port never prepares), is refused and matches ErrRefused, nothing looked
	// at. A manifest with no grant whose id is its
	// tool matches ErrGrant. A tool that is not on this machine at a
	// version its grant serves matches ErrStepNotReady; one that prints
	// another version than the tools file pins it at is refused, matching
	// ErrRefused. ctx ending first is its own error.
	Bind(ctx context.Context, s plugin.Scope) (StepRunner, error)
	// SweepCaches removes every folder the runner keeps a grant's caches in
	// from one step to the next (each an absolute folder below the tools
	// cache), and all each holds, a folder a program left read-only among
	// them, and answers how many folders it removed. A link is removed,
	// never followed, and nothing is removed but what stands at a name the
	// runner gives such a folder. The folder they are kept in is judged as a
	// step judges it, and a link at its name, or one it does not trust, is
	// refused before anything is removed. No tools cache, set or there, or
	// no folder in it, is 0 and no error; one that cannot be removed is an
	// error, and the ones before it stay removed. No lock keeps a step off
	// them, so a step that runs while it sweeps may lose its cache and fail.
	SweepCaches() (int, error)
	// CachesLeft answers how many folders SweepCaches would remove now (each
	// an absolute folder below the tools cache), by the same rule and
	// judgement, and writes nothing: the count a clear's plan shows. No
	// tools cache, set or there, is 0 and no error; a cache SweepCaches
	// would refuse is refused here the same way.
	CachesLeft() (int, error)
}

// Writer replaces files inside a root, appends to them and removes one. It
// is the only port that writes a file.
type Writer interface {
	// Replace makes the file at path, slash-separated and relative to root,
	// hold data, making its folder when it is missing. A reader sees the old
	// file or the new one, never half of either; the mode and owner of an old
	// file are kept. A nil data is an empty file. A path that leaves the
	// root is refused and matches ErrOutside; a root another account owns
	// is refused and matches ErrNotOwner; a failed write leaves the old
	// file whole.
	Replace(root model.Root, path string, data []byte) error
	// Append adds lines and one newline to the file at path, in one write,
	// ending a torn last line first. lines is one line, or several joined by a
	// newline, none of them empty when there are several, and no newline ends
	// it: a plan that has several lines to add adds them together, so a write
	// that is cut short leaves the lines it made or none of them to the
	// journal's roll-back. The file is made when it is missing. A nil line is
	// an empty line. A newline at either end, or two together, is an error and
	// writes nothing.
	Append(root model.Root, path string, lines []byte) error
	// Remove takes away the one regular file at path, slash-separated and
	// relative to root, so a roll-back undoes a file a run made. A file that
	// is not there is no error, so a roll-back can be run again. A folder, a
	// link or anything that is not a file is refused and stays; a link is
	// never followed. A path that leaves the root is refused and matches
	// ErrOutside. The folder that held the file stays.
	Remove(root model.Root, path string) error
}

// Lock keeps two runs that write from writing at once. A run takes the root
// lock first and the scratch lock second, never the reverse.
// Taking the root lock while this run holds a scratch lock is an error of its
// own, not ErrHeld: the order is the caller's to keep.
type Lock interface {
	// Take takes the lock which on root, without waiting past ctx, and
	// records the holder's start time in seconds, UTC. When another process
	// holds it, the error matches ErrHeld, names the start time, in seconds,
	// the holder recorded, or says it recorded none, and the hold is the zero value, which no
	// Release accepts. ctx ending first is its own error. A lock kept in a
	// folder another account owns is refused and matches ErrNotOwner. The
	// scratch lock is one for the machine (model.ScratchLock): held for any
	// root, a Take of it for any other matches ErrHeld. A lock kept in a
	// folder a clone carries is refused before anything is made or taken
	// there, and matches ErrCarried.
	Take(ctx context.Context, root model.Root, which model.Lock) (model.Hold, error)
	// Wait takes the lock as Take does, with the same checks and the same
	// order, and waits for another process that holds it until ctx ends
	// (gragas#1690). The hold and its start time are Take's, in seconds, UTC,
	// and a failed Wait answers the zero hold. When ctx ends first, the error
	// matches ErrHeld, names the holder's start time as Take's does, and
	// matches ctx's error too. A
	// lock this run already holds is never waited for: only its own Release
	// could free it, so Wait answers ErrHeld at once.
	Wait(ctx context.Context, root model.Root, which model.Lock) (model.Hold, error)
	// Release puts back the lock Take answered, the hold with its Since in seconds, so the next
	// Take of it succeeds.
	// A hold that is not held, or the zero model.Hold, is an error and
	// changes nothing; a lock is never released twice without an error.
	Release(h model.Hold) error
}

// Journal keeps the record of a run that writes: its intent before the first
// effect, its steps, the copies of the files it replaced and its close. It
// cannot be rebuilt: a closed record is kept for the age the settings file
// names, and the copies with it. A journal kept in a folder a clone carries is
// refused by every method before a record or a copy is read or written there,
// matching ErrCarried: a run planted there would otherwise be
// rolled back into the root.
type Journal interface {
	// Append writes one record to the journal of root, in one write, before
	// the caller makes the effect the record announces, and sets its Version,
	// At and Root. A record with an empty Run, a Run that is not 1 to 64 letters,
	// digits, dashes or underscores, an unknown Kind, or a Detail that is not one
	// JSON value is refused, and so is a write that could not be made: the
	// caller then makes no effect. A record of a run already in the journal
	// reads it first, so a cancelled ctx refuses it; a rollback
	// or a close that must be written after a cancel passes
	// context.WithoutCancel.
	Append(ctx context.Context, root model.Root, r model.Record) error
	// Keep stores old, the content of the file at path before a run replaces
	// it, beside the journal, under run, as bytes. A nil old is an empty copy.
	// A second copy for the same run and path is refused with an error that names
	// both, so a retry never stores the replaced bytes as the original; Kept
	// still answers the first. A copy that cannot be stored is an error, and then
	// the caller does not replace the file.
	Keep(root model.Root, run, path string, old []byte) error
	// Kept answers the bytes of the copy Keep stored for path under run, or an
	// error matching fs.ErrNotExist when there is none. An empty copy is an
	// empty slice, not an error.
	Kept(root model.Root, run, path string) ([]byte, error)
	// KeptAt answers where the copy Kept answers lies, as an absolute path
	// outside root that a person can copy the bytes from by hand, or an error
	// matching fs.ErrNotExist when there is none. It is shown to a person,
	// and decides nothing (arch.ports.19).
	KeptAt(root model.Root, run, path string) (string, error)
	// Records answers every record of the journal of root, the runs in the
	// order they began and each run's records in the order they were written,
	// and how many lines or records were torn and skipped. A root with no
	// journal is no records, 0 torn and no error. A record of a newer Version
	// than model.RecordVersion is an error that names both versions, a
	// model.NewerRecordError, and a journal of the older form release 1
	// wrote is one naming the line, a model.OlderJournalError. A cancelled
	// ctx is an error.
	Records(ctx context.Context, root model.Root) (records []model.Record, torn int, err error)
	// RecordsOf answers what Records answers for a root whose journal file
	// holds data, the bytes a caller read of it: the same records in the
	// same order, the same torn count and the same errors, the older form's
	// naming no path. It opens nothing. No data is no records,
	// 0 torn and no error. A journal whose state folder lies in a folder a
	// clone carries is refused, matching ErrCarried, whatever data holds and
	// with none, so a caller asks with none before it reads a journal file
	// there.
	RecordsOf(ctx context.Context, data []byte) (records []model.Record, torn int, err error)
	// Sweep removes every run of root whose close record is older, in seconds
	// by the Clock, than the age the settings file names: its records and
	// the copies Keep stored under it. A run with no close is open and is
	// never swept, whatever its age, since it may still need rolling back.
	// It answers how many runs it removed. A journal it cannot read whole is an error and removes
	// nothing, and so is a link at any component of a copy's path: no link
	// is followed. The caller holds the root lock. A cancelled ctx is an
	// error and removes nothing.
	Sweep(ctx context.Context, root model.Root) (int, error)
}

// History reads a repository's own history, read-only. A rev is a tag, a
// commit or a branch name; "" is the index.
type History interface {
	// Base answers the full object name of the merge base of revs a and b.
	// Histories with no common commit, or with more than one merge base, are
	// an error matching ErrNoBase; a rev that does not exist one matching
	// ErrNoRev.
	Base(ctx context.Context, root model.Root, a, b string) (string, error)
	// Files answers the files of the tree at rev, one entry per file, sorted
	// by path, none as an empty list. A symbolic link the tree tracks is an
	// entry with Link set; Blob never reads one. Each entry's Object is the
	// name git gives its content: a file that differs between two revs has
	// two, and one that does not has one. A rev that does not
	// exist is an error matching ErrNoRev.
	Files(ctx context.Context, root model.Root, rev string) ([]model.Entry, error)
	// WorkObjects answers, for each of paths in the order given, the Object
	// Files would give the file if the work tree's bytes at that path were
	// staged as they are: hashed with no filter, so no attribute, no
	// end-of-line conversion and no program the repository names runs, and
	// nothing is written. A path that is not a regular file of the work tree
	// reached through no link, one that is absent, a link or a folder, or
	// one whose name begins with a double quote, answers "", as does a path
	// that is not clean, relative and slash-separated. One read of the work
	// tree for the lot; no path is an empty list. A read that fails is an
	// error, and no name is answered with it.
	WorkObjects(ctx context.Context, root model.Root, paths []string) ([]string, error)
	// Blob answers the whole content of the file at path, relative to root,
	// as it is at rev, in bytes. A path the rev lacks matches fs.ErrNotExist,
	// a rev that does not exist ErrNoRev, and an empty file is an empty slice.
	Blob(ctx context.Context, root model.Root, rev, path string) ([]byte, error)
	// Blobs answers the whole content of each of paths at rev, in the order
	// given, as Blob would answer each one alone: one read of the repository
	// for the lot, not one for each file. The first path that
	// Blob would refuse is the answer's error, and nothing is returned with it;
	// no path is an empty list.
	Blobs(ctx context.Context, root model.Root, rev string, paths []string) ([][]byte, error)
	// Tags answers every tag of the repository, sorted by name; none is an
	// empty list, not an error. A repository that cannot be read is an error.
	Tags(ctx context.Context, root model.Root) ([]model.Tag, error)
	// Messages answers the message of each commit reachable from head and
	// not from base, through every parent of a merge, once each, sorted by
	// commit; none is an empty list. Each is whole, as the commit's object
	// stores it, byte for byte. A commit whose message holds a NUL, or whose
	// header names an encoding other than UTF-8, is an error naming it,
	// never a re-coded or shorter text.
	// base and head are each the full object name of a commit, of the
	// repository's own length; a ref name, a short name or anything else is
	// refused before anything is read. A range past what can be read whole
	// is an error, never a part of the list.
	Messages(ctx context.Context, root model.Root, base, head string) ([]model.Message, error)
	// Who answers the name and address, as text of one line each, the repository
	// commits as.
	// A repository with none configured is an error, never an empty identity.
	Who(ctx context.Context, root model.Root) (model.Identity, error)
	// Merging answers the full object names of the commits the merge in
	// progress in root's worktree brings in, in order as the repository's
	// own merge state records them; none, an empty list, when no merge is in
	// progress. A ref of any name is never read for it: a tag or a branch
	// named as the merge state is is no merge. A merge state that cannot be
	// read, or names what is not a commit, is an error.
	Merging(ctx context.Context, root model.Root) ([]string, error)
	// Commits answers the commits reachable from rev to and not from rev
	// from, each by its full object name with its parents', every commit
	// after its parents (in order); none is an empty list. A rev that does
	// not exist is an error matching
	// ErrNoRev, and a history that does not hold every parent, as a shallow
	// clone's, is an error.
	Commits(ctx context.Context, root model.Root, from, to string) ([]model.Commit, error)
}

// Remote reads releases from elsewhere and makes the two writes a release
// makes. It is a different subject from History, served by the same adapter
// package since one external program serves both.
type Remote interface {
	// Fetch brings the tag of the repository at from, a host, a path or a
	// mirror, into the releases cache and answers the root of what it
	// fetched, an absolute folder under a root. An empty tag or from is an
	// error. A tag that is not there, or a source that cannot be reached, is an
	// error; nothing already in the cache changes.
	Fetch(ctx context.Context, from, tag string) (model.Root, error)
	// Commit commits the named paths, relative to root and slash-separated,
	// with message, and answers the full object name of the new commit. No
	// path, or a path with no change, is an error and makes no commit; an
	// empty message is refused.
	Commit(ctx context.Context, root model.Root, paths []string, message string) (string, error)
	// Tag creates the annotated tag name on commit, holding message, one text
	// of lines. It is never signed: a release is pinned by its content hash
	// (release-pin-by-hash). An empty message is an error. A tag that exists
	// is an error and is left as it is.
	Tag(ctx context.Context, root model.Root, name, commit, message string) error
}

// Sandbox prepares a folder for a run at a named level.
type Sandbox interface {
	// Prepare makes a scratch folder holding the fixtures and the config
	// set of p, with a scrubbed environment and p's limits in seconds and
	// mebibytes, and answers the absolute folder and the level it reached,
	// which may be lower than asked. No fixtures is an empty folder. A level
	// the platform cannot reach matches ErrUnavailable and makes no folder.
	Prepare(ctx context.Context, p model.Prepare) (model.Scratch, error)
	// Release removes a scratch folder Prepare answered, an absolute folder, and every file in
	// it. A folder that is already gone is not an error; one that cannot be
	// removed is, and the zero model.Scratch is refused.
	Release(s model.Scratch) error
	// Sweep removes every scratch folder Prepare made that no run holds (each
	// an absolute folder), and every file in it, and answers how many folders it
	// removed. A run of any root holds its folders under the scratch lock,
	// which is one for the machine (model.ScratchLock, store.consistency.1),
	// so the caller holds that lock: no folder is then held, and a folder a
	// stopped run left is removed. Nothing but a folder Prepare could have made
	// is removed, and a link is never followed. No folder, or none made yet, is
	// 0 and no error; a folder that cannot be removed is an error, and the ones
	// before it stay removed.
	Sweep() (int, error)
	// Left answers how many folders Sweep would remove now (each an absolute
	// folder Prepare could have made), by the same rule, and removes nothing:
	// the count a clear's plan shows. It needs no lock, so a run may make or
	// remove one before a Sweep. No folder, or none made yet, is 0 and no
	// error; a base Sweep would refuse is refused here the same way.
	Left() (int, error)
}

// Schema checks a document against a JSON Schema. The schemas are bytes main
// hands the adapter once, at start.
type Schema interface {
	// Check validates doc, one JSON document, against the schema named name,
	// such as "rules.schema.json". It answers the problems sorted by place,
	// at most ten, and nil when the document is valid. A name that was not
	// compiled is one problem, never a panic; a doc that is not JSON is an
	// error.
	Check(name string, doc []byte) ([]model.Problem, error)
}

// Clock tells the time. It is the only port that does, and the only place a
// clock is read (ts.type-safety.11 holds the same on the TypeScript side).
type Clock interface {
	// Now answers the present instant in UTC. It never fails and never
	// answers the zero time.
	Now() time.Time
}

// Assets holds the files embedded in the program: the handbook, the schemas,
// the adapter data, the profiles, the fixture rulebook, the explorer.
type Assets interface {
	// Read answers the whole content of the embedded file at path,
	// slash-separated and relative to the embedded folder. A path that is
	// not embedded matches fs.ErrNotExist; an empty file is an empty slice.
	Read(path string) ([]byte, error)
	// List answers the paths of the embedded files below dir, "" being the
	// whole folder, sorted. A dir with none is an empty list, and a dir that
	// leaves the folder is an error.
	List(dir string) ([]string, error)
}

// Terminal is the person's terminal, and the guard's standard input.
type Terminal interface {
	// Width answers the terminal's width in columns, or 0 when output is not
	// a terminal. It never fails.
	Width() int
	// Colors answers the colour depth in bits, 0 when there is no colour. It
	// never fails.
	Colors() int
	// Ask writes question and reads one line of answer, without its
	// newline; an empty line is an empty answer, not an error. At the end of
	// input it answers an error matching io.EOF. ctx ending first is its own
	// error.
	Ask(ctx context.Context, question string) (string, error)
	// Say writes one line, text without a newline, with control sequences it
	// did not write stripped. An empty text is an empty line. A write that
	// fails is an error.
	Say(text string) error
	// Show draws screen, block by block, in the look of the terminal: colour
	// where the output is a terminal that paints, plain text where it is not,
	// in the columns Width gives. Text in a block that the program did not write
	// is shown with its control sequences removed. A screen that
	// model.NewScreen did not build, one with no block, is an error, and a
	// write that fails is an error.
	Show(screen model.Screen) error
	// Banner writes the wordmark and the version, or one plain line where there
	// is no room, no colour or no one to see it. A write that fails is an error.
	Banner(version string) error
	// Line reads one line of standard input, without its newline; an empty
	// line is an empty string, not an error. At the end of input it answers an
	// error matching io.EOF, and ctx ending first is its own error.
	Line(ctx context.Context) (string, error)
	// CanAsk answers whether a question could be answered here, in one line: a person at a
	// terminal. It answers false, never an error, where no one is there. Where
	// it is false nothing is asked, and the caller prints what it would have
	// printed before menus existed.
	CanAsk() bool
	// Pick offers choices, in order, under title and answers the Value of the one
	// picked, one line each. It never fails. flag is what answers the pick without a terminal, shown under the title
	// so a person learns the shorter way (cli.environment.14). picked is false
	// when nobody can answer, when choices is empty and when the person backed
	// out, which is an answer and says nothing.
	Pick(title string, choices []model.Choice, flag string) (value string, picked bool)
}

// Ports is every port gragas reaches the world through, filled once by
// cmd/gragas and handed down. A field tagged required is one every platform
// fills with a working adapter. A field tagged optional is one a platform
// may fill with an adapter whose every method answers ErrUnavailable: it is
// still filled, so no code tests a port for nil, and a port is never a stub
// that pretends to work (arch.ports.5, arch.ports.17).
type Ports struct {
	Tree     Tree     `port:"required"`
	Writer   Writer   `port:"required"`
	Lock     Lock     `port:"required"`
	Journal  Journal  `port:"required"`
	History  History  `port:"required"`
	Remote   Remote   `port:"required"`
	Sandbox  Sandbox  `port:"optional"`
	Schema   Schema   `port:"required"`
	Clock    Clock    `port:"required"`
	Assets   Assets   `port:"required"`
	Terminal Terminal `port:"required"`
	Owned    Owned    `port:"required"`
	// FileNames is the thirteenth port: its one
	// method takes no root, so it is not the Tree's.
	FileNames FileNames `port:"required"`
	// Trust is the fourteenth port: the approvals of
	// this machine, kept in the state folder and never in a root.
	Trust Trust `port:"required"`
	// Steps is the fifteenth port: the one place a
	// plugin's steps are run, under its manifest's grants.
	Steps Steps `port:"required"`
	// Bundles is the sixteenth port: the installed bundles of a root as they lie on disk,
	// which plugin list reads through the core.
	Bundles Bundles `port:"required"`
	// Plugins is the seventeenth port: the
	// installed plugins, declared in plugin.go, which host/plugin fills over
	// the Bundles above.
	Plugins Plugins `port:"required"`
}

// Bundles is the installed bundles of one root as they lie on disk: the lock's
// list, and each bundle's files and digests as read. host/plugin's opener is
// handed one at construction and makes every refusal on what it reads; host/fs
// fills it. Nothing here judges a bundle: a changed module, a manifest the
// engine refuses, a manifest whose name, version or wasm digest is not the
// folder's or the module's, and a digest that is not the lock's are all read as
// they are, and Open refuses them.
//
// What a reader refuses is what it cannot read as the bundle's own bytes, since
// the opener cannot tell those from the bytes: a link, symbolic or a second
// name of a file; a file that is not regular; a name a disk reads as another;
// anything outside root; and a module over the size bound its maker sets. Each
// is an error that names the file and matches no sentinel here.
type Bundles interface {
	// List is the plugins the lock of root names, in the order of their name
	// and then their version as bytes; a root with no lock, or a lock that
	// names none, is a nil list and no error. A lock it cannot read whole is
	// an error that names it.
	List(ctx context.Context, root string) ([]model.PluginRef, error)
	// Read is the bundle the lock of root names under ref: the lock's digest,
	// the bytes of plugin.yaml and plugin.wasm and of every other regular file
	// of its folder, each whole, the others by path, and the digest of the
	// bundle, pluginbundle.Digest of those very files, from the one read that
	// answers them, never from a second read of the folder. A ref the lock does not
	// name matches ErrNotInstalled, and so does one whose folder is absent; a
	// file it cannot read, or refuses as above, is an error that names it. A
	// cancelled or expired ctx ends it with the context's own error, which
	// matches no sentinel here.
	Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error)
}
