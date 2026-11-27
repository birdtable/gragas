package plugin

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/birdtable/gragas/internal/port"
)

// compiling is the refusal of compile, Open's check C5, of m in a runtime made
// from cfg, or nil for a module it compiles.
func compiling(t *testing.T, cfg wazero.RuntimeConfig, m mod) error {
	t.Helper()
	return compilingBytes(t, cfg, m.bytes())
}

// compilingBytes is compiling of a module already in the binary format.
func compilingBytes(t *testing.T, cfg wazero.RuntimeConfig, wasm []byte) error {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		t.Fatal(err)
	}
	wasi := rt.Module(wasi_snapshot_preview1.ModuleName).ExportedFunctionDefinitions()
	cm, _, err := compile(ctx, rt, wasi, wasm)
	if err == nil {
		_ = cm.Close(ctx)
	}
	return err
}

// tabled is a command that does nothing, with tables of these limits.
func tabled(tables ...limits) mod {
	m := wasiMod(1, nil)
	m.tables = tables
	return m
}

// TestATablePastTheLimitIsRefusedAtOpen: a module whose table declares a least
// size past TableLimit, or a most past it, is refused by check C5, the
// refusal naming the table's index and bounds; one at the limit compiles. A
// table with no most is held to its least, since no table grows in a call.
func TestATablePastTheLimitIsRefusedAtOpen(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		table limits
		named string
	}{
		{"a least past the limit, no most", limits{least: TableLimit + 1, most: -1}, "table 0: least 16385, no most"},
		{"a least past the limit, and a most", limits{least: TableLimit + 1, most: TableLimit + 1}, "table 0: least 16385, most 16385"},
		{"a most past the limit", limits{least: 1, most: TableLimit + 1}, "table 0: least 1, most 16385"},
		{"planted: a least at the limit, no most", limits{least: TableLimit, most: -1}, ""},
		{"planted: a least and a most at the limit", limits{least: TableLimit, most: TableLimit}, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			err := compiling(t, RuntimeConfig(), tabled(row.table))
			switch {
			case row.named == "" && err != nil:
				t.Errorf("a table of %s = %v; want it compiled", table(row.table), err)
			case row.named != "" && (!errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), row.named)):
				t.Errorf("a table of %s = %v; want port.ErrModule naming %q", table(row.table), err, row.named)
			}
		})
	}
}

// TestTheGuestsTablesAreWithinTheLimit: the tables the spike's guests and
// standin.wasm declare compile, read from their table sections: standard Go
// declares one table with no most, TinyGo one of a fixed size.
func TestTheGuestsTablesAreWithinTheLimit(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		guest string
		table limits
	}{
		{"the spike's eslint, standard Go", limits{least: 6560, most: -1}},
		{"the spike's hog, standard Go", limits{least: 5965, most: -1}},
		{"standin.wasm, standard Go", limits{least: 6947, most: -1}},
		{"the spike's eslint, TinyGo", limits{least: 385, most: 385}},
		{"the spike's hog, TinyGo", limits{least: 35, most: 35}},
	} {
		if err := compiling(t, RuntimeConfig(), tabled(row.table)); err != nil {
			t.Errorf("the table of %s, %s = %v; want it compiled", row.guest, table(row.table), err)
		}
	}
}

// TestTablesAreCountedTogether: the least sizes of a module's tables are held
// to TableLimit all together, so many tables within it each cannot add up
// past it. The runtime refuses a second table already, since it has no
// reference types; the count holds without that.
func TestTablesAreCountedTogether(t *testing.T) {
	t.Parallel()
	half := table{least: TableLimit / 2, most: -1}
	if err := withinTableLimit([]table{half, half}); err != nil {
		t.Errorf("two tables of half the limit = %v; want them within it", err)
	}
	over := []table{half, {least: TableLimit/2 + 1, most: -1}}
	if err := withinTableLimit(over); !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), "table 0: least 8192, no most; table 1: least 8193, no most") {
		t.Errorf("two tables one entry past the limit = %v; want port.ErrModule naming both", err)
	}
	two := tabled(limits{least: 1, most: -1}, limits{least: 1, most: -1})
	if err := compiling(t, RuntimeConfig(), two); !errors.Is(err, port.ErrModule) {
		t.Errorf("a module of two tables = %v; want port.ErrModule", err)
	}
	_, _, tables, err := declared(two.bytes())
	if err != nil || len(tables) != 2 {
		t.Errorf("the tables of a module of two = %v, %v; want both read", tables, err)
	}
}

// TestATableGrowIsRefusedAtOpen: a module that grows a table does not compile
// in the runtime, which has no reference types, so it is refused at Open and
// never runs. Planted red: a runtime with reference types compiles it.
func TestATableGrowIsRefusedAtOpen(t *testing.T) {
	t.Parallel()
	grows := wasiMod(1, tableGrow(1<<30))
	grows.tables = []limits{{least: 1, most: -1}}
	if err := compiling(t, RuntimeConfig(), grows); !errors.Is(err, port.ErrModule) {
		t.Errorf("a module that grows a table = %v; want port.ErrModule", err)
	}
	planted := wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(api.CoreFeaturesV2)
	if err := compiling(t, planted, grows); err != nil {
		t.Errorf("planted: a runtime with reference types refused a module that grows a table: %v", err)
	}
}

// TestTheRuntimeHasWebAssembly2WithoutReferenceTypes: the runtime's features
// are exactly WebAssembly 2.0's without reference types, so that turning them
// back on, or any other change of the features, is seen here first. wazero
// keeps them in a field it does not show; reflect reads it, and a wazero that
// renames it fails this row, which the intake of a wazero version reads.
func TestTheRuntimeHasWebAssembly2WithoutReferenceTypes(t *testing.T) {
	t.Parallel()
	features := func(cfg wazero.RuntimeConfig) api.CoreFeatures {
		f := reflect.ValueOf(cfg).Elem().FieldByName("enabledFeatures")
		if !f.IsValid() {
			t.Fatal("wazero's runtime config has no enabledFeatures field")
		}
		return api.CoreFeatures(f.Uint())
	}
	want := api.CoreFeaturesV2 &^ api.CoreFeatureReferenceTypes
	if got := features(RuntimeConfig()); got != want {
		t.Errorf("the runtime's features = %s; want %s", got, want)
	}
	if got := features(RuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2)); got == want {
		t.Errorf("planted: reference types turned back on read as %s", got)
	}
}
