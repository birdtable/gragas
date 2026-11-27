package internal

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// The shared registry held to the tree (arch.shared.8): each owner is a
// package of layers.yaml, each banned primitive has its ban and its owner's
// exclusion in .golangci.yml and every ban under the rule has a banned row,
// each banned row's export is in its owner, and each shared-module export is
// in the module's source at the version go.mod pins. The registry's schema is
// held by make gen, which reads it through gate/schema/shared.schema.json.

// sharedRegistry is the registry and the table of the tree.
func sharedRegistry(t *testing.T) (*layers.Shared, *layers.Table) {
	t.Helper()
	tb := table(t)
	var tree []string
	for _, p := range goList(t) {
		tree = append(tree, strings.TrimPrefix(strings.TrimPrefix(p, tb.Module), "/"))
	}
	tb.WithTree(tree)
	data, err := os.ReadFile(filepath.Join(root, layers.SharedFile))
	if err != nil {
		t.Fatal(err)
	}
	s, err := layers.ParseShared(data, func(any) []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return s, tb
}

// dirSources are the non-test Go files directly in dir, by name.
func dirSources(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = b
	}
	return out
}

// exportsOf is the exported names of the package in dir.
func exportsOf(t *testing.T, dir string) []string {
	t.Helper()
	names, err := layers.Exported(dirSources(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	return names
}

// moduleDir is the folder of a shared-module package, as go list finds it at
// the version go.mod pins.
func moduleDir(t *testing.T, pkg string) string {
	t.Helper()
	var dir string
	err := golist(func(dec *json.Decoder) error {
		var p struct{ Dir string }
		if err := dec.Decode(&p); err != nil {
			return err
		}
		dir = p.Dir
		return nil
	}, "-json=Dir", "--", pkg)
	if err != nil || dir == "" {
		t.Fatalf("%s is not at the version go.mod pins (run make private): %v", pkg, err)
	}
	return dir
}

// sharedProblems is what does not hold of s over tb, the lint file and the
// exports each package of a row has.
func sharedProblems(s *layers.Shared, tb *layers.Table, lint []byte, exports map[string][]string) []string {
	var out []string
	if err := tb.Hold(s); err != nil {
		out = append(out, err.Error())
	}
	out = append(out, s.HeldBy(lint)...)
	for _, pkg := range slices.Sorted(maps.Keys(exports)) {
		out = append(out, s.Missing(pkg, exports[pkg])...)
	}
	return out
}

// treeExports is the exports of each owner of a banned row, from the tree,
// and of each shared-module package, from the module cache.
func treeExports(t *testing.T, s *layers.Shared) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, c := range s.Rows {
		if c.Banned() {
			out[c.Owner] = exportsOf(t, filepath.Join(root, filepath.FromSlash(c.Owner)))
		}
		if c.Module != "" {
			out[c.Module] = exportsOf(t, moduleDir(t, c.Module))
		}
	}
	return out
}

func TestTheSharedRegistryHoldsToTheTree(t *testing.T) {
	t.Parallel()
	s, tb := sharedRegistry(t)
	if err := tb.Hold(s); err != nil {
		t.Fatal(err)
	}
	lint, err := os.ReadFile(filepath.Join(root, tb.LintFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range sharedProblems(s, tb, lint, treeExports(t, s)) {
		t.Error(p)
	}
}

// TestTheSharedRegistryChecksFireOnAPlantedBreach: on the tree's own files,
// an owner the table lacks, a banned primitive whose ban is gone, a ban no
// row makes, and a shared-module export the pinned version lacks each fail.
func TestTheSharedRegistryChecksFireOnAPlantedBreach(t *testing.T) {
	t.Parallel()
	s, tb := sharedRegistry(t)
	lint, err := os.ReadFile(filepath.Join(root, tb.LintFile))
	if err != nil {
		t.Fatal(err)
	}
	exports := treeExports(t, s)
	if p := sharedProblems(s, tb, lint, exports); len(p) != 0 {
		t.Fatalf("the tree does not hold, so no plant is read: %q", p)
	}
	ban := "- pattern: '^syscall\\.Flock$'"
	if !strings.Contains(string(lint), ban) {
		t.Fatalf("the lint file has no %s to plant on", ban)
	}
	gone := *s
	gone.Rows = slices.Clone(s.Rows)
	gone.Rows[0].Owner = "internal/host/gone"
	module := slices.IndexFunc(s.Rows, func(c layers.Capability) bool { return c.Module != "" })
	if module < 0 {
		t.Fatal("no row names a shared-module export to plant on")
	}
	lacking := maps.Clone(exports)
	lacking[s.Rows[module].Module] = slices.DeleteFunc(slices.Clone(exports[s.Rows[module].Module]), func(n string) bool { return n == s.Rows[module].ModuleExport })
	for _, tc := range []struct {
		name    string
		s       *layers.Shared
		lint    string
		exports map[string][]string
		says    string
	}{
		{"an owner the table lacks", &gone, string(lint), exports, "is not a package of layers.yaml"},
		{"a banned primitive with no ban", s, strings.Replace(string(lint), ban, "- pattern: '^syscall\\.Flick$'", 1), exports, "has no ban in the lint file"},
		{"a ban with no row", s, strings.Replace(string(lint), "use its Lock.Take", "use its Lock.Hold", 1), exports, "is no ban a banned row"},
		{"a shared-module export the pin lacks", s, string(lint), lacking, "at the pinned tag has no export"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sharedProblems(tc.s, tb, []byte(tc.lint), tc.exports)
			if !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.says) }) {
				t.Errorf("problems = %q, want one saying %q", got, tc.says)
			}
		})
	}
}

