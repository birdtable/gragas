package plugin_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/plugin"
)

// TestUpgradeHandsTheRunItsSourceAndRefAndWritesOnlyWithApply: the two
// arguments reach the run in order, one or three is a usage error that runs
// nothing, and the command carries the writes annotation and takes --apply.
// Risk: a writer that is not gated, or a source and ref swapped.
func TestUpgradeHandsTheRunItsSourceAndRefAndWritesOnlyWithApply(t *testing.T) {
	t.Parallel()
	s := &session{}
	run(t, s, "upgrade", "../plugins", "standin@1.1.0")
	if len(s.upgraded) != 1 || s.upgraded[0] != "../plugins standin@1.1.0" {
		t.Errorf("upgrade handed the run %q, want the source and the ref", s.upgraded)
	}
	for _, args := range [][]string{{"upgrade", "../plugins"}, {"upgrade", "a", "b", "c"}} {
		bad := &session{}
		c := plugin.New(bad)
		c.SetArgs(args)
		c.SilenceUsage, c.SilenceErrors = true, true
		if err := c.Execute(); err == nil || len(bad.upgraded) != 0 {
			t.Errorf("%v: err %v, ran %q; want a usage error and no run", args, err, bad.upgraded)
		}
	}
	for _, sub := range plugin.New(&session{}).Commands() {
		if sub.Name() != "upgrade" {
			continue
		}
		if sub.Annotations["writes"] != "with --apply" || sub.Flags().Lookup("apply") == nil {
			t.Errorf("upgrade: writes %q, apply flag %v; want the annotation and the flag", sub.Annotations["writes"], sub.Flags().Lookup("apply"))
		}
		return
	}
	t.Error("plugin has no upgrade")
}
