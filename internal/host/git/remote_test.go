package git_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/birdtable/go-files/filestest"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	"github.com/birdtable/go-proc/gitenv"
	testenv "github.com/birdtable/go-testkit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// remoteWorld is a repository with one commit, a source repository with
// the tag v1 and a releases cache, each at its resolved path.
type remoteWorld struct {
	home, base string
	root       model.Root
	src, rels  string
}

// newRemote makes a remoteWorld whose repository holds files at its one
// commit and whose source's v1 holds TheRelease.
func newRemote(t *testing.T, files map[string]string) remoteWorld {
	t.Helper()
	who := contracttest.FixtureWho()
	home, root := fixture(t, &who)
	w := remoteWorld{home: home, root: model.Root(resolved(t, string(root))), base: resolved(t, filestest.Private(t))}
	commitAll(t, w.home, string(w.root), files, "one")
	w.src = filepath.Join(w.base, "src")
	mkdir(t, w.src)
	gitIn(t, home, w.src, "init", "-q")
	commitAll(t, w.home, w.src, contracttest.TheRelease(), "v1")
	story(t, home, w.src, "tag", "-a", "-m", "v1", "v1")
	w.rels = filepath.Join(w.base, "releases")
	return w
}

// commitAll makes the work tree of dir hold exactly files and commits them.
func commitAll(t *testing.T, home, dir string, files map[string]string, msg string) {
	t.Helper()
	write(t, dir, files)
	story(t, home, dir, "add", "-A", ".")
	story(t, home, dir, "commit", "-q", "--no-verify", "-m", msg)
}

// adapter is the adapter of w, with the groups on the way to git holding
// root and this account alone, as adapterOf.
func (w remoteWorld) adapter(t *testing.T) *git.Git {
	t.Helper()
	return w.adapterUsing(t, configtest.Git(t))
}

// adapterUsing is the adapter of w that runs the git at gitPath.
func (w remoteWorld) adapterUsing(t *testing.T, gitPath string) *git.Git {
	t.Helper()
	g, err := git.New(git.Config{Git: gitPath, Releases: w.rels, Home: w.home, Run: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// hooked plants a hook of each name in dir's hooks folder that leaves a
// mark, and answers the mark's path.
func hooked(t *testing.T, gitDir string) string {
	t.Helper()
	mark := filepath.Join(filepath.Dir(gitDir), "hook-ran")
	hooks := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooks, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"pre-commit", "post-commit", "reference-transaction", "post-index-change", "pre-auto-gc", "post-checkout"} {
		configtest.Fake(t, hooks, h, configtest.Step{Write: mark, Text: h + "\n", Append: true})
	}
	provesMark(t, filepath.Join(hooks, "pre-commit"), mark)
	return mark
}

// runs starts the planted program at p and answers how it ended.
func runs(t *testing.T, p string) error {
	t.Helper()
	return exec.CommandContext(t.Context(), p).Run()
}

// provesMark runs the planted program at p once and requires the mark it writes,
// then removes the mark: a test that asserts the mark was not written is vacuous
// when the program cannot write it.
func provesMark(t *testing.T, p, mark string) {
	t.Helper()
	c := exec.CommandContext(t.Context(), p)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("the planted program %s did not run: %v %s", p, err, out)
	}
	if _, err := os.Stat(mark); err != nil {
		t.Fatalf("the planted program %s left no mark: %v", p, err)
	}
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
}

// noMark fails t when a planted hook ran.
func noMark(t *testing.T, mark string) {
	t.Helper()
	if b, err := os.ReadFile(mark); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a hook ran: %q, %v", b, err)
	}
}

// ran is the verbs g ran, in order.
func ran(g *git.Git) []string {
	var out []string
	for _, r := range g.Ran() {
		out = append(out, r.Verb)
	}
	return out
}

func TestTheGitAdapterPassesTheRemoteSuite(t *testing.T) {
	t.Parallel()
	contracttest.Remote(t, func(t *testing.T) contracttest.RemoteSetup {
		t.Helper()
		w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
		return contracttest.RemoteSetup{
			R: w.adapter(t), Root: w.root, From: w.src,
			Write: func(p, c string) { writeFile(t, filepath.Join(string(w.root), p), c) },
			Read: func(r model.Root, p string) (string, error) {
				b, err := os.ReadFile(filepath.Join(string(r), filepath.FromSlash(p)))
				return string(b), err
			},
		}
	})
}

func writeFile(t *testing.T, path, c string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(c), 0o600); err != nil {
		t.Fatal(err)
	}
}

// the files of a commit's tree, by path.
func treeOf(t *testing.T, home, dir, rev string) []string {
	t.Helper()
	out := story(t, home, dir, "ls-tree", "-r", "--name-only", rev)
	return strings.Split(out, "\n")
}

// TestACommitHoldsTheNamedPathsAlone: a commit holds HEAD's tree with the
// named paths set, a change and a removal among them, and nothing else: not
// a file changed in the work tree, nor one staged. HEAD moves to it; the
// index then holds the commit's file at each named path and still holds
// what else was staged; and no hook runs. Planted failures: the
// repository's own index committed; the hooks not pinned; the index left
// as it was.
func TestACommitHoldsTheNamedPathsAlone(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n", "b.txt": "b\n", "c.txt": "c\n", "sub/d.txt": "d\n"})
	dir := string(w.root)
	parent := story(t, w.home, dir, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(dir, "rules.yaml"), "two\n")
	writeFile(t, filepath.Join(dir, "b.txt"), "b changed, not named\n")
	writeFile(t, filepath.Join(dir, "e.txt"), "staged, not named\n")
	story(t, w.home, dir, "add", "e.txt")
	writeFile(t, filepath.Join(dir, "f.txt"), "new\n")
	if err := os.Remove(filepath.Join(dir, "c.txt")); err != nil {
		t.Fatal(err)
	}
	g := w.adapter(t)
	mark := hooked(t, filepath.Join(dir, ".git"))
	id, err := g.Commit(t.Context(), w.root, []string{"rules.yaml", "c.txt", "f.txt"}, "release one")
	if err != nil {
		t.Fatal(err)
	}
	noMark(t, mark)
	if head := story(t, w.home, dir, "rev-parse", "HEAD"); head != id {
		t.Errorf("HEAD is %s, want the commit %s", head, id)
	}
	if got := story(t, w.home, dir, "rev-parse", id+"^"); got != parent {
		t.Errorf("the commit's parent is %s, want %s", got, parent)
	}
	if got, want := treeOf(t, w.home, dir, id), []string{"b.txt", "f.txt", "rules.yaml", "sub/d.txt"}; !slices.Equal(got, want) {
		t.Errorf("the commit holds %q, want %q", got, want)
	}
	for p, want := range map[string]string{"b.txt": "b", "rules.yaml": "two", "f.txt": "new"} {
		if got := story(t, w.home, dir, "show", id+":"+p); got != want {
			t.Errorf("the commit's %s is %q, want %q", p, got, want)
		}
	}
	if got := story(t, w.home, dir, "log", "-1", "--format=%B", id); got != "release one" {
		t.Errorf("the message is %q", got)
	}
	if got := story(t, w.home, dir, "diff", "--cached", "--name-only"); got != "e.txt" {
		t.Errorf("staged after the commit: %q, want e.txt alone", got)
	}
	if got := story(t, w.home, dir, "diff", "--name-only"); got != "b.txt" {
		t.Errorf("changed in the work tree after the commit: %q, want b.txt alone", got)
	}
}

