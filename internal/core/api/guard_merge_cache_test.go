package coreapi

import (
	"bytes"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestATreeBeforeIsAForkAndAWriteOverItLeavesTheCachedTreeAlone (gragas#1205):
// a replay writes its lines and files over the tree before it, and that tree
// is the cache's own: the next judge of the merge reads it again as the tree
// after of one commit and the tree before of the next, and the three-way reads
// the merge base from it. A write that reached the cached tree would make
// every later judge read a base that already holds a commit's lines.
// The plant is treeBefore answering the cached tree itself (guard_merge_cache.go,
// `return t.fork()` as `return t`): the path written below is then listed in
// the second answer, and history.jsonl reads the new bytes.
// Risk: a correct merge refused, or a refused one let through, over a base
// that is no longer the base.
func TestATreeBeforeIsAForkAndAWriteOverItLeavesTheCachedTreeAlone(t *testing.T) {
	t.Parallel()
	const rev = "1111111111111111111111111111111111111111"
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{})
	w.SeedRepo(root, porttest.Repo{
		Who: &model.Identity{Name: "Ada", Email: "ada@example.org"},
		Revs: []porttest.Rev{{Commit: rev, Files: map[string]string{
			"history.jsonl": "{\"line\":1}\n",
			"rulebook.yaml": "a: b\n",
		}}},
	})
	p := w.Ports()
	ask := StagedAsk{Root: root}
	cache := newRevCache()

	listed, err := cache.listed(t.Context(), p, ask, rev)
	if err != nil {
		t.Fatal(err)
	}
	was := slices.Clone(listed.paths())
	before, err := listed.Read(root, "history.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	fork := cache.treeBefore(listed)
	fork.write("history.jsonl", []byte("{\"line\":1}\n{\"line\":2}\n"))
	fork.write("made.txt", []byte("a replay's file\n"))

	again, err := cache.listed(t.Context(), p, ask, rev)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(again.paths(), was) {
		t.Errorf("the cached tree lists %v after a write over the tree before; want %v", again.paths(), was)
	}
	got, err := again.Read(root, "history.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, before) {
		t.Errorf("the cached history.jsonl reads %q after a write over the tree before; want %q", got, before)
	}
	if mine, _ := fork.Read(root, "history.jsonl"); bytes.Equal(mine, before) {
		t.Errorf("the fork does not hold the write: history.jsonl reads %q", mine)
	}
}
