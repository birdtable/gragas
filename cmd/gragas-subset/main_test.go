package main

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestTheSubsetFillsEveryRequiredPortOfTheOldForm: the adapters the subset
// wires leave none of the five empty (arch.ports.17).
func TestTheSubsetFillsEveryRequiredPortOfTheOldForm(t *testing.T) {
	t.Parallel()
	if missing := porttest.Missing(ports(config.Config{}), true); len(missing) > 0 {
		t.Errorf("the subset leaves %v empty", missing)
	}
}

// TestTheSubsetsOldLockTrustsTheConfiguredMounts (gragas#1803): the old lock's
// walk of its folder trusts the configured mounts, as cmd/gragas's does.
// Planted: the old lock left without them (red here).
func TestTheSubsetsOldLockTrustsTheConfiguredMounts(t *testing.T) {
	t.Parallel()
	cfg := config.Config{Mounts: []string{"/work/disk"}}
	if old, ok := ports(cfg).Locks.(hostfs.LegacyLocks); !ok || !slices.Equal(old.Mounts, cfg.Mounts) {
		t.Errorf("the old lock = %#v; want it to trust the configured %v", ports(cfg).Locks, cfg.Mounts)
	}
}

// TestTheSubsetsAssetsAnswerUnavailableAndNothingElse: Read and List both say
// the platform cannot serve it, never an empty list or a page.
func TestTheSubsetsAssetsAnswerUnavailableAndNothingElse(t *testing.T) {
	t.Parallel()
	var a port.Assets = unavailable{}
	if b, err := a.Read("explorer/index.html"); b != nil || !errors.Is(err, port.ErrUnavailable) {
		t.Errorf("Read = %q, %v, want ErrUnavailable", b, err)
	}
	if l, err := a.List(""); l != nil || !errors.Is(err, port.ErrUnavailable) {
		t.Errorf("List = %v, %v, want ErrUnavailable", l, err)
	}
}
