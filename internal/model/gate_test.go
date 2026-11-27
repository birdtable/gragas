package model_test

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestProjectKinds: the list holds each project kind once, none of the
// rulebook gate's, and a finding of a kind not on it is not one FailOn knows.
func TestProjectKinds(t *testing.T) {
	t.Parallel()
	got := model.ProjectKinds()
	want := []model.GateKind{model.GateToolConfig, model.GateTierContract, model.GateExemption,
		model.GateNames, model.GateTrackedFiles, model.GateCounts}
	if !slices.Equal(got, want) {
		t.Fatalf("ProjectKinds = %q, want %q", got, want)
	}
	seen := map[model.GateKind]bool{}
	for _, k := range got {
		if seen[k] || slices.Contains(model.GateKinds(), k) {
			t.Errorf("kind %q is listed twice or is a kind of the rulebook's gate", k)
		}
		seen[k] = true
	}
	p := model.ProjectCheck{Findings: []model.GateFinding{{Kind: "made up"}}}
	if p.FailOn(model.ProjectKinds()) {
		t.Errorf("a finding of a kind not on the list is on FailOn's")
	}
}

// TestAToolNotCheckedEndsNotReadyAndARowWithNoToolDoesNot: the word of a
// project check.
func TestAToolNotCheckedEndsNotReadyAndARowWithNoToolDoesNot(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		rows []model.NotChecked
		want model.Word
	}{
		"none":         {nil, model.Done},
		"a tool":       {[]model.NotChecked{{Tool: "golangci-lint", Bindings: 36, Msg: "not found"}}, model.NotReady},
		"no tool":      {[]model.NotChecked{{Msg: "names: no list named"}}, model.Done},
		"both of them": {[]model.NotChecked{{Msg: "names: no list named"}, {Tool: "taskfile", Bindings: 2, Msg: "unread"}}, model.NotReady},
	} {
		if got := (model.ProjectCheck{NotChecked: c.rows}).Word(); got != c.want {
			t.Errorf("%s: Word = %q, want %q", name, got, c.want)
		}
	}
}
