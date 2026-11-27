package trust

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gotrust "github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"

	testenv "github.com/birdtable/go-testkit"
)

// TestARealSystemFileIsReadForItsOwnerAlone (B2's system-file case): a real
// file the system owns, which this account neither owns nor may write, is read
// through readOwned when its owner is the account asked for after this one, and
// refused as another account's when only this account is. The owner is the one
// trusttest.SystemFile reads, never taken to be uid 0: in a user namespace that
// does not map root, root's file reads as the overflow account's. A system file
// past MaxOwned is refused for its size, never for its owner. Risk: a settings
// file the system put in place is refused, or one of another account's is read
// (cli.config.12). Planted: the owner asked for second made this account, so
// the file is refused; the refusal's check taken out, so it is read.
func TestARealSystemFileIsReadForItsOwnerAlone(t *testing.T) {
	t.Parallel()
	me, err := gotrust.Account()
	if err != nil {
		t.Fatal(err)
	}
	path, owner := trusttest.SystemFile(t)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := readOwned(path, me, owner, nil)
	switch {
	case errors.Is(err, gotrust.ErrNotOwners):
		t.Errorf("%s, owned by %s: %v; want it read for its owner", path, owner, err)
	case len(want) > MaxOwned:
		if !errors.Is(err, gotrust.ErrTooLong) {
			t.Errorf("%s holds %d bytes, past %d: %v; want it refused for its size", path, len(want), MaxOwned, err)
		}
	case err != nil || string(data) != string(want):
		t.Errorf("%s, owned by %s: %d bytes, %v; want its %d bytes", path, owner, len(data), err, len(want))
	}
	data, err = readOwned(path, me, me, nil)
	if data != nil || !errors.Is(err, gotrust.ErrNotOwners) {
		t.Errorf("%s, owned by %s, asked for %s alone: %d bytes, %v; want it refused as another account's", path, owner, me, len(data), err)
	}
}

