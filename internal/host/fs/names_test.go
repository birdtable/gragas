package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// TestTheAdapterPassesTheNamesSuite: each second name is a hard link made
// in the folder, which git never carries but something on the machine may
// make.
func TestTheAdapterPassesTheNamesSuite(t *testing.T) {
	t.Parallel()
	contracttest.Names(t, func(t *testing.T, files, names map[string]string) (port.Tree, string) {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "root")
		write(t, dir, files)
		for second, first := range names {
			if err := os.Link(filepath.Join(dir, filepath.FromSlash(first)), filepath.Join(dir, filepath.FromSlash(second))); err != nil {
				t.Fatal(err)
			}
		}
		return newTree(t), dir
	})
}
