package coreapi_test

import (
	"io"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
)

// countedView is a work tree of one regular file, big.txt, whose Stat says it
// is past the cap, and whose handle counts the bytes read from it. The view
// is what the project check wraps in readFS.
type countedView struct {
	fs.FS
	read *atomic.Int64
}

func (v countedView) Open(name string) (fs.File, error) {
	if name == "big.txt" {
		return countedFile{read: v.read}, nil
	}
	return v.FS.Open(name)
}

type countedFile struct{ read *atomic.Int64 }

func (countedFile) Stat() (fs.FileInfo, error) {
	return statInfo{size: coreapi.MaxRead + 1}, nil
}
func (countedFile) Close() error { return nil }
func (f countedFile) Read(p []byte) (int, error) {
	left := coreapi.MaxRead + 1 - f.read.Load()
	if left <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), left)
	f.read.Add(n)
	return int(n), nil
}

// TestAFilePastTheCapThroughTheProjectChecksViewIsSkippedWithNothingRead is
// the row that joins the two halves of the bound: the project check hands the
// audit its view wrapped in readFS, and a file past the cap on that path is
// named in a row and has not a byte read.
// Planted failure: readFS answers Open from a whole read of the file, so the
// handle it hands back is a memory file whose bytes were all read first.
func TestAFilePastTheCapThroughTheProjectChecksViewIsSkippedWithNothingRead(t *testing.T) {
	t.Parallel()
	read := new(atomic.Int64)
	a := trackedAudit(t, map[string]string{"big.txt": "x", "small.txt": "about zorbquux\n"}, nil)
	a.Src = coreapi.ReadFSOf(countedView{FS: a.Src, read: read})
	r := coreapi.CheckRepo(t.Context(), a)
	if n := read.Load(); n != 0 {
		t.Errorf("%d bytes were read of a file past the cap, want none", n)
	}
	said := false
	for _, row := range r.NotChecked {
		said = said || strings.Contains(row.Why, "big.txt is larger than")
	}
	if !said {
		t.Errorf("no row names big.txt as past the cap; rows %v", r.NotChecked)
	}
}
