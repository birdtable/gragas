package sandbox_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/sandbox"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TestTheFolderTheRunsAreMadeInIsThisAccountsAlone (review item 6): Prepare
// makes the base private to this account, and a base another account can
// write refuses Prepare and Release by name, with no run folder made and
// none removed. Risk: another account that can write the base swaps a run
// folder for one of its own, its config files in it, before the tool starts
// there. Who may write it is changed by go-files' trusttest, the mode bits on
// Unix and the access list on Windows, so no row names a system; a base
// others may only read is the control, as a 0755 one was.
//
// The isolated level opens on every system, Windows among them, where once
// an owner of the SID form was refused unjudged:
// go-files' trust.Held now judges the access list on the handle. Planted
// failures: the SID refusal back (the first Prepare refused on Windows, as a
// Windows run once saw), and Held not called (every loosened base passes, red here).
func TestTheFolderTheRunsAreMadeInIsThisAccountsAlone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		who     trusttest.Who
		refused bool
	}{{trusttest.GroupWrites, true}, {trusttest.OthersWrite, true}, {trusttest.OthersRead, false}} {
		t.Run(name(tc.who), func(t *testing.T) {
			t.Parallel()
			dir := cache(t)
			s := sandbox.New(sandbox.Config{Dir: dir})
			sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
			if err != nil {
				t.Fatalf("the first Prepare = %v, want the isolated level open", err)
			}
			base := filepath.Join(dir, sandbox.Folder)
			trusttest.Perm(t, base, 0o700)
			trusttest.Loosen(t, base, tc.who)
			got, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
			if !tc.refused {
				if err != nil {
					t.Fatalf("the control, a base %s = %v; want it prepared", name(tc.who), err)
				}
				for _, made := range []model.Scratch{got, sc} {
					if err := s.Release(made); err != nil {
						t.Errorf("the control's Release of %s: %v", made.Dir, err)
					}
				}
				return
			}
			if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "can be written by another account") {
				t.Errorf("Prepare in a base %s = %+v, %v; want refused", name(tc.who), got, err)
			}
			if err := s.Release(sc); !errors.Is(err, port.ErrRefused) {
				t.Errorf("Release in a base %s = %v; want refused", name(tc.who), err)
			}
			if left := runs(t, dir); len(left) != 1 {
				t.Errorf("a base %s holds %v, want the one run folder", name(tc.who), left)
			}
		})
	}
}

// name is what the test says of a base who is let into.
func name(who trusttest.Who) string {
	switch who {
	case trusttest.GroupWrites:
		return "its group may write"
	case trusttest.OthersWrite:
		return "others may write"
	case trusttest.OthersRead:
		return "others may read"
	}
	return "loosened"
}

// TestJudgeRefusesAnotherAccountsBase: the owner judge of the base on its
// own, with owners in the form trust.Account gives, "uid:N" and a SID.
// Another account is refused, root and the system's own included, which
// trust.Held alone would pass as root's. This account's own is passed in
// either form, and what else may write it is Held's, on the handle
// (TestTheFolderTheRunsAreMadeInIsThisAccountsAlone). Planted failure: the
// owner not compared, and every row passes.
func TestJudgeRefusesAnotherAccountsBase(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ owner, me string }{
		{"uid:1001", "uid:501"},
		{"uid:0", "uid:501"},
		{"S-1-5-18", "S-1-5-21-1-2-3-1001"},
		{"S-1-5-32-544", "S-1-5-21-1-2-3-1001"},
		{"uid:5010", "uid:501"},
	} {
		if err := sandbox.Judge("/work/scratch", tc.owner, tc.me); !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "is owned by account "+tc.owner+",") {
			t.Errorf("a base owned by %s, for %s = %v; want refused as its", tc.owner, tc.me, err)
		}
	}
	for _, me := range []string{"uid:501", "S-1-5-21-1-2-3-1001"} {
		if err := sandbox.Judge("/work/scratch", me, me); err != nil {
			t.Errorf("the control, a base of this account's, %s: %v", me, err)
		}
	}
}

