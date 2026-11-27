package fs_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
)

// TestAFileWithNoIndexIsNamed is go-files' refusal of a file whose index
// cannot tell it from another (ErrNoFileIndex) in the store's
// words: what is not known, where to keep the store, and go-files' own words
// and sentinel kept under them. Any other error is left to the store's
// generic words.
func TestAFileWithNoIndexIsNamed(t *testing.T) {
	sentinel, ok := hostfs.ReplaceRefusal("no ID")
	if !ok {
		t.Fatal("Replace names no refusal of a file with no ID")
	}
	if !errors.Is(sentinel, errors.ErrUnsupported) {
		t.Errorf("the refusal named for a file with no ID is %v, want go-files' ErrNoFileIndex, which matches errors.ErrUnsupported", sentinel)
	}
	inner := fmt.Errorf("x.yaml %w", sentinel)
	err := hostfs.ReplaceRefused("x.yaml", inner)
	if err == nil {
		t.Fatal("ReplaceRefused(ErrNoFileIndex) = nil, want the store's words")
	}
	for _, want := range []string{"replace x.yaml: ", "no ID that tells it from one put in its place", "keep the store on a local disk whose files keep their own IDs", inner.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ReplaceRefused(ErrNoFileIndex) = %q, want it to hold %q", err, want)
		}
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("ReplaceRefused(ErrNoFileIndex) = %v, want it to match ErrNoFileIndex", err)
	}
	if err := hostfs.ReplaceRefused("x.yaml", errors.New("other")); err != nil {
		t.Errorf("ReplaceRefused(other) = %v, want nil, for the store's generic words", err)
	}
}
