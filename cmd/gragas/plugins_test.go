package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	hostplugin "github.com/birdtable/gragas/internal/host/plugin"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// emptyModule is the smallest WebAssembly module: the magic and the version.
const emptyModule = "\x00asm\x01\x00\x00\x00"

// standinManifest is a manifest of standin 1.0.0 for module that the engine's
// judgement passes.
func standinManifest(module []byte) []byte {
	return []byte(`contract: 1
needs: []
name: standin
version: 1.0.0
tool: standin
tool_versions: ">=1.0.0 <2.0.0"
targets: [source]
provides: [plan, entries]
extends: false
warning: false
programs:
  - id: standin
    path: bin/standin
    version_argv: [--version]
    versions: ">=1.0.0 <2.0.0"
wasm: ` + porttest.Wasm(module) + "\n")
}

// refusing is a Trust that approves nothing and counts its asks. The host
// only asks: any other method is a nil call, and panics.
type refusing struct {
	port.Trust
	asked atomic.Int64
}

func (r *refusing) Approved(context.Context, model.Digest) (bool, error) {
	r.asked.Add(1)
	return false, nil
}

// TestMainsPluginsAreTheHostOverItsBundles: main fills
// Plugins with host/plugin's Host over host/fs's Bundles, so a root
// with no lock has no plugin, and an Open of one is port.ErrNotInstalled with
// no module run. Risk: a Plugins that is not the host, or a host that answers
// a plugin nobody locked. Planted: the same host over a Bundles that holds
// standin opens past ErrNotInstalled (below).
func TestMainsPluginsAreTheHostOverItsBundles(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	w := world(nil, config.Config{StateDir: t.TempDir()})
	if _, ok := w.Plugins.(*hostplugin.Host); !ok {
		t.Fatalf("main's Plugins is %T, want *plugin.Host", w.Plugins)
	}
	root := t.TempDir()
	ref := model.PluginRef{Name: "standin", Version: "1.0.0"}
	if refs, err := w.Plugins.List(ctx, root); err != nil || len(refs) != 0 {
		t.Errorf("Plugins.List of a root with no lock = %v, %v; want none, nil", refs, err)
	}
	if _, err := w.Plugins.Open(ctx, root, ref); !errors.Is(err, port.ErrNotInstalled) {
		t.Errorf("Plugins.Open in a root with no lock = %v, want port.ErrNotInstalled", err)
	}
}

// TestAFreshMachineOpensAPluginOnlyOnceApproved: on a machine
// that never approved a plugin, so with no state folder at all, the host main
// makes over main's own approvals refuses standin.wasm as not approved, and
// for that alone: the missing record is no error of its own. Once main's
// Trust approves the lock's digest, the same host opens it. Risk: the first
// state of every machine read as a record that cannot be read, so the person
// is told of a missing file in place of the approval they owe. Planted:
// before the fix the refusal also matched fs.ErrNotExist, naming
// approvals.json.
func TestAFreshMachineOpensAPluginOnlyOnceApproved(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	wasm := configtest.StandinWasm(t)
	ref := model.PluginRef{Name: "standin", Version: "1.0.0"}
	bundles := &porttest.Bundles{}
	bundles.Put("/project", ref, standinManifest(wasm), wasm)
	state := filepath.Join(t.TempDir(), "state") // not made: a fresh machine
	w := world(nil, config.Config{StateDir: state})
	host, ok := pluginsOf(w.FileNames, w.Trust, bundles).(*hostplugin.Host)
	if !ok {
		t.Fatal("pluginsOf makes no *plugin.Host")
	}
	t.Cleanup(func() { _ = host.Close(context.WithoutCancel(ctx)) })

	_, err := host.Open(ctx, "/project", ref)
	if !errors.Is(err, port.ErrNotApproved) || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open before any approval = %v, want port.ErrNotApproved and no fs.ErrNotExist", err)
	}
	if _, err := os.Lstat(state); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open made the state folder %s: %v", state, err)
	}

	// The approve command's first approval: no prior yes, none
	// recorded for the name, and not the missing file.
	if _, err := w.Trust.Prior(ctx, "standin"); !errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "approvals.json") {
		t.Fatalf("Prior on a fresh machine = %v, want fs.ErrNotExist, a first approval, and no word of the file", err)
	}
	b, err := bundles.Read(ctx, "/project", ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Trust.Approve(ctx, model.Approval{Digest: b.Locked, Ref: "standin@1.0.0", Grants: contracttest.DigestOf(1)}); err != nil {
		t.Fatalf("Approve on a fresh machine = %v", err)
	}
	p, err := host.Open(ctx, "/project", ref)
	if err != nil {
		t.Fatalf("Open once approved = %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}

// TestMainsPluginHostAsksTheTrustItIsGiven: the host main makes
// judges a bundle with the engine's judgement and asks the Trust it is handed
// before it compiles anything, so a plugin nobody approved is
// port.ErrNotApproved. Risk: a host made with another Trust, or with none, so
// an approval made by gragas plugin approve is not the one the host reads.
// Planted: over bundles that hold none the Trust is never asked, so the
// count tells.
func TestMainsPluginHostAsksTheTrustItIsGiven(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ref := model.PluginRef{Name: "standin", Version: "1.0.0"}
	bundles := &porttest.Bundles{}
	bundles.Put("/project", ref, standinManifest([]byte(emptyModule)), []byte(emptyModule))
	trust := &refusing{}
	names := world(nil, config.Config{StateDir: t.TempDir()}).FileNames
	if _, err := pluginsOf(names, trust, bundles).Open(ctx, "/project", ref); !errors.Is(err, port.ErrNotApproved) {
		t.Errorf("Open of a plugin nobody approved = %v, want port.ErrNotApproved", err)
	}
	if n := trust.asked.Load(); n != 1 {
		t.Errorf("the Trust main handed the host was asked %d times, want 1", n)
	}

	t.Run("a host over no bundles never asks", func(t *testing.T) {
		t.Parallel()
		quiet := &refusing{}
		if _, err := pluginsOf(names, quiet, &porttest.Bundles{}).Open(ctx, "/project", ref); !errors.Is(err, port.ErrNotInstalled) {
			t.Errorf("Open over no bundles = %v, want port.ErrNotInstalled", err)
		}
		if n := quiet.asked.Load(); n != 0 {
			t.Errorf("the Trust was asked %d times over no bundles, want 0", n)
		}
	})
}
