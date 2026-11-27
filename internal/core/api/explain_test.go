package coreapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestResolveReadsTheParentFromTheCacheAndNamesThePinWhenItIsNotThere: an
// overlay with its parent's release in the cache is two layers, root first; with
// none it ends not ready naming the pin. Risk: a chain that fetches, or that
// resolves one layer and says nothing of the parent it could not read.
func TestResolveReadsTheParentFromTheCacheAndNamesThePinWhenItIsNotThere(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w, root := completed(t, fetched(t))
	r, err := coreapi.ResolveChain(ctx, w.Ports(), root, coreapi.ChainAsk{Cache: cacheDir, Contracts: w.Ports().Schema})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Resolved.Layers, ","); got != "fixture-parent,house" {
		t.Errorf("layers = %q, want the parent then the overlay", got)
	}
	none, nroot := completed(t, nil)
	_, err = coreapi.ResolveChain(ctx, none.Ports(), nroot, coreapi.ChainAsk{Contracts: none.Ports().Schema})
	var parent *coreapi.ParentError
	if !errors.As(err, &parent) || !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "parent not fetched: fixture-parent 0.9.0") {
		t.Errorf("err = %v, want not ready naming the pin", err)
	}
}

// TestExplainSaysWhatHoldsARuleByItsIdAndAnUnknownOneIsNamed: a rule asked by
// its id is explained, and one the rulebook does not hold is an UnknownRule.
func TestExplainSaysWhatHoldsARuleByItsIdAndAnUnknownOneIsNamed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w, root := completed(t, fetched(t))
	e, err := coreapi.ExplainRule(ctx, w.Ports(), root, "go.errors.1")
	if err != nil || len(e.Rules) != 1 || string(e.Rules[0].Rule) != "go.errors.1" {
		t.Fatalf("explained %+v, err %v", e, err)
	}
	_, err = coreapi.ExplainRule(ctx, w.Ports(), root, "go.nothing.9")
	if _, ok := errors.AsType[*coreapi.UnknownRule](err); !ok {
		t.Errorf("err = %v, want an UnknownRule", err)
	}
}

// completed is overlayWorld's book at the root of its own folder, with the
// layout file its structure names, which the world lacks, so the rulebook reads
// whole where the commands read it. cache is the files of the cache root, nil
// for none.
func completed(t *testing.T, cache map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(append(allSchemas(), "v3/exclusions.schema.json", "v1/document-bundle.schema.json")...)
	w.SeedAssets(realContractSchemas(t))
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("book", map[string]string{
		"rulebook.yaml": overlayIdentity, "structure.yaml": ruleStructure,
		"language/go/rules/errors.yaml": bookRules, "language/go/rules/layout.yaml": "format: 3\npart: go\nconcern: layout\nrules: []\n",
	})
	if cache != nil {
		w.Seed(cacheDir, cache)
	}
	return w, root
}
