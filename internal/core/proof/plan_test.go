package proof_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
)

// TestASecondPlanOfAnUnchangedTreeRunsNoTool is the plan's promise at the
// proof's own seam (the journey is the prove command's): the cells a
// run wrote, planned again with nothing changed, keep every cell and leave
// nothing to run, and running that plan asks the tool nothing.
func TestASecondPlanOfAnUnchangedTreeRunsNoTool(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.say(1, "")
	r.say(2, "no-x src/a.go\n")
	req := r.request(model.Isolated, theCase(3, "src/a.go"), theCase(6, "src/b.go"))
	first, err := proof.Run(context.Background(), r.w.Ports(), req)
	if err != nil || len(first) != 2 || first[0].Result != model.TruthPass {
		t.Fatalf("first run = %+v, %v", first, err)
	}
	jobs := req.Jobs
	planned := proof.Plan(jobs, first, false)
	if len(planned.Jobs) != 0 || !slices.Equal(planned.Kept, first) {
		t.Fatalf("second plan = %+v, want nothing to run and both cells kept", planned)
	}
	before := len(r.calls)
	req.Jobs = planned.Jobs
	second, err := proof.Run(context.Background(), r.w.Ports(), req)
	if err != nil || len(second) != 0 || len(r.calls) != before {
		t.Fatalf("second run = %+v, %v, after %d reads of the tool; want none", second, err, len(r.calls)-before)
	}
	if got := proof.Join(planned.Kept, second); !slices.Equal(got, proof.Join(first, nil)) {
		t.Errorf("the grid after the second run = %+v, want the first's", got)
	}
	if forced := proof.Plan(jobs, first, true); len(forced.Kept) != 0 || len(forced.Jobs) != 1 || len(forced.Jobs[0].Cases) != 2 {
		t.Errorf("a forced plan = %+v, want both cases run and nothing kept", forced)
	}
}

// world is a plan's inputs: three bindings with a pair each and a fourth at
// the reviewed rung, under two versions of one tool, with one config file.
func world() []proof.Job {
	var cases []proof.Case
	for i, rule := range []model.RuleID{"go.a.1", "go.b.1", "go.c.1", "go.d.1"} {
		rung := model.Rung(3)
		if rule == "go.d.1" {
			rung = model.MaxRung
		}
		cases = append(cases, proof.Case{Rule: rule,
			Binding: model.Binding{Tool: "lint", Target: "code", Key: "k" + string(rune('a'+i)), Rung: rung, Pair: 1, PairPath: "src/x.go"},
			Pair:    proof.Pair{Digest: model.Digest("sha256:" + strings.Repeat(string(rune('a'+i)), 64))}})
	}
	var jobs []proof.Job
	for _, v := range []string{"1.0.0", "2.0.0"} {
		jobs = append(jobs, proof.Job{Tool: "lint", Version: v, Adapter: "1.0.0", ConfigPath: "lint.yaml",
			Config: []model.File{{Path: "lint.yaml", Data: []byte("x: 1\n")}}, Cases: slices.Clone(cases)})
	}
	return jobs
}

// proven is the grid a run of jobs would write, every cell a pass on the
// inputs it was run on, or a review at the reviewed rung.
func proven(jobs []proof.Job) []model.Cell {
	var cells []model.Cell
	for _, j := range jobs {
		for _, c := range j.Cases {
			result := model.TruthPass
			if c.Binding.Rung == model.MaxRung {
				result = model.TruthReviewed
			}
			cells = append(cells, model.Cell{Binding: proof.BindingText(c.Binding.Ref(c.Rule)), ToolVersion: j.Version,
				Tool: j.Tool, Result: result, Rung: int(c.Binding.Rung), Level: model.RunPair,
				Adapter: j.Adapter, Inputs: proof.Inputs(j, c)})
		}
	}
	return cells
}

// places is whether a case's examples are placed in a folder and run, as Run
// places them: below the reviewed rung, with a pair path.
func places(c proof.Case) bool {
	return c.Binding.Rung != model.MaxRung && c.Binding.PairPath != ""
}

// stale names what a plan runs, rule@version, in the order planned.
func stale(p proof.Planned) string {
	var out []string
	for _, j := range p.Jobs {
		for _, c := range j.Cases {
			out = append(out, string(c.Rule)+"@"+j.Version)
		}
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, " ")
}

