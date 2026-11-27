package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/host/sandbox"
	"github.com/birdtable/gragas/internal/model"
)

// TestTheSandboxMakesItsFoldersInTheCacheAsTheScratchKind: the Sandbox main
// wires prepares a folder in the cache folder, never in a project, at a
// name the table of kept files holds as built, of the scratch kind the cache
// command lists. Risk: a run's files written where nothing says gragas keeps
// them.
func TestTheSandboxMakesItsFoldersInTheCacheAsTheScratchKind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.Config{CacheDir: dir, StateDir: filepath.Join(dir, "state"), ConfigDir: filepath.Join(dir, "config")}
	p := world(nil, cfg)
	sc, err := p.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a/b.go"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Sandbox.Release(sc); err != nil {
			t.Error(err)
		}
	})
	place, name, ok := cfg.Where(filepath.FromSlash(sc.Dir))
	if !ok || place != config.InCache || !strings.HasPrefix(name, sandbox.Folder+"/") {
		t.Fatalf("the folder %s is at %q, %q, %v; want in the cache's %s folder", sc.Dir, place, name, ok, sandbox.Folder)
	}
	if row, held := config.Holds(config.Persisted(), config.InCache, name); !held || !row.Built {
		t.Errorf("%s is held by %+v, %v; want a built row of the table of kept files", name, row, held)
	}
	kinds := config.CacheKinds()
	i := slices.IndexFunc(kinds, func(k config.CacheKind) bool { return k.Name == sandbox.Folder })
	if i < 0 || !kinds[i].Built || kinds[i].Place != config.InCache || !slices.Contains(kinds[i].Names, sandbox.Folder+"/*") {
		t.Errorf("no built scratch kind in the cache naming %s/*: %+v", sandbox.Folder, kinds)
	}
}

// TestCacheListAndClearReachTheWiredSandboxsNestedFolders: over the disk, the
// folders the wired Sandbox prepares are counted by cache list down to their
// files, counted by the clear plan, and removed by cache clear under the
// locks, which end with no lock held. Risk: a listing that misses the files of
// a run's folder, or a clear that leaves what a stopped run left.
func TestCacheListAndClearReachTheWiredSandboxsNestedFolders(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	cfg := config.Config{CacheDir: filepath.Join(base, "cache"), StateDir: filepath.Join(base, "state"), ConfigDir: filepath.Join(base, "config")}
	for _, d := range []string{cfg.CacheDir, cfg.StateDir, filepath.Join(base, "project")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	p := world(nil, cfg)
	// Main wires Steps after world; the clear plan counts the tools' cache folders through it.
	p.Steps = tools(cfg, 0)
	root := opened(t, p, filepath.Join(base, "project"))
	var made []model.Scratch
	for _, files := range [][]model.File{
		{{Path: "src/a.go", Data: []byte("aa")}, {Path: "src/deep/b.go", Data: []byte("bbb")}},
		{{Path: "c.go", Data: []byte("c")}},
	} {
		sc, err := p.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: files})
		if err != nil {
			t.Fatal(err)
		}
		made = append(made, sc)
	}
	var kinds []coreapi.CacheKind
	for _, k := range config.CacheKinds() {
		if k.Name == "scratch" {
			kinds = append(kinds, coreapi.CacheKind{Name: k.Name, Holds: k.Holds, Dir: cfg.CacheDir, Names: k.Names, Built: k.Built, Nested: k.Nested})
		}
	}
	listing, err := coreapi.CacheList(t.Context(), p, coreapi.CacheAsk{Keep: "30d", KeepSeconds: 30 * 86400, Kinds: kinds})
	if err != nil || listing.Kinds[0].Files != 3 || listing.Kinds[0].Size == nil || *listing.Kinds[0].Size != 6 {
		t.Fatalf("cache list = %+v, %v; want the scratch kind to hold 3 files of 6 bytes", listing, err)
	}
	// Names no Prepare makes: the plan must not count what the clear leaves.
	scratch := config.ScratchDir(cfg.CacheDir)
	for _, f := range []string{filepath.Join(scratch, "mine", "sub", "f"), filepath.Join(scratch, "stray")} {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := coreapi.CacheClearPlan(t.Context(), p, root, 30*86400)
	if err != nil || plan.Scratch != 2 {
		t.Fatalf("the clear plan = %+v, %v; want 2 scratch folders", plan, err)
	}
	cleared, err := coreapi.CacheClear(t.Context(), p, root)
	if err != nil || cleared.Word != model.Done || cleared.Scratch != plan.Scratch {
		t.Fatalf("cache clear = %+v, %v; want done with the plan's %d scratch folders removed", cleared, err, plan.Scratch)
	}
	for _, sc := range made {
		if _, err := os.Lstat(filepath.FromSlash(sc.Dir)); !os.IsNotExist(err) {
			t.Errorf("%s is still there after a clear: %v", sc.Dir, err)
		}
	}
	for _, f := range []string{filepath.Join(scratch, "mine", "sub", "f"), filepath.Join(scratch, "stray")} {
		if _, err := os.Lstat(f); err != nil {
			t.Errorf("%s, no folder Prepare makes, is gone after a clear: %v", f, err)
		}
	}
	for _, which := range []model.Lock{model.RootLock, model.ScratchLock} {
		h, err := p.Lock.Take(t.Context(), root, which)
		if err != nil {
			t.Fatalf("the %s lock is held after a clear: %v", which, err)
		}
		if err := p.Lock.Release(h); err != nil {
			t.Error(err)
		}
	}
}

