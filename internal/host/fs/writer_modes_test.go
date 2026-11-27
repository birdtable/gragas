package fs_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"

	"github.com/birdtable/gragas/internal/port"
)

// The modes below are read through go-files' trusttest.Perm, which holds a
// path to the mode this system keeps for a mode os.Chmod is given: the mode
// itself on Unix, the read-only attribute's reading of it on Windows, and a
// private one private to this account by its access list as well.

// TestAReplaceKeepsTheModeAndOwnerOfTheFileItReplaces (cli.commands.23): the
// old file's mode and owner, as trust.OwnerOf reads it, are the new
// file's. Where this account does not own the old file, as on Windows under
// an elevated token that makes files the Administrators', go-files keeps no
// owner but the writer's, so the replace is refused with go-files'
// write.ErrNoOwner, names why, and leaves the file as it was. That a group is
// kept is go-files' KeepOwner's, which its own tests hold.
func TestAReplaceKeepsTheModeAndOwnerOfTheFileItReplaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"f": "old"})
	p := filepath.Join(dir, "f")
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	owner := ownerOf(t, p)
	me, err := trust.Account()
	if err != nil || me == "" {
		t.Fatalf("trust.Account = %q, %v", me, err)
	}
	tr, w, root := opened(t, dir)

	err = w.Replace(root, "f", []byte("new"))

	if owner != me {
		if !errors.Is(err, errors.ErrUnsupported) || !strings.Contains(err.Error(), "another account's") {
			t.Errorf("Replace over a file %s owns, run as %s = %v, want go-files' ErrNoOwner (errors.ErrUnsupported), said", owner, me, err)
		}
		holdsFile(t, tr, root, "f", "old")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if by := ownerOf(t, p); by != owner {
		t.Errorf("the new file's owner is %s; want %s", by, owner)
	}
	trusttest.Perm(t, p, 0o640)
}

// TestTheStoreMakesFilesAndFoldersShared (store.seams.3): a new file, made by
// Replace or by Append, and its new folder are shared, as this system keeps a
// file given 0644 and a folder given 0755. That they hold under a umask that
// would narrow them is go-files' write.Shared's and confine.MkdirShared's,
// which their own tests hold, and Append's own Chmod.
func TestTheStoreMakesFilesAndFoldersShared(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, w, root := opened(t, dir)
	if err := w.Replace(root, "a/f", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(root, "b/log", []byte("x")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a", "b"} {
		trusttest.Perm(t, filepath.Join(dir, p), 0o755)
	}
	for _, p := range []string{"a/f", "b/log"} {
		trusttest.Perm(t, filepath.Join(dir, p), 0o644)
	}
}

// TestTheStoreTellsMissingAndUnreadableApart (store.seams.3): a missing file
// matches fs.ErrNotExist, and a folder neither that, nor fs.ErrPermission,
// nor ErrOutside, and each says which it is. A file this account may not
// read is not planted here: a mode of 0 is no denial on Windows, nor to
// root.
func TestTheStoreTellsMissingAndUnreadableApart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"folder/a": "a"})
	tr, _, root := opened(t, dir)
	_, missing := tr.Read(root, "none")
	_, unreadable := tr.Read(root, "folder")
	if !errors.Is(missing, fs.ErrNotExist) || errors.Is(missing, fs.ErrPermission) || !strings.Contains(missing.Error(), "missing") {
		t.Errorf("a missing file: %v", missing)
	}
	if unreadable == nil || errors.Is(unreadable, fs.ErrNotExist) || errors.Is(unreadable, fs.ErrPermission) ||
		errors.Is(unreadable, port.ErrOutside) || !strings.Contains(unreadable.Error(), "cannot be read") {
		t.Errorf("a folder read as a file: %v", unreadable)
	}
}

// modeOf is the mode the system reports for path, its type bits with it, so
// a test compares a file's mode before and after a write with no number of
// its own.
func modeOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

// TestAnAppendKeepsTheModeOfAFileItDidNotMake: an append to a file that is
// there, given 0600, leaves its mode as it was; only a file the append's own
// open made is shared, which the control, a new file, shows.
func TestAnAppendKeepsTheModeOfAFileItDidNotMake(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"private.jsonl": "{}\n"})
	p := filepath.Join(dir, "private.jsonl")
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	before := modeOf(t, p)
	_, w, root := opened(t, dir)
	for _, f := range []string{"private.jsonl", "new.jsonl"} {
		if err := w.Append(root, f, []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	if got := modeOf(t, p); got != before {
		t.Errorf("after an append private.jsonl is %v, want %v as it was", got, before)
	}
	trusttest.Perm(t, filepath.Join(dir, "new.jsonl"), 0o644)
}

// TestAnAppendRacingAnotherMakerNeverWidensTheFileTheOtherMade: over 2,000
// rounds another goroutine makes the file at 0600 with O_CREATE|O_EXCL while
// Append runs. Whenever the other side made it, the file keeps the mode it
// was made with after the append, whose line is in it; whenever the append
// made it, it is shared. Each side must win some rounds, or the race was not
// run.
func TestAnAppendRacingAnotherMakerNeverWidensTheFileTheOtherMade(t *testing.T) {
	t.Parallel()
	const workers, each = 4, 500
	const rounds = workers * each
	var theirs, ours atomic.Int64
	t.Run("rounds", func(t *testing.T) {
		for n := range workers {
			t.Run(fmt.Sprintf("worker %d", n), func(t *testing.T) {
				t.Parallel()
				racingRounds(t, n*each, each, &theirs, &ours)
			})
		}
	})
	if theirs.Load() == 0 || ours.Load() == 0 {
		t.Errorf("over %d rounds the other maker won %d and the append %d; each must win some, or the race was not run", rounds, theirs.Load(), ours.Load())
	}
	t.Logf("the other maker won %d rounds, the append %d", theirs.Load(), ours.Load())
}

// racingRounds runs the rounds from first, each over a file named by its
// number in a folder of its own, and counts who made each file.
func racingRounds(t *testing.T, first, count int, theirs, ours *atomic.Int64) {
	t.Helper()
	dir := t.TempDir()
	_, w, root := opened(t, dir)
	for i := first; i < first+count; i++ {
		name := fmt.Sprintf("r%d.jsonl", i)
		p := filepath.Join(dir, name)
		start := make(chan struct{})
		// made is the mode the other maker's file had when it was made, or
		// nothing when the append made it first.
		made := make(chan *fs.FileMode)
		go func() {
			<-start
			f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				if !errors.Is(err, fs.ErrExist) {
					t.Errorf("round %d: the other maker's open: %v", i, err)
				}
				made <- nil
				return
			}
			info, err := f.Stat()
			err = errors.Join(err, f.Close())
			if err != nil {
				t.Errorf("round %d: the other maker's file: %v", i, err)
				made <- nil
				return
			}
			m := info.Mode()
			made <- &m
		}()
		close(start)
		appendErr := w.Append(root, name, []byte("{}"))
		other := <-made
		if appendErr != nil {
			t.Fatalf("round %d: %v", i, appendErr)
		}
		if other != nil {
			theirs.Add(1)
			if got := modeOf(t, p); got != *other {
				t.Errorf("round %d: the other maker made it at %v; after the append it is %v", i, *other, got)
			}
		} else {
			ours.Add(1)
			trusttest.Perm(t, p, 0o644)
		}
		if b, err := os.ReadFile(p); err != nil || string(b) != "{}\n" {
			t.Errorf("round %d: the file holds %q, %v, want the appended line", i, b, err)
		}
	}
}
