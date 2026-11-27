package git_test

import (
	"bytes"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// TestBlobsReadEveryFileOfACommitInOneRunAndAnswerAsBlobDoes:
// against a real git, the files of a commit come back from one cat-file
// --batch, byte for byte what Blob answers for each alone, an empty file as an
// empty slice, and the verb that reads one object does not run.
// Risk: a batch that differs from the one-by-one read in a byte, or one git
// process still run for each file.
func TestBlobsReadEveryFileOfACommitInOneRunAndAnswerAsBlobDoes(t *testing.T) {
	home, root := makeHistory(t, contracttest.TheHistory())
	g := adapter(t, configtest.Git(t), home)
	files, err := g.Files(t.Context(), root, "v2")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		if !f.Dir && !f.Link {
			paths = append(paths, f.Path)
		}
	}
	if len(paths) < 2 {
		t.Fatalf("the fixture's v2 holds %d regular files, want several", len(paths))
	}
	got, err := g.Blobs(t.Context(), root, "v2", paths)
	if err != nil || len(got) != len(paths) {
		t.Fatalf("Blobs(v2): %d files, %v; want %d", len(got), err, len(paths))
	}
	for i, p := range paths {
		want, err := g.Blob(t.Context(), root, "v2", p)
		if err != nil || !bytes.Equal(got[i], want) || got[i] == nil {
			t.Errorf("%s: batch %q, Blob %q (%v); want the same, never nil", p, got[i], want, err)
		}
	}
	var batches int
	for _, r := range g.Ran() {
		if r.Verb == "blob-objects" {
			batches++
		}
	}
	if batches != 1 {
		t.Errorf("the batch ran %d times for %d files, want once", batches, len(paths))
	}
}
