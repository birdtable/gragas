package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/scan"
)

// TestPublicTextStartsTheCountAndOnlyEverLowersIt: with no count file the
// scan is red, and its apply starts the count at what the tree holds; a hit
// more is red and the apply leaves the count as it was; hits fewer are green
// and the apply lowers the count to them, and a hit that comes back after
// that is red, never counted anew.
func TestPublicTextStartsTheCountAndOnlyEverLowersIt(t *testing.T) {
	t.Parallel()
	dir := scanRepo(t)
	hit := "see owner/repo" + "#" + "12" + "\n"
	put(t, dir, "a.md", hit+hit)
	gitIn(t, dir, "add", "a.md")
	file := filepath.Join(dir, filepath.FromSlash(scan.PublicTextFile))
	read := func() string {
		b, err := os.ReadFile(file)
		if err != nil {
			return ""
		}
		return string(b)
	}
	scanWith := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := runScan(t.Context(), func(string) (string, bool) { return "", false }, os.Environ(), append([]string{"public-text", "-C", dir}, args...), &out, &errb)
		return code, out.String() + errb.String()
	}

	if code, _ := scanWith(); code != exitFailed || read() != "" {
		t.Fatalf("no count file: exit %d, file %q: want red and no file written", code, read())
	}
	if code, out := scanWith("--apply"); code != exitDone || !strings.Contains(read(), "count a.md 2\n") {
		t.Fatalf("the start: exit %d, file %q, output %q: want green and a.md counted at 2", code, read(), out)
	}
	if code, out := scanWith(); code != exitDone || strings.Contains(out, "under the count") {
		t.Fatalf("at the count: exit %d, output %q: want green", code, out)
	}

	put(t, dir, "a.md", hit+hit+hit)
	before := read()
	if code, out := scanWith("--apply"); code != exitFailed || read() != before || !strings.Contains(out, "a.md:3: tracker-number") {
		t.Fatalf("a hit more: exit %d, file changed %v, output %q: want red naming the line, and the count kept", code, read() != before, out)
	}

	put(t, dir, "a.md", hit)
	if code, out := scanWith(); code != exitDone || !strings.Contains(out, "run with --apply to lower it") || read() != before {
		t.Fatalf("a hit fewer: exit %d, output %q: want green, a note, and the file as it was", code, out)
	}
	if code, _ := scanWith("--apply"); code != exitDone || !strings.Contains(read(), "count a.md 1\n") {
		t.Fatalf("lowered: file %q: want a.md at 1", read())
	}

	put(t, dir, "a.md", "clean\n")
	if code, _ := scanWith("--apply"); code != exitDone || strings.Contains(read(), "a.md") {
		t.Fatalf("clean: file %q: want a.md out of the count", read())
	}
	put(t, dir, "a.md", hit)
	if code, _ := scanWith("--apply"); code != exitFailed || strings.Contains(read(), "a.md") {
		t.Fatalf("a hit back after a clean file: exit %d, file %q: want red, and no count for it", code, read())
	}
}

// TestPublicTextPrintsEveryHitWithList: --list prints each hit by file and
// line, also those under a count.
func TestPublicTextPrintsEveryHitWithList(t *testing.T) {
	t.Parallel()
	dir := scanRepo(t)
	put(t, dir, "a.md", "one\nGD"+"-"+"3\n")
	put(t, dir, scan.PublicTextFile, "count a.md 1\n")
	gitIn(t, dir, "add", "a.md", scan.PublicTextFile)
	var out, errb bytes.Buffer
	code := runScan(t.Context(), func(string) (string, bool) { return "", false }, os.Environ(), []string{"public-text", "--list", "-C", dir}, &out, &errb)
	if code != exitDone || !strings.Contains(out.String(), "  a.md:2: decision-id\n") {
		t.Errorf("exit %d, output %q, errors %q: want green and the hit listed", code, out.String(), errb.String())
	}
}
