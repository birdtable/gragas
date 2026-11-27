package tool_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// keyA and keyB are names a key folder takes: as many lowercase hex digits
// as a key holds.
const (
	keyA = "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"
	keyB = "1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b"
)

// plant makes each folder of dirs and writes each file of files, slash paths
// below base, private to this account.
func plant(t *testing.T, base string, dirs []string, files ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.FromSlash(base+"/"+d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.FromSlash(base+"/"+f), []byte("kept"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// there is whether the slash path p is there, read without following a link.
func there(p string) bool {
	_, err := os.Lstat(filepath.FromSlash(p))
	return err == nil
}

// sweep is the count and error of tl's CachesLeft, then of its SweepCaches.
func sweep(tl *tool.Tool) (left, removed int, lerr, rerr error) {
	left, lerr = tl.CachesLeft()
	removed, rerr = tl.SweepCaches()
	return left, removed, lerr, rerr
}

// TestAClearRemovesEveryKeyFolderAProgramLeftReadOnlyAmongThem: a module
// cache a tool keeps below its key folder holds folders it made read-only,
// and a clear removes every key folder whole, those among them, and counts
// each; CachesLeft counts them first and removes nothing, and a name no key
// takes is left. A second clear finds none.
// Planted: keyFolders removing by confine.RemoveAll, which cannot empty a
// read-only folder (the read-only row red off Windows); keyFolders counting
// every entry (the other name's row red); CachesLeft removing (the plan row
// red).
func TestAClearRemovesEveryKeyFolderAProgramLeftReadOnlyAmongThem(t *testing.T) {
	t.Parallel()
	cache := private(t) + "/tools"
	plant(t, cache, []string{keyA + "/LINT_MOD_CACHE/mod/pkg@v1.0.0", keyB + "/home-cache", "other"},
		keyA+"/LINT_MOD_CACHE/mod/pkg@v1.0.0/go.mod", keyB+"/home-cache/kept", "other/kept")
	for _, d := range []string{"/LINT_MOD_CACHE/mod/pkg@v1.0.0", "/LINT_MOD_CACHE/mod"} {
		p := filepath.FromSlash(cache + "/" + keyA + d)
		if err := os.Chmod(p, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(p, 0o700) })
	}
	tl := cacheTool(cache, "")
	if n, err := tl.CachesLeft(); n != 2 || err != nil {
		t.Errorf("CachesLeft = %d, %v; want 2 and no error", n, err)
	}
	if !there(cache + "/" + keyA + "/LINT_MOD_CACHE/mod/pkg@v1.0.0/go.mod") {
		t.Error("CachesLeft removed a key folder; want nothing written")
	}
	if n, err := tl.SweepCaches(); n != 2 || err != nil {
		t.Errorf("SweepCaches = %d, %v; want 2 and no error", n, err)
	}
	for _, k := range []string{keyA, keyB} {
		if there(cache + "/" + k) {
			t.Errorf("the key folder %s is there after the clear; want it removed, the read-only folders it holds among it", k)
		}
	}
	if !there(cache + "/other/kept") {
		t.Error("a folder at a name no key takes was removed; want it left")
	}
	if left, removed, lerr, rerr := sweep(tl); left != 0 || removed != 0 || lerr != nil || rerr != nil {
		t.Errorf("a second clear: %d left, %d removed, %v, %v; want none and no error", left, removed, lerr, rerr)
	}
}

// TestAClearRemovesALinkInTheCacheAndNothingItPointsAt: every step's program
// runs as this account, so a link in the cache may be an earlier step's. A
// link at a key folder's name, and one inside a key folder, pointing at
// folders of this account's outside the cache, are removed, and what they
// point at is not; the link at a key's name is counted as its folder is.
// Planted: keyNames keeping only the entries that are folders (the key link
// row red, 1 counted and the link left).
func TestAClearRemovesALinkInTheCacheAndNothingItPointsAt(t *testing.T) {
	t.Parallel()
	cache, outside := private(t)+"/tools", private(t)
	plant(t, cache, []string{keyA + "/LINT_CACHE"})
	plant(t, outside, []string{"pointed", "within"}, "pointed/kept", "within/kept")
	testenv.Link(t, filepath.FromSlash(outside+"/pointed"), filepath.FromSlash(cache+"/"+keyB))
	testenv.Link(t, filepath.FromSlash(outside+"/within"), filepath.FromSlash(cache+"/"+keyA+"/LINT_CACHE/out"))
	left, removed, lerr, rerr := sweep(cacheTool(cache, ""))
	if left != 2 || removed != 2 || lerr != nil || rerr != nil {
		t.Errorf("a cache with links: %d left, %d removed, %v, %v; want 2 and 2 and no error", left, removed, lerr, rerr)
	}
	for _, k := range []string{keyA, keyB} {
		if there(cache + "/" + k) {
			t.Errorf("%s is there after the clear; want it removed", k)
		}
	}
	for _, f := range []string{"pointed/kept", "within/kept"} {
		if !there(outside + "/" + f) {
			t.Errorf("%s, outside the cache, was removed through a link; want it untouched", f)
		}
	}
}

// TestAToolsCacheALinkOrOpenToOthersIsRefusedAndNothingRemoved: the tools
// cache judged as a step judges it, so a link at its name, pointing wherever,
// a cache any account may write, and a file at its name each refuse the
// count and the clear, and nothing is removed, below the link's end least of
// all.
// Planted: keyFolders opening the Cache by name with os.OpenRoot, which
// follows the link (the link row red, the folder at its end emptied);
// keyFolders neither judging the Cache by name nor holding it (cacheHeld)
// (the open row red).
func TestAToolsCacheALinkOrOpenToOthersIsRefusedAndNothingRemoved(t *testing.T) {
	t.Parallel()
	open := private(t) + "/tools"
	plant(t, open, []string{keyA})
	trusttest.Loosen(t, open, trusttest.OthersWrite)
	holder := private(t)
	plant(t, holder, nil, "tools")
	file := holder + "/tools"
	real, linked := private(t), private(t)+"/tools"
	plant(t, real, []string{keyA})
	testenv.Link(t, filepath.FromSlash(real), filepath.FromSlash(linked))
	for name, c := range map[string]struct{ cache, key string }{
		"a link at the cache's name": {linked, real + "/" + keyA},
		"open to every account":      {open, open + "/" + keyA},
		"a file at the cache's name": {file, file},
	} {
		left, removed, lerr, rerr := sweep(cacheTool(c.cache, ""))
		if !errors.Is(lerr, port.ErrRefused) || !errors.Is(rerr, port.ErrRefused) || left != 0 || removed != 0 {
			t.Errorf("%s: %d left, %d removed, %v, %v; want both refused, nothing counted", name, left, removed, lerr, rerr)
		}
		if !there(c.key) {
			t.Errorf("%s: %s was removed; want nothing removed", name, c.key)
		}
	}
}

// TestNoToolsCacheClearsNothingAndMakesNothing: a Tool with no tools cache,
// and one whose cache, or the folder above it, is not there, count and remove
// none with no error, and make no folder.
// Planted: keyFolders making the folder above the Cache by host/trust's
// Private (the missing rows red, a folder made).
func TestNoToolsCacheClearsNothingAndMakesNothing(t *testing.T) {
	t.Parallel()
	base := private(t)
	for name, cache := range map[string]string{
		"no cache":                 "",
		"a cache not there":        base + "/tools",
		"a folder above not there": base + "/missing/tools",
	} {
		if left, removed, lerr, rerr := sweep(cacheTool(cache, "")); left != 0 || removed != 0 || lerr != nil || rerr != nil {
			t.Errorf("%s: %d left, %d removed, %v, %v; want none and no error", name, left, removed, lerr, rerr)
		}
	}
	if entries, err := os.ReadDir(filepath.FromSlash(base)); err != nil || len(entries) != 0 {
		t.Errorf("the folder above holds %v, %v; want nothing made", entries, err)
	}
}
