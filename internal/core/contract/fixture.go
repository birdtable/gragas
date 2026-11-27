package contract

import (
	"fmt"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/port"
)

// FixtureDir is the folder of the Assets port that holds the fixture
// rulebook's documents: a generated copy of the repository's contract/fixture
// (the embed site cannot reach the folder above it).
const FixtureDir = "contract/fixture"

// FixtureFile is one file of the fixture's documents.
type FixtureFile struct {
	// Path is slash-separated and relative to the fixture folder.
	Path string
	// Data is its bytes, the canonical bytes of a document.
	Data []byte
}

// Fixture is the fixture rulebook's documents and the pages and notes beside
// them, sorted by path, read through the Assets port. Every document is held
// to its schema by a test, and the repository's gate compares what a run of
// `contract fixture` writes with contract/fixture.
func Fixture(files port.Assets) ([]FixtureFile, error) {
	paths, err := files.List(FixtureDir)
	if err != nil {
		return nil, fmt.Errorf("list the fixture's documents: %w", err)
	}
	var out []FixtureFile
	for _, p := range paths {
		data, err := files.Read(p)
		if err != nil {
			return nil, fmt.Errorf("read the fixture's %s: %w", p, err)
		}
		out = append(out, FixtureFile{Path: strings.TrimPrefix(p, FixtureDir+"/"), Data: data})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the build holds no fixture documents under %s", path.Clean(FixtureDir))
	}
	return out, nil
}
