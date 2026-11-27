package main

import (
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/surface"
)

// TestATagNamedOriginMainIsNeverTheBase (gragas#589): git resolves a bare
// origin/main as refs/tags/origin/main before refs/remotes/origin/main, so a
// tag of that name, which anyone who can push a tag can fetch into a clone,
// would choose the base the surface marks are held to. In a repository of the
// test's own, the remote branch holds the branch's marks, and a tag named
// origin/main a commit holding the tag's: the read of the base returns the
// branch's. Planted failure: baseRef or releaseBaseRef spelled as the bare
// name.
func TestATagNamedOriginMainIsNeverTheBase(t *testing.T) {
	t.Parallel()
	files := func(whose string) map[string]string {
		return map[string]string{
			surface.MarksFile: "# " + whose + "\n",
			"a.go":            "package a // " + whose + "\n",
		}
	}
	dir := fixture(t, files("the branch's"))
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "the branch")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	for p, data := range files("the tag's") {
		write(t, dir, p, data)
	}
	git(t, dir, "commit", "-q", "-am", "the tag")
	git(t, dir, "tag", "origin/main", "HEAD")
	git(t, dir, "reset", "-q", "--hard", "refs/remotes/origin/main")

	got, err := gitBaseMarks(t.Context(), os.Environ(), dir, baseRef)
	if err != nil || !strings.Contains(string(got), "the branch's") {
		t.Errorf("the surface marks file at the base = %q, %v; want the remote branch's, not the tag's", got, err)
	}
}
