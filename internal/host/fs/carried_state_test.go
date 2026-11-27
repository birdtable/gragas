package fs_test

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/confine"
	testenv "github.com/birdtable/go-testkit"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// opening is the journal record every call here appends.
func opening() model.Record {
	return model.Record{Run: "r1", Kind: model.KindIntent, Command: "gragas test --apply", Detail: []byte("{}")}
}

// projectEntries is how many entries the project holds.
func projectEntries(t *testing.T, project string) int {
	t.Helper()
	got, err := os.ReadDir(project)
	if err != nil {
		t.Fatal(err)
	}
	return len(got)
}

// TestTheJournalAndTheLockRefuseAStateFolderAProjectCarries: a state folder in
// the project, or the project itself, is refused
// by every call of the Journal, by the Lock's take and by the old lock's
// take, which the run recorder makes before it begins its record, matching
// ErrCarried, before anything is read or made there, so the project holds
// what it held. It is refused by the root the call names alone too, with no
// other folder answered, the old lock's -C root among them,
// and RecordsOf, which cache list asks first, refuses it.
// The control is the same state folder beside the project, which each takes.
// Planted: runApart judging nothing (each call then makes its state folder in
// the project, red here); the old lock leaving its root out, and RecordsOf
// judging nothing (each their own call red).
func TestTheJournalAndTheLockRefuseAStateFolderAProjectCarries(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		dir   func(project string) string
		roots func(project string) []string
		want  error
	}{
		{"a folder in the project", func(p string) string { return filepath.Join(p, ".state") }, func(p string) []string { return []string{p} }, port.ErrCarried},
		{"the project itself", func(p string) string { return p }, func(p string) []string { return []string{p} }, port.ErrCarried},
		{"in the root the call names, no other answered", func(p string) string { return filepath.Join(p, ".state") }, func(string) []string { return nil }, port.ErrCarried},
		{"the control, beside the project", func(p string) string { return filepath.Join(filepath.Dir(p), "state") }, func(p string) []string { return []string{p} }, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			project := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(project, 0o750); err != nil {
				t.Fatal(err)
			}
			dir := c.dir(project)
			roots := func() ([]string, error) { return c.roots(project), nil }
			tree := newTree(t)
			root, err := tree.Open(project)
			if err != nil {
				t.Fatal(err)
			}
			held := projectEntries(t, project)
			lock := &hostfs.Lock{Dir: dir, Clock: lockInstant(), Roots: roots}
			jr := hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: lockInstant(), Dir: dir, Roots: roots}
			calls := map[string]error{}
			h, err := lock.Take(t.Context(), root, model.RootLock)
			calls["Lock.Take"] = err
			if err == nil {
				if err := lock.Release(h); err != nil {
					t.Fatal(err)
				}
			}
			// The old lock is handed the project as the folder -C names,
			// so it judges it as the Lock judges its call's
			// root, in the case that answers no other folder too.
			old, err := hostfs.LegacyLocks{Clock: lockInstant(), Roots: roots}.Take(filepath.Join(dir, "gragas.lock"), project)
			calls["LegacyLocks.Take"] = err
			if err == nil {
				if err := old.Release(); err != nil {
					t.Fatal(err)
				}
			}
			calls["Journal.Append"] = jr.Append(t.Context(), root, opening())
			calls["Journal.Keep"] = jr.Keep(root, "r1", "a.txt", []byte("old"))
			_, calls["Journal.Kept"] = jr.Kept(root, "r1", "a.txt")
			_, calls["Journal.KeptAt"] = jr.KeptAt(root, "r1", "a.txt")
			_, _, calls["Journal.Records"] = jr.Records(t.Context(), root)
			_, calls["Journal.Sweep"] = jr.Sweep(t.Context(), root)
			// RecordsOf names no root, so it judges only the folders
			// answered: it has no row in the case that answers none.
			if len(c.roots(project)) > 0 {
				_, _, calls["Journal.RecordsOf"] = jr.RecordsOf(t.Context(), nil)
			}
			for call, err := range calls {
				switch {
				case c.want != nil && !errors.Is(err, c.want):
					t.Errorf("%s = %v, want %v", call, err, c.want)
				case c.want == nil && err != nil:
					t.Errorf("%s = %v, want no error", call, err)
				}
			}
			if c.want != nil {
				if got := projectEntries(t, project); got != held {
					t.Errorf("the project holds %d entries after the refusals, want %d", got, held)
				}
			}
		})
	}
}

