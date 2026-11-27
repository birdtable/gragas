package coreapi_test

import (
	"context"
	"sync"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// scriptedSandbox makes a scratch folder per Prepare and records each. It
// reaches the level it is told, the isolated one when told none.
type scriptedSandbox struct {
	mu       sync.Mutex
	level    model.Level
	prepared []model.Prepare
	released []model.Scratch
}

func (s *scriptedSandbox) Prepare(_ context.Context, p model.Prepare) (model.Scratch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prepared = append(s.prepared, p)
	level := s.level
	if level == "" {
		level = model.Isolated
	}
	return model.Scratch{Dir: "/cache/scratch/run-1", Level: level, Seconds: p.Seconds, MemoryMiB: p.MemoryMiB}, nil
}

func (s *scriptedSandbox) Release(sc model.Scratch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released = append(s.released, sc)
	return nil
}

func (s *scriptedSandbox) Sweep() (int, error) { return 0, nil }

func (s *scriptedSandbox) Left() (int, error) { return 0, nil }

var _ port.Sandbox = (*scriptedSandbox)(nil)