// TestACommitOfAnExecutableKeepsItsMode: a file this account may execute
// is committed as one, and a mode change alone is a change.
func TestACommitOfAnExecutableKeepsItsMode(t *testing.T) {
	t.Parallel()
	if !testenv.KeepsModes(t) {
		return // the executable bit is a fact of a system that keeps mode bits; git there reads none (core.filemode false)
	}
	w := newRemote(t, map[string]string{"run.sh": "run\n"})
	dir := string(w.root)
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	id, err := w.adapter(t).Commit(t.Context(), w.root, []string{"run.sh"}, "exec")
	if err != nil {
		t.Fatal(err)
	}
	if got := story(t, w.home, dir, "ls-tree", id, "run.sh"); !strings.HasPrefix(got, "100755 ") {
		t.Errorf("the commit lists %q, want mode 100755", got)
	}
}

// TestACommitWithNoFileModeKeepsHEADsMode: in a repository whose
// core.fileMode is false, a file HEAD holds as 100755 is committed as
// 100755 whatever its bits on the disk, as git commits it, and the same
// content with other bits is no change (go-proc's gitenv.CommitMode).
// Planted failure: the mode read from the file alone.
func TestACommitWithNoFileModeKeepsHEADsMode(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"run.sh": "run\n"})
	dir := string(w.root)
	story(t, w.home, dir, "update-index", "--chmod=+x", "run.sh")
	story(t, w.home, dir, "commit", "-q", "-m", "exec")
	story(t, w.home, dir, "config", "core.fileMode", "false")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := w.adapter(t)
	if got, err := g.Commit(t.Context(), w.root, []string{"run.sh"}, "same"); !errors.Is(err, git.ErrNoChange) {
		t.Errorf("Commit of HEAD's content with other bits = %q, %v; want no change", got, err)
	}
	writeFile(t, filepath.Join(dir, "run.sh"), "run again\n")
	id, err := g.Commit(t.Context(), w.root, []string{"run.sh"}, "again")
	if err != nil {
		t.Fatal(err)
	}
	if got := story(t, w.home, dir, "ls-tree", id, "run.sh"); !strings.HasPrefix(got, "100755 ") {
		t.Errorf("the commit lists %q, want mode 100755, HEAD's", got)
	}
}

// TestACommitOverALinkInHEADIsRefused: a path HEAD holds as a link, 120000,
// is refused when a file stands there now, as gitenv.CommitMode refuses a
// file written over a link: HEAD stays, no commit is written,
// and the tree still lists the link. A path HEAD holds as a link and that
// nothing stands at now is no change, as before, never the link's removal.
// Planted failure: HEAD's links dropped before the mode is looked up, so the
// file is committed over the link as 100644.
func TestACommitOverALinkInHEADIsRefused(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := string(w.root)
	link(t, "rules.yaml", filepath.Join(dir, "ln"))
	story(t, w.home, dir, "add", "ln")
	story(t, w.home, dir, "commit", "-q", "-m", "a link")
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	if err := os.Remove(filepath.Join(dir, "ln")); err != nil {
		t.Fatal(err)
	}
	g := w.adapter(t)
	if got, err := g.Commit(t.Context(), w.root, []string{"ln"}, "gone"); !errors.Is(err, git.ErrNoChange) {
		t.Errorf("Commit of a link HEAD holds and the tree does not = %q, %v; want no change", got, err)
	}
	writeFile(t, filepath.Join(dir, "ln"), "a file now\n")
	got, err := g.Commit(t.Context(), w.root, []string{"ln"}, "over the link")
	if err == nil || !strings.Contains(err.Error(), "commit ln: ") {
		t.Errorf("Commit of a file over HEAD's link = %q, %v; want it refused", got, err)
	}
	if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != head {
		t.Errorf("HEAD moved from %s to %s", head, now)
	}
	if listed := story(t, w.home, dir, "ls-tree", "HEAD", "ln"); !strings.HasPrefix(listed, "120000 ") {
		t.Errorf("HEAD lists %q, want the link, 120000", listed)
	}
}

// TestACommitBelowALinkOrOverASubmoduleInHEADIsRefused: a
// path below a folder HEAD holds as a link, ld/y and a/ld/y, or as a
// submodule, sub/y, is refused, and so is a file at a path HEAD holds as a
// submodule; HEAD stays and the tree still lists the link and the
// submodule. A submodule HEAD holds that nothing stands at now is no change,
// never its removal. Control: a file in a real folder, d/y, commits.
// Planted failure: in the code before it, the listing leaves the
// submodule out and Commit reads no folder above the path, so each of the
// four is committed, the link or the submodule replaced.
func TestACommitBelowALinkOrOverASubmoduleInHEADIsRefused(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := string(w.root)
	link(t, "rules.yaml", filepath.Join(dir, "ld"))
	writeFile(t, filepath.Join(dir, "a", "keep"), "a\n")
	link(t, filepath.Join("..", "rules.yaml"), filepath.Join(dir, "a", "ld"))
	story(t, w.home, dir, "add", "ld", "a")
	sub := story(t, w.home, dir, "rev-parse", "HEAD")
	story(t, w.home, dir, "update-index", "--add", "--cacheinfo", "160000,"+sub+",sub")
	story(t, w.home, dir, "commit", "-q", "-m", "a link, a deeper link and a submodule")
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	for _, p := range []string{"ld", filepath.Join("a", "ld")} {
		if err := os.Remove(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}
	g := w.adapter(t)
	if got, err := g.Commit(t.Context(), w.root, []string{"sub"}, "gone"); !errors.Is(err, git.ErrNoChange) {
		t.Errorf("Commit of a submodule HEAD holds and the tree does not = %q, %v; want no change", got, err)
	}
	writeFile(t, filepath.Join(dir, "sub"), "a file now\n")
	if got, err := g.Commit(t.Context(), w.root, []string{"sub"}, "over the submodule"); err == nil || !strings.Contains(err.Error(), "commit sub: ") {
		t.Errorf("Commit of a file over HEAD's submodule = %q, %v; want it refused", got, err)
	}
	if err := os.Remove(filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"ld/y", "a/ld/y", "sub/y"} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(p)), "below\n")
		got, err := g.Commit(t.Context(), w.root, []string{p}, "below")
		if err == nil || !strings.Contains(err.Error(), "commit "+p+": HEAD holds ") {
			t.Errorf("Commit of %s = %q, %v; want it refused, naming what HEAD holds", p, got, err)
		}
	}
	if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != head {
		t.Errorf("HEAD moved from %s to %s", head, now)
	}
	for p, mode := range map[string]string{"ld": "120000 ", "a/ld": "120000 ", "sub": "160000 "} {
		if listed := story(t, w.home, dir, "ls-tree", "HEAD", p); !strings.HasPrefix(listed, mode) {
			t.Errorf("HEAD lists %q for %s, want %s", listed, p, mode)
		}
	}
	writeFile(t, filepath.Join(dir, "d", "y"), "in a folder\n")
	if _, err := g.Commit(t.Context(), w.root, []string{"d/y"}, "a real folder"); err != nil {
		t.Errorf("control: Commit of d/y in a real folder: %v", err)
	}
}

