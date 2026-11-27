package proof_test

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/proof"
)

// withinCase is one line of testdata/within_paths.txt.
type withinCase struct{ name, dir, path, want string }

// withinCases reads testdata/within_paths.txt: a claim, a folder, a path and
// what it is read as, split by tabs; \\ is a backslash and a folder of
// "(empty)" is the empty string. The rows live in testdata because they spell
// paths of other systems, which no Go literal of this tree holds.
func withinCases(tb testing.TB) []withinCase {
	tb.Helper()
	data, err := fs.ReadFile(os.DirFS("testdata"), "within_paths.txt")
	if err != nil {
		tb.Fatal(err)
	}
	unescape := strings.NewReplacer(`\\`, `\`).Replace
	var out []withinCase
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			tb.Fatalf("within_paths.txt: %q has %d fields, want 4", line, len(f))
		}
		c := withinCase{name: f[0], dir: unescape(f[1]), path: unescape(f[2]), want: f[3]}
		if c.dir == "(empty)" {
			c.dir = ""
		}
		out = append(out, c)
	}
	return out
}

func TestAFindingIsReadInsideTheFolderInEverySystemsSpellingAndRefusedOutsideIt(t *testing.T) {
	t.Parallel()
	cases := withinCases(t)
	if len(cases) == 0 {
		t.Fatal("within_paths.txt holds no case")
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := proof.Within(c.dir, []string{c.path})
			if c.want == "refused" {
				if !errors.Is(err, proof.ErrOutput) || got != nil {
					t.Errorf("Within(%q, %q) = %q, %v, want ErrOutput", c.dir, c.path, got, err)
				}
				return
			}
			if err != nil || !slices.Equal(got, []string{c.want}) {
				t.Errorf("Within(%q, %q) = %q, %v, want %q", c.dir, c.path, got, err, c.want)
			}
		})
	}
}

func TestOneFindingOutsideTheFolderRefusesTheWholeOutput(t *testing.T) {
	t.Parallel()
	for _, c := range withinCases(t) {
		if c.want == "refused" || c.dir == "" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := proof.Within(c.dir, []string{c.path, c.dir + "/../x.go"})
			if !errors.Is(err, proof.ErrOutput) || got != nil {
				t.Errorf("Within(%q, [%q, a climb]) = %q, %v, want ErrOutput", c.dir, c.path, got, err)
			}
		})
	}
}
