package lifecycle_test

import (
	"fmt"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// TestAStatusChangeNamesACommandOnlyWhereOneMovesIt: promote moves a rule one
// step up, so a lowering gets no hint rather than a command that leaves the
// rule as it is.
func TestAStatusChangeNamesACommandOnlyWhereOneMovesIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		from, to model.Status
		want     string
	}{
		{model.Proposed, model.Trial, "gragas rule promote x"},
		{model.Proposed, model.Active, "gragas rule promote x"},
		{model.Trial, model.Active, "gragas rule promote x"},
		{model.Active, model.Trial, ""},
		{model.Active, model.Proposed, ""},
		{model.Trial, model.Proposed, ""},
		{model.Active, model.Retired, "gragas rule retire x"},
		{model.Retired, model.Active, "gragas rule restore x"},
	} {
		t.Run(fmt.Sprintf("%s to %s", c.from, c.to), func(t *testing.T) {
			t.Parallel()
			before := model.Book{Rules: []model.Rule{{ID: "x", Status: c.from}}}
			after := model.Book{Rules: []model.Rule{{ID: "x", Status: c.to}}}
			got := lifecycle.GuardEdit(before, after, func(v any) string { return fmt.Sprint(v) })
			if len(got) != 1 || got[0].Field != "status" || got[0].Command != c.want {
				t.Errorf("got %+v, want one status change with command %q", got, c.want)
			}
		})
	}
}
