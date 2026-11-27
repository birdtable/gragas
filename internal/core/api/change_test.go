package coreapi_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestEachApplyOfAChangeFilesItsJournalUnderARunIdOfItsOwn: a yes to a plan
// that moved before the lock, then a yes to the new plan. The first apply is
// refused as moved under the spec's run id, the second applies under the
// spec's id with its try, and the journal holds two runs, neither open.
// Risk: a second try filed under the first's id, after its close, where a
// crash of it would never read as open (A37).
func TestEachApplyOfAChangeFilesItsJournalUnderARunIdOfItsOwn(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("/work/rulebook", map[string]string{"identity.yaml": "name: demo\n", "rules/a.yaml": "status: active\n"})
	c := coreapi.NewChange(w.Ports(), root, coreapi.Spec{
		Command: "gragas test", Applies: "gragas test --apply", Engine: "v0.0.0-test", Run: "run-1",
		Scope: []string{"rules/*.yaml"},
		Plan: func(context.Context) (model.Plan, error) {
			return model.Plan{
				Delta:  model.Delta{Changes: []model.Change{{Entity: "rule:a", Field: "status", Before: "active", After: "retired"}}},
				Inputs: []model.Input{{Path: "identity.yaml", Digest: digestOf(t, w, root, "identity.yaml")}},
			}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) {
			return []model.Target{{Path: "rules/a.yaml", Before: digestOf(t, w, root, "rules/a.yaml"), Data: []byte("status: retired\n")}}, nil
		},
	})
	shown, ok := c.Preview(t.Context())
	if !ok {
		t.Fatalf("the preview ended %q, cause %v", shown.Word, shown.Cause)
	}
	if err := w.Ports().Writer.Replace(root, "identity.yaml", []byte("name: renamed\n")); err != nil {
		t.Fatal(err)
	}

	moved := c.Apply(t.Context(), shown.Plan.Digest)
	applied := c.Apply(t.Context(), moved.Plan.Digest)

	if !moved.Moved || moved.Run != "run-1" {
		t.Errorf("the first apply ended %q moved %v under %q, want moved under run-1", moved.Word, moved.Moved, moved.Run)
	}
	if applied.Word != model.Done || applied.Run != "run-1-2" {
		t.Errorf("the second apply ended %q under %q (cause %v), want done under run-1-2", applied.Word, applied.Run, applied.Cause)
	}
	recs, _, err := w.Ports().Journal.Records(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	var runs []string
	for _, r := range recs {
		if !slices.Contains(runs, r.Run) {
			runs = append(runs, r.Run)
		}
	}
	if !slices.Equal(runs, []string{"run-1", "run-1-2"}) {
		t.Errorf("the journal holds runs %q, want run-1 and run-1-2", runs)
	}
}

// digestOf is the digest of the bytes at p, as a target's Before holds it.
func digestOf(t *testing.T, w *porttest.World, root model.Root, p string) model.Digest {
	t.Helper()
	b, err := w.Ports().Tree.Read(root, p)
	if err != nil {
		t.Fatal(err)
	}
	return apply.DigestBytes(b)
}
