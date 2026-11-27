package trust

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gotrust "github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/go-files/filestest"
)

// TestTheTempFoldersRefusalsAreInTheWordsOfTheTempFolder: go-files's refusal of
// the walk, of a folder trusttest lets its group write, is an *Unsafe with the
// walk's words and its fix, a temp folder inside the work tree is an *Unsafe
// that says so with the fix of moving it, whether the last check or the walk's
// Who.Apart refused it, and any other error is the one given. Planted: the
// walk's refusal passed on as it is; the work tree's refusal dropped; the
// walk's Who.Apart refusal taken as any other refusal of the walk, with no fix.
func TestTheTempFoldersRefusalsAreInTheWordsOfTheTempFolder(t *testing.T) {
	t.Parallel()
	loose := filepath.Join(filestest.Private(t), "cache")
	if err := os.Mkdir(loose, 0o700); err != nil {
		t.Fatal(err)
	}
	trusttest.Loosen(t, loose, trusttest.GroupWrites)
	_, err := walk(loose, Folder, gotrust.Who{})
	walked, ok := errors.AsType[*gotrust.Unsafe](err)
	if !ok || walked.Fix == (gotrust.Fix{}) {
		t.Fatalf("the walk of a folder its group may write = %v: want go-files's *Unsafe with a fix", err)
	}
	var u *Unsafe
	if err := refusal(fmt.Errorf("temp: %w", walked)); !errors.As(err, &u) || u.Observed != walked.Observed || u.Fix == "" {
		t.Errorf("the walk's refusal = %v: want an *Unsafe in its words with a fix", err)
	}
	if err := refusal(fmt.Errorf("/w/tmp: %w", gotrust.ErrInside)); !errors.As(err, &u) || !strings.Contains(u.Observed, "work tree") || !strings.Contains(u.Fix, "outside the work tree") {
		t.Errorf("ErrInside = %v: want an *Unsafe that names the work tree", err)
	}
	apart := gotrust.Unsafe{Path: "/w", Observed: "/w is a folder it is kept apart from"}
	if err := refusal(fmt.Errorf("temp: %w", errors.Join(&apart, gotrust.ErrInside))); !errors.As(err, &u) || !strings.Contains(u.Observed, "work tree") || !strings.Contains(u.Fix, "outside the work tree") {
		t.Errorf("the walk's refusal by Who.Apart = %v: want an *Unsafe that names the work tree, with the fix of moving it", err)
	}
	other := errors.New("disk full")
	if err := refusal(other); !errors.Is(err, other) || errors.As(err, &u) {
		t.Errorf("another error = %v: want it as it is", err)
	}
}

// TestTheWorkTreesAreAbsoluteAndThere: an empty entry is
// left out, a folder that is there is kept as an absolute clean path, and one
// that is not there is an *Unsafe before anything is made. Planted: the missing
// folder passed on to go-files.
func TestTheWorkTreesAreAbsoluteAndThere(t *testing.T) {
	t.Parallel()
	here := t.TempDir()
	got, err := workTrees([]string{"", filepath.Join(here, "."), ""})
	if err != nil || len(got) != 1 || got[0] != filepath.Clean(here) {
		t.Errorf("workTrees(here) = %v, %v: want [%s]", got, err, here)
	}
	var u *Unsafe
	if _, err := workTrees([]string{filepath.Join(here, "gone")}); !errors.As(err, &u) || !strings.Contains(u.Observed, "cannot be checked") {
		t.Errorf("workTrees(missing) = %v: want an *Unsafe", err)
	}
}

// TestTheStableTempFolderIsTmpInTheTempFolderOfTheAccount:
// StableTemp answers a folder named tmp, at 0700, in go-files's folder for
// gragas, the same one on a second call, and one inside the work tree is
// refused. Planted: a path that moves between calls.
func TestTheStableTempFolderIsTmpInTheTempFolderOfTheAccount(t *testing.T) {
	t.Parallel()
	// The run's temp folder, which TestMain's testenv.Main points every temp
	// variable at; no other test of the package asks for go-files' folder.
	base := os.TempDir()
	first, err := StableTemp(nil)
	if err != nil {
		t.Fatalf("StableTemp(nil) in %s: %v", base, err)
	}
	if filepath.Base(first) != "tmp" || !strings.Contains(filepath.Base(filepath.Dir(first)), "gragas") {
		t.Errorf("StableTemp = %s: want a folder tmp in one named gragas… in the temp folder", first)
	}
	if second, err := StableTemp(nil); err != nil || second != first {
		t.Errorf("second call %q, %v, first %q: the folder moved", second, err, first)
	}
	var u *Unsafe
	if _, err := StableTemp([]string{filepath.Dir(filepath.Dir(first))}); !errors.As(err, &u) {
		t.Errorf("StableTemp inside the work tree = %v: want an *Unsafe", err)
	}
}

