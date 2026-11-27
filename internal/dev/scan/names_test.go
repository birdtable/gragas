package scan_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/scan"
	"github.com/birdtable/gragas/internal/digest"
)

// gitRepo is one repository the fake git keeps: its index, its commits,
// oldest first, how many of them origin/main holds (-1 for no origin/main),
// its branch, its config, whether it holds a replace ref, and whether a
// merge is under way.
type gitRepo struct {
	index   map[string]string
	commits []gitCommit
	base    int
	branch  string
	// last is the index as the last commit left it.
	last     map[string]string
	config   map[string]string
	replaced bool
	merging  bool
}

// gitCommit is one commit of a fake repository, the patch git log -p
// prints for it, and whether it is a merge.
type gitCommit struct {
	message, address, patch string
	merge                   bool
}

// gitlink is the body the fake index holds for a submodule entry.
const gitlink = "\x00gitlink"

// gitNull is the name git writes in a diff header for no file. It is the
// same on every system, and no path of this one, so it is not os.DevNull.
const gitNull = "/de" + "v/null"

// noReplyAddress is a no-reply address of the stand-in.
const noReplyAddress = "1+alex@users.noreply.github.com"

// trackedFiles is the files git tracks in dir: the tree's own for "", else
// a plant repository's index, which a host broken with gitBreak "files"
// reads as nothing.
func (f *fake) trackedFiles(dir string) map[string][]byte {
	out := map[string][]byte{}
	if dir == "" {
		for p, body := range f.tree {
			if !strings.HasPrefix(p, "#") {
				out[p] = []byte(body)
			}
		}
		return out
	}
	if r := f.repos[dir]; r != nil && f.gitBreak != "files" {
		for p, body := range r.index {
			out[p] = []byte(body)
		}
	}
	return out
}

// namesGit is a git that keeps the names scan's repositories in memory, the
// tree's at f.repos[""].
func namesGit(f *fake) {
	f.tools["git"] = func(args []string, _ func(string) string) scan.Result {
		address := ""
		for len(args) > 1 && args[0] == "-c" {
			if a, ok := strings.CutPrefix(args[1], "user.email="); ok {
				address = a
			}
			args = args[2:]
		}
		r := f.repos[f.dir]
		if args[0] == "init" {
			branch, _ := strings.CutPrefix(args[2], "--initial-branch=")
			f.repos[f.dir] = &gitRepo{index: map[string]string{}, base: -1, branch: branch, config: map[string]string{}}
			return scan.Result{}
		}
		if r == nil {
			return scan.Result{Code: 128, Stderr: []byte("fatal: not a git repository")}
		}
		return r.run(f, address, args)
	}
}

// orphanID is the id of the fake git's one commit no branch holds.
const orphanID = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

// run is one git command in r.
func (r *gitRepo) run(f *fake, address string, args []string) scan.Result {
	switch args[0] {
	case "add":
		r.index[args[1]] = f.written[f.dir+"/"+args[1]]
	case "rm":
		delete(r.index, args[len(args)-1])
	case "update-index":
		r.updateIndex(args)
	case "commit":
		r.commits = append(r.commits, gitCommit{args[slices.Index(args, "-m")+1], address, r.patch(), r.merging})
		r.last, r.merging = maps.Clone(r.index), false
	case "update-ref":
		r.base = len(r.commits)
		if args[2] == orphanID {
			r.base = 0
		}
	case "write-tree":
		return scan.Result{Stdout: []byte(strings.Repeat("d", 40) + "\n")}
	case "commit-tree":
		return scan.Result{Stdout: []byte(orphanID + "\n")}
	case "rev-parse":
		if slices.Contains(args, "--is-shallow-repository") {
			return scan.Result{Stdout: []byte(shallow(f, f.dir) + "\n")}
		}
		if args[1] == "HEAD" {
			return scan.Result{Stdout: []byte(fmt.Sprintf("%040x\n", len(r.commits)))}
		}
		if r.base < 0 {
			return scan.Result{Code: 1}
		}
	case "symbolic-ref":
		if r.branch == "" {
			return scan.Result{Code: 1}
		}
		return scan.Result{Stdout: []byte(r.branch + "\n")}
	case "rev-list":
		return scan.Result{Stdout: []byte(strconv.Itoa(len(r.commits)-r.base) + "\n")}
	case "log":
		if slices.Contains(args, "-p") {
			return scan.Result{Stdout: []byte(r.patchOf(args, f.gitBreak == "patch"))}
		}
		return scan.Result{Stdout: []byte(r.log(f.gitBreak == "log"))}
	case "config":
		r.config[args[1]] = args[2]
	case "replace":
		r.replaced = true
	case "for-each-ref":
		if r.replaced {
			return scan.Result{Stdout: []byte("refs/replace/" + strings.Repeat("ab", 20) + "\n")}
		}
	case "merge":
		r.merging = true
	case "checkout":
	default:
		return scan.Result{Code: 129, Stderr: []byte("unknown")}
	}
	return scan.Result{}
}

