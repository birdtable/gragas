package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/ordered"
)

// TestTheGragasPresetHoldsTheKindsGragasComputes: presets/gragas/base.yaml
// holds a key for each kind the gragas rows of the readers table compute, and
// no other but the keys that compute the repository's own checks (gragas#928);
// and a gragas row reads golangci-lint's config, so its files are
// the golangci-lint row's, in its order (gragas#993).
// Planted failure: a key with no row and no computes, a row with no key, or a
// gragas row whose files differ from golangci-lint's.
func TestTheGragasPresetHoldsTheKindsGragasComputes(t *testing.T) {
	preset := yamlMap(t, filepath.Join("presets", "gragas", "base.yaml"))
	readers := yamlMap(t, filepath.Join("internal", "host", "assets", "audit", "readers.yaml"))
	var keys []string
	for _, k := range preset.Keys() {
		if preset.Sub(k).Str("computes") == "" {
			keys = append(keys, k)
		} else if preset.Sub(k).Str("reads") != "" {
			t.Errorf("presets/gragas/base.yaml's %s both reads a config and computes the repository", k)
		}
	}
	var kinds []string
	var lint string
	for _, r := range ordered.Maps(readers.List("readers")) {
		if r.Str("tool") == "golangci-lint" {
			lint = fmt.Sprint(r.List("files"))
		}
	}
	for _, r := range ordered.Maps(readers.List("readers")) {
		if r.Str("tool") != "gragas" {
			continue
		}
		kinds = append(kinds, r.Str("kind"))
		if reads := preset.Sub(r.Str("kind")); reads == nil || reads.Str("reads") != r.Str("host") {
			t.Errorf("presets/gragas/base.yaml's %s reads is not the readers row's host %q", r.Str("kind"), r.Str("host"))
		}
		if files := fmt.Sprint(r.List("files")); files != lint {
			t.Errorf("the gragas %s row reads %s, and golangci-lint reads %s: it reads golangci-lint's config, so the lists are one", r.Str("kind"), files, lint)
		}
	}
	slices.Sort(keys)
	slices.Sort(kinds)
	if len(kinds) == 0 {
		t.Fatal("the readers table has no gragas row: the preset holds keys nothing computes")
	}
	if !slices.Equal(keys, kinds) {
		t.Errorf("presets/gragas/base.yaml holds %v, but gragas's readers rows compute %v: a key needs its row, a row its key", keys, kinds)
	}
}

// yamlMap is the mapping of a YAML file of the repository.
func yamlMap(t *testing.T, rel string) *ordered.Map {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ordered.ParseStrict(data)
	m, ok := v.(*ordered.Map)
	if err != nil || !ok {
		t.Fatalf("%s is no mapping: %v", rel, err)
	}
	return m
}

// TestThePresetsComputesKeysAreTheOnesTheAuditComputes: the keys of
// presets/gragas/base.yaml that carry computes are exactly audit.Computed's,
// both ways (gragas#1974).
// Planted failure: the old preset with age: over the code that no longer
// computes it, or a computed key the preset does not list.
func TestThePresetsComputesKeysAreTheOnesTheAuditComputes(t *testing.T) {
	preset := yamlMap(t, filepath.Join("presets", "gragas", "base.yaml"))
	var keys []string
	for _, k := range preset.Keys() {
		if preset.Sub(k).Str("computes") != "" {
			keys = append(keys, k)
		}
	}
	if problems := computesDiffer(keys, audit.ComputedKeys()); len(problems) > 0 {
		t.Error(problems)
	}
	for _, c := range []struct {
		name   string
		preset []string
		want   string
	}{
		{"the old preset with age:", append(slices.Clone(audit.ComputedKeys()), "age"), `"age"`},
		{"a computed key the preset lacks", audit.ComputedKeys()[1:], `"link"`},
	} {
		if got := computesDiffer(c.preset, audit.ComputedKeys()); len(got) == 0 || !strings.Contains(got[0], c.want) {
			t.Errorf("%s is not red naming %s: %v", c.name, c.want, got)
		}
	}
}

// computesDiffer says, per key, where the preset's computes keys and the
// audit's differ.
func computesDiffer(preset, computed []string) []string {
	var out []string
	for _, k := range preset {
		if !audit.Computed(k) {
			out = append(out, fmt.Sprintf("the preset computes %q, which the audit does not", k))
		}
	}
	for _, k := range computed {
		if !slices.Contains(preset, k) {
			out = append(out, fmt.Sprintf("the audit computes %q, which the preset does not list", k))
		}
	}
	return out
}
