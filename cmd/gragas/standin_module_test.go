package main

import (
	"context"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	hostplugin "github.com/birdtable/gragas/internal/host/plugin"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// approving is a Trust that approves every digest. The host only asks: any
// other method is a nil call, and panics.
type approving struct{ port.Trust }

func (approving) Approved(context.Context, model.Digest) (bool, error) { return true, nil }

// TestTheHostServesStandinWasmAsTheCasesSay:
// standin.wasm, built from internal/dev/standin, opened by the host main
// makes, with the engine's codec and judgement, answers every case of
// internal/port/contracttest as the in-memory fake and the source called in
// process do. Risk: the module answers otherwise than the fake every core test
// uses, or the host reads its answer otherwise than the codec, so a core test
// that passes on the fake fails, or passes wrongly, on the real plugin.
func TestTheHostServesStandinWasmAsTheCasesSay(t *testing.T) {
	t.Parallel()
	wasm := configtest.StandinWasm(t)
	ref := model.PluginRef{Name: "standin", Version: "1.0.0"}
	bundles := &porttest.Bundles{}
	bundles.Put("/project", ref, standinManifest(wasm), wasm)
	names := world(nil, config.Config{StateDir: t.TempDir()}).FileNames
	host, ok := pluginsOf(names, approving{}, bundles).(*hostplugin.Host)
	if !ok {
		t.Fatal("pluginsOf makes no *plugin.Host")
	}
	t.Cleanup(func() { _ = host.Close(context.WithoutCancel(t.Context())) })
	contracttest.Plugin(t, func(t *testing.T) port.Plugin {
		t.Helper()
		p, err := host.Open(t.Context(), "/project", ref)
		if err != nil {
			t.Fatalf("Open of standin.wasm = %v", err)
		}
		t.Cleanup(func() { _ = p.Close() })
		return p
	})
}
