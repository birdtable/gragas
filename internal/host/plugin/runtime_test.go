package plugin_test

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/plugin"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// addModule is a module that exports add(i32, i32) i32.
func addModule() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
		0x03, 0x02, 0x01, 0x00,
		0x07, 0x07, 0x01, 0x03, 0x61, 0x64, 0x64, 0x00, 0x00,
		0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b,
	}
}

// TestTheRuntimeRunsAModuleInTheInterpreter: a module is compiled, instantiated
// and called through the runtime, and the answer is the module's.
func TestTheRuntimeRunsAModuleInTheInterpreter(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	rt := wazero.NewRuntimeWithConfig(ctx, plugin.RuntimeConfig())
	defer rt.Close(ctx)
	mod, err := rt.Instantiate(ctx, addModule())
	if err != nil {
		t.Fatal(err)
	}
	got, err := mod.ExportedFunction("add").Call(ctx, 2, 3)
	if err != nil || len(got) != 1 || got[0] != 5 {
		t.Fatalf("add(2, 3) = %v, %v; want [5]", got, err)
	}
}
