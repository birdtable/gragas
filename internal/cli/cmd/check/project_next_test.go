package check_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/check"
	"github.com/birdtable/gragas/internal/model"
)

func rowOf(c model.GateCause) model.NotChecked {
	return model.NotChecked{Tool: "examplelint", Bindings: 3, Cause: c, Msg: "run gragas tools: git could not be opened"}
}

func endOf(t *testing.T, rows ...model.NotChecked) (next, what string) {
	t.Helper()
	s := inProject(model.ProjectCheck{NotChecked: rows})
	runCheck(t, s)
	return s.ended.Next, s.ended.What
}

// TestTheNextLineOfANotReadyEndingFollowsItsCause: the table has a row for
// every cause the model lists, and each ending says its own row's line. The
// sentence of every row here says gragas tools, so a Next read from it, and
// the old constant "gragas tools" for every ending, are both red for each
// cause but the tool's.
func TestTheNextLineOfANotReadyEndingFollowsItsCause(t *testing.T) {
	t.Parallel()
	table := check.NextTable()
	var have []model.GateCause
	for _, r := range table {
		have = append(have, r.Cause)
	}
	if !slices.Equal(slices.Sorted(slices.Values(have)), slices.Sorted(slices.Values(model.GateCauses()))) {
		t.Errorf("the table holds %v, the model lists %v", have, model.GateCauses())
	}
	for _, r := range table {
		next, what := endOf(t, rowOf(r.Cause))
		if next != r.Next || strings.Contains(what, "first the") {
			t.Errorf("%s: next %q, said %q; want %q and no order sentence", r.Cause, next, what, r.Next)
		}
		if (next == "gragas tools") != (r.Cause == model.CauseTool) {
			t.Errorf("%s: next is %q", r.Cause, next)
		}
	}
}

// TestManyCausesSayWhichNextComesFirst: the order is the table's whatever the
// order of the rows, and the ending says which cause first and why.
func TestManyCausesSayWhichNextComesFirst(t *testing.T) {
	t.Parallel()
	first := check.NextTable()[0]
	next, what := endOf(t, rowOf(model.CauseTool), rowOf(model.CauseBase), rowOf(model.CauseHistory))
	if next != first.Next {
		t.Errorf("next %q, want the first cause's %q", next, first.Next)
	}
	for _, w := range []string{"first the " + string(first.Cause) + " cause", "base and tool", first.Why} {
		if !strings.Contains(what, w) {
			t.Errorf("said %q, lacks %q", what, w)
		}
	}
}