// TestACommitThatChangesTheKindOfWhatHEADHoldsIsRefused:
// no commit puts a folder where HEAD holds a file, or a file where HEAD
// holds a folder. Each refusal names the entry HEAD holds and says to remove
// it first, and HEAD stays, its listing unchanged:
//   - f/y, where HEAD's f is a file;
//   - dd as a file, where HEAD's folder dd holds dd/x and dd/z;
//   - de as a file, where HEAD's folder de holds only a link and a
//     submodule.
//
// Controls: d/y beside a folder d that HEAD holds commits, and so does a
// file over a file. Planted failure: with underHeld's file case out, f/y
// is committed and f is gone; with overHeld out, dd and de are committed
// and what each held is gone, the link and the submodule with it.
func TestACommitThatChangesTheKindOfWhatHEADHoldsIsRefused(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n", "f": "f\n", "d/x": "x\n", "dd/x": "x\n", "dd/z": "z\n"})
	dir := string(w.root)
	mkdir(t, filepath.Join(dir, "de"))
	link(t, filepath.Join("..", "rules.yaml"), filepath.Join(dir, "de", "ln"))
	story(t, w.home, dir, "add", "de")
	sub := story(t, w.home, dir, "rev-parse", "HEAD")
	story(t, w.home, dir, "update-index", "--add", "--cacheinfo", "160000,"+sub+",de/sub")
	story(t, w.home, dir, "commit", "-q", "-m", "a link and a submodule in de")
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	listed := story(t, w.home, dir, "ls-tree", "-r", "HEAD")
	if err := os.Remove(filepath.Join(dir, "f")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "f", "y"), "below a file\n")
	for _, p := range []string{"dd", "de"} {
		if err := os.RemoveAll(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, p), "a file now\n")
	}
	g := w.adapter(t)
	for p, want := range map[string]string{
		"f/y": "commit f/y: HEAD holds f as a file, 100644, and no folder is committed where it stands: remove f first, by its own name",
		"dd":  "commit dd: HEAD holds dd as a folder, with dd/x in it, and no file is committed where it stands: remove what it holds first, each by its own name",
		"de":  "commit de: HEAD holds de as a folder, with de/ln in it, and no file is committed where it stands: remove what it holds first, each by its own name",
	} {
		if got, err := g.Commit(t.Context(), w.root, []string{p}, "another kind"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Commit of %s = %q, %v; want it refused: %s", p, got, err, want)
		}
	}
	if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != head {
		t.Errorf("HEAD moved from %s to %s", head, now)
	}
	if now := story(t, w.home, dir, "ls-tree", "-r", "HEAD"); now != listed {
		t.Errorf("HEAD lists\n%s\nwant\n%s", now, listed)
	}
	writeFile(t, filepath.Join(dir, "d", "y"), "beside x\n")
	if _, err := g.Commit(t.Context(), w.root, []string{"d/y"}, "beside a folder HEAD holds"); err != nil {
		t.Errorf("control: Commit of d/y beside HEAD's d/x: %v", err)
	}
	writeFile(t, filepath.Join(dir, "rules.yaml"), "two\n")
	if _, err := g.Commit(t.Context(), w.root, []string{"rules.yaml"}, "a file over a file"); err != nil {
		t.Errorf("control: Commit of a file over a file: %v", err)
	}
}

// TestASecondSpellingOfANameHEADHoldsIsRefusedWhereTheDiskReadsThemAsOne:
// where the disk reads two spellings as one file, as the
// Mac's folds case and composition, each of these is refused, naming HEAD's
// spelling, and HEAD stays; where it does not, each commits as a new name:
//   - LD/y, where HEAD holds the link ld;
//   - a/LD/y, where HEAD holds the link a/ld, a folder down;
//   - notes.md, where HEAD holds the file Notes.md;
//   - the name café decomposed, where HEAD holds it composed.
//
// The disk is asked by the test alone: whether the two spellings stat as
// the same file. Controls: le/y beside the link ld, and a/z beside HEAD's
// a/keep, commit on every disk. Planted failure: with foldedOnto out, on
// the Mac each of the four is committed beside HEAD's spelling.
func TestASecondSpellingOfANameHEADHoldsIsRefusedWhereTheDiskReadsThemAsOne(t *testing.T) {
	t.Parallel()
	const composed, decomposed = "café", "café"
	w := newRemote(t, map[string]string{"rules.yaml": "one\n", "Notes.md": "n\n", "a/keep": "a\n", composed: "c\n"})
	dir := string(w.root)
	link(t, "rules.yaml", filepath.Join(dir, "ld"))
	link(t, filepath.Join("..", "rules.yaml"), filepath.Join(dir, "a", "ld"))
	story(t, w.home, dir, "add", "ld", filepath.Join("a", "ld"))
	story(t, w.home, dir, "commit", "-q", "-m", "two links")
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	for _, p := range []string{"ld", filepath.Join("a", "ld"), "Notes.md", composed} {
		if err := os.Remove(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}
	type spelt struct{ p, held, as string }
	var folded, apart []spelt
	for _, c := range []spelt{
		{"LD/y", "ld", "LD"},
		{"a/LD/y", "a/ld", "a/LD"},
		{"notes.md", "Notes.md", "notes.md"},
		{decomposed, composed, decomposed},
	} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(c.p)), "a second spelling\n")
		mine, errMine := os.Lstat(filepath.Join(dir, filepath.FromSlash(c.as)))
		heads, errHeads := os.Lstat(filepath.Join(dir, filepath.FromSlash(c.held)))
		if errMine == nil && errHeads == nil && os.SameFile(mine, heads) {
			folded = append(folded, c)
		} else {
			apart = append(apart, c)
		}
	}
	t.Logf("this disk reads as one: %+q; as two: %+q", folded, apart)
	g := w.adapter(t)
	for _, c := range folded {
		want := "commit " + c.p + ": HEAD holds " + c.held + ", a name this folder reads as " + c.as + ", and no second spelling of one name is committed beside it"
		if got, err := g.Commit(t.Context(), w.root, []string{c.p}, "a second spelling"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Commit of %q = %q, %v; want it refused: %s", c.p, got, err, want)
		}
	}
	if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != head {
		t.Errorf("HEAD moved from %s to %s", head, now)
	}
	for _, c := range apart {
		if _, err := g.Commit(t.Context(), w.root, []string{c.p}, "a name of its own"); err != nil {
			t.Errorf("Commit of %q on a disk that reads it apart from %q: %v", c.p, c.held, err)
		}
	}
	for _, p := range []string{"le/y", "a/z"} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(p)), "control\n")
		if _, err := g.Commit(t.Context(), w.root, []string{p}, "control"); err != nil {
			t.Errorf("control: Commit of %s: %v", p, err)
		}
	}
}