// TestTheBaseIsJudgedOnTheFolderOpenedNotTheName: owned opens the folder and
// holds the handle to the Lstat it was handed, so a folder swapped in at the
// name between the two is refused before its owner or mode is read. The
// test hands owned the Lstat of another folder, which is what a swap looks
// like from inside. Planted failure: the handle's Stat not compared, and
// the folder at the name judged as if it were the one the Lstat read.
func TestTheBaseIsJudgedOnTheFolderOpenedNotTheName(t *testing.T) {
	t.Parallel()
	dir := cache(t)
	at, other := filepath.Join(dir, "at"), filepath.Join(dir, "other")
	for _, d := range []string{at, other} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fi, err := os.Lstat(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := sandbox.Owned(at, fi, nil); !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "was moved while it was opened") {
		t.Errorf("a folder whose Lstat is another's = %v; want refused as moved", err)
	}
	if err := sandbox.Owned(filepath.Join(dir, "gone"), fi, nil); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a folder gone before the open = %v; want it not found", err)
	}
}

// TestABaseOnADiskWhoseOwnersAreNotJudgedSaysTheDiskIsWhy: a
// base trust.Held refuses with ErrMount, as a cache on a disk mounted nosuid,
// or noexec without nosuid, is refused by name with host/trust's fix line for
// it, the walk's own: the way the disk is mounted is why, another folder on
// it is refused too, and the line names the setting that lets such a disk be
// judged. It
// keeps both errors. Any other refusal of Held keeps the line of who may
// write it, without the mount's. Planted failure: ErrMount not told apart,
// and the mount's row says another account can write the base.
func TestABaseOnADiskWhoseOwnersAreNotJudgedSaysTheDiskIsWhy(t *testing.T) {
	t.Parallel()
	mount := fmt.Errorf("/work/scratch/cache/gragas is on a disk mounted nosuid: %w", trust.ErrMount)
	err := sandbox.Unheld("/work/scratch/cache/gragas", mount)
	if !errors.Is(err, port.ErrRefused) || !errors.Is(err, trust.ErrMount) {
		t.Errorf("a base on a disk not judged = %v; want it refused, with ErrMount kept", err)
	}
	for _, want := range []string{"/work/scratch/cache/gragas, where scratch folders are made, is on a disk whose owners are not judged", hosttrust.MountFix} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("a base on a disk not judged = %v; want it to say %q", err, want)
		}
	}
	other := errors.New("/work/scratch can be written by its group")
	err = sandbox.Unheld("/work/scratch", other)
	if !errors.Is(err, port.ErrRefused) || !errors.Is(err, other) || !strings.Contains(err.Error(), "can be written by another account") {
		t.Errorf("a base another account can write = %v; want it refused as written by another account", err)
	}
	if err != nil && strings.Contains(err.Error(), hosttrust.MountFix) {
		t.Errorf("a base another account can write = %v; want no line of the mount's", err)
	}
}

// cacheNosuidProblems is what the refusal err of the cache folder dir, on a
// disk mounted nosuid, gets wrong: it must be refused, match trust.ErrMount,
// and say dir, the mount and MountFix, which names GRAGAS_MOUNTS and the
// mounts setting.
func cacheNosuidProblems(err error, dir string) []string {
	if err == nil {
		return []string{"the cache folder is not refused"}
	}
	var out []string
	if !errors.Is(err, port.ErrRefused) || !errors.Is(err, trust.ErrMount) {
		out = append(out, "the refusal is not the disk's, refused with ErrMount: "+err.Error())
	}
	for _, want := range []string{dir + ", the cache folder scratch folders are made in, is not trusted", "mounted nosuid", hosttrust.MountFix} {
		if !strings.Contains(err.Error(), want) {
			out = append(out, "the refusal does not say "+strconv.Quote(want)+": "+err.Error())
		}
	}
	return out
}

