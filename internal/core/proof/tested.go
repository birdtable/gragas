package proof

import (
	"cmp"
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// TestedKind is what a finding of the proposed tested set says.
type TestedKind string

// The findings of a proposal. None refuses: each is listed for the person who
// says yes at release.
const (
	// NoColumn is a version of the claimed set that no cell of the grid ran.
	NoColumn TestedKind = "a tested-set version with no proof column"
	// Outside is a version a cell ran that the claimed set does not hold.
	Outside TestedKind = "a proof column for a version outside the tested set"
	// NotRun is a tool version the grid has a cell not run for and no cell that ran
	// the tool at (tool missing, limit hit), so nothing is proposed for it. A
	// version whose cells are all reviews was never asked to run and is not one.
	NotRun TestedKind = "a tool version whose cells were not run"
	// NoTool is a cell that names no tool, as a grid written before cells
	// named theirs, so its version is proposed for none.
	NoTool TestedKind = "a proof column that names no tool"
)

// TestedFinding is one finding of a proposal: what it says, whether it is of
// a tool or an adapter, and which.
type TestedFinding struct {
	Kind TestedKind
	// Of is "tool" or "adapter".
	Of            string
	Name, Version string
}

// NotProposed says why plugins and kits are left out of every proposal: no
// run records their versions yet. A value the program cannot establish is
// left out with its reason, never guessed (cli.output.9).
const NotProposed = "plugins and kits: no proof run records their versions yet"

// Proposal is the tested set a grid proposes and what comparing it with the
// claimed set found.
type Proposal struct {
	Set      model.Tested
	Findings []TestedFinding
	// Unknown is why part of the set is not proposed: NotProposed.
	Unknown string
}

// ProposeTestedSet is the tested set the cells of a grid were proven on:
// each tool version at least one of its cells ran the tool at (pass
// or fail; a review and a cell not run run none), and the adapter version
// beside it, and engine as the oldest engine that reads it, the one that ran
// the proof. It is compared with claimed, the identity's set (nil for none):
// a claimed version no cell ran, and a run version the claim does not hold,
// are findings; with no claimed set there is nothing to compare, and none
// is. The identity is never written here: its set changes only by
// release's yes (M5-10).
//
// The same cells and claim are always the same proposal: names and versions
// are in text order, findings in the order of their kind's list above, then
// of, name and version.
func ProposeTestedSet(cells []model.Cell, claimed *model.Tested, engine string) Proposal {
	tools, adapters := map[model.Version]bool{}, map[model.Version]bool{}
	asked := map[model.Version]bool{}
	var out Proposal
	for _, c := range cells {
		if c.Tool == "" {
			out.Findings = append(out.Findings, TestedFinding{Kind: NoTool, Of: "tool", Version: c.ToolVersion})
			continue
		}
		v := model.Version{Name: c.Tool, Version: c.ToolVersion}
		if c.Result == model.TruthNotRun {
			asked[v] = true
		}
		if c.Result != model.TruthPass && c.Result != model.TruthFail {
			continue // a review runs no tool, and a cell not run ran none
		}
		tools[v] = true
		if c.Adapter != "" {
			adapters[model.Version{Name: c.Tool, Version: c.Adapter}] = true
		}
	}
	for v := range asked {
		if !tools[v] {
			out.Findings = append(out.Findings, TestedFinding{Kind: NotRun, Of: "tool", Name: v.Name, Version: v.Version})
		}
	}
	out.Set = model.Tested{Tools: sorted(tools), Adapters: sorted(adapters), MinEngine: engine}
	if claimed != nil {
		out.Findings = append(out.Findings, compare("tool", claimed.Tools, tools)...)
		out.Findings = append(out.Findings, compare("adapter", claimed.Adapters, adapters)...)
	}
	out.Unknown = NotProposed
	order := []TestedKind{NoColumn, Outside, NotRun, NoTool}
	slices.SortFunc(out.Findings, func(a, b TestedFinding) int {
		return cmp.Or(cmp.Compare(slices.Index(order, a.Kind), slices.Index(order, b.Kind)),
			cmp.Compare(a.Of, b.Of), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Version, b.Version))
	})
	out.Findings = slices.Compact(out.Findings)
	return out
}

// compare is the findings of one kind of version: each claimed one not run,
// and each run one not claimed.
func compare(of string, claimed []model.Version, ran map[model.Version]bool) []TestedFinding {
	var out []TestedFinding
	held := map[model.Version]bool{}
	for _, v := range claimed {
		held[v] = true
		if !ran[v] {
			out = append(out, TestedFinding{Kind: NoColumn, Of: of, Name: v.Name, Version: v.Version})
		}
	}
	for v := range ran {
		if !held[v] {
			out = append(out, TestedFinding{Kind: Outside, Of: of, Name: v.Name, Version: v.Version})
		}
	}
	return out
}

// sorted is the versions of set, by name and then version text.
func sorted(set map[model.Version]bool) []model.Version {
	out := make([]model.Version, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b model.Version) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Version, b.Version))
	})
	return out
}
