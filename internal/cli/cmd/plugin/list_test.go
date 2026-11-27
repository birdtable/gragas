package plugin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

func (s *session) PluginList(context.Context) (model.PluginsListing, error) { return s.list, s.err }

// TestListEndsDoneAndSaysWhichAreApproved: each row is a line of the table with
// its approval, the closing sentence counts them, and under --json the document
// is the listing itself. Risk: an unapproved bundle drawn as approved.
func TestListEndsDoneAndSaysWhichAreApproved(t *testing.T) {
	t.Parallel()
	l := model.PluginsListing{Rows: []model.PluginRow{
		{Name: "alpha", Version: "1.0.0", Tool: "alpha", Digest: "sha256:aa", Approved: true},
		{Name: "beta", Version: "2.0.0", Digest: "sha256:bb", Note: "the bundle changed since it was locked"},
	}}
	s := &session{list: l}
	run(t, s, "list")
	if s.ending == nil || s.ending.Word != model.Done || !strings.Contains(s.ending.What, "2 plugins, 1 approved") {
		t.Fatalf("ending = %+v, want done with 2 plugins, 1 approved", s.ending)
	}
	tab, ok := s.ending.Lead[0].(model.Table)
	if !ok || len(tab.Rows) != 2 || tab.Rows[0][2] != "yes" || tab.Rows[1][2] != "no" {
		t.Errorf("lead = %+v, want a table of alpha yes and beta no", s.ending.Lead)
	}
	j := &session{list: l, json: true}
	run(t, j, "list")
	if got, ok := j.doc.(model.PluginsListing); !ok || len(got.Rows) != 2 {
		t.Errorf("document = %#v, want the listing", j.doc)
	}
}

// TestListOfNoPluginsSaysSoAndAnErrorEndsFailed: none locked is done with a
// note, and a lock or approvals it cannot read end failed, never done.
func TestListOfNoPluginsSaysSoAndAnErrorEndsFailed(t *testing.T) {
	t.Parallel()
	s := &session{}
	run(t, s, "list")
	if s.ending == nil || s.ending.Word != model.Done || len(s.ending.Lead) != 1 {
		t.Fatalf("ending = %+v, want done with the one note", s.ending)
	}
	f := &session{err: errors.New("the approvals could not be read")}
	run(t, f, "list")
	if f.ending == nil || f.ending.Word != model.Failed || !strings.Contains(f.ending.What, "approvals") {
		t.Errorf("ending = %+v, want failed naming the cause", f.ending)
	}
}
