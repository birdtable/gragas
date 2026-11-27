package coreapi_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const pluginWasm = "\x00asm\x01\x00\x00\x00"

const (
	entriesRequest = `{"body":{"config":[],"have":[],"scratch":"/s","target":"source","tool_version":"10.13.0"},"contract":1,"features":[],"op":"entries"}`
	// runPlan is a plan for run of round 0, whose answer is one run step of
	// the granted program lint-a: a step plugin test never runs.
	runPlan    = `{"body":{"config":[],"fixtures":[],"for":"run","have":[],"round":0,"scratch":"/s","target":"source","tool_version":"10.13.0"},"contract":1,"features":[],"op":"plan"}`
	runStep    = `{"ok":{"final":true,"steps":[{"argv":["--list"],"kind":"run","program":"lint-a"}]}}`
	configPlan = `{"body":{"config":[{"data":{"text":"extends ../b\n"},"path":"c.conf"}],"fixtures":[],"for":"config","have":[],"round":0,"scratch":"/s","target":"source","tool_version":"10.13.0"},"contract":1,"features":[],"op":"plan"}`
	entryB     = `{"key":"b","on":true,"options":null,"severity":"error","value":1}`
	entryA     = `{"key":"a","on":true,"options":null,"severity":"error","value":1}`
)

// pluginBundle is the files of the bundle plugins/lint-a/2.0.0 that pass every
// check this command runs, with the module pluginModule scripts.
func pluginBundle() map[string]string {
	return map[string]string{
		"plugin.yaml": `contract: 1
needs: []
name: lint-a
version: 2.0.0
tool: lint-a
tool_versions: ">=10.13.0 <11.0.0"
targets: [source]
provides: [entries, plan]
extends: false
warning: false
programs:
  - id: lint-a
    path: bin/lint-a
    version_argv: [--version]
    versions: ">=10.13.0 <11.0.0"
wasm: ` + digest.Prefixed([]byte(pluginWasm)) + "\n",
		"plugin.wasm":                            pluginWasm,
		"testdata/cases/1-entries/request.json":  entriesRequest,
		"testdata/cases/1-entries/response.json": `{"ok":{"entries":[]}}`,
		"testdata/cases/2-plan/request.json":     runPlan,
		"testdata/cases/2-plan/response.json":    runStep,
		"testdata/cases/3-plan/request.json":     configPlan,
		"testdata/cases/3-plan/response.json":    `{"error":{"code":"refused","message":"no","path":"/config/0/data"}}`,
		"testdata/recordings/10.13.0/out.txt":    "raw output",
	}
}

// inRange is the tool versions the module serves.
func inRange(v string) bool { return strings.HasPrefix(v, "10.") }

func unsupported() error { return &plugin.CallError{Code: plugin.CodeUnsupported, Message: "no"} }

// typed is a module's answer to every request that is no case's: the probes
// of C14 to C17.
func typed() *porttest.StandinPlugin {
	return &porttest.StandinPlugin{
		OnPlan: func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
			if in.Target != plugin.TargetSource || !in.For.Valid() || !inRange(in.ToolVersion) {
				return plugin.PlanResponse{}, unsupported()
			}
			return plugin.PlanResponse{Final: true, Steps: []plugin.Step{}}, nil
		},
		OnEntries: func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			if in.Target != plugin.TargetSource || !inRange(in.ToolVersion) {
				return plugin.EntriesResponse{}, unsupported()
			}
			return plugin.EntriesResponse{Entries: []plugin.Entry{}}, nil
		},
	}
}

// unknownField is the member C12 adds to a case's body.
const unknownField = "unknown_to_the_contract"

// rawCall is one call of a case's request: the case, whether its body held
// unknownField, and how many calls of that request there were, this one
// counted.
type rawCall struct {
	name    string
	unknown bool
	n       int
}

