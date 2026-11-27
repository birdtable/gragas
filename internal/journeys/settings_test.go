package journeys

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
	"github.com/birdtable/go-files/appdirs"
	testenv "github.com/birdtable/go-testkit"
)

// settingsFile is a settings file holding body, 0600, in a folder of the
// test's own, and the folder.
func settingsFile(t *testing.T, body string) (file, dir string) {
	t.Helper()
	dir = testenv.ShortDir(t)
	file = filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o600); err != nil { // past the umask
		t.Fatal(err)
	}
	return file, dir
}

// TestABadSettingsFileEndsTheRunNamingItsLine: a settings file with an
// unknown key ends every run refused, before the command, naming the file,
// the line and the keys there are. The run's --json document is held by a
// golden, with the file put back as <config>/settings.yaml; the text run is
// held by its exit and words. The text is no golden: its reason wraps at the
// terminal's width, at most 96 columns, at a point the file's path decides,
// and that path's length is the system's temp folder's. The
// owner transcripts hold how a refusal is laid out. Risk: a run that starts
// on settings it could not read, or a refusal a person cannot find in their
// file.
func TestABadSettingsFileEndsTheRunNamingItsLine(t *testing.T) {
	t.Parallel()
	file, _ := settingsFile(t, "version: 1\n\ncolr: never\n")
	env := map[string]string{"GRAGAS_CONFIG": file}
	r := run(t, fixture(t), env, "check")
	if said := r.stdout + r.stderr; r.code != 5 || !strings.Contains(said, file+":3:") || !strings.Contains(said, `unknown key "colr"`) {
		t.Fatalf("exit %d, want 5 naming %s:3\nstdout:\n%s\nstderr:\n%s", r.code, file, r.stdout, r.stderr)
	}
	j := run(t, fixture(t), env, "--json", "check")
	if j.code != 5 {
		t.Fatalf("--json: exit %d, want 5\nstdout:\n%s\nstderr:\n%s", j.code, j.stdout, j.stderr)
	}
	// The document writes the file as a JSON string, so a separator the
	// string escapes is put back escaped too.
	quoted, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	got := "$ GRAGAS_CONFIG=<config>/settings.yaml gragas --json check\n--- stdout\n" + j.stdout + "--- stderr\n" + j.stderr
	uitest.Golden(t, "settings-refused-json", strings.ReplaceAll(got, strings.Trim(string(quoted), `"`), "<config>/settings.yaml"))
}

// TestARelativeHomeEndsTheRunNotReadyNamingHome: with no XDG folder
// set, a relative HOME would put gragas's folders under the folder it runs
// in, a repository. A run that writes, gen --apply, ends not ready before
// the command, naming the variable the home is read from (appdirs.HomeVar: HOME
// here, and the system's own where it has none), and makes no folder in the repository: joined,
// HOME=h puts the lock and the run log at h/.local/state/gragas in it.
// Planted failures: config.folder joining HOME as it is turns the folder
// line red alone, with the exit line off; Load's check removed turns the
// exit line red.
func TestARelativeHomeEndsTheRunNotReadyNamingHome(t *testing.T) {
	t.Parallel()
	dir := fixture(t)
	r := run(t, dir, map[string]string{homeKey: "h", configKey: "", stateKey: "", cacheKey: ""}, "gen", "--apply")
	if _, err := os.Lstat(filepath.Join(dir, "h")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a folder was made under the repository at h: %v", err)
	}
	if said := r.stdout + r.stderr; r.code != 4 || !strings.Contains(said, appdirs.HomeVar+` must be an absolute path, and it is "h"`) {
		t.Errorf("exit %d, want 4 naming "+appdirs.HomeVar+"\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
}