// TestTheSettingsAreReadOnlyFromThisAccountOrRoot: ReadOwned reads a file this
// account owns, then root's, and only once the first read is refused for its
// owner; each read checks the descriptor it reads. Risk: a settings file
// another account owns, or one its group or others can write, names the
// programs gragas runs (cli.config.12). Planted: the accounts readOwned is
// asked for, so another account's file and root's are made with no second
// account. Who may write a file is planted with trusttest.Loosen, and the
// refusal's words and fix are go-files' own for that file on this system.
func TestTheSettingsAreReadOnlyFromThisAccountOrRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	me, err := gotrust.Account()
	if err != nil {
		t.Fatal(err)
	}
	// Two accounts that are not this one, in the form "uid:N": go-files
	// compares an owner of either form on every system.
	other := func(n int) string { return "uid:" + strconv.Itoa(4000000000+n) }
	put := func(t *testing.T, name string, who ...trusttest.Who) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("version: 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, w := range who {
			trusttest.Loosen(t, p, w)
		}
		return p
	}
	refused := func(t *testing.T, err error, why, fix string) {
		t.Helper()
		o, ok := errors.AsType[*Owned](err)
		if !ok || !strings.Contains(o.Why, why) || o.Fix != fix {
			t.Errorf("%v; want refused as %q, fix %q", err, why, fix)
		}
	}
	t.Run("this account's file, which only it may write, is read", func(t *testing.T) {
		t.Parallel()
		if data, err := ReadOwned(put(t, "mine.yaml"), nil); err != nil || string(data) != "version: 1\n" {
			t.Errorf("%q, %v", data, err)
		}
	})
	t.Run("another account's file is refused, with the line that gives it to this one", func(t *testing.T) {
		t.Parallel()
		// The account asked for is a real one in this system's form, the
		// owner of a file the system owns, so go-files gives the line that
		// hands this file to it on every system: a uid's on Unix, a SID's
		// on Windows.
		_, owner := trusttest.SystemFile(t)
		p := put(t, "theirs.yaml")
		data, err := readOwned(p, owner, other(2), nil)
		fix := ownerFix(t, p, owner)
		if fix == "" {
			t.Fatalf("go-files gives no line that hands %s to %s", p, owner)
		}
		refused(t, err, "is owned by "+me+", not by this account or root", fix)
		if data != nil || !errors.Is(err, gotrust.ErrNotOwners) {
			t.Errorf("%q, %v; want nothing read, matching ErrNotOwners", data, err)
		}
	})
	t.Run("root's file is read once this account's read is refused for its owner", func(t *testing.T) {
		t.Parallel()
		if data, err := readOwned(put(t, "roots.yaml"), other(1), me, nil); err != nil || string(data) != "version: 1\n" {
			t.Errorf("%q, %v", data, err)
		}
	})
	t.Run("root's file its group can write is refused for that", func(t *testing.T) {
		t.Parallel()
		p := put(t, "roots-shared.yaml", trusttest.GroupWrites)
		_, err := readOwned(p, other(1), me, nil)
		refusedAsGoFiles(t, err, p, me)
	})
	for _, tc := range []struct {
		name string
		who  []trusttest.Who
	}{
		{"its group", []trusttest.Who{trusttest.GroupWrites}},
		{"others", []trusttest.Who{trusttest.OthersWrite}},
		{"its group and others", []trusttest.Who{trusttest.GroupWrites, trusttest.OthersWrite}},
	} {
		t.Run("this account's file "+tc.name+" can write is refused with its fix", func(t *testing.T) {
			t.Parallel()
			p := put(t, strings.ReplaceAll(tc.name, " ", "-")+".yaml", tc.who...)
			data, err := ReadOwned(p, nil)
			refusedAsGoFiles(t, err, p, me)
			if data != nil {
				t.Errorf("read %q from a refused file", data)
			}
		})
	}
	t.Run("a folder is refused", func(t *testing.T) {
		t.Parallel()
		_, err := ReadOwned(dir, nil)
		refused(t, err, "is a folder, not a regular file", "")
	})
	t.Run("a pipe is refused, and does not hold the run", func(t *testing.T) {
		t.Parallel()
		p, release := testenv.HoldRead(t)
		defer release()
		done := make(chan error, 1)
		go func() { _, err := ReadOwned(p, nil); done <- err }()
		select {
		case err := <-done:
			if !errors.Is(err, gotrust.ErrNotPlain) {
				t.Errorf("%v, want refused as not a regular file", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("reading a pipe with no writer held the run")
		}
	})
	t.Run("a link at the name is not followed, and the refusal says to put the file there", func(t *testing.T) {
		t.Parallel()
		p := filepath.Join(dir, "link.yaml")
		testenv.Link(t, put(t, "target.yaml"), p)
		data, err := ReadOwned(p, nil)
		if err == nil {
			t.Errorf("read %q through a link", data)
		}
		refused(t, err, "is a link", "replace the link with the file itself")
	})
	t.Run("a pipe's refusal gives no link's fix", func(t *testing.T) {
		t.Parallel()
		p, release := testenv.HoldRead(t)
		defer release()
		_, err := ReadOwned(p, nil)
		if o, ok := errors.AsType[*Owned](err); !ok || o.Fix != "" {
			t.Errorf("%v; want refused with no fix", err)
		}
	})
	t.Run("a SID owner has the line go-files gives for this system's path, and none for a path of another system", func(t *testing.T) {
		t.Parallel()
		// go-files reads an owner of either form on every system, so a SID
		// for another account is refused here as a Windows file of another's is.
		// Its fix is for a Windows path (TestEachFixGoFilesGivesIsTheLineItWroteBefore):
		// go-files gives the icacls line where the path is one, and no line
		// where it is a path of another system, and the refusal gives that.
		sid := "S-1-5-21-1-2-3-1001"
		p := put(t, "sid.yaml")
		_, err := readOwned(p, sid, rootAccount, nil)
		refused(t, err, "is owned by "+me+", not by this account or root; only an elevated shell can give it to this account", ownerFix(t, p, sid))
	})
	t.Run("a missing file matches fs.ErrNotExist and is no refusal", func(t *testing.T) {
		t.Parallel()
		_, err := ReadOwned(filepath.Join(dir, "none.yaml"), nil)
		if _, isOwned := errors.AsType[*Owned](err); !errors.Is(err, fs.ErrNotExist) || isOwned {
			t.Errorf("%v, want fs.ErrNotExist", err)
		}
	})
	t.Run("a file this account cannot read is an error, not a refusal", func(t *testing.T) {
		t.Parallel()
		p := put(t, "shut.yaml")
		trusttest.Deny(t, p) // skipped, saying so, where this account reads it still
		_, err := ReadOwned(p, nil)
		if _, isOwned := errors.AsType[*Owned](err); !errors.Is(err, fs.ErrPermission) || isOwned {
			t.Errorf("%v, want a permission error", err)
		}
	})
}

// ownerFix is the line that gives the file at p to owner, from go-files' own
// steps for it on this system (gotrust.Fix.Commands), or "" where go-files
// gives none, as for a path not in the form of the owner's system. Each word
// of each step is in the line, and the line is those steps as fixLine prints
// them, whose quoting TestEachFixGoFilesGivesIsTheLineItWroteBefore holds.
func ownerFix(t *testing.T, p, owner string) string {
	t.Helper()
	f := gotrust.Fix{Path: p, Owner: owner}
	steps, ok := f.Commands()
	if !ok {
		return ""
	}
	line := fixLine(f)
	for _, s := range steps {
		for _, w := range s.Argv {
			if !strings.Contains(line, w) {
				t.Fatalf("the line %q leaves out %q of go-files' step %q", line, w, s.Argv)
			}
		}
	}
	return line
}

// refusedAsGoFiles fails t unless err is ReadOwned's refusal of the file at
// p in go-files' words for it, with a fix line that holds every word of the
// commands go-files' own refusal of p, read as owner, gives on this system:
// the system's fix, and no one system's words. A refusal
// that go-files gives no command for fails it too.
func refusedAsGoFiles(t *testing.T, err error, p, owner string) {
	t.Helper()
	o, ok := errors.AsType[*Owned](err)
	if !ok {
		t.Fatalf("%v; want an *Owned refusal", err)
	}
	_, gerr := read(p, owner, nil)
	u, ok := errors.AsType[*gotrust.Unsafe](gerr)
	if !ok {
		t.Fatalf("go-files' read of %s = %v; want its refusal", p, gerr)
	}
	if o.Why == "" || !strings.Contains(u.Observed, o.Why) {
		t.Errorf("%v; want refused in go-files' words %q", err, u.Observed)
	}
	steps, ok := u.Fix.Commands()
	if !ok || len(steps) == 0 {
		t.Fatalf("go-files gives no command that fixes %s: %+v", p, u.Fix)
	}
	for _, s := range steps {
		for _, w := range s.Argv {
			if !strings.Contains(o.Fix, w) {
				t.Errorf("fix %q leaves out %q of go-files' command %q", o.Fix, w, s.Argv)
			}
		}
	}
}

// TestAnOwnedFileIsReadUpToItsLimit: a file of exactly MaxOwned bytes is
// read whole, and one byte more is refused as too long, with nothing read.
// Risk: a settings file grown without bound is read into memory whole.
//
// boundary: MaxOwned
func TestAnOwnedFileIsReadUpToItsLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, n := range []int{MaxOwned, MaxOwned + 1} {
		p := filepath.Join(dir, strconv.Itoa(n)+".yaml")
		if err := os.WriteFile(p, []byte(strings.Repeat("a", n)), 0o600); err != nil {
			t.Fatal(err)
		}
		data, err := ReadOwned(p, nil)
		switch {
		case n == MaxOwned && (err != nil || len(data) != n):
			t.Errorf("a file of MaxOwned bytes: %d read, %v; want it read whole", len(data), err)
		case n > MaxOwned && (data != nil || !errors.Is(err, gotrust.ErrTooLong)):
			t.Errorf("a file one byte over MaxOwned: %d read, %v; want nothing read, refused as too long", len(data), err)
		}
	}
}

