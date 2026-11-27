// Package toml is the one reader of TOML in gragas (ruling of 14 Nov 2026):
// it reads .gitleaks.toml for the config checker and nothing else. The
// library is github.com/pelletier/go-toml/v2, which the register of Go
// libraries allows here and nowhere else in the reading.
package toml

import (
	"fmt"

	gotoml "github.com/pelletier/go-toml/v2"
)

// Reader is the adapter of the port that reads TOML. A document that is not
// valid TOML, or that repeats a key, is an error.
type Reader struct{}

// Read parses data as a TOML document into maps and lists of the plain kinds
// (map[string]any, []any, string, bool, int64, float64).
func (Reader) Read(data []byte) (map[string]any, error) {
	doc := map[string]any{}
	if err := gotoml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("not valid TOML: %w", err)
	}
	return doc, nil
}
