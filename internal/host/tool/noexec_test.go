package tool_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/tool"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
)

// TestAToolOnADiskTheWalkCannotJudgeIsRefusedWithTheMountFix proves on every
// system, with no disk mounted, what TestAToolOnANoexecDiskIsRefusedUnlessItsMountIsNamed
// proves only where go-files can mount a test disk (bind_test.go):
// the trust walk's answer for a program on a mount whose owners go-files does
// not judge — ErrMount: a disk mounted noexec without nosuid, a nosuid disk, or
// a file system of a type it does not know — is the adapter's own refusal. That
// is a *tool.Refused that matches port.ErrRefused and hints hosttrust.MountFix,
// the line that names GRAGAS_MOUNTS. The walk reads nothing of the system
// (trust.go), and tool's seam for its answer is WalkRefusal, so planting the
// answer holds the mapping on every system; bind_test.go keeps the proof, where
// go-files can mount a test disk, that a real such disk gives this answer, and
// is left alone.
//
// The planted answer is the one the walk builds for ErrMount: go-files gives no
// fix of its own, so the walk's fix is MountFix and the name is one it cannot
// judge (trust.go moveLine and unjudged). No mount is made.
//
// Risk: a program on a disk gragas cannot judge is let run unchecked where no
// test disk can be mounted to prove it, or its refusal reaches a person with no MountFix, so they never learn to name
// the mount in GRAGAS_MOUNTS and the setting.
func TestAToolOnADiskTheWalkCannotJudgeIsRefusedWithTheMountFix(t *testing.T) {
	t.Parallel()
	const observed = "the tool at /work/disk/lint-a is on a disk mounted noexec, whose owners go-files does not judge"
	answer := &hosttrust.Unsafe{Observed: observed, Fix: hosttrust.MountFix, Unjudged: true}

	err := tool.WalkRefusal(fmt.Errorf("walk: %w", answer))
	r, ok := errors.AsType[*tool.Refused](err)
	if !ok || !errors.Is(err, port.ErrRefused) || r.Hint() != hosttrust.MountFix || !strings.Contains(r.Observed(), observed) {
		t.Fatalf("a tool on a disk the walk cannot judge = %v; want a *tool.Refused matching port.ErrRefused, saying %q, with the fix naming %s", err, observed, hosttrust.MountsVar)
	}

	// The guard's negative: the same answer with the mount fix dropped is still a
	// refusal, but its hint is no longer MountFix (judged gives the move-the-program
	// line), so a change that drops the hint — in the walk's answer or the
	// adapter's mapping — fails the MountFix check above.
	dropped := tool.WalkRefusal(fmt.Errorf("walk: %w", &hosttrust.Unsafe{Observed: observed}))
	if d, ok := errors.AsType[*tool.Refused](dropped); !ok || d.Hint() == hosttrust.MountFix {
		t.Fatalf("with the mount fix dropped = %v; want a *tool.Refused whose hint is not MountFix", dropped)
	}
}
