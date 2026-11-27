package trust

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/go-files/filestest"
)

// mountsHome is a folder of this account's for t that the walk passes, with
// no link in its name (filestest.Private), and the deepest folder above it
// that another account owns and the walk passes as the subject's own last
// folder: a folder only the system can make, which this account cannot
// change. go-files says which folder that is, and no name of one system's
// does: the root of the disk is that folder on one system and
// not on another, where any signed-in account may add a folder to it.
func mountsHome(t *testing.T) (mine, system string) {
	t.Helper()
	mine = filestest.Private(t)
	me, err := gotrust.Account()
	if err != nil {
		t.Fatal(err)
	}
	for dir := filepath.Dir(mine); ; dir = filepath.Dir(dir) {
		if owner, err := ownerOf(dir); err == nil && owner != me {
			if _, err := Walk(Subject{Name: "the system's folder", Last: Folder}, dir, Who{}); err == nil {
				return mine, dir
			}
		}
		if filepath.Dir(dir) == dir {
			t.Fatalf("no folder above %s is another account's that the walk passes", mine)
		}
	}
}

// ownerOf is the owner of the folder at dir, read on its handle.
func ownerOf(dir string) (string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return "", err
	}
	owner, err := gotrust.OwnerOf(f)
	return owner, errors.Join(err, f.Close())
}

// entriesJudged is whether go-files judges a Who.Mounts entry on this
// system: not on every one. It asks go-files itself, with an
// entry that is not absolute, which it refuses wherever it judges one, on a
// walk of mine, which passes with no mount, so only the entry decides. Where
// it judges none, the test holds that the mount is not judged and says so in
// its log, since a skip needs an allow-list entry (tests.admission.5).
func entriesJudged(t *testing.T, mine string) bool {
	t.Helper()
	if _, err := Walk(Subject{Name: "the probe", Last: Folder}, mine, Who{}); err != nil {
		t.Fatalf("the probe's walk of %s with no mount: %v", mine, err)
	}
	if _, err := Walk(Subject{Name: "the probe", Last: Folder}, mine, Who{Mounts: []string{"relative"}}); err == nil {
		t.Log("go-files judges no Who.Mounts entry on this system: the mounts are held to pass unjudged")
		return false
	}
	return true
}

// entryRefused is whether err is the walk's refusal of the mount m as an
// entry: an *Unsafe that names it, with MountEntryFix, never Unjudged.
func entryRefused(err error, m string) bool {
	u, ok := errors.AsType[*Unsafe](err)
	return ok && strings.HasPrefix(u.Observed, "the mount "+m) && u.Fix == MountEntryFix && !u.Unjudged
}

// TestAMountGoFilesRefusesIsNamedWithItsFix: go-files judges
// each Who.Mounts entry before the path, and refuses one
// below a name an account but root may change. The walk answers that
// refusal as the mount's, in the words of the setting, with the fix that
// names GRAGAS_MOUNTS and the mounts setting, and never as the disk's,
// though ErrMountEntry wraps ErrMount. With no mounts the walk passes. Which
// entries pass is go-files's to test; that one passes here is
// TestANameOnANosuidDiskIsTrustedOnlyWithItsMountNamed's, in a namespace.
// Planted: ErrMountEntry answered as ErrMount (MountFix, Unjudged).
func TestAMountGoFilesRefusesIsNamedWithItsFix(t *testing.T) {
	t.Parallel()
	mine, root := mountsHome(t)
	judged := entriesJudged(t, mine)
	git := Subject{Name: "git", File: "the settings file", Last: Folder}
	if _, err := Walk(git, root, Who{}); err != nil {
		t.Fatalf("%s with no mounts: %v", root, err)
	}
	cases := map[string]string{"a folder of this account's": mine, "a folder that is not there": filepath.Join(mine, "gone")}
	for name, m := range cases {
		if _, err := Walk(git, root, Who{Mounts: []string{m}}); judged && !entryRefused(err, m) || !judged && err != nil {
			t.Errorf("%s, %s: %#v; want an *Unsafe naming it, with MountEntryFix, where go-files judges entries (%v), else none", name, m, err, judged)
		}
	}
	for _, s := range []string{MountsVar, MountsKey} {
		if !strings.Contains(MountEntryFix, s) || !strings.Contains(MountFix, s) {
			t.Errorf("the fix lines %q and %q do not name %s", MountEntryFix, MountFix, s)
		}
	}
}