// updateIndex adds a submodule entry to the index, or removes a path.
func (r *gitRepo) updateIndex(args []string) {
	if args[1] == "--force-remove" {
		delete(r.index, args[2])
		return
	}
	entry := strings.Split(args[3], ",")
	r.index[entry[2]] = gitlink
}

// log is git log -z of the commits past origin/main, newest first, in the
// format the scan asks for; nothing when broken.
func (r *gitRepo) log(broken bool) string {
	if broken {
		return ""
	}
	var b strings.Builder
	for i := len(r.commits) - 1; i >= r.base; i-- {
		c := r.commits[i]
		fmt.Fprintf(&b, "%040x\x1f%s\x1f%s\x1f%s\n\x00", i+1, c.address, c.address, c.message)
	}
	return b.String()
}

// patch is the patch of a commit of the index now: each file that changed
// since the last commit, written whole, as git log -p --unified=0 prints a
// file it rewrites, and a submodule entry as git prints one.
func (r *gitRepo) patch() string {
	var b strings.Builder
	for _, p := range slices.Sorted(maps.Keys(r.index)) {
		body := r.index[p]
		if old, ok := r.last[p]; ok && old == body {
			continue
		}
		if body == gitlink {
			fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode 160000\n--- "+gitNull+"\n+++ b/%s\n@@ -0,0 +1 @@\n+Subproject commit %s\n", p, p, p, strings.Repeat("ab", 20))
			continue
		}
		lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1 +1,%d @@\n", p, p, p, p, len(lines))
		for _, l := range lines {
			b.WriteString("+" + l + "\n")
		}
	}
	return b.String()
}

// patchOf is the patch of the commit the last of args names, as the log
// names it; nothing when broken. As git does, it prints no line of a file
// a binary attribute marks unless args hold --text, nor one a textconv
// driver prints as nothing unless they hold --no-textconv; nothing for a
// root commit when the config sets log.showRoot false unless they hold
// --root, nor for a merge unless they hold --diff-merges=first-parent; and
// no submodule entry when the config sets diff.ignoreSubmodules all unless
// they hold --ignore-submodules=none.
func (r *gitRepo) patchOf(args []string, broken bool) string {
	n, err := strconv.ParseInt(args[len(args)-1], 16, 64)
	if broken || err != nil || n < 1 || int(n) > len(r.commits) {
		return ""
	}
	c := r.commits[n-1]
	hidden := (strings.Contains(c.patch, " binary\n") && !slices.Contains(args, "--text")) ||
		(strings.Contains(c.patch, " diff=plant\n") && r.config["diff.plant.textconv"] != "" && !slices.Contains(args, "--no-textconv")) ||
		(n == 1 && r.config["log.showRoot"] == "false" && !slices.Contains(args, "--root")) ||
		(c.merge && !slices.Contains(args, "--diff-merges=first-parent"))
	if hidden {
		return ""
	}
	if r.config["diff.ignoreSubmodules"] == "all" && !slices.Contains(args, "--ignore-submodules=none") {
		return withoutGitlinks(c.patch)
	}
	return c.patch
}

