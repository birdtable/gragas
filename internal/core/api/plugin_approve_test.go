package coreapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// approveWorld is a world and a bundles fake holding zed 1.0.0 and zed 2.0.0
// and alpha 1.0.0.
func approveWorld(t *testing.T) (*porttest.World, *porttest.Bundles) {
	t.Helper()
	w := porttest.NewWorld(t)
	var b porttest.Bundles
	for _, r := range []model.PluginRef{{Name: "zed", Version: "1.0.0"}, {Name: "zed", Version: "2.0.0"}, {Name: "alpha", Version: "1.0.0"}} {
		b.Put("r", r, listManifest(r.Name, r.Version), listWasm())
	}
	return w, &b
}

// TestShowPluginReadsTheGrantsFromTheManifestAlone: the screen's facts are the
// manifest's: the tool and its range, each grant with where its program is
// found, and that a grant with no script hands the plugin the whole command
// line. Planted: a grant of a path read as a script, or the digest not the
// bundle's.
func TestShowPluginReadsTheGrantsFromTheManifestAlone(t *testing.T) {
	t.Parallel()
	w, b := approveWorld(t)
	got, err := coreapi.ShowPlugin(context.Background(), b, w.Ports().Trust, "r", "alpha")
	if err != nil {
		t.Fatalf("ShowPlugin = %v", err)
	}
	if got.Name != "alpha" || got.Version != "1.0.0" || got.Tool != "standin" || got.ToolVersions != ">=1.0.0 <2.0.0" || !got.Digest.Formed() || !got.GrantsDigest.Formed() || got.Approved {
		t.Errorf("ShowPlugin = %+v, want alpha 1.0.0 of standin, digests formed, not approved", got)
	}
	if len(got.Grants) != 1 {
		t.Fatalf("grants = %+v, want one", got.Grants)
	}
	if g := got.Grants[0]; g.ID != "standin" || g.Where != "path" || g.Path != "bin/standin" || !g.WholeLine || g.Versions != ">=1.0.0 <2.0.0" || len(g.VersionArgv) != 1 {
		t.Errorf("grant = %+v, want the path grant standin at bin/standin with the whole line", g)
	}
}

// TestShowPluginNeedsTheVersionWhenThereAreTwo: a name the lock holds twice is
// refused naming both, name@version picks one, and a name the lock does not
// hold is its own refusal.
func TestShowPluginNeedsTheVersionWhenThereAreTwo(t *testing.T) {
	t.Parallel()
	w, b := approveWorld(t)
	ctx := context.Background()
	if _, err := coreapi.ShowPlugin(ctx, b, w.Ports().Trust, "r", "zed"); !errors.Is(err, coreapi.ErrManyVersions) || !strings.Contains(err.Error(), "zed@1.0.0, zed@2.0.0") {
		t.Errorf("ShowPlugin(zed) = %v, want ErrManyVersions naming both", err)
	}
	if got, err := coreapi.ShowPlugin(ctx, b, w.Ports().Trust, "r", "zed@2.0.0"); err != nil || got.Version != "2.0.0" {
		t.Errorf("ShowPlugin(zed@2.0.0) = %+v, %v, want version 2.0.0", got, err)
	}
	if _, err := coreapi.ShowPlugin(ctx, b, w.Ports().Trust, "r", "nope"); !errors.Is(err, coreapi.ErrNoSuchPlugin) {
		t.Errorf("ShowPlugin(nope) = %v, want ErrNoSuchPlugin", err)
	}
}

// TestShowPluginNeverShowsABundleThatChanged: a module changed after the lock
// is the judge's refusal, not a screen of grants for a yes.
func TestShowPluginNeverShowsABundleThatChanged(t *testing.T) {
	t.Parallel()
	w, b := approveWorld(t)
	if err := b.Change("r", model.PluginRef{Name: "alpha", Version: "1.0.0"}, append(listWasm(), 'x')); err != nil {
		t.Fatal(err)
	}
	got, err := coreapi.ShowPlugin(context.Background(), b, w.Ports().Trust, "r", "alpha")
	if !errors.Is(err, port.ErrDigest) || len(got.Grants) != 0 {
		t.Errorf("ShowPlugin of a changed bundle = %+v, %v; want ErrDigest and no grants", got, err)
	}
}

