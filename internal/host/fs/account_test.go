package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust"

	testenv "github.com/birdtable/go-testkit"
	hostfs "github.com/birdtable/gragas/internal/host/fs"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// account is who a case takes this process to run as: as, in go-files' trust
// form, or this process's own account when real is set, and whether a folder
// owner owns is refused for it.
type account struct {
	as      string
	real    bool
	refused bool
}

// accounts are the cases of who this process runs as over a folder the
// account owner owns, in go-files' trust form: the owner itself, which
// writes; an account of no system's; root and SYSTEM and Administrators,
// which own no other account's folder here, each refused unless
// it is the owner; and this process's own account as go-files reads it,
// refused exactly when the folder is not its own. The forms of both systems
// are in every run, for mine compares the two strings and nothing else.
func accounts(t *testing.T, owner string) map[string]account {
	t.Helper()
	me, err := trust.Account()
	if err != nil || me == "" {
		t.Fatalf("trust.Account = %q, %v: this run's account cannot be read", me, err)
	}
	cases := map[string]account{
		"the owner":          {as: owner},
		"another account":    {as: "uid:4294967294-of-no-system", refused: true},
		"this run's account": {real: true, refused: me != owner},
		"root":               {as: "uid:0", refused: true},
		"SYSTEM":             {as: "S-1-5-18", refused: true},
		"the Administrators": {as: "S-1-5-32-544", refused: true},
	}
	for name, tc := range cases {
		if !tc.real && tc.as == owner { // a run as root, or by an admin on Windows
			tc.refused = false
			cases[name] = tc
		}
	}
	return cases
}

// says is what the refusal names the account of tc by: as, or this process's.
func (tc account) says(t *testing.T) string {
	t.Helper()
	if !tc.real {
		return tc.as
	}
	me, err := trust.Account()
	if err != nil {
		t.Fatal(err)
	}
	return me
}

// writer is a Writer of tr running as tc's account.
func (tc account) writer(tr *hostfs.Tree) hostfs.Writer {
	w := hostfs.Writer{Tree: tr}
	if tc.real {
		return w
	}
	return hostfs.WithAccount(w, tc.as)
}

// ownerOf is the account that owns the folder or file at path, as go-files'
// trust.OwnerOf reads it on the handle.
func ownerOf(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	owner, err := trust.OwnerOf(f)
	if err != nil {
		t.Fatalf("trust.OwnerOf(%s) = %v", path, err)
	}
	return owner
}

// read is the file at path.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// notOwner holds err to be the refusal of a folder another account owns,
// naming the folder, its owner's account and this run's.
func notOwner(t *testing.T, what string, err error, dir, owner, as string) {
	t.Helper()
	if !errors.Is(err, port.ErrNotOwner) {
		t.Fatalf("%s = %v, want port.ErrNotOwner", what, err)
	}
	for _, says := range []string{dir, "account " + owner, "account " + as} {
		if !strings.Contains(err.Error(), says) {
			t.Errorf("%s = %q, which does not say %q", what, err, says)
		}
	}
}

// TestAReplaceInARootAnotherAccountOwnsIsRefused: go-files writes by a path,
// which the root's owner could swap on the way, so Replace refuses a root
// another account owns, root's run and SYSTEM's included, before it writes
// anything, and says whose root it is. Risk: a run as root over another
// account's tree raced into writing outside it.
func TestAReplaceInARootAnotherAccountOwnsIsRefused(t *testing.T) {
	t.Parallel()
	refusedIn(t, "Replace", "sub/f", func(w hostfs.Writer, root model.Root, path string) error {
		return w.Replace(root, path, []byte("new\n"))
	})
}

// TestAnAppendInARootAnotherAccountOwnsIsRefused: Append refuses a root
// another account owns as Replace does, before it makes a folder or opens a
// file, and says whose root it is.
// Risk: a run as root raced into appending through a name another account's
// tree swapped.
func TestAnAppendInARootAnotherAccountOwnsIsRefused(t *testing.T) {
	t.Parallel()
	refusedIn(t, "Append", "sub/new/log", func(w hostfs.Writer, root model.Root, path string) error {
		return w.Append(root, path, []byte("line"))
	})
}

