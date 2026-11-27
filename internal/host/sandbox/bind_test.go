package sandbox_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"

	"github.com/birdtable/gragas/internal/host/sandbox"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TestACacheOnANosuidDiskIsRefusedUnlessItsMountIsNamed, a
// Bind plant: it needs a disk mounted nosuid, which only a user namespace
// that keeps its capabilities gives without root, so it runs where
// trusttest.Bind can mount one and Bind skips it elsewhere. A cache on such a disk, whose owners go-files does not judge,
// refuses Prepare as the disk's, with MountFix, which names GRAGAS_MOUNTS
// and the mounts setting; the same cache, with the folder the disk is
// mounted on in Mounts, prepares a scratch folder and releases it. In the
// namespace root's / is the overflow account's, which host/trust's Held of
// a test binary takes. Planted: Mounts not handed to Held (the second
// Prepare refused); Held asked without Who.Overflow (the entry refused, the
// second Prepare too); ErrMount answered as another account's write (the
// first refusal's words).
func TestACacheOnANosuidDiskIsRefusedUnlessItsMountIsNamed(t *testing.T) {
	t.Parallel()
	entry, dir := trusttest.Bind(t, trusttest.Nosuid)
	_, err := sandbox.New(sandbox.Config{Dir: dir}).Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	if !errors.Is(err, port.ErrRefused) || !errors.Is(err, trust.ErrMount) || !strings.Contains(err.Error(), hosttrust.MountFix) {
		t.Fatalf("a cache on a nosuid disk, no mounts named = %v; want refused with ErrMount and the fix naming %s", err, hosttrust.MountsVar)
	}
	s := sandbox.New(sandbox.Config{Dir: dir, Mounts: []string{entry}})
	sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	if err != nil {
		t.Fatalf("the same cache with %s named = %v; want it prepared", entry, err)
	}
	if err := s.Release(sc); err != nil {
		t.Errorf("Release of %s: %v", sc.Dir, err)
	}
}
