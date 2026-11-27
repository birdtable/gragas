package fs_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
)

func TestTheNamesFileIsOpened(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "names")
	if err := os.WriteFile(p, []byte("acme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := (hostfs.Files{}).Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil || string(got) != "acme\n" {
		t.Errorf("read %q, %v", got, err)
	}
}

func TestAMissingNamesFileIsAnError(t *testing.T) {
	t.Parallel()
	if _, err := (hostfs.Files{}).Open(filepath.Join(t.TempDir(), "none")); err == nil {
		t.Error("want an error")
	}
}
