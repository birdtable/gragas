package coreapi_test

import (
	"context"
	"reflect"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// TestShowPluginSaysWhichGrantsChangedSinceThePriorApproval: the screen's
// grants are compared with the newest approval of the plugin by name, by id and
// digest: added, removed, changed in place (the id stays, the digest differs),
// nothing changed, a first approval, and a prior approval that kept no grants
// by id. Planted: ids alone compared (the changed row is then
// empty), or a first approval read as every grant removed.
func TestShowPluginSaysWhichGrantsChangedSinceThePriorApproval(t *testing.T) {
	t.Parallel()
	w0, b0 := approveWorld(t)
	base, err := coreapi.ShowPlugin(context.Background(), b0, w0.Ports().Trust, "r", "alpha")
	if err != nil || len(base.Held) != 1 {
		t.Fatalf("ShowPlugin = %+v, %v; want one grant held", base, err)
	}
	mine := base.Held[0]
	other := model.GrantHeld{ID: "standin", Digest: base.GrantsDigest}
	gone := model.GrantHeld{ID: "zzz", Digest: mine.Digest}
	for _, tc := range []struct {
		name  string
		prior *model.Approval
		want  model.GrantsSince
	}{
		{"a first approval", nil, model.GrantsSince{Changes: []model.GrantChange{}}},
		{"nothing changed", &model.Approval{Held: []model.GrantHeld{mine}},
			model.GrantsSince{Prior: "alpha@0.9.0", Recorded: true, Changes: []model.GrantChange{}}},
		{"a grant added", &model.Approval{Held: []model.GrantHeld{}},
			model.GrantsSince{Prior: "alpha@0.9.0", Recorded: true, Changes: []model.GrantChange{{ID: "standin", Change: model.GrantAdded}}}},
		{"a grant removed", &model.Approval{Held: []model.GrantHeld{mine, gone}},
			model.GrantsSince{Prior: "alpha@0.9.0", Recorded: true, Changes: []model.GrantChange{{ID: "zzz", Change: model.GrantRemoved}}}},
		{"a grant changed in place", &model.Approval{Held: []model.GrantHeld{other}},
			model.GrantsSince{Prior: "alpha@0.9.0", Recorded: true, Changes: []model.GrantChange{{ID: "standin", Change: model.GrantChanged}}}},
		{"a prior approval that kept no grants by id", &model.Approval{},
			model.GrantsSince{Prior: "alpha@0.9.0", Changes: []model.GrantChange{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, b := approveWorld(t)
			if tc.prior != nil {
				a := *tc.prior
				a.Digest, a.Ref, a.Grants = model.Digest("sha256:"+zeros()), "alpha@0.9.0", base.GrantsDigest
				w.SeedApproval(a)
			}
			got, err := coreapi.ShowPlugin(context.Background(), b, w.Ports().Trust, "r", "alpha")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Since, tc.want) {
				t.Errorf("Since = %+v, want %+v", got.Since, tc.want)
			}
		})
	}
}

func zeros() string {
	s := make([]byte, 64)
	for i := range s {
		s[i] = '0'
	}
	return string(s)
}
