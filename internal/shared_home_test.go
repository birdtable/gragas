package internal

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// homeKey is what a row's home: reads as: a platform repository and an issue, repo#n.
var homeKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*#[0-9]+$`)

// under says whether path is dir or a package below it.
func under(path, dir string) bool { return path == dir || strings.HasPrefix(path, dir+"/") }

// importsAny says whether the package imports the module's package rel, or one below it when below.
func (w world) importsAny(p *pkg, rel string, below bool) bool {
	for _, imp := range p.imports {
		if r, ok := w.rel(imp); ok && (r == rel || below && under(r, rel)) {
			return true
		}
	}
	return false
}

// sharedPackages are the packages of line 1 (gragas#1753), read from go list: a package with a
// non-test file that is not a main, is not internal/model itself, sits outside internal/port and
// internal/dev, and imports neither internal/model nor internal/port. A tree that has none is no tree
// the check can judge: a go list that answered nothing would pass for ever.
func sharedPackages(w world) ([]string, error) {
	if len(w.pkgs) == 0 {
		return nil, errors.New("could not run: go list answered no package")
	}
	var out []string
	for _, p := range w.sorted() {
		switch {
		case len(p.files) == 0, p.name == "main", p.path == "internal/model", p.path == ".",
			under(p.path, "internal/port"), under(p.path, "internal/dev"),
			w.importsAny(p, "internal/model", false), w.importsAny(p, "internal/port", true):
			continue
		}
		out = append(out, p.path)
	}
	if len(out) == 0 {
		return nil, errors.New("could not run: go list found no shared package, and the tree has some (internal/ordered is one)")
	}
	return out, nil
}

// names says whether text names package rel as a whole word: internal/config does not name internal/config/testing.
func names(text, rel string) bool {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_/.-])` + regexp.QuoteMeta(rel) + `($|[^A-Za-z0-9_/-])`).MatchString(text)
}

// homeProblems holds the row of package rel to the line 1: it carries home: or product:, never both
// and never neither, and its why names the package, so a new package cannot arrive under an old reason.
func homeProblems(tb *layers.Table, rel string) []string {
	r := tb.RowOf(rel)
	if r == nil {
		return []string{rel + ": in no layer of " + layers.File + ", so it has no home or product"}
	}
	var out []string
	switch {
	case r.Home != "" && r.Product != "":
		out = append(out, rel+": layer "+r.Layer+" names a home and a product, and a row names one")
	case r.Home == "" && r.Product == "":
		out = append(out, rel+": layer "+r.Layer+" carries neither home: <platform repo>#<n> nor product: <why>: "+
			"a package no other code of the program needs is a platform one's, or says why it is the product's")
	case r.Home != "" && !homeKey.MatchString(r.Home):
		out = append(out, rel+": layer "+r.Layer+" home "+fmt.Sprintf("%q", r.Home)+" is no <platform repo>#<n>")
	}
	if !names(r.Why, rel) {
		out = append(out, rel+": the why of layer "+r.Layer+" does not name it, so it would arrive under a reason written before it")
	}
	return out
}

// sharedHomes is every finding of line 1 over the world.
func sharedHomes(tb *layers.Table, w world) ([]string, error) {
	shared, err := sharedPackages(w)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, rel := range shared {
		out = append(out, homeProblems(tb, rel)...)
	}
	return out, nil
}

// TestSharedPackageHasAHome holds the line 1 (gragas#1753): each package that is no part of the
// product's domain (it imports neither internal/model nor internal/port, and is no main, no port, no
// dev package) sits in a row of layers.yaml that says where its home is or why it is the product's, and
// whose why names it. Risk: a leaf that arrives with no question of whether platform has it.
func TestSharedPackageHasAHome(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)
	problems, err := sharedHomes(tb, w)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestSharedPackageHasAHomePlanted plants each way line 1 is broken, in the real table: a package with
// no home or product; a row with both; a malformed home; a reason that does not name the package; a
// package added to a row whose reason was written before it; and a tree that shows no package.
func TestSharedPackageHasAHomePlanted(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)
	shared, err := sharedPackages(w)
	if err != nil {
		t.Fatal(err)
	}
	const victim = "internal/canon"
	if !slices.Contains(shared, victim) {
		t.Fatalf("%s is no shared package of the tree: %v", victim, shared)
	}
	row := func(mut func(r *layers.Row)) []string {
		pt := table(t)
		mut(pt.RowOf(victim))
		return homeProblems(pt, victim)
	}
	if got := homeProblems(tb, victim); len(got) != 0 {
		t.Errorf("the real row of %s is red: %q", victim, got)
	}
	fires(t, row(func(r *layers.Row) { r.Home, r.Product = "", "" }), "neither home:")
	fires(t, row(func(r *layers.Row) { r.Home, r.Product = "go-files#1", "a reason" }), "names a home and a product")
	fires(t, row(func(r *layers.Row) { r.Home, r.Product = "go-files", "" }), "is no <platform repo>#<n>")
	fires(t, row(func(r *layers.Row) { r.Why = strings.ReplaceAll(r.Why, victim, "") }), "does not name it")

	// A package that arrives in a row that has the keys and a why written before it.
	fresh := table(t)
	old := fresh.RowOf(victim)
	old.Packages = append(old.Packages, "internal/fresh")
	nw := w.clone()
	nw.pkgs["internal/fresh"] = &pkg{path: "internal/fresh", name: "fresh", files: []string{"fresh.go"}}
	got, err := sharedHomes(fresh, nw)
	if err != nil {
		t.Fatal(err)
	}
	fires(t, got, "internal/fresh: the why of layer "+old.Layer+" does not name it")

	// A package that no row holds.
	nw.pkgs["internal/stray"] = &pkg{path: "internal/stray", name: "stray", files: []string{"stray.go"}}
	got, err = sharedHomes(table(t), nw)
	if err != nil {
		t.Fatal(err)
	}
	fires(t, got, "internal/stray: in no layer")

	// A package that imports internal/model is not subject, and a main is not: neither is asked for a home.
	nw = w.clone()
	nw.pkgs["internal/domainish"] = &pkg{path: "internal/domainish", name: "domainish", files: []string{"d.go"}, imports: []string{tb.Module + "/internal/model"}}
	nw.pkgs["internal/tool"] = &pkg{path: "internal/tool", name: "main", files: []string{"m.go"}}
	if got, err = sharedHomes(table(t), nw); err != nil || slices.ContainsFunc(got, func(s string) bool {
		return strings.HasPrefix(s, "internal/domainish") || strings.HasPrefix(s, "internal/tool")
	}) {
		t.Errorf("a package of the domain or a main was asked for a home: %q %v", got, err)
	}

	// The subject missing: a go list that answered nothing, or only packages that are no subject, is refused.
	if _, err := sharedHomes(tb, world{module: tb.Module, pkgs: map[string]*pkg{}}); err == nil {
		t.Error("a go list that answered no package passed")
	}
	only := world{module: tb.Module, pkgs: map[string]*pkg{"cmd/gragas": {path: "cmd/gragas", name: "main", files: []string{"m.go"}}}}
	if _, err := sharedHomes(tb, only); err == nil {
		t.Error("a tree with no shared package passed")
	}
}
