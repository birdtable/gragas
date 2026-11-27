package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// the fixture's commits by name
func commitOf(t *testing.T, name string) string {
	t.Helper()
	for _, c := range contracttest.TheHistory().Commits {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("the fixture has no commit %s", name)
	return ""
}

// cleanMain is the fixture with the index and the work tree put back to
// main, so a merge can start.
func cleanMain(t *testing.T) (home string, root model.Root) {
	t.Helper()
	home, root = makeHistory(t, contracttest.TheHistory())
	story(t, home, string(root), "reset", "-q", "--hard")
	return home, root
}

// TestAMergeStoppedBeforeItsCommitIsMerging: git merge --no-commit leaves
// the commit it brings in in the merge state, and Merging answers it; once
// the merge is committed there is none. Planted failure: the file read at
// the root instead of where git keeps it.
func TestAMergeStoppedBeforeItsCommitIsMerging(t *testing.T) {
	t.Parallel()
	home, root := cleanMain(t)
	g := adapter(t, configtest.Git(t), home)
	story(t, home, string(root), "merge", "-q", "--no-ff", "--no-commit", "side")
	if got, err := g.Merging(t.Context(), root); err != nil || !slices.Equal(got, []string{commitOf(t, "side")}) {
		t.Errorf("Merging mid-merge = %q, %v; want side's commit", got, err)
	}
	story(t, home, string(root), "commit", "-q", "--no-verify", "--no-edit")
	if got, err := g.Merging(t.Context(), root); err != nil || got == nil || len(got) != 0 {
		t.Errorf("Merging after the commit = %q, %v; want an empty list", got, err)
	}
}

// TestAMergeThatStoppedOnAConflictIsMerging: an index with paths at stages
// 1 to 3 is still a merge Merging answers, though Files of it is not ready.
// Planted failure: Merging made to wait for a clean index.
func TestAMergeThatStoppedOnAConflictIsMerging(t *testing.T) {
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
	if got, err := adapter(t, configtest.Git(t), home).Merging(t.Context(), root); err != nil || !slices.Equal(got, []string{commitOf(t, "two")}) {
		t.Errorf("Merging on a conflict = %q, %v; want main's commit", got, err)
	}
}

// TestARefNamedAsTheMergeStateIsNoMerge: a tag and a branch named
// MERGE_HEAD are refs, not the merge state; with neither merge nor file,
// Merging answers none. Planted failure: the merge read as a rev.
func TestARefNamedAsTheMergeStateIsNoMerge(t *testing.T) {
	t.Parallel()
	home, root := cleanMain(t)
	// the fixture has the tag already; a branch of the name too
	story(t, home, string(root), "branch", "MERGE_HEAD", "side")
	if got, err := adapter(t, configtest.Git(t), home).Merging(t.Context(), root); err != nil || got == nil || len(got) != 0 {
		t.Errorf("Merging with refs named MERGE_HEAD = %q, %v; want an empty list", got, err)
	}
}

// TestAMergeStateGitDoesNotWriteIsFailed: a merge state file that names a
// blob, a tag object, nothing, or what is not an object name, that lacks
// its last newline, is past the cap, is a link or a folder, is failed,
// never answered and never none. Planted failure: each line resolved as a
// rev and its commit answered, so the tag object reads as v2's commit.
func TestAMergeStateGitDoesNotWriteIsFailed(t *testing.T) {
	t.Parallel()
	home, root := cleanMain(t)
	dir := string(root)
	side := commitOf(t, "side")
	blob := story(t, home, dir, "rev-parse", "main:README")
	tag := story(t, home, dir, "rev-parse", "v2")
	if tag == commitOf(t, "two") {
		t.Fatal("v2 is not a tag object")
	}
	at := filepath.Join(dir, ".git", "MERGE_HEAD")
	put := func(t *testing.T, s string) {
		t.Helper()
		if err := os.WriteFile(at, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	g := adapter(t, configtest.Git(t), home)
	cases := map[string]func(t *testing.T){
		"a blob":             func(t *testing.T) { t.Helper(); put(t, blob+"\n") },
		"a tag object":       func(t *testing.T) { t.Helper(); put(t, tag+"\n") },
		"an object it lacks": func(t *testing.T) { t.Helper(); put(t, strings.Repeat("1", 40)+"\n") },
		"no object name":     func(t *testing.T) { t.Helper(); put(t, "side\n") },
		"nothing":            func(t *testing.T) { t.Helper(); put(t, "") },
		"a blank line":       func(t *testing.T) { t.Helper(); put(t, side+"\n\n") },
		"no last newline":    func(t *testing.T) { t.Helper(); put(t, side) },
		"a carriage return":  func(t *testing.T) { t.Helper(); put(t, side+"\r\n") },
		"a second field":     func(t *testing.T) { t.Helper(); put(t, side+" x\n") },
		"past the cap":       func(t *testing.T) { t.Helper(); put(t, strings.Repeat(side+"\n", 2000)) },
		"a folder":           func(t *testing.T) { t.Helper(); mkdir(t, at) },
		"a link to a good state": func(t *testing.T) {
			t.Helper()
			// relative, in the same folder, where a root would follow it
			if err := os.WriteFile(filepath.Join(dir, ".git", "ELSEWHERE"), []byte(side+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			link(t, "ELSEWHERE", at)
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			unplant(t, at)
			plant(t)
			if got, err := g.Merging(t.Context(), root); err == nil || got != nil {
				t.Errorf("Merging = %q, %v; want an error and no commit", got, err)
			}
		})
	}
	unplant(t, at)
	put(t, side+"\n")
	if got, err := g.Merging(t.Context(), root); err != nil || !slices.Equal(got, []string{side}) {
		t.Errorf("the control, side's commit, = %q, %v", got, err)
	}
}

// unplant takes away the file, link or empty folder at p, if any.
func unplant(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// TestEachWorktreeHasItsOwnMerge: a merge stopped in a linked worktree is
// that worktree's, and the main worktree has none. Planted failure: the
// file read in the common folder.
func TestEachWorktreeHasItsOwnMerge(t *testing.T) {
	t.Parallel()
	home, root := cleanMain(t)
	other := filepath.Join(filepath.Dir(string(root)), "other")
	story(t, home, string(root), "worktree", "add", "-q", "-b", "other", other, "main")
	story(t, home, other, "merge", "-q", "--no-ff", "--no-commit", "side")
	g := adapter(t, configtest.Git(t), home)
	if got, err := g.Merging(t.Context(), model.Root(other)); err != nil || !slices.Equal(got, []string{commitOf(t, "side")}) {
		t.Errorf("Merging in the linked worktree = %q, %v; want side's commit", got, err)
	}
	if got, err := g.Merging(t.Context(), root); err != nil || got == nil || len(got) != 0 {
		t.Errorf("Merging in the main worktree = %q, %v; want an empty list", got, err)
	}
}

// TestAStashMergedInIsTwoCommits: git stash makes a commit of the index on
// HEAD and one of the work tree on both; merged in with --no-commit, the
// merge brings in the stash, and Commits from HEAD answers the index's
// commit and then the stash's, each with its parents.
// Planted failure: --parents left off rev-list.
func TestAStashMergedInIsTwoCommits(t *testing.T) {
	t.Parallel()
	home, root := cleanMain(t)
	dir := string(root)
	write(t, dir, map[string]string{"rules/a.yaml": "hand\n", "README": "read me\n", "empty": ""})
	story(t, home, dir, "stash", "push", "-q")
	stash := story(t, home, dir, "rev-parse", "stash@{0}")
	index := story(t, home, dir, "rev-parse", "stash@{0}^2")
	story(t, home, dir, "merge", "-q", "--no-ff", "--no-commit", "stash@{0}")
	g := adapter(t, configtest.Git(t), home)
	if got, err := g.Merging(t.Context(), root); err != nil || !slices.Equal(got, []string{stash}) {
		t.Fatalf("Merging of the stash = %q, %v; want %s", got, err, stash)
	}
	head := commitOf(t, "two")
	want := []model.Commit{{ID: index, Parents: []string{head}}, {ID: stash, Parents: []string{head, index}}}
	if got, err := g.Commits(t.Context(), root, "HEAD", stash); err != nil || !slices.EqualFunc(got, want, func(a, b model.Commit) bool {
		return a.ID == b.ID && slices.Equal(a.Parents, b.Parents)
	}) {
		t.Errorf("Commits(HEAD, stash) = %+v, %v; want %+v", got, err, want)
	}
}

// TestCommitsInAShallowCloneAreNotReady: a shallow clone's first commit
// names a parent it lacks, so Commits is not ready before rev-list runs.
// Planted failure: the shallow check gone.
func TestCommitsInAShallowCloneAreNotReady(t *testing.T) {
	t.Parallel()
	home, src := makeHistory(t, contracttest.TheHistory())
	dir := filepath.Join(filepath.Dir(string(src)), "shallow")
	story(t, home, filepath.Dir(dir), "clone", "-q", "--depth", "1", "--branch", "main", "file://"+string(src), dir)
	g := adapter(t, configtest.Git(t), home)
	if got, err := g.Commits(t.Context(), model.Root(dir), "main", "main"); !isNotReady(err) || got != nil {
		t.Errorf("Commits in a shallow clone = %+v, %v; want not ready", got, err)
	}
	for _, r := range g.Ran() {
		if r.Verb == "commits-between" {
			t.Error("rev-list ran in a shallow clone")
		}
	}
}