// TestACacheClearInOneFolderLeavesTheScratchFolderARunInAnotherHolds: over the
// disk, two folders share the cache's one scratch folder. A run of the second
// holds its locks as proof.Run's do and has a folder prepared; a clear from
// the first then ends busy and leaves that folder whole, and once the run
// gives its locks back the clear removes it. Risk: a clear in one project that
// empties the folder a tool of another is running in.
func TestACacheClearInOneFolderLeavesTheScratchFolderARunInAnotherHolds(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	cfg := config.Config{CacheDir: filepath.Join(base, "cache"), StateDir: filepath.Join(base, "state"), ConfigDir: filepath.Join(base, "config")}
	for _, d := range []string{cfg.CacheDir, cfg.StateDir, filepath.Join(base, "a"), filepath.Join(base, "b")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	a, b := world(nil, cfg), world(nil, cfg)
	// Main wires Steps after world; the clear plan counts the tools' cache folders through it.
	a.Steps, b.Steps = tools(cfg, 0), tools(cfg, 0)
	rootA := opened(t, a, filepath.Join(base, "a"))
	rootB := opened(t, b, filepath.Join(base, "b"))
	var holds []model.Hold
	// The run of b ends here whatever the test does: a lock file is open for as
	// long as its lock is held, and Windows removes no folder with a file open,
	// so a failed step before the release below left TempDir's cleanup to
	// fail on it and hide the failure. Registered after TempDir, it runs first.
	t.Cleanup(func() {
		for _, h := range slices.Backward(holds) {
			_ = b.Lock.Release(h) // a hold already given back is refused, and nothing is left to do
		}
	})
	for _, which := range []model.Lock{model.RootLock, model.ScratchLock} {
		h, err := b.Lock.Take(t.Context(), rootB, which)
		if err != nil {
			t.Fatalf("the run of b takes its %s lock: %v", which, err)
		}
		holds = append(holds, h)
	}
	sc, err := b.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "src/a.go", Data: []byte("aa")}}})
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := coreapi.CacheClear(t.Context(), a, rootA)
	if err != nil || cleared.Word != model.Busy || cleared.Scratch != 0 {
		t.Fatalf("a clear from a while b's run holds its folder = %+v, %v; want busy and nothing removed", cleared, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(sc.Dir), "src", "a.go")); err != nil {
		t.Fatalf("the folder b's run holds was touched by a's clear: %v", err)
	}
	for _, h := range slices.Backward(holds) {
		if err := b.Lock.Release(h); err != nil {
			t.Fatal(err)
		}
	}
	if cleared, err := coreapi.CacheClear(t.Context(), a, rootA); err != nil || cleared.Word != model.Done || cleared.Scratch != 1 {
		t.Fatalf("a clear from a once b's run stopped = %+v, %v; want done with its folder removed", cleared, err)
	}
}
