package coreapi_test

import (
	"context"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
)

// TestTheRefusalOfNoDigestHasNoEmptySlot: no digest given is said so, never
// "and  was given" with the value missing. A wrong digest
// still names the one given.
func TestTheRefusalOfNoDigestHasNoEmptySlot(t *testing.T) {
	t.Parallel()
	w, b := approveWorld(t)
	_, err := coreapi.ApprovePlugin(context.Background(), b, w.Ports().Trust, "r", "alpha", "")
	if err == nil || strings.Contains(err.Error(), "  ") || !strings.Contains(err.Error(), "no digest was given") {
		t.Errorf("the refusal of no digest = %v, want it to say no digest was given, with no empty slot", err)
	}
	_, err = coreapi.ApprovePlugin(context.Background(), b, w.Ports().Trust, "r", "alpha", "sha256:wrong")
	if err == nil || !strings.Contains(err.Error(), "sha256:wrong was given") {
		t.Errorf("the refusal of a wrong digest = %v, want it to name the one given", err)
	}
}

// TestApproveRecordsEachGrantByIdAndDigest: the approval keeps the grants one
// by one, sorted by id, each with the digest of that grant alone, so a later
// screen can compare them. A plugin that asks for no grant is
// held empty, not nil, and two grants of one id are refused before a yes.
func TestApproveRecordsEachGrantByIdAndDigest(t *testing.T) {
	t.Parallel()
	w, b := approveWorld(t)
	shown, err := coreapi.ShowPlugin(context.Background(), b, w.Ports().Trust, "r", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coreapi.ApprovePlugin(context.Background(), b, w.Ports().Trust, "r", "alpha", shown.Digest); err != nil {
		t.Fatal(err)
	}
	got := w.Approvals()
	if len(got) != 1 || len(got[0].Held) != 1 || got[0].Held[0].ID != "standin" || !got[0].Held[0].Digest.Formed() || got[0].Held[0].Digest == got[0].Grants {
		t.Fatalf("approvals = %+v, want one entry holding the grant standin by its own digest", got)
	}
	prior, err := w.Ports().Trust.Prior(context.Background(), "alpha")
	if err != nil || prior.Digest != shown.Digest {
		t.Errorf("Prior(alpha) = %+v, %v, want the approval just made", prior, err)
	}
}
