package fs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/port"
)

// refusedPaths name something outside a root, or nothing a root holds: the
// table testdata/refused-paths.txt, and a NUL, alone and inside a name.
func refusedPaths(tb testing.TB) []string {
	tb.Helper()
	return append(table(tb, "refused-paths"), "\x00", "a\x00b")
}

// takenPaths are clean relative paths inside a root, testdata/taken-paths.txt.
func takenPaths(tb testing.TB) []string {
	tb.Helper()
	return table(tb, "taken-paths")
}

// table reads testdata/<name>.txt: one case a line, a line that starts with #
// a comment, and \\, \n and \t read as a backslash, a newline and a tab.
func table(tb testing.TB, name string) []string {
	tb.Helper()
	data, err := fs.ReadFile(os.DirFS("testdata"), name+".txt")
	if err != nil {
		tb.Fatal(err)
	}
	unescape := strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\t`, "\t")
	var out []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, unescape.Replace(line))
	}
	return out
}

// TestConfineRefusesWhatLeavesTheRootAndTakesTheRest (store.state-files.8):
// climbing, absolute, drive-style, a stream, a backslash, an unclean path
// and a NUL are each refused as ErrOutside; a clean relative path is taken
// in this system's form, and the empty path is the root.
func TestConfineRefusesWhatLeavesTheRootAndTakesTheRest(t *testing.T) {
	t.Parallel()
	refused, taken := refusedPaths(t), takenPaths(t)
	if len(refused) == 0 || len(taken) == 0 {
		t.Fatal("no path to check")
	}
	for _, p := range refused {
		if got, err := hostfs.Confine(p); !errors.Is(err, port.ErrOutside) {
			t.Errorf("Confine(%q) = %q, %v, want ErrOutside", p, got, err)
		}
	}
	for _, p := range taken {
		if got, err := hostfs.Confine(p); err != nil || got != filepath.FromSlash(p) {
			t.Errorf("Confine(%q) = %q, %v, want it taken", p, got, err)
		}
	}
	if got, err := hostfs.Confine(""); err != nil || got != "." {
		t.Errorf("Confine(\"\") = %q, %v, want the root", got, err)
	}
}

// FuzzConfine (tests.generated.1): whatever confine takes is local to the
// root on this system, holds no backslash, colon or NUL, and is the slash
// path it was given; whatever it refuses matches ErrOutside.
func FuzzConfine(f *testing.F) {
	for _, p := range append(append(refusedPaths(f), takenPaths(f)...), "", "a/b/../c", "CON", "a/\u2215b") {
		f.Add(p)
	}
	f.Fuzz(func(t *testing.T, p string) {
		got, err := hostfs.Confine(p)
		if err != nil {
			if !errors.Is(err, port.ErrOutside) {
				t.Errorf("Confine(%q) refused with %v, which is not ErrOutside", p, err)
			}
			return
		}
		if p == "" {
			if got != "." {
				t.Errorf("Confine(\"\") = %q, want .", got)
			}
			return
		}
		if !filepath.IsLocal(got) || strings.ContainsAny(got, ":\x00") || strings.ContainsRune(p, '\\') {
			t.Errorf("Confine(%q) = %q, which is not local to the root", p, got)
		}
		if filepath.ToSlash(got) != p || filepath.Clean(got) != got {
			t.Errorf("Confine(%q) = %q, which is not the path given, clean", p, got)
		}
		for part := range strings.SplitSeq(p, "/") {
			if part == ".." || part == "." || part == "" {
				t.Errorf("Confine(%q) took the element %q", p, part)
			}
		}
	})
}
