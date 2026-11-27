package contracttest

import (
	"errors"
	"path"
	"path/filepath"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// SandboxSetup is the Sandbox under test, and two ways to see into a folder
// it prepared: Files answers every file in it, slash-separated relative
// paths in any order, and whether the folder is there at all; Read answers
// one file's content.
type SandboxSetup struct {
	Sandbox port.Sandbox
	Files   func(s model.Scratch) ([]string, bool)
	Read    func(s model.Scratch, rel string) ([]byte, error)
}

// SandboxMaker makes the setup for one subtest.
type SandboxMaker func(t *testing.T) SandboxSetup

// Sandbox runs the Sandbox suite against the port build answers, at the
// isolated level, which every adapter reaches.
func Sandbox(t *testing.T, build SandboxMaker) {
	t.Helper()
	sandboxPlacing(t, build)
	sandboxRefusing(t, build)
	sandboxReleasing(t, build)
	sandboxSweeping(t, build)
}

// prepared prepares p and fails the test when it is refused.
func prepared(t *testing.T, s SandboxSetup, p model.Prepare) model.Scratch {
	t.Helper()
	sc, err := s.Sandbox.Prepare(t.Context(), p)
	if err != nil {
		t.Fatalf("Prepare(%+v) = %v", p, err)
	}
	return sc
}

func sandboxPlacing(t *testing.T, build SandboxMaker) {
	t.Helper()
	t.Run("each fixture and each file of the config set is placed at its path, and the limits are the ones asked", func(t *testing.T) {
		s := build(t)
		p := model.Prepare{
			Level:    model.Isolated,
			Fixtures: []model.File{{Path: "bad/one.go", Data: []byte("package bad\n")}, {Path: "good.go", Data: nil}},
			Config:   []model.File{{Path: ".tool.yml", Data: []byte("version: \"2\"\n")}},
			Seconds:  7, MemoryMiB: 64,
		}
		sc := prepared(t, s, p)
		// The folder is slash-separated, and absolute in the port's slash form (a
		// fake's /scratch/1) or in this system's form: C:/Users/... on Windows,
		// which path.IsAbs reads as relative.
		if !slashAbs(sc.Dir) || sc.Level != model.Isolated || sc.Seconds != 7 || sc.MemoryMiB != 64 {
			t.Errorf("Prepare = %+v, want an absolute folder at the isolated level held to 7 seconds and 64 MiB", sc)
		}
		files, there := s.Files(sc)
		slices.Sort(files)
		if want := []string{".tool.yml", "bad/one.go", "good.go"}; !there || !slices.Equal(files, want) {
			t.Errorf("the folder holds %v (there: %v), want %v", files, there, want)
		}
		for _, f := range append(slices.Clone(p.Fixtures), p.Config...) {
			if got, err := s.Read(sc, f.Path); err != nil || string(got) != string(f.Data) {
				t.Errorf("%s holds %q, %v; want %q", f.Path, got, err, f.Data)
			}
		}
	})
	// gragas sets no memory limit of its own: a
	// deadline of 0 is the adapter's own, a memory limit of 0 is none.
	// Planted failure: the adapter's 4096 MiB default back.
	t.Run("no fixtures is an empty folder, a deadline of 0 is the adapter's own, and a memory limit of 0 is none", func(t *testing.T) {
		s := build(t)
		sc := prepared(t, s, model.Prepare{Level: model.Isolated})
		if files, there := s.Files(sc); !there || len(files) != 0 {
			t.Errorf("the folder holds %v (there: %v), want an empty folder", files, there)
		}
		if sc.Seconds < 1 || sc.MemoryMiB != 0 {
			t.Errorf("Prepare = %+v, want the adapter's deadline and no memory limit", sc)
		}
	})
	t.Run("two prepares are two folders", func(t *testing.T) {
		s := build(t)
		a, b := prepared(t, s, model.Prepare{Level: model.Isolated}), prepared(t, s, model.Prepare{Level: model.Isolated})
		if a.Dir == b.Dir {
			t.Errorf("both runs were given %s", a.Dir)
		}
	})
}

func sandboxRefusing(t *testing.T, build SandboxMaker) {
	t.Helper()
	t.Run("a file whose path leaves the folder is not placed", func(t *testing.T) {
		for _, p := range table(t, "sandbox-outside") {
			s := build(t)
			if sc, err := s.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: p}}}); !errors.Is(err, port.ErrOutside) {
				t.Errorf("a fixture at %q = %+v, %v; want refused as outside", p, sc, err)
			}
			if sc, err := s.Sandbox.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Config: []model.File{{Path: p}}}); !errors.Is(err, port.ErrOutside) {
				t.Errorf("a config file at %q = %+v, %v; want refused as outside", p, sc, err)
			}
		}
	})
	t.Run("a file with no path, two files at one path, a level that is neither and a limit below none are refused", func(t *testing.T) {
		for name, p := range map[string]model.Prepare{
			"no path":             {Level: model.Isolated, Fixtures: []model.File{{Path: ""}}},
			"two at one path":     {Level: model.Isolated, Fixtures: []model.File{{Path: "a.go"}}, Config: []model.File{{Path: "a.go"}}},
			"a file under a file": {Level: model.Isolated, Fixtures: []model.File{{Path: "a.go"}, {Path: "a.go/b.go"}}},
			"no level":            {},
			"another level":       {Level: "contained"},
			"negative time":       {Level: model.Isolated, Seconds: -1},
			"negative memory":     {Level: model.Isolated, MemoryMiB: -1},
		} {
			s := build(t)
			if sc, err := s.Sandbox.Prepare(t.Context(), p); err == nil {
				t.Errorf("%s: Prepare = %+v, want refused", name, sc)
			}
		}
	})
}

