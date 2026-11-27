package git_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/birdtable/go-files/filestest"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// story runs real git in dir for a fixture's set-up, as gitIn does, with
// the fixture's identity and date, so every object name is the same on
// every machine. It answers what git prints.
func story(t *testing.T, home, dir string, args ...string) string {
	t.Helper()
	return storyIn(t, home, dir, "", args...)
}

// storyIn is story with stdin given to git, and the output as story answers it, trimmed of space.
func storyIn(t *testing.T, home, dir, stdin string, args ...string) string {
	t.Helper()
	out, stderr, err := storyTry(t, home, dir, stdin, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return out
}

// storyTry is storyIn that answers git's failure, with what it printed on
// stderr, in place of ending the test.
func storyTry(t *testing.T, home, dir, stdin string, args ...string) (out, stderr string, err error) {
	t.Helper()
	who := contracttest.FixtureWho()
	cmd := exec.CommandContext(t.Context(), "git")
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(fixtureEnv(home), "LC_ALL=C",
		"GIT_AUTHOR_NAME="+who.Name, "GIT_AUTHOR_EMAIL="+who.Email, "GIT_AUTHOR_DATE="+contracttest.FixtureDate,
		"GIT_COMMITTER_NAME="+who.Name, "GIT_COMMITTER_EMAIL="+who.Email, "GIT_COMMITTER_DATE="+contracttest.FixtureDate,
	)
	b, err := cmd.Output()
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		stderr = string(ee.Stderr)
	}
	return strings.TrimSpace(string(b)), stderr, err
}

