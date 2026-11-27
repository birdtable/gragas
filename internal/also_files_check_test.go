package internal

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// alsoFileFindings holds each files: entry of an also row to its need
// (gragas#1806), as staleAlso holds the row: a named file must stand in its
// owner's folder and import the row's library, and tests needs a test file of
// the folder that does. read answers the imports of one file by its slash path
// from the root, as go list reads it.
func alsoFileFindings(tb *layers.Table, w world, read func(file string) ([]string, error)) []string {
	var out []string
	for _, e := range tb.External {
		for _, a := range e.Also {
			p := w.pkgs[a.Owner]
			for _, f := range a.Files {
				row := a.Owner + " is listed as another owner of " + e.Path + " in " + f + " of " + layers.File
				switch {
				case p == nil:
					// the row's own finding is staleAlso's
				case f == layers.TestsOnly:
					if !slices.ContainsFunc(p.testImports, func(imp string) bool { return isLibrary(tb, w, imp, e) }) {
						out = append(out, row+", and no test file of its folder imports it: drop tests from the files")
					}
				case !slices.Contains(p.files, f) && !slices.Contains(p.tests, f):
					out = append(out, row+", and the file is not there: remove it from the files")
				default:
					imps, err := read(path.Join(a.Owner, f))
					switch {
					case err != nil:
						out = append(out, row+", and go list could not read it: "+err.Error())
					case !slices.ContainsFunc(imps, func(imp string) bool { return isLibrary(tb, w, imp, e) }):
						out = append(out, row+", and the file no longer imports it: remove it from the files")
					}
				}
			}
		}
	}
	return out
}

// isLibrary is whether the import imp is the library e, as importsLibrary
// reads one.
func isLibrary(tb *layers.Table, w world, imp string, e layers.External) bool {
	if _, own := w.rel(imp); own {
		return false
	}
	got, ok := tb.Owner(imp)
	return ok && got.Path == e.Path
}

// goFileImports reads one file's imports, its tests' too, with go list of the
// file alone.
func goFileImports(file string) ([]string, error) {
	out, err := goOut(nil, "-e", "-json=Imports,TestImports,XTestImports", "./"+file)
	if err != nil {
		return nil, err
	}
	var lp struct{ Imports, TestImports, XTestImports []string }
	if err := json.Unmarshal(out, &lp); err != nil {
		return nil, fmt.Errorf("go list of %s: %w", file, err)
	}
	return slices.Concat(lp.Imports, lp.TestImports, lp.XTestImports), nil
}

// TestEveryAlsoFileExistsAndImportsItsLibrary: the real layers.yaml holds
// each files: entry to its need.
func TestEveryAlsoFileExistsAndImportsItsLibrary(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)
	if got := alsoFileFindings(tb, w, goFileImports); len(got) != 0 {
		t.Fatalf("the tree: %q", got)
	}
}

// TestAnAlsoFileThatIsGoneOrDroppedItsImportFails: planted, a listed file the
// folder does not hold, a listed file that no longer imports the library, and
// tests with no test file that imports it.
func TestAnAlsoFileThatIsGoneOrDroppedItsImportFails(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		claim, owner, lib, file string
		drop                    func(p *pkg)
		read                    func(string) ([]string, error)
		want                    string
	}{
		{"a file that is gone", "internal/host/sandbox", "go-files/confine", "sandbox.go",
			func(p *pkg) { p.files = slices.DeleteFunc(p.files, func(f string) bool { return f == "sandbox.go" }) }, nil,
			"in sandbox.go of layers.yaml, and the file is not there"},
		{"a file that dropped the import", "internal/host/sandbox", "go-files/confine", "sandbox.go", func(*pkg) {},
			func(string) ([]string, error) { return []string{"os"}, nil },
			"in sandbox.go of layers.yaml, and the file no longer imports it"},
		{"tests that no test file imports", "cmd/gragas-tmp", "go-proc/proc", "tests",
			func(p *pkg) { p.testImports = nil }, nil,
			"in tests of layers.yaml, and no test file of its folder imports it"},
	} {
		t.Run(c.claim, func(t *testing.T) {
			t.Parallel()
			tb, w := tree(t)
			p := w.pkgs[c.owner]
			c.drop(p)
			read := c.read
			if read == nil {
				read = goFileImports
			}
			got := alsoFileFindings(tb, w, read)
			if !slices.ContainsFunc(got, func(f string) bool { return strings.Contains(f, c.owner) && strings.Contains(f, c.want) }) {
				t.Errorf("no finding names %s and says %q; found %q", c.owner, c.want, got)
			}
		})
	}
}
