package main

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// plantOpenRun writes, into the state folder at state, the journal of an open
// run r0 of root as a clone that carries the folder could write it: a kept
// copy of rules/b.yaml holding planted bytes, and of hook.sh, which is not
// there, holding others, each with a step its file now matches. The journal
// is written by the host's own Journal over a folder outside the root, then
// moved to state, so the adapter's own judgement of state plays no part in
// the planting.
func plantOpenRun(t *testing.T, tree *hostfs.Tree, root model.Root, dir, state string) {
	t.Helper()
	elsewhere := filepath.Join(t.TempDir(), "state")
	jr := hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: fixedClock{}, Dir: elsewhere}
	add := func(kind, detail string) {
		t.Helper()
		if err := jr.Append(t.Context(), root, model.Record{Run: "r0", Kind: kind, Command: "gragas gen --apply", Detail: []byte(detail)}); err != nil {
			t.Fatal(err)
		}
	}
	now, err := os.ReadFile(filepath.Join(dir, "rules", "b.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	add(model.KindIntent, "{}")
	for _, f := range []struct{ path, kept, after string }{
		{"rules/b.yaml", "status: planted\n", string(apply.DigestBytes(now))},
		{"hook.sh", "planted\n", ""},
	} {
		if err := jr.Keep(root, "r0", f.path, []byte(f.kept)); err != nil {
			t.Fatal(err)
		}
		add(model.KindCopy, `{"path":"`+f.path+`","existed":true,"before":"`+string(apply.DigestBytes([]byte(f.kept)))+`"}`)
		add(model.KindStep, `{"path":"`+f.path+`","op":"write","after":"`+f.after+`"}`)
	}
	if err := os.Rename(elsewhere, state); err != nil {
		t.Fatal(err)
	}
}

// TestAStateFolderInsideTheRootIsRefusedBeforeItsOpenRunIsRolledBack:
// the state folder lies
// in the root a run writes, and holds an open run the clone planted. The next
// --apply in that root, over the adapters world wires, must end before it
// reads that journal or writes anything, with ErrCarried, and leave the root
// byte for byte as it was. Without the judgement the run rolls the planted
// run back on its own: rules/b.yaml takes the planted bytes and hook.sh is
// made. The control is the same root with the state folder beside it, which
// ends done.
func TestAStateFolderInsideTheRootIsRefusedBeforeItsOpenRunIsRolledBack(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, inside bool) (apply.Outcome, map[string]string, map[string]string) {
		t.Helper()
		base := t.TempDir()
		dir := filepath.Join(base, "root")
		if err := os.MkdirAll(filepath.Join(dir, "rules"), 0o750); err != nil {
			t.Fatal(err)
		}
		for p, c := range map[string]string{"rules/a.yaml": "status: active\n", "rules/b.yaml": "status: active\n"} {
			if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(c), 0o640); err != nil {
				t.Fatal(err)
			}
		}
		state := filepath.Join(base, "state")
		if inside {
			state = filepath.Join(dir, ".state")
		}
		p := world(nil, config.Config{StateDir: state})
		root := opened(t, p, dir)
		if inside {
			plantOpenRun(t, p.Tree.(*hostfs.Tree), root, dir, state)
		}
		before := onDisk(t, dir)
		o := apply.Run(t.Context(), p, root, apply.Spec{
			Command: "gragas test --apply", Engine: "v0.0.0-test", Run: "run-1",
			Scope: []string{"rules/*.yaml"},
			Plan: func(context.Context) (model.Plan, error) {
				return model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "rule:a", Field: "status", Before: "active", After: "retired"}}}}, nil
			},
			Render: func(model.Delta) ([]model.Target, error) {
				return []model.Target{{Path: "rules/a.yaml", Before: apply.DigestBytes([]byte("status: active\n")), Data: []byte("status: retired\n")}}, nil
			},
		})
		return o, before, onDisk(t, dir)
	}

	o, before, after := run(t, true)
	if !maps.Equal(after, before) {
		t.Errorf("the root after\n %q\nwant it byte for byte as before\n %q", after, before)
	}
	if o.Word == model.Done || !errors.Is(o.Cause, port.ErrCarried) {
		t.Errorf("ended %q, cause %v; want a refusal matching ErrCarried", o.Word, o.Cause)
	}

	t.Run("the control, with the state folder beside the root, ends done", func(t *testing.T) {
		t.Parallel()
		o, _, after := run(t, false)
		if o.Word != model.Done || after["rules/a.yaml"] == "" {
			t.Fatalf("ended %q, cause %v, root %q", o.Word, o.Cause, after)
		}
	})
}