// TestApprovePluginRecordsOnlyForTheDigestShown: the digest of the installed
// bundle records the approval, kept with the grants' digest; any other, and
// none, records nothing; and a plugin whose module then changes is not
// approved. Planted: the digest not compared.
func TestApprovePluginRecordsOnlyForTheDigestShown(t *testing.T) {
	t.Parallel()
	w, b := approveWorld(t)
	ctx := context.Background()
	p := w.Ports()
	shown, err := coreapi.ShowPlugin(ctx, b, p.Trust, "r", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []model.Digest{"", shown.Digest[:len(shown.Digest)-1] + "0"} {
		if _, err := coreapi.ApprovePlugin(ctx, b, p.Trust, "r", "alpha", d); !errors.Is(err, coreapi.ErrDigestGiven) {
			t.Errorf("ApprovePlugin with digest %q = %v, want ErrDigestGiven", d, err)
		}
	}
	if ok, _ := p.Trust.Approved(ctx, shown.Digest); ok {
		t.Fatal("a refused approval recorded the bundle")
	}
	got, err := coreapi.ApprovePlugin(ctx, b, p.Trust, "r", "alpha", shown.Digest)
	if err != nil || !got.Recorded || !got.Approved {
		t.Fatalf("ApprovePlugin = %+v, %v; want recorded", got, err)
	}
	if ok, err := p.Trust.Approved(ctx, shown.Digest); err != nil || !ok {
		t.Errorf("Approved after = %v, %v; want yes", ok, err)
	}
	if err := b.Change("r", model.PluginRef{Name: "alpha", Version: "1.0.0"}, append(listWasm(), 'x')); err != nil {
		t.Fatal(err)
	}
	if _, err := coreapi.ShowPlugin(ctx, b, p.Trust, "r", "alpha"); err == nil {
		t.Error("a bundle changed after its approval was shown for a yes again")
	}
}

// TestShowPluginRefusesARecordItCannotRead: approvals that cannot be read are
// an error, never "not approved" on a screen that asks for a yes.
func TestShowPluginRefusesARecordItCannotRead(t *testing.T) {
	t.Parallel()
	w, b := approveWorld(t)
	w.SeedTrustRefused(port.ErrNotOwned)
	if _, err := coreapi.ShowPlugin(context.Background(), b, w.Ports().Trust, "r", "alpha"); !errors.Is(err, port.ErrNotOwned) {
		t.Errorf("ShowPlugin over a refused record = %v, want port.ErrNotOwned", err)
	}
}

// cachedManifest is listManifest with the feature cache-vars needed and the
// grant's cache_vars names.
func cachedManifest(name, v, names string) []byte {
	m := strings.Replace(string(listManifest(name, v)), "needs: []", "needs: [cache-vars]", 1)
	grant := "    versions: \">=1.0.0 <2.0.0\"\n"
	return []byte(strings.Replace(m, grant, grant+"    cache_vars: ["+names+"]\n", 1))
}

// TestAGainedCacheVarIsNotApprovedByTheOldYes: cache_vars is
// part of what an approval is a yes to. A bundle approved with one cache name
// that comes back with a second is not approved: its digest and its grant's
// digest both move, the screen names the grant changed, and it shows both
// names before the new yes. Planted failure: cache_vars left out of the
// grant's canonical form. Risk: a plugin gains a cache that steps share after
// its yes, and no one sees it.
func TestAGainedCacheVarIsNotApprovedByTheOldYes(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	var b porttest.Bundles
	ref := model.PluginRef{Name: "alpha", Version: "1.0.0"}
	ctx := context.Background()
	p := w.Ports()
	b.Put("r", ref, cachedManifest("alpha", "1.0.0", "GOCACHE"), listWasm())
	shown, err := coreapi.ShowPlugin(ctx, &b, p.Trust, "r", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got := shown.Grants[0].CacheVars; len(got) != 1 || got[0] != "GOCACHE" {
		t.Fatalf("the grant's cache vars = %q, want GOCACHE", got)
	}
	if _, err := coreapi.ApprovePlugin(ctx, &b, p.Trust, "r", "alpha", shown.Digest); err != nil {
		t.Fatal(err)
	}
	b.Put("r", ref, cachedManifest("alpha", "1.0.0", "GOCACHE, GOMODCACHE"), listWasm())
	again, err := coreapi.ShowPlugin(ctx, &b, p.Trust, "r", "alpha")
	switch {
	case err != nil:
		t.Fatalf("ShowPlugin after the gain = %v", err)
	case again.Approved || again.Digest == shown.Digest:
		t.Errorf("the bundle with a gained cache name is approved %v, digest %s; want not approved and another digest", again.Approved, again.Digest)
	case again.GrantsDigest == shown.GrantsDigest:
		t.Errorf("the grants' digest %s did not move with the gained name", again.GrantsDigest)
	case len(again.Since.Changes) != 1 || again.Since.Changes[0].ID != "standin" || again.Since.Changes[0].Change != model.GrantChanged:
		t.Errorf("since = %+v, want the grant standin changed", again.Since)
	case len(again.Grants[0].CacheVars) != 2:
		t.Errorf("the screen shows the cache vars %q, want both", again.Grants[0].CacheVars)
	}
}
