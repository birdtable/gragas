package trust

import (
	"errors"
	"fmt"
	"github.com/birdtable/go-files/filestest"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// git is a subject as the git adapter names one.
func git() Subject {
	return Subject{Name: "git", File: "git", Last: Program, Point: "set programs.git to its path there"}
}

// TestWhatTheWalkCannotFindIsTheReadsOwnErrorNeverUnsafe: a
// path with nothing at its end answers the read's error, which matches
// fs.ErrNotExist, so a caller tries the next place; a path that is not
// absolute and clean is refused before any name is read. Planted: the
// missing name answered as *Unsafe or as *Failed; the relative path walked.
func TestWhatTheWalkCannotFindIsTheReadsOwnErrorNeverUnsafe(t *testing.T) {
	t.Parallel()
	gone := filepath.Join(t.TempDir(), "git")
	_, err := Walk(git(), gone, Who{})
	var u *Unsafe
	var f *Failed
	if !errors.Is(err, fs.ErrNotExist) || errors.As(err, &u) || errors.As(err, &f) {
		t.Errorf("Walk(%s) = %v: want the read's own error, matching fs.ErrNotExist", gone, err)
	}
	for _, rel := range []string{"git", filepath.Join("bin", "..", "git")} {
		if _, err := Walk(git(), rel, Who{}); !errors.As(err, &u) {
			t.Errorf("Walk(%q) = %v: want *Unsafe before any read", rel, err)
		}
	}
}

// TestOnlyTheAdminGroupGoFilesNamesMayWrite: the group
// go-files's AdminGroup names may write a name, and no other; where it names
// none no group may; a lookup that fails is the error, never a guess.
// Planted: any group accepted; a failed lookup answered as none.
func TestOnlyTheAdminGroupGoFilesNamesMayWrite(t *testing.T) {
	t.Parallel()
	none, err := groupFrom(func() (uint32, bool, error) { return 0, false, nil })
	if none != nil || err != nil {
		t.Errorf("ok false: got a group, %v; want none", err)
	}
	g, err := groupFrom(func() (uint32, bool, error) { return 80, true, nil })
	if err != nil || g == nil || !g(80) || g(20) || g(0) {
		t.Errorf("gid 80: want group 80 alone, got err %v", err)
	}
	boom := errors.New("lookup failed")
	g, err = groupFrom(func() (uint32, bool, error) { return 80, true, boom })
	if g != nil || !errors.Is(err, boom) {
		t.Errorf("a failed lookup: got a group, %v; want the error", err)
	}
}

// TestQuoteIsOneWordForAShell: a quote in s is closed, escaped and opened
// again, and an empty s is the empty word. Planted: the quote left as it is.
func TestQuoteIsOneWordForAShell(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"/repo/bin/git": "'/repo/bin/git'", "it's": `'it'\''s'`} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
	if got := Quote(""); got != "''" {
		t.Errorf("Quote(\"\") = %s, want ''", got)
	}
}

// TestARefusalForAnotherOwnerSaysHowToHandItOver: a file Windows'
// Administrators group owns, as one made from an elevated shell is, stays
// refused, and the refusal says whose it is and gives the icacls line, run
// elevated, that hands it to this account; a Unix account's is chown. Risk: a
// refusal a person cannot act on, so the settings are moved away, or the check
// is wanted off. Planted: the accounts and the owner, so each system's words
// are checked on every system.
func TestARefusalForAnotherOwnerSaysHowToHandItOver(t *testing.T) {
	t.Parallel()
	const me = "S-1-5-21-1-2-3-1001"
	if got, want := notMine("is owned by "+administrators, me), "is owned by S-1-5-32-544, the Administrators group, as a file made from an elevated shell is, not by this account or root; only an elevated shell can give it to this account"; got != want {
		t.Errorf("Administrators' file: %q, want %q", got, want)
	}
	if got, want := notMine("is owned by uid:0", "uid:1000"), "is owned by uid:0, not by this account or root"; got != want {
		t.Errorf("a Unix file: %q, want %q", got, want)
	}
}

// TestAMountItCannotJudgeIsNotFixedByAnotherFolder: a name on
// a mount whose owners go-files does not judge, as a disk mounted nosuid or
// noexec alone, gets no fix of go-files's own.
// Its line says the mount is why and names GRAGAS_MOUNTS and the mounts
// setting, which let such a disk be judged, and never moves the file to
// another folder of this account's, which is on the same disk and refused
// too. Any other refusal with no fix keeps the line that moves it.
func TestAMountItCannotJudgeIsNotFixedByAnotherFolder(t *testing.T) {
	t.Parallel()
	mount := fmt.Errorf("/repo/bin/git is on a file system a disk backs: %w", gotrust.ErrMount)
	line := git().moveLine(mount)
	for _, want := range []string{"the way the disk under it is mounted is why", MountsVar, "the " + MountsKey + " setting"} {
		if !strings.Contains(line, want) {
			t.Errorf("the fix of a name on a mount not judged is %q, want it to hold %q", line, want)
		}
	}
	if strings.Contains(line, "move git") {
		t.Errorf("the fix of a name on a mount not judged is %q, which moves git to another folder on the same disk", line)
	}
	if got, want := git().moveLine(errors.New("in a sticky folder")), "move git to a folder of this account's, and set programs.git to its path there"; got != want {
		t.Errorf("the fix of another refusal with none of go-files's own is %q, want %q", got, want)
	}
	if got := (Subject{Name: "the key"}).moveLine(errors.New("x")); got != "" {
		t.Errorf("the fix for a subject with no setting is %q, want none", got)
	}
}