// TestTheRecordersLockRefusesAStateFolderTheWorkingFolderCarries:
// the old lock ports wires, which the run recorder takes in the
// state folder before it begins the run's record, judges that folder against
// the folders a clone carries, as world's lock does, so a state folder in the
// working folder (this package's own, inside the repository) is refused
// matching ErrCarried and never made. Planted: ports leaving the old lock's
// Roots unset (the folder is then made in the working folder, red here).
func TestTheRecordersLockRefusesAStateFolderTheWorkingFolderCarries(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{StateDir: filepath.Join(wd, ".state-2068-never-made")}
	lock := cfg.LockPath()
	t.Cleanup(func() {
		// Made only when the row is red: put back what the take made.
		_ = os.Remove(lock)
		_ = os.Remove(cfg.StateDir)
	})
	l, err := ports(cfg).Locks.Take(lock, "")
	if err == nil {
		t.Error("the old lock in the working folder is no error")
		if err := l.Release(); err != nil {
			t.Error(err)
		}
	} else if !errors.Is(err, port.ErrCarried) {
		t.Errorf("the old lock in the working folder = %v, want ErrCarried", err)
	}
	if _, err := os.Lstat(cfg.StateDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the state folder in the working folder is there after the refusal: %v", err)
	}
}

// inCache is a config whose state folder a variable moved into its cache
// folder, which is there and lies outside the working folder.
func inCache(t *testing.T) config.Config {
	t.Helper()
	cache := t.TempDir()
	return config.Config{StateDir: filepath.Join(cache, "state"), CacheDir: cache}
}

// TestAStateFolderInTheCacheIsRefusedByBothLocks: the cache
// folder, where every rulebook a run fetches is kept, is one of the folders
// a moved state folder is held apart from, so the recorder's old lock and
// world's Lock refuse a state folder there, matching ErrCarried, and make
// nothing. Planted: carriedRoots leaving the cache out (both takes are then
// no error and the state folder is made in the cache, red here).
func TestAStateFolderInTheCacheIsRefusedByBothLocks(t *testing.T) {
	t.Parallel()
	cfg := inCache(t)
	if l, err := ports(cfg).Locks.Take(cfg.LockPath(), ""); !errors.Is(err, port.ErrCarried) {
		t.Errorf("the old lock in the cache = %v, want ErrCarried", err)
		if err == nil {
			_ = l.Release()
		}
	}
	p := world(nil, cfg)
	root := opened(t, p, t.TempDir())
	if h, err := p.Lock.Take(t.Context(), root, model.RootLock); !errors.Is(err, port.ErrCarried) {
		t.Errorf("the lock in the cache = %v, want ErrCarried", err)
		if err == nil {
			_ = p.Lock.Release(h)
		}
	}
	if _, err := os.Lstat(cfg.StateDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the state folder in the cache is there after the refusals: %v", err)
	}
}

// TestAStateFolderWhereTheCacheWillBeIsRefusedBeforeEitherIsMade:
// a cache folder no run has made yet is held
// apart by its name, so a state folder in its place is refused by both locks,
// matching ErrCarried, and neither the state folder nor the cache is made; a
// state folder elsewhere is still taken with the cache missing. Planted:
// carriedRoots skipping a missing cache, as it did (the old lock then takes
// and makes the state folder there, red here).
func TestAStateFolderWhereTheCacheWillBeIsRefusedBeforeEitherIsMade(t *testing.T) {
	t.Parallel()
	cache := filepath.Join(t.TempDir(), "cache")
	cfg := config.Config{StateDir: filepath.Join(cache, "releases", "evil", "state"), CacheDir: cache}
	if l, err := ports(cfg).Locks.Take(cfg.LockPath(), ""); !errors.Is(err, port.ErrCarried) {
		t.Errorf("the old lock where the cache will be = %v, want ErrCarried", err)
		if err == nil {
			_ = l.Release()
		}
	}
	p := world(nil, cfg)
	root := opened(t, p, t.TempDir())
	if h, err := p.Lock.Take(t.Context(), root, model.RootLock); !errors.Is(err, port.ErrCarried) {
		t.Errorf("the lock where the cache will be = %v, want ErrCarried", err)
		if err == nil {
			_ = p.Lock.Release(h)
		}
	}
	if _, err := os.Lstat(cache); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the cache folder is there after the refusals: %v", err)
	}
	clear := config.Config{StateDir: filepath.Join(t.TempDir(), "state"), CacheDir: cache}
	l, err := ports(clear).Locks.Take(clear.LockPath(), "")
	if err != nil {
		t.Fatalf("the old lock outside a missing cache = %v, want it taken", err)
	}
	if err := l.Release(); err != nil {
		t.Error(err)
	}
}