// TestACommitOverATreeListedOutOfOrderIsRefused: a hand-made
// tree that lists the link ld last, after zz, is refused when HEAD is read,
// naming ld, so ld/y never replaces the link a search of the listing would
// miss; HEAD stays. Planted failure: with parseTree's order check out, the
// search misses ld and ld/y is committed, the link replaced by a folder.
func TestACommitOverATreeListedOutOfOrderIsRefused(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := string(w.root)
	var tree []byte
	for _, e := range []struct{ name, mode, body string }{{"rules.yaml", "100644", "one\n"}, {"zz", "100644", "z\n"}, {"ld", "120000", "rules.yaml"}} {
		oid, err := hex.DecodeString(storyIn(t, w.home, dir, e.body, "hash-object", "-w", "--stdin"))
		if err != nil {
			t.Fatal(err)
		}
		tree = append(append(tree, e.mode+" "+e.name+"\x00"...), oid...)
	}
	made := storyIn(t, w.home, dir, string(tree), "hash-object", "-t", "tree", "-w", "--literally", "--stdin")
	c := story(t, w.home, dir, "commit-tree", made, "-p", "HEAD", "-m", "ld listed last")
	story(t, w.home, dir, "update-ref", "HEAD", c)
	writeFile(t, filepath.Join(dir, "ld", "y"), "below the link\n")
	got, err := w.adapter(t).Commit(t.Context(), w.root, []string{"ld/y"}, "below")
	if want := `the tree lists "ld" after "zz", out of git's order`; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Commit of ld/y over a tree out of order = %q, %v; want it refused: %s", got, err, want)
	}
	if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != c {
		t.Errorf("HEAD moved from %s to %s", c, now)
	}
}

// TestACommitRefusesAndMakesNone: each of these is an error, HEAD stays,
// and no commit is written: no path, a path named twice, a path that is
// not a plain path of the tree, a path whose content is HEAD's or that is
// in neither, a link or a path through one, a folder, a file past the
// cap, a message that is
// empty or holds a control character, a root that is not the top of its
// repository, and a repository with no identity. Planted failures: each
// check taken out in turn.
func TestACommitRefusesAndMakesNone(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n", "sub/d.txt": "d\n"})
	dir := string(w.root)
	writeFile(t, filepath.Join(dir, "rules.yaml"), "two\n")
	writeFile(t, filepath.Join(dir, "out.txt"), "outside\n")
	link(t, "out.txt", filepath.Join(dir, "ln"))
	link(t, "sub", filepath.Join(dir, "lnk"))
	writeFile(t, filepath.Join(dir, "big.bin"), strings.Repeat("x", git.MaxListing+1))
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	type refusal struct {
		name    string
		root    model.Root
		paths   []string
		message string
		noop    bool
		// says is what the refusal says, when the check that refuses is not
		// the only one that would
		says string
		cap  bool
	}
	cases := []refusal{
		{name: "no path", message: "m"},
		{name: "a path named twice", paths: []string{"rules.yaml", "rules.yaml"}, message: "m"},
		{name: "an absolute path to a file the tree holds", paths: []string{dir + "/rules.yaml"}, message: "m"},
		{name: "a path whose content is HEAD's", paths: []string{"sub/d.txt"}, message: "m", noop: true},
		{name: "a path in neither", paths: []string{"nowhere"}, message: "m", noop: true},
		{name: "a link", paths: []string{"ln"}, message: "m", says: "it is a link or no regular file"},
		{name: "a file past the cap", paths: []string{"big.bin"}, message: "m", cap: true},
		{name: "a path through a link", paths: []string{"lnk/d.txt"}, message: "m"},
		{name: "a folder", paths: []string{"sub"}, message: "m"},
		{name: "an empty message", paths: []string{"rules.yaml"}},
		{name: "a blank message", paths: []string{"rules.yaml"}, message: " \n\t"},
		{name: "a NUL in the message", paths: []string{"rules.yaml"}, message: "a\x00b"},
		{name: "an escape in the message", paths: []string{"rules.yaml"}, message: "\x1b[31mred"},
		{name: "a root below the top", root: model.Root(filepath.Join(dir, "sub")), paths: []string{"d.txt"}, message: "m"},
	}
	for _, r := range pathTable(t, "refused-paths.txt") {
		cases = append(cases, refusal{name: r.what, paths: []string{r.path}, message: "m"})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := w.root
			if c.root != "" {
				root = c.root
			}
			g := w.adapter(t)
			got, err := g.Commit(t.Context(), root, c.paths, c.message)
			if err == nil || got != "" {
				t.Fatalf("Commit = %q, %v; want an error and no commit", got, err)
			}
			if c.noop != errors.Is(err, git.ErrNoChange) {
				t.Errorf("Commit: %v; want it to be no change: %v", err, c.noop)
			}
			if c.cap != errors.Is(err, git.ErrPastCap) {
				t.Errorf("Commit: %v; want it to be past the cap: %v", err, c.cap)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("Commit: %v; want it to say %q", err, c.says)
			}
			if slices.Contains(ran(g), "commit-tree") || slices.Contains(ran(g), "update-ref") {
				t.Errorf("a refused commit ran %q", ran(g))
			}
			if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != head {
				t.Errorf("HEAD moved to %s", now)
			}
		})
	}
	t.Run("no identity", func(t *testing.T) {
		home, root := fixture(t, nil)
		commitAllAs(t, home, string(root))
		writeFile(t, filepath.Join(string(root), "rules.yaml"), "two\n")
		g := remoteWorld{home: home, rels: w.rels}.adapter(t)
		if _, err := g.Commit(t.Context(), root, []string{"rules.yaml"}, "m"); !isNotReady(err) {
			t.Errorf("Commit with no identity: %v, want not ready", err)
		}
	})
}

// commitAllAs makes a first commit in dir with the fixture's identity
// handed by the environment, so the repository itself names none.
func commitAllAs(t *testing.T, home, dir string) {
	t.Helper()
	write(t, dir, map[string]string{"rules.yaml": "one\n"})
	story(t, home, dir, "add", "-A", ".")
	story(t, home, dir, "commit", "-q", "--no-verify", "-m", "one")
}

