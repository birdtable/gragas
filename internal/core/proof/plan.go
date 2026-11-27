package proof

import (
	"cmp"
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// Planned is what a proof run must do and what it keeps: the jobs, each with
// only the cases whose cell is stale, in the order given, and the cells kept
// from the grid as they are.
type Planned struct {
	// Jobs are the jobs that still have a case to run, one tool version a
	// job, so Run batches by tool version as it is handed them.
	Jobs []Job
	// Kept are the grid's cells that are fresh, in the order of the cases.
	Kept []model.Cell
}

// Plan is the run that brings grid up to date with jobs: every case whose cell
// is stale is run again, and every other case keeps its cell. A cell is
// fresh only when the grid holds one for the case's binding and tool version
// whose Inputs are the case's Inputs now, and whose result is pass, fail or
// reviewed. Any other cell is stale:
//   - one with no Inputs, as every cell written before they were;
//   - one whose pair, binding, tool, versions or config set has changed;
//   - one not run, since tool missing and limit hit say what this machine had
//     at that run, not what the inputs are, and a no-pair cell costs no run;
//   - one whose examples are placed, when any other placed case of its job is
//     stale. The placed cases of a job run in shared folders, so they are run
//     together or not at all: a case run without the others would be run in a
//     folder no full run makes, and a cell kept beside them would keep a
//     result proven beside other neighbours. A change to one pair therefore
//     stales only the cells of its job, but all of its placed ones.
//
// With force, every case is stale. A cell of the grid that matches no case
// now (a binding since changed, or a tool version no longer asked) is neither
// kept nor run, so the grid the caller writes drops it.
//
// The jobs answered hold their cases in the order given, so every placed case
// Run is handed stands where it stood in the job, and the cells it writes
// carry the Inputs a later Plan computes.
//
// Plan is pure: it reads only its arguments and runs nothing.
func Plan(jobs []Job, grid []model.Cell, force bool) Planned {
	byKey := make(map[model.CellKey]model.Cell, len(grid))
	for _, c := range grid {
		byKey[c.Key()] = c
	}
	var out Planned
	for _, j := range jobs {
		cells := make([]model.Cell, len(j.Cases))
		ok := make([]bool, len(j.Cases))
		folder := force
		for i, c := range j.Cases {
			cells[i], ok[i] = fresh(byKey, j, c)
			ok[i] = ok[i] && !force
			folder = folder || (places(c) && !ok[i])
		}
		stale := j
		stale.Cases = nil
		for i, c := range j.Cases {
			if ok[i] && (!folder || !places(c)) {
				out.Kept = append(out.Kept, cells[i])
				continue
			}
			stale.Cases = append(stale.Cases, c)
		}
		if len(stale.Cases) > 0 {
			out.Jobs = append(out.Jobs, stale)
		}
	}
	return out
}

// fresh is the grid's cell for c under j, and whether it may be kept.
func fresh(byKey map[model.CellKey]model.Cell, j Job, c Case) (model.Cell, bool) {
	key := model.CellKey{Binding: BindingText(c.Binding.Ref(c.Rule)), ToolVersion: j.Version}
	cell, ok := byKey[key]
	if !ok || cell.Inputs == "" || cell.Inputs != Inputs(j, c) {
		return model.Cell{}, false
	}
	switch cell.Result {
	case model.TruthPass, model.TruthFail, model.TruthReviewed:
		return cell, true
	case model.TruthNotRun:
		return model.Cell{}, false // what this machine had then, not what the inputs are
	}
	return model.Cell{}, false
}

// Join is the grid a run leaves: the kept cells and the cells run, one for
// each binding and tool version, in the order of their binding text and then
// their tool version, so the same cells are always the same grid whatever
// order they were planned or run in. A run cell replaces a kept one of its
// key, which a plan never makes.
func Join(kept, ran []model.Cell) []model.Cell {
	byKey := map[model.CellKey]model.Cell{}
	for _, c := range kept {
		byKey[c.Key()] = c
	}
	for _, c := range ran {
		byKey[c.Key()] = c
	}
	out := make([]model.Cell, 0, len(byKey))
	for _, c := range byKey {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b model.Cell) int {
		return cmp.Or(cmp.Compare(a.Binding, b.Binding), cmp.Compare(a.ToolVersion, b.ToolVersion))
	})
	return out
}