// TestTheSharedAllowancesAreCountedInTheBaselines: gate/baselines.yaml holds the
// number of allowances gate/shared.yaml lists, so an allowance cannot be added
// without the count moving where a reviewer reads it, and the count cannot fall
// short of the file. Risk: an allowance added with the baseline left behind.
func TestTheSharedAllowancesAreCountedInTheBaselines(t *testing.T) {
	t.Parallel()
	s, _ := sharedRegistry(t)
	n := 0
	for _, c := range s.Rows {
		n += len(c.Allow)
	}
	data, err := os.ReadFile(filepath.Join(root, "gate", "baselines.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "\nshared_allowances: " + strconv.Itoa(n) + "\n"
	if !strings.Contains("\n"+string(data), want) {
		t.Errorf("gate/baselines.yaml holds no line %q: gate/shared.yaml lists %d allowances", strings.TrimSpace(want), n)
	}
}

// issuedIDs is every rule id rulebook/ids.lock lists, one to a line, with
// its comments and blank lines left out, as the rulebook's loader reads it.
func issuedIDs(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(layers.IDsFile)))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for line := range strings.Lines(string(data)) {
		if id := strings.TrimSpace(line); id != "" && !strings.HasPrefix(id, "#") {
			ids = append(ids, id)
		}
	}
	return ids
}

// TestEveryRuleIDTheLintFileCitesWasIssued: each rule id a ban, a deny or an
// exclusion of .golangci.yml opens with is in rulebook/ids.lock. On the
// tree's own file, a real id passes and a made-up one in the os.Exit ban is
// refused, naming the ban's pattern and the id (gragas#1861). Risk: a ban
// that cites a rule that does not exist, which a probe on #1842
// showed passing the shared-registry check.
func TestEveryRuleIDTheLintFileCitesWasIssued(t *testing.T) {
	t.Parallel()
	lint, err := os.ReadFile(filepath.Join(root, table(t).LintFile))
	if err != nil {
		t.Fatal(err)
	}
	ids := issuedIDs(t)
	for _, p := range layers.Unissued(lint, ids) {
		t.Error(p)
	}
	issued := `msg: "go.errors.4: only main exits"`
	if !slices.Contains(ids, "go.errors.4") || !strings.Contains(string(lint), issued) {
		t.Fatalf("the lint file has no %s of an issued id to plant on", issued)
	}
	made := "gates.readers." + strconv.Itoa(9)
	if slices.Contains(ids, made) {
		t.Fatalf("%s is issued, so it cannot stand for one that is not", made)
	}
	planted := strings.Replace(string(lint), issued, `msg: "`+made+`: only main exits"`, 1)
	got := layers.Unissued([]byte(planted), ids)
	if len(got) != 1 || !strings.Contains(got[0], `the forbidigo ban ^os\.Exit$ opens with `+made) {
		t.Errorf("a ban that cites %s: Unissued = %q, want it refused, naming the pattern and the id", made, got)
	}
}