// TestAnEntryIsSaidInTheWordsOfTheSetting: go-files names a
// refused entry by what its field stands for, "the mounts named as trusted hold
// "/work/disk", a link, ..." (go-files v0.10.0), which no person set; the words
// are the mount's, "the mount /work/disk, a link, ...",
// and go-files's words whole where they start otherwise. Planted: the field
// name kept.
func TestAnEntryIsSaidInTheWordsOfTheSetting(t *testing.T) {
	t.Parallel()
	for observed, want := range map[string]string{
		`the mounts named as trusted hold "/work/disk", a link, which any account that can change it points at another disk`: "the mount /work/disk, a link, which any account that can change it points at another disk",
		"/work/disk is refused": "the mount /work/disk: /work/disk is refused",
	} {
		if got := mountEntry(&gotrust.Unsafe{Path: "/work/disk", Observed: observed}); got.Observed != want || got.Fix != MountEntryFix {
			t.Errorf("%q = %q, %q; want %q, MountEntryFix", observed, got.Observed, got.Fix, want)
		}
	}
}

// TestAMountAtALinkInAFolderOfThisAccountsIsRefused: a link to
// the root in a folder of this account's names the root's disk today and any
// disk the account points it at tomorrow, so the walk refuses on it by its
// own name, before it walks. Planted: the link followed to the root, which
// passes.
func TestAMountAtALinkInAFolderOfThisAccountsIsRefused(t *testing.T) {
	t.Parallel()
	mine, root := mountsHome(t)
	judged := entriesJudged(t, mine)
	link := filepath.Join(mine, "disk")
	testenv.Link(t, root, link)
	_, err := Walk(Subject{Name: "git", File: "the settings file", Last: Folder}, root, Who{Mounts: []string{link}})
	if judged && !entryRefused(err, link) || !judged && err != nil {
		t.Errorf("a walk with a link to %s as a mount = %#v; want the mount's refusal, before the walk, where go-files judges entries (%v), else none", root, err, judged)
	}
}

// TestANosuidRefusalNeverGetsTheChownFix: go-files' ErrMount
// wraps ErrNotOwners, and no owner is why, so a settings file on a disk
// mounted nosuid keeps go-files' words and is told to set GRAGAS_MOUNTS,
// which the key in that very file cannot do, and never to chown it: neither
// when the file is this account's, nor when go-files' words name another
// owner. Planted: ErrMount answered as another account's file, ErrNotOwners
// judged first.
func TestANosuidRefusalNeverGetsTheChownFix(t *testing.T) {
	t.Parallel()
	p := "/h/.config/gragas/settings.yaml"
	for _, observed := range []string{
		p + " is on a file system mounted nosuid",
		p + " is owned by uid:1001, not by uid:1000, on a file system mounted nosuid",
	} {
		refusal := &gotrust.Unsafe{Path: p, Observed: observed}
		err := owned(p, "uid:1000", fmt.Errorf("%w: %w", refusal, gotrust.ErrMount))
		o, ok := errors.AsType[*Owned](err)
		if !ok || o.Why != strings.TrimPrefix(observed, p+" ") || o.Fix != readMountFix || strings.Contains(o.Fix, "chown") || !errors.Is(err, gotrust.ErrMount) {
			t.Errorf("ErrMount on %q = %#v; want go-files' words, readMountFix and ErrMount kept", observed, err)
		}
	}
	if !strings.Contains(readMountFix, MountsVar) || !strings.Contains(readMountFix, MountsKey) || strings.Contains(readMountFix, "chown") {
		t.Errorf("readMountFix %q: want it to name %s, and why %s will not do, with no chown", readMountFix, MountsVar, MountsKey)
	}
}

