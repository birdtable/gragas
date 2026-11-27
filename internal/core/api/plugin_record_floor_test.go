package coreapi_test

import (
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// TestARecordRunsAtTheFloorItsConfigsGive: a record prepares its
// scratch folder at the floor configFloor gives its configs, as the entries
// call does, and holds its steps to it: a config that loads code
// (eslint.config.js) runs sandboxed, so a folder that reached only isolated
// refuses the step with the entries call's reason, and a data config (JSON,
// or none) still runs isolated. The highest floor of the configs decides. A
// plugin whose manifest says config_loads_code has its data config recorded
// sandboxed too, as its entries call reads it. The plant is a
// record that prepares at model.Isolated whatever the configs, which runs the
// project's code below the floor it is held to.
// Risk: a recording whose tool loads the repository's code at isolated.
func TestARecordRunsAtTheFloorItsConfigsGive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		configs []string
		reached model.Level
		level   model.Level
		refused bool
		loads   bool
	}{
		{"code config, sandbox available", []string{"eslint.config.js"}, model.Sandboxed, model.Sandboxed, false, false},
		{"code config, only isolated", []string{"eslint.config.js"}, model.Isolated, model.Sandboxed, true, false},
		{"json config", []string{"a.json"}, model.Isolated, model.Isolated, false, false},
		{"no config", nil, model.Isolated, model.Isolated, false, false},
		{"json then code, the highest decides", []string{"a.json", "eslint.config.js"}, model.Isolated, model.Sandboxed, true, false},
		{"code then json, the last does not decide", []string{"eslint.config.js", "a.json"}, model.Isolated, model.Sandboxed, true, false},
		{"json config of a plugin whose configs load code, only isolated", []string{"a.json"}, model.Isolated, model.Sandboxed, true, true},
		{"json config of a plugin whose configs load code, sandbox available", []string{"a.json"}, model.Sandboxed, model.Sandboxed, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, root := recordWorld(t, func(plugin.PlanRequest) (plugin.PlanResponse, error) {
				return plugin.PlanResponse{Final: true, Steps: []plugin.Step{{Run: &plugin.Run{Program: "t", Argv: []string{"--list"}}}}}, nil
			}, func(m *plugin.Manifest) { m.ConfigLoadsCode = &tc.loads })
			w.SeedStep("t", plugin.Result{Kind: plugin.KindRun, Exit: new(0), Cut: new(false), Files: []plugin.File{}, Stdout: &plugin.Bytes{Text: new("")}, Stderr: &plugin.Bytes{Text: new("")}})
			files := map[string]string{}
			for _, c := range tc.configs {
				files[c] = "{}\n"
			}
			w.Seed("/repo", files)
			ports := w.Ports()
			sb := &scriptedSandbox{level: tc.reached}
			ports.Sandbox = sb
			ask := coreapi.PluginRecordAsk{Ask: changeAsk(root, "gragas plugin record author/p/1.0.0"), Dir: "/repo/author/p/1.0.0", Op: "entries", Apply: true, Config: tc.configs}
			_, ref, err := coreapi.PluginRecord(t.Context(), ports, ask)
			if err != nil {
				t.Fatalf("PluginRecord: %v", err)
			}
			if len(sb.prepared) != 1 || sb.prepared[0].Level != tc.level {
				t.Errorf("prepared %+v, want one folder at %s", sb.prepared, tc.level)
			}
			switch {
			case tc.refused && (ref == nil || !strings.Contains(fmt.Sprint(ref.Observed()), "scratch folder reached isolated")):
				t.Errorf("refusal = %+v, want the step refused: it runs at sandboxed and the folder reached isolated", ref)
			case !tc.refused && ref != nil:
				t.Errorf("refusal = %+v, want the recording to run", ref)
			}
		})
	}
}
