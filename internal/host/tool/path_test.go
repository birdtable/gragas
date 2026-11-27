package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/tool"
)

// The words every path that is not an absolute, clean, slash-separated path
// is refused with.
const notAbsClean = "is not an absolute, clean path"

// absHere is the answer a path absolute on this system gets, by filepath's
// own word, and otherwise the refusal it is given: so each row holds on
// every system with no branch on the system.
func absHere(p, otherwise string) string {
	if filepath.IsAbs(p) {
		return ""
	}
	return otherwise
}

// TestAPathIsAbsoluteByThisSystemsOwnWordAndClean: a path the port takes is
// absolute by filepath.IsAbs and its volume, never by a leading / alone, and
// clean in its slash form. Some paths are refused the same way everywhere;
// the rest are answered as filepath judges them here: on unix /repo is
// absolute, and on Windows it is rooted on no volume and refused by name, as
// C:repo is one with a volume and no root.
// Risk: a path absolute on one system is taken on another where it is
// relative to a current folder, and the adapter runs what lies there.
func TestAPathIsAbsoluteByThisSystemsOwnWordAndClean(t *testing.T) {
	t.Parallel()
	here := filepath.ToSlash(t.TempDir())
	for p, want := range map[string]string{
		// The same on every system.
		"repo":               notAbsClean,
		"./repo":             notAbsClean,
		"//host/share/repo/": notAbsClean,
		// A folder of this system's, absolute here, and its unclean forms.
		here:             "",
		here + "/a b/c":  "",
		here + "/a/../b": notAbsClean,
		here + "/a/./b":  notAbsClean,
		here + "/a/":     notAbsClean,
		here + "//b":     notAbsClean,
	} {
		if got := tool.PathFault(p); got != want {
			t.Errorf("%q: %q; want %q", p, got, want)
		}
	}
	// The rows that write a volume, a backslash or a rooted path are the
	// table's, not code's; unc is clean and absolute only where a share is a volume.
	for _, r := range tableRows(t, "testdata/pathfault.txt") {
		p := r[1]
		want := map[string]string{
			"same":   notAbsClean,
			"abs":    absHere(p, notAbsClean),
			"absc":   absHere(p, notAbsClean),
			"rooted": notAbsClean,
			"volume": notAbsClean,
			"unc":    either(filepath.ToSlash(filepath.Clean(p)) == p, absHere(p, notAbsClean), notAbsClean),
		}[r[0]]
		if got := tool.PathFault(p); got != want {
			t.Errorf("%q (%s): %q; want %q", p, r[0], got, want)
		}
	}
	if got := tool.PathFault(""); got != notAbsClean {
		t.Errorf("no path: %q; want it not absolute and clean", got)
	}
}

// TestThePinnedPathLeavesOutAFolderThatHoldsTheSeparator: a
// folder whose name holds the list separator is left out, so the plain join
// of what is kept gives back exactly the folders kept and no relative one.
// Planted: the check taken out, and the name is kept.
// Risk: a quoted element with a separator is read as one folder, kept, and
// joined into two, the second relative to the folder the run is in.
// What this does not prove: the split of a quoted element is Windows's alone
// and is not run here.
func TestThePinnedPathLeavesOutAFolderThatHoldsTheSeparator(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() + string(filepath.ListSeparator) + "b"
	if got, ok := tool.PathFolder(dir); ok || got != "" {
		t.Errorf("%q: %q, %v; want it left out", dir, got, ok)
	}
}

// TestThePinnedPathLeavesOutAFolderThatHoldsAQuote: a folder
// whose name holds a double quote is left out, as a quoted PATH element is
// read with its quotes removed.
// Planted: the check taken out, and the name is kept.
func TestThePinnedPathLeavesOutAFolderThatHoldsAQuote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() + "\"b"
	if got, ok := tool.PathFolder(dir); ok || got != "" {
		t.Errorf("%q: %q, %v; want it left out", dir, got, ok)
	}
}

// TestAPinnedFindKeepsAPATHFolderAbsoluteHereWithNoDotDot: a folder of the
// PATH a pinned find may keep is absolute by filepath.IsAbs, has no ..
// segment, and is compared in its clean slash form.
// Risk: a pinned find keeps a folder that is relative here, or one a link
// before its .. reaches through, and env finds the project's program there.
func TestAPinnedFindKeepsAPATHFolderAbsoluteHereWithNoDotDot(t *testing.T) {
	t.Parallel()
	here := t.TempDir()
	slash := filepath.ToSlash(here)
	sep := "/"
	type kept struct {
		slash string
		ok    bool
	}
	keptHere := func(dir, slash string) kept {
		if filepath.IsAbs(dir) {
			return kept{slash, true}
		}
		return kept{}
	}
	for dir, want := range map[string]kept{
		// The same on every system.
		"bin":   {},
		"./bin": {},
		// A folder of this system's, and its forms.
		here:                            {slash, true},
		here + sep + "bin" + sep:        {slash + sep + "bin", true},
		here + sep + sep + "bin":        {slash + sep + "bin", true},
		here + sep + "." + sep + "bin":  {slash + sep + "bin", true},
		here + sep + ".." + sep + "x":   {},
		here + sep + "bin" + sep + "..": {},
	} {
		if s, ok := tool.PathFolder(dir); s != want.slash || ok != want.ok {
			t.Errorf("%q: %q, %v; want %q, %v", dir, s, ok, want.slash, want.ok)
		}
	}
	// The rows that write a volume, a backslash or a rooted path are the
	// table's, not code's; \tools is a plant: rooted with no volume is not absolute.
	for _, r := range tableRows(t, "testdata/pathfolder.txt") {
		want := kept{}
		if r[0] == "kept" {
			want = keptHere(r[1], r[2])
		}
		if s, ok := tool.PathFolder(r[1]); s != want.slash || ok != want.ok {
			t.Errorf("%q: %q, %v; want %q, %v", r[1], s, ok, want.slash, want.ok)
		}
	}
}

// tableRows reads a testdata table: a line is its tab-separated columns, and a
// line that starts with # is a note.
func tableRows(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	for l := range strings.SplitSeq(string(b), "\n") {
		if l != "" && !strings.HasPrefix(l, "#") {
			rows = append(rows, strings.Split(l, "\t"))
		}
	}
	return rows
}

// either is yes when cond holds and no otherwise.
func either(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
