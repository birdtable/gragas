// Package docs holds the repository's checks of its documents:
// the pages under docs/ and the boards, their front matter, links, paths,
// rule ids, numbers, prose and format, and AGENTS.md. They read the docs, the
// rulebook's rule files and layers.yaml, and run git only to list the
// tracked files. Split from internal so that a change to the Go tree alone
// replays them from Go's test cache.
package docs

import (
	"os"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// root is the module's root, from this package's folder.
const root = "../../.."