// program plants an executable file in a folder of its own only this
// account can write, the walk-trusted private folder of configtest, letting
// in whom who names (trusttest.Loosen), and answers its path with no link
// in it.
func program(t *testing.T, who ...trusttest.Who) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(filestest.Private(t))
	if err != nil {
		t.Fatal(err)
	}
	p := configtest.Fake(t, dir, "git")
	for _, w := range who {
		trusttest.Loosen(t, p, w)
	}
	return p
}

// refused is the *Unsafe Walk answers for p, or the test's failure.
func refused(t *testing.T, s Subject, p string, who Who) *Unsafe {
	t.Helper()
	_, err := Walk(s, p, who)
	var u *Unsafe
	if !errors.As(err, &u) {
		t.Fatalf("Walk(%s) = %v: want *Unsafe", p, err)
	}
	return u
}

// raw is go-files' own refusal of p walked for last, as Walk asks it with
// no Who of the caller's, or the test's failure.
func raw(t *testing.T, p string, last Kind) *gotrust.Unsafe {
	t.Helper()
	admin, err := group()
	if err != nil {
		t.Fatal(err)
	}
	_, err = walk(p, last, gotrust.Who{Group: admin})
	u, ok := errors.AsType[*gotrust.Unsafe](err)
	if !ok {
		t.Fatalf("go-files' walk of %s = %v: want its refusal", p, err)
	}
	return u
}

// fixedAsGoFiles fails t unless fix, the line Walk gave, holds every word of
// the commands go-files' refusal u gives on this system: the system's fix,
// and no one system's words. A refusal go-files gives no
// command for fails it too.
func fixedAsGoFiles(t *testing.T, fix string, u *gotrust.Unsafe) {
	t.Helper()
	steps, ok := u.Fix.Commands()
	if !ok || len(steps) == 0 {
		t.Fatalf("go-files gives no command for %q: %+v", u.Observed, u.Fix)
	}
	for _, s := range steps {
		for _, w := range s.Argv {
			if !strings.Contains(fix, w) {
				t.Errorf("fix %q leaves out %q of go-files' command %q", fix, w, s.Argv)
			}
		}
	}
}

// TestARealPlantIsRefusedInItsSubjectsWords (trust-walk):
// real files, each refused by go-files's walk, in the words of the subject
// that asked: its name and path before what the walk saw, the walk's own fix
// when it has one, and the line that moves the file when it has none. Plants
// (trusttest): a program others can write; a folder its group
// can write above it; a folder where the program should be, which no fix
// but a move mends; this account's own program walked as root's alone, the
// one name a test can make that an untrusted account owns. The words and
// the fix are go-files' own for the plant on this system.
// Planted failures: Owned set for a name its group can write, or for one
// whose fix is a move; the move line given where go-files has a fix; the
// subject's name left off; Bare ignored; the path said twice when the walk
// judged the name itself, or once when it judged a folder on the way.
func TestARealPlantIsRefusedInItsSubjectsWords(t *testing.T) {
	t.Parallel()
	t.Run("a program others can write", func(t *testing.T) {
		t.Parallel()
		p := program(t, trusttest.OthersWrite)
		u, g := refused(t, git(), p, Who{}), raw(t, p, Program)
		if !strings.HasPrefix(g.Observed, p+" ") || u.Observed != "git at "+g.Observed || u.Owned {
			t.Errorf("%+v: want git at its path, said once, in go-files' words %q, not owned", u, g.Observed)
		}
		fixedAsGoFiles(t, u.Fix, g)
	})
	t.Run("a folder its group can write above it", func(t *testing.T) {
		t.Parallel()
		p := program(t)
		trusttest.Loosen(t, filepath.Dir(p), trusttest.GroupWrites)
		u, g := refused(t, git(), p, Who{}), raw(t, p, Program)
		if !strings.HasPrefix(g.Observed, filepath.Dir(p)+" ") || u.Observed != "git at "+p+": "+g.Observed || u.Owned {
			t.Errorf("%+v: want the folder named after git's path, in go-files' words %q, not owned", u, g.Observed)
		}
		fixedAsGoFiles(t, u.Fix, g)
		bare := git()
		bare.Bare = true
		if b := refused(t, bare, p, Who{}); b.Observed != g.Observed {
			t.Errorf("bare: %q: want what the walk saw alone, %q", b.Observed, g.Observed)
		}
	})
	t.Run("a folder where the program should be", func(t *testing.T) {
		t.Parallel()
		p := filepath.Dir(program(t))
		u, g := refused(t, git(), p, Who{}), raw(t, p, Program)
		want := "move git to a folder of this account's, and set programs.git to its path there"
		if g.Fix != (gotrust.Fix{}) || !strings.HasPrefix(u.Observed, "git at "+p+" ") || u.Fix != want || u.Owned {
			t.Errorf("%+v (go-files' fix %+v): want git at the folder, no fix of go-files', the move line, not owned", u, g.Fix)
		}
	})
	t.Run("this account's program walked as root's alone", func(t *testing.T) {
		t.Parallel()
		p := program(t)
		// With only root trusted, the walk refuses the first name root does not
		// own: the program where root owns every folder above it (a temp below a
		// root-owned /tmp), or a folder on the way where it does not (a temp below
		// a home or macOS's per-account folder, which only an elevated prompt
		// could make root's). Which it is depends on the machine, so the test
		// asks go-files' own root-only walk of the same path what it refuses, and
		// holds the product's refusal and fix line to it, as the subtests above
		// do with raw. No name of a system is read.
		u := refused(t, git(), p, Who{RootOnly: true})
		_, err := walk(p, Program, gotrust.Who{RootOnly: true})
		g, ok := errors.AsType[*gotrust.Unsafe](err)
		if !ok {
			t.Fatalf("go-files' root-only walk of %s = %v: want its refusal", p, err)
		}
		if !strings.HasSuffix(u.Observed, g.Observed) {
			t.Errorf("%+v: want go-files' words %q at the end", u, g.Observed)
		}
		fixedAsGoFiles(t, u.Fix, g)
	})
}