// TestAMovedHEADIsNotOverwritten: a commit made on the HEAD it read, after
// HEAD has moved on, is an error and HEAD stays where it moved. Planted
// failure: update-ref without the old value.
func TestAMovedHEADIsNotOverwritten(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := string(w.root)
	old := story(t, w.home, dir, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(dir, "rules.yaml"), "two\n")
	g := w.adapter(t)
	moved, err := g.Commit(t.Context(), w.root, []string{"rules.yaml"}, "two")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "rules.yaml"), "three\n")
	if got, err := git.CommitOn(t.Context(), g, w.root, old, []string{"rules.yaml"}, "three"); err == nil || got != "" {
		t.Errorf("a commit on a HEAD that moved = %q, %v; want an error", got, err)
	}
	if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != moved {
		t.Errorf("HEAD is %s, want it left at %s", now, moved)
	}
}

// fileSet is every file under root, by slash path, with its content and
// whether it is executable; a link or a folder that is not on the way to a
// file is listed as such.
func fileSet(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode().Type() == fs.ModeSymlink:
			out[rel] = "a link"
		case info.IsDir():
		default:
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			// the mode git would commit for it as a new path, by gitenv's rule
			mode, err := gitenv.CommitMode(info, true, "")
			if err != nil {
				return err
			}
			out[rel] = string(b) + map[bool]string{true: " (exec)", false: ""}[mode == "100755"]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAFetchIsTheTagsFilesAloneFromAPathOrAMirror: the tag fetched from a
// repository and from a bare mirror of it is the same files, an executable
// still one, with no link, no submodule and no .git; the cache holds no
// folder of a fetch in progress; no hook of the cache runs; and each fetch
// runs with every protocol but file refused, no credential helper and
// every object checked. Planted failures: a link written; the mode
// dropped; a pin taken out of the fetch's row.
func TestAFetchIsTheTagsFilesAloneFromAPathOrAMirror(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	writeFile(t, filepath.Join(w.src, "bin/run.sh"), "run\n")
	if err := os.Chmod(filepath.Join(w.src, "bin/run.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	link(t, "rules.yaml", filepath.Join(w.src, "ln"))
	story(t, w.home, w.src, "add", "-A", ".")
	blob := story(t, w.home, w.src, "rev-parse", "HEAD:rules.yaml")
	story(t, w.home, w.src, "update-index", "--add", "--cacheinfo", "160000,"+blob[:0]+strings.Repeat("ab", 20)+",mod")
	story(t, w.home, w.src, "commit", "-q", "--no-verify", "-m", "v2")
	story(t, w.home, w.src, "tag", "-a", "-m", "v2", "v2")
	mirror := filepath.Join(w.base, "mirror.git")
	gitIn(t, w.home, w.base, "clone", "-q", "--mirror", w.src, mirror)
	// A file's executable bit is a fact of a system that keeps mode bits, which a checkout elsewhere does not.
	exec := ""
	if testenv.KeepsModes(t) {
		exec = " (exec)"
	}
	want := map[string]string{"rules.yaml": "r: 1\n", "presets/a.json": "{}\n", "bin/run.sh": "run\n" + exec}
	g := w.adapter(t)
	var roots []model.Root
	for _, from := range []string{w.src, mirror} {
		root, err := g.Fetch(t.Context(), from, "v2")
		if err != nil {
			t.Fatalf("Fetch(%s): %v", from, err)
		}
		if got := fileSet(t, string(root)); !mapsEqual(got, want) {
			t.Errorf("Fetch(%s) holds %q, want %q", from, got, want)
		}
		if rel, err := filepath.Rel(w.rels, string(root)); err != nil || rel == "." || !filepath.IsLocal(rel) {
			t.Errorf("Fetch(%s) = %s, not in the cache %s", from, root, w.rels)
		}
		roots = append(roots, root)
	}
	if roots[0] == roots[1] || filepath.Base(string(roots[0])) != filepath.Base(string(roots[1])) {
		t.Errorf("the roots %q: want one folder per source, named by the same commit", roots)
	}
	entries, err := os.ReadDir(w.rels)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("the cache holds %s, a fetch's folder left behind", e.Name())
		}
	}
	// a hook planted in the cache's repository does not run on the next fetch
	cache := string(roots[0])[:len(string(roots[0]))-len(filepath.Base(string(roots[0])))-1] + ".git"
	mark := hooked(t, cache)
	if _, err := g.Fetch(t.Context(), w.src, "v1"); err != nil {
		t.Fatal(err)
	}
	noMark(t, mark)
	pins := []string{"protocol.allow=never", "protocol.file.allow=always", "credential.helper=", "transfer.fsckObjects=true", "gc.auto=0"}
	for _, r := range g.Ran() {
		if r.Verb != "fetch-tag" {
			continue
		}
		for _, p := range pins {
			if !slices.Contains(r.Argv, p) {
				t.Errorf("a fetch ran %q, without %s", r.Argv, p)
			}
		}
		if !slices.ContainsFunc(r.Argv, func(a string) bool {
			p, ok := strings.CutPrefix(a, "core.hooksPath=")
			return ok && filepath.IsAbs(p)
		}) {
			t.Errorf("a fetch ran %q, with no hooks path of its own", r.Argv)
		}
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// TestAFetchRefusesASourceOrATagBeforeAnythingRuns: any source but an
// absolute, clean path or an https:// or ssh:// URL as parseURL holds one,
// and a tag that names more than a tag, is refused; none runs git. Planted
// failures: a source passed as it is; refs/ passed as a tag; a URL passed
// with a user on https.
func TestAFetchRefusesASourceOrATagBeforeAnythingRuns(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	refused := []struct{ name, from, tag string }{
		{"a user on https", "https://me@example.com/book", "v1"},
		{"a password", "ssh://me:pw@example.com/book", "v1"},
		{"a host that is an option", "ssh://-oProxyCommand=x/book", "v1"},
		{"a user that is an option", "ssh://-oProxyCommand=x@example.com/book", "v1"},
		{"a path through ..", "https://example.com/a/../book", "v1"},
		{"a query", "https://example.com/book?x=1", "v1"},
		{"a fragment", "https://example.com/book#x", "v1"},
		{"a port of 0", "https://example.com:0/book", "v1"},
		{"a port past 65535", "https://example.com:65536/book", "v1"},
		{"a port with a leading 0", "https://example.com:022/book", "v1"},
		{"no path", "https://example.com", "v1"},
		{"an empty segment", "https://example.com//book", "v1"},
		{"a quote in the path", "https://example.com/bo'ok", "v1"},
		{"a space in the path", "https://example.com/bo ok", "v1"},
		{"a percent in the path", "https://example.com/bo%20ok", "v1"},
		{"an upper-case scheme", "HTTPS://example.com/book", "v1"},
		{"a bracketed host", "ssh://[::1]/book", "v1"},
		{"a transport helper", "ext::sh -c touch% /work/scratch/x", "v1"},
		{"a relative path", "relative/book", "v1"},
		{"a file URL", "file://" + w.src, "v1"},
		{"host:path", "example.com:book", "v1"},
		{"a path through ..", w.src + "/../src", "v1"},
		{"a path with a trailing slash", w.src + "/", "v1"},
		{"a path holding ::", "/a::b", "v1"},
		{"an option", "-u/x", "v1"},
		{"no source", "", "v1"},
		{"http", "http://example.com/b", "v1"},
		{"HEAD as the tag", w.src, "HEAD"},
		{"a ref as the tag", w.src, "refs/heads/main"},
		{"an option as the tag", w.src, "-x"},
		{"a range as the tag", w.src, "v1..v2"},
		{"@ as the tag", w.src, "@"},
		{"no tag", w.src, ""},
		{"a path in the tag", w.src, "v1:x"},
	}
	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			g := w.adapter(t)
			if got, err := g.Fetch(t.Context(), c.from, c.tag); err == nil || got != "" || isNotReady(err) {
				t.Errorf("Fetch(%q, %q) = %q, %v; want refused", c.from, c.tag, got, err)
			}
			if len(g.Ran()) != 0 {
				t.Errorf("Fetch(%q, %q) ran %q", c.from, c.tag, ran(g))
			}
		})
	}
}

