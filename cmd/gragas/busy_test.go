package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestUnderApplyOverHostFsACompetingProcessIsBusy: two processes over one
// root, each with its own host Lock on the one state folder, as two gragas
// processes have. The first takes the root lock before it plans, so the
// second, started while the first's plan function runs, ends busy without
// planning, and the first ends done. Risk: a lock the fake holds and a disk
// does not, or one taken after the plan (store.consistency.1, A44).
func TestUnderApplyOverHostFsACompetingProcessIsBusy(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir, state := filepath.Join(base, "root"), filepath.Join(base, "state")
	if err := os.MkdirAll(filepath.Join(dir, "rules"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules", "a.yaml"), []byte("status: active\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	clk := w.Ports().Clock
	tree := newTree(t)
	process := func() port.Ports {
		return port.Ports{
			Tree: tree, FileNames: tree, Writer: hostfs.Writer{Tree: tree}, Lock: &hostfs.Lock{Dir: state, Clock: clk},
			Journal: hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: clk, Dir: state},
		}
	}
	root, err := tree.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := func(run string, plan apply.PlanFunc) apply.Spec {
		return apply.Spec{
			Command: "gragas test --apply", Engine: "v0.0.0-test", Run: run, Scope: []string{"rules/*.yaml"},
			Plan: plan,
			Render: func(model.Delta) ([]model.Target, error) {
				b, err := os.ReadFile(filepath.Join(dir, "rules", "a.yaml"))
				if err != nil {
					return nil, err
				}
				return []model.Target{{Path: "rules/a.yaml", Before: apply.DigestBytes(b), Data: []byte("status: retired\n")}}, nil
			},
		}
	}
	retire := model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "rule:a", Field: "status", Before: "active", After: "retired"}}}}
	var second apply.Outcome
	secondPlanned := false

	first := apply.Run(t.Context(), process(), root, spec("run-1", func(ctx context.Context) (model.Plan, error) {
		second = apply.Run(ctx, process(), root, spec("run-2", func(context.Context) (model.Plan, error) {
			secondPlanned = true
			return retire, nil
		}))
		return retire, nil
	}))

	if first.Word != model.Done {
		t.Fatalf("the first process ended %q %+v, cause %v; want done", first.Word, first.Failure, first.Cause)
	}
	if second.Word != model.Busy || second.Refusal == nil || secondPlanned {
		t.Errorf("the second process ended %q (refusal %v), planned %v; want busy without a plan", second.Word, second.Refusal, secondPlanned)
	}
}
