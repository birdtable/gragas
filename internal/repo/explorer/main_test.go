// Package explorer holds the repository's checks of the explorer boards: the
// canvas pages under docs/explorer-boards and the parts seed of explorer/core,
// which they read and nothing else. Split from internal so that
// Go's test cache replays them when neither changed.
package explorer

import (
	"os"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// root is the module's root, from this package's folder.
const root = "../../.."
