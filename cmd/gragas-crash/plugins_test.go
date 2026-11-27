package main

import (
	"os"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/clock"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestTheCrashWorldHandsAPluginHost (gragas#2325, arch.ports.5): the crash
// program's world carries Plugins from host, as cmd/gragas's does.
func TestTheCrashWorldHandsAPluginHost(t *testing.T) {
	t.Parallel()
	cfg := config.Config{StateDir: t.TempDir()}
	if world(nil, cfg, clock.Clock{}, &bounds{}).Plugins == nil {
		t.Error("the crash world has no Plugins; want host's")
	}
}