// TestEachRefusalOfTheOwnedReadHasItsReason: an Owned
// is told by go-files' error alone, the narrower first: a mount entry is not
// called a mount, a mount or a volume that ignores ownership is not called
// another account's, though go-files' errors match the broader too, and a
// refusal of none is Other. The refusals a file of this account's cannot show
// here (another owner, a mount, a mount entry, a volume that ignores
// ownership, a file replaced at each try) are go-files' own errors, wrapped
// as go-files wraps them; host/fs's test plants the rest on real files.
// Planted: the mount entry asked after the mount (its row OnMount, red).
func TestEachRefusalOfTheOwnedReadHasItsReason(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "list")
	for _, c := range []struct {
		err  error
		want Reason
	}{
		{gotrust.ErrMountEntry, MountEntry},
		{gotrust.ErrMount, OnMount},
		{gotrust.ErrOwnersIgnored, OnMount},
		{gotrust.ErrNotOwners, ByOther},
		{gotrust.ErrOthersWrite, Writable},
		{gotrust.ErrNotPlain, NotRegular},
		{gotrust.ErrTooLong, TooLong},
		{gotrust.ErrReplaced, Other},
		{gotrust.ErrOverflow, Other},
	} {
		if got := reasonOf(path, fmt.Errorf("refused: %w", c.err)); got != c.want {
			t.Errorf("reasonOf(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}
