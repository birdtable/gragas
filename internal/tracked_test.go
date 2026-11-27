package internal

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// trackedIn is every file git tracks in dir, slash-separated, as git
// ls-files lists them, a file deleted from disk but not from the index
// included. A tree with no .git, an export, holds only tracked files, so
// there every file on disk is one. A listing that fails ends the test. It is
// the docs checks' own (internal/repo/docs, where its plants are), kept here
// for the lint runners' check when the docs checks left this package
// (gragas#1194).
func trackedIn(t *testing.T, dir string) []string {
	t.Helper()
	out, err := listTracked(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// listTracked is trackedIn without a test: the error is returned, so the
// caller fails on its own t (#735).
func listTracked(ctx context.Context, dir string) ([]string, error) {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
		cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
		cmd.Dir = dir
		cmd.Env = append(configtest.Inherited(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("list the files git tracks in %s: %w", dir, err)
		}
		return strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 }), nil
	}
	// An export walks the root, and no tracked file is under a node_modules
	// folder, which explorer-test rewrites: reading it would break Go's test
	// cache for this package (walks_node_modules_test.go).
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		out = append(out, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list the files of %s: %w", dir, err)
	}
	return out, nil
}
