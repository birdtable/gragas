package tool_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model"
)

// TestTheVersionProbeRunsInAHomeOfItsOwn: a program that cannot
// start with no HOME (Conftest panics, "XDG_DATA_HOME is relative") failed
// its version probe, so it was never found, though a step was given a Home of
// its own. The probe now runs in a Home made for it as a step's
// is, outside the project, and gone when the probe ends.
//
// Risk: the probe runs with no HOME and refuses the tool before any step
// runs; or its Home is left behind in the temp folder, one for each find.
func TestTheVersionProbeRunsInAHomeOfItsOwn(t *testing.T) {
	t.Parallel()
	root := private(t)
	said := filepath.Join(private(t), "home") // where the probe writes the HOME it was given
	prog := program(t, private(t)+"/lint-a", fakeSteps(
		configtest.WriteUnder(homeName(), "probe", "x"), // exits 9 with no HOME
		configtest.VarsTo(said, homeName()),
		configtest.Version("lint-a 1.2.3", ""),
	))
	tl := over(map[string]map[string]string{root: {"lint-a@1.2.3": prog}}, nil)
	f, err := tool.LocateGrant(tl, t.Context(), model.Root(root), foundGrant(), "lint-a", false)
	if err != nil || f.Version != "1.2.3" {
		t.Fatalf("LocateGrant = %+v, %v; want lint-a found at 1.2.3", f, err)
	}
	raw, err := os.ReadFile(said)
	if err != nil {
		t.Fatalf("the probe wrote no HOME: %v", err)
	}
	home := string(raw)
	if !filepath.IsAbs(home) {
		t.Fatalf("the probe's %s is %q, not an absolute folder", homeName(), home)
	}
	if rel, err := filepath.Rel(filepath.FromSlash(root), home); err == nil && filepath.IsLocal(rel) {
		t.Errorf("the probe's home %s is inside the project %s", home, root)
	}
	// HOME is the folder "home" in the one RunHome made, which the probe's end removes.
	for _, gone := range []string{home, filepath.Dir(home)} {
		if _, err := os.Stat(gone); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s is left after the probe: Stat = %v, want it removed", gone, err)
		}
	}
}
