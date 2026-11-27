package plugin_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestApproveScreenSaysHowTheGrantsCompareWithThePriorApproval: the dry run
// names a first approval, a prior approval that kept no grants, no change, and
// each grant added, removed or changed in place, and every grant is still
// shown below. Planted: a changed grant left unnamed, or the
// not-recorded line read as no change.
func TestApproveScreenSaysHowTheGrantsCompareWithThePriorApproval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		since model.GrantsSince
		want  []string
	}{
		{"first", model.GrantsSince{}, []string{"first approval of alpha: every grant shown"}},
		{"not recorded", model.GrantsSince{Prior: "alpha@0.9.0"}, []string{"grants not recorded: every grant shown", "alpha@0.9.0"}},
		{"unchanged", model.GrantsSince{Prior: "alpha@0.9.0", Recorded: true}, []string{"no grant changed since alpha@0.9.0"}},
		{"each kind", model.GrantsSince{Prior: "alpha@0.9.0", Recorded: true, Changes: []model.GrantChange{
			{ID: "a", Change: model.GrantAdded}, {ID: "b", Change: model.GrantRemoved}, {ID: "c", Change: model.GrantChanged}}},
			[]string{"added a", "removed b", "changed c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := shownPlugin()
			p.Since = tc.since
			s := &session{shown: p}
			run(t, s, "approve", "alpha")
			screen := fmt.Sprint(s.ending.Lead)
			for _, want := range append(tc.want, "grant alpha", "grant node") {
				if !strings.Contains(screen, want) {
					t.Errorf("the screen lacks %q: %s", want, screen)
				}
			}
			if tc.name == "not recorded" && strings.Contains(screen, "no grant changed") {
				t.Errorf("a prior approval with no grants recorded was read as no change: %s", screen)
			}
		})
	}
}
