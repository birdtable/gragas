package main

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/dev/join"
	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/surface"
)

// A generator reads and writes the repository through one os.Root, so a link
// on a path it uses, committed in a head the base's generator is run over,
// leads nowhere: the read or the write is refused and the folder the link
// points at stays as it was (#509).

// outsideWith makes a folder outside the repository, with one file in it.
func outsideWith(t *testing.T, name, text string) string {
	t.Helper()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}

func link(t *testing.T, target, at string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, target, at)
}

// joinTree is a repository with what gen commands reads: the table and one
// command package, and the folder its file lands in.
func joinTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	put := func(rel, from string) {
		data, err := os.ReadFile(filepath.Join("..", "..", from))
		if err != nil {
			t.Fatal(err)
		}
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(layers.File, layers.File)
	put("internal/cli/cmd/version/command.go", "internal/cli/cmd/version/command.go")
	if err := os.MkdirAll(filepath.Join(dir, "internal/cli/root"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func untouched(t *testing.T, outside string, want int) {
	t.Helper()
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != want {
		t.Errorf("the folder outside holds %d entries, want %d (%v)", len(entries), want, err)
	}
}

func TestGenCommandsFollowsNoLinkOutOfTheRepository(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	clean := joinTree(t)
	if code, _, errs := do("gen", "commands", "-C", clean); code != exitDone {
		t.Fatalf("the clean control: exit %d: %s", code, errs)
	}
	// The check of the join it wrote ends on the count the gate runner reads
	// (#695): the one file compared.
	if code, out, errs := do("gen", "commands", "--check", "-C", clean); code != exitDone {
		t.Errorf("the check of the clean control: exit %d: %s", code, errs)
	} else if _, n := counted(t, out, "files"); n != 1 {
		t.Errorf("a count of %d files compared, want the join\n%s", n, out)
	}
	for _, tc := range []struct {
		name  string
		plan  func(t *testing.T, dir, outside string)
		keep  int
		check bool
	}{
		{"the folder the join lands in", func(t *testing.T, dir, outside string) {
			t.Helper()
			if err := os.Remove(filepath.Join(dir, "internal/cli/root")); err != nil {
				t.Fatal(err)
			}
			link(t, outside, filepath.Join(dir, "internal/cli/root"))
		}, 1, false},
		{"the folder of the command packages", func(t *testing.T, dir, outside string) {
			t.Helper()
			if err := os.RemoveAll(filepath.Join(dir, "internal/cli/cmd")); err != nil {
				t.Fatal(err)
			}
			link(t, outside, filepath.Join(dir, "internal/cli/cmd"))
		}, 1, false},
		{"the table", func(t *testing.T, dir, outside string) {
			t.Helper()
			if err := os.Remove(filepath.Join(dir, layers.File)); err != nil {
				t.Fatal(err)
			}
			// A copy of the real table: a plain read parses it and goes on, so only
			// the Root stops this one.
			table, err := os.ReadFile(filepath.Join("..", "..", layers.File))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "outside.txt"), table, 0o644); err != nil {
				t.Fatal(err)
			}
			link(t, filepath.Join(outside, "outside.txt"), filepath.Join(dir, layers.File))
		}, 1, false},
		{"the join already on disk", func(t *testing.T, dir, outside string) {
			t.Helper()
			// The current join sits outside, behind a link: a plain read says
			// current, and the Root refuses it.
			if code, _, errs := do("gen", "commands", "-C", dir); code != exitDone {
				t.Fatalf("exit %d: %s", code, errs)
			}
			current, err := os.ReadFile(filepath.Join(dir, join.File))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, filepath.Base(join.File)), current, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(dir, filepath.Dir(join.File))); err != nil {
				t.Fatal(err)
			}
			link(t, outside, filepath.Join(dir, filepath.Dir(join.File)))
		}, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := joinTree(t)
			outside := outsideWith(t, "outside.txt", "x\n")
			tc.plan(t, dir, outside)
			args := []string{"gen", "commands", "-C", dir}
			if tc.check {
				args = append(args, "--check")
			}
			code, _, errs := do(args...)
			if code != exitFailed || errs == "" {
				t.Errorf("exit %d, want %d with a message: %q", code, exitFailed, errs)
			}
			untouched(t, outside, tc.keep)
		})
	}
	_ = join.File
}

func TestTheSurfaceFollowsNoLinkOutOfTheRepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	outside := outsideWith(t, "outside.txt", "x\n")
	link(t, outside, filepath.Join(dir, "docs"))
	var out say.Speaker
	if err := writeSurface(root, nil, nil, nil, &out); err == nil {
		t.Error("the listing was written through a linked folder")
	}
	untouched(t, outside, 1)
	// The marks file sits outside, behind the linked docs: a plain read would
	// return it.
	if err := os.WriteFile(filepath.Join(outside, path.Base(surface.MarksFile)), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := readOptional(root, surface.MarksFile); err == nil || data != nil {
		t.Errorf("the marks were read through a linked folder: %q, %v", data, err)
	}
}

func TestTheSurfacedPackagesAreReadThroughTheRoot(t *testing.T) {
	t.Parallel()
	dir, outside := t.TempDir(), t.TempDir()
	// The packages are all outside, each with a file: a plain read through
	// the link returns them.
	for _, pkg := range surfaced() {
		full := filepath.Join(outside, strings.TrimPrefix(pkg, "internal/"))
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "a.go"), []byte("package a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link(t, outside, filepath.Join(dir, "internal"))
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if srcs, err := readSurfaced(root); err == nil {
		t.Errorf("%d surfaced packages were read through a linked folder", len(srcs))
	}
}
