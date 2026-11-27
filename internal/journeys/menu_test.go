package journeys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// TestTheBareCommandPipedPrintsHelpAndEndsUsageInEveryPlace: nobody can answer
// a menu off a terminal, so the bare command prints help, says why on standard
// error and ends usage (exit 2) whether the folder is a rulebook, a project,
// both or neither (cli.commands.12, cli.environment.5, S19). The menu's rows
// per place are held in internal/cli/root, where the kit's list is handed them.
func TestTheBareCommandPipedPrintsHelpAndEndsUsageInEveryPlace(t *testing.T) {
	t.Parallel()
	both := fixture(t)
	pinProject(t, both)
	project := t.TempDir()
	pinProject(t, project)
	for _, tc := range []struct{ name, dir string }{
		{"rulebook", fixture(t)}, {"project", project}, {"both", both}, {"neither", t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, tc.dir, map[string]string{"COLUMNS": "80"})
			if r.code != 2 {
				t.Fatalf("exit %d, want 2 (usage)\n%s%s", r.code, r.stdout, r.stderr)
			}
			uitest.Golden(t, "menu-piped-"+tc.name, "$ gragas\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr)
		})
	}
}

// pinProject makes dir a project: a folder that has chosen a rulebook.
func pinProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".gragas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gragas", "pin.yaml"), []byte("rulebook: house\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
