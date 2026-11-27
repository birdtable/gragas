package configtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/confine"
)

// MkdirPrivate makes the folder name below dir, each level of it that is not
// there, as only this account reaches it, and answers its path; it fails the
// test if it cannot. It is confine.MkdirPrivate below a root
// opened on dir, so a test names no mode and follows no link out of dir.
// dir is a folder the test made, t.TempDir or Private. A folder
// already there is kept as it is.
func MkdirPrivate(t testing.TB, dir, name string) string {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	made, err := confine.MkdirPrivate(root, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := made.Close(); err != nil {
		t.Error(err)
	}
	return filepath.Join(dir, name)
}
