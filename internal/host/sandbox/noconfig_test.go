package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/gragas/internal/host/sandbox"
	"github.com/birdtable/gragas/internal/model"
)

// TestAScratchFolderPreparedWithNoConfigHoldsNoFile: a read with no config is prepared at the
// isolated level with none, and its scratch folder holds no file, so a config
// in the repository's root, or in the folder the engine's cache sits in, is not
// in it. What this does not hold, and the row says so: a tool that searches
// its working folder and every parent for its config reaches what is above the
// cache folder, which the isolated level does not stop. Those parents belong
// to the user and the system, never to the repository, and the adapter refuses
// one another account can write (owned_test.go). The floor is read
// again with the sandboxed adapter.
func TestAScratchFolderPreparedWithNoConfigHoldsNoFile(t *testing.T) {
	t.Parallel()
	c := cache(t)
	root := t.TempDir()
	for _, dir := range []string{root, filepath.Dir(c)} {
		if err := os.WriteFile(filepath.Join(dir, "planted.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := sandbox.New(sandbox.Config{Dir: c})
	sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Release(sc) })
	if sc.Level != model.Isolated {
		t.Errorf("the scratch folder is at %v, want isolated", sc.Level)
	}
	if got, ok := files(filepath.FromSlash(sc.Dir)); !ok || len(got) != 0 {
		t.Errorf("the scratch folder holds %v (made: %v), want it made and empty", got, ok)
	}
}
