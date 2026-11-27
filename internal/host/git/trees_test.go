package git

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/model"
)

// The two commits and the blob of the moving stand-in.
const (
	treeCommitA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	treeCommitB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	treeBlob    = "cccccccccccccccccccccccccccccccccccccccc"
)

// moving is a stand-in for git whose v1 names commit A until the file moved
// exists in its folder, and commit B after; A holds the file a, B the file
// b, each of two bytes.
func moving() []Rule {
	return []Rule{
		{End: " version", Answer: Answer{Out: "git version 2.56.0\n"}},
		{End: "--is-shallow-repository", Answer: Answer{Out: "false\n"}},
		{End: "--is-inside-work-tree --show-prefix", Answer: Answer{Out: "true\n\n"}},
		{End: "--show-prefix", Answer: Answer{Out: "\n"}},
		{Has: []string{"rev-parse --verify"}, If: "moved", Answer: Answer{Out: treeCommitB + "\n"}},
		{Has: []string{"rev-parse --verify"}, Answer: Answer{Out: treeCommitA + "\n"}},
		{Has: []string{"ls-tree", treeCommitA}, Answer: Answer{Out: "100644 blob " + treeBlob + " 2\ta\x00"}},
		{Has: []string{"ls-tree", treeCommitB}, Answer: Answer{Out: "100644 blob " + treeBlob + " 2\tb\x00"}},
		{Has: []string{"ls-files"}, Answer: Answer{Out: "100644 " + treeBlob + " 2 0\ta\x00"}},
		{Has: []string{"cat-file"}, Answer: Answer{Out: "x\n"}},
	}
}

func runs(g *Git, verb string) int {
	n := 0
	for _, v := range verbsRun(g) {
		if v == verb {
			n++
		}
	}
	return n
}

// TestASecondReadOfACommitRunsNoLsTree: a commit's listing is read once by
// this value, whichever file is read from it; a copy of it is answered, so
// a caller that cuts its answer short leaves the next read whole.
// Planted failures: no cache, so each read lists the tree again; the
// cached slice itself handed out.
func TestASecondReadOfACommitRunsNoLsTree(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	g := program(t, moving()...)
	for range 3 {
		if _, err := g.Blob(t.Context(), root, "v1", "a"); err != nil {
			t.Fatal(err)
		}
	}
	if n := runs(g, verbTree); n != 1 {
		t.Errorf("ls-tree ran %d times for three reads of one commit, want 1", n)
	}
	files, err := g.files(t.Context(), root, "v1")
	if err != nil {
		t.Fatal(err)
	}
	files[0].path = "changed"
	if again, _ := g.files(t.Context(), root, "v1"); len(again) != 1 || again[0].path != "a" {
		t.Errorf("a caller's change of its answer reached the next read: %v", again)
	}
}

// TestAMovedRefReturnsTheNewTree: v1 named commit A at the first read and
// names B at the second, and the second answers B's tree, not A's. The
// index is never kept: each read of it lists it again.
// Planted failures: the cache keyed by the rev as given; the index kept.
func TestAMovedRefReturnsTheNewTree(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	g := program(t, moving()...)
	if _, err := g.Blob(t.Context(), root, "v1", "a"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(string(root), "moved"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Blob(t.Context(), root, "v1", "b"); err != nil {
		t.Errorf("the file the moved ref's commit holds: %v", err)
	}
	if _, err := g.Blob(t.Context(), root, "v1", "a"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the file the old commit held: %v, want not found", err)
	}
	if n := runs(g, verbTree); n != 2 {
		t.Errorf("ls-tree ran %d times for two commits, want 2", n)
	}
	for range 2 {
		if _, err := g.Files(t.Context(), root, ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := runs(g, verbIndex); n != 2 {
		t.Errorf("ls-files ran %d times for two reads of the index, want 2", n)
	}
	if slices.Contains(verbsRun(g), "") {
		t.Error("a run with no verb")
	}
}
