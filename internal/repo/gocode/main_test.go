// Package gocode holds the repository's checks that read the Go source files
// themselves, file by file, and no go list, snapshot or program:
// where a link is made, where a program exits and with what
// code, and which folders may name an adapter's tool. Split from internal
// so that a change to the docs or the rulebook replays them from Go's test
// cache.
package gocode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/layers"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// root is the module's root, from this package's folder.
const root = "../../.."

// table is layers.yaml read as internal's table helper reads it, each time
// its own.
func table(t *testing.T) *layers.Table {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, layers.File))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := layers.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// fires fails t unless one of the findings says want.
func fires(t *testing.T, findings []string, want string) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(f, want) {
			return
		}
	}
	t.Errorf("no finding says %q; found %q", want, findings)
}
