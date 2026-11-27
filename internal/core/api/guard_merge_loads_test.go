package coreapi_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// countedHistory counts the lists of a rev's files, by rev: each is one git
// process of the real History.
type countedHistory struct {
	port.History
	mu    sync.Mutex
	files map[string]int
}

func (h *countedHistory) Files(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
	h.mu.Lock()
	h.files[rev]++
	h.mu.Unlock()
	return h.History.Files(ctx, root, rev)
}

// TestAMergeReadsEachTreeOfTheChainItBringsInOnce (gragas#1205): a merge that
// brings in a chain of N commits over a rulebook lists each commit's tree
// once, and the merge base once: N+1 trees, not the 2N+ a commit's tree before
// and its tree after list when each is read anew, as the next commit's tree
// before is this one's tree after, and the merge's own three-way reads the
// base and the other side's tip again. The plant is a merge with no cache of
// the trees by rev (s.cache nil), which lists the base twice and each commit
// but the first twice or more. A time budget waits for a journey-time kind in
// gate/budgets.yaml, gate's: this holds the cost by count.
// Risk: a branch far behind main shows the cost of a merge in the commit hook,
// two tree listings and two book loads for each commit it brings in.
func TestAMergeReadsEachTreeOfTheChainItBringsInOnce(t *testing.T) {
	t.Parallel()
	const n = 6
	w, root, made := initOver(t)

	revs := []porttest.Rev{{Commit: cB, Files: made}}
	parents := map[string][]string{cB: {}, cO: {cB}}
	chain := make([]string, n)
	files := made
	parent := cB
	for i := range chain {
		chain[i] = fmt.Sprintf("%040x", i+1)
		files = with(files, map[string]string{fmt.Sprintf("x%d.txt", i): "an ordinary commit\n"})
		parents[chain[i]] = []string{parent}
		parent = chain[i]
		if i < n-1 {
			revs = append(revs, porttest.Rev{Commit: chain[i], Files: files})
		}
	}
	ours := with(made, map[string]string{"note.txt": "main moved on\n"})
	tip := chain[n-1]

	w.SeedRepo(root, porttest.Repo{
		Who:     &model.Identity{Name: "Ada", Email: "ada@example.org"},
		Merging: []string{"theirs"},
		Heads:   map[string]porttest.Rev{"HEAD": {Commit: cO, Files: ours}, "theirs": {Commit: tip, Files: files}},
		Revs:    revs,
		Parents: parents,
		Bases:   map[[2]string]string{{"HEAD", "theirs"}: cB},
		Index:   with(files, map[string]string{"note.txt": "main moved on\n"}),
	})
	ports := w.Ports()
	h := &countedHistory{History: ports.History, files: map[string]int{}}
	ports.History = h
	_, ref, err := coreapi.GuardStaged(t.Context(), ports, coreapi.StagedAsk{Root: root, Replay: initReplay(root), Run: "guard-1", Engine: "v0.0.0-test"})
	allowedMerge(t, "the merge of a chain", ref, err)

	for i, rev := range append([]string{cB}, chain...) {
		if got := h.files[rev]; got != 1 {
			t.Errorf("the files of tree %d of the chain (0 is the merge base) were listed %d times; want once", i, got)
		}
	}
}
