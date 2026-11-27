// Package rulebook holds the repository's checks that read the rulebook's
// rule files and the register of libraries, and no Go source:
// split from internal so that a change to the Go tree replays
// them from Go's test cache, and a change to the rulebook runs only them and
// the checks that read it too.
package rulebook

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

// theRegister reads the register of the tree through its loader, as
// internal's preset register test does.
func theRegister(t *testing.T) *layers.Register {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, layers.RegisterFile))
	if err != nil {
		t.Fatal(err)
	}
	// The schema is held by cmd/gragas-dev; the loader holds the rest.
	r, err := layers.ParseRegister(data, func(any) []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return r
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
