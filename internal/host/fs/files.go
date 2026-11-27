package fs

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// Files fills legacy.Files from the disk.
type Files struct{}

// Repo is the repository at dir, read under os.Root.
func (Files) Repo(dir string) (legacy.Source, error) { return Open(dir) }

// Open opens the file at path for reading. It is the private forbidden-names
// file, which lives outside every repository: its path comes from the
// environment, and its contents are never printed. It is read whole by
// host/trust's owned read with no mounts, the strict side, as the Tree's
// ReadOwned reads it, and a refusal matches port.ErrNotOwned.
func (Files) Open(path string) (io.ReadCloser, error) {
	data, err := trust.ReadOwned(path, nil)
	if o, ok := errors.AsType[*trust.Owned](err); ok {
		return nil, fmt.Errorf("open names file: %w", notOwned(o))
	}
	if err != nil {
		return nil, fmt.Errorf("open names file: %w", err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// notOwned is o, the owned read's refusal, as the port has it: it matches
// port.ErrNotOwned and the port's error of o's reason, where it has one, and
// keeps o, and so its Refusal.
func notOwned(o *trust.Owned) error {
	if r := reasonErr(o.Reason); r != nil {
		return fmt.Errorf("%w: %w: %w", port.ErrNotOwned, r, o)
	}
	return fmt.Errorf("%w: %w", port.ErrNotOwned, o)
}

// reasonErr is the port's error of the reason r of an owned read's refusal,
// nil for trust.Other.
func reasonErr(r trust.Reason) error {
	switch r {
	case trust.ByOther:
		return port.ErrOwnedByOther
	case trust.Writable:
		return port.ErrOwnedWritable
	case trust.IsLink:
		return port.ErrOwnedLink
	case trust.NotRegular:
		return port.ErrOwnedNotRegular
	case trust.TooLong:
		return port.ErrOwnedTooLong
	case trust.OnMount:
		return port.ErrOwnedMount
	case trust.MountEntry:
		return port.ErrOwnedMountEntry
	case trust.Other:
	}
	return nil
}
