package gatetest

import (
	"os"
	"path/filepath"
	"testing"
)

// TestShippedReadsTheLiveBookAloneNotEveryRulebookYamlOfTheTree: the live branch of Of
// returns the repository's root, which holds other rulebook.yaml files (testdata); Shipped
// opens the book's own folder, so format.Find sees one. Risk: a green converted branch and a
// red switch.
func TestShippedReadsTheLiveBookAloneNotEveryRulebookYamlOfTheTree(t *testing.T) {
	t.Parallel()
	converted := Of(t, filepath.Join("..", "..", ".."))
	root := t.TempDir()
	if err := os.CopyFS(filepath.Join(root, "rulebook"), converted.Rulebook); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "testdata", "other"), 0o750); err != nil {
		t.Fatal(err)
	}
	stray, err := os.ReadFile(filepath.Join(root, "rulebook", "rulebook.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "testdata", "other", "rulebook.yaml"), stray, 0o600); err != nil {
		t.Fatal(err)
	}
	// The live branch reads the generated lines of the committed presets below presets/:
	// the tree holds the repository's own.
	if err := os.CopyFS(filepath.Join(root, "presets"), os.DirFS(filepath.Join("..", "..", "..", "presets"))); err != nil {
		t.Fatal(err)
	}
	live := Of(t, root)
	live.Statements = converted.Statements // the copy is a converted book, not yet one a person reworded
	if live.Dir != root {
		t.Fatalf("Of took the converted branch over a tree with rulebook/rulebook.yaml: Dir %s", live.Dir)
	}
	if got := live.Shipped(t); len(got) < 20 {
		t.Errorf("the live branch ships %d rules, fewer than the rulebook holds", len(got))
	}
}