// TestAFetchRefusesASourceOthersCouldChangeBeforeAnythingRuns:
// on real folders, a source or a git dir another account can write, a
// linked worktree, whose .git is a file, and a worktree's git dir, which
// holds commondir, are each refused through Fetch, naming the path, and
// none runs git. So are the two sources git would look past, to a folder
// beside them: an empty folder with a bare x.git beside it, and a folder
// that is a git dir by its commondir, whose empty .git git would skip.
// Another account's owner is driven through the facts in source_test.go.
// Planted failure: Fetch not holding its source.
func TestAFetchRefusesASourceOthersCouldChangeBeforeAnythingRuns(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	tree := filepath.Join(w.base, "tree")
	story(t, w.home, w.src, "worktree", "add", "-q", tree, "v1")
	empty := filepath.Join(w.base, "x")
	mkdir(t, empty)
	gitIn(t, w.home, w.base, "clone", "-q", "--bare", w.src, empty+".git")
	common := filepath.Join(w.base, "y")
	writeFile(t, filepath.Join(common, "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(common, "commondir"), filepath.Join(w.src, ".git")+"\n")
	mkdir(t, filepath.Join(common, ".git"))
	// a case that loosens chmod wants the trust walk's own words of it, as
	// the adapter walks it as, since go-files's detail is this system's
	cases := []struct {
		name, from, chmod string
		as                git.Walk
		refuse            string
	}{
		{"a source others can write", w.src, w.src, git.WalkSource, ""},
		{"a git dir others can write", w.src, filepath.Join(w.src, ".git"), git.WalkGitDir, ""},
		{"a linked worktree", tree, "", 0, filepath.Join(tree, ".git") + ", the git dir of the source at " + tree + ", is a file"},
		{"a worktree's git dir", filepath.Join(w.src, ".git", "worktrees", "tree"), "", 0, filepath.Join(w.src, ".git", "worktrees", "tree", "commondir") + " names another folder"},
		{"an empty folder with a bare repository beside it", empty, "", 0, empty + " is no git dir and holds no .git, and git would then open " + empty + ".git beside it"},
		{"an empty .git in a folder that is a git dir by its commondir", common, "", 0, filepath.Join(common, ".git") + " is a folder git would not open as a git dir"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refuse := c.refuse
			if c.chmod != "" {
				trusttest.Loosen(t, c.chmod, trusttest.OthersWrite)
				refuse = git.Walked(t, c.chmod, c.as)
			}
			g := w.adapter(t)
			got, err := g.Fetch(t.Context(), c.from, "v1")
			if rf, ok := errors.AsType[*git.Refused](err); !ok || got != "" || !strings.Contains(rf.Observed(), refuse) {
				t.Errorf("Fetch(%s) = %q, %v; want refused, %q", c.from, got, err, refuse)
			}
			if len(g.Ran()) != 0 {
				t.Errorf("Fetch(%s) ran %q", c.from, ran(g))
			}
		})
	}
	// a folder the hold takes as a git dir, which git does not, as one whose
	// HEAD names nothing, with a bare z.git beside it: git opens the held
	// folder or nothing (upload-pack --strict), so the fetch fails and z.git
	// is not read
	odd := filepath.Join(w.base, "z")
	writeFile(t, filepath.Join(odd, "HEAD"), "not a ref\n")
	mkdir(t, filepath.Join(odd, "objects"))
	mkdir(t, filepath.Join(odd, "refs"))
	gitIn(t, w.home, w.base, "clone", "-q", "--bare", w.src, odd+".git")
	if got, err := w.adapter(t).Fetch(t.Context(), odd, "v1"); err == nil || got != "" {
		t.Errorf("Fetch(%s) = %q, %v; want it failed, not fetched from %s.git", odd, got, err, odd)
	}
	// the source as it was is fetched, so each refusal above is its own
	if _, err := w.adapter(t).Fetch(t.Context(), w.src, "v1"); err != nil {
		t.Errorf("Fetch(%s): %v, want it fetched", w.src, err)
	}
}

// rawTree is a tree object of one file entry named name, for a tree git's
// own tools would not make.
func rawTree(t *testing.T, home, dir, name, blob string) string {
	t.Helper()
	return rawTreeOf(t, home, dir, [2]string{name, blob})
}

// rawTreeOf is a tree object of the file entries, each a name and a blob,
// in the order given, which must be git's.
func rawTreeOf(t *testing.T, home, dir string, entries ...[2]string) string {
	t.Helper()
	var body []byte
	for _, e := range entries {
		oid, err := hex.DecodeString(e[1])
		if err != nil {
			t.Fatal(err)
		}
		body = append(append(body, "100644 "+e[0]+"\x00"...), oid...)
	}
	cmd := exec.CommandContext(t.Context(), "git", "hash-object", "-t", "tree", "-w", "--literally", "--stdin")
	cmd.Dir, cmd.Env, cmd.Stdin = dir, fixtureEnv(home), bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// TestAFetchedTreeThatClimbsIsRefusedAtTheFetch: a tag whose tree holds .., .git
// or a name a disk takes for .git fails at the fetch, which
// checks every object, and nothing of it is in the cache. Planted failure:
// transfer.fsckObjects taken out.
func TestAFetchedTreeThatClimbsIsRefusedAtTheFetch(t *testing.T) {
	t.Parallel()
	// .git with a zero-width non-joiner, GIT~1 and .git. are the names HFS+
	// and NTFS take for .git: the fetch refuses them by strict fsck alone
	for _, name := range []string{"..", ".git", ".GIT", ".git\u200c", "GIT~1", ".git."} {
		t.Run(name, func(t *testing.T) {
			w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
			blob := story(t, w.home, w.src, "rev-parse", "v1:rules.yaml")
			tree := rawTree(t, w.home, w.src, name, blob)
			commit := story(t, w.home, w.src, "commit-tree", tree, "-m", "odd")
			story(t, w.home, w.src, "tag", "bad", commit)
			g := w.adapter(t)
			if got, err := g.Fetch(t.Context(), w.src, "bad"); err == nil || got != "" {
				t.Fatalf("Fetch(bad) = %q, %v; want an error", got, err)
			}
			rs := g.Ran()
			if last := rs[len(rs)-1]; last.Verb != "fetch-tag" || last.Code == 0 {
				t.Errorf("the last run was %s, exit %d; want the fetch to fail", last.Verb, last.Code)
			}
			if err := filepath.WalkDir(w.rels, func(p string, _ fs.DirEntry, err error) error {
				if err == nil && strings.Contains(p, commit) {
					t.Errorf("the cache holds %s", p)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestATagMovedAtItsSourceIsNotFetchedOver: a tag fetched once and moved
// at its source is an error at the next fetch, and the folder of the first
// stays as it was. Planted failure: the refspec forced.
func TestATagMovedAtItsSourceIsNotFetchedOver(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	g := w.adapter(t)
	first, err := g.Fetch(t.Context(), w.src, "v1")
	if err != nil {
		t.Fatal(err)
	}
	commitAll(t, w.home, w.src, map[string]string{"rules.yaml": "moved\n"}, "moved")
	story(t, w.home, w.src, "tag", "-f", "-a", "-m", "v1", "v1")
	if got, err := g.Fetch(t.Context(), w.src, "v1"); err == nil {
		t.Errorf("Fetch of a moved tag = %q; want an error", got)
	}
	if got := fileSet(t, string(first)); !mapsEqual(got, contracttest.TheRelease()) {
		t.Errorf("the first fetch's folder holds %q", got)
	}
}

// TestATagIsAnnotatedAndNeverSigned: the tag is a tag object holding the
// message and no signature, and no hook runs, though the repository's own
// config asks for every tag signed with a signing program that leaves a mark
// and fails (release-pin-by-hash). Planted failures, each
// starting the program so the tag fails or holds a signature: -s put back on
// the tag's row; the tag.gpgSign override taken out; and -a taken out with
// the tag.forceSignAnnotated override (with -a given, git reads no
// forceSignAnnotated, so that override taken out alone stays green).
func TestATagIsAnnotatedAndNeverSigned(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := string(w.root)
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	signed := filepath.Join(w.base, "sign-ran")
	program := filepath.Join(w.base, "sign")
	configtest.Fake(t, filepath.Dir(program), filepath.Base(program), configtest.Step{Write: signed, Text: "ran\n", Append: true}, configtest.Exit(1))
	if err := runs(t, program); err == nil {
		t.Fatal("the planted signer did not fail as planned")
	}
	if _, err := os.Stat(signed); err != nil {
		t.Fatalf("the planted signer left no mark: %v", err)
	}
	if err := os.Remove(signed); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{"tag.gpgSign", "true"}, {"tag.forceSignAnnotated", "true"}, {"gpg.format", "ssh"},
		{"user.signingKey", filepath.Join(w.base, "key")}, {"gpg.ssh.program", program}, {"gpg.program", program},
	} {
		story(t, w.home, dir, "config", "--local", kv[0], kv[1])
	}
	g := w.adapter(t)
	mark := hooked(t, filepath.Join(dir, ".git"))
	if err := g.Tag(t.Context(), w.root, "v2", head, "release v2\n\nthe notes"); err != nil {
		t.Fatal(err)
	}
	noMark(t, mark)
	if b, err := os.ReadFile(signed); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the tag started the signing program: %q, %v", b, err)
	}
	if got := story(t, w.home, dir, "cat-file", "-t", "v2"); got != "tag" {
		t.Errorf("v2 is a %s, want a tag object", got)
	}
	body := story(t, w.home, dir, "cat-file", "tag", "v2")
	if !strings.Contains(body, "object "+head) || !strings.Contains(body, "release v2\n\nthe notes") || strings.Contains(body, "-----BEGIN") {
		t.Errorf("the tag object is %q, want the commit, the message and no signature", body)
	}
}

// TestATagRefusesAndMakesNone: each of these is an error and runs no tag:
// a name that is no tag's, a commit that is no full object name, is not
// in the repository or is not a commit, a tag object among them, a message that is empty or holds a
// control character, and a tag already there, which stays. Planted
// failures: each check taken out in turn.
func TestATagRefusesAndMakesNone(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := string(w.root)
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	tree := story(t, w.home, dir, "rev-parse", "HEAD^{tree}")
	story(t, w.home, dir, "tag", "-a", "-m", "v1", "v1")
	v1 := story(t, w.home, dir, "rev-parse", "v1")
	cases := []struct {
		name, tag, commit, message string
		kind                       func(error) bool
	}{
		{name: "HEAD as a name", tag: "HEAD"},
		{name: "a ref as a name", tag: "refs/heads/v2"},
		{name: "an option as a name", tag: "-f"},
		{name: "a short commit", commit: head[:12]},
		{name: "a rev as the commit", commit: "HEAD"},
		{name: "a commit not there", commit: strings.Repeat("ab", 20)},
		{name: "a tree as the commit", commit: tree},
		{name: "a tag object as the commit", commit: v1},
		{name: "an empty message", message: "-"},
		{name: "an escape in the message", message: "\x1b[2Jgone"},
		{name: "a tag already there", tag: "v1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tag, commit, message := or(c.tag, "v2"), or(c.commit, head), or(c.message, "release")
			g := w.adapter(t)
			err := g.Tag(t.Context(), w.root, tag, commit, message)
			if err == nil || c.kind != nil && !c.kind(err) {
				t.Fatalf("Tag: %v; want an error of its kind", err)
			}
			if slices.Contains(ran(g), "tag-annotated") {
				t.Errorf("a refused tag ran %q", ran(g))
			}
			if got := story(t, w.home, dir, "tag", "-l"); got != "v1" {
				t.Errorf("the tags are %q, want v1 alone", got)
			}
			if got := story(t, w.home, dir, "rev-parse", "v1"); got != v1 {
				t.Errorf("v1 moved to %s", got)
			}
		})
	}
}

// or is s, or def when s is "", and "" when s is "-".
func or(s, def string) string {
	switch s {
	case "":
		return def
	case "-":
		return ""
	}
	return s
}

func isRefused(err error) bool {
	_, ok := errors.AsType[*git.Refused](err)
	return ok
}

// TestAFetchedTreeWhoseNamesFoldTogetherKeepsBoth: a tag whose tree holds
// A and a is refused on a disk that folds case, since the second file is
// made new and A is there, and nothing of it is in the cache; on a disk
// that does not, each holds its own content. Planted failure: the file
// opened with O_TRUNC in place of O_EXCL, so a overwrites A.
func TestAFetchedTreeWhoseNamesFoldTogetherKeepsBoth(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	upper := story(t, w.home, w.src, "rev-parse", "v1:rules.yaml")
	lower := story(t, w.home, w.src, "rev-parse", "v1:presets/a.json")
	tree := rawTreeOf(t, w.home, w.src, [2]string{"A", upper}, [2]string{"a", lower})
	commit := story(t, w.home, w.src, "commit-tree", tree, "-m", "fold")
	story(t, w.home, w.src, "tag", "fold", commit)
	g := w.adapter(t)
	got, err := g.Fetch(t.Context(), w.src, "fold")
	if !folds(t, w.base) {
		if err != nil {
			t.Fatalf("Fetch(fold) on a disk that keeps case: %v", err)
		}
		want := map[string]string{"A": contracttest.TheRelease()["rules.yaml"], "a": contracttest.TheRelease()["presets/a.json"]}
		if files := fileSet(t, string(got)); !mapsEqual(files, want) {
			t.Errorf("Fetch(fold) holds %q, want %q", files, want)
		}
		return
	}
	if err == nil || got != "" {
		t.Fatalf("Fetch(fold) on a disk that folds case = %q, %v; want an error", got, err)
	}
	noneCached(t, w.rels, commit)
}

// folds is whether the disk under dir takes two names that differ in case
// alone for one.
func folds(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "Fold-probe")
	writeFile(t, probe, "")
	_, err := os.Lstat(filepath.Join(dir, "fold-PROBE"))
	if rerr := os.Remove(probe); rerr != nil {
		t.Fatal(rerr)
	}
	return err == nil
}

// noneCached fails t when anything under rels names commit, or a fetch's
// own folder is left there.
func noneCached(t *testing.T, rels, commit string) {
	t.Helper()
	if err := filepath.WalkDir(rels, func(p string, _ fs.DirEntry, err error) error {
		if err == nil && (strings.Contains(p, commit) || strings.HasPrefix(filepath.Base(p), ".fetch-")) {
			t.Errorf("the cache holds %s", p)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAFetchedFileOfAnotherSizeThanItsListingIsRefused: a git whose
// cat-file prints a byte more than the tree's listing says is an error,
// and nothing of the tag is in the cache. Planted failure: the size check
// taken out.
func TestAFetchedFileOfAnotherSizeThanItsListingIsRefused(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := filepath.Join(filestest.Private(t), "bin")
	mkdir(t, dir)
	real := configtest.Git(t)
	liar := git.StandIn(t, dir,
		git.Rule{Has: []string{" cat-file blob "}, Answer: git.Answer{Run: real, Out: "x"}},
		git.Always(git.Answer{Run: real}))
	g := w.adapterUsing(t, resolved(t, liar))
	got, err := g.Fetch(t.Context(), w.src, "v1")
	if err == nil || got != "" {
		t.Fatalf("Fetch through a git that prints more = %q, %v; want an error", got, err)
	}
	if !strings.Contains(err.Error(), "printed what it does not print") {
		t.Errorf("Fetch: %v; want it to say git printed what it does not print", err)
	}
	noneCached(t, w.rels, story(t, w.home, w.src, "rev-parse", "v1^{commit}"))
}

// TestACachedReleaseThatIsNoFolderIsAnError: when the cache holds a file
// where a fetched tag's folder goes, the fetch is an error, and the file is
// not answered as the release. Planted failure: the folder check taken out.
func TestACachedReleaseThatIsNoFolderIsAnError(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	first, err := w.adapter(t).Fetch(t.Context(), w.src, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(string(first)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, string(first), "not a release\n")
	if got, err := w.adapter(t).Fetch(t.Context(), w.src, "v1"); err == nil || got != "" {
		t.Errorf("Fetch over a file in the cache = %q, %v; want an error", got, err)
	}
}

// TestANamedFileSwappedBetweenTheCheckAndTheOpenIsRefused: a regular file
// put over a named path after its check and before its open is another
// file, so the commit is refused, no commit is made and HEAD stays. Planted
// failure: the SameFile check taken out, so the swapped file is committed.
func TestANamedFileSwappedBetweenTheCheckAndTheOpenIsRefused(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	dir := string(w.root)
	head := story(t, w.home, dir, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(dir, "rules.yaml"), "two\n")
	writeFile(t, filepath.Join(dir, "other.txt"), "not what was checked\n")
	g := w.adapter(t)
	git.Opening(g, func(name string) {
		if err := os.Rename(filepath.Join(dir, "other.txt"), filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	})
	got, err := g.Commit(t.Context(), w.root, []string{"rules.yaml"}, "m")
	if err == nil || got != "" || !strings.Contains(err.Error(), "it changed between the check and the open") {
		t.Fatalf("Commit of a swapped file = %q, %v; want it refused as changed between the check and the open", got, err)
	}
	if slices.Contains(ran(g), "commit-tree") || slices.Contains(ran(g), "update-ref") {
		t.Errorf("a refused commit ran %q", ran(g))
	}
	if now := story(t, w.home, dir, "rev-parse", "HEAD"); now != head {
		t.Errorf("HEAD moved to %s", now)
	}
}

// TestACommitWithANonASCIIFileInHead: HEAD holds naïve.md and
// a second non-ASCII name, and a Commit of another path succeeds, with
// both still in the commit's tree. Planted failure: a listing HEAD is read
// through that quotes a path, so the commit is refused for the name.
func TestACommitWithANonASCIIFileInHead(t *testing.T) {
	t.Parallel()
	// A second name git quotes, one every system keeps: a double quote is no
	// name's letter where Windows names files, so the quote is not in a name.
	filestest.AllPortable(t, "naïve.md", "日本.md")
	w := newRemote(t, map[string]string{"rules.yaml": "one\n", "naïve.md": "n\n", "日本.md": "q\n"})
	dir := string(w.root)
	writeFile(t, filepath.Join(dir, "rules.yaml"), "two\n")
	id, err := w.adapter(t).Commit(t.Context(), w.root, []string{"rules.yaml"}, "release")
	if err != nil {
		t.Fatalf("Commit with naïve.md in HEAD: %v, want it made", err)
	}
	// -z leaves a name unquoted, which the plain --name-only listing does not.
	got := strings.Split(strings.TrimSuffix(story(t, w.home, dir, "ls-tree", "-r", "-z", "--name-only", id), "\x00"), "\x00")
	if want := []string{"naïve.md", "rules.yaml", "日本.md"}; !slices.Equal(got, want) {
		t.Errorf("the commit holds %q, want %q", got, want)
	}
}
