package fs_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/port"
)

// The two tests below hold the host's own Tree.Within and Tree.Written to
// links, on every system that lets this account make one (testenv.Link
// skips where it cannot). Each path past a link is joined by hand with the
// system's separator, never by filepath.Join, which would clean a ".." away
// before the link it follows is read.

// TestWithinFollowsALinkBothWays: a host path is placed in a root by where
// its links lead, as the host would write it. A link from outside to a folder
// inside answers the file below it; a link inside that leads out, to a file
// or to nothing, is outside. A link from outside to a file inside is a link
// at the last part, ErrLinked (TestWithinRefusesALinkAsTheLastPart). Risk: an agent's edit
// through a link is judged as the wrong file, or as no file of the rulebook's.
func TestWithinFollowsALinkBothWays(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir, away := filepath.Join(base, "book"), filepath.Join(base, "away")
	write(t, dir, map[string]string{"rules/a.yaml": "a: 1\n"})
	write(t, away, map[string]string{"x.yaml": "x: 1\n", "sub/y.yaml": "y\n"})
	for link, target := range map[string]string{
		filepath.Join(away, "in"):     filepath.Join(dir, "rules", "a.yaml"),
		filepath.Join(away, "rules"):  filepath.Join(dir, "rules"),
		filepath.Join(dir, "out"):     filepath.Join(away, "x.yaml"),
		filepath.Join(dir, "outdir"):  filepath.Join(away, "sub"),
		filepath.Join(dir, "dangle"):  filepath.Join(away, "missing.yaml"),
		filepath.Join(dir, "rel-out"): filepath.Join("..", "away", "x.yaml"),
	} {
		testenv.Link(t, target, link)
	}
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for abs, want := range map[string]string{
		filepath.Join(away, "rules") + filepath.FromSlash("/a.yaml"):   "rules/a.yaml",
		filepath.Join(away, "rules") + filepath.FromSlash("/new.yaml"): "rules/new.yaml",
		filepath.Join(dir, "out"):                                      "",
		filepath.Join(dir, "outdir") + filepath.FromSlash("/y.yaml"):   "",
		filepath.Join(dir, "outdir") + filepath.FromSlash("/new.yaml"): "",
		filepath.Join(dir, "dangle"):                                   "",
		filepath.Join(dir, "rel-out"):                                  "",
	} {
		got, err := tr.Within(root, abs)
		switch {
		case want == "" && err == nil:
			t.Errorf("Within(%q) = %q, want it outside or refused", abs, got)
		case want != "" && (err != nil || got != want):
			t.Errorf("Within(%q) = %q, %v, want %q", abs, got, err, want)
		}
	}
	if got, err := tr.Within(root, filepath.Join(away, "in")); !errors.Is(err, port.ErrLinked) {
		t.Errorf("Within(away/in) = %q, %v, want ErrLinked", got, err)
	}
	for _, abs := range []string{filepath.Join(dir, "out"), filepath.Join(dir, "dangle"), filepath.Join(dir, "outdir") + filepath.FromSlash("/new.yaml")} {
		if _, err := tr.Within(root, abs); !errors.Is(err, port.ErrOutside) {
			t.Errorf("Within(%q) = %v, want ErrOutside", abs, err)
		}
	}
}

// TestWithinRefusesAPathThatIsNotClean: a probe, on
// the disk. Planted: with book/l a link to book/x/y, the kernel reads
// book/l/../rules/a.yaml as x/rules/a.yaml and a tool that cleans the
// spelling as rules/a.yaml; Within places it nowhere and answers
// ErrNotClean, as it does a .. after a link that leads out, a .. below a
// part that does not exist, a . part, a doubled separator and one at the
// end. Control: a clean path through the same link answers where it leads.
// Risk: the guard judges the file the kernel reaches while the host writes
// the one the spelling names.
func TestWithinRefusesAPathThatIsNotClean(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir, away := filepath.Join(base, "book"), filepath.Join(base, "away")
	write(t, dir, map[string]string{"rules/a.yaml": "a: 1\n", "x/rules/a.yaml": "a: 2\n", "x/y/b.yaml": "b: 1\n"})
	write(t, away, map[string]string{"x.yaml": "x: 1\n", "sub/y.yaml": "y\n"})
	testenv.Link(t, filepath.Join(dir, "x", "y"), filepath.Join(dir, "l"))
	testenv.Link(t, filepath.Join(away, "sub"), filepath.Join(dir, "outdir"))
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := tr.Within(root, filepath.Join(dir, "l")+filepath.FromSlash("/b.yaml")); err != nil || got != "x/y/b.yaml" {
		t.Errorf("Within(l/b.yaml) = %q, %v, want x/y/b.yaml", got, err)
	}
	for _, below := range []string{"/l/../rules/a.yaml", "/outdir/../x.yaml", "/outdir/../rules/a.yaml", "/missing/../rules/a.yaml", "/./rules/a.yaml", "//rules/a.yaml", "/rules/"} {
		abs := dir + filepath.FromSlash(below)
		if got, err := tr.Within(root, abs); !errors.Is(err, port.ErrNotClean) {
			t.Errorf("Within(%q) = %q, %v, want ErrNotClean", abs, got, err)
		}
	}
}