// TestAPlantTheWalkTrustsIsAnsweredWithNoLinkLeft: this account's program,
// reached through a link in a folder of its own, is answered at the file the
// link ends at; NoLinks refuses the same link. Planted: the link answered as
// it is; NoLinks dropped on the way to go-files.
func TestAPlantTheWalkTrustsIsAnsweredWithNoLinkLeft(t *testing.T) {
	t.Parallel()
	p := program(t)
	link := filepath.Join(filepath.Dir(p), "git-link")
	testenv.Link(t, p, link)
	if got, err := Walk(git(), link, Who{}); err != nil || got != p {
		t.Errorf("Walk(%s) = %q, %v: want %s", link, got, err, p)
	}
	refused(t, git(), link, Who{NoLinks: true})
}

// TestTheAdminGroupsFolderPasses: a folder of this account's
// that go-files' admin group owns and may write passes as a folder, and
// go-files' walk with no group refuses it. Where go-files names no admin
// group, or this account cannot give a folder to it, there is nothing to
// plant, and groupFrom's own test covers the rest. Planted: Walk given no
// group.
func TestTheAdminGroupsFolderPasses(t *testing.T) {
	t.Parallel()
	gid, ok, err := gotrust.AdminGroup()
	if err != nil || !ok {
		return // go-files names no admin group here; groupFrom's own test covers the rest
	}
	dir := filepath.Join(filestest.Private(t), "shared")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	trusttest.GiveGroup(t, dir, gid) // skips the test where this account cannot give it
	trusttest.Loosen(t, dir, trusttest.GroupWrites)
	if _, err := Walk(Subject{Name: "the folder", Last: Folder}, dir, Who{}); err != nil {
		t.Errorf("Walk(%s), the admin group's to write = %v: want a pass", dir, err)
	}
	if _, err := walk(dir, Folder, gotrust.Who{}); err == nil {
		t.Errorf("go-files' walk of %s with no group passed: the plant proves nothing", dir)
	}
}

// TestAFolderKeptApartIsRefusedByItsNameOnTheWay: a folder
// walked with Who.Apart naming a folder on its way, spelled through a link
// to it, is refused, and Inside names that folder as the walk reached it;
// the same folder walked with nothing apart passes. Planted: Apart dropped
// on the way to go-files; Inside left empty, or set on another refusal.
func TestAFolderKeptApartIsRefusedByItsNameOnTheWay(t *testing.T) {
	t.Parallel()
	dir, err := filepath.EvalSymlinks(filestest.Private(t))
	if err != nil {
		t.Fatal(err)
	}
	proj, other := filepath.Join(dir, "proj"), filepath.Join(dir, "other")
	below := filepath.Join(proj, "sub", "home")
	for _, d := range []string{below, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(other, "proj-link")
	testenv.Link(t, proj, link)
	s := Subject{Name: "the home", File: "the home", Last: Folder, Bare: true}
	if _, err := Walk(s, below, Who{NoLinks: true}); err != nil {
		t.Fatalf("Walk(%s) with nothing apart = %v: want a pass", below, err)
	}
	if u := refused(t, s, below, Who{NoLinks: true, Apart: []string{link}}); u.Inside != proj {
		t.Errorf("%+v: want Inside %s, the folder on the way that is the one kept apart", u, proj)
	}
	if u := refused(t, s, link, Who{NoLinks: true}); u.Inside != "" {
		t.Errorf("%+v: a link refused, want no Inside", u)
	}
}
