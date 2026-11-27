package tool_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"

	"github.com/birdtable/gragas/internal/host/tool"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
)

// TestAToolOnANoexecDiskIsRefusedUnlessItsMountIsNamed, a Bind
// plant: it needs a disk mounted noexec without nosuid, which only a user
// namespace that keeps its capabilities gives without root, so Bind skips it
// where the system gives none. A program on such a disk, whose owners go-files does not
// judge, is refused by the trust walk with MountFix, which names
// GRAGAS_MOUNTS and the mounts setting; with the folder the disk is mounted
// on in Mounts the walk passes it and answers its path. Planted: Mounts not
// handed to the walk (the second walk refused); the disk judged by nosuid
// alone (the first walk passes unnamed).
func TestAToolOnANoexecDiskIsRefusedUnlessItsMountIsNamed(t *testing.T) {
	t.Parallel()
	entry, dir := trusttest.Bind(t, trusttest.NoexecWithoutNosuid)
	p := filepath.ToSlash(place(t, dir, "lint"))
	_, err := tool.TrustedOn(p, nil)
	if r, ok := errors.AsType[*tool.Refused](err); !ok || !errors.Is(err, port.ErrRefused) || r.Hint() != hosttrust.MountFix {
		t.Fatalf("a program on a noexec disk, no mounts named = %v; want refused with the fix naming %s", err, hosttrust.MountsVar)
	}
	if got, err := tool.TrustedOn(p, []string{entry}); err != nil || got != p {
		t.Errorf("the same program with %s named = %q, %v; want %q", entry, got, err, p)
	}
}
