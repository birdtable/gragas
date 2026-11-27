package internal

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The explorer stays separable from the engine (arch.layers.13, A61): the
// subset build, cmd/gragas-subset, is the shipped command tree with the Assets
// port filled by an adapter that answers port.ErrUnavailable, and the gate
// builds it in every run. What each Go subset must never link is data,
// gate/subsets.json's rows of build "go" (gragas#341): its forbidden packages,
// and with no_embeds no package of the module that embeds a file, past those
// it allows, each on a decision record. The TypeScript rows are
// explorer/test/subset-builds.test.ts's.

// subsetsFile is the subset builds' list.
const subsetsFile = "gate/subsets.json"

// shippedMain is the binary every Go subset is a subset of: its build is the
// control, linking what the subsets must not.
const shippedMain = "./cmd/gragas"

// subsetRow is one row of gate/subsets.json, as much as the Go check reads.
type subsetRow struct {
	Name      string   `json:"name"`
	Build     string   `json:"build"`
	Entry     string   `json:"entry"`
	Forbidden []string `json:"forbidden"`
	NoEmbeds  bool     `json:"no_embeds"`
	Allowed   []struct {
		Package string   `json:"package"`
		Files   []string `json:"files"`
	} `json:"allowed"`
}

// goSubsets are the rows of build "go"; none is a failure, since the
// separable engine then has no check.
func goSubsets(t *testing.T) []subsetRow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, subsetsFile))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Subsets []subsetRow `json:"subsets"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", subsetsFile, err)
	}
	out := slices.DeleteFunc(doc.Subsets, func(s subsetRow) bool { return s.Build != "go" })
	if len(out) == 0 {
		t.Fatalf("%s lists no Go subset, so nothing holds the engine separable", subsetsFile)
	}
	return out
}

// embedding is one package of a build and the files it embeds.
type embedding struct {
	imports []string
	embeds  []string
}

// buildOf is go list -deps of one main package: each package, by import path,
// with what it imports and what it embeds.
func buildOf(t *testing.T, main string) map[string]embedding {
	t.Helper()
	out := map[string]embedding{}
	err := golist(func(dec *json.Decoder) error {
		var lp struct {
			ImportPath string
			Imports    []string
			EmbedFiles []string
		}
		if err := dec.Decode(&lp); err != nil {
			return err
		}
		out[lp.ImportPath] = embedding{imports: lp.Imports, embeds: lp.EmbedFiles}
		return nil
	}, buildArgs(main)...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// within is whether pkg, a path from the module, is one of list's packages or
// inside one.
func within(pkg string, list []string) bool {
	return slices.ContainsFunc(list, func(f string) bool { return pkg == f || strings.HasPrefix(pkg, f+"/") })
}

// linksForbidden is why a build's packages break the row, or nil: a
// forbidden package among them, and with no_embeds a package of the module
// that embeds a file and is not allowed. A library's own embedded files (the
// schema checker's metaschemas) are not the explorer's and are the closure
// list's to hold.
func linksForbidden(module string, row subsetRow, build map[string]embedding) []string {
	allowed := make([]string, 0, len(row.Allowed))
	listed := map[string][]string{}
	for _, a := range row.Allowed {
		allowed = append(allowed, a.Package)
		if a.Files != nil {
			listed[a.Package] = a.Files
		}
	}
	var out []string
	for p, e := range build {
		rel, ours := strings.CutPrefix(p, module+"/")
		switch {
		case !ours:
		case within(rel, row.Forbidden):
			out = append(out, p+": forbidden by "+subsetsFile)
		case row.NoEmbeds && len(e.embeds) > 0 && !slices.Contains(allowed, rel):
			out = append(out, p+": embeds files")
		case row.NoEmbeds && listed[rel] != nil && !sameFiles(e.embeds, listed[rel]):
			out = append(out, p+": embeds files other than the row's list")
		}
	}
	slices.Sort(out)
	return out
}

// sameFiles is whether two lists name the same files, in any order.
func sameFiles(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// TestEachGoSubsetLinksNothingItsRowForbids: each Go subset's package list
// holds no forbidden package and, with no_embeds, no embedded file past those
// allowed, and each allowed package is still linked, so the list only falls.
// The control is that cmd/gragas links each forbidden package and the check
// names it; the plants are a forbidden package linked into the subset and a
// library of the subset that embeds a file. Risk: the explorer's files
// travelling in through an importer of the command tree.
func TestEachGoSubsetLinksNothingItsRowForbids(t *testing.T) {
	t.Parallel()
	module := table(t).Module
	shipped := buildOf(t, shippedMain)
	for _, row := range goSubsets(t) {
		t.Run(row.Name, func(t *testing.T) {
			subset := buildOf(t, row.Entry)
			if _, ok := subset[module+"/internal/cli/root"]; !ok {
				t.Fatal("the subset build does not link the command tree, so it proves nothing")
			}
			if bad := linksForbidden(module, row, subset); len(bad) != 0 {
				t.Errorf("the subset build links what %s forbids: %v", subsetsFile, bad)
			}
			for _, a := range row.Allowed {
				if _, ok := subset[module+"/"+a.Package]; !ok {
					t.Errorf("%s allows %s, which the subset no longer links: take it off", subsetsFile, a.Package)
				}
			}

			// The control: the shipped binary links each forbidden package and,
			// under no_embeds, a package that embeds files; each allowed
			// package does embed some, or it needs no allowance.
			bad := linksForbidden(module, row, shipped)
			for _, f := range row.Forbidden {
				if !slices.Contains(bad, module+"/"+f+": forbidden by "+subsetsFile) {
					t.Errorf("%s forbids %s, which %s does not link either: the check cannot see it", subsetsFile, f, shippedMain)
				}
			}
			if row.NoEmbeds && !slices.ContainsFunc(bad, func(b string) bool { return strings.HasSuffix(b, ": embeds files") }) {
				t.Errorf("%s links no package that embeds files, so the no_embeds check cannot see one", shippedMain)
			}
			for _, a := range row.Allowed {
				if len(subset[module+"/"+a.Package].embeds) == 0 {
					t.Errorf("%s allows %s, which embeds no file: take it off", subsetsFile, a.Package)
				}
			}

			// The plant: the subset links each forbidden package.
			for _, f := range row.Forbidden {
				planted := maps.Clone(subset)
				planted[module+"/"+f] = shipped[module+"/"+f]
				if bad := linksForbidden(module, row, planted); !slices.Contains(bad, module+"/"+f+": forbidden by "+subsetsFile) {
					t.Errorf("a planted link of %s gave %v, want it named", f, bad)
				}
			}

			// The plant: a library of the subset embeds a file.
			if row.NoEmbeds {
				lib := module + "/internal/cli/root"
				planted := maps.Clone(subset)
				planted[lib] = embedding{imports: subset[lib].imports, embeds: []string{"x.txt"}}
				if bad := linksForbidden(module, row, planted); !slices.Contains(bad, lib+": embeds files") {
					t.Errorf("a planted embedded file in %s gave %v, want it named", lib, bad)
				}
			}

			// The plant: an allowed package that lists its files embeds a second
			// one. The control is the package as the subset links it, which embeds
			// exactly its list and is not named.
			for _, a := range row.Allowed {
				if a.Files == nil {
					continue
				}
				pkg := module + "/" + a.Package
				planted := maps.Clone(subset)
				planted[pkg] = embedding{imports: subset[pkg].imports, embeds: append(slices.Clone(subset[pkg].embeds), "second.txt")}
				if bad := linksForbidden(module, row, planted); !slices.Contains(bad, pkg+": embeds files other than the row's list") {
					t.Errorf("a planted second embedded file in %s gave %v, want it named", a.Package, bad)
				}
				if bad := linksForbidden(module, row, subset); slices.Contains(bad, pkg+": embeds files other than the row's list") {
					t.Errorf("%s embeds %v, not %v, the row's list", a.Package, subset[pkg].embeds, a.Files)
				}
			}
		})
	}
}