// TestWithinRefusesALinkAsTheLastPart: on the disk. Planted: with book/rules/ln.yaml a link to
// book/x/rules/a.yaml, Within placed it at x/rules/a.yaml, while a tool that
// writes a new file and renames it over replaces the link at rules/ln.yaml.
// Within places it nowhere and answers ErrLinked, naming x/rules/a.yaml, as
// it does a relative link, a link to a link, a link to a missing file inside,
// a link to a folder, and a link from outside to a file inside. Controls: the
// file the link leads to, a file through a folder link, and a new file beside
// the link answer where they are. Risk: the guard judges the edit at the
// file the link leads to, and the host writes a new file at the link's name.
func TestWithinRefusesALinkAsTheLastPart(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir, away := filepath.Join(base, "book"), filepath.Join(base, "away")
	write(t, dir, map[string]string{"rules/a.yaml": "a: 1\n", "x/rules/a.yaml": "a: 2\n", "x/y/b.yaml": "b: 1\n"})
	write(t, away, map[string]string{"x.yaml": "x: 1\n"})
	target := filepath.Join(dir, "x", "rules", "a.yaml")
	testenv.Link(t, target, filepath.Join(dir, "rules", "ln.yaml"))
	testenv.Link(t, filepath.Join("..", "x", "rules", "a.yaml"), filepath.Join(dir, "rules", "rel.yaml"))
	testenv.Link(t, filepath.Join(dir, "rules", "ln.yaml"), filepath.Join(dir, "rules", "twice.yaml"))
	testenv.Link(t, filepath.Join(dir, "x", "rules", "gone.yaml"), filepath.Join(dir, "rules", "gone.yaml"))
	testenv.Link(t, filepath.Join(dir, "x", "y"), filepath.Join(dir, "l"))
	testenv.Link(t, target, filepath.Join(away, "in"))
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for abs, leads := range map[string]string{
		filepath.Join(dir, "rules", "ln.yaml"):    "x/rules/a.yaml",
		filepath.Join(dir, "rules", "rel.yaml"):   "x/rules/a.yaml",
		filepath.Join(dir, "rules", "twice.yaml"): "x/rules/a.yaml",
		filepath.Join(dir, "rules", "gone.yaml"):  "x/rules/gone.yaml",
		filepath.Join(dir, "l"):                   "x/y",
		filepath.Join(away, "in"):                 "x/rules/a.yaml",
	} {
		got, err := tr.Within(root, abs)
		if !errors.Is(err, port.ErrLinked) || got != "" {
			t.Errorf("Within(%q) = %q, %v, want ErrLinked", abs, got, err)
			continue
		}
		if !strings.Contains(err.Error(), leads) {
			t.Errorf("Within(%q): %v, which does not name %s", abs, err, leads)
		}
	}
	for abs, want := range map[string]string{
		target: "x/rules/a.yaml",
		filepath.Join(dir, "l") + filepath.FromSlash("/b.yaml"): "x/y/b.yaml",
		filepath.Join(dir, "rules", "new.yaml"):                 "rules/new.yaml",
	} {
		if got, err := tr.Within(root, abs); err != nil || got != want {
			t.Errorf("control: Within(%q) = %q, %v, want %q", abs, got, err, want)
		}
	}
}

// TestWrittenResolvesTheRootAloneKeepingTheRestAsWritten: a link that leads
// to the root is the root, and every part below it keeps the spelling the
// path gives it, a link below the root unfollowed; a link from outside to a
// folder inside is no root, and outside. A path that is not clean is
// ErrNotClean, as for Within (the
// guard never hands it one, and no .. is applied by its name alone any
// more). Risk: the guard holds the disk's
// spelling of a part to the names, not the edit's, and a host that folds
// case writes a file the guard read under another name (as found on
// Windows).
func TestWrittenResolvesTheRootAloneKeepingTheRestAsWritten(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir, away := filepath.Join(base, "book"), filepath.Join(base, "away")
	write(t, dir, map[string]string{"rules/a.yaml": "a: 1\n"})
	write(t, away, map[string]string{"x.yaml": "x: 1\n"})
	testenv.Link(t, dir, filepath.Join(base, "alias"))
	testenv.Link(t, filepath.Join(dir, "rules"), filepath.Join(away, "rules"))
	testenv.Link(t, away, filepath.Join(dir, "outdir"))
	tr := newTree(t)
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for abs, want := range map[string]string{
		filepath.Join(base, "alias") + filepath.FromSlash("/Rules/a.yaml"): "Rules/a.yaml",
		filepath.Join(dir, "outdir") + filepath.FromSlash("/x.yaml"):       "outdir/x.yaml",
	} {
		if got, err := tr.Written(root, abs); err != nil || got != want {
			t.Errorf("Written(%q) = %q, %v, want %q", abs, got, err, want)
		}
	}
	for _, abs := range []string{filepath.Join(away, "rules") + filepath.FromSlash("/a.yaml"), filepath.Join(away, "x.yaml"), dir} {
		if got, err := tr.Written(root, abs); err == nil {
			t.Errorf("Written(%q) = %q, want it outside or refused", abs, got)
		}
	}
	for _, below := range []string{"/outdir/../rules/a.yaml", "/x/../Rules/b.yaml", "/./rules/a.yaml", "//rules/a.yaml", "/rules/"} {
		abs := dir + filepath.FromSlash(below)
		if got, err := tr.Written(root, abs); !errors.Is(err, port.ErrNotClean) {
			t.Errorf("Written(%q) = %q, %v, want ErrNotClean", abs, got, err)
		}
	}
}