// TestACacheFolderOnANosuidDiskIsRefusedByTheDiskOnEverySystem:
// the refusal TestACacheOnANosuidDiskIsRefusedUnlessItsMountIsNamed sees
// where trusttest.Bind can mount a nosuid disk, worded on every system from
// the walk's refusal of such a disk (host/trust's words for an error that
// wraps trust.ErrMount, TestTheWalksNosuidRefusalNamesTheMountAndItsFix):
// it names the cache folder and the mount, gives MountFix, and matches
// ErrMount and ErrRefused. Planted: the mount cut from the walk's words, red
// for the mount; the walk's move line in place of MountFix, red for the fix
// and for ErrMount. The control is the walk's error that is no refusal,
// answered as it is.
func TestACacheFolderOnANosuidDiskIsRefusedByTheDiskOnEverySystem(t *testing.T) {
	t.Parallel()
	dir := "/work/cache/gragas"
	walked := &hosttrust.Unsafe{Observed: dir + " is on a file system mounted nosuid", Fix: hosttrust.MountFix, Unjudged: true}
	if p := cacheNosuidProblems(sandbox.Untrusted(dir, walked), dir); len(p) != 0 {
		t.Errorf("a cache folder on a nosuid disk: %q", p)
	}
	for name, tc := range map[string]struct {
		walked *hosttrust.Unsafe
		says   []string
	}{
		"the mount cut": {&hosttrust.Unsafe{Observed: dir, Fix: hosttrust.MountFix, Unjudged: true}, []string{`say "mounted nosuid"`}},
		"the move line": {&hosttrust.Unsafe{Observed: walked.Observed, Fix: "move the cache folder to a folder of this account's, and point this account's cache folder there"},
			[]string{"refused with ErrMount", "say " + strconv.Quote(hosttrust.MountFix)}},
	} {
		p := cacheNosuidProblems(sandbox.Untrusted(dir, tc.walked), dir)
		for _, says := range tc.says {
			if !slices.ContainsFunc(p, func(l string) bool { return strings.Contains(l, says) }) {
				t.Errorf("the plant %s gives %q, want a problem saying %s", name, p, says)
			}
		}
	}
	failed := errors.New("the path of the cache folder could not be read")
	if err := sandbox.Untrusted(dir, failed); !errors.Is(err, failed) || err.Error() != failed.Error() || errors.Is(err, port.ErrRefused) {
		t.Errorf("the control, an error that is no refusal = %v; want it as it is", err)
	}
}

// TestABaseJudgedWithAMountGragasCannotTrustIsRefused: the
// base's mounts are judged by go-files as entries before it judges the base,
// as the walk judges them, so a mount that is a folder of
// this account's is refused by name, with the fix line, and the base is
// never judged with it. Planted: the entry's refusal answered as the disk's
// (unheld's MountFix line).
func TestABaseJudgedWithAMountGragasCannotTrustIsRefused(t *testing.T) {
	t.Parallel()
	dir := cache(t)
	at := filepath.Join(dir, "at")
	if err := os.Mkdir(at, 0o700); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(at)
	if err != nil {
		t.Fatal(err)
	}
	if err := sandbox.Owned(at, fi, nil); err != nil {
		t.Fatalf("the base with no mounts = %v; want it held", err)
	}
	err = sandbox.Owned(at, fi, []string{dir})
	if _, probe := hosttrust.Walk(hosttrust.Subject{Name: "the probe", Last: hosttrust.Folder}, dir, hosttrust.Who{Mounts: []string{"relative"}}); probe == nil {
		// go-files judges no Who.Mounts entry here, as on Windows, which reads
		// none: the mount passes unjudged (a skip needs an allow-list entry).
		t.Log("go-files judges no Who.Mounts entry on this system: the mount is held to pass unjudged")
		if err != nil {
			t.Errorf("the base with a mount go-files does not judge = %v; want it held", err)
		}
		return
	}
	if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "the mount "+dir+",") || !strings.Contains(err.Error(), hosttrust.MountEntryFix) || strings.Contains(err.Error(), hosttrust.MountFix) {
		t.Errorf("the base with a mount of this account's = %v; want it refused, naming the mount, with its fix", err)
	}
}

// TestEveryFolderAndFilePrepareMakesIsThisAccountsAlone: the
// cache's missing folders, made for the base, and the folders above a file
// placed in the scratch folder are each private to this account as they
// are made, through confine.Mkdir, and the file is its owner's alone to
// read and write. trusttest.Perm reads each as this system keeps it, the
// mode on Unix and the access list on Windows. Risk: a folder made at the
// umask's mode, in which another account may put a file of its own before
// the run starts. Planted: MkdirAll back where the umask lets the group in.
func TestEveryFolderAndFilePrepareMakesIsThisAccountsAlone(t *testing.T) {
	t.Parallel()
	top := cache(t)
	dir := filepath.Join(top, "missing", "cache")
	sc, err := sandbox.New(sandbox.Config{Dir: dir}).Prepare(t.Context(), model.Prepare{
		Level: model.Isolated, Fixtures: []model.File{{Path: "a/b/c.yaml", Data: []byte("c")}},
	})
	if err != nil {
		t.Fatalf("Prepare in a cache folder that is not there yet = %v; want it made", err)
	}
	run := filepath.FromSlash(sc.Dir)
	for _, d := range []string{filepath.Join(top, "missing"), dir, filepath.Join(dir, sandbox.Folder), filepath.Join(run, "a"), filepath.Join(run, "a", "b")} {
		trusttest.Perm(t, d, 0o700)
	}
	trusttest.Perm(t, filepath.Join(run, "a", "b", "c.yaml"), 0o600)
}

