package coreapi_test

import (
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// TestRestoreNeverReadsOrSaysAPathUnderGit: a closed run whose record names
// .git/config, a hook, .GIT/hooks/pre-commit or a path outside the root makes no line and reads no
// file: the restore is refused, naming the record's path, whatever the
// digests. Planted failure:
// restore without the check, which reads the file and prints rm for it.
func TestRestoreNeverReadsOrSaysAPathUnderGit(t *testing.T) {
	t.Parallel()
	for _, p := range []string{".git/config", ".git/hooks/post-checkout", ".GIT/hooks/pre-commit", "../outside.txt", "/outside.txt", "sub/../../x"} {
		j := newJournalWorld(t, map[string]string{"a.txt": "new"})
		j.record("r1", model.KindIntent, now-300, nil)
		j.copied("r1", p, "", false, now-299)
		j.record("r1", model.KindClose, now-290, object(canon.Object{"outcome": canon.String("done"), "file": canon.String(""), "files": canon.Array{
			canon.Object{"path": canon.String(p), "before": canon.String(""), "after": canon.String("")},
		}}))
		got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
		if !errors.Is(err, coreapi.ErrUnsafePath) {
			t.Errorf("%s: restore ended %v, want %v", p, err, coreapi.ErrUnsafePath)
		}
		if len(got.Lines) != 0 || len(got.Files) != 0 {
			t.Errorf("%s: restore said %q and %v, want nothing", p, got.Lines, got.Files)
		}
	}
}
