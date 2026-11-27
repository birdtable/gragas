package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/exit"
)

// gitEnv is the environment of every git here, the build's clones among them:
// the test's own, with no system config and a global config that is an empty
// file of the test's, so no setting of the machine changes what git answers.
func gitEnv(t *testing.T) []string {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return append(configtest.Inherited(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+global)
}

// repoGit runs git in dir, as a person who commits unsigned, and fails the
// test when it fails.
func repoGit(t *testing.T, env []string, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "-c", "user.name=t", "-c", "user.email=t@users.noreply.github.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false")
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// releaseCall is one start of goreleaser, as build asked for it.
type releaseCall struct {
	dir, program string
	args, extra  []string
}

// buildChain is the fake toolchain with a real git, for the clones, and a
// goreleaser that writes a fixture's good dist folder into the clone it runs
// in and keeps what it was asked.
type buildChain struct {
	*fakeChain
	git  realToolchain
	env  map[string]string
	dist string
	// program is the goreleaser build is given, a file tools.yaml pins.
	program string
	calls   []releaseCall
}

func (b *buildChain) Git(ctx context.Context, dir string, args ...string) (string, int, error) {
	return b.git.Git(ctx, dir, args...)
}

func (b *buildChain) Release(_ context.Context, r goreleaserRun) error {
	b.calls = append(b.calls, releaseCall{dir: r.dir, program: r.program, args: r.args, extra: r.extra})
	return os.CopyFS(filepath.Join(r.dir, "dist"), os.DirFS(b.dist))
}

func (b *buildChain) Getenv(key string) string { return b.env[key] }

// buildRepo is a repository of one commit that holds this repository's
// .goreleaser.yaml, the licence and notices the fixture's archives carry, and a
// tools.yaml that pins a goreleaser outside it for the fake host, and the chain
// whose goreleaser writes that fixture's dist folder.
func buildRepo(t *testing.T) (string, *buildChain) {
	t.Helper()
	env := gitEnv(t)
	work := t.TempDir()
	repoGit(t, env, work, "init", "-q", "-b", "main")
	write(t, filepath.Join(work, configName), realConfig(t))
	for _, f := range carried() {
		write(t, filepath.Join(work, filepath.FromSlash(f)), carriedText(f))
	}
	program := filepath.Join(t.TempDir(), "goreleaser")
	write(t, program, "the pinned goreleaser\n")
	write(t, filepath.Join(work, toolsName), pinsOf(goreleaserID, "linux/amd64", "the pinned goreleaser\n"))
	repoGit(t, env, work, "add", ".")
	repoGit(t, env, work, "commit", "-q", "-m", "base")
	return work, &buildChain{fakeChain: goodChain(), git: newToolchain(func(string) string { return "" }, env), dist: newFixture(t).dist, program: program}
}

func runBuild(t *testing.T, tc toolchain, args ...string) (int, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := run(t.Context(), append([]string{"build"}, args...), &out, &errs, tc)
	return code, out.String() + errs.String()
}

const fakeGoreleaser = "/pinned/goreleaser"

// A publish empties its folder, builds from two clones of HEAD, the first
// with the tag's notes and the token, the second with a cold cache and no
// token, then compares the two and records the unpack step's pass.
func TestTheBuildPublishesFromTwoClonesOfHead(t *testing.T) {
	t.Parallel()
	work, tc := buildRepo(t)
	tc.env = map[string]string{"GITHUB_TOKEN": "a token"}
	out := filepath.Join(work, ".tools", "release")
	write(t, filepath.Join(out, "stale"), "from the last build\n")
	notes := t.TempDir()
	write(t, filepath.Join(notes, testTag+".md"), "the notes\n")

	code, said := runBuild(t, tc, "-C", work, "-out", out, "-goreleaser", tc.program, "-systems", systemsFlag(),
		"-publish", "-tag", testTag, "-notes", notes)
	if code != exit.Done {
		t.Fatalf("exit %d, want done; said:\n%s", code, said)
	}
	if _, err := os.Stat(filepath.Join(out, "stale")); !os.IsNotExist(err) {
		t.Errorf("the stale file of the last build is still there: %v", err)
	}
	if len(tc.calls) != 2 {
		t.Fatalf("goreleaser ran %d times, want twice: %+v", len(tc.calls), tc.calls)
	}
	first, second := tc.calls[0], tc.calls[1]
	want := []releaseCall{
		{dir: filepath.Join(out, "a"), program: tc.program, args: []string{"release", "--clean", "--release-notes", filepath.Join(notes, testTag+".md")}},
		{dir: filepath.Join(out, "b"), program: tc.program, args: []string{"release", "--clean", "--skip=publish"}, extra: []string{"GOCACHE=" + filepath.Join(out, "cache-b"), "GITHUB_TOKEN="}},
	}
	for i, got := range []releaseCall{first, second} {
		if got.dir != want[i].dir || got.program != want[i].program || !slices.Equal(got.args, want[i].args) || !slices.Equal(got.extra, want[i].extra) {
			t.Errorf("goreleaser start %d was %+v, want %+v", i+1, got, want[i])
		}
	}
	head := strings.TrimSpace(gitOut(t, tc, work, "rev-parse", "HEAD"))
	for _, c := range []string{"a", "b"} {
		if got := strings.TrimSpace(gitOut(t, tc, filepath.Join(out, c), "rev-parse", "HEAD")); got != head {
			t.Errorf("the clone %s has %s checked out, want HEAD %s", c, got, head)
		}
	}
	rec, err := os.ReadFile(filepath.Join(out, "unpack.json"))
	if err != nil || !strings.Contains(string(rec), `"pass"`) {
		t.Errorf("unpack.json is %q (%v), want a pass", rec, err)
	}
	for _, step := range []string{"sources: the 3 sources", "compare:"} {
		if !strings.Contains(said, step) {
			t.Errorf("the build did not say %q:\n%s", step, said)
		}
	}
}

func gitOut(t *testing.T, tc *buildChain, dir string, args ...string) string {
	t.Helper()
	text, code, err := tc.Git(t.Context(), dir, args...)
	if err != nil || code != 0 {
		t.Fatalf("git %v in %s: exit %d, %v", args, dir, code, err)
	}
	return text
}

// A snapshot builds both clones with --snapshot, the second with a cold cache
// and the token left as it is, and is held to the snapshot tag.
func TestTheBuildSnapshotsBothClones(t *testing.T) {
	t.Parallel()
	work, tc := buildRepo(t)
	out := filepath.Join(t.TempDir(), "release")
	code, said := runBuild(t, tc, "-C", work, "-out", out, "-goreleaser", tc.program, "-systems", systemsFlag())
	// the fixture's archives are named for testTag, so the unpack step
	// refuses them under the snapshot tag: that is the tag it held them to
	if code != exit.Failed || !strings.Contains(said, "gragas_"+strings.TrimPrefix(snapshotTag, "v")+"_") {
		t.Errorf("exit %d, want failed at the unpack step, naming %s; said:\n%s", code, snapshotTag, said)
	}
	if len(tc.calls) != 2 {
		t.Fatalf("goreleaser ran %d times, want twice", len(tc.calls))
	}
	snap := []string{"release", "--clean", "--snapshot"}
	if !slices.Equal(tc.calls[0].args, snap) || tc.calls[0].extra != nil {
		t.Errorf("the first start was %+v, want %q with nothing over the environment", tc.calls[0], snap)
	}
	if !slices.Equal(tc.calls[1].args, snap) || !slices.Equal(tc.calls[1].extra, []string{"GOCACHE=" + filepath.Join(out, "cache-b")}) {
		t.Errorf("the second start was %+v, want %q with a cold cache", tc.calls[1], snap)
	}
}

// Each refusal before the build starts names why and empties nothing: a
// publish with no token or no notes, and a folder that is the repository or
// holds it.
func TestTheBuildRefusesBeforeItEmptiesAnything(t *testing.T) {
	t.Parallel()
	work, tc := buildRepo(t)
	out := filepath.Join(t.TempDir(), "release")
	write(t, filepath.Join(out, "stale"), "kept\n")
	notes := t.TempDir()
	write(t, filepath.Join(notes, testTag+".md"), "the notes\n")
	base := []string{"-C", work, "-goreleaser", tc.program, "-systems", systemsFlag()}
	for claim, c := range map[string]struct {
		env  map[string]string
		args []string
		want string
	}{
		"no token":       {nil, []string{"-out", out, "-publish", "-tag", testTag, "-notes", notes}, "GITHUB_TOKEN is not set"},
		"no notes file":  {map[string]string{"GITHUB_TOKEN": "x"}, []string{"-out", out, "-publish", "-tag", "v9.9.9", "-notes", notes}, "the notes of v9.9.9"},
		"the repository": {nil, []string{"-out", work}, "is the repository or holds it"},
		"its parent":     {nil, []string{"-out", filepath.Dir(work)}, "is the repository or holds it"},
	} {
		tc.env = c.env
		code, said := runBuild(t, tc, append(slices.Clone(base), c.args...)...)
		if code != exit.Failed || !strings.Contains(said, c.want) {
			t.Errorf("%s: exit %d, said %q; want failed, naming %q", claim, code, said, c.want)
		}
	}
	if len(tc.calls) != 0 {
		t.Errorf("goreleaser ran: %+v", tc.calls)
	}
	if _, err := os.Stat(filepath.Join(out, "stale")); err != nil {
		t.Errorf("a refused build emptied its folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, configName)); err != nil {
		t.Errorf("a refused build emptied the repository: %v", err)
	}
}

// A line build does not take is a usage fault that names what is wrong.
func TestTheBuildsFlags(t *testing.T) {
	t.Parallel()
	ok := []string{"-out", "o", "-goreleaser", fakeGoreleaser, "-systems", "linux/amd64"}
	for claim, c := range map[string]struct {
		args []string
		want string
	}{
		"an argument":           {append(slices.Clone(ok), "x"), `was given "x"`},
		"no folder":             {[]string{"-goreleaser", fakeGoreleaser, "-systems", "linux/amd64"}, "-out names no folder"},
		"no goreleaser":         {[]string{"-out", "o", "-systems", "linux/amd64"}, "-goreleaser names no program"},
		"no system":             {[]string{"-out", "o", "-goreleaser", fakeGoreleaser}, "is not os/arch"},
		"a system twice":        {[]string{"-out", "o", "-goreleaser", fakeGoreleaser, "-systems", "linux/amd64,linux/amd64"}, "twice"},
		"a publish with no tag": {append(slices.Clone(ok), "-publish", "-notes", "n"), "is not a release tag"},
		"a publish, no notes":   {append(slices.Clone(ok), "-publish", "-tag", testTag), "-notes names no folder"},
		"a snapshot with a tag": {append(slices.Clone(ok), "-tag", testTag), "a snapshot is " + snapshotTag},
	} {
		code, said := runBuild(t, &fakeChain{}, c.args...)
		if code != exit.Usage || !strings.Contains(said, c.want) || !strings.Contains(said, "usage: gragas-release build") {
			t.Errorf("%s: exit %d, said %q; want usage, naming %q", claim, code, said, c.want)
		}
	}
}

// The real goreleaser runs only from an absolute path, and never in an
// environment of nil, which os/exec reads as all of it.
func TestTheRealReleaseRefusesARelativeProgramAndNoEnvironment(t *testing.T) {
	t.Parallel()
	r := goreleaserRun{dir: t.TempDir(), program: "goreleaser", stdout: io.Discard, stderr: io.Discard}
	if err := newToolchain(func(string) string { return "" }, []string{}).Release(t.Context(), r); err == nil || !strings.Contains(err.Error(), "not an absolute path") {
		t.Errorf("a relative goreleaser: %v, want refused", err)
	}
	r.program = fakeGoreleaser
	if err := newToolchain(func(string) string { return "" }, nil).Release(t.Context(), r); err == nil || !strings.Contains(err.Error(), "no environment") {
		t.Errorf("no environment: %v, want refused", err)
	}
}

// pinsOf is a tools.yaml of one release tool, id, whose program on platform is
// the file holding program.
func pinsOf(id, platform, program string) string {
	return "tools:\n  - id: " + id + "\n    from: release\n    platforms:\n      " + platform + ":\n        sha256: " + digest.Hex([]byte(program)) + "\n"
}

// A goreleaser that is not the one tools.yaml pins for the host is never
// started, and nothing is emptied or cloned first: another program in its
// place, at a publish too, where it would
// have been handed GITHUB_TOKEN; a path that is not absolute; and none.
func TestABuildStartsNoGoreleaserButThePinnedOne(t *testing.T) {
	t.Parallel()
	work, tc := buildRepo(t)
	tc.env = map[string]string{"GITHUB_TOKEN": "a token"}
	out := filepath.Join(t.TempDir(), "release")
	write(t, filepath.Join(out, "stale"), "kept\n")
	notes := t.TempDir()
	write(t, filepath.Join(notes, testTag+".md"), "the notes\n")
	planted := filepath.Join(t.TempDir(), "goreleaser")
	write(t, planted, "FAKE GORELEASER\n")
	publish := []string{"-publish", "-tag", testTag, "-notes", notes}
	for claim, c := range map[string]struct {
		program string
		args    []string
		want    string
	}{
		"another program":              {planted, nil, "is not the pinned goreleaser"},
		"another program at a publish": {planted, publish, "is not the pinned goreleaser"},
		"a relative path":              {"goreleaser", nil, "is not an absolute path"},
		"no program there":             {filepath.Join(t.TempDir(), "goreleaser"), publish, "goreleaser "},
	} {
		args := append([]string{"-C", work, "-out", out, "-goreleaser", c.program, "-systems", systemsFlag()}, c.args...)
		code, said := runBuild(t, tc, args...)
		if code != exit.Failed || !strings.Contains(said, c.want) {
			t.Errorf("%s: exit %d, said %q; want failed, naming %q", claim, code, said, c.want)
		}
	}
	if len(tc.calls) != 0 {
		t.Errorf("a goreleaser that is not the pin was started: %+v", tc.calls)
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 1 || entries[0].Name() != "stale" {
		t.Errorf("a refused goreleaser touched the release folder: %v, %v", entries, err)
	}
}

// The pin is the sha256 of the host's platform of the one release row of the
// id: gragas's own tools.yaml pins goreleaser for each system the release is
// made on, and a row missing, twice, not a release, with no sha256 for the
// host or a sum that is not 64 hex digits is refused.
func TestThePinIsTheHostsSumOfOneReleaseRow(t *testing.T) {
	t.Parallel()
	real, err := os.ReadFile(filepath.Join("..", "..", toolsName))
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []system{{"darwin", "arm64"}, {"linux", "amd64"}} {
		if sum, err := pinnedSum(real, goreleaserID, host); err != nil || !sumForm.MatchString(sum) {
			t.Errorf("gragas's tools.yaml for %v: %q, %v; want goreleaser's pin", host, sum, err)
		}
	}
	host := system{OS: "linux", Arch: "amd64"}
	row := pinsOf(goreleaserID, "linux/amd64", "x")
	if sum, err := pinnedSum([]byte(row), goreleaserID, host); err != nil || sum != digest.Hex([]byte("x")) {
		t.Errorf("a row's pin: %q, %v", sum, err)
	}
	for claim, c := range map[string]struct{ text, want string }{
		"no row":           {"tools:\n  - id: other\n    from: release\n", "has no row for goreleaser"},
		"two rows":         {row + strings.TrimPrefix(row, "tools:\n"), "2 rows for goreleaser"},
		"not a release":    {strings.Replace(row, "from: release", "from: go", 1), "is not a release tool"},
		"another platform": {pinsOf(goreleaserID, "darwin/arm64", "x"), "pins no sha256 of goreleaser for linux/amd64"},
		"not a sum":        {strings.Replace(row, digest.Hex([]byte("x")), "abc", 1), `(it has "abc")`},
		"not yaml":         {"tools: [\n", "does not parse"},
		"not a mapping":    {"- a\n", "is not a mapping"},
	} {
		if _, err := pinnedSum([]byte(c.text), goreleaserID, host); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want refused naming %q", claim, err, c.want)
		}
	}
}