func sandboxReleasing(t *testing.T, build SandboxMaker) {
	t.Helper()
	t.Run("a released folder is gone, and releasing it again is no error", func(t *testing.T) {
		s := build(t)
		sc := prepared(t, s, model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a/b.go", Data: []byte("x")}}})
		if err := s.Sandbox.Release(sc); err != nil {
			t.Fatalf("Release = %v", err)
		}
		if files, there := s.Files(sc); there {
			t.Errorf("after Release the folder is there, holding %v", files)
		}
		if err := s.Sandbox.Release(sc); err != nil {
			t.Errorf("a second Release = %v, want none", err)
		}
	})
	t.Run("the zero scratch folder is refused", func(t *testing.T) {
		if err := build(t).Sandbox.Release(model.Scratch{}); err == nil {
			t.Error("Release of the zero scratch folder = nil, want refused")
		}
	})
}

// sandboxSweeping holds Sweep and Left: what a stopped run left is removed,
// Left counts what a sweep removes and removes nothing, and a sweep with
// nothing left removes nothing.
func sandboxSweeping(t *testing.T, build SandboxMaker) {
	t.Helper()
	sweepRemovesWhatWasLeft(t, build)
	leftCountsWhatASweepRemoves(t, build)
	sweepWithNothingLeft(t, build)
}

func leftCountsWhatASweepRemoves(t *testing.T, build SandboxMaker) {
	t.Helper()
	t.Run("left counts the folders a sweep removes and removes none of them", func(t *testing.T) {
		s := build(t)
		kept := prepared(t, s, model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a/b.go", Data: []byte("x")}}})
		left := prepared(t, s, model.Prepare{Level: model.Isolated})
		released := prepared(t, s, model.Prepare{Level: model.Isolated})
		if err := s.Sandbox.Release(released); err != nil {
			t.Fatalf("Release = %v", err)
		}
		if n, err := s.Sandbox.Left(); err != nil || n != 2 {
			t.Fatalf("Left = %d, %v; want the 2 folders a Sweep removes", n, err)
		}
		for _, sc := range []model.Scratch{kept, left} {
			if _, there := s.Files(sc); !there {
				t.Errorf("after Left %s is gone; Left removes nothing", sc.Dir)
			}
		}
		if n, err := s.Sandbox.Sweep(); err != nil || n != 2 {
			t.Fatalf("Sweep after Left = %d, %v; want the 2 Left counted", n, err)
		}
		if n, err := s.Sandbox.Left(); err != nil || n != 0 {
			t.Errorf("Left after a Sweep = %d, %v; want 0", n, err)
		}
	})
}

func sweepRemovesWhatWasLeft(t *testing.T, build SandboxMaker) {
	t.Helper()
	t.Run("a sweep removes every folder prepared and not released, and answers how many", func(t *testing.T) {
		s := build(t)
		kept := prepared(t, s, model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a/b.go", Data: []byte("x")}}})
		left := prepared(t, s, model.Prepare{Level: model.Isolated})
		released := prepared(t, s, model.Prepare{Level: model.Isolated})
		if err := s.Sandbox.Release(released); err != nil {
			t.Fatalf("Release = %v", err)
		}
		n, err := s.Sandbox.Sweep()
		if err != nil || n != 2 {
			t.Fatalf("Sweep = %d, %v; want 2 folders removed and no error", n, err)
		}
		for _, sc := range []model.Scratch{kept, left} {
			if files, there := s.Files(sc); there {
				t.Errorf("after Sweep %s is there, holding %v", sc.Dir, files)
			}
		}
	})
}

func sweepWithNothingLeft(t *testing.T, build SandboxMaker) {
	t.Helper()
	t.Run("a sweep with no folder left removes none, and a folder made after it is whole", func(t *testing.T) {
		s := build(t)
		if n, err := s.Sandbox.Sweep(); err != nil || n != 0 {
			t.Errorf("Sweep before any Prepare = %d, %v; want 0 and no error", n, err)
		}
		sc := prepared(t, s, model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a.go", Data: []byte("x")}}})
		if n, err := s.Sandbox.Sweep(); err != nil || n != 1 {
			t.Errorf("Sweep = %d, %v; want 1", n, err)
		}
		if n, err := s.Sandbox.Sweep(); err != nil || n != 0 {
			t.Errorf("a second Sweep = %d, %v; want 0 and no error", n, err)
		}
		if err := s.Sandbox.Release(sc); err != nil {
			t.Errorf("Release of a swept folder = %v, want none", err)
		}
		fresh := prepared(t, s, model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a.go", Data: []byte("y")}}})
		if got, err := s.Read(fresh, "a.go"); err != nil || string(got) != "y" {
			t.Errorf("a folder made after a sweep holds %q, %v; want y", got, err)
		}
	})
}

// slashAbs is whether p, slash-separated, is absolute: in the port's slash form
// (a fake's /work/bin) or in this system's own (C:/Users/... on Windows, which
// path.IsAbs reads as relative).
func slashAbs(p string) bool {
	return path.IsAbs(p) || filepath.IsAbs(filepath.FromSlash(p))
}