// volumeFolds is whether the folder dir reads a name in another case as the
// same name, asked of it by a probe through go-files' confine.Collide, which
// makes the probe and removes it, so a row holds on either kind of volume.
// It is the tests' own answer, not the code's: the code asks confine.Folds,
// and TestFoldsNamesARuleWhereTheProbeFolds holds the two together. dir is a
// folder of the test's own, never one whose entries a row counts.
func volumeFolds(t *testing.T, dir string) bool {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	folds, err := confine.Collide(root, "fold-probe", "FOLD-PROBE")
	if err != nil {
		t.Fatal(err)
	}
	return folds
}

// TestFoldsNamesARuleWhereTheProbeFolds: where a probe finds that a folder
// reads a name in another case as the same name, confine.Folds, which the
// code asks and which writes nothing, answers a rule other than FoldNone for
// it, so the compare never reads two names the folder takes for one as two.
// Where the probe folds nothing, Folds may answer any rule: a rule that folds
// there only refuses more. Risk: Folds answering FoldNone on a volume that
// folds case, the Mac's temp among them; the compare would then take a state
// folder in another case of a missing cache.
func TestFoldsNamesARuleWhereTheProbeFolds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folding, err := confine.Folds(dir)
	if !volumeFolds(t, dir) {
		t.Logf("the probe folds nothing here; Folds(%s) = %v, %v", dir, folding, err)
		return
	}
	if folding == confine.FoldNone {
		t.Errorf("Folds(%s) = FoldNone, %v, where the probe reads FOLD-PROBE as fold-probe", dir, err)
	}
}

// TestAStateFolderInACaseVariantOfAMissingCacheIsRefusedWhereTheVolumeFolds:
// the cache is not there yet, and the state folder is named in another case
// of it. On a volume that
// reads a name in another case as the same name, the Mac's temp among them,
// the first run would make the state folder and then the cache at its own
// spelling, the same folder, so the old lock refuses it, matching ErrCarried,
// and makes neither. On a volume that does not fold, the other case is another
// folder, and it is taken. The cache's own name is refused on every volume,
// and the control, another name, is taken on every volume. The folder the
// cache will be made in is tried both empty and holding an entry of its own.
// Planted: inMissingRoot answering false (the upper-case rows take the lock
// where the volume folds, red here), and sameName comparing bytes alone under
// every rule (the same rows red).
// Risk: a first run that makes its state folder inside the cache it fetches
// rulebooks into, on a volume that folds case.
func TestAStateFolderInACaseVariantOfAMissingCacheIsRefusedWhereTheVolumeFolds(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		state   string
		folded  bool // refused only where the volume folds
		refused bool // refused on every volume
	}{
		{"the cache's name in upper case", filepath.Join("Caches", "GRAGAS", "state"), true, false},
		{"the cache's name below its folder in another case", filepath.Join("caches", "GRAGAS", "state"), true, false},
		{"the cache's own name, written as it is", filepath.Join("Caches", "gragas", "state"), false, true},
		{"the control, another name", filepath.Join("Caches", "gragas-elsewhere", "state"), false, false},
	} {
		for _, populated := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s, the folder populated %v", c.name, populated), func(t *testing.T) {
				t.Parallel()
				base := t.TempDir()
				folds := volumeFolds(t, base)
				caches := filepath.Join(base, "Caches")
				if err := os.Mkdir(caches, 0o750); err != nil {
					t.Fatal(err)
				}
				if populated {
					if err := os.WriteFile(filepath.Join(caches, "Other"), nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				cache := filepath.Join(caches, "gragas")
				dir := filepath.Join(base, c.state)
				roots := func() ([]string, error) { return []string{cache}, nil }
				old, err := hostfs.LegacyLocks{Clock: lockInstant(), Roots: roots}.Take(filepath.Join(dir, "gragas.lock"), "")
				want := c.refused || (c.folded && folds)
				switch {
				case want && !errors.Is(err, port.ErrCarried):
					t.Fatalf("the old lock in %s against a missing %s (the volume folds: %v) = %v, want ErrCarried", c.state, cache, folds, err)
				case !want && err != nil:
					t.Fatalf("the old lock in %s against a missing %s (the volume folds: %v) = %v, want it taken", c.state, cache, folds, err)
				case !want:
					if err := old.Release(); err != nil {
						t.Fatal(err)
					}
					return
				}
				for _, made := range []string{dir, cache} {
					if _, err := os.Lstat(made); !errors.Is(err, os.ErrNotExist) {
						t.Errorf("%s is there after the refusal: %v", made, err)
					}
				}
			})
		}
	}
}

