package tool_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// TestAStepRunsInAHomeOfItsOwn: golangci-lint exits 1 with "$HOME
// is not defined" when a run has no home, and no caller gave a step one. Each
// step's run now has HOME set to a folder made for that run alone, outside the
// project and the scratch folder, one a second step does not share, and gone
// when the step ends. The run is started only on a Home that exists and passes
// the trust walk (checkHome), so a HOME the program reads is a folder that was
// there.
//
// Risk: a step runs with no HOME, or with one folder kept between steps, so a
// cache one step writes steers the next.
func TestAStepRunsInAHomeOfItsOwn(t *testing.T) {
	t.Parallel()
	s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.EnvOut())
	r := bind(t, tl, s)
	var homes []string
	for i := range 2 {
		res, err := r.Run(t.Context(), 0, i, runOf())
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
		home, flags := "", ""
		for line := range strings.SplitSeq(text(t, res.Stdout), "\n") {
			switch name, v, _ := strings.Cut(line, "="); name {
			case homeName():
				home = v
			case "GOFLAGS":
				flags = v
			}
		}
		// The go command's module cache lives in the Home: without
		// -modcacherw it leaves read-only folders no removal can empty, and
		// the step would fail after a run that passed.
		if flags != "-modcacherw" {
			t.Errorf("GOFLAGS = %q, want -modcacherw", flags)
		}
		if home == "" {
			t.Fatalf("the step has no %s:\n%s", homeName(), text(t, res.Stdout))
		}
		homes = append(homes, home)
	}
	if homes[0] == homes[1] {
		t.Errorf("two steps share the home %s", homes[0])
	}
	for _, h := range homes {
		for _, apart := range []string{s.Scratch.Dir, string(s.Root)} {
			if rel, err := filepath.Rel(filepath.FromSlash(apart), h); err == nil && filepath.IsLocal(rel) {
				t.Errorf("the home %s is inside %s", h, apart)
			}
		}
		if _, err := os.Stat(h); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the home %s is left after its step: Stat = %v", h, err)
		}
	}
}

// TestAHomeWithAReadOnlyFolderIsRemovedWithItsStep: a program
// leaves a folder in its Home at mode 0555 (the go command's module cache does
// without -modcacherw). go-files' RemoveAllOwned gives the Home's own folders
// the owner's write bit and removes the tree, so the step's answer stands and
// no Home is left.
//
// Risk: the removal's error joins the step's, or the Homes pile up in the
// temp folder for good.
func TestAHomeWithAReadOnlyFolderIsRemovedWithItsStep(t *testing.T) {
	t.Parallel()
	s, tl, _ := steps(t, model.Isolated, plugin.LevelIsolated, &notes{}, configtest.EnvOut(), configtest.StuckUnder(homeName(), "stuck"))
	r := bind(t, tl, s)
	res, err := r.Run(t.Context(), 0, 0, runOf())
	if err != nil {
		t.Fatalf("Run = %v, want the tool's answer with no removal error", err)
	}
	home := ""
	for line := range strings.SplitSeq(text(t, res.Stdout), "\n") {
		if name, v, _ := strings.Cut(line, "="); name == homeName() {
			home = v
		}
	}
	if home == "" {
		t.Fatalf("the step has no %s:\n%s", homeName(), text(t, res.Stdout))
	}
	top := filepath.Dir(home)
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(home, "stuck"), 0o755)
		_ = os.RemoveAll(top)
	})
	if _, err := os.Stat(top); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the Home %s is left after its step: Stat = %v", top, err)
	}
}
