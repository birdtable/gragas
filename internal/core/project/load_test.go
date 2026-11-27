package project_test

import (
	"errors"
	"io/fs"
	"maps"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// the pin's name below a folder, as config.PinFile names it.
const pinName = ".gragas/pin.yaml"

// repoDir is the repository every test loads from, a folder of the world.
const repoDir = "/w/repo"

// git is what a repository's root holds: .git, here a worktree's file.
func git(files map[string]string) map[string]string {
	files[".git"] = "gitdir: elsewhere\n"
	return files
}

// notReady fails the test unless err is a *project.NotReady that ends "not
// ready" and names each of says.
func notReady(t *testing.T, err error, says ...string) {
	t.Helper()
	nr, ok := errors.AsType[*project.NotReady](err)
	if !ok {
		t.Fatalf("Load = %v, want a *project.NotReady", err)
	}
	if nr.Word() != model.NotReady {
		t.Errorf("Word() = %q, want %q", nr.Word(), model.NotReady)
	}
	for _, s := range says {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("Load = %q, want it to name %q", err, s)
		}
	}
}

// TestLoadFindsThePinNearestTheWorkingFolder: from a
// folder of the repository, Load answers the root, the pin and the
// rulebook's root, opened below the folder that holds the pin. A pin nearer
// the working folder wins over the root's, and a start at the root reads the
// root's.
func TestLoadFindsThePinNearestTheWorkingFolder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, start, pinPath, book string
	}{
		{"from a folder below the root", repoDir + "/src/deep", ".gragas/pin.yaml", "rulebook"},
		{"from the root", repoDir, ".gragas/pin.yaml", "rulebook"},
		{"from below a nearer pin", repoDir + "/sub/inner", "sub/.gragas/pin.yaml", "sub/rulebook"},
		{"from the nearer pin's own folder", repoDir + "/sub", "sub/.gragas/pin.yaml", "sub/rulebook"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			root := w.Seed(repoDir, git(map[string]string{
				".gragas/pin.yaml":     cleanPin,
				"rulebook/parts.yaml":  "root\n",
				"src/deep/a.go":        "package a\n",
				"sub/.gragas/pin.yaml": cleanPin,
				"sub/rulebook/x.yaml":  "sub\n",
				"sub/inner/b.go":       "package b\n",
			}))
			tree := w.Ports().Tree
			got, err := project.Load(tree, c.start, pinName)
			if err != nil {
				t.Fatalf("Load(%s) = %v", c.start, err)
			}
			book, err := tree.OpenIn(root, c.book)
			if err != nil {
				t.Fatalf("OpenIn(%s) = %v", c.book, err)
			}
			if got.Root != root || got.PinPath != c.pinPath || got.Rulebook != book || got.Pin.Rulebook != "fixture-base" {
				t.Errorf("Load(%s) = %+v, want the root %s, the pin %s and the rulebook %s", c.start, got, root, c.pinPath, book)
			}
		})
	}
}

// TestLoadNeverReadsAPinAboveTheRepository is the planted case of line 2: a
// pin in the folder above the repository, with a rulebook beside it, is
// never read, from a folder of the repository or from its root; the walk
// stops at the root, and the project is not ready.
func TestLoadNeverReadsAPinAboveTheRepository(t *testing.T) {
	t.Parallel()
	for _, start := range []string{repoDir + "/src", repoDir} {
		t.Run(start, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.Seed("/w", map[string]string{".gragas/pin.yaml": cleanPin, "rulebook/parts.yaml": "above\n"})
			w.Seed(repoDir, git(map[string]string{"src/a.go": "package a\n"}))
			got, err := project.Load(w.Ports().Tree, start, pinName)
			if err == nil {
				t.Fatalf("Load(%s) = %+v, nil: the pin above the repository was read", start, got)
			}
			notReady(t, err, pinName, repoDir)
		})
	}
}

// TestLoadOutsideARepositoryIsNotReady: with no .git at or above the working
// folder, a pin there is no project; Load says .git was looked for.
func TestLoadOutsideARepositoryIsNotReady(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Seed("/w", map[string]string{".gragas/pin.yaml": cleanPin, "rulebook/parts.yaml": "x\n", "src/a.go": "package a\n"})
	_, err := project.Load(w.Ports().Tree, "/w/src", pinName)
	notReady(t, err, ".git", "/w/src")
}

// TestLoadWithNoPinIsNotReady: a repository with no pin, from its root or a
// folder in it, is not ready, and Load says what it looked for and from
// where.
func TestLoadWithNoPinIsNotReady(t *testing.T) {
	t.Parallel()
	for start, from := range map[string]string{repoDir + "/src": "src", repoDir: "the repository's root"} {
		t.Run(start, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.Seed(repoDir, git(map[string]string{"src/a.go": "package a\n", "rulebook/parts.yaml": "x\n"}))
			_, err := project.Load(w.Ports().Tree, start, pinName)
			notReady(t, err, pinName, from)
		})
	}
}