// file writes text to a new file name in dir, outside the work tree, and
// answers its path.
func file(t *testing.T, dir, name, text string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// write makes the work tree of dir hold exactly files, beside .git.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != ".git" {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// makeHistory makes the repository f in a home of its own with real git, and
// fails when a commit's object name is not the one f gives it. A raw
// commit is made by git commit with its name for a message, then written
// again by hash-object with the fixture's message in its place, which git
// commit would check.
func makeHistory(t *testing.T, f contracttest.HistoryFixture) (home string, root model.Root) {
	t.Helper()
	home, root = fixture(t, nil)
	dir := string(root)
	// fixture ran git init; start again with the fixture's branch and format
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	story(t, home, dir, "init", "-q", "--object-format=sha1", "-b", f.Commits[0].Branch)
	ids := map[string]string{}
	branches := map[string]bool{f.Commits[0].Branch: true}
	at := f.Commits[0].Branch
	for _, c := range f.Commits {
		switch {
		case c.Branch == at:
		case branches[c.Branch]:
			story(t, home, dir, "checkout", "-q", "-f", c.Branch)
		case c.From != "":
			story(t, home, dir, "checkout", "-q", "-f", "-b", c.Branch, ids[c.From])
		default:
			story(t, home, dir, "checkout", "-q", "-f", "--orphan", c.Branch)
		}
		at, branches[c.Branch] = c.Branch, true
		if c.Merge != "" {
			story(t, home, dir, "merge", "-q", "--no-ff", "--no-commit", ids[c.Merge])
		}
		write(t, dir, c.Files)
		story(t, home, dir, "add", "-A", ".")
		for p, to := range c.Links {
			blob := story(t, home, dir, "hash-object", "-w", file(t, home, "link-"+c.Name, to))
			story(t, home, dir, "update-index", "--add", "--cacheinfo", "120000,"+blob+","+p)
		}
		msg := c.Stored()
		if c.Raw {
			msg = c.Name + "\n"
		}
		commit := []string{"commit", "-q", "--no-verify", "--cleanup=verbatim", "-F", file(t, home, "message-"+c.Name, msg)}
		if c.Encoding != "" {
			commit = append([]string{"-c", "i18n.commitEncoding=" + c.Encoding}, commit...)
		}
		story(t, home, dir, commit...)
		if c.Raw {
			head, _, _ := strings.Cut(story(t, home, dir, "cat-file", "commit", "HEAD"), "\n\n")
			obj := file(t, home, "object-"+c.Name, head+"\n\n"+c.Stored())
			story(t, home, dir, "update-ref", "HEAD", story(t, home, dir, "hash-object", "--literally", "-t", "commit", "-w", obj))
		}
		ids[c.Name] = story(t, home, dir, "rev-parse", "HEAD")
		if ids[c.Name] != c.ID {
			t.Fatalf("the commit %s is %s, and the fixture says %s", c.Name, ids[c.Name], c.ID)
		}
	}
	for _, tag := range f.Tags {
		if tag.Annotated {
			story(t, home, dir, "tag", "-a", "-m", tag.Name, tag.Name, ids[tag.Commit])
		} else {
			story(t, home, dir, "tag", tag.Name, ids[tag.Commit])
		}
	}
	story(t, home, dir, "checkout", "-q", "-f", f.Branch)
	write(t, dir, f.Index)
	story(t, home, dir, "add", "-A", ".")
	for p, to := range f.IndexLinks {
		blob := story(t, home, dir, "hash-object", "-w", file(t, home, "index-link", to))
		story(t, home, dir, "update-index", "--add", "--cacheinfo", "120000,"+blob+","+p)
	}
	if f.Work != nil {
		write(t, dir, f.Work)
	}
	for p, to := range f.WorkLinks {
		testenv.Link(t, to, filepath.Join(dir, filepath.FromSlash(p)))
	}
	return home, root
}

// TestTheGitAdapterPassesTheHistorySuite runs the History suite on real
// git over a repository it builds. Planted failure: the index answered as
// HEAD's tree.
func TestTheGitAdapterPassesTheHistorySuite(t *testing.T) {
	t.Parallel()
	contracttest.History(t, func(t *testing.T, f contracttest.HistoryFixture) (port.History, model.Root) {
		t.Helper()
		home, root := makeHistory(t, f)
		return adapter(t, configtest.Git(t), home), root
	})
}

// TestARepositoryWithNoTagHasAnEmptyList: none is an empty list, never nil
// and never an error. Planted failure: nil answered.
func TestARepositoryWithNoTagHasAnEmptyList(t *testing.T) {
	t.Parallel()
	home, root := fixture(t, nil)
	contracttest.NoTags(t, adapter(t, configtest.Git(t), home), root)
}

// TestEveryHistoryRunIsOfTheTableWithThePrefix: each run of each method is a
// verb of the table, by the absolute path, with the prefix, and, on a git
// of 2.44 or later, --no-lazy-fetch on every verb that reads objects. The
// version is read once. Planted failure: the prefix left off a verb.
func TestEveryHistoryRunIsOfTheTableWithThePrefix(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	g := adapter(t, configtest.Git(t), home)
	if _, err := g.Base(t.Context(), root, "v2", "side"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Blob(t.Context(), root, "v1", "README"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Blob(t.Context(), root, "", "README"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Tags(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Merging(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Commits(t.Context(), root, "v1", "v2"); err != nil {
		t.Fatal(err)
	}
	var verbs []string
	for _, r := range g.Ran() {
		verbs = append(verbs, r.Verb)
		if !filepath.IsAbs(r.Argv[0]) || r.Dir != string(root) || r.Code != 0 {
			t.Errorf("the run %s: want git by its absolute path, in the root, exit 0", git.Shown([]git.Ran{r}))
		}
		pre := []string{"--no-replace-objects", "-c", "core.commitGraph=false", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "advice.graftFileDeprecated=false"}
		if reads := r.Verb != "version" && r.Verb != "is-shallow" && r.Verb != "config-promisor" && r.Verb != "merge-head-path" && r.Verb != "work-top"; reads {
			pre = append([]string{"--no-lazy-fetch"}, pre...)
		}
		if len(r.Argv) < len(pre)+1 || !slices.Equal(r.Argv[1:len(pre)+1], pre) {
			t.Errorf("the run %s: %q, want it to start %q", r.Verb, r.Argv, pre)
		}
	}
	want := []string{
		"version", "is-shallow", "resolve-commit", "resolve-commit", "merge-base",
		"work-top", "resolve-commit", "tree-files", "blob", "work-top", "index-files", "blob", "tags",
		"merge-head-path", "is-shallow", "resolve-commit", "resolve-commit", "commits-between",
	}
	if !slices.Equal(verbs, want) {
		t.Errorf("the runs were %q, want %q", verbs, want)
	}
}

// TestARevThatCouldBeAnOptionIsRefusedBeforeAnythingRuns: a rev or a path
// that could be read as an option, or as more than a name, is refused, and
// git never starts. Planted failure: the rev passed to git unchecked.
func TestARevThatCouldBeAnOptionIsRefusedBeforeAnythingRuns(t *testing.T) {
	t.Parallel()
	home, root := fixture(t, nil)
	g := adapter(t, configtest.Git(t), home)
	for _, rev := range []string{"-n", "--output=/work/scratch/x", "v1..v2", "v1^", "v1~1", "HEAD@{1}", "v1:README", ":/x", "a b", "a\nb", "/abs", ".hidden", "x.lock", "x/", strings.Repeat("a", 256), "\x1b[31m"} {
		if _, err := g.Files(t.Context(), root, rev); err == nil {
			t.Errorf("Files(%q) answered", rev)
		}
		if _, err := g.Base(t.Context(), root, "main", rev); err == nil {
			t.Errorf("Base(main, %q) answered", rev)
		}
		if _, err := g.Blob(t.Context(), root, rev, "README"); err == nil {
			t.Errorf("Blob(%q, README) answered", rev)
		}
		if _, err := g.Commits(t.Context(), root, "main", rev); err == nil {
			t.Errorf("Commits(main, %q) answered", rev)
		}
		if _, err := g.Commits(t.Context(), root, rev, "main"); err == nil {
			t.Errorf("Commits(%q, main) answered", rev)
		}
	}
	for _, r := range pathTable(t, "refused-paths.txt") {
		if _, err := g.Blob(t.Context(), root, "main", r.path); err == nil || errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Blob(main, %q), %s: %v, want refused and not fs.ErrNotExist", r.path, r.what, err)
		}
	}
	if _, err := g.Base(t.Context(), root, "", "main"); err == nil {
		t.Error(`Base("", main) answered: the index has no merge base`)
	}
	if len(g.Ran()) != 0 {
		t.Errorf("a refused value ran git: %s", git.Shown(g.Ran()))
	}
}

// TestARevTheRepositoryLacksIsMissingNotNotExist: a rev that names nothing
// is a *Missing naming it. Planted failure: answered as fs.ErrNotExist.
func TestARevTheRepositoryLacksIsMissingNotNotExist(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	g := adapter(t, configtest.Git(t), home)
	for _, rev := range []string{"v9", "nowhere", strings.Repeat("0", 40)} {
		_, err := g.Files(t.Context(), root, rev)
		m, ok := errors.AsType[*git.Missing](err)
		if !ok || m.Rev != rev || errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Files(%s): %v, want *Missing naming it", rev, err)
		}
	}
	if _, err := g.Base(t.Context(), root, "v9", "main"); !isMissing(err) {
		t.Errorf("Base(v9, main): %v, want *Missing", err)
	}
}

func isMissing(err error) bool {
	_, ok := errors.AsType[*git.Missing](err)
	return ok
}

// TestNoCommitInCommonIsNoBase: histories with none in common are a *NoBase
// of 0, never a name. Planted failure: exit 1 read as a failure.
func TestNoCommitInCommonIsNoBase(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	_, err := adapter(t, configtest.Git(t), home).Base(t.Context(), root, "main", "lone")
	if nb, ok := errors.AsType[*git.NoBase](err); !ok || nb.Bases != 0 || nb.A != "main" || nb.B != "lone" {
		t.Errorf("Base(main, lone): %v, want *NoBase of none", err)
	}
	if !errors.Is(err, port.ErrNoBase) || errors.Is(err, port.ErrNoRev) {
		t.Errorf("Base(main, lone): %v matches ErrNoBase %t and ErrNoRev %t; want only ErrNoBase", err, errors.Is(err, port.ErrNoBase), errors.Is(err, port.ErrNoRev))
	}
}

// TestAGraftFileSwapsNoParent: a
// repository's info/grafts, which git still reads and which gives a commit
// other parents than its object holds, moves neither the merge base nor a
// range: lone grafted onto one, and join onto two alone, are read as their
// objects are. The graft is shown live by git run as a person runs it.
// Planted failure: the graft file left to the repository.
func TestAGraftFileSwapsNoParent(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	id := map[string]string{}
	for _, c := range contracttest.TheHistory().Commits {
		id[c.Name] = c.ID
	}
	dir := string(root)
	grafts := id["lone"] + " " + id["one"] + "\n" + id["join"] + " " + id["two"] + "\n"
	// git init makes info from its templates, and a git may have none
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "grafts"), []byte(grafts), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := story(t, home, dir, "-c", "advice.graftFileDeprecated=false", "merge-base", "main", "lone"); got != id["one"] {
		t.Fatalf("git with the graft: merge-base(main, lone) = %q, want %s, the graft live", got, id["one"])
	}
	g := adapter(t, configtest.Git(t), home)
	if _, err := g.Base(t.Context(), root, "main", "lone"); !errors.As(err, new(*git.NoBase)) {
		t.Errorf("Base(main, lone) with a graft: %v, want *NoBase, the objects' parents", err)
	}
	msgs, err := g.Messages(t.Context(), root, id["one"], id["join"])
	var got []string
	for _, m := range msgs {
		got = append(got, m.Commit)
	}
	want := []string{id["two"], id["side"], id["join"]}
	slices.Sort(want)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("Messages(one, join) with a graft = %q, %v; want %q, through side", got, err, want)
	}
}

// graphParentNone is what a commit-graph file stores for no parent.
const graphParentNone = 0x70000000

// unparent rewrites the commit-graph file git wrote in dir so that the
// commit id has no second parent, though its object keeps one: the chunk
// table locates the sorted object names (OIDL) and the commit data (CDAT),
// whose entry is the tree's name, the first and second parents'
// positions and the generation and date. Dropping an edge keeps every
// generation number true, so git walks the graph as it is written.
func unparent(t *testing.T, dir, id string) {
	t.Helper()
	path := filepath.Join(dir, ".git", "objects", "info", "commit-graph")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 || string(data[:4]) != "CGPH" || data[5] != 1 {
		t.Fatalf("the commit-graph is not a version-1 file of SHA-1 names: %q", data[:min(8, len(data))])
	}
	chunks := map[string]uint64{}
	for i := range int(data[6]) {
		e := data[8+12*i:]
		chunks[string(e[:4])] = binary.BigEndian.Uint64(e[4:12])
	}
	oidl, cdat := chunks["OIDL"], chunks["CDAT"]
	want, err := hexBytes(id)
	if err != nil || oidl == 0 || cdat == 0 {
		t.Fatalf("chunks %v, the name %s: %v", chunks, id, err)
	}
	for i := 0; oidl+uint64(20*(i+1)) <= cdat; i++ {
		if bytes.Equal(data[oidl+uint64(20*i):oidl+uint64(20*(i+1))], want) {
			second := data[cdat+uint64(36*i)+24:]
			if binary.BigEndian.Uint32(second) == graphParentNone {
				t.Fatalf("the commit %s has no second parent in the graph", id)
			}
			binary.BigEndian.PutUint32(second, graphParentNone)
			// git writes the graph read-only
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("the commit %s is not in the graph", id)
}

// hexBytes is the bytes a full object name in hex spells.
func hexBytes(id string) ([]byte, error) {
	var out []byte
	_, err := fmt.Sscanf(id, "%x", &out)
	return out, err
}

// TestACommitGraphSwapsNoParent: a
// commit-graph file whose parent lists disagree with the objects moves
// neither the merge base nor a range. join merges two into side; the graph
// written by git is changed so join has side alone. git run as a person
// trusts it; the adapter reads the objects. Planted failure: -c
// core.commitGraph=false left out of the prefix.
//
// Every query names after, a child of join, so join is reached by the walk.
// git 2.47.3 reads a commit named on its command line from its object,
// whatever the graph says, and only the commits it walks to from the graph;
// 2.56.0 reads both from the graph. A query naming join itself shows the
// graph live on the newer git alone.
func TestACommitGraphSwapsNoParent(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	id := map[string]string{}
	for _, c := range contracttest.TheHistory().Commits {
		id[c.Name] = c.ID
	}
	dir := string(root)
	story(t, home, dir, "commit-graph", "write", "--reachable")
	unparent(t, dir, id["join"])
	after := story(t, home, dir, "commit-tree", id["join"]+"^{tree}", "-p", id["join"], "-m", "after")
	if got := story(t, home, dir, "merge-base", id["two"], after); got != id["one"] {
		t.Fatalf("git with the graph: merge-base(two, after) = %q, want %s, the graph live", got, id["one"])
	}
	g := adapter(t, configtest.Git(t), home)
	if got, err := g.Base(t.Context(), root, id["two"], after); err != nil || got != id["two"] {
		t.Errorf("Base(two, after) with the graph = %q, %v; want %s, the objects' parents", got, err, id["two"])
	}
	msgs, err := g.Messages(t.Context(), root, id["one"], after)
	var got []string
	for _, m := range msgs {
		got = append(got, m.Commit)
	}
	want := []string{id["two"], id["side"], id["join"], after}
	slices.Sort(want)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("Messages(one, after) with the graph = %q, %v; want %q, through two", got, err, want)
	}
}

// TestTwoMergeBasesAreAnErrorAndNoneIsPicked: after a criss-cross merge two
// commits are each a best common ancestor; Base refuses rather than picking
// one. Planted failure: merge-base without --all, or the first line read.
func TestTwoMergeBasesAreAnErrorAndNoneIsPicked(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	dir := string(root)
	// a and b both start at main; each merges the other's first commit
	story(t, home, dir, "checkout", "-q", "-f", "-b", "a", "main")
	story(t, home, dir, "commit", "-q", "--allow-empty", "-m", "a1")
	story(t, home, dir, "checkout", "-q", "-f", "-b", "b", "main")
	story(t, home, dir, "commit", "-q", "--allow-empty", "-m", "b1")
	story(t, home, dir, "checkout", "-q", "-f", "a")
	story(t, home, dir, "merge", "-q", "--no-ff", "-m", "a2", "b~0")
	story(t, home, dir, "checkout", "-q", "-f", "b")
	story(t, home, dir, "merge", "-q", "--no-ff", "-m", "b2", "a~1")
	_, err := adapter(t, configtest.Git(t), home).Base(t.Context(), root, "a", "b")
	if nb, ok := errors.AsType[*git.NoBase](err); !ok || nb.Bases != 2 {
		t.Errorf("Base(a, b) after a criss-cross: %v, want *NoBase of 2", err)
	}
	if !errors.Is(err, port.ErrNoBase) {
		t.Errorf("Base(a, b) after a criss-cross: %v does not match ErrNoBase", err)
	}
}

// TestALinkIsMarkedAndASubmoduleIsNotAFile: a tree's link is in Files with
// Link set, from git's mode field, and its submodule is left out; Blob of
// either matches fs.ErrNotExist; an executable file is Exec. Planted
// failure: the modes not read.
func TestALinkIsMarkedAndASubmoduleIsNotAFile(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	dir := string(root)
	testenv.Link(t, "README", filepath.Join(dir, "link"))
	filestest.WriteProgram(t, filepath.Join(dir, "run.sh"), []byte("run\n"))
	story(t, home, dir, "add", "link", "run.sh")
	story(t, home, dir, "update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("ab", 20)+",sub")
	story(t, home, dir, "commit", "-q", "-m", "links")
	story(t, home, dir, "tag", "v3")
	g := adapter(t, configtest.Git(t), home)
	for _, rev := range []string{"v3", ""} {
		files, err := g.Files(t.Context(), root, rev)
		if err != nil {
			t.Fatal(err)
		}
		var paths, links []string
		exec := false
		for _, f := range files {
			paths = append(paths, f.Path)
			exec = exec || f.Path == "run.sh" && f.Exec
			if f.Link {
				links = append(links, f.Path)
			}
		}
		if slices.Contains(paths, "sub") || !slices.Contains(paths, "run.sh") || testenv.KeepsModes(t) && !exec {
			t.Errorf("Files(%q) = %q, want run.sh, executable, and no sub", rev, paths)
		}
		// ln is the link the fixture's index stages
		if !slices.Equal(links, []string{"link", "ln"}) {
			t.Errorf("Files(%q) marks %q a link, want [link ln] alone", rev, links)
		}
		for _, p := range []string{"link", "ln", "sub"} {
			if _, err := g.Blob(t.Context(), root, rev, p); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Blob(%q, %s): %v, want fs.ErrNotExist", rev, p, err)
			}
		}
	}
}

// TestANameGitWouldQuoteIsListedAndRead: the
// listing runs with -z and no --format, so git prints a path as it is, and a
// tree holding a name with a non-ASCII letter or a double quote lists and
// reads. Planted failure: --format put back, whose %(path) is quoted whatever
// -z says, so both names come back in quotes and the listing is failed.
func TestANameGitWouldQuoteIsListedAndRead(t *testing.T) {
	t.Parallel()
	for name, p := range map[string]string{"a non-ASCII letter": "naïve.md", "a double quote": `a"b.md`} {
		t.Run(name, func(t *testing.T) {
			home, root := makeHistory(t, contracttest.TheHistory())
			dir := string(root)
			blob := story(t, home, dir, "rev-parse", "v1:README")
			entries := storyIn(t, home, dir, "", "ls-tree", "-z", "HEAD") + "100644 blob " + blob + "\t" + p + "\x00"
			tree := storyIn(t, home, dir, entries, "mktree", "-z")
			story(t, home, dir, "update-ref", "HEAD", story(t, home, dir, "commit-tree", "-p", "HEAD", "-m", "named", tree))
			g := adapter(t, configtest.Git(t), home)
			files, err := g.Files(t.Context(), root, "HEAD")
			if err != nil {
				t.Fatalf("Files(HEAD) with %q: %v, want it listed", p, err)
			}
			if !slices.ContainsFunc(files, func(e model.Entry) bool { return e.Path == p }) {
				t.Errorf("Files(HEAD) lacks %q: %v", p, files)
			}
			want := story(t, home, dir, "cat-file", "blob", blob)
			if got, err := g.Blob(t.Context(), root, "HEAD", p); err != nil || strings.TrimSuffix(string(got), "\n") != want {
				t.Errorf("Blob(HEAD, %q): %q, %v; want %q", p, got, err, want)
			}
		})
	}
}

// TestAnOddPathInATreeIsRefused: a path git allows in a tree but this
// adapter does not pass on, a control character or a backslash, fails the
// listing with the run id, rather than reaching a caller. The listing is not
// quoted, so each is refused for what it holds: the escape and the newline
// by checkPath's control-character test, the backslash for itself. Planted
// failure: the path not checked.
func TestAnOddPathInATreeIsRefused(t *testing.T) {
	t.Parallel()
	for _, r := range pathTable(t, "odd-tree-paths.txt") {
		p := r.path
		// What the refusal names: the first rune checkPath stops at, the escape and the newline as
		// control characters, the backslash for itself.
		holds := "holds " + strconv.QuoteRune(oddRune(p))
		t.Run(r.what, func(t *testing.T) {
			home, root := makeHistory(t, contracttest.TheHistory())
			dir := string(root)
			// The tree is made by mktree, which makes no check of a path, and not by update-index, whose
			// own check on a system with a protected name refuses some of these before the adapter sees them.
			blob := story(t, home, dir, "rev-parse", "v1:README")
			entries := storyIn(t, home, dir, "", "ls-tree", "-z", "HEAD") + "100644 blob " + blob + "\t" + p + "\x00"
			tree := storyIn(t, home, dir, entries, "mktree", "-z")
			// The commit and its listing come first: Files(HEAD) reads no index, so it is proved even where
			// read-tree puts the path through the index check of the system (core.protectNTFS refuses a
			// backslash) and the set-up of the index half stops there.
			story(t, home, dir, "update-ref", "HEAD", story(t, home, dir, "commit-tree", "-p", "HEAD", "-m", "odd", tree))
			g := adapter(t, configtest.Git(t), home)
			if _, err := g.Files(t.Context(), root, "HEAD"); !isFailed(err) || !strings.Contains(err.Error(), holds) {
				t.Errorf("Files(HEAD) with %q: %v, want failed, the path %s", p, err, holds)
			}
			// The index half: git's own index check, where the system keeps one, may refuse the path
			// (core.protectNTFS refuses a backslash and a control character), and then the path never
			// reaches the index and no caller sees it either way. That refusal is wanted in git's words,
			// so a read-tree that fails for any other reason still fails the test.
			_, stderr, rerr := storyTry(t, home, dir, "", "read-tree", tree)
			if rerr != nil && !strings.Contains(stderr, "invalid path") {
				t.Fatalf("git read-tree: %v\n%s", rerr, stderr)
			}
			files, err := g.Files(t.Context(), root, "")
			f, failed := errors.AsType[*git.Failed](err)
			switch {
			case failed && f.Run() == "run-1":
			case rerr != nil && err == nil && !slices.ContainsFunc(files, func(e model.Entry) bool { return e.Path == p }):
			default:
				t.Errorf("Files(index) with %q (git's index check: %v): %v, want failed with the run id", p, rerr, err)
			}
		})
	}
}

// oddRune is the first rune of p that checkPath refuses: a control character or a backslash.
func oddRune(p string) rune {
	for _, r := range p {
		if unicode.IsControl(r) || r == '\\' {
			return r
		}
	}
	return utf8.RuneError
}

func isFailed(err error) bool {
	_, ok := errors.AsType[*git.Failed](err)
	return ok
}

// TestAMergeInProgressIsNotReady: an index with a path at stages 1 to 3 has
// no one file there, so Files of the index is not ready naming the merge.
// Planted failure: the stage not read, so the path is listed three times.
func TestAMergeInProgressIsNotReady(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	dir := string(root)
	story(t, home, dir, "checkout", "-q", "-f", "side")
	write(t, dir, map[string]string{"rules/a.yaml": "side\n", "side.txt": "side\n"})
	story(t, home, dir, "commit", "-q", "-a", "-m", "side two")
	cmd := exec.CommandContext(t.Context(), "git", "merge", "-q", "main")
	cmd.Dir = dir
	cmd.Env = append(fixtureEnv(home), "GIT_AUTHOR_NAME=x", "GIT_AUTHOR_EMAIL=x@example.com", "GIT_COMMITTER_NAME=x", "GIT_COMMITTER_EMAIL=x@example.com")
	if err := cmd.Run(); err == nil {
		t.Fatal("the fixture's merge had no conflict")
	}
	_, err := adapter(t, configtest.Git(t), home).Files(t.Context(), root, "")
	if nr, ok := errors.AsType[*git.NotReady](err); !ok || !strings.Contains(nr.Observed(), "merge in progress") || !strings.Contains(nr.Hint(), "git merge --abort") {
		t.Errorf("Files(index) mid-merge: %v, want not ready naming the merge", err)
	}
}

// TestAShallowCloneIsNotReady: in a clone of depth 1, Base is not ready
// before merge-base runs, and a rev past the cut is not ready naming
// git fetch --unshallow, not *Missing; a rev in the clone still reads.
// Planted failure: the shallow check gone, so merge-base answers.
func TestAShallowCloneIsNotReady(t *testing.T) {
	t.Parallel()
	home, src := makeHistory(t, contracttest.TheHistory())
	dir := filepath.Join(filepath.Dir(string(src)), "shallow")
	story(t, home, filepath.Dir(dir), "clone", "-q", "--depth", "1", "--branch", "main", "file://"+string(src), dir)
	root := model.Root(dir)
	g := adapter(t, configtest.Git(t), home)
	_, err := g.Base(t.Context(), root, "main", "main")
	if nr, ok := errors.AsType[*git.NotReady](err); !ok || nr.Hint() != "git fetch --unshallow" {
		t.Errorf("Base in a shallow clone: %v, want not ready naming git fetch --unshallow", err)
	}
	for _, r := range g.Ran() {
		if r.Verb == "merge-base" {
			t.Errorf("merge-base ran in a shallow clone")
		}
	}
	for _, rev := range []string{"v1", contracttest.TheHistory().Commits[0].ID} {
		if _, err := g.Files(t.Context(), root, rev); !isNotReady(err) {
			t.Errorf("Files(%s) past the cut: %v, want not ready", rev, err)
		}
	}
	if got, err := g.Blob(t.Context(), root, "main", "rules/a.yaml"); err != nil || string(got) != "two\n" {
		t.Errorf("Blob(main) in the clone: %q, %v", got, err)
	}
}

func isNotReady(err error) bool {
	_, ok := errors.AsType[*git.NotReady](err)
	return ok
}

// TestAPartialCloneFetchesNothing: in a clone with no blobs, a blob it lacks
// is failed with the run id, and nothing is fetched: the object is still
// missing after. Planted failure: --no-lazy-fetch left off cat-file.
func TestAPartialCloneFetchesNothing(t *testing.T) {
	t.Parallel()
	home, src := makeHistory(t, contracttest.TheHistory())
	story(t, home, string(src), "config", "uploadpack.allowfilter", "true")
	dir := filepath.Join(filepath.Dir(string(src)), "partial")
	story(t, home, filepath.Dir(dir), "clone", "-q", "--no-checkout", "--filter=blob:none", "file://"+string(src), dir)
	g := adapter(t, configtest.Git(t), home)
	_, err := g.Blob(t.Context(), model.Root(dir), "v1", "rules/a.yaml")
	if f, ok := errors.AsType[*git.Failed](err); !ok || f.Run() != "run-1" {
		t.Errorf("Blob of a blob the clone lacks: %v, want failed with the run id", err)
	}
	blob := story(t, home, string(src), "rev-parse", "v1:rules/a.yaml")
	cmd := exec.CommandContext(t.Context(), "git")
	cmd.Args = append(cmd.Args, "--no-lazy-fetch", "cat-file", "-e", blob)
	cmd.Dir = dir
	cmd.Env = fixtureEnv(home)
	if cmd.Run() == nil {
		t.Error("the blob is in the clone after the read: it was fetched")
	}
}

// everyMethod runs each History method that reads objects once, and
// WorkObjects, which hashes the work tree, and answers each one's error, by
// name.
func everyMethod(t *testing.T, g *git.Git, root model.Root) map[string]error {
	t.Helper()
	out := map[string]error{}
	_, out["Base"] = g.Base(t.Context(), root, "v1", "main")
	_, out["Files at a rev"] = g.Files(t.Context(), root, "v1")
	_, out["Files of the index"] = g.Files(t.Context(), root, "")
	_, out["Blob"] = g.Blob(t.Context(), root, "v1", "README")
	_, out["Tags"] = g.Tags(t.Context(), root)
	_, out["WorkObjects"] = g.WorkObjects(t.Context(), root, []string{"README"})
	return out
}

// TestEveryHistoryMethodRunsOnlyTheSetGit: with no setting and a git
// first in PATH, each method is not ready naming programs.git; with a git
// others can write, each is refused; neither runs anything. Planted
// failures: a fallback to exec.LookPath; the walk skipped for a verb that
// reads objects.
func TestEveryHistoryMethodRunsOnlyTheSetGit(t *testing.T) {
	home, root := makeHistory(t, contracttest.TheHistory())
	dir, _ := fakeGit(t, 0)
	real := configtest.Git(t)
	t.Setenv("PATH", dir)
	g := adapter(t, "", home)
	for name, err := range everyMethod(t, g, root) {
		if nr, ok := errors.AsType[*git.NotReady](err); !ok || !strings.Contains(nr.Observed(), "programs.git") {
			t.Errorf("%s with no setting and git in PATH: %v, want not ready naming programs.git", name, err)
		}
	}
	if len(g.Ran()) != 0 {
		t.Errorf("no setting, and git ran: %s", git.Shown(g.Ran()))
	}
	_, open := fakeGit(t, trusttest.OthersWrite)
	g = adapter(t, open, home)
	for name, err := range everyMethod(t, g, root) {
		if rf, ok := errors.AsType[*git.Refused](err); !ok || rf.Hint() == "" {
			t.Errorf("%s with a git others can write: %v, want refused with a fix", name, err)
		}
	}
	if len(g.Ran()) != 0 {
		t.Errorf("a git others can write ran: %s", git.Shown(g.Ran()))
	}
	g = adapter(t, real, home)
	for name, err := range everyMethod(t, g, root) {
		if err != nil {
			t.Errorf("the control, %s with the set git: %v", name, err)
		}
	}
}

// TestAFilePastAShortVerbsCapIsRead: a blob and a listing are each held to
// their own verb's cap, not to the 64 KiB of a verb that prints a line, so
// a file of 200 KiB and a tree of 1500 files are read whole. Planted
// failure: every verb's output held to the short cap.
func TestAFilePastAShortVerbsCapIsRead(t *testing.T) {
	t.Parallel()
	home, root := fixture(t, nil)
	files := map[string]string{"big": strings.Repeat("0123456789abcdef\n", 200<<10/17)}
	for i := range 1500 {
		files[fmt.Sprintf("many/file-%04d-of-a-tree-past-the-short-cap", i)] = "x\n"
	}
	write(t, string(root), files)
	story(t, home, string(root), "add", "-A", ".")
	story(t, home, string(root), "commit", "-q", "--no-verify", "-m", "big")
	g := adapter(t, configtest.Git(t), home)
	if got, err := g.Blob(t.Context(), root, "HEAD", "big"); err != nil || string(got) != files["big"] {
		t.Errorf("Blob(big) = %d bytes, %v; want %d bytes", len(got), err, len(files["big"]))
	}
	if got, err := g.Files(t.Context(), root, "HEAD"); err != nil || len(got) != len(files) {
		t.Errorf("Files(HEAD) = %d files, %v; want %d", len(got), err, len(files))
	}
}

// TestATagsObjectIsNoEndOfARange: the object name of an annotated tag,
// which git peels to its commit, is refused as an end of a range, as base
// and as head, and so is a name of the other hash's length. Planted
// failure: an end checked by what it peels to.
func TestATagsObjectIsNoEndOfARange(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	tag := story(t, home, string(root), "rev-parse", "refs/tags/v2")
	commit := story(t, home, string(root), "rev-parse", "v2^{commit}")
	if tag == commit {
		t.Fatal("v2 is no tag object")
	}
	g := adapter(t, configtest.Git(t), home)
	for _, end := range []string{tag, commit + strings.Repeat("0", 24)} {
		for _, pair := range [][2]string{{end, commit}, {commit, end}} {
			if msgs, err := g.Messages(t.Context(), root, pair[0], pair[1]); err == nil || msgs != nil {
				t.Errorf("Messages(%s, %s) = %q, %v; want refused", pair[0], pair[1], msgs, err)
			}
		}
	}
}
