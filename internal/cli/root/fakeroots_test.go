package root

import (
	"os"
	"path/filepath"
)

// The folders and the settings file the tests name on a fake tree: each is
// absolute on the system the tests run on, so none is a Unix-rooted literal.
func fakeState() string { return filepath.Join(os.TempDir(), "gragas-fake", "state") }
func fakeCache() string { return filepath.Join(os.TempDir(), "gragas-fake", "cache") }

func settingsFile() string {
	return filepath.Join(os.TempDir(), "gragas-fake", "c", "settings.yaml")
}