// TestLoadRefusesAPinItCannotRead: a pin that is there and is a folder, that
// cannot be read (here its folder is a link, which no read follows), or that
// ParsePin refuses, is refused by its path, never passed over for the good
// pin at the root.
func TestLoadRefusesAPinItCannotRead(t *testing.T) {
	t.Parallel()
	const near = "sub/.gragas/pin.yaml"
	cases := []struct {
		name  string
		files map[string]string
		link  string
	}{
		{"a folder", map[string]string{near + "/x": "x\n"}, ""},
		{"a pin whose folder is a link", map[string]string{near: cleanPin, "real/pin.yaml": cleanPin}, "sub/.gragas"},
		{"a pin ParsePin refuses", map[string]string{near: "rulebook: [\n"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			files := git(map[string]string{".gragas/pin.yaml": cleanPin, "rulebook/parts.yaml": "x\n", "sub/rulebook/y.yaml": "y\n"})
			maps.Copy(files, c.files)
			root := w.Seed(repoDir, files)
			if c.link != "" {
				w.Link(root, c.link, "../real")
			}
			got, err := project.Load(w.Ports().Tree, repoDir+"/sub", pinName)
			if err == nil {
				t.Fatalf("Load = %+v, nil: the pin at %s was passed over", got, near)
			}
			pe, ok := errors.AsType[*project.PinError](err)
			if !ok || pe.Path != near {
				t.Errorf("Load = %v, want a *project.PinError naming %s", err, near)
			}
		})
	}
}

// statRefused is the fake's Tree with Stat of one path failing as a fault
// other than absence does. The fake's own Stat answers only absence, so
// this is how a run reaches the refusal.
type statRefused struct {
	port.Tree
	path string
}

func (s statRefused) Stat(root model.Root, p string) (model.Entry, error) {
	if p == s.path {
		return model.Entry{}, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrPermission}
	}
	return s.Tree.Stat(root, p)
}

// TestLoadRefusesAPinItCannotStat: a pin whose Stat fails with anything but
// absence is refused by its path, never passed over for the good pin at the
// root.
func TestLoadRefusesAPinItCannotStat(t *testing.T) {
	t.Parallel()
	const near = "sub/.gragas/pin.yaml"
	w := porttest.NewWorld(t)
	w.Seed(repoDir, git(map[string]string{".gragas/pin.yaml": cleanPin, near: cleanPin, "rulebook/parts.yaml": "x\n", "sub/rulebook/y.yaml": "y\n"}))
	got, err := project.Load(statRefused{w.Ports().Tree, near}, repoDir+"/sub", pinName)
	if err == nil {
		t.Fatalf("Load = %+v, nil: the pin at %s was passed over", got, near)
	}
	pe, ok := errors.AsType[*project.PinError](err)
	if !ok || pe.Path != near || !strings.Contains(pe.Error(), "cannot be read") {
		t.Errorf("Load = %v, want a *project.PinError saying %s cannot be read", err, near)
	}
}

// TestLoadRefusesASourceThroughALinkOut is the planted case of line 3: a
// source that is a link leading out of the repository matches
// port.ErrOutside, and no root is answered. The control: a link that stays
// inside opens as the folder it leads to.
func TestLoadRefusesASourceThroughALinkOut(t *testing.T) {
	t.Parallel()
	load := func(t *testing.T, target string) (project.Loaded, model.Root, port.Tree, error) {
		t.Helper()
		w := porttest.NewWorld(t)
		w.Seed("/w", map[string]string{"outside/parts.yaml": "outside\n"})
		root := w.Seed(repoDir, git(map[string]string{".gragas/pin.yaml": with("source", "book"), "rulebook/parts.yaml": "x\n"}))
		w.Link(root, "book", target)
		got, err := project.Load(w.Ports().Tree, repoDir, pinName)
		return got, root, w.Ports().Tree, err
	}
	t.Run("out", func(t *testing.T) {
		t.Parallel()
		got, _, _, err := load(t, "../outside")
		if !errors.Is(err, port.ErrOutside) || got.Rulebook != "" {
			t.Errorf("Load = %+v, %v, want no root and port.ErrOutside", got, err)
		}
		if err != nil && !strings.Contains(err.Error(), "book") {
			t.Errorf("Load = %v, want it to name the source", err)
		}
	})
	t.Run("the control: inside", func(t *testing.T) {
		t.Parallel()
		got, root, tree, err := load(t, "rulebook")
		if err != nil {
			t.Fatalf("Load = %v", err)
		}
		if want, err := tree.OpenIn(root, "rulebook"); err != nil || got.Rulebook != want {
			t.Errorf("Load's rulebook = %q, want %q (%v)", got.Rulebook, want, err)
		}
	})
}

// TestLoadRefusesASourceThatIsNotThere: a source no folder is at is not
// ready, naming the path.
func TestLoadRefusesASourceThatIsNotThere(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Seed(repoDir, git(map[string]string{".gragas/pin.yaml": cleanPin, "src/a.go": "package a\n"}))
	got, err := project.Load(w.Ports().Tree, repoDir, pinName)
	if got.Rulebook != "" {
		t.Errorf("Load's rulebook = %q, want none", got.Rulebook)
	}
	notReady(t, err, "rulebook", "is not there")
}

// TestLoadRefusesASourceThatIsAFile: a source that is a file is not ready,
// naming the path.
func TestLoadRefusesASourceThatIsAFile(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Seed(repoDir, git(map[string]string{".gragas/pin.yaml": cleanPin, "rulebook": "a file\n"}))
	got, err := project.Load(w.Ports().Tree, repoDir, pinName)
	if got.Rulebook != "" {
		t.Errorf("Load's rulebook = %q, want none", got.Rulebook)
	}
	notReady(t, err, "rulebook", "is a file")
}
