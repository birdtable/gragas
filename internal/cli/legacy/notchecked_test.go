package legacy

import (
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// TestAPartialRowEndsNothingAndAToolRowEndsNotReady: the old check ends as the new one does. A tool no plugin could judge
// ends it not ready with Next `gragas tools`; a row that says one part of a
// tool was not read (Partial) names no tool left unjudged, so a run with only
// such rows ends no differently. Risk: the old check stricter than the new,
// or a tool row left to a warning.
func TestAPartialRowEndsNothingAndAToolRowEndsNotReady(t *testing.T) {
	t.Parallel()
	partial := coreapi.ConfigNotChecked{Tool: "task", Why: "one tier was not read", Partial: true}
	tool := coreapi.ConfigNotChecked{Tool: "lint-a", Why: "no plugin is installed for lint-a"}

	only := &commands{cf: coreapi.Conformance{NotChecked: []coreapi.ConfigNotChecked{partial}}}
	if e := only.notCheckedEnding(); e != nil {
		t.Errorf("a Partial-only run ends %+v, want no ending, as the new check", e)
	}

	both := &commands{cf: coreapi.Conformance{NotChecked: []coreapi.ConfigNotChecked{partial, tool}}}
	e := both.notCheckedEnding()
	if e == nil || e.Word != model.NotReady || e.Next != "gragas tools" || !strings.Contains(e.What, "lint-a") || strings.Contains(e.What, "one tier") {
		t.Errorf("a tool row with a Partial row ends %+v, want not ready naming lint-a alone, Next gragas tools", e)
	}
}
