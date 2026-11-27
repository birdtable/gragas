package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wideModule is a module of five packages: core, which a, b and c import (more than half), and a leaf.
func wideModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	put(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
	put(t, dir, "core/core.go", "package core\n\nconst X = 1\n")
	for _, p := range []string{"a", "b", "c"} {
		put(t, dir, p+"/"+p+".go", "package "+p+"\n\nimport \"example.com/m/core\"\n\nconst Y = core.X\n")
	}
	put(t, dir, "leaf/leaf.go", "package leaf\n\nconst Z = 1\n")
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// wideAfter changes one file of a fresh module on a branch and answers what wide prints against main.
func wideAfter(t *testing.T, change string) (int, string, string) {
	t.Helper()
	dir := wideModule(t)
	git(t, dir, "checkout", "-q", "-b", "work")
	was, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(change)))
	if err != nil {
		t.Fatal(err)
	}
	put(t, dir, change, string(was)+"\n// changed\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "work")
	var out, errs strings.Builder
	code := runWide(t.Context(), os.Environ(), []string{"-C", dir, "main"}, &out, &errs)
	return code, out.String(), errs.String()
}

// TestAWidelyImportedPackageIsToldAndALeafIsNot changes the package three others
// import: the line names it and the count of the total. A go.mod change reaches all five, and the same
// change to a leaf prints nothing. It runs the Go command, which needs no sh.
func TestAWidelyImportedPackageIsToldAndALeafIsNot(t *testing.T) {
	t.Parallel()
	code, out, errs := wideAfter(t, "core/core.go")
	if code != exitDone || errs != "" {
		t.Fatalf("a change to the widely imported package: exit %d, %q", code, errs)
	}
	for _, want := range []string{"example.com/m/core", "3 of 5 packages"} {
		if !strings.Contains(out, want) {
			t.Errorf("the line lacks %q:\n%s", want, out)
		}
	}
	if _, out, _ := wideAfter(t, "go.mod"); !strings.Contains(out, "go.mod or go.sum is a dependency of 5 of 5 packages") {
		t.Errorf("a change to go.mod re-runs every package and the line does not say so:\n%s", out)
	}
	if code, out, errs := wideAfter(t, "leaf/leaf.go"); code != exitDone || out != "" || errs != "" {
		t.Errorf("a change to a leaf: exit %d, printed %q %q", code, out, errs)
	}
	var out2, errs2 strings.Builder
	if code := runWide(t.Context(), os.Environ(), nil, &out2, &errs2); code != exitUsage {
		t.Errorf("no base: exit %d, want usage", code)
	}
}