// TestWhichCellsAreStaleAfterEachKindOfChange is the plan's golden: the world is
// proven, then changed one way, and the plan names the cells it runs again.
// Each row is also a plant: an input left out of the key would keep a cell
// fresh here and the golden would differ.
func TestWhichCellsAreStaleAfterEachKindOfChange(t *testing.T) {
	t.Parallel()
	type change struct {
		name string
		jobs func([]proof.Job) []proof.Job
		grid func([]model.Cell) []model.Cell
	}
	same := func(js []proof.Job) []proof.Job { return js }
	kept := func(cs []model.Cell) []model.Cell { return cs }
	each := func(js []proof.Job, f func(*proof.Job)) []proof.Job {
		for i := range js {
			f(&js[i])
		}
		return js
	}
	changes := []change{
		{"nothing", same, kept},
		{"one pair's examples", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) { j.Cases[1].Pair.Digest = model.Digest("sha256:" + strings.Repeat("f", 64)) })
		}, kept},
		{"one binding's key", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) { j.Cases[2].Binding.Key = "kz" })
		}, kept},
		{"the review's pair", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) { j.Cases[3].Pair.Digest = model.Digest("sha256:" + strings.Repeat("f", 64)) })
		}, kept},
		{"a binding added", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) {
				j.Cases = append(j.Cases, proof.Case{Rule: "go.e.1",
					Binding: model.Binding{Tool: "lint", Target: "code", Key: "ke", Rung: 3, Pair: 1, PairPath: "src/z.go"},
					Pair:    proof.Pair{Digest: model.Digest("sha256:" + strings.Repeat("e", 64))}})
			})
		}, kept},
		{"a binding taken out", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) { j.Cases = slices.Delete(j.Cases, 1, 2) })
		}, kept},
		{"the bindings in another order", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) { j.Cases[0], j.Cases[1] = j.Cases[1], j.Cases[0] })
		}, kept},
		{"a neighbour's cell missing", same, func(cs []model.Cell) []model.Cell { return slices.Delete(cs, 1, 2) }},
		{"the review's cell missing", same, func(cs []model.Cell) []model.Cell { return slices.Delete(cs, 3, 4) }},
		{"the config's bytes", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) { j.Config = []model.File{{Path: "lint.yaml", Data: []byte("x: 2\n")}} })
		}, kept},
		{"the config path", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) {
				j.Config = append(j.Config, model.File{Path: "other.yaml"})
				j.ConfigPath = "other.yaml"
			})
		}, kept},
		{"the adapter of version 2.0.0", func(js []proof.Job) []proof.Job { js[1].Adapter = "1.1.0"; return js }, kept},
		{"the tool's name", func(js []proof.Job) []proof.Job {
			return each(js, func(j *proof.Job) { j.Tool = "lint2" })
		}, kept},
		{"a third tool version asked", func(js []proof.Job) []proof.Job {
			extra := js[1]
			extra.Version = "3.0.0"
			extra.Cases = slices.Clone(extra.Cases)
			return append(js, extra)
		}, kept},
		{"version 1.0.0 no longer asked", func(js []proof.Job) []proof.Job { return js[1:] }, kept},
		{"a cell with no inputs", same, func(cs []model.Cell) []model.Cell { cs[0].Inputs = ""; return cs }},
		{"a cell whose inputs are another's", same, func(cs []model.Cell) []model.Cell { cs[0].Inputs = cs[1].Inputs; return cs }},
		{"a cell failed on its inputs", same, func(cs []model.Cell) []model.Cell { cs[0].Result = model.TruthFail; return cs }},
		{"a cell not run, tool missing", same, func(cs []model.Cell) []model.Cell {
			cs[0].Result, cs[0].Reason = model.TruthNotRun, model.ReasonToolMissing
			return cs
		}},
		{"a cell not run, limit hit", same, func(cs []model.Cell) []model.Cell {
			cs[5].Result, cs[5].Reason = model.TruthNotRun, model.ReasonLimitHit
			return cs
		}},
		{"no grid", same, func([]model.Cell) []model.Cell { return nil }},
	}
	var b strings.Builder
	for _, ch := range changes {
		grid := ch.grid(proven(world()))
		p := proof.Plan(ch.jobs(world()), grid, false)
		fmt.Fprintf(&b, "%-36s runs %s; keeps %d\n", ch.name+":", stale(p), len(p.Kept))
	}
	configtest.Golden(t, "stale.golden", []byte(b.String()))
}

