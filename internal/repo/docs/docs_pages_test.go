package docs

// The links of the explorer's boards (pages.cycle.1): the static pages under
// docs/explorer-boards/canvas are read beside the documents of docs/ and held
// by the same docs test, with a planted dead link.

import (
	"io/fs"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// canvasDirOfPages is where the explorer's boards, static pages, are.
const canvasDirOfPages = "docs/explorer-boards/canvas"

// readPages is every .dc.html page of canvasDirOfPages, by path from the root.
func readPages() (map[string]string, error) {
	out := map[string]string{}
	tree := os.DirFS(root)
	err := fs.WalkDir(tree, canvasDirOfPages, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".dc.html") {
			return err
		}
		b, err := fs.ReadFile(tree, p)
		out[p] = string(b)
		return err
	})
	return out, err
}

var pageLink = regexp.MustCompile(`\b(?:href|src)="([^"]*)"`)

// pageLinks is pages.cycle.1 for the boards: every link of a page to a file
// of the repository (a relative href or src) resolves. A link to another site,
// an anchor, a data URI and a route of the app (a path from its root) are not
// followed (offline).
func pageLinks(w docWorld) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(w.pages)) {
		for _, m := range pageLink.FindAllStringSubmatch(w.pages[name], -1) {
			target := m[1]
			if target == "" || strings.Contains(target, ":") || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "#") {
				continue
			}
			file, _, _ := strings.Cut(target, "#")
			if dest := path.Join(path.Dir(name), file); !w.exists(dest) {
				out = append(out, name+": the link "+target+" goes to no file")
			}
		}
	}
	return out
}

// plantedPage is w with the page at name set to text.
func (w docWorld) plantedPage(name, text string) docWorld {
	w.pages = maps.Clone(w.pages)
	w.pages[name] = text
	return w
}

// TestABoardsLinksToFilesThatAreThereAndToTheAppsRoutesPass is the control of
// the page-link plant: a link to a file that exists, a route of the app, an
// anchor and another site are not refused.
func TestABoardsLinksToFilesThatAreThereAndToTheAppsRoutesPass(t *testing.T) {
	t.Parallel()
	w := readDocWorld(t)
	page := `<a href="../README.md#x">a</a><a href="/rules">r</a><a href="#top">t</a><link href="https://example.org/a.css">`
	if got := pageLinks(w.plantedPage(canvasDirOfPages+"/X.dc.html", page)); len(got) != 0 {
		t.Errorf("refused: %q", got)
	}
}
