package trust

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/go-files/filestest"
)

// theFolder is the subject these rows make and judge.
func theFolder() Subject {
	return Subject{Name: "the folder", File: "the folder", Last: Folder, Bare: true}
}

// ours is a folder of this account's that the walk passes, its links
// resolved, so a row's own names are all it plants.
func ours(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(filestest.Private(t))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// closeRoot closes made when there is one.
func closeRoot(t *testing.T, made *os.Root) {
	t.Helper()
	if made != nil {
		if err := made.Close(); err != nil {
			t.Error(err)
		}
	}
}

// TestRunsMakingOneFolderAtOnceAreAllAccepted: runs that each
// make and judge one missing folder while each creates a file in it, as two
// first runs that each take a lock in a state folder not there yet do, are
// all accepted. Risk: a run whose make won and whose check came after another
// run's file was in the folder is refused, its lock or approval not taken.
// go-files's judged make sends such a run back to its judge, which takes the
// folder as one that was there. Planted: a Private that refuses the folder
// another run made (a run is refused, red here).
func TestRunsMakingOneFolderAtOnceAreAllAccepted(t *testing.T) {
	t.Parallel()
	const rounds, runs = 40, 8
	base := ours(t)
	for round := range rounds {
		dir := filepath.Join(base, "r"+strconv.Itoa(round), "state")
		start := make(chan struct{})
		errs := make([]error, runs)
		var wg sync.WaitGroup
		for n := range runs {
			wg.Go(func() {
				<-start
				made, err := Private(dir, theFolder(), Who{})
				if err != nil {
					errs[n] = err
					return
				}
				f, err := made.Create("run-" + strconv.Itoa(n))
				errs[n] = errors.Join(err, closed(f), made.Close())
			})
		}
		close(start)
		wg.Wait()
		for n, err := range errs {
			if err != nil {
				t.Fatalf("round %d, run %d of %d making %s at once = %v; want every run accepted", round, n, runs, dir, err)
			}
		}
	}
}

// closed closes f when there is one.
func closed(f *os.File) error {
	if f == nil {
		return nil
	}
	return f.Close()
}

// TestAMissingFolderIsMadePrivateAndAFolderThereKeepsItsMode: a folder that
// is missing, and each missing folder above it, is made private to this
// account whatever the umask; a folder that is there is opened as it is, never
// changed, whatever its mode. Planted: a make at 0755 (red on the made
// folders), and a make that chmods the folder that was there (red on its
// mode).
func TestAMissingFolderIsMadePrivateAndAFolderThereKeepsItsMode(t *testing.T) {
	t.Parallel()
	base := ours(t)
	dir := filepath.Join(base, "a", "b", "state")
	own, err := os.Stat(base)
	if err != nil {
		t.Fatal(err)
	}
	made, err := Private(dir, theFolder(), Who{})
	if err != nil {
		t.Fatalf("Private over missing folders = %v, want it made", err)
	}
	closeRoot(t, made)
	for _, d := range []string{filepath.Join(base, "a"), filepath.Join(base, "a", "b"), dir} {
		if fi, err := os.Stat(d); err != nil || fi.Mode() != own.Mode() {
			t.Errorf("made %s = %v, %v; want a folder of the mode a test's private folder has, %v", d, fi, err, own.Mode())
		}
	}
	there := filepath.Join(base, "there")
	if err := os.Mkdir(there, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(there)
	if err != nil {
		t.Fatal(err)
	}
	made, err = Private(there, theFolder(), Who{})
	if err != nil {
		t.Fatalf("Private over a folder that is there = %v, want it opened", err)
	}
	closeRoot(t, made)
	if after, err := os.Stat(there); err != nil || after.Mode() != before.Mode() {
		t.Errorf("the folder that was there, mode %v then %v (%v); want it unchanged", before.Mode(), after.Mode(), err)
	}
}

// TestAFolderThatIsThereIsJudgedWhole: a dir that is there, as
// one another run made, is judged whole before it is opened, so each refusal
// of a folder Private did not make stands. Risk: taking dir on its being there
// alone opens whatever another account put at the name. Each refusing row is
// red with the folder opened unjudged.
//
// A private folder of this same account (the control) is accepted. A test
// cannot give a folder to another account without root, so the row of a
// folder this account does not own is the walk trusting root alone
// (RootOnly), to which this account's folder is another's.
func TestAFolderThatIsThereIsJudgedWhole(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		who  Who
		// plant makes dir below base, and answers it.
		plant func(t *testing.T, base string) string
		ok    bool
	}{
		{"a link, in a folder others may write", Who{}, func(t *testing.T, base string) string {
			t.Helper()
			open := filepath.Join(base, "open")
			if err := os.Mkdir(open, 0o700); err != nil {
				t.Fatal(err)
			}
			testenv.Link(t, ours(t), filepath.Join(open, "state"))
			trusttest.Loosen(t, open, trusttest.OthersWrite)
			return filepath.Join(open, "state")
		}, false},
		{"a folder its group may write", Who{}, func(t *testing.T, base string) string {
			t.Helper()
			dir := filepath.Join(base, "state")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			trusttest.Loosen(t, dir, trusttest.GroupWrites)
			return dir
		}, false},
		{"a folder others may write", Who{}, func(t *testing.T, base string) string {
			t.Helper()
			dir := filepath.Join(base, "state")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			trusttest.Loosen(t, dir, trusttest.OthersWrite)
			return dir
		}, false},
		{"a folder of an account the walk does not trust", Who{RootOnly: true}, func(t *testing.T, base string) string {
			t.Helper()
			dir := filepath.Join(base, "state")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			return dir
		}, false},
		{"the control, a private folder of this account", Who{}, func(t *testing.T, base string) string {
			t.Helper()
			dir := filepath.Join(base, "state")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			return dir
		}, true},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			dir := tc.plant(t, ours(t))
			made, err := Private(dir, theFolder(), tc.who)
			closeRoot(t, made)
			if tc.ok && err != nil {
				t.Fatalf("Private(%s) = %v; want it accepted", dir, err)
			}
			if _, ok := errors.AsType[*Unsafe](err); !tc.ok && !ok {
				t.Errorf("Private(%s) = %v; want an *Unsafe in the folder's words", dir, err)
			}
		})
	}
}

