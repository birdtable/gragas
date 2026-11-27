package main

import "testing"

// scanRepo is a git repository of the test's own, committed, with a
// .gitignore that ignores bin/ and a file in bin/ that is tracked all the
// same.
func scanRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	put(t, dir, "go.sum", "\n")
	put(t, dir, ".gitignore", "bin/\n")
	put(t, dir, "main.go", "package main\n")
	put(t, dir, "bin/kept.txt", "tracked though ignored\n")
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "add", "-f", "bin/kept.txt")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	return dir
}