// withoutGitlinks is patch without its submodule entries.
func withoutGitlinks(patch string) string {
	var files []string
	for line := range strings.SplitAfterSeq(patch, "\n") {
		if strings.HasPrefix(line, "diff --git ") || len(files) == 0 {
			files = append(files, "")
		}
		files[len(files)-1] += line
	}
	return strings.Join(slices.DeleteFunc(files, func(f string) bool { return strings.Contains(f, "\nnew file mode 160000\n") }), "")
}

// listOf is a names list made from names at run time, as names hash makes
// it.
func listOf(t *testing.T, names ...string) string {
	t.Helper()
	var lines []string
	for _, n := range names {
		hs, err := scan.NameHashes(n)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, hs...)
	}
	return "# made at run time\n" + strings.Join(lines, "\n") + "\n"
}

// namesTree is a clean tree on branch work, one commit past origin/main,
// with a list of the stand-in host's name outside it.
func namesTree(t *testing.T) *fake {
	t.Helper()
	f := newFake()
	f.tree["main.go"] = "package main\n"
	f.tree["docs/notes.md"] = "notes\n"
	f.repos[""] = &gitRepo{index: map[string]string{}, commits: []gitCommit{{message: "start", address: noReplyAddress}, {message: "add notes", address: noReplyAddress, patch: "diff --git a/docs/notes.md b/docs/notes.md\n--- " + gitNull + "\n+++ b/docs/notes.md\n@@ -0,0 +1 @@\n+notes\n"}}, base: 1, branch: "work"}
	f.namesPath, f.namesData = "/work/hashes", listOf(t, "box")
	namesGit(f)
	return f
}

func TestTheNamesScanProvesItselfThenPassesACleanTree(t *testing.T) {
	t.Parallel()
	f := namesTree(t)
	lines, err := scan.Names(context.Background(), f.machine())
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(lines, "\n"))
	}
	got := strings.Join(lines, "\n")
	for _, want := range []string{
		"names: 1 hash in the list GRAGAS_NAMES_HASHES names",
		"  ok  red: a tracked file that holds the planted name is red",
		"  ok  red: a tracked file whose two-word name breaks across a line is red",
		"  ok  red: a tracked file whose path holds the name run together is red",
		"  ok  red: a branch named for the planted name is red",
		"  ok  red: a commit message past origin/main that holds the planted name is red",
		"  ok  red: a name a commit past origin/main adds and a later one removes is red",
		"  ok  red: that name in a file its attributes mark binary is red",
		"  ok  red: that name in a file a textconv driver of its attributes would print as nothing is red",
		"  ok  red: that name in a root commit past origin/main, though the repository's config sets log.showRoot false, is red",
		"  ok  red: that name only a merge's own lines add is red",
		"  ok  red: that name as the path of a submodule entry a commit adds and a later one removes, though the repository's config sets diff.ignoreSubmodules all, is red",
		"  ok  red: a commit past origin/main whose address is not a no-reply one is red",
		"  ok  red: a clean repository that holds a replace ref is red",
		"  ok  red: a clean repository whose git folder holds a grafts file is red",
		"  ok  red: a name a commit past origin/main adds and a later one removes, in a clone made shallow at the later one, is red",
		"  ok  green: the same name in a message already on origin/main is green",
		"  ok  green: a clean repository, read whole, is green",
		"names: 2 tracked files, the branch, and 1 commit past origin/main",
		"names: no name on the list, and every address is a no-reply one",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no line %q in\n%s", want, got)
		}
	}
	if !slices.ContainsFunc(f.env, func(e string) bool { return e == "GIT_CONFIG_GLOBAL="+os.DevNull }) {
		t.Errorf("git ran with the host's config: env %q", f.env)
	}
}

