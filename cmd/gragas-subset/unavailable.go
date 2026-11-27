package main

import (
	"fmt"

	"github.com/birdtable/gragas/internal/port"
)

// unavailable is the Assets port of a build with no embedded files: every
// method answers port.ErrUnavailable and nothing pretends to work
// (arch.ports.5, arch.ports.17).
type unavailable struct{}

var _ port.Assets = unavailable{}

func (unavailable) Read(p string) ([]byte, error) {
	return nil, fmt.Errorf("read %q: no embedded files in the subset build: %w", p, port.ErrUnavailable)
}

func (unavailable) List(dir string) ([]string, error) {
	return nil, fmt.Errorf("list %q: no embedded files in the subset build: %w", dir, port.ErrUnavailable)
}