// pluginModule is the module the bundle's files were made from: a case's
// request, with or without unknownField, is answered with what files saves
// for it, and any other request by typed. answer, when set, plants another
// answer to a case.
func pluginModule(t *testing.T, files map[string]string, answer func(a rawCall, saved []byte) ([]byte, error)) *porttest.StandinPlugin {
	t.Helper()
	other, err := (&porttest.Standin{Trial: typed()}).Try(t.Context(), plugin.Manifest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved := map[string]string{}
	for p, request := range files {
		name, ok := strings.CutSuffix(strings.TrimPrefix(p, "testdata/cases/"), "/request.json")
		if ok {
			if key, _, isKey := keyOf([]byte(request)); isKey {
				saved[key] = name
			}
		}
	}
	var mu sync.Mutex
	seen := map[string]int{}
	p := typed()
	p.OnRaw = func(request []byte) ([]byte, error) {
		key, unknown, ok := keyOf(request)
		name, isCase := saved[key]
		if !ok || !isCase {
			return other.Raw(context.Background(), request)
		}
		mu.Lock()
		seen[key]++
		n := seen[key]
		mu.Unlock()
		out := []byte(files["testdata/cases/"+name+"/response.json"])
		if answer != nil {
			return answer(rawCall{name: name, unknown: unknown, n: n}, out)
		}
		return out, nil
	}
	return p
}

// keyOf is a request in canonical form with unknownField taken out of its
// body, and whether the body held it.
func keyOf(request []byte) (key string, unknown, ok bool) {
	v, err := canon.Decode(request)
	if err != nil {
		return "", false, false
	}
	o, isObject := v.(canon.Object)
	body, isBody := o["body"].(canon.Object)
	if !isObject || !isBody {
		return "", false, false
	}
	_, unknown = body[unknownField]
	body = maps.Clone(body)
	delete(body, unknownField)
	o = maps.Clone(o)
	o["body"] = body
	b, err := canon.Encode(o, canon.Canonical)
	return string(b), unknown, err == nil
}

// pluginRun runs plugin test over files, the module scripted by module (none
// is a module the host refuses), and answers the run and the ports it used.
func pluginRun(t *testing.T, files map[string]string, module *porttest.StandinPlugin) (model.PluginTest, *porttest.Standin) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.Seed("plugins/lint-a/2.0.0", files)
	w.PluginsOf().Trial = module
	got, err := coreapi.PluginTest(t.Context(), w.Ports(), "plugins/lint-a/2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	return got, w.PluginsOf()
}

func failedChecks(p model.PluginTest) []int {
	var out []int
	for _, c := range p.Checks {
		if c.Status == model.CheckFailed {
			out = append(out, c.N)
		}
	}
	return out
}

// TestPluginTestReadsABundleThroughThePort: the files of a good bundle, its
// module tried through the port, pass all 23 checks or are not asked them,
// and the run is done; the engine's own contract number and features are the
// ones the checks are held to.
func TestPluginTestReadsABundleThroughThePort(t *testing.T) {
	t.Parallel()
	files := pluginBundle()
	got, plugins := pluginRun(t, files, pluginModule(t, files, nil))
	if got.Word() != model.Done || !slices.Equal(got.Cases, []string{"1-entries", "2-plan", "3-plan"}) {
		t.Fatalf("PluginTest = %+v, want done and the three cases", got)
	}
	if len(got.Checks) != 23 {
		t.Errorf("%d checks, want C1 to C23", len(got.Checks))
	}
	for _, c := range got.Checks {
		if c.N >= 5 && c.N <= 17 && c.Status != model.CheckPassed {
			t.Errorf("C%d is %s %v, want passed: the module is tried and called", c.N, c.Status, c.What)
		}
	}
	if len(plugins.Tried) != 1 || string(plugins.Tried[0]) != pluginWasm {
		t.Errorf("the host tried %d modules, want the bundle's one", len(plugins.Tried))
	}
}

// TestPluginTestNamesTheCheckThatFails: a file changed in the bundle, or a
// module that answers otherwise than its files say, fails the one check that
// holds it, read through the engine's own codec (core/wire).
func TestPluginTestNamesTheCheckThatFails(t *testing.T) {
	t.Parallel()
	answers := func(name string, f func(a rawCall, saved []byte) ([]byte, error)) func(a rawCall, saved []byte) ([]byte, error) {
		return func(a rawCall, saved []byte) ([]byte, error) {
			if a.name != name {
				return saved, nil
			}
			return f(a, saved)
		}
	}
	for _, c := range []struct {
		name   string
		change func(map[string]string)
		answer func(a rawCall, saved []byte) ([]byte, error)
		// refused is a module the host refuses (port.ErrModule).
		refused bool
		want    []int
		said    string
	}{
		{name: "a need the engine lacks", change: func(f map[string]string) {
			f["plugin.yaml"] = strings.Replace(f["plugin.yaml"], "needs: []", "needs: [describe]", 1)
		}, want: []int{2}, said: "describe"},
		{name: "a module changed", change: func(f map[string]string) { f["plugin.wasm"] += "x" }, want: []int{3}, said: "plugin.wasm is sha256:"},
		{name: "a request refused", change: func(f map[string]string) { f["testdata/cases/1-entries/request.json"] = `{}` }, want: []int{6, 18}, said: "1-entries"},
		{name: "a case without its answer", change: func(f map[string]string) {
			delete(f, "testdata/cases/1-entries/response.json")
		}, want: []int{6, 18}, said: "response.json"},
		{name: "no recording", change: func(f map[string]string) { delete(f, "testdata/recordings/10.13.0/out.txt") }, want: []int{18}, said: "recording"},
		{name: "a module that imports what WASI preview 1 does not hold", refused: true, want: []int{5}, said: "scripts no module"},
		{name: "a call that ends in a trap", answer: answers("1-entries", func(rawCall, []byte) ([]byte, error) {
			return nil, port.ErrTrap
		}), want: []int{7}, said: "exit code 0"},
		{name: "stdout that is no response", answer: answers("1-entries", func(rawCall, []byte) ([]byte, error) {
			return []byte(`{"ok":{"entries":"none"}}`), nil
		}), want: []int{8}, said: "is no answer to entries"},
		{name: "entries out of order", change: func(f map[string]string) {
			f["testdata/cases/1-entries/response.json"] = `{"ok":{"entries":[` + entryB + `,` + entryA + `]}}`
		}, want: []int{9}, said: "out of order"},
		{name: "an answer that is not the saved one", answer: answers("1-entries", func(rawCall, []byte) ([]byte, error) {
			return []byte(`{"ok":{"entries":[` + entryA + `]}}`), nil
		}), want: []int{10}, said: "is not response.json"},
		{name: "a second run with other bytes", answer: answers("1-entries", func(a rawCall, saved []byte) ([]byte, error) {
			if a.n == 2 && !a.unknown {
				return append(slices.Clone(saved), ' '), nil
			}
			return saved, nil
		}), want: []int{11}, said: "other bytes"},
		{name: "an unknown field that changes the answer", answer: answers("1-entries", func(a rawCall, saved []byte) ([]byte, error) {
			if a.unknown {
				return []byte(`{"ok":{"entries":[` + entryA + `]}}`), nil
			}
			return saved, nil
		}), want: []int{12}, said: unknownField},
		{name: "a run step of a program no grant names", change: func(f map[string]string) {
			f["testdata/cases/2-plan/response.json"] = strings.Replace(runStep, `"program":"lint-a"`, `"program":"curl"`, 1)
		}, want: []int{13}, said: "/ok/steps/0/program"},
		{name: "a read step outside the scratch folder", change: func(f map[string]string) {
			f["testdata/cases/2-plan/response.json"] = `{"ok":{"final":true,"steps":[{"glob":"../etc/*","kind":"read"}]}}`
		}, want: []int{13}, said: "/ok/steps/0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := pluginBundle()
			if c.change != nil {
				c.change(f)
			}
			var module *porttest.StandinPlugin
			if !c.refused {
				module = pluginModule(t, f, c.answer)
			}
			got, _ := pluginRun(t, f, module)
			if failed := failedChecks(got); !slices.Equal(failed, c.want) {
				t.Errorf("failed %v, want %v", failed, c.want)
			}
			for _, check := range got.Checks {
				if check.N == c.want[0] && !strings.Contains(strings.Join(check.What, "\n"), c.said) {
					t.Errorf("C%d says %v, want it to say %q", check.N, check.What, c.said)
				}
			}
		})
	}
}