// TestARemoveInARootAnotherAccountOwnsIsRefused: Remove refuses a root
// another account owns as Replace does, before it removes anything, and says
// whose root it is. Risk: a run as root
// raced into removing a file another account's tree swapped onto the path.
func TestARemoveInARootAnotherAccountOwnsIsRefused(t *testing.T) {
	t.Parallel()
	refusedIn(t, "Remove", "sub/f", func(w hostfs.Writer, root model.Root, path string) error {
		return w.Remove(root, path)
	})
}

// refusedIn runs op on path in a root this process made, as each of
// accounts: the owner's op is done, and every other account's is refused as
// ErrNotOwner naming the root and both accounts, with the root as it was.
func refusedIn(t *testing.T, op, path string, do func(w hostfs.Writer, root model.Root, path string) error) {
	t.Helper()
	probe := t.TempDir()
	for name, tc := range accounts(t, ownerOf(t, probe)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "root")
			write(t, dir, map[string]string{"sub/f": "old\n"})
			owner := ownerOf(t, dir)
			tr := newTree(t)
			root, err := tr.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, dir)

			err = do(tc.writer(tr), root, path)

			if !tc.refused {
				if err != nil || equalTrees(before, snapshot(t, dir)) {
					t.Errorf("%s = %v, want the owner's root changed", op, err)
				}
				return
			}
			notOwner(t, op, err, dir, owner, tc.says(t))
			if after := snapshot(t, dir); !equalTrees(before, after) {
				t.Errorf("the root holds %q after the refusal, want it as it was", after)
			}
		})
	}
}

// TestALockInAStateFolderAnotherAccountOwnsIsRefused: go-files opens the lock
// file by its path, so a link the state folder's owner puts at its name
// after the check would have the lock cut short the file it names. Take
// refuses a state folder another account owns before it opens anything, and
// a link planted there is left as it is. Risk: a run as root made to truncate
// a file through the lock name.
func TestALockInAStateFolderAnotherAccountOwnsIsRefused(t *testing.T) {
	t.Parallel()
	for name, tc := range accounts(t, ownerOf(t, t.TempDir())) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, aim := t.TempDir(), filepath.Join(t.TempDir(), "precious")
			if err := os.WriteFile(aim, []byte("keep\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			owner := ownerOf(t, state)
			l := &hostfs.Lock{Dir: state, Clock: lockInstant()}
			if !tc.real {
				hostfs.LockAs(l, tc.as)
			}
			root := model.Root("/made/up/root")

			h, err := l.Take(t.Context(), root, model.RootLock)

			if !tc.refused {
				if err != nil {
					t.Fatalf("Take = %v, want the owner's lock taken", err)
				}
				if err := l.Release(h); err != nil {
					t.Fatal(err)
				}
				return
			}
			if h != (model.Hold{}) {
				t.Errorf("Take answered the hold %+v with its refusal", h)
			}
			notOwner(t, "Take", err, state, owner, tc.says(t))
			if entries, err := os.ReadDir(state); err != nil || len(entries) != 0 {
				t.Errorf("the state folder holds %v (%v) after the refusal, want nothing", entries, err)
			}
			// The same refusal with a link planted at the lock file's name:
			// the file it names is not touched.
			testenv.Link(t, aim, filepath.Join(state, lockName(root, model.RootLock)))
			if _, err := l.Take(t.Context(), root, model.RootLock); !errors.Is(err, port.ErrNotOwner) {
				t.Errorf("Take over a planted link = %v, want ErrNotOwner", err)
			}
			if got := read(t, aim); got != "keep\n" {
				t.Errorf("the file the planted link names holds %q, want it as it was", got)
			}
		})
	}
}