// nosuidProblems is what the walk's refusal r of dir, on a disk mounted
// nosuid, gets wrong: it must name dir and the mount, carry MountFix, which
// names GRAGAS_MOUNTS and the mounts setting, and be unjudged.
func nosuidProblems(r *Unsafe, dir string) []string {
	var out []string
	if !strings.Contains(r.Observed, dir) || !strings.Contains(r.Observed, "mounted nosuid") {
		out = append(out, "the refusal does not name the mount: "+r.Observed)
	}
	if r.Fix != MountFix || !strings.Contains(r.Fix, MountsVar) || !strings.Contains(r.Fix, "the "+MountsKey+" setting") {
		out = append(out, "the fix is not the mount's, naming "+MountsVar+" and the "+MountsKey+" setting: "+r.Fix)
	}
	if !r.Unjudged {
		out = append(out, "the refusal is not unjudged")
	}
	return out
}

// TestTheWalksNosuidRefusalNamesTheMountAndItsFix: go-files
// gives ErrMount only on Linux, where trusttest.Bind can mount a nosuid disk
// and skips elsewhere, so the walk's words for it are proved here on every
// system from an error that wraps go-files' refusal beside ErrMount, as a
// subject that says its path and one that is bare. The refusal names the
// folder and the mount, and its fix is MountFix. Planted: the mount cut from
// the words, and the move line in place of MountFix, each red. The control
// is the same refusal without ErrMount beside it: no MountFix, not
// unjudged. A walk that judged go-files' Unsafe alone gave this refusal the
// move line.
func TestTheWalksNosuidRefusalNamesTheMountAndItsFix(t *testing.T) {
	t.Parallel()
	dir := "/work/cache/gragas"
	u := &gotrust.Unsafe{Path: dir, Observed: dir + " is on a file system mounted nosuid"}
	err := fmt.Errorf("%w: %w", u, gotrust.ErrMount)
	bare := Subject{Name: "the cache folder", File: "the cache folder", Last: Folder, Bare: true, Point: "point this account's cache folder there"}
	for _, s := range []Subject{git(), bare} {
		r := s.unsafe(dir, u, err)
		if p := nosuidProblems(r, dir); len(p) != 0 {
			t.Errorf("%s: %q", s.Name, p)
		}
		cut := *r
		cut.Observed = strings.ReplaceAll(cut.Observed, " is on a file system mounted nosuid", "")
		moved := *r
		moved.Fix = s.moveLine(u)
		for plant, says := range map[*Unsafe]string{&cut: "does not name the mount", &moved: "the fix is not the mount's"} {
			if p := nosuidProblems(plant, dir); !slices.ContainsFunc(p, func(l string) bool { return strings.Contains(l, says) }) {
				t.Errorf("%s: the plant %#v gives %q, want a problem saying %q", s.Name, plant, p, says)
			}
		}
		if c := s.unsafe(dir, u, u); c.Fix == MountFix || c.Unjudged {
			t.Errorf("%s: the control, no ErrMount beside the refusal = %#v; want no MountFix, judged", s.Name, c)
		}
	}
}

// TestASettingsReadWithAMountGragasCannotTrustIsRefused: a
// mount of the variable that go-files refuses as an entry refuses the read,
// naming the variable and the mount, with the variable's fix and never the
// disk's; the same file read with no mounts is read. Planted: ErrMountEntry
// answered as ErrMount (readMountFix).
func TestASettingsReadWithAMountGragasCannotTrustIsRefused(t *testing.T) {
	t.Parallel()
	mine, _ := mountsHome(t)
	file := filepath.Join(mine, "settings.yaml")
	if err := os.WriteFile(file, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadOwned(file, nil); err != nil || string(data) != "version: 1\n" {
		t.Fatalf("a read with no mounts = %q, %v; want the file", data, err)
	}
	data, err := ReadOwned(file, []string{mine})
	if !entriesJudged(t, mine) {
		if err != nil || string(data) != "version: 1\n" {
			t.Errorf("a read with a mount go-files does not judge = %q, %v; want the file", data, err)
		}
		return
	}
	o, ok := errors.AsType[*Owned](err)
	if !ok || !strings.Contains(o.Why, MountsVar) || !strings.Contains(o.Why, "the mount "+mine) || o.Fix != readMountEntryFix || !errors.Is(err, gotrust.ErrMountEntry) {
		t.Errorf("a read with a mount of this account's = %#v; want refused, naming %s and the mount, with readMountEntryFix", err, MountsVar)
	}
}
