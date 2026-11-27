package fs

// The adapter of legacy.Writer, folded here from the old kit adapter: the
// generated files of the old `gen --apply` written into one repository
// through go-files' write.FileIn, each replaced whole, flushed, at mode 0644,
// as Writer.Replace in writer.go writes a store's. It serves the old
// commands only: api.Apply, which cmd/gragas and cmd/gragas-subset reach,
// and the run recorder of cli/root/recorder.go. It goes with the old
// commands.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/birdtable/go-files/write"

	"github.com/birdtable/gragas/internal/port/legacy"
)

// LegacyWriter fills legacy.Writer.
type LegacyWriter struct{ as func() string }

// Write replaces path, slash-separated and relative to dir, with data,
// making its folder (0755) when it is missing, in a repository this account
// owns: dir is opened once as a root, and every step after goes
// through it, so a link or a folder swapped in on the way cannot carry the
// write out of it.
func (w LegacyWriter) Write(dir, path string, data []byte) (err error) {
	local := filepath.FromSlash(path)
	if !filepath.IsLocal(local) {
		return fmt.Errorf("write %s: the path leaves the repository", path)
	}
	repo, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() { err = errClosed(err, repo.Close()) }()
	if err := mine("the repository", repo, w.as); err != nil {
		return fmt.Errorf("write %s: %w", path, oldNotOwner{err})
	}
	if err := repo.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := write.FileIn(repo, local, data, write.Mode(0o644)); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// oldNotOwner is mine's refusal, said as it is, which matches
// legacy.ErrNotOwner too, since the old commands see only the old ports.
type oldNotOwner struct{ error }

func (e oldNotOwner) Is(target error) bool { return target == legacy.ErrNotOwner }

func (e oldNotOwner) Unwrap() error { return e.error }
