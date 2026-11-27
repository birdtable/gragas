package coreapi_test

import (
	"slices"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/wire"
)

// TestPluginContractDocIsTheCodecsValue: the number and the features are
// wire's, the features are sorted and a list of none is empty, not nil.
func TestPluginContractDocIsTheCodecsValue(t *testing.T) {
	t.Parallel()
	d := coreapi.PluginContractDoc()
	if d.Contract != wire.Contract {
		t.Errorf("contract = %d, want wire's %d", d.Contract, wire.Contract)
	}
	if d.Features == nil || !slices.Equal(d.Features, slices.Sorted(slices.Values(wire.Features()))) {
		t.Errorf("features = %#v, want wire's, sorted and not nil", d.Features)
	}
}