// countedSteps is a Steps port that counts the runners bound: the one way a
// step of a plan is run.
type countedSteps struct {
	port.Steps
	bound *int
}

func (c countedSteps) Bind(ctx context.Context, s plugin.Scope) (port.StepRunner, error) {
	*c.bound++
	return c.Steps.Bind(ctx, s)
}

// TestPluginTestRunsNoStepOfAPlan: the good bundle's plan case answers a run
// step of its granted program, the answer is held to C13 and passes, and no
// step runner is bound, so nothing of it is started: plugin test runs the
// module in the sandbox, and none of its steps.
func TestPluginTestRunsNoStepOfAPlan(t *testing.T) {
	t.Parallel()
	files := pluginBundle()
	w := porttest.NewWorld(t)
	w.Seed("plugins/lint-a/2.0.0", files)
	w.SeedStep("lint-a", plugin.Result{Kind: plugin.KindRun})
	w.PluginsOf().Trial = pluginModule(t, files, nil)
	ports := w.Ports()
	bound := 0
	ports.Steps = countedSteps{Steps: ports.Steps, bound: &bound}
	got, err := coreapi.PluginTest(t.Context(), ports, "plugins/lint-a/2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if c := got.Checks[12]; c.N != 13 || c.Status != model.CheckPassed {
		t.Fatalf("C13 is %+v, want passed for the granted run step", c)
	}
	if bound != 0 {
		t.Errorf("plugin test bound %d step runners, want none", bound)
	}
}

// TestAHostErrorThatIsNoRefusalIsTheOperations: a try that fails for another
// reason than the module is no answer of C5; it ends the operation.
func TestAHostErrorThatIsNoRefusalIsTheOperations(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Seed("plugins/lint-a/2.0.0", pluginBundle())
	broken := typed()
	broken.OpenErr = errors.New("the runtime could not start")
	w.PluginsOf().Trial = broken
	if _, err := coreapi.PluginTest(t.Context(), w.Ports(), "plugins/lint-a/2.0.0"); err == nil || !strings.Contains(err.Error(), "could not be tried") {
		t.Errorf("PluginTest = %v, want the try's error", err)
	}
}
