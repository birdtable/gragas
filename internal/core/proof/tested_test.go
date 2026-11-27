package proof_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
)

// ran is a cell of rule at tool and version, with an adapter and a result.
func ran(rule, tool, version, adapter string, result model.Truth) model.Cell {
	c := model.Cell{Binding: rule + "#00", Tool: tool, ToolVersion: version, Adapter: adapter, Result: result, Rung: 3, Level: model.RunPair}
	if result == model.TruthNotRun {
		c.Reason = model.ReasonToolMissing
	}
	return c
}

// shown is a proposal as text, for the golden.
func shown(p proof.Proposal) string {
	var b strings.Builder
	list := func(vs []model.Version) string {
		var s []string
		for _, v := range vs {
			s = append(s, v.Name+"@"+v.Version)
		}
		return "[" + strings.Join(s, " ") + "]"
	}
	fmt.Fprintf(&b, "tools %s\nadapters %s\nplugins [] kits []\nmin engine %q\nunknown: %s\n", list(p.Set.Tools), list(p.Set.Adapters), p.Set.MinEngine, p.Unknown)
	for _, f := range p.Findings {
		fmt.Fprintf(&b, "finding: %s: %s %s@%s\n", f.Kind, f.Of, f.Name, f.Version)
	}
	return b.String()
}

// grid209 is a grid of two versions of lint, one of which ran and one whose
// tool was missing, a third whose cell is a review, and a cell of an older
// grid that names no tool.
func grid209() []model.Cell {
	return []model.Cell{
		ran("go.a.1", "lint", "1.0.0", "1.0.0", model.TruthPass),
		ran("go.b.1", "lint", "1.0.0", "1.0.0", model.TruthFail),
		ran("go.a.1", "lint", "2.0.0", "1.1.0", model.TruthPass),
		ran("go.a.1", "lint", "3.0.0", "1.1.0", model.TruthNotRun),
		{Binding: "go.c.1#00", Tool: "vet", ToolVersion: "1.0.0", Result: model.TruthReviewed, Rung: 6, Level: model.RunPair},
		{Binding: "go.d.1#00", ToolVersion: "9.0.0", Result: model.TruthPass, Rung: 3, Level: model.RunPair},
	}
}

// TestTheProposalOfTwoVersionsAndAMissingOne is the proposal's golden: a grid with two
// versions of one tool proposes both, a version whose tool was missing
// proposes nothing and is flagged, and the claimed set is compared both ways.
func TestTheProposalOfTwoVersionsAndAMissingOne(t *testing.T) {
	t.Parallel()
	claimed := &model.Tested{
		Tools:     []model.Version{{Name: "lint", Version: "1.0.0"}, {Name: "lint", Version: "3.0.0"}},
		Adapters:  []model.Version{{Name: "lint", Version: "1.0.0"}},
		MinEngine: "0.1.0",
	}
	var b strings.Builder
	b.WriteString("== against the claimed set\n" + shown(proof.ProposeTestedSet(grid209(), claimed, "0.4.0")))
	b.WriteString("== with no claimed set\n" + shown(proof.ProposeTestedSet(grid209(), nil, "0.4.0")))
	b.WriteString("== an empty grid\n" + shown(proof.ProposeTestedSet(nil, claimed, "0.4.0")))
	configtest.Golden(t, "tested.golden", []byte(b.String()))
}

// TestTheProposalIsTheSameForTheSameGrid is the proposal's determinism: the same
// cells in any order are the same proposal, to the byte.
func TestTheProposalIsTheSameForTheSameGrid(t *testing.T) {
	t.Parallel()
	claimed := &model.Tested{Tools: []model.Version{{Name: "lint", Version: "3.0.0"}, {Name: "lint", Version: "1.0.0"}}}
	want := proof.ProposeTestedSet(grid209(), claimed, "0.4.0")
	r := &draw{state: 209}
	for range 200 {
		cells := grid209()
		r.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
		got := proof.ProposeTestedSet(cells, claimed, "0.4.0")
		if !reflect.DeepEqual(got, want) || shown(got) != shown(want) {
			t.Fatalf("another order proposed\n%s\nwant\n%s", shown(got), shown(want))
		}
	}
}

// TestTheClaimedSetIsNeverChanged: the identity's set is release's to change
// (M5-10); proposing from it reads it and leaves it as it was.
func TestTheClaimedSetIsNeverChanged(t *testing.T) {
	t.Parallel()
	claimed := &model.Tested{Tools: []model.Version{{Name: "lint", Version: "3.0.0"}}, MinEngine: "0.1.0"}
	before := *claimed
	before.Tools = slices.Clone(claimed.Tools)
	proof.ProposeTestedSet(grid209(), claimed, "0.4.0")
	if !reflect.DeepEqual(*claimed, before) {
		t.Errorf("the claimed set became %+v", *claimed)
	}
}
