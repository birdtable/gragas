package git

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/model"
)

// TestBlobsPrintedWrongAreFailedNeverShortOrEmpty: the objects
// one commit reads come back from one cat-file --batch, and what git prints
// is held to the listing: an object git answers missing for is failed, not
// read as an empty file; a header that gives another size than the listing's,
// one whose bytes are not the size it gives, a header that names another
// object, and bytes past the last object are each failed.
// Risk: a missing object read as empty, or a cut answer passed on as a file.
func TestBlobsPrintedWrongAreFailedNeverShortOrEmpty(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	other := strings.Repeat("4", 40)
	for name, out := range map[string]string{
		"an object git has not":        standBlob + " missing\n",
		"a size other than the listed": standBlob + " blob 3\nxxx\n",
		"bytes short of the size":      standBlob + " blob 2\nx",
		"no newline after the bytes":   standBlob + " blob 2\nxxy",
		"another object":               other + " blob 2\nxx\n",
		"another type":                 standBlob + " tree 2\nxx\n",
		"no header":                    "xx\n",
		"more than was asked for":      standBlob + " blob 2\nxx\nmore\n",
		"a header with a sign":         standBlob + " blob +2\nxx\n",
	} {
		t.Run(name, func(t *testing.T) {
			g := program(t, withBlobs(t, standIn("2.56.0", "", nil), Answer{Out: out})...)
			got, err := g.Blobs(t.Context(), root, "v1", []string{"a"})
			if !isFailed(err) || got != nil {
				t.Errorf("%s: %q, %v; want failed and no content", name, got, err)
			}
		})
	}
	// the control: what git prints for the listing is read, and the run is the batch's
	g := program(t, withBlobs(t, standIn("2.56.0", "", nil), Answer{Out: standBlob + " blob 2\nxx\n"})...)
	got, err := g.Blobs(t.Context(), root, "v1", []string{"a"})
	if err != nil || len(got) != 1 || string(got[0]) != "xx" {
		t.Errorf("the control: %q, %v; want xx", got, err)
	}
	if !slices.Contains(verbsRun(g), verbBlobs) || slices.Contains(verbsRun(g), verbBlob) {
		t.Errorf("the verbs run are %q, want the batch alone", verbsRun(g))
	}
}

// TestBlobsPastTheCapAreFailedBeforeAnythingIsRead: a file the
// listing says is past the cap is failed without cat-file running, as Blob
// does, and a path the tree lacks is fs.ErrNotExist, so the answer is never
// cut short or partial.
// Risk: the batch reading what Blob refuses, or answering the files before
// the one it refuses.
func TestBlobsPastTheCapAreFailedBeforeAnythingIsRead(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	big := with(t, standIn("2.56.0", "", nil), "ls-tree",
		Answer{Out: fmt.Sprintf("100644 blob %s 2\ta\x00100644 blob %s %d\tb\x00", standBlob, standBlob, maxListing+1)})
	g := program(t, big...)
	if got, err := g.Blobs(t.Context(), root, "v1", []string{"a", "b"}); !errors.Is(err, errPastCap) || !isFailed(err) || got != nil {
		t.Errorf("a file past the cap: %q, %v; want failed past the cap and no content", got, err)
	}
	if slices.Contains(verbsRun(g), verbBlobs) {
		t.Error("cat-file ran for a file past the cap")
	}
	if got, err := g.Blobs(t.Context(), root, "v1", []string{"a", "gone"}); !errors.Is(err, fs.ErrNotExist) || got != nil {
		t.Errorf("a path the tree lacks: %q, %v; want fs.ErrNotExist and no content", got, err)
	}
	if got, err := g.Blobs(t.Context(), root, "v1", nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("no path: %v, %v; want an empty list", got, err)
	}
}

// TestBlobsPastTheCapTogetherAreReadInMoreThanOneRun: files
// that each fit the cap and together do not are read in a run for each cap's
// worth, so no run's answer is cut at the cap.
// Risk: one run for any number of files, its answer cut at the cap and failed.
func TestBlobsPastTheCapTogetherAreReadInMoreThanOneRun(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	const size = 3 << 20
	rules := with(t, standIn("2.56.0", "", nil), "ls-tree",
		Answer{Out: fmt.Sprintf("100644 blob %s %d\ta\x00100644 blob %s %d\tb\x00", standBlob, size, standBlob, size)})
	rules = withBlobs(t, rules, Answer{Out: fmt.Sprintf("%s blob %d\n%s\n", standBlob, size, strings.Repeat("x", size))})
	g := program(t, rules...)
	got, err := g.Blobs(t.Context(), root, "v1", []string{"a", "b"})
	if err != nil || len(got) != 2 || len(got[0]) != size || len(got[1]) != size {
		t.Fatalf("two files of %d bytes: %d answered, %v; want both", size, len(got), err)
	}
	runs := 0
	for _, v := range verbsRun(g) {
		if v == verbBlobs {
			runs++
		}
	}
	if runs != 2 {
		t.Errorf("the batch ran %d times, want 2, a run for each cap's worth", runs)
	}
}

// TestAFileAtTheCapIsReadByBlobsAsBlobReadsIt: a
// file of exactly maxListing bytes is read, alone, though its header and the
// newline after it run the answer past maxListing.
// Risk: a file Blob reads that Blobs refuses, failing a commit's check.
func TestAFileAtTheCapIsReadByBlobsAsBlobReadsIt(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	rules := with(t, standIn("2.56.0", "", nil), "ls-tree",
		Answer{Out: fmt.Sprintf("100644 blob %s %d\ta\x00", standBlob, maxListing)})
	rules = withBlobs(t, rules, Answer{Out: fmt.Sprintf("%s blob %d\n%s\n", standBlob, maxListing, strings.Repeat("x", maxListing))})
	got, err := program(t, rules...).Blobs(t.Context(), root, "v1", []string{"a"})
	if err != nil || len(got) != 1 || len(got[0]) != maxListing {
		t.Errorf("a file of %d bytes: %d answered, %v; want it read", maxListing, len(got), err)
	}
}
