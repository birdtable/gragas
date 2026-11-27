package git_test

import (
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// TestMain answers as a stand-in for git when this binary is one (see
// git.StandIn), and runs the tests otherwise.
func TestMain(m *testing.M) {
	if code, ok := git.StandInMain(); ok {
		os.Exit(code)
	}
	os.Exit(configtest.Main(m))
}

// pathRow is one row of a path table in testdata: a path, and what it is.
type pathRow struct{ path, what string }

// pathTable is the rows of testdata/name: on each line a path as a Go string
// literal, then what it is; a blank line, or one starting #, is none. The
// paths a check is fed are data there, some spelled as one system's paths,
// and not code.
func pathTable(t *testing.T, name string) []pathRow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var rows []pathRow
	for i, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		q, err := strconv.QuotedPrefix(line)
		if err != nil {
			t.Fatalf("testdata/%s:%d: %v", name, i+1, err)
		}
		p, err := strconv.Unquote(q)
		what := strings.TrimSpace(line[len(q):])
		if err != nil || what == "" {
			t.Fatalf("testdata/%s:%d: want a quoted path, then what it is", name, i+1)
		}
		rows = append(rows, pathRow{p, what})
	}
	if len(rows) == 0 {
		t.Fatalf("testdata/%s holds no paths", name)
	}
	return rows
}

