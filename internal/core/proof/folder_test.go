package proof_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// packageTool is the read of a tool that reads the folder it runs in as one
// package, as a package-scoped check does: it flags "unused <path>" for a file
// that declares a name ("decl NAME") no file of the folder uses ("use NAME"),
// and "no-x <path>" for a file holding the line "x". It is the probe that
// found a kept cell differing from a forced run's, kept as the test.
func packageTool(w *porttest.World, tree port.Tree) proof.Findings {
	return func(_ context.Context, _ proof.Job, sc model.Scratch, _ []string) (proof.Side, error) {
		files, _ := w.Scratch(sc)
		text := map[string]string{}
		used := map[string]bool{}
		for _, f := range files {
			data, err := tree.Read(model.Root(sc.Dir), f)
			if err != nil {
				return proof.Side{}, err
			}
			text[f] = string(data)
			for l := range strings.SplitSeq(string(data), "\n") {
				if name, ok := strings.CutPrefix(l, "use "); ok {
					used[name] = true
				}
			}
		}
		var out strings.Builder
		for _, f := range files {
			for l := range strings.SplitSeq(text[f], "\n") {
				if name, ok := strings.CutPrefix(l, "decl "); ok && !used[name] {
					out.WriteString("unused " + f + "\n")
				}
				if l == "x" {
					out.WriteString("no-x " + f + "\n")
				}
			}
		}
		return sideRan(0, words(out.String())...), nil
	}
}

// TestAKeptCellIsWhatAForcedRunGives: two bindings whose pairs
// stand in one folder, under a tool that reads the folder as a whole. Whatever
// the grid a plan starts from, the grid a plan and run leaves is the grid a
// forced run of the same tree leaves, so no cell kept or run alone carries a
// result proven beside other neighbours than a full run gives it.
func TestAKeptCellIsWhatAForcedRunGives(t *testing.T) {
	t.Parallel()
	a := proof.Case{Rule: "go.a.1",
		Pair:    proof.Pair{Number: 1, Digest: "sha256:a1", Good: proof.Example{Name: "good.go", Data: []byte("ok")}, Bad: proof.Example{Name: "bad.go", Data: []byte("x")}},
		Binding: model.Binding{Tool: "lint", Target: "code", Key: "no-x", Rung: 3, Pair: 1, PairPath: "pkg/a.go"}}
	b := proof.Case{Rule: "go.b.1",
		Pair:    proof.Pair{Number: 1, Digest: "sha256:b1", Good: proof.Example{Name: "good.go", Data: []byte("decl helper\nuse helper")}, Bad: proof.Example{Name: "bad.go", Data: []byte("decl helper")}},
		Binding: model.Binding{Tool: "lint", Target: "code", Key: "unused", Rung: 3, Pair: 1, PairPath: "pkg/b.go"}}
	// Pair A rewritten: both its examples now use the name B's bad example
	// declares, so B's bad example draws no finding beside it. B is untouched.
	a2 := a
	a2.Pair.Digest = "sha256:a2"
	a2.Pair.Good.Data, a2.Pair.Bad.Data = []byte("ok\nuse helper"), []byte("x\nuse helper")

	for _, tc := range []struct {
		name string
		// grid is the grid the plan starts from, given the cells of a
		// forced run of the old tree and of the new one.
		grid func(old, now []model.Cell) []model.Cell
	}{
		{"a neighbour's pair changed", func(old, _ []model.Cell) []model.Cell { return old }},
		// Every cell's own pair is as it was: only the neighbours in the key
		// tell that B's cell was proven beside the old A.
		{"the cells of two runs merged", func(old, now []model.Cell) []model.Cell {
			return []model.Cell{now[0], old[1]}
		}},
		// Every cell kept is fresh on its own: only running a job's placed
		// cases together keeps B from being run without A.
		{"a neighbour's cell is missing", func(_, now []model.Cell) []model.Cell {
			return slices.DeleteFunc(slices.Clone(now), func(c model.Cell) bool { return strings.HasPrefix(c.Binding, "go.b.1") })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			ports := r.w.Ports()
			r.read = packageTool(r.w, ports.Tree)
			run := func(req proof.Request) []model.Cell {
				t.Helper()
				cells, err := proof.Run(context.Background(), ports, req)
				if err != nil {
					t.Fatal(err)
				}
				return cells
			}
			old := run(r.request(model.Isolated, a, b))
			for _, c := range old {
				if c.Result != model.TruthPass {
					t.Fatalf("the old tree: %+v, want every cell pass", old)
				}
			}
			now := r.request(model.Isolated, a2, b)
			full := proof.Join(nil, run(now))

			planned := proof.Plan(now.Jobs, tc.grid(old, full), false)
			part := now
			part.Jobs = planned.Jobs
			incremental := proof.Join(planned.Kept, run(part))
			if !slices.Equal(incremental, full) {
				for i := range full {
					t.Logf("%s: plan and run = %s, forced run = %s", full[i].Binding[:6], incremental[i].Result, full[i].Result)
				}
				t.Errorf("the plan ran %s and kept %d; the grid it leaves is not the grid a forced run of the same tree leaves", stale(planned), len(planned.Kept))
			}
		})
	}
}
