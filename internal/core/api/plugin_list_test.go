package coreapi_test

import (
	"context"
	"errors"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func listWasm() []byte { return []byte("\x00asm\x01\x00\x00\x00standin") }

// listManifest is a manifest of the plugin name at version v that the judge
// passes.
func listManifest(name, v string) []byte {
	return []byte("contract: 1\nneeds: []\nname: " + name + "\nversion: " + v + "\ntool: standin\n" +
		"tool_versions: \">=1.0.0 <2.0.0\"\ntargets: [source]\nprovides: [entries]\nextends: false\nwarning: false\n" +
		"programs:\n  - id: standin\n    path: bin/standin\n    version_argv: [--version]\n    versions: \">=1.0.0 <2.0.0\"\n" +
		"wasm: " + string(digest.Prefixed(listWasm())) + "\n")
}

// TestListPluginsShowsEachInstalledPluginAndWhetherItsDigestIsApproved: two
// plugins, one approved; the rows come by name and version, with the tool of
// the manifest and the digest as read. Planted: the approval taken from the
// digest, or the rows unsorted.
func TestListPluginsShowsEachInstalledPluginAndWhetherItsDigestIsApproved(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	var b porttest.Bundles
	b.Put("r", model.PluginRef{Name: "zed", Version: "1.0.0"}, listManifest("zed", "1.0.0"), listWasm())
	b.Put("r", model.PluginRef{Name: "alpha", Version: "2.0.0"}, listManifest("alpha", "2.0.0"), listWasm())
	got, err := coreapi.ListPlugins(context.Background(), &b, w.Ports().Trust, "r")
	if err != nil || len(got.Rows) != 2 {
		t.Fatalf("ListPlugins = %+v, %v; want two rows", got, err)
	}
	if a := got.Rows[0]; a.Name != "alpha" || a.Version != "2.0.0" || a.Tool != "standin" || a.Approved || a.Note != "" || !a.Digest.Formed() {
		t.Errorf("first row = %+v, want alpha 2.0.0 of standin, unapproved, no note", a)
	}
	w.SeedApproval(model.Approval{Digest: got.Rows[1].Digest, Ref: "zed@1.0.0"})
	got, err = coreapi.ListPlugins(context.Background(), &b, w.Ports().Trust, "r")
	if err != nil || got.Rows[0].Approved || !got.Rows[1].Approved || got.Rows[1].Name != "zed" {
		t.Fatalf("ListPlugins after zed's approval = %+v, %v; want only zed approved", got, err)
	}
}

// TestListPluginsOfAChangedModuleIsUnapprovedWithItsNewDigest: a module that
// changed by a byte after the approval is another digest, so it is not
// approved, and the row says why.
func TestListPluginsOfAChangedModuleIsUnapprovedWithItsNewDigest(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	var b porttest.Bundles
	ref := model.PluginRef{Name: "zed", Version: "1.0.0"}
	b.Put("r", ref, listManifest("zed", "1.0.0"), listWasm())
	before, _ := coreapi.ListPlugins(context.Background(), &b, w.Ports().Trust, "r")
	w.SeedApproval(model.Approval{Digest: before.Rows[0].Digest, Ref: "zed@1.0.0"})
	if err := b.Change("r", ref, append([]byte{}, append(listWasm(), 'x')...)); err != nil {
		t.Fatal(err)
	}
	got, err := coreapi.ListPlugins(context.Background(), &b, w.Ports().Trust, "r")
	if err != nil || len(got.Rows) != 1 {
		t.Fatalf("ListPlugins = %+v, %v", got, err)
	}
	if r := got.Rows[0]; r.Approved || r.Digest == before.Rows[0].Digest || r.Note == "" {
		t.Errorf("row of a changed module = %+v, want unapproved, another digest and a note", r)
	}
}

// TestListPluginsOfARootWithNoPluginsIsNoRows: none is a nil list and no error.
func TestListPluginsOfARootWithNoPluginsIsNoRows(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	got, err := coreapi.ListPlugins(context.Background(), &porttest.Bundles{}, w.Ports().Trust, "r")
	if err != nil || got.Rows != nil {
		t.Errorf("ListPlugins = %+v, %v; want no rows", got, err)
	}
}

// TestListPluginsRefusesARecordItCannotRead: an approvals record the adapter
// refuses is an error, never a plugin listed as unapproved.
func TestListPluginsRefusesARecordItCannotRead(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedTrustRefused(port.ErrNotOwned)
	var b porttest.Bundles
	b.Put("r", model.PluginRef{Name: "zed", Version: "1.0.0"}, listManifest("zed", "1.0.0"), listWasm())
	if _, err := coreapi.ListPlugins(context.Background(), &b, w.Ports().Trust, "r"); !errors.Is(err, port.ErrNotOwned) {
		t.Errorf("ListPlugins over a refused record = %v, want port.ErrNotOwned", err)
	}
}

// TestListPluginsOfARefusedManifestIsNeverApprovedWhateverItsDigest: a bundle
// whose manifest names another plugin than its folder has its digest approved
// and is listed with a note and unapproved, while a judged-good one with the
// same approval is approved. Planted: Approved without the judge (the refused
// row goes red); Approved never set (the good row goes red).
func TestListPluginsOfARefusedManifestIsNeverApprovedWhateverItsDigest(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	var b porttest.Bundles
	good := model.PluginRef{Name: "good", Version: "1.0.0"}
	bad := model.PluginRef{Name: "bad", Version: "1.0.0"}
	b.Put("r", good, listManifest("good", "1.0.0"), listWasm())
	b.Put("r", bad, listManifest("other", "1.0.0"), listWasm())
	first, err := coreapi.ListPlugins(context.Background(), &b, w.Ports().Trust, "r")
	if err != nil || len(first.Rows) != 2 {
		t.Fatalf("ListPlugins = %+v, %v; want two rows", first, err)
	}
	for _, r := range first.Rows {
		w.SeedApproval(model.Approval{Digest: r.Digest, Ref: r.Name + "@" + r.Version})
	}
	got, err := coreapi.ListPlugins(context.Background(), &b, w.Ports().Trust, "r")
	if err != nil || len(got.Rows) != 2 {
		t.Fatalf("ListPlugins after the approvals = %+v, %v", got, err)
	}
	if r := got.Rows[0]; r.Name != "bad" || r.Approved || r.Note == "" {
		t.Errorf("row of a refused manifest = %+v, want unapproved with a note", r)
	}
	if r := got.Rows[1]; r.Name != "good" || !r.Approved || r.Note != "" {
		t.Errorf("row of a judged-good bundle = %+v, want approved, no note", r)
	}
}