// fixtureEnv is the whole environment of a git the test runs for its own
// set-up: the account's git config is home's .gitconfig, the file a
// product run with home as its home reads, and the machine's is not read.
func fixtureEnv(home string) []string {
	return []string{"GIT_CONFIG_GLOBAL=" + filepath.Join(home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM=1"}
}

// gitIn runs real git in dir for the test's own set-up, with fixtureEnv's
// config of home and nothing else of this process's git.
func gitIn(t *testing.T, home, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git")
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = dir
	cmd.Env = fixtureEnv(home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// fixture is a home of its own and a repository in it, with who as the
// repository's identity, or none when who is nil.
func fixture(t *testing.T, who *model.Identity) (home string, root model.Root) {
	t.Helper()
	base := filestest.Private(t)
	home, repo := filepath.Join(base, "home"), filepath.Join(base, "book")
	for _, d := range []string{home, repo} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, home, repo, "init", "-q")
	if who != nil {
		gitIn(t, home, repo, "config", "user.name", who.Name)
		gitIn(t, home, repo, "config", "user.email", who.Email)
	}
	return home, model.Root(repo)
}

// adapter is the adapter at the machine's git, for home, in run "run-1".
func adapter(t *testing.T, gitPath, home string) *git.Git {
	t.Helper()
	g, err := git.New(git.Config{Git: gitPath, Home: home, Run: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestTheGitAdapterPassesTheWhoSuite(t *testing.T) {
	t.Parallel()
	contracttest.Who(t, func(t *testing.T, who *model.Identity) (contracttest.Identifier, model.Root) {
		t.Helper()
		home, root := fixture(t, who)
		return adapter(t, configtest.Git(t), home), root
	})
}

// TestTheAccountsOwnIdentityIsRead: with none in the repository, the identity
// of the account's own config (HOME) is the one answered; the repository's
// own wins over it. Planted failure: HOME not handed to git.
func TestTheAccountsOwnIdentityIsRead(t *testing.T) {
	t.Parallel()
	home, root := fixture(t, nil)
	gitIn(t, home, home, "config", "--global", "user.name", "Ada Example")
	gitIn(t, home, home, "config", "--global", "user.email", "ada@example.com")
	g := adapter(t, configtest.Git(t), home)
	if got, err := g.Who(t.Context(), root); err != nil || got != (model.Identity{Name: "Ada Example", Email: "ada@example.com"}) {
		t.Errorf("the account's identity: %+v, %v", got, err)
	}
	gitIn(t, home, string(root), "config", "user.name", "Grace Example")
	if got, err := g.Who(t.Context(), root); err != nil || got != (model.Identity{Name: "Grace Example", Email: "ada@example.com"}) {
		t.Errorf("the repository's own over the account's: %+v, %v", got, err)
	}
}

// TestEachRunIsOneRecordedCallOfTheTable: one Who is one run of git, by the
// absolute path, with the table's argv, recorded. Planted failure: a second
// call to read the email.
func TestEachRunIsOneRecordedCallOfTheTable(t *testing.T) {
	t.Parallel()
	who := contracttest.TheIdentity()
	home, root := fixture(t, &who)
	path := configtest.Git(t)
	g := adapter(t, path, home)
	if _, err := g.Who(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	ran := g.Ran()
	if len(ran) != 1 {
		t.Fatalf("one Who ran git %d times: %s", len(ran), git.Shown(ran))
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{resolved, "--no-replace-objects", "-c", "core.commitGraph=false", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "advice.graftFileDeprecated=false", "config", "-z", "--get-regexp", `^user\.(name|email)$`}
	if r := ran[0]; r.Verb != "config-identity" || strings.Join(r.Argv, " ") != strings.Join(want, " ") || r.Dir != string(root) || r.Code != 0 {
		t.Errorf("the run: %s, want verb config-identity, argv %q, dir %s, code 0", git.Shown([]git.Ran{r}), want, root)
	}
}

// mallory is the rule of a git that prints an identity no fixture has.
func mallory() git.Rule {
	return git.Always(git.Answer{Out: "user.name\nMallory Example\x00user.email\nmallory@example.com\x00"})
}

// fakeGit writes a program named git into a folder of its own that prints
// an identity no fixture has, and answers the folder and the program.
func fakeGit(t *testing.T, who trusttest.Who) (dir, path string) {
	t.Helper()
	dir = filepath.Join(filestest.Private(t), "bin")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path = git.StandIn(t, dir, mallory())
	if who != 0 {
		trusttest.Loosen(t, path, who)
	}
	return dir, path
}

// linkTo makes a link in dir to the program target, named as it is, so the
// link's name runs on every system as the program's does.
func linkTo(t *testing.T, target, dir string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.Base(target))
	testenv.Link(t, target, p)
	return p
}

// TestAGitInPathOnlyIsNotUsed: a git that PATH finds first is never run.
// With no setting, Who is not ready naming programs.git and runs nothing;
// with the setting, the set git is run and PATH's is not. Planted failure:
// a fallback to exec.LookPath.
func TestAGitInPathOnlyIsNotUsed(t *testing.T) {
	who := contracttest.TheIdentity()
	home, root := fixture(t, &who)
	dir, fake := fakeGit(t, 0)
	real := configtest.Git(t)
	t.Setenv("PATH", dir)
	// the fake is what a lookup by name finds first, on every system, so
	// the checks below mean something: a stand-in named a bare git is no
	// program where a program's name needs a suffix
	if found, err := exec.LookPath("git"); err != nil || found != fake {
		t.Fatalf("PATH's git is %q, %v; want the stand-in %s", found, err, fake)
	}

	g := adapter(t, "", home)
	_, err := g.Who(t.Context(), root)
	var nr *git.NotReady
	if !errors.As(err, &nr) || nr.Word() != model.NotReady || !strings.Contains(nr.Observed(), "programs.git") || !strings.Contains(nr.Hint(), "programs.git") {
		t.Errorf("no setting, git in PATH: %v, want not ready naming programs.git", err)
	}
	if len(g.Ran()) != 0 {
		t.Errorf("no setting, and git ran: %s", git.Shown(g.Ran()))
	}

	g = adapter(t, real, home)
	if got, err := g.Who(t.Context(), root); err != nil || got != who {
		t.Errorf("the set git, with another first in PATH: %+v, %v", got, err)
	}
}

// TestAMissingGitIsNotReadyNamingTheSetting: a setting that is not
// absolute, or where no file is, is not ready naming programs.git, as it is
// and not inside a failed, and nothing runs. Planted failures: the missing
// file answered as failed; the not ready wrapped in a failed.
func TestAMissingGitIsNotReadyNamingTheSetting(t *testing.T) {
	t.Parallel()
	home, root := fixture(t, nil)
	cases := []string{"git", "bin/git", filepath.Join(filestest.Private(t), "nowhere", "git"), "/work/scratch/../scratch/git"}
	for _, set := range cases {
		g := adapter(t, set, home)
		_, err := g.Who(t.Context(), root)
		var nr *git.NotReady
		if !errors.As(err, &nr) || err.Error() != nr.Error() || nr.Word() != model.NotReady || !strings.Contains(nr.Observed(), "programs.git") {
			t.Errorf("programs.git %q: %v, want not ready as it is, naming programs.git", set, err)
		}
		if len(g.Ran()) != 0 {
			t.Errorf("programs.git %q ran git", set)
		}
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
}

// TestAGitOthersCanWriteIsRefused: git at a path its group or others
// can write, in a folder they can write, reached through a link to one, or
// that is no executable regular file, is refused with the line that fixes
// it, and nothing runs. The controls, the same program with mode 0700 in a
// folder of 0700 and a link to it, run. Planted failures: the mode check
// gone; links not resolved, so a link to a safe git is refused.
func TestAGitOthersCanWriteIsRefused(t *testing.T) {
	t.Parallel()
	who := contracttest.TheIdentity()
	home, root := fixture(t, &who)
	cases := map[string]func(t *testing.T) string{
		"writable by others":    func(t *testing.T) string { t.Helper(); _, p := fakeGit(t, trusttest.OthersWrite); return p },
		"writable by its group": func(t *testing.T) string { t.Helper(); _, p := fakeGit(t, trusttest.GroupWrites); return p },
		"not executable": func(t *testing.T) string {
			t.Helper()
			// a system that keeps no mode bits keeps no execute bit to take off
			testenv.Modes(t)
			_, p := fakeGit(t, 0)
			noExec(t, p)
			return p
		},
		"in a folder others can write": func(t *testing.T) string {
			t.Helper()
			d, p := fakeGit(t, 0)
			trusttest.Loosen(t, d, trusttest.OthersWrite)
			return p
		},
		"a link to one others can write": func(t *testing.T) string {
			t.Helper()
			_, p := fakeGit(t, trusttest.OthersWrite)
			return linkTo(t, p, filestest.Private(t))
		},
		"a folder": func(t *testing.T) string { t.Helper(); d, _ := fakeGit(t, 0); return d },
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			g := adapter(t, plant(t), home)
			_, err := g.Who(t.Context(), root)
			var rf *git.Refused
			if !errors.As(err, &rf) || err.Error() != rf.Error() || rf.Word() != model.Refused || rf.Hint() == "" {
				t.Errorf("git %s: %v, want refused as it is, with a fix", name, err)
			}
			if len(g.Ran()) != 0 {
				t.Errorf("git %s ran", name)
			}
		})
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	_, ok := fakeGit(t, 0)
	if got, err := adapter(t, ok, home).Who(t.Context(), root); err != nil || got.Name != "Mallory Example" {
		t.Errorf("the control, a program of 0700 in a folder of 0700: %+v, %v", got, err)
	}
	// a link, as /usr/bin/git often is, runs what it names when that passes
	link := linkTo(t, ok, filestest.Private(t))
	if got, err := adapter(t, link, home).Who(t.Context(), root); err != nil || got.Name != "Mallory Example" {
		t.Errorf("the control, a link to a program of 0700: %+v, %v", got, err)
	}
}

// noExec takes every execute bit from the program at p.
func noExec(t *testing.T, p string) {
	t.Helper()
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
}

// mkdir makes the folder path 0700, whatever the umask.
func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// link makes a link at path to target.
func link(t *testing.T, target, path string) {
	t.Helper()
	testenv.Link(t, target, path)
}

// TestAWayOthersCanWriteIsRefused: the walk from / judges every name
// on the way to git, not only the file it resolves to. Each of these is
// refused, as it is, naming the name that fails, and runs nothing:
//   - the review's demo: a link in a folder others can write, to /bin/sh,
//     which is root's in root's folder, with a script at the root's
//     ./config that would answer another identity;
//   - a link in a safe folder to a git in a folder others can write;
//   - a safe git below a folder above it that its group can write, when the
//     group holds an account other than root and this one;
//   - a link that names itself.
//
// The controls run at the resolved path: a relative link with .. to a safe
// git, and a link in the middle of the path, as /bin to usr/bin. Planted
// failures: the folders on the way not judged; a link's target folder not
// judged; the link count gone; the names after a link dropped.
func TestAWayOthersCanWriteIsRefused(t *testing.T) {
	t.Parallel()
	who := contracttest.TheIdentity()
	home, root := fixture(t, &who)
	script := "printf 'user.name\\nMallory Example\\000user.email\\nm@example.com\\000'\n"
	if err := os.WriteFile(filepath.Join(string(root), "config"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		plant func(t *testing.T) string
		says  string
	}{
		{"a link to a trusted git in a folder others can write", func(t *testing.T) string {
			t.Helper()
			shared := filepath.Join(filestest.Private(t), "shared")
			mkdir(t, shared)
			trusttest.Loosen(t, shared, trusttest.GroupWrites)
			trusttest.Loosen(t, shared, trusttest.OthersWrite)
			link(t, configtest.Git(t), filepath.Join(shared, "git"))
			return filepath.Join(shared, "git")
		}, ""},
		{"a link to a git in a folder others can write", func(t *testing.T) string {
			t.Helper()
			base := filestest.Private(t)
			open, safe := filepath.Join(base, "open"), filepath.Join(base, "safe")
			mkdir(t, open)
			mkdir(t, safe)
			p := git.StandIn(t, open, mallory())
			trusttest.Loosen(t, open, trusttest.GroupWrites)
			trusttest.Loosen(t, open, trusttest.OthersWrite)
			return linkTo(t, p, safe)
		}, ""},
		{"a safe git below a folder its group, with another account, can write", func(t *testing.T) string {
			t.Helper()
			open := filepath.Join(filestest.Private(t), "open")
			mkdir(t, open)
			trusttest.Loosen(t, open, trusttest.GroupWrites)
			mkdir(t, filepath.Join(open, "bin"))
			return git.StandIn(t, filepath.Join(open, "bin"), mallory())
		}, ""},
		{"a link that names itself", func(t *testing.T) string {
			t.Helper()
			p := filepath.Join(filestest.Private(t), "git")
			link(t, "git", p)
			return p
		}, "passes more than 40 links"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := c.plant(t)
			says := c.says
			if says == "" {
				says = git.Walked(t, p, git.WalkGit)
			}
			g := adapter(t, p, home)
			got, err := g.Who(t.Context(), root)
			var rf *git.Refused
			if !errors.As(err, &rf) || err.Error() != rf.Error() || !strings.Contains(rf.Observed(), says) || rf.Hint() == "" {
				t.Errorf("%s: %+v, %v, want refused as it is, saying %q", c.name, got, err, says)
			}
			if len(g.Ran()) != 0 {
				t.Errorf("%s ran %s", c.name, git.Shown(g.Ran()))
			}
		})
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	base := filestest.Private(t)
	a, b := filepath.Join(base, "a"), filepath.Join(base, "b")
	mkdir(t, a)
	mkdir(t, b)
	p := git.StandIn(t, b, mallory())
	name := filepath.Base(p)
	link(t, filepath.Join("..", "b", name), filepath.Join(a, name))
	g := adapter(t, filepath.Join(a, name), home)
	if got, err := g.Who(t.Context(), root); err != nil || got.Name != "Mallory Example" {
		t.Errorf("the control, a relative link to a safe git: %+v, %v", got, err)
	}
	if want := resolved(t, p); len(g.Ran()) != 1 || g.Ran()[0].Argv[0] != want {
		t.Errorf("the control did not run the resolved path %s: %s", want, git.Shown(g.Ran()))
	}
	// the second control, a link in the middle of the path, as /bin to
	// usr/bin: the setting <link>/git runs at the resolved path
	base = filestest.Private(t)
	usr := filepath.Join(base, "usr")
	mkdir(t, usr)
	p = git.StandIn(t, usr, mallory())
	link(t, "usr", filepath.Join(base, "bin"))
	g = adapter(t, filepath.Join(base, "bin", filepath.Base(p)), home)
	if got, err := g.Who(t.Context(), root); err != nil || got.Name != "Mallory Example" {
		t.Errorf("the control, a link in the middle of the path: %+v, %v", got, err)
	}
	if want := resolved(t, p); len(g.Ran()) != 1 || g.Ran()[0].Argv[0] != want {
		t.Errorf("the control did not run the resolved path %s: %s", want, git.Shown(g.Ran()))
	}
}

// resolved is the fixture's path p with every link in it resolved, read by
// the test from the fixture and not by the code under test: a temp folder
// can sit under a link the system made, as macOS's /var to /private/var.
func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestNoIdentityIsNotReadyNamingGitConfig: none configured, or one half
// missing, is a typed not ready naming git config and what is missing.
// Planted failure: an empty identity answered for none.
func TestNoIdentityIsNotReadyNamingGitConfig(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		set     []string
		missing string
	}{
		"none":          {nil, "user.name and user.email"},
		"no email":      {[]string{"user.name", "Ada Example"}, "user.email"},
		"no name":       {[]string{"user.email", "ada@example.com"}, "user.name"},
		"an empty name": {[]string{"user.name", "", "user.email", "ada@example.com"}, "user.name"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			home, root := fixture(t, nil)
			for i := 0; i+1 < len(c.set); i += 2 {
				gitIn(t, home, string(root), "config", c.set[i], c.set[i+1])
			}
			got, err := adapter(t, configtest.Git(t), home).Who(t.Context(), root)
			var nr *git.NotReady
			if !errors.As(err, &nr) || nr.Word() != model.NotReady {
				t.Fatalf("%s: %+v, %v, want not ready", name, got, err)
			}
			if !strings.Contains(nr.Observed(), "git config sets no "+c.missing) || !strings.Contains(nr.Hint(), "git config") {
				t.Errorf("%s: %q / %q, want it to name git config and %s", name, nr.Observed(), nr.Hint(), c.missing)
			}
			if got != (model.Identity{}) {
				t.Errorf("%s: an identity beside the error: %+v", name, got)
			}
		})
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
}

// TestTheNameIsStrippedOfControlSequences (cli.errors.4): a name or address
// holding escape sequences, control characters or a newline is answered as
// one line with them removed. Planted failure: the value answered as git
// printed it.
func TestTheNameIsStrippedOfControlSequences(t *testing.T) {
	t.Parallel()
	home, root := fixture(t, nil)
	gitIn(t, home, string(root), "config", "user.name", "Ada \x1b[31mExam\x1b]0;title\x07ple\x1b[0m\r\nuser.email\tx")
	gitIn(t, home, string(root), "config", "user.email", "ada@example.com\x1b[2J")
	got, err := adapter(t, configtest.Git(t), home).Who(t.Context(), root)
	if err != nil || got != (model.Identity{Name: "Ada Exampleuser.emailx", Email: "ada@example.com"}) {
		t.Errorf("Who = %q, %v; want the name and address with no control sequence", got, err)
	}
}

// TestAHookEnvironmentIsNotInherited: run inside what a git hook sets
// (GIT_DIR and GIT_INDEX_FILE at another repository) and with git's own
// overrides set (GIT_CONFIG_GLOBAL, GIT_CONFIG_PARAMETERS, GIT_AUTHOR_NAME),
// Who still answers the root's identity. Planted failure: os.Environ()
// handed to git.
func TestAHookEnvironmentIsNotInherited(t *testing.T) {
	who := contracttest.TheIdentity()
	home, root := fixture(t, &who)
	other := contracttest.TheIdentity()
	other.Name = "Mallory Example"
	otherHome, otherRoot := fixture(t, &other)
	gitIn(t, otherHome, otherHome, "config", "--global", "user.name", "Mallory Example")
	t.Setenv("GIT_DIR", filepath.Join(string(otherRoot), ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(string(otherRoot), ".git", "index"))
	t.Setenv("GIT_WORK_TREE", string(otherRoot))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(otherHome, ".gitconfig"))
	t.Setenv("GIT_CONFIG_PARAMETERS", "'user.name'='Mallory Example'")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.name")
	t.Setenv("GIT_CONFIG_VALUE_0", "Mallory Example")
	t.Setenv("GIT_AUTHOR_NAME", "Mallory Example")
	if got, err := adapter(t, configtest.Git(t), home).Who(t.Context(), root); err != nil || got != who {
		t.Errorf("Who inside a hook's environment = %+v, %v; want %+v", got, err, who)
	}
}

// TestNewRefusesAHomeThatIsNotAnAbsoluteCleanPath: a relative home would make
// git read the account's config from the repository it runs in. Planted
// failure: New taking any Home.
func TestNewRefusesAHomeThatIsNotAnAbsoluteCleanPath(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"planted", "./a", "/a/../b", "/a/", "/a\nb"} {
		if _, err := git.New(git.Config{Run: "r1", Home: h}); err == nil {
			t.Errorf("New(Home %q) accepted it", h)
		}
	}
	// the accepted home is this system's own absolute folder, in slash form
	home := filepath.ToSlash(t.TempDir())
	if _, err := git.New(git.Config{Run: "r1", Home: home}); err != nil {
		t.Errorf("New(Home %s) = %v", home, err)
	}
}

// TestAHomeInTheModelFormIsClean: HomeOK takes the slash form a config holds
// and the system's own form an environment holds, on every system, and refuses
// a relative one, a climb and a doubled slash. Risk: Clean's
// backslash on Windows refusing every slash-form Home.
func TestAHomeInTheModelFormIsClean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, c := range map[string]struct {
		home string
		ok   bool
	}{
		"the slash form":    {filepath.ToSlash(dir), true},
		"the system's form": {dir, true},
		"relative":          {"home", false},
		"a climb":           {filepath.ToSlash(dir) + "/../x", false},
		"a doubled slash":   {filepath.ToSlash(dir) + "//x", false},
	} {
		if got := git.HomeOK(c.home); got != c.ok {
			t.Errorf("%s: HomeOK(%q) = %v, want %v", name, c.home, got, c.ok)
		}
	}
}