// freshHome is the default layout on a home that holds neither .local nor
// .cache: the state folder at .local/state/gragas and the cache at
// .cache/gragas, neither there.
func freshHome(home string) (dir string, roots []string) {
	return filepath.Join(home, ".local", "state", "gragas"), []string{filepath.Join(home, ".cache", "gragas")}
}

// TestAFreshHomeIsJudgedWithNothingWrittenInIt: in the default layout on a
// fresh home, Carried takes the state folder and the home holds no entry
// after: with the home writable, with it read-only, with two judgements at
// once on one home, and beside another program making the home's .local,
// round after round. The compare asks the folder its rule and makes no name
// in it. Planted: the compare asking go-files' confine.Collide of each name
// pair in the home, which makes the first name as its probe (the read-only
// row refused with the probe's permission error, and the other program's
// mkdir refused while the probe stands, red in three runs of three; the two
// judgements held in those runs, as Collide at go-files v0.17.0 looks again
// where a look changed).
// Risk: an empty file left at the home's .local, which every program that
// keeps its files there then fails on, or a run on a fresh account refused.
func TestAFreshHomeIsJudgedWithNothingWrittenInIt(t *testing.T) {
	t.Parallel()
	judged := func(t *testing.T, home string) {
		t.Helper()
		dir, roots := freshHome(home)
		if err := hostfs.Carried(dir, roots); err != nil {
			t.Errorf("Carried(%s, %q) = %v, want it taken", dir, roots, err)
		}
	}
	t.Run("a writable home", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		judged(t, home)
		if got := projectEntries(t, home); got != 0 {
			t.Errorf("the home holds %d entries after the judgement, want 0", got)
		}
	})
	t.Run("a read-only home", func(t *testing.T) {
		t.Parallel()
		if !testenv.KeepsModes(t) {
			return
		}
		home := t.TempDir()
		if err := os.Chmod(home, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(home, 0o700); err != nil {
				t.Error(err)
			}
		})
		judged(t, home)
		if got := projectEntries(t, home); got != 0 {
			t.Errorf("the home holds %d entries after the judgement, want 0", got)
		}
	})
	t.Run("two judgements at once", func(t *testing.T) {
		t.Parallel()
		for range 200 {
			home := t.TempDir()
			dir, roots := freshHome(home)
			start, errs := make(chan struct{}), make(chan error, 2)
			for range 2 {
				go func() {
					<-start
					errs <- hostfs.Carried(dir, roots)
				}()
			}
			close(start)
			for range 2 {
				if err := <-errs; err != nil {
					t.Fatalf("Carried(%s, %q) at once with another = %v, want it taken", dir, roots, err)
				}
			}
			if got := projectEntries(t, home); got != 0 {
				t.Fatalf("the home holds %d entries after two judgements, want 0", got)
			}
		}
	})
	t.Run("another program making the home's .local at once", func(t *testing.T) {
		t.Parallel()
		for range 500 {
			home := t.TempDir()
			dir, roots := freshHome(home)
			start, judged, made := make(chan struct{}), make(chan error, 1), make(chan error, 1)
			go func() {
				<-start
				judged <- hostfs.Carried(dir, roots)
			}()
			go func() {
				<-start
				made <- os.Mkdir(filepath.Join(home, ".local"), 0o700)
			}()
			close(start)
			if err := <-made; err != nil {
				t.Fatalf("another program's mkdir of the home's .local beside the judgement = %v, want it made", err)
			}
			if err := <-judged; err != nil {
				t.Fatalf("Carried(%s, %q) beside another program's mkdir = %v, want it taken", dir, roots, err)
			}
		}
	})
}

