package journeys

import (
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// journeyGit is the real git, handed to the binary as its setting through a
// stand-in in the test's own folder that starts it. The binary walks the path
// of its git program, and a folder there that its group can write and others
// cannot (Homebrew's, on a Mac) makes the binary ask the host's accounts who
// is in that group: on macOS a run of the real dscl, which times out under load
// and made a golden flake at the landing. The stand-in's path
// holds no such folder, so a golden compares gragas's output and not how fast
// the directory service answered. The adapter's own test, internal/host/accounts,
// keeps the real dscl.
func journeyGit(t *testing.T) string {
	t.Helper()
	// These journeys run the real git: testenv.GitPath's, which fails the
	// test on a host whose PATH holds none.
	real := configtest.Git(t)
	// Not t.TempDir: its name is the test's, and a subtest named for a flag,
	// "--apply=true", puts an = in the path, where the process library starts no program
	// ; filestest.Private's name holds no test's name. The
	// repository a journey runs in keeps the test's name, = and all: git runs
	// in such a folder.
	dir := filestest.Private(t)
	stand := filepath.Join(dir, "git")
	if strings.ContainsRune(stand, '=') {
		t.Fatalf("the stand-in for git is at %s, whose path holds =, where the process library starts no program; set TMPDIR to a folder whose path holds none", stand)
	}
	stand = configtest.Fake(t, dir, "git", configtest.Exec(real))
	if g := refusedFolder(stand); g != "" {
		// A failure: the golden would reach the host's directory service, and
		// a journey that can time out there is a known flake.
		t.Errorf("the stand-in for git sits below %s, which another account can write: the binary may ask the host's directory service about it; set TMPDIR to a folder only you can write", g)
	}
	return stand
}

// refusedFolder is the folder on the way down to path that another account can
// write, which the binary's trust walk refuses and names, or "". A folder a
// group's write bit opens on Unix, and a grant to a group such as Windows' Users
// that trusttest.Loosen makes, is refused the one way on every system, with
// trust.ErrOthersWrite named by the refusal's Path. Who asks for no group:
// Windows has none to ask, so the group bit is never the question, only that
// another account can write. It is asked of the walk itself (go-files' trust,
// which the binary uses). path must be absolute and clean.
func refusedFolder(path string) string {
	_, err := trust.Path(path, trust.File, trust.Who{})
	u, ok := errors.AsType[*trust.Unsafe](err)
	if !ok || !errors.Is(err, trust.ErrOthersWrite) {
		return ""
	}
	return u.Path
}

// TestTheCheckForAGroupFolderSeesOneAndFollowsALink: the check journeyGit
// reports a folder its group can write and others cannot by sees one, and
// through a link, as the binary's walk does.
// Risk: a check that sees nothing, so a stand-in below such a folder goes
// unreported and a golden reaches the real dscl.
func TestTheCheckForAGroupFolderSeesOneAndFollowsALink(t *testing.T) {
	t.Parallel()
	plant := filepath.Join(filestest.Private(t), "shared")
	if err := os.Mkdir(plant, 0o700); err != nil {
		t.Fatal(err)
	}
	trusttest.Loosen(t, plant, trusttest.GroupWrites)
	real, err := filepath.EvalSymlinks(plant)
	if err != nil {
		t.Fatal(err)
	}
	stand := configtest.Fake(t, plant, "git", configtest.Exec(real))
	if got := refusedFolder(stand); got != real {
		t.Errorf("a folder another account can write was not refused and named: got %q, want %q", got, real)
	}
	link := filepath.Join(filestest.Private(t), "link")
	testenv.Link(t, plant, link)
	if got := refusedFolder(filepath.Join(link, "git")); got != real {
		t.Errorf("a link to a folder another account can write was not followed: got %q, want %q", got, real)
	}
	if got := refusedFolder(configtest.Fake(t, filestest.Private(t), "git", configtest.Exec(real))); got != "" {
		t.Errorf("a private folder was refused as one another account can write: %q", got)
	}
}

// TestNoJourneyHandsTheBinaryAGitButThroughJourneyGit: no journeys file but
// trustedgit_test.go looks git up on the PATH, and none that writes a
// programs setting lacks journeyGit, so every git the binary is given is the
// stand-in whose path holds no folder only its group can write.
// Risk: a later journey hands the binary the host's own git, whose folder
// (Homebrew's) makes the walk ask the directory service, and the flake of
// flake returns with nothing red to show it.
func TestNoJourneyHandsTheBinaryAGitButThroughJourneyGit(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("the journeys' test files were not found: %v", err)
	}
	sets := regexp.MustCompile(`\bprograms"?\s*:`)
	for _, f := range files {
		if f == "trustedgit_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if strings.Contains(src, `exec.LookPath("git")`) {
			t.Errorf("%s looks git up itself: take journeyGit(t) instead", f)
		}
		if sets.MatchString(src) && !strings.Contains(src, "journeyGit(") {
			t.Errorf("%s writes a programs setting without journeyGit(t)", f)
		}
	}
}