// TestReplanningIsEmptyAndOneChangeStalesOnlyItsJobsCells is the property
// (tests.generated.2): over generated worlds, a plan of the grid its own run
// would write runs nothing; changing the pair of one binding stales the cells
// of its job and no other: every placed case of each job when the binding is
// placed, its own cell alone when it is a review; and the grid the run of
// that plan writes plans to nothing again.
func TestReplanningIsEmptyAndOneChangeStalesOnlyItsJobsCells(t *testing.T) {
	t.Parallel()
	r := &draw{state: 208}
	for draw := range 500 {
		jobs := genJobs(r)
		grid := proven(jobs)
		if p := proof.Plan(jobs, grid, false); len(p.Jobs) != 0 || len(p.Kept) != len(grid) {
			t.Fatalf("draw %d: an unchanged grid planned %s and kept %d of %d", draw, stale(p), len(p.Kept), len(grid))
		}
		if len(jobs[0].Cases) == 0 {
			continue
		}
		which := r.IntN(len(jobs[0].Cases))
		for i := range jobs {
			jobs[i].Cases[which].Pair.Digest += "0"
		}
		p := proof.Plan(jobs, grid, false)
		want := []string{}
		for _, j := range jobs {
			for i, c := range j.Cases {
				if i == which || (places(j.Cases[which]) && places(c)) {
					want = append(want, string(c.Rule)+"@"+j.Version)
				}
			}
		}
		if got := stale(p); got != strings.Join(want, " ") {
			t.Fatalf("draw %d: changing case %d's pair staled %s, want %s", draw, which, got, strings.Join(want, " "))
		}
		// The cells a run of the plan writes carry the Inputs of the stale
		// job it was handed; they must be the Inputs of the whole job.
		after := proof.Join(p.Kept, proven(p.Jobs))
		if again := proof.Plan(jobs, after, false); len(again.Jobs) != 0 {
			t.Fatalf("draw %d: the grid the plan's run writes planned %s again", draw, stale(again))
		}
	}
}

// genJobs is up to four tool versions of one tool over up to five bindings,
// each with its own rule and pair, some at the reviewed rung, and a config set
// of up to two files.
func genJobs(r *draw) []proof.Job {
	var cases []proof.Case
	for i := range r.IntN(6) {
		cases = append(cases, proof.Case{Rule: model.RuleID(fmt.Sprintf("go.r.%d", i+1)),
			Binding: model.Binding{Tool: "lint", Key: fmt.Sprintf("k%d", r.IntN(3)), Rung: model.Rung(r.IntN(int(model.MaxRung) + 1)), Pair: 1, PairPath: "x.go"},
			Pair:    proof.Pair{Digest: model.Digest(fmt.Sprintf("sha256:%064d", r.IntN(1000)))}})
	}
	var config []model.File
	for i := range r.IntN(3) {
		config = append(config, model.File{Path: fmt.Sprintf("c%d.yaml", i), Data: []byte{byte(r.IntN(256))}})
	}
	var jobs []proof.Job
	for v := range 1 + r.IntN(4) {
		jobs = append(jobs, proof.Job{Tool: "lint", Version: fmt.Sprintf("%d.0.0", v+1), Adapter: "1.0.0",
			Config: config, Cases: slices.Clone(cases)})
	}
	return jobs
}

// TestJoinIsOneCellAKeyInOneOrder: the grid a run leaves is the same grid
// whatever order the kept and run cells come in.
func TestJoinIsOneCellAKeyInOneOrder(t *testing.T) {
	t.Parallel()
	cells := proven(world())
	want := proof.Join(cells, nil)
	r := &draw{state: 12}
	for range 50 {
		shuffled := slices.Clone(cells)
		r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		cut := r.IntN(len(shuffled) + 1)
		if got := proof.Join(shuffled[:cut], shuffled[cut:]); !slices.Equal(got, want) {
			t.Fatalf("Join in another order = %+v, want %+v", got, want)
		}
	}
	if err := proof.Validate(want); err != nil {
		t.Errorf("the joined grid breaks the grid's invariant: %v", err)
	}
}
