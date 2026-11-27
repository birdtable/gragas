package coreapi_test

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	mc "github.com/birdtable/gragas/internal/model/contract"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The base fixture rulebook and the bundle the contract's fixture holds for it
// (internal/host/assets/fixture/README.md: the bundle is a document the
// encoders produce from that input), read from the source tree.
const (
	fixtureBase   = "../../host/assets/fixture/base"
	fixtureBundle = "../../host/assets/contract/fixture/bundle.json"
	bundleSchemas = "../../host/assets/contract/schema/v1"
)

// filesBelow is every file below dir, by slash path below it. It fails the test
// when it finds none, so a comparison over it never passes on nothing.
func filesBelow(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	base := os.DirFS(filepath.FromSlash(dir))
	err := fs.WalkDir(base, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(base, p)
		if err != nil {
			return err
		}
		out[p] = string(b)
		return nil
	})
	if err != nil || len(out) == 0 {
		t.Fatalf("%s: %d files, %v", dir, len(out), err)
	}
	return out
}

// engineMissingDecision is the rules the engine lists as missing decision for
// the base fixture rulebook: the book as the format reads it, through Attend.
func engineMissingDecision(t *testing.T) []string {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/terms.schema.json",
		"v3/sources.schema.json", "v3/decisions.schema.json", "v3/exclusions.schema.json", "v3/history.schema.json",
		"v3/proposals.schema.json", "v3/block.schema.json", "v3/translations.schema.json")
	p := w.Ports()
	loaded, err := format.Load(p.Tree, p.Schema, w.Seed("repo", filesBelow(t, fixtureBase)))
	if err != nil || len(loaded.Findings) != 0 || len(loaded.Book.Rules) == 0 {
		t.Fatalf("the base fixture: %d rules, findings %v, %v", len(loaded.Book.Rules), loaded.Findings, err)
	}
	out := []string{}
	for _, it := range lifecycle.Attend(loaded.Book, "2026-11-22").Items {
		if it.Kind == model.AttentionNoDecision {
			out = append(out, string(it.Rule))
		}
	}
	slices.Sort(out)
	return out
}

// fixtureBundleDoc is the contract's fixture bundle, read as a reader reads it.
func fixtureBundleDoc(t *testing.T) mc.BundleDocument {
	t.Helper()
	w := porttest.NewWorld(t)
	assets := map[string]string{}
	for name, body := range filesBelow(t, bundleSchemas) {
		assets[path.Join(contract.SchemaDir, "v1", name)] = body
	}
	w.SeedSchemas("v1/document-bundle.schema.json")
	w.SeedAssets(assets)
	data, err := os.ReadFile(filepath.FromSlash(fixtureBundle))
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	doc, err := contract.ReadBundle(data, contract.Schemas{Check: p.Schema, Files: p.Assets})
	if err != nil {
		t.Fatalf("ReadBundle of the fixture bundle: %v", err)
	}
	return doc
}

// bundleMissingDecision is the rules a bundle reader decides are missing
// decision, from the bundle alone: with the governance's decision_required, each
// rule that is not retired and whose reasoning_decision is absent.
func bundleMissingDecision(doc mc.BundleDocument) []string {
	out := []string{}
	if !doc.Identity.Governance.DecisionRequired {
		return out
	}
	for _, r := range doc.Rules {
		if r.Status != mc.StatusRetired && (r.ReasoningDecision == nil || *r.ReasoningDecision == "") {
			out = append(out, string(r.ID))
		}
	}
	slices.Sort(out)
	return out
}

// TestTheBundleStatesTheDecisionTheEngineListsAsMissing: a bundle
// reader decides the attention kind missing decision from the bundle alone, as
// the engine does (internal/core/lifecycle/attention.go): the fixture bundle's
// set, from each rule's reasoning_decision and the governance's
// decision_required, is the engine's listing for the base fixture rulebook, and
// the rule whose reasoning names a decision carries its slug. Planted failure:
// the field dropped from the fixture bundle's go.errors.1, which then reads as
// missing a decision the engine does not list.
func TestTheBundleStatesTheDecisionTheEngineListsAsMissing(t *testing.T) {
	t.Parallel()
	doc := fixtureBundleDoc(t)
	engine, bundle := engineMissingDecision(t), bundleMissingDecision(doc)
	if len(engine) == 0 || !slices.Equal(engine, bundle) {
		t.Errorf("missing decision: the engine lists %v, the bundle gives %v; want the same, and not none", engine, bundle)
	}
	i := slices.IndexFunc(doc.Rules, func(r mc.Rule) bool { return r.ID == "go.errors.1" })
	if i < 0 || doc.Rules[i].ReasoningDecision == nil || *doc.Rules[i].ReasoningDecision != "wrap-errors" {
		t.Errorf("the fixture bundle's go.errors.1 does not carry reasoning_decision wrap-errors, the decision its reasoning names")
	}
}