// TestNothingIsMadeBelowANameTheWalkRefuses: a folder missing
// below a link that sits in a folder others may write is refused, and nothing
// is made in the folder the link leads to. Planted: a make that follows the
// link before the walk judges it (the folder is made, red on the stat).
func TestNothingIsMadeBelowANameTheWalkRefuses(t *testing.T) {
	t.Parallel()
	base := ours(t)
	open, target := filepath.Join(base, "open"), filepath.Join(base, "target")
	for _, d := range []string{open, target} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	testenv.Link(t, target, filepath.Join(open, "link"))
	trusttest.Loosen(t, open, trusttest.OthersWrite)
	made, err := Private(filepath.Join(open, "link", "state"), theFolder(), Who{})
	closeRoot(t, made)
	if _, ok := errors.AsType[*Unsafe](err); !ok {
		t.Errorf("Private below a link in a folder others may write = %v; want an *Unsafe", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "state")); err == nil {
		t.Errorf("a folder was made in %s through the link; want nothing made", target)
	}
}

// TestNothingIsMadeThroughALinkIntoAFolderKeptApart: a folder
// missing below a link into a folder of Who.Apart is refused and nothing is
// made there, as a clone's folder is not made in by a state folder's make.
// Planted: Apart not handed to go-files (the folder is made, red).
func TestNothingIsMadeThroughALinkIntoAFolderKeptApart(t *testing.T) {
	t.Parallel()
	base := ours(t)
	clone := filepath.Join(base, "clone")
	if err := os.Mkdir(clone, 0o700); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, clone, filepath.Join(base, "link"))
	made, err := Private(filepath.Join(base, "link", "state"), theFolder(), Who{Apart: []string{clone}})
	closeRoot(t, made)
	if _, ok := errors.AsType[*Unsafe](err); !ok {
		t.Errorf("Private below a link into a folder kept apart = %v; want an *Unsafe", err)
	}
	if _, err := os.Lstat(filepath.Join(clone, "state")); err == nil {
		t.Errorf("a folder was made in %s, a folder kept apart; want nothing made", clone)
	}
}
