package plugin_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/plugin"
)

// TestAddHandsTheRunItsFolderAndWritesOnlyWithApply: the one argument reaches
// the run, none or two is a usage error that runs nothing, and the command
// carries the writes annotation and takes --apply. Risk: a writer that is not
// gated, or a folder that never arrives.
func TestAddHandsTheRunItsFolderAndWritesOnlyWithApply(t *testing.T) {
	t.Parallel()
	s := &session{}
	run(t, s, "add", "plugins/alpha/1.0.0")
	if len(s.added) != 1 || s.added[0] != "plugins/alpha/1.0.0" {
		t.Errorf("add handed the run %q, want the folder", s.added)
	}
	for _, args := range [][]string{{"add"}, {"add", "a", "b"}} {
		bad := &session{}
		c := plugin.New(bad)
		c.SetArgs(args)
		c.SilenceUsage, c.SilenceErrors = true, true
		if err := c.Execute(); err == nil || len(bad.added) != 0 {
			t.Errorf("%v: err %v, ran %q; want a usage error and no run", args, err, bad.added)
		}
	}
	for _, sub := range plugin.New(&session{}).Commands() {
		if sub.Name() != "add" {
			continue
		}
		if sub.Annotations["writes"] != "with --apply" || sub.Flags().Lookup("apply") == nil {
			t.Errorf("add: writes %q, apply flag %v; want the annotation and the flag", sub.Annotations["writes"], sub.Flags().Lookup("apply"))
		}
		return
	}
	t.Error("plugin has no add")
}