// TestANameTheKeyCannotCompareIsRefusedWhereTheFolderMayFold: the cache is
// not there yet, and the state folder's name beside it is one go-files'
// confine.Portable refuses, gragas. with its trailing dot, which Windows
// strips, so it is the cache there. Where the folder may fold, the name
// cannot be compared by key, so Carried refuses it with Portable's reason,
// matching fs.ErrInvalid, and nothing is made. Where it folds nothing, the
// bytes differ and it is taken. Planted: a key refusal taken as another name
// (the folding row taken, red here).
func TestANameTheKeyCannotCompareIsRefusedWhereTheFolderMayFold(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	caches := filepath.Join(base, "Caches")
	if err := os.Mkdir(caches, 0o750); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(caches, "gragas.", "state")
	roots := []string{filepath.Join(caches, "gragas")}
	err := hostfs.Carried(dir, roots)
	if volumeFolds(t, base) {
		if !errors.Is(err, iofs.ErrInvalid) {
			t.Errorf("Carried(%s, %q) = %v, want Portable's refusal, matching fs.ErrInvalid", dir, roots, err)
		}
	} else if err != nil {
		t.Errorf("Carried(%s, %q) on a folder that folds nothing = %v, want it taken", dir, roots, err)
	}
	if got := projectEntries(t, caches); got != 0 {
		t.Errorf("the folder holds %d entries after the judgement, want 0", got)
	}
}

// TestALinkMadeIntoAProjectAfterTheJudgementMakesNoJournalOrLockThere (as
// TestALinkMadeIntoACloneAfterTheCheckMakesNothingThere is the approvals'):
// the state folder is missing and clear of the project when the Journal or
// the Lock judges it, and a link into the project is made at its name before
// the make. The make holds the project apart, so the take and the append are
// refused and the project holds nothing. Planted: the make judging with no
// folder held apart (the lock file and the journal are then made in the
// project, red here).
func TestALinkMadeIntoAProjectAfterTheJudgementMakesNoJournalOrLockThere(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (project, dir string, roots func() ([]string, error), tree *hostfs.Tree, root model.Root, link func()) {
		t.Helper()
		project = filepath.Join(t.TempDir(), "project")
		if err := os.MkdirAll(project, 0o750); err != nil {
			t.Fatal(err)
		}
		dir = filepath.Join(t.TempDir(), "state")
		roots = func() ([]string, error) { return []string{project}, nil }
		tree = newTree(t)
		root, err := tree.Open(project)
		if err != nil {
			t.Fatal(err)
		}
		link = func() {
			testenv.Link(t, project, dir)
		}
		return project, dir, roots, tree, root, link
	}

	t.Run("the lock", func(t *testing.T) {
		t.Parallel()
		project, dir, roots, _, root, link := setup(t)
		lock := &hostfs.Lock{Dir: dir, Clock: lockInstant(), Roots: roots}
		hostfs.LockJudged(lock, link)
		if h, err := lock.Take(t.Context(), root, model.RootLock); err == nil {
			t.Error("the take through a link into the project is no error")
			if err := lock.Release(h); err != nil {
				t.Error(err)
			}
		}
		if got := projectEntries(t, project); got != 0 {
			t.Errorf("the project holds %d entries after the refused take", got)
		}
	})

	t.Run("the journal", func(t *testing.T) {
		t.Parallel()
		project, dir, roots, tree, root, link := setup(t)
		jr := hostfs.JournalJudged(hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: lockInstant(), Dir: dir, Roots: roots}, link)
		if err := jr.Append(t.Context(), root, opening()); err == nil {
			t.Error("the append through a link into the project is no error")
		}
		if got := projectEntries(t, project); got != 0 {
			t.Errorf("the project holds %d entries after the refused append", got)
		}
	})
}
