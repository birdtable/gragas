package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/exit"
)

func page(typ, forLine string) string {
	s := "---\ntype: " + typ + "\nstatus: current\nas-of: 2026-11-09\n"
	if forLine != "" {
		s += "for: " + forLine + "\n"
	}
	return s + "---\n\n# a page\n"
}

// TestAPageWithAForLineIsARowInItsPlace: the table is written from the pages'
// type: and for:, in path order, with the two generated pages' rows after
// them, and the prose around the block stays.
func TestAPageWithAForLineIsARowInItsPlace(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	put(t, dir, "docs/zeta.md", page("reference", "Zeta things"))
	put(t, dir, "docs/boards/README.md", page("reference", "What a board shows"))
	put(t, dir, "docs/alpha.md", page("spec", "Alpha things"))
	put(t, dir, "docs/quiet.md", page("reference", ""))
	if code, _, errs := runIn(t, "-C", dir); code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	text := read(t, dir, agentsFile)
	var at []int
	for _, row := range []string{"| Alpha things | `docs/alpha.md` |", "| What a board shows | `docs/boards/README.md` |", "| Zeta things | `docs/zeta.md` |", "| The commands, their contexts and closing words | `docs/commands.md` |", "`docs/layers.md`, from `layers.yaml` |"} {
		i := strings.Index(text, row)
		if i < 0 {
			t.Fatalf("no row %q in\n%s", row, text)
		}
		at = append(at, i)
	}
	for i := 1; i < len(at); i++ {
		if at[i-1] > at[i] {
			t.Errorf("the rows are out of order: %v\n%s", at, text)
		}
	}
	if strings.Contains(text, "quiet.md") {
		t.Errorf("a page with no for: line is a row:\n%s", text)
	}
	if !strings.Contains(text, "## Before a push") || !strings.Contains(text, "# Working") {
		t.Errorf("the prose around the block was lost:\n%s", text)
	}
	if code, _, errs := runIn(t, "-C", dir, "--check"); code != 0 {
		t.Errorf("--check on what was just written: exit %d: %s", code, errs)
	}
	put(t, dir, "docs/late.md", page("reference", "A late page"))
	if code, _, errs := runIn(t, "-C", dir, "--check"); code == 0 || !strings.Contains(errs, agentsFile) {
		t.Errorf("a page the table lacks was not told: exit %d: %s", code, errs)
	}
}

// TestAForLineNeedsATypeAndTheBlockNeedsItsMarkers: a page that says what it is
// for but not what it is, and an AGENTS.md with no block, are told and write
// nothing.
func TestAForLineNeedsATypeAndTheBlockNeedsItsMarkers(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	put(t, dir, "docs/untyped.md", "---\nstatus: current\nfor: Something\n---\n\n# u\n")
	if code, _, errs := runIn(t, "-C", dir); code == 0 || !strings.Contains(errs, "needs a type") {
		t.Errorf("an untyped page: exit %d: %q", code, errs)
	}
	dir = repo(t)
	put(t, dir, agentsFile, "# Working\n")
	if code, _, errs := runIn(t, "-C", dir); code == 0 || !strings.Contains(errs, "has no block marked") {
		t.Errorf("no block: exit %d: %q", code, errs)
	}
}

// TestALinkedPageOrAgentsFileIsRefusedByTheTable: the table's pages and the
// AGENTS.md it is written into are read through the root, so a page under
// docs/, or AGENTS.md, linked to a copy outside the repository is refused and
// not read. Planted failure: the walk, a page's read or the AGENTS.md read
// made by path.
func TestALinkedPageOrAgentsFileIsRefusedByTheTable(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	for _, rel := range []string{"docs/linked.md", agentsFile} {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			dir, outside := repo(t), filepath.Join(t.TempDir(), "copy")
			put(t, dir, "docs/linked.md", page("reference", "Linked things"))
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
			if _, err := whereDocuments(tree); err == nil || !strings.Contains(err.Error(), "escapes") {
				t.Errorf("the table with %s linked out of the tree: %v, want refused", rel, err)
			}
			if code, _, errs := runIn(t, "--check", "-C", dir); code != exit.Failed || !strings.Contains(errs, "escapes") {
				t.Errorf("%s linked out of the tree = %d %q, want failed", rel, code, errs)
			}
		})
	}
}
