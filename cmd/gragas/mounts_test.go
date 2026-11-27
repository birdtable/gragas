package main

import (
	"testing"

	"github.com/birdtable/gragas/internal/config"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// TestTheMountsFixNamesTheSettingConfigReads: host/trust's fix
// lines name the variable and the key a person sets to let a disk be judged,
// and config is what reads them; the two packages may not import each other,
// so main, which wires both, holds the names equal. Risk: a fix line that
// names a variable or a key gragas never reads, so the person's setting does
// nothing. Planted failure: either name renamed on one side.
func TestTheMountsFixNamesTheSettingConfigReads(t *testing.T) {
	t.Parallel()
	if config.MountsVar != hosttrust.MountsVar {
		t.Errorf("config reads %q, and the fix line names %q", config.MountsVar, hosttrust.MountsVar)
	}
	found := 0
	for _, k := range config.Keys() {
		if k.Name != hosttrust.MountsKey {
			continue
		}
		found++
		if k.Env != hosttrust.MountsVar {
			t.Errorf("the key %s is overridden by %q, and the fix line names %q", k.Name, k.Env, hosttrust.MountsVar)
		}
	}
	if found != 1 {
		t.Errorf("config has %d keys named %q, the key the fix line names; want 1", found, hosttrust.MountsKey)
	}
}
