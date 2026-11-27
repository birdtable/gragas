package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// npmrcFiles lists the .npmrc files under dir, outside the folders no one
// tracks (the git folder, installed packages, the pinned tools).
func npmrcFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".tools":
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == ".npmrc" {
			out = append(out, filepath.ToSlash(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestNoNpmrcIsTrackedInTheRepository: the explorer's install settings are
// flags in the Makefile and keys in pnpm-workspace.yaml, in plain view, and a
// registry login has no file here to live in. Risk: a credential reference
// committed in a file the review cannot read.
func TestNoNpmrcIsTrackedInTheRepository(t *testing.T) {
	t.Parallel()
	if got := npmrcFiles(t, root); len(got) > 0 {
		t.Errorf("an .npmrc is in the tree: %s; put the setting in pnpm-workspace.yaml or the install command", strings.Join(got, ", "))
	}
}

// The check fires on a planted .npmrc, below a package folder too.
func TestAPlantedNpmrcIsFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	deep := filepath.Join(dir, "explorer", "packages", "x")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := npmrcFiles(t, dir); len(got) != 0 {
		t.Fatalf("the control finds %v", got)
	}
	if err := os.WriteFile(filepath.Join(deep, ".npmrc"), []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := npmrcFiles(t, dir); len(got) != 1 {
		t.Errorf("a planted .npmrc: found %v, want one", got)
	}
}