// TestTheCacheFolderAboveTheBaseIsWalkedFirst: the cache
// folder the base is made in, and the folders above it, are walked by
// host/trust before the base is held, so Prepare and Release are refused by
// the cache folder's name when another account may write it or a folder
// above it, with no run folder made and none removed. Risk: an account that
// may write the cache folder renames the judged base away and puts a folder
// of its own at the name, its config files in it, where the run starts
// (part a, every system). Who may write is changed by trusttest: the mode
// bits here, an allow entry for Users or Everyone on Windows (the Windows
// plant is the GroupWrites row there). A cache folder others may only read is the
// control. Part b, a junction swapped in at the base between its Lstat and
// its open, needs Windows and is not proved here; the walk this proves
// shuts it for any account the walk does not trust. Planted: the walk
// dropped (every loosened row prepares, red here).
func TestTheCacheFolderAboveTheBaseIsWalkedFirst(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what    string
		plant   func(t *testing.T, top string) (dir, loosened string)
		refused bool
	}{
		{"a cache folder its group may write", func(t *testing.T, top string) (string, string) {
			t.Helper()
			trusttest.Loosen(t, top, trusttest.GroupWrites)
			return top, top
		}, true},
		{"a cache folder in a folder others may write", func(t *testing.T, top string) (string, string) {
			t.Helper()
			dir := filepath.Join(top, "cache")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			trusttest.Loosen(t, top, trusttest.OthersWrite)
			return dir, top
		}, true},
		{"a cache folder reached through a link in a folder others may write", func(t *testing.T, top string) (string, string) {
			t.Helper()
			target, open := filepath.Join(top, "target"), filepath.Join(top, "open")
			for _, d := range []string{target, open} {
				if err := os.Mkdir(d, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			link := filepath.Join(open, "cache")
			testenv.Link(t, target, link)
			trusttest.Loosen(t, open, trusttest.OthersWrite)
			return link, open
		}, true},
		{"the control, a cache folder others may read", func(t *testing.T, top string) (string, string) {
			t.Helper()
			trusttest.Loosen(t, top, trusttest.OthersRead)
			return top, top
		}, false},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			top := cache(t)
			dir, loosened := tc.plant(t, top)
			s := sandbox.New(sandbox.Config{Dir: dir})
			sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
			if !tc.refused {
				if err != nil {
					t.Fatalf("%s: Prepare = %v; want it prepared", tc.what, err)
				}
				if err := s.Release(sc); err != nil {
					t.Errorf("%s: Release = %v", tc.what, err)
				}
				return
			}
			if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), dir+", the cache folder") || !strings.Contains(err.Error(), loosened) {
				t.Errorf("%s: Prepare = %+v, %v; want refused by the cache folder's name and %s's", tc.what, sc, err, loosened)
			}
			if left := runs(t, dir); len(left) != 0 {
				t.Errorf("%s: the base holds %v after a refused Prepare, want nothing", tc.what, left)
			}
		})
	}
}

// TestReleaseIsRefusedOnceTheCacheFolderIsNotTrusted: a run
// folder prepared while the cache folder was trusted is not removed, by
// name, once another account may write the cache folder: Release is
// refused, naming it, and the folder stays. Planted: Release reaching the
// base without the walk (the folder removed, red here).
func TestReleaseIsRefusedOnceTheCacheFolderIsNotTrusted(t *testing.T) {
	t.Parallel()
	dir := cache(t)
	s := sandbox.New(sandbox.Config{Dir: dir})
	sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	if err != nil {
		t.Fatal(err)
	}
	trusttest.Loosen(t, dir, trusttest.GroupWrites)
	if err := s.Release(sc); !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), dir+", the cache folder") {
		t.Errorf("Release in a cache folder its group may write = %v; want refused by its name", err)
	}
	if left := runs(t, dir); len(left) != 1 {
		t.Errorf("the base holds %v after a refused Release, want the one run folder", left)
	}
}
