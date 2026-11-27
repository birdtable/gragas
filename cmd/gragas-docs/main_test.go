package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/cli/root"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/exit"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// repo is a folder with a README that has the markers and nothing else.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	readme := "# x\n\n" + root.ReadmeBegin + "\n\nold\n" + root.ReadmeEnd + "\n\ntail\n"
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, root.ReadmeFile), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	put(t, dir, archFile, "# a\n\nProse.\n\n## 17. v6\n\n"+plannedOpen+"\n"+plannedClose+"\n")
	put(t, dir, layersFile, "planned:\n  - created: M1\n    layer: core\n    packages: [internal/core/format, internal/core/contract]\n")
	return dir
}

// countOfFiles is the count line the gate runner reads, of files,
// as the last line, and not 0.
var countOfFiles = regexp.MustCompile(`(^|\n)count: [1-9][0-9]* files\n$`)

func runIn(t *testing.T, args ...string) (code int, out, errs string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(t.Context(), args, &o, &e)
	return code, o.String(), e.String()
}

func TestGenWritesTheDocumentsAndCheckPassesOnThemAndFailsOnADrift(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	// Stale: --check tells every document and writes nothing.
	code, _, errs := runIn(t, "--check", "-C", dir)
	if code != exit.Failed || !strings.Contains(errs, root.CommandsDoc+" differs") {
		t.Fatalf("--check on a bare repository = %d %q, want failed naming the command table", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, root.CommandsDoc)); err == nil {
		t.Fatal("--check wrote a file")
	}
	code, out, _ := runIn(t, "-C", dir)
	if code != exit.Done || !strings.Contains(out, "wrote "+root.TreeFile) {
		t.Fatalf("generate = %d %q", code, out)
	}
	readme, _ := os.ReadFile(filepath.Join(dir, root.ReadmeFile))
	if !strings.Contains(string(readme), "| `gragas check` |") || !strings.HasSuffix(string(readme), "\n\ntail\n") || strings.Contains(string(readme), "\nold\n") {
		t.Errorf("the README was not spliced between its markers:\n%s", readme)
	}
	// It ends on the count the gate runner reads: the documents it
	// compared, never none.
	if code, out, errs := runIn(t, "--check", "-C", dir); code != exit.Done || !countOfFiles.MatchString(out) {
		t.Errorf("--check on fresh documents = %d %q %q, want done and a count of the files compared", code, out, errs)
	}
	// A hand edit of a generated document is told.
	if err := os.WriteFile(filepath.Join(dir, root.CommandsDoc), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runIn(t, "--check", "-C", dir); code != exit.Failed || !strings.Contains(errs, root.CommandsDoc) {
		t.Errorf("--check on a hand edit = %d %q", code, errs)
	}
}

func TestAReadmeWithoutMarkersAndAnUnknownFlagFail(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, root.ReadmeFile), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runIn(t, "-C", dir); code != exit.Failed || !strings.Contains(errs, "markers") {
		t.Errorf("a README with no markers = %d %q", code, errs)
	}
	if code, _, _ := runIn(t, "--nope"); code != exit.Usage {
		t.Errorf("an unknown flag = %d, want usage", code)
	}
}

// TestALinkOutOfTheTreeIsNeitherReadNorWritten: a docs folder that is a link
// to a folder outside the repository is refused, and nothing is written
// there. A README that is such a link is refused too, beside a real docs
// folder; the README is one of the documents too, so were its first
// read made by path, its read as a document would still refuse it.
// Planted failure: the documents read and written by path, not through
// the root.
func TestALinkOutOfTheTreeIsNeitherReadNorWritten(t *testing.T) {
	t.Parallel()
	dir, outside := repo(t), t.TempDir()
	if err := os.RemoveAll(filepath.Join(dir, "docs")); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, outside, filepath.Join(dir, "docs"))
	if code, _, errs := runIn(t, "-C", dir); code != exit.Failed || !strings.Contains(errs, "escapes") {
		t.Errorf("a docs folder linked out of the tree = %d %q, want failed", code, errs)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote %d files outside the tree", len(entries))
	}
	if err := os.Remove(filepath.Join(dir, "docs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.Rename(filepath.Join(dir, root.ReadmeFile), filepath.Join(other, root.ReadmeFile)); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, filepath.Join(other, root.ReadmeFile), filepath.Join(dir, root.ReadmeFile))
	if code, _, errs := runIn(t, "--check", "-C", dir); code != exit.Failed || !strings.Contains(errs, "escapes") {
		t.Errorf("a README linked out of the tree = %d %q, want failed", code, errs)
	}
}

// TestALinkedSourceOfThePlannedDocumentIsRefused: the architecture and
// layers.yaml, which the planned-packages block is made from, are read through
// the root too, so each, linked to a copy outside the repository, is refused
// and not read. Planted failure: either read made by path.
func TestALinkedSourceOfThePlannedDocumentIsRefused(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	for _, rel := range []string{archFile, layersFile} {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			dir, outside := repo(t), filepath.Join(t.TempDir(), "copy")
			at := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.Rename(at, outside); err != nil {
				t.Fatal(err)
			}
			testenv.Link(t, outside, at)
			tree, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tree.Close() }()
			if _, err := plannedDocuments(tree); err == nil || !strings.Contains(err.Error(), "escapes") {
				t.Errorf("the planned document with %s linked out of the tree: %v, want refused", rel, err)
			}
			if code, _, errs := runIn(t, "--check", "-C", dir); code != exit.Failed || !strings.Contains(errs, "escapes") {
				t.Errorf("%s linked out of the tree = %d %q, want failed", rel, code, errs)
			}
		})
	}
}
