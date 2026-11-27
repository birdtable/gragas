package plugin_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

const dig = model.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

func (s *session) PluginApproval(context.Context, string) (model.PluginApproval, error) {
	return s.shown, s.err
}

func (s *session) PluginApprove(_ context.Context, _ string, d model.Digest) (model.PluginApproval, error) {
	s.given = append(s.given, d)
	got := s.shown
	got.Approved, got.Recorded = s.err == nil, s.err == nil
	return got, s.err
}

func shownPlugin() model.PluginApproval {
	return model.PluginApproval{
		Name: "alpha", Version: "1.0.0", Digest: dig, GrantsDigest: dig, Tool: "alpha", ToolVersions: ">=1.0.0 <2.0.0",
		Grants: []model.PluginGrant{
			{ID: "alpha", Where: "path", Path: "bin/alpha", Versions: ">=1", VersionArgv: []string{"--version"}, WholeLine: true},
			{ID: "node", Where: "script", Manifest: "package.json", ScriptAt: "/scripts/alpha", Interpreter: "node", Versions: ">=20"},
		},
	}
}

// TestApproveWithoutApplyShowsTheGrantsAndRecordsNothing: the dry run draws
// the digest and each grant, says a grant with no script hands the plugin the
// whole command line, never calls the record, and ends on the line to type
// with the digest in it. Risk: an approval on the screen alone.
func TestApproveWithoutApplyShowsTheGrantsAndRecordsNothing(t *testing.T) {
	t.Parallel()
	s := &session{shown: shownPlugin()}
	run(t, s, "approve", "alpha")
	if len(s.given) != 0 || s.ending == nil || s.ending.Word != model.Done {
		t.Fatalf("ending %+v, recorded %v; want done and nothing recorded", s.ending, s.given)
	}
	if want := fmt.Sprintf("gragas plugin approve alpha@1.0.0 --digest %s --apply", dig); s.ending.Next != want {
		t.Errorf("next = %q, want %q", s.ending.Next, want)
	}
	screen := fmt.Sprint(s.ending.Lead)
	for _, want := range []string{string(dig), "pinned places and bin/alpha", "script /scripts/alpha in package.json, run by node", "the plugin hands it the whole command line", "the script first"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the screen lacks %q: %s", want, screen)
		}
	}
}

// TestApproveShowsAGrantsCacheVars: a grant's cache_vars are
// drawn in its block before the yes, with what they mean, and a grant with
// none draws no such row. Planted failure: the row left out. Risk: a person
// approves a cache that steps share without seeing it.
func TestApproveShowsAGrantsCacheVars(t *testing.T) {
	t.Parallel()
	a := shownPlugin()
	a.Grants[0].CacheVars = []string{"GOCACHE", "GOMODCACHE"}
	s := &session{shown: a}
	run(t, s, "approve", "alpha")
	screen := fmt.Sprint(s.ending.Lead)
	if want := "cache vars    GOCACHE GOMODCACHE (each a folder gragas keeps for this grant and project, which its later steps read)"; !strings.Contains(screen, want) {
		t.Errorf("the screen lacks %q: %s", want, screen)
	}
	if n := strings.Count(screen, "cache vars"); n != 1 {
		t.Errorf("the screen draws %d cache vars rows, want 1, for the one grant that has them: %s", n, screen)
	}
}

// TestApproveWithApplyRecordsTheDigestGiven: --apply hands the digest given to
// the record, and a digest that is not the bundle's is refused with nothing
// recorded.
func TestApproveWithApplyRecordsTheDigestGiven(t *testing.T) {
	t.Parallel()
	s := &session{shown: shownPlugin()}
	run(t, s, "approve", "alpha", "--digest", string(dig), "--apply")
	if len(s.given) != 1 || s.given[0] != dig || s.ending.Word != model.Done || !strings.Contains(s.ending.What, "approved") {
		t.Errorf("given %v, ending %+v; want the digest recorded and done", s.given, s.ending)
	}
	r := &session{shown: shownPlugin(), err: fmt.Errorf("%w: it is x", coreapi.ErrDigestGiven)}
	run(t, r, "approve", "alpha", "--apply")
	if r.ending.Word != model.Refused {
		t.Errorf("ending = %+v, want refused for a digest not given", r.ending)
	}
}

// TestApproveEndsRefusedOrFailed: a plugin not meant, a digest not given,
// are refused; a record that cannot be read is failed.
func TestApproveEndsRefusedOrFailed(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]model.Word{
		coreapi.ErrNoSuchPlugin: model.Refused, coreapi.ErrManyVersions: model.Refused,
		coreapi.ErrDigestGiven: model.Refused, errors.New("the record cannot be read"): model.Failed,
	} {
		s := &session{err: err}
		run(t, s, "approve", "alpha")
		if s.ending == nil || s.ending.Word != want {
			t.Errorf("%v: ending %+v, want %s", err, s.ending, want)
		}
	}
}

// TestApproveAlreadyApprovedSaysSoAndOffersNoLine: nothing to record.
func TestApproveAlreadyApprovedSaysSoAndOffersNoLine(t *testing.T) {
	t.Parallel()
	a := shownPlugin()
	a.Approved = true
	s := &session{shown: a}
	run(t, s, "approve", "alpha")
	if s.ending.Next != "" || !strings.Contains(s.ending.What, "already approved") {
		t.Errorf("ending = %+v, want already approved and no line", s.ending)
	}
}

// TestApproveUnderJSONIsOneDocument: the shown approval is the document.
func TestApproveUnderJSONIsOneDocument(t *testing.T) {
	t.Parallel()
	s := &session{shown: shownPlugin(), json: true}
	run(t, s, "approve", "alpha")
	if got, ok := s.doc.(model.PluginApproval); !ok || got.Digest != dig {
		t.Errorf("document = %#v, want the approval shown", s.doc)
	}
}
