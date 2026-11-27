package fs

import (
	"io/fs"
	"os"
	"strings"
)

// WithAccount is w taking this process to run as the account me, in
// go-files' trust form ("uid:N" or a SID).
func WithAccount(w Writer, me string) Writer {
	w.as = func() string { return me }
	return w
}

// ErrRecord is the refusal of an approvals record that is not whole JSON of
// its form, and ErrNoRoots that of an Approvals that names no root.
var (
	ErrRecord  = errRecord
	ErrNoRoots = errNoRoots
)

// Carried is carried, the one comparison of a state folder with the folders a
// clone carries.
func Carried(dir string, roots []string) error { return carried(dir, roots) }

// Where and Make are an Approve's check of the state folder and its make of
// it, so a test changes the folder between the two.
func (a *Approvals) Where() ([]string, error) { return a.where() }

// Make is make, the state folder held apart from apart.
func (a *Approvals) Make(apart []string) error { return a.make(apart) }

// LockAs makes l take this process to run as the account me.
func LockAs(l *Lock, me string) { l.as = func() string { return me } }

// LockTaking makes l call taking with the lock file's path after its name is
// checked and before go-files opens it.
func LockTaking(l *Lock, taking func(path string)) { l.taking = taking }

// LockJudged makes l call judged after its state folder is judged against
// the roots and before it is made.
func LockJudged(l *Lock, judged func()) { l.judged = judged }

// JournalJudged is j calling judged after its state folder is judged against
// the roots and before it is made or opened by Dir.
func JournalJudged(j Journal, judged func()) Journal {
	j.judged = judged
	return j
}

// JournalHold is how long an Append waits for another process to let go of
// the journal.
const JournalHold = journalHold

// LegacyWriterAs is the old gen --apply's writer, taking this process to run
// as the account me.
func LegacyWriterAs(me string) LegacyWriter { return LegacyWriter{as: func() string { return me }} }

// LegacyLocksAs is the old gen --apply's lock, taking this process to run as
// the account me.
func LegacyLocksAs(me string) LegacyLocks { return LegacyLocks{as: func() string { return me }} }

// Confine is local, go-files' confine.Local with its refusal matching
// port.ErrOutside, for the fuzz target.
func Confine(p string) (string, error) { return local(p) }

// Same is same, so a test sees a file swapped between the walk and the open
// refused without racing one.
func Same(op, path string, f *os.File, walked fs.FileInfo, made bool) error {
	return same(op, path, f, walked, made)
}

// WithOpening makes t call opening with the path between the walk and the
// open of each Read and Append, so a test swaps the last component there.
func WithOpening(t *Tree, opening func(path string)) {
	t.opening = opening
}

// Outside is whether p, absolute and clean, is outside base, every link on
// it followed.
func Outside(base, p string) bool { return outside(base, p) }

// Noted is the second the note of the lock file name in dir says for this
// process, as Take reads it back once it has written it.
func Noted(dir, name string) (since int64, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	return noted(root, holderName(name))
}

// ReplaceRefused is Replace's words for go-files' refusal err of path, or
// nil when it names none.
func ReplaceRefused(path string, err error) error { return replaceRefused(path, err) }

// ReplaceRefusal is the go-files sentinel Replace names in words holding
// word, so a test matches it without importing go-files/write, which only
// the two writers do (arch.layers.2).
func ReplaceRefusal(word string) (error, bool) {
	for _, r := range replaceRefusals() {
		if strings.Contains(r.why, word) {
			return r.is, true
		}
	}
	return nil, false
}