// TestSweepHomesRemovesTheHomesNoRunHolds: a Home whose run has
// ended is removed with its hold file, and one a run still holds stays, and so
// does what RunHome could not have made.
//
// Risk: a sweep that removes a Home a live step is writing in, or one that
// leaves for good a Home a step could not remove.
func TestSweepHomesRemovesTheHomesNoRunHolds(t *testing.T) {
	t.Parallel() // a sweep leaves every Home a run holds, so it cannot touch another test's
	held, removeHeld, err := RunHome(nil)
	if err != nil {
		t.Fatal(err)
	}
	left, removeLeft, err := RunHome(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = removeHeld() })
	// A run whose Home holds a folder its program left read-only and that
	// no step removed: the sweep removes it whole.
	stuck := filepath.Join(left, "stuck")
	if err := os.Mkdir(stuck, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stuck, "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stuck, 0o555); err != nil {
		t.Fatal(err)
	}
	_ = removeLeft
	old := time.Unix(1, 0) // long past, so older than any sweep age
	if err := os.Chtimes(left, old, old); err != nil {
		t.Fatal(err)
	}
	n, err := SweepHomes(nil, time.Hour)
	if err != nil {
		t.Fatalf("SweepHomes = %v", err)
	}
	if n < 1 {
		t.Errorf("SweepHomes = %d, want the left Home counted", n)
	}
	if _, err := os.Stat(left); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the left Home %s is there after a sweep: Stat = %v", left, err)
	}
	if _, err := os.Stat(held); err != nil {
		t.Errorf("the held Home %s was swept: Stat = %v", held, err)
	}
}

// TestAFreshFolderIsNeverSwept (gragas#2264): SweepHomes takes an old Home and
// leaves an older FreshFolder beside it, and a prefix that starts as a Home's is
// refused, so no FreshFolder is ever one SweepHomes can take.
//
// Risk: a sweep by any gragas step on the machine that removes a pre-push
// worktree mid-run, because its folder looks like a Home that no run holds.
func TestAFreshFolderIsNeverSwept(t *testing.T) {
	t.Parallel() // a sweep leaves every folder that is not an old Home, so it cannot touch another test's
	for _, prefix := range []string{"run-", "run-gragas-"} {
		if dir, _, err := FreshFolder(nil, prefix); err == nil {
			t.Errorf("FreshFolder(%q) = %s, want a refusal: SweepHomes would take it", prefix, dir)
		}
	}
	home, removeHome, err := RunHome(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = removeHome() })
	kept, removeKept, err := FreshFolder(nil, "gragas-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := removeKept(); err != nil {
			t.Errorf("the removal of the fresh folder = %v", err)
		}
	})
	if got := filepath.Base(kept); !strings.HasPrefix(got, "gragas-test-") {
		t.Errorf("FreshFolder = %s, want gragas-test-*", kept)
	}
	// Never swept by place: the folder SweepHomes reads, the Homes' folder,
	// neither holds a FreshFolder nor lies inside the folder that does.
	homes, work := filepath.Dir(home), filepath.Dir(kept)
	if within(homes, kept) || within(work, homes) {
		t.Errorf("FreshFolder = %s, want it in a folder of its own, not under the Homes' folder %s or holding it", kept, homes)
	}
	// The trust walk, not a mode read: no name on the way to the
	// folder, the folder included, can be written by another account.
	if _, err := Walk(temp(), kept, Who{NoLinks: true}); err != nil {
		t.Fatalf("the fresh folder %s is not one only this account can write: %v", kept, err)
	}
	old := time.Unix(1, 0) // long past, so older than any sweep age
	for _, dir := range []string{home, kept} {
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SweepHomes(nil, time.Hour); err != nil {
		t.Fatalf("SweepHomes = %v", err)
	}
	if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the old Home %s is there after a sweep: Stat = %v", home, err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the fresh folder %s was swept: Stat = %v", kept, err)
	}
}

// within reports whether path is dir or lies inside it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

