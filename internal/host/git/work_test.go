package git_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// verbsOf is the verb of each run g made, in order.
func verbsOf(g *git.Git) []string {
	var verbs []string
	for _, r := range g.Ran() {
		verbs = append(verbs, r.Verb)
	}
	return verbs
}

// TestFilesRunsNoNewVerb: the count check's two listings, the
// base's and the index's, run the verbs they ran before ids were listed, and
// the ids of the work tree add exactly one run, hash-object --no-filters
// --stdin-paths with no -w, with the prefix every verb has. Planted
// failures: a run a path; a cat-file run for the ids; -w left on.
func TestFilesRunsNoNewVerb(t *testing.T) {
	t.Parallel()
	home, root := makeHistory(t, contracttest.TheHistory())
	listings := func(g *git.Git) {
		t.Helper()
		for _, rev := range []string{"v2", ""} {
			if _, err := g.Files(t.Context(), root, rev); err != nil {
				t.Fatal(err)
			}
		}
	}
	before := adapter(t, configtest.Git(t), home)
	listings(before)
	after := adapter(t, configtest.Git(t), home)
	listings(after)
	paths := []string{"README", "empty", "new.txt", "rules/a.yaml"}
	ids, err := after.WorkObjects(t.Context(), root, paths)
	if err != nil || len(ids) != len(paths) {
		t.Fatalf("WorkObjects(%q) = %q, %v", paths, ids, err)
	}
	want := append(verbsOf(before), "work-objects")
	if got := verbsOf(after); !slices.Equal(got, want) {
		t.Fatalf("the runs were %q; want the listings' %q and one more, work-objects", got, verbsOf(before))
	}
	last := after.Ran()[len(after.Ran())-1]
	pre := []string{"--no-replace-objects", "-c", "core.commitGraph=false", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "advice.graftFileDeprecated=false"}
	argv := slices.Concat(pre, []string{"hash-object", "--no-filters", "--stdin-paths"})
	if !slices.Equal(last.Argv[1:], argv) || last.Dir != string(root) || last.Code != 0 {
		t.Errorf("the run %+v; want git %q in the root, exit 0", last, argv)
	}
}

// committed makes a repository at home whose one commit holds name, staged
// from text with .gitattributes' attrs, and whose work tree then holds text
// unchanged: git status reports nothing. It answers the adapter, the root
// and the commit's listing of name.
func committed(t *testing.T, attrs, name, text string, config ...string) (*git.Git, model.Root, model.Entry) {
	t.Helper()
	home, root := fixture(t, nil)
	dir := string(root)
	for i := 0; i+1 < len(config); i += 2 {
		story(t, home, dir, "config", config[i], config[i+1])
	}
	write(t, dir, map[string]string{".gitattributes": attrs, name: text})
	story(t, home, dir, "add", "-A", ".")
	story(t, home, dir, "commit", "-q", "--no-verify", "-m", "one")
	if st := story(t, home, dir, "status", "--porcelain"); st != "" {
		t.Fatalf("git status after the commit = %q; want the work tree as git sees it unchanged", st)
	}
	g := adapter(t, configtest.Git(t), home)
	files, err := g.Files(t.Context(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range files {
		if e.Path == name {
			return g, root, e
		}
	}
	t.Fatalf("Files(HEAD) = %+v; want %s in it", files, name)
	return nil, "", model.Entry{}
}

// TestAFileGitStoresWithAnotherEndOfLineIsNotItsBase:
// .gitattributes gives a file eol=crlf, so git stores it with LF and the
// work tree holds CRLF, and git status calls it unchanged; its work-tree id
// is of the CRLF bytes, so it differs from its base and the count check
// reads it. Planted failure: hash-object run without --no-filters.
func TestAFileGitStoresWithAnotherEndOfLineIsNotItsBase(t *testing.T) {
	t.Parallel()
	g, root, base := committed(t, "a.txt text eol=crlf\n", "a.txt", "one\r\ntwo\r\n")
	if base.Size != int64(len("one\ntwo\n")) {
		t.Fatalf("the base of a.txt is %d bytes; want git to have stored it with LF, %d", base.Size, len("one\ntwo\n"))
	}
	ids, err := g.WorkObjects(t.Context(), root, []string{"a.txt"})
	if err != nil || len(ids) != 1 || ids[0] == "" || ids[0] == base.Object {
		t.Errorf("WorkObjects(a.txt) = %q, %v; want an id of the CRLF bytes, not the base's %s", ids, err, base.Object)
	}
}

// TestNoCleanProgramTheRepositoryNamesRuns: the repository's
// config names a clean program for a file, which git add ran, and git status
// calls the file unchanged; its work-tree id is of the bytes on disk, not of
// what the program makes of them, so the program did not run and the file
// is read. Planted failure: hash-object run without --no-filters.
func TestNoCleanProgramTheRepositoryNamesRuns(t *testing.T) {
	t.Parallel()
	clean := strconv.Quote(configtest.Git(t)) + " stripspace --comment-lines"
	g, root, base := committed(t, "f.txt filter=mark\n", "f.txt", "hello\n", "filter.mark.clean", clean, "filter.mark.required", "true")
	if base.Size != int64(len("# hello\n")) {
		t.Fatalf("the base of f.txt is %d bytes; want the clean program's %d, so the set-up ran it", base.Size, len("# hello\n"))
	}
	ids, err := g.WorkObjects(t.Context(), root, []string{"f.txt"})
	if err != nil || len(ids) != 1 || ids[0] == "" || ids[0] == base.Object {
		t.Errorf("WorkObjects(f.txt) = %q, %v; want the id of the bytes on disk, not the clean program's, %s", ids, err, base.Object)
	}
}

// TestAPathGitWouldUnquoteIsNotHashed:
// --stdin-paths C-unquotes a line that begins with a double quote, so the
// line "a.go" names a.go. A work-tree file literally named "a.go", with bytes
// of its own, beside an a.go that equals its base, answers no id, and the
// count check reads it; a.go still answers its base's id. A system that
// cannot hold the name cannot hold the file, and the row holds there with
// nothing to hash. Planted failure: the leading-quote guard in regularFiles
// left out, so "a.go" answers a.go's id and would pass as unchanged.
func TestAPathGitWouldUnquoteIsNotHashed(t *testing.T) {
	t.Parallel()
	g, root, base := committed(t, "", "a.go", "package a\n")
	quoted := `"a.go"`
	if err := os.WriteFile(filepath.Join(string(root), quoted), []byte("package a\n\n//nolint\n"), 0o600); err != nil {
		t.Logf("this system does not hold the name %s: %v", quoted, err)
	}
	ids, err := g.WorkObjects(t.Context(), root, []string{quoted, "a.go"})
	if err != nil || len(ids) != 2 || ids[0] != "" || ids[1] != base.Object {
		t.Errorf("WorkObjects(%s, a.go) = %q, %v; want no id for %s and the base's %s for a.go", quoted, ids, err, quoted, base.Object)
	}
}