// TestTheOldWriterRefusesARepositoryAnotherAccountOwns: the old gen --apply
// writes by a path through go-files, which the repository's owner could swap
// on the way, so it refuses a repository another account owns before it
// makes a folder or writes a file, matching the old port's error and the new
// one's. Risk: root running gen --apply over another account's repository
// raced into writing outside it.
func TestTheOldWriterRefusesARepositoryAnotherAccountOwns(t *testing.T) {
	t.Parallel()
	for name, tc := range accounts(t, ownerOf(t, t.TempDir())) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "repo")
			write(t, dir, map[string]string{"rulebook/rules.json": "old\n"})
			owner := ownerOf(t, dir)
			before := snapshot(t, dir)
			w := hostfs.LegacyWriter{}
			if !tc.real {
				w = hostfs.LegacyWriterAs(tc.as)
			}

			err := w.Write(dir, "rulebook/base/x/README.md", []byte("# x\n"))

			if !tc.refused {
				if err != nil || read(t, filepath.Join(dir, "rulebook", "base", "x", "README.md")) != "# x\n" {
					t.Errorf("Write = %v, want the owner's file written with its folders", err)
				}
				return
			}
			if !errors.Is(err, legacy.ErrNotOwner) {
				t.Errorf("Write = %v, want legacy.ErrNotOwner as well", err)
			}
			notOwner(t, "Write", err, dir, owner, tc.says(t))
			if after := snapshot(t, dir); !equalTrees(before, after) {
				t.Errorf("the repository holds %q after the refusal, want it as it was, no folder made", after)
			}
		})
	}
}

// TestTheOldLockRefusesAStateFolderAnotherAccountOwns: the old gen --apply
// takes its lock through go-files by a path, so a link the folder's owner puts
// at the lock's name would have the lock cut short the file it names. Take
// refuses a folder another account owns before go-files opens anything, and a
// link planted there is left as it is. Risk: root running gen --apply made to
// truncate a file through the lock name.
func TestTheOldLockRefusesAStateFolderAnotherAccountOwns(t *testing.T) {
	t.Parallel()
	for name, tc := range accounts(t, ownerOf(t, t.TempDir())) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, aim := t.TempDir(), filepath.Join(t.TempDir(), "precious")
			if err := os.WriteFile(aim, []byte("keep\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			owner := ownerOf(t, state)
			path := filepath.Join(state, "gragas.lock")
			locks := hostfs.LegacyLocks{}
			if !tc.real {
				locks = hostfs.LegacyLocksAs(tc.as)
			}
			locks.Clock = lockInstant()

			l, err := locks.Take(path, "")

			if !tc.refused {
				if err != nil {
					t.Fatalf("Take = %v, want the owner's lock taken", err)
				}
				if err := l.Release(); err != nil {
					t.Fatal(err)
				}
				return
			}
			if l != nil {
				t.Errorf("Take answered a lock with its refusal")
			}
			if !errors.Is(err, legacy.ErrNotOwner) {
				t.Errorf("Take = %v, want legacy.ErrNotOwner as well", err)
			}
			notOwner(t, "Take", err, state, owner, tc.says(t))
			if errors.Is(err, legacy.ErrHeld) {
				t.Errorf("Take = %v, which says busy: the refusal is not a held lock", err)
			}
			if entries, err := os.ReadDir(state); err != nil || len(entries) != 0 {
				t.Errorf("the state folder holds %v (%v) after the refusal, want nothing", entries, err)
			}
			// The same refusal with a link planted at the lock's name: the
			// file it names is not touched.
			testenv.Link(t, aim, path)
			if _, err := locks.Take(path, ""); !errors.Is(err, legacy.ErrNotOwner) {
				t.Errorf("Take over a planted link = %v, want ErrNotOwner", err)
			}
			if got := read(t, aim); got != "keep\n" {
				t.Errorf("the file the planted link names holds %q, want it as it was", got)
			}
		})
	}
}