// TestAWorkTreeInATreeFolderLeavesTheTempFolderTrusted (gragas#2264): with
// apart a work tree made inside a TreeFolder, as the pre-push hook makes its
// worktree, StableTemp, RunHome, SweepHomes and FreshFolder all answer without
// a refusal.
//
// Risk: the hook's worktree made in the folder of the Homes or of the
// FreshFolders, so that folder holds it, and every Home, sweep or FreshFolder
// a member makes, as boards does, is refused for it: a push of any ref but the
// clean HEAD red.
func TestAWorkTreeInATreeFolderLeavesTheTempFolderTrusted(t *testing.T) {
	t.Parallel()
	dir, remove, err := TreeFolder(nil, "gragas-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = remove() })
	tree := filepath.Join(dir, "tree")
	if err := os.Mkdir(tree, 0o700); err != nil {
		t.Fatal(err)
	}
	apart := []string{tree}
	if got, err := StableTemp(apart); err != nil {
		t.Errorf("StableTemp(a tree in a TreeFolder) = %q, %v: want the stable tmp", got, err)
	}
	home, removeHome, err := RunHome(apart)
	if err != nil {
		t.Errorf("RunHome(a tree in a TreeFolder) = %v: want a Home", err)
	} else {
		t.Cleanup(func() { _ = removeHome() })
		if within(home, tree) || within(tree, home) {
			t.Errorf("RunHome = %s, kept apart from %s, and is not", home, tree)
		}
	}
	if _, err := SweepHomes(apart, time.Hour); err != nil {
		t.Errorf("SweepHomes(a tree in a TreeFolder) = %v: want a sweep", err)
	}
	fresh, removeFresh, err := FreshFolder(apart, "gragas-test-")
	if err != nil {
		t.Errorf("FreshFolder(a tree in a TreeFolder) = %v: want a folder", err)
	} else {
		t.Cleanup(func() { _ = removeFresh() })
		if within(fresh, tree) || within(tree, fresh) {
			t.Errorf("FreshFolder = %s, kept apart from %s, and is not", fresh, tree)
		}
	}
	if _, _, err := TreeFolder(nil, homePrefix+"x"); err == nil {
		t.Errorf("TreeFolder(prefix %q) = nil: want a refusal, as a Home's prefix", homePrefix+"x")
	}
}

// TestAFreshFolderIsItsCallersAloneAndGoesWithItsRemoval (gragas#2264): two
// calls make two folders; the removal one answers takes its folder and all it
// holds and leaves the other; a temp folder inside a folder of apart is an
// *Unsafe and nothing is made; and a prefix that would name a folder outside
// the account's temp folder is refused before anything is made.
//
// Risk: a pre-push worktree's removal that takes a scan's folder beside it, a
// scratch folder made inside the tree it checks, where a head can commit a
// file the tools read, or a prefix that leaves the temp folder.
func TestAFreshFolderIsItsCallersAloneAndGoesWithItsRemoval(t *testing.T) {
	t.Parallel()
	one, removeOne, err := FreshFolder(nil, "gragas-test-")
	if err != nil {
		t.Fatal(err)
	}
	other, removeOther, err := FreshFolder(nil, "gragas-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = removeOther() })
	if one == other {
		t.Fatalf("two calls answered the one folder %s", one)
	}
	if err := os.WriteFile(filepath.Join(one, "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(one, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeOne(); err != nil {
		t.Fatalf("the removal of %s = %v", one, err)
	}
	if _, err := os.Stat(one); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the removed folder %s is there: Stat = %v", one, err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("the removal of %s took %s beside it: Stat = %v", one, other, err)
	}
	var u *Unsafe
	if dir, _, err := FreshFolder([]string{filepath.Dir(filepath.Dir(other))}, "gragas-test-"); !errors.As(err, &u) {
		t.Errorf("FreshFolder kept apart from the folder holding the temp folder = %s, %v: want an *Unsafe", dir, err)
	}
	for _, prefix := range []string{"../gragas-test-", "a/b-"} {
		if dir, _, err := FreshFolder(nil, prefix); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("FreshFolder(%q) = %s, %v: want a refusal matching fs.ErrInvalid", prefix, dir, err)
		}
	}
}

// TestARunHomeWithAReadOnlyFolderIsRemovedWhole: the Home's own
// removal, at the end of a step, empties a folder its program left read-only
// (0555) with a file in it, as the go command leaves a module cache, and
// removes the Home itself.
//
// Risk: a Home left behind for good by a step that ran a tool that wrote
// read-only folders, one a sweep would have to find.
func TestARunHomeWithAReadOnlyFolderIsRemovedWhole(t *testing.T) {
	t.Parallel()
	home, remove, err := RunHome(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(home, "mod", "deep"), 0o755) })
	deep := filepath.Join(home, "mod", "deep")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{deep, filepath.Join(home, "mod")} {
		if err := os.Chmod(d, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	if err := remove(); err != nil {
		t.Fatalf("the removal of a Home with read-only folders in it = %v", err)
	}
	if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the Home %s is there after its removal: Stat = %v", home, err)
	}
}

// TestARunHomeRemovalLeavesWhatALinkInItPointsAt: a program
// links a read-only folder outside its Home into the Home. The removal takes
// the link and nothing it points at: the folder keeps its file and its mode.
//
// Risk: the write bit given to a Home's own folders reaches a folder outside
// the Home through a link, or its content is removed.
func TestARunHomeRemovalLeavesWhatALinkInItPointsAt(t *testing.T) {
	t.Parallel()
	home, remove, err := RunHome(nil)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(outside, "keep")
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outside, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(outside, 0o755) })
	before, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, outside, filepath.Join(home, "link"))
	if err := remove(); err != nil {
		t.Fatalf("the removal = %v", err)
	}
	if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the Home %s is there after its removal: Stat = %v", home, err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("what the link points at lost its file: Stat = %v", err)
	}
	if after, err := os.Stat(outside); err != nil || after.Mode() != before.Mode() {
		t.Errorf("what the link points at = %v, %v, want the mode it had, %v", after, err, before.Mode())
	}
}