// TestCacheListRefusesAStateFolderTheCacheCarries: cache
// list over the adapters world wires judges the state kind's folder as the
// Journal does before it reads a journal there, so a state folder in the
// cache holding a planted open run ends the listing matching ErrCarried, and
// the run is counted nowhere. Planted: fill not asking the reader first, with
// RecordsOf judging nothing (the listing then counts the planted run open,
// red here).
func TestCacheListRefusesAStateFolderTheCacheCarries(t *testing.T) {
	t.Parallel()
	cfg := inCache(t)
	p := world(nil, cfg)
	elsewhere := filepath.Join(t.TempDir(), "state")
	tree := p.Tree.(*hostfs.Tree)
	root := opened(t, p, t.TempDir())
	jr := hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: fixedClock{}, Dir: elsewhere}
	if err := jr.Append(t.Context(), root, model.Record{Run: "r0", Kind: model.KindIntent, Command: "gragas gen --apply", Detail: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(elsewhere, cfg.StateDir); err != nil {
		t.Fatal(err)
	}
	var ask coreapi.CacheAsk
	for _, k := range config.CacheKinds() {
		dir := cfg.StateDir
		if k.Place == config.InCache {
			dir = cfg.CacheDir
		}
		ask.Kinds = append(ask.Kinds, coreapi.CacheKind{Name: k.Name, Holds: k.Holds, Dir: dir, Names: k.Names, Journal: k.Journal, Built: k.Built, Nested: k.Nested})
	}
	got, err := coreapi.CacheList(t.Context(), p, ask)
	if !errors.Is(err, port.ErrCarried) {
		t.Errorf("cache list = %+v, %v; want ErrCarried", got, err)
	}
}

// TestAHomeVariableNamingAProjectIsNoAccountsOwnFolder:
// the home main judges the account's own state folder by
// is the one the system records, never the HOME variable, so with HOME naming
// a project and the state folder following it there, world's Lock and the old
// lock both refuse it, matching ErrCarried, and make nothing. It sets HOME, so
// it is not parallel. Planted: os.UserHomeDir, which reads HOME, in place of
// config.SystemHome at either stateRoots call in main (that lock then takes,
// red here).
func TestAHomeVariableNamingAProjectIsNoAccountsOwnFolder(t *testing.T) {
	project := t.TempDir()
	t.Setenv(config.HomeVar, project)
	cfg := config.FromEnv(func(k string) string {
		if k == config.HomeVar {
			return project
		}
		return ""
	})
	if cfg.StateDir == "" {
		t.Fatalf("no state folder under %s", project)
	}
	if l, err := ports(cfg).Locks.Take(cfg.LockPath(), project); !errors.Is(err, port.ErrCarried) {
		t.Errorf("the old lock in a state folder HOME moved into a project = %v, want ErrCarried", err)
		if err == nil {
			_ = l.Release()
		}
	}
	p := world(nil, cfg)
	root := opened(t, p, project)
	if h, err := p.Lock.Take(t.Context(), root, model.RootLock); !errors.Is(err, port.ErrCarried) {
		t.Errorf("the lock in a state folder HOME moved into a project = %v, want ErrCarried", err)
		if err == nil {
			_ = p.Lock.Release(h)
		}
	}
	if _, err := os.Lstat(cfg.StateDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the state folder is there after the refusals: %v", err)
	}
}

// TestTheAccountsOwnStateFolderIsNotHeldAgainstTheWorkingFolder:
// the state folder the system's home gives, with no variable moving it,
// is not judged by the Journal or the locks, so a run whose working folder is
// that home, here this package's folder inside the repository, is no
// refusal; one a variable moved under the same home is judged and refused.
// The approvals keep their roots. Planted: stateRoots answering roots for
// the account's own folder (the first row is then red).
func TestTheAccountsOwnStateFolderIsNotHeldAgainstTheWorkingFolder(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home := func() (string, error) { return wd, nil }
	own := config.FromEnv(func(k string) string {
		if k == config.HomeVar {
			return wd
		}
		return ""
	})
	if own.StateDir == "" {
		t.Fatalf("no state folder under %s", wd)
	}
	roots := carriedRoots(&hostfs.Tree{}, own.CacheDir)
	if got := stateRoots(own, home, roots); got != nil {
		t.Errorf("the account's own state folder %s is judged", own.StateDir)
	}
	moved := own
	moved.StateDir = filepath.Join(wd, ".state-2122-never-made")
	judged := stateRoots(moved, home, roots)
	if judged == nil {
		t.Fatalf("a moved state folder %s is not judged", moved.StateDir)
	}
	if _, err := (hostfs.LegacyLocks{Roots: judged}).Take(moved.LockPath(), ""); !errors.Is(err, port.ErrCarried) {
		t.Errorf("the old lock in a moved state folder in the working folder = %v, want ErrCarried", err)
		_ = os.Remove(moved.LockPath())
		_ = os.Remove(moved.StateDir)
	}
}