// TestTheNamesScanRefusesWhatItCannotTrust: a list unset, unreadable, inside
// the tree, in its case or another and by either separator, or not of
// hashes, a missing git, a clone without origin/main, a git whose log
// reads no commit or whose patches read nothing or read as the attributes
// shape them, a host that reads no file of a plant, and a tree of no file
// each fail closed; a name in the tree is a refusal naming
// the home and the fix. No message holds the list's plain text. A list
// beside the tree, in a folder whose name starts as the tree's, passes.
func TestTheNamesScanRefusesWhatItCannotTrust(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		edit  func(*fake)
		says  string
		proof bool
	}{
		{"the list unset", func(f *fake) { f.namesPath = "" }, "GRAGAS_NAMES_HASHES is not set, so no name can be checked", false},
		{"the list unreadable", func(f *fake) { f.namesErr = errors.New("permission denied") }, "the list GRAGAS_NAMES_HASHES names cannot be read: permission denied", false},
		{"the list inside the tree", func(f *fake) { f.namesPath = "/repo/gate/hashes" }, "is inside the tree, and it is kept outside it", false},
		{"the list inside the tree, named in another case", func(f *fake) { f.namesPath = "/REPO/Gate/hashes" }, "is inside the tree, and it is kept outside it", false},
		{"a list of plain words", func(f *fake) { f.namesData = "box\n" }, "line 1 of the list is not a SHA-256 in hex", false},
		{"a list of no hash", func(f *fake) { f.namesData = "# nothing yet\n" }, scan.ErrEmpty.Error(), false},
		{"no git", func(f *fake) { f.missing = "git" }, "names: git is not on the PATH", false},
		{"no origin/main", func(f *fake) { f.repos[""].base = -1 }, "origin/main is not in this clone", false},
		{"a log that reads no commit", func(f *fake) { f.gitBreak = "log" }, "git log printed 0 commits past origin/main, and git rev-list counts 1", true},
		{"a host that reads no file of a plant", func(f *fake) { f.gitBreak = "files" }, "the control read 0 files", true},
		{"a git whose patches read nothing", func(f *fake) { f.gitBreak = "patch" }, "a name a commit past origin/main adds and a later one removes is red: " + scan.ErrProof.Error() + ": it was green", true},
		{"a git log whose patch the attributes shape", func(f *fake) { without(f, "git", "--text") }, "that name in a file its attributes mark binary is red: " + scan.ErrProof.Error() + ": it was green", true},
		{"a git whose patch read fails", func(f *fake) {
			tool := f.tools["git"]
			f.tools["git"] = func(args []string, read func(string) string) scan.Result {
				if f.dir == "" && slices.Contains(args, "-p") {
					return scan.Result{Code: 128, Stderr: []byte("fatal: bad object")}
				}
				return tool(args, read)
			}
		}, "git log exited 128: fatal: bad object", false},
		{"a tree of no file", func(f *fake) { f.tree = map[string]string{} }, scan.ErrEmpty.Error() + ": git tracks no file", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := namesTree(t)
			c.edit(f)
			_, err := scan.Names(context.Background(), f.machine())
			if err == nil || !strings.Contains(err.Error(), c.says) || errors.Is(err, scan.ErrProof) != c.proof {
				t.Fatalf("err %v, want one saying %q (a failed proof: %v)", err, c.says, c.proof)
			}
			if strings.Contains(strings.ToLower(err.Error()), "box") {
				t.Errorf("the refusal prints the list's word: %v", err)
			}
		})
	}
	t.Run("a list beside the tree, in a folder whose name starts as the tree's", func(t *testing.T) {
		t.Parallel()
		f := namesTree(t)
		f.namesPath = "/repository/hashes"
		if lines, err := scan.Names(context.Background(), f.machine()); err != nil {
			t.Fatalf("a list outside the tree was refused: %v\n%s", err, strings.Join(lines, "\n"))
		}
	})
	for _, c := range []struct {
		name string
		edit func(*fake)
		want string
	}{
		{"a replace ref", func(f *fake) { f.repos[""].replaced = true }, "refs/replace/" + strings.Repeat("ab", 20) + ": git shows another commit in place of the one it names, and the scan refuses it: delete it with git replace -d"},
		{"a grafts file", func(f *fake) { f.tree["#git/info/grafts"] = "" }, "info/grafts in the git folder: git gives a commit other parents than it has, and the scan refuses it: delete the file"},
		{"a shallow clone", func(f *fake) { f.tree["#shallow"] = "true" }, "the clone is shallow: git shows its oldest commits without their parents, and the scan refuses it: run git fetch --unshallow"},
		{"an old git's echo of the shallow flag", func(f *fake) { f.tree["#shallow"] = "--is-shallow-repository" }, "the clone is shallow: git shows its oldest commits without their parents, and the scan refuses it: run git fetch --unshallow"},
	} {
		t.Run("a tree whose history "+c.name+" rewrites", func(t *testing.T) {
			t.Parallel()
			f := namesTree(t)
			c.edit(f)
			f.repos[""].commits[1].message = "thanks box"
			lines, err := scan.Names(context.Background(), f.machine())
			var r *scan.Refusal
			if !errors.As(err, &r) || !slices.Equal(r.Findings, []string{c.want}) {
				t.Fatalf("err %v, want a refusal of %q alone, as no commit is read", err, c.want)
			}
			if !slices.Contains(lines, "names: 2 tracked files, the branch, and 0 commits past origin/main") {
				t.Errorf("the scan did not read the files and no commit:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
	t.Run("a name in the tree", func(t *testing.T) {
		t.Parallel()
		f := namesTree(t)
		f.tree["docs/notes.md"] = "the build box\n"
		_, err := scan.Names(context.Background(), f.machine())
		var r *scan.Refusal
		if !errors.As(err, &r) || !slices.Equal(r.Findings, []string{"docs/notes.md:1: a name of 1 word on the list"}) || !strings.Contains(r.Home, "gragas-dev names hash") || !strings.Contains(r.Fix, "no-reply") {
			t.Fatalf("err %v", err)
		}
	})
}

// TestJudgeNamesFindsEachNameAndNeverPrintsIt: a name of the list is found
// in a file however its words are cased, split or run together, across a
// line break, in a path, in the branch and in a message past the base, and
// an address is judged against the no-reply ones. A word that only holds a
// name, such as a longer word, is not one. No finding holds a name.
func TestJudgeNamesFindsEachNameAndNeverPrintsIt(t *testing.T) {
	t.Parallel()
	l, err := scan.ReadNamesList([]byte(listOf(t, "alex", "alex box")))
	if err != nil {
		t.Fatal(err)
	}
	commit := func(message, author string) scan.Commit {
		return scan.Commit{ID: strings.Repeat("ab", 20), Author: author, Committer: noReplyAddress, Message: message}
	}
	for _, c := range []struct {
		name string
		p    scan.Pushed
		want []string
	}{
		{"a word in a file", scan.Pushed{Files: map[string][]byte{"a.txt": []byte("x\nHello, ALEX.\n")}}, []string{"a.txt:2: a name of 1 word on the list"}},
		{"two words across a line break", scan.Pushed{Files: map[string][]byte{"a.txt": []byte("x\nmet Alex\n  Box\n")}}, []string{"a.txt:2: a name of 1 word on the list", "a.txt:2: a name of 2 words on the list"}},
		{"two words in camel case", scan.Pushed{Files: map[string][]byte{"a.go": []byte("var alexBox = 1\n")}}, []string{"a.go:1: a name of 1 word on the list", "a.go:1: a name of 2 words on the list"}},
		{"two words run together", scan.Pushed{Files: map[string][]byte{"a.txt": []byte("ALEXBOX\n")}}, []string{"a.txt:1: a name of 1 word on the list"}},
		{"a path", scan.Pushed{Files: map[string][]byte{"docs/AlexBox.md": []byte("clean\n")}}, []string{"docs/<name>.md: its path holds a name on the list"}},
		{"a path and its contents", scan.Pushed{Files: map[string][]byte{"alex/a.txt": []byte("alex\n")}}, []string{"<name>/a.txt: its path holds a name on the list", "<name>/a.txt:1: a name of 1 word on the list"}},
		{"the branch", scan.Pushed{Branch: "fix-alex"}, []string{"the branch name holds a name on the list"}},
		{"a message", scan.Pushed{Commits: []scan.Commit{commit("subject\n\nthanks to\nAlex\n", noReplyAddress)}}, []string{"commit abababababab: its message, line 4, holds a name of 1 word on the list"}},
		{"an address of a person", scan.Pushed{Commits: []scan.Commit{commit("ok\n", "alex@example.com")}}, []string{"commit abababababab: its author address is not a no-reply address"}},
		{"a longer word, and words that only hold the name", scan.Pushed{Files: map[string][]byte{"a.txt": []byte("Alexander alexa xalex boxalex\n")}, Branch: "work"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := scan.JudgeNames(l, c.p)
			if !slices.Equal(got, c.want) {
				t.Errorf("%q\nwant %q", got, c.want)
			}
			for _, f := range got {
				if strings.Contains(strings.ToLower(strings.ReplaceAll(f, "<name>", "")), "alex") {
					t.Errorf("a finding prints a name: %q", f)
				}
			}
		})
	}
}

// TestTheLinesEachPushedCommitAddsAreJudged: a name in a line a commit
// adds is found at its line in the file, across a line break of the run
// it adds, and in a path it writes, quoted or not, each masked. A line it
// removes, a file it deletes and a context line are not read, and a line
// it adds that looks like a file's header is read as a line.
func TestTheLinesEachPushedCommitAddsAreJudged(t *testing.T) {
	t.Parallel()
	l, err := scan.ReadNamesList([]byte(listOf(t, "alex", "alex box")))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("cd", 20)
	head := func(p string) string { return "diff --git a/" + p + " b/" + p + "\n--- a/" + p + "\n+++ b/" + p + "\n" }
	for _, c := range []struct {
		name, patch string
		want        []string
	}{
		{"a line it adds", head("a.txt") + "@@ -3,0 +4,2 @@\n+clean\n+met Alex\n", []string{"commit cdcdcdcdcdcd: a.txt:5, a line it adds, holds a name of 1 word on the list"}},
		{"two words across a line break", head("a.txt") + "@@ -0,0 +1,2 @@\n+met alex\n+box today\n", []string{"commit cdcdcdcdcdcd: a.txt:1, a line it adds, holds a name of 1 word on the list", "commit cdcdcdcdcdcd: a.txt:1, a line it adds, holds a name of 2 words on the list"}},
		{"a path it writes", head("docs/AlexBox.md") + "@@ -0,0 +1 @@\n+clean\n", []string{"commit cdcdcdcdcdcd: it writes docs/<name>.md, whose path holds a name on the list"}},
		{"a quoted path", "diff --git \"a/x\\talex\" \"b/x\\talex\"\n--- " + gitNull + "\n+++ \"b/x\\talex\"\n@@ -0,0 +1 @@\n+clean\n", []string{"commit cdcdcdcdcdcd: it writes x\t<name>, whose path holds a name on the list"}},
		{"a line it adds to a path that holds a name", head("alex/a.txt") + "@@ -0,0 +1 @@\n+met alex\n", []string{"commit cdcdcdcdcdcd: it writes <name>/a.txt, whose path holds a name on the list", "commit cdcdcdcdcdcd: <name>/a.txt:1, a line it adds, holds a name of 1 word on the list"}},
		{"a second hunk counts from its own head", head("a.txt") + "@@ -1 +1 @@\n-old\n+new\n@@ -9,0 +10 @@\n+alex\n", []string{"commit cdcdcdcdcdcd: a.txt:10, a line it adds, holds a name of 1 word on the list"}},
		{"a line after a context line", head("a.txt") + "@@ -1,2 +1,2 @@\n keep\n-x\n+alex\n", []string{"commit cdcdcdcdcdcd: a.txt:2, a line it adds, holds a name of 1 word on the list"}},
		{"a line that looks like a header", head("a.txt") + "@@ -0,0 +1 @@\n+++ b/alex\n", []string{"commit cdcdcdcdcdcd: a.txt:1, a line it adds, holds a name of 1 word on the list"}},
		{"a line it removes, a file it deletes, a context line", "diff --git a/alex.txt b/alex.txt\n--- a/alex.txt\n+++ " + gitNull + "\n@@ -1 +0,0 @@\n-alex\n" + head("b.txt") + "@@ -1,2 +1,2 @@\n alex\n-x\n+y\n", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := scan.JudgeNames(l, scan.Pushed{Commits: []scan.Commit{{ID: id, Author: noReplyAddress, Committer: noReplyAddress, Message: "ok\n", Patch: c.patch}}})
			if !slices.Equal(got, c.want) {
				t.Errorf("%q\nwant %q", got, c.want)
			}
			for _, f := range got {
				if strings.Contains(strings.ToLower(strings.ReplaceAll(f, "<name>", "")), "alex") {
					t.Errorf("a finding prints a name: %q", f)
				}
			}
		})
	}
}

// TestOnlyANoReplyAddressPasses: an account's no-reply address, with or
// without its id, a bot's, and GitHub's web merges' pass; a person's
// address, a look-alike domain, a login GitHub would refuse, an address
// with its name and no address at all are refused.
func TestOnlyANoReplyAddressPasses(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, addr string
		want       bool
	}{
		{"an account's, with its id", "22815456+Some-User@users.noreply.github.com", true},
		{"an account's, without its id", "some-user@users.noreply.github.com", true},
		{"a bot's", "41898282+github-actions[bot]@users.noreply.github.com", true},
		{"GitHub's web merges'", "noreply@github.com", true},
		{"a person's", "alex@example.com", false},
		{"a look-alike domain", "x@users.noreply.github.com.example.com", false},
		{"a login that starts with a hyphen", "-x@users.noreply.github.com", false},
		{"a login that ends with a hyphen", "x-@users.noreply.github.com", false},
		{"a look-alike of the merges' address", "noreply@github.com.example.com", false},
		{"an address with its name", "alex <1+alex@users.noreply.github.com>", false},
		{"no address", "", false},
	} {
		if scan.NoReply(c.addr) != c.want {
			t.Errorf("%s: NoReply(%q) = %v, want %v", c.name, c.addr, !c.want, c.want)
		}
	}
}

// TestTheListIsReadAsHashesAlone: the list holds a SHA-256 in hex a line,
// in either case, with blank lines and comments passed over; a line that
// is not one is refused by its number alone, and a list of no hash is
// empty.
func TestTheListIsReadAsHashesAlone(t *testing.T) {
	t.Parallel()
	h := digest.Hex([]byte("box"))
	l, err := scan.ReadNamesList([]byte("# a comment\n\n" + h + "\n  " + strings.ToUpper(h) + "  \n"))
	if err != nil || len(l) != 1 {
		t.Fatalf("%d hashes, %v", len(l), err)
	}
	for _, c := range []struct{ name, data, says string }{
		{"a word in the clear", h + "\nbox\n", "line 2 of the list is not a SHA-256 in hex"},
		{"a short hash", h[:40] + "\n", "line 1 of the list is not a SHA-256 in hex"},
		{"no hash", "# none\n\n", scan.ErrEmpty.Error()},
	} {
		_, err := scan.ReadNamesList([]byte(c.data))
		if err == nil || !strings.Contains(err.Error(), c.says) || strings.Contains(err.Error(), "box") {
			t.Errorf("%s: err %v, want one saying %q and no word of the list", c.name, err, c.says)
		}
	}
}

// TestANameIsHashedAsTheScanReadsIt: a name's words are folded and split
// where the case turns, as the scan splits text, so the line a person
// writes and the text the scan reads give the same hash.
func TestANameIsHashedAsTheScanReadsIt(t *testing.T) {
	t.Parallel()
	sum := func(s string) string {
		return digest.Hex([]byte(s))
	}
	for name, want := range map[string][]string{
		"Alex":           {sum("alex")},
		"  alex   BOX  ": {sum("alex box"), sum("alexbox")},
		"AlexBox":        {sum("alex box"), sum("alexbox")},
		"HTTPServer":     {sum("http server"), sum("httpserver")},
		"alex-box":       {sum("alex box"), sum("alexbox")},
	} {
		got, err := scan.NameHashes(name)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("NameHashes(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	for _, name := range []string{"", "--", "a b c d e"} {
		if _, err := scan.NameHashes(name); err == nil {
			t.Errorf("NameHashes(%q) passed", name)
		}
	}
}

// without wraps the tool name of f so that it never sees flag, as a tool
// that would not honour it.
func without(f *fake, name, flag string) {
	tool := f.tools[name]
	f.tools[name] = func(args []string, read func(string) string) scan.Result {
		return tool(slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == flag }), read)
	}
}
