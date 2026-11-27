package plugintest_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/plugintest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const (
	v10   = `"tool_version":"10.13.0"`
	entry = `{"key":"k","on":true,"options":null,"severity":"error","value":1}`
	// ran is the recorded result of a run of the tool, as an entries case holds it.
	ran      = `{"cut":false,"exit":0,"files":[],"kind":"run","round":0,"step":0,"stderr":{"text":""},"stdout":{"text":"out"}}`
	presetIn = `{"key":"k","options":null,"rules":["r"],"severity":"error","target":"source","trial":false,"value":1}`
)

func env(op, body string) string {
	return `{"body":` + body + `,"contract":1,"features":[],"op":"` + op + `"}`
}

func kase(name, op, body, resp string) plugintest.Case {
	return plugintest.Case{Name: name, Request: []byte(env(op, body)), Response: []byte(resp)}
}

func ok(body string) string { return `{"ok":` + body + `}` }

func refused(path string) string {
	return `{"error":{"code":"refused","message":"no","path":"` + path + `"}}`
}

func pair(a, b string) string {
	return `{"aspect":"value","found":` + b + `,"key":"k","preset":` + a + `}`
}

// cases are the cases of a bundle that holds the evidence of every operation.
// No two send the same request, as a module answers one request one way
// (C10, C11).
func cases() []plugintest.Case {
	cfg := func(scratch string) string { return `"config":[],"have":[],"scratch":"` + scratch + `"` }
	optioned := strings.Replace(presetIn, `"options":null`, `"options":{"x":1}`, 1)
	return []plugintest.Case{
		kase("1-entries", "entries", `{"config":[],"have":[`+ran+`],"scratch":"/s","target":"source",`+v10+`}`, ok(`{"entries":[`+entry+`]}`)),
		kase("2-findings", "findings", `{`+cfg("/s2")+`,"fixtures":[],`+v10+`}`,
			ok(`{"findings":[{"key":"k","line":1,"message":"m","path":"a.go","severity":"error"}],"ran":true}`)),
		kase("3-findings", "findings", `{`+cfg("/s3")+`,"fixtures":[],`+v10+`}`, ok(`{"findings":[],"ran":true}`)),
		kase("4-findings", "findings", `{`+cfg("/s4")+`,"fixtures":[],`+v10+`}`, ok(`{"findings":[],"ran":false,"why":"the tool failed"}`)),
		kase("5-plan", "plan", `{`+cfg("/s")+`,"fixtures":[],"for":"run","round":0,"target":"source",`+v10+`}`, ok(`{"final":true,"steps":[]}`)),
		kase("6-plan", "plan", `{"config":[{"data":{"text":"extends ../b\n"},"path":"c.conf"}],"have":[],"scratch":"/s","fixtures":[],"for":"config","round":0,"target":"source",`+v10+`}`, refused("/config/0/data")),
		kase("7-preset", "preset", `{"entries":[`+presetIn+`],"inputs":[],"mark":"MARK",`+v10+`}`,
			ok(`{"files":[{"path":"p.yml","text":"# MARK\nk: 1\n"}],"notes":[]}`)),
		kase("8-preset", "preset", `{"entries":[`+optioned+`],"inputs":[],"mark":"MARK",`+v10+`}`, refused("/entries/0/options")),
		kase("9-rank", "rank", `{"pairs":[`+pair("1", "2")+`,`+pair("2", "1")+`,`+pair("1", "1")+`],`+v10+`}`,
			ok(`{"words":["tighter","weaker","same"]}`)),
	}
}

func fullManifest(replace ...string) []byte {
	return manifest(append([]string{"provides: [entries]", "provides: [entries, findings, plan, rank, preset]"}, replace...)...)
}

func full() plugintest.Bundle {
	b := good()
	b.Manifest = fullManifest()
	b.Cases = cases()
	b.Recordings = []plugintest.Recording{{Version: "10.13.0", Files: []string{"out.txt"}}}
	return b
}

// inRange is the tool versions the stand-in serves.
func inRange(v string) bool { return strings.HasPrefix(v, "10.") }

func unsupported() error { return &plugin.CallError{Code: plugin.CodeUnsupported, Message: "no"} }

// standin is a plugin that answers what a good bundle's cases say.
func standin() *porttest.StandinPlugin {
	return &porttest.StandinPlugin{
		OnPlan: func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
			if in.Target != plugin.TargetSource || !in.For.Valid() || !inRange(in.ToolVersion) {
				return plugin.PlanResponse{}, unsupported()
			}
			return plugin.PlanResponse{Final: true}, nil
		},
		OnEntries: func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			if in.Target != plugin.TargetSource || !inRange(in.ToolVersion) {
				return plugin.EntriesResponse{}, unsupported()
			}
			if len(in.Config) == 0 {
				return plugin.EntriesResponse{Entries: []plugin.Entry{}}, nil
			}
			sev := plugin.Severity("error")
			return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "k", On: true, Severity: &sev, Value: canonInt(1)}}}, nil
		},
	}
}

func opened(t *testing.T, p *porttest.StandinPlugin) plugintest.Caller {
	t.Helper()
	s := &porttest.Standin{}
	ref := model.PluginRef{Name: "lint-a", Version: "2.0.0"}
	s.Install("r", ref, p)
	c, err := s.Open(context.Background(), "r", ref)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// withEngine is the test engine with a range and a codec: a request in
// canonical form, and an answer its rules take as it is.
func withEngine() plugintest.Engine {
	e := engine()
	e.Ends = func(string) ([]string, []string, error) { return []string{"10.13.0"}, []string{"0.0.0", "11.0.0"}, nil }
	e.Encode = encode
	e.Rules = func(plugin.Request, []byte, []string) error { return nil }
	return e
}

func run(t *testing.T, b plugintest.Bundle) map[int]model.PluginCheck {
	t.Helper()
	return runWith(t, withEngine(), b)
}

func withPlugin(t *testing.T) plugintest.Bundle {
	t.Helper()
	b := full()
	b.Plugin = answer(b.Cases, opened(t, standin()))
	return b
}

// TestTheStandinPassesEveryCheck: a bundle with the evidence of every
// operation and a plugin that answers it passes all 23.
func TestTheStandinPassesEveryCheck(t *testing.T) {
	t.Parallel()
	got := run(t, withPlugin(t))
	for n := 1; n <= plugintest.Checks; n++ {
		if got[n].Status != model.CheckPassed {
			t.Errorf("C%d is %s %v, want passed", n, got[n].Status, got[n].What)
		}
	}
}

// TestWithNoPluginTheChecksThatCallItAreNotRun: C5, C7 to C17 and the round
// trip of C23 need the opened module, and with none they say why and never
// pass: a module the host refused points at C5.
func TestWithNoPluginTheChecksThatCallItAreNotRun(t *testing.T) {
	t.Parallel()
	calling := []int{5, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 23}
	got := run(t, full())
	for _, n := range calling {
		if got[n].Status != model.CheckNotRun || !strings.Contains(strings.Join(got[n].What, ""), "was not opened") {
			t.Errorf("C%d is %s %v, want not run saying the module was not opened", n, got[n].Status, got[n].What)
		}
	}
	refused := full()
	refused.Refused = "the module imports env.clock, which is not WASI preview 1"
	byRefusal := run(t, refused)
	for _, n := range calling[1:] {
		if byRefusal[n].Status != model.CheckNotRun || !strings.Contains(strings.Join(byRefusal[n].What, ""), "see C5") {
			t.Errorf("with a refused module C%d is %s %v, want not run pointing at C5", n, byRefusal[n].Status, byRefusal[n].What)
		}
	}
	for n := 18; n <= 22; n++ {
		if got[n].Status != model.CheckPassed {
			t.Errorf("C%d is %s %v, want passed", n, got[n].Status, got[n].What)
		}
	}
}

// TestEachPlantedBundleFailsItsCheckAndNoOther: for each of C14 to C23, a bundle
// that fails that check, red with its number, and every other of the 23 as the
// good bundle has it.
func TestEachPlantedBundleFailsItsBehaviourOrEvidenceCheckAndNoOther(t *testing.T) {
	t.Parallel()
	without := func(prefix string) func(*plugintest.Bundle) {
		return func(b *plugintest.Bundle) {
			b.Cases = slices.DeleteFunc(b.Cases, func(c plugintest.Case) bool { return strings.HasPrefix(c.Name, prefix) })
		}
	}
	script := func(f func(*porttest.StandinPlugin)) func(*plugintest.Bundle) {
		return func(b *plugintest.Bundle) { p := standin(); f(p); b.Plugin = answer(b.Cases, opened(t, p)) }
	}
	replace := func(name, from, to string) func(*plugintest.Bundle) {
		return func(b *plugintest.Bundle) {
			for i, c := range b.Cases {
				if c.Name == name {
					b.Cases[i].Response = []byte(strings.Replace(string(c.Response), from, to, 1))
					if from == "" {
						b.Cases[i].Response = []byte(to)
					}
				}
			}
		}
	}
	// request is a plant that changes the request of the case name.
	request := func(name, from, to string) func(*plugintest.Bundle) {
		return func(b *plugintest.Bundle) {
			for i, c := range b.Cases {
				if c.Name == name {
					b.Cases[i].Request = []byte(strings.Replace(string(c.Request), from, to, 1))
				}
			}
		}
	}
	for _, c := range []struct {
		name, said string
		n          int
		plant      func(*plugintest.Bundle)
	}{
		{"an unknown target that is served", "unknown target", 14, script(func(p *porttest.StandinPlugin) {
			on := p.OnEntries
			p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
				if in.Target == "nowhere" {
					return plugin.EntriesResponse{}, nil
				}
				return on(in)
			}
		})},
		{"an unknown for that is served", "unknown for", 14, script(func(p *porttest.StandinPlugin) {
			p.OnPlan = func(plugin.PlanRequest) (plugin.PlanResponse, error) { return plugin.PlanResponse{}, nil }
		})},
		{"an operation the plugin does not provide, served", "findings, which provides leaves out", 15, func(b *plugintest.Bundle) {
			b.Manifest = fullManifest("provides: [entries, findings, plan, rank, preset]", "provides: [entries, plan, rank, preset]")
			p := standin()
			p.OnFindings = func(plugin.FindingsRequest) (plugin.FindingsResponse, error) { return plugin.FindingsResponse{}, nil }
			b.Plugin = answer(b.Cases, opened(t, p))
			without("2-")(b)
			without("3-")(b)
			without("4-")(b)
		}},
		{"a version outside the range that is served", "outside the range", 16, script(func(p *porttest.StandinPlugin) {
			on := p.OnEntries
			p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
				if !inRange(in.ToolVersion) {
					return plugin.EntriesResponse{}, nil
				}
				return on(in)
			}
		})},
		{"an end of the range that is not served", "does not serve it", 16, script(func(p *porttest.StandinPlugin) {
			on := p.OnEntries
			p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
				if in.ToolVersion == "10.13.0" && in.Target == plugin.TargetSource && len(in.Config) == 0 {
					return plugin.EntriesResponse{}, unsupported()
				}
				return on(in)
			}
		})},
		{"a malformed request that is served", "ended in success", 17, func(b *plugintest.Bundle) {
			a := answer(b.Cases, opened(t, standin()))
			a.lenient = true
			b.Plugin = a
		}},
		{"no recording for a tool version tested", "holds no recording", 18, func(b *plugintest.Bundle) { b.Recordings = nil }},
		{"a recording folder with no file", "holds no recording", 18, func(b *plugintest.Bundle) { b.Recordings[0].Files = nil }},
		{"no planted findings", "planted output", 19, replace("2-findings", `"findings":[{"key":"k","line":1,"message":"m","path":"a.go","severity":"error"}]`, `"findings":[]`)},
		{"no clean control", "clean control", 19, without("3-")},
		{"no tool failure", "the tool failed", 19, without("4-")},
		{"no one-round plan", "one round", 20, replace("5-plan", `"final":true`, `"final":false`)},
		{"a plan final at round 1 is no one-round plan", "one round", 20, request("5-plan", `"round":0`, `"round":1`)},
		{"no refused plan with a path", "refuses a config", 20, replace("6-plan", "", ok(`{"final":true,"steps":[]}`))},
		{"a refusal of the file name and not its content", "by its content", 20, replace("6-plan", `"/config/0/data"`, `"/config/0/path"`)},
		{"a refusal of a config given with no data", "by its content", 20, request("6-plan", `{"data":{"text":"extends ../b\n"},"path":"c.conf"}`, `{"path":"c.conf"}`)},
		{"a refusal after a step", "by its content", 20, request("6-plan", `"round":0`, `"round":1`)},
		{"no preset golden", "golden", 21, without("7-")},
		{"a golden with no mark on the first line", "the mark", 21, replace("7-preset", "# MARK", "# other")},
		{"no refused option", "refuses an option", 21, without("8-")},
		{"no pair answered tighter", "answered tighter", 22, replace("9-rank", `"tighter","weaker"`, `"same","same"`)},
		{"a swap that does not swap", "swapped it answers", 22, replace("9-rank", `"tighter","weaker"`, `"tighter","tighter"`)},
		{"a pair with no swapped pair", "no swapped pair", 22, func(b *plugintest.Bundle) {
			b.Cases[8] = kase("9-rank", "rank", `{"pairs":[`+pair("1", "2")+`,`+pair("1", "1")+`,`+pair("3", "3")+`],`+v10+`}`, ok(`{"words":["tighter","same","same"]}`))
		}},
		{"a preset that is read back with another value", "another value", 23, script(func(p *porttest.StandinPlugin) {
			on := p.OnEntries
			p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
				r, err := on(in)
				for i := range r.Entries {
					r.Entries[i].Value = canonInt(2)
				}
				return r, err
			}
		})},
		{"an entry that is not in the preset", "not in the preset", 23, script(func(p *porttest.StandinPlugin) {
			on := p.OnEntries
			p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
				r, err := on(in)
				if len(in.Config) > 0 {
					r.Entries = append(r.Entries, plugin.Entry{Key: "extra", On: true, Value: canonInt(1)})
				}
				return r, err
			}
		})},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := withPlugin(t)
			c.plant(&b)
			got := run(t, resave(b))
			for n := 1; n <= plugintest.Checks; n++ {
				switch {
				case n == c.n:
					if got[n].Status != model.CheckFailed || !strings.Contains(strings.Join(got[n].What, "\n"), c.said) {
						t.Errorf("C%d is %s %v, want failed and saying %q", n, got[n].Status, got[n].What, c.said)
					}
				case got[n].Status == model.CheckFailed:
					t.Errorf("C%d failed %v: the plant fails C%d alone", n, got[n].What, c.n)
				}
			}
		})
	}
}

func canonInt(n int64) canon.Value { return canon.Int(n) }

// unknownField is the member C12 adds to a case's body.
const unknownField = "unknown_to_the_contract"

// call is one raw call answering saw for a case: the case, whether its body
// held unknownField, and how many calls of that case there were, this one
// counted.
type call struct {
	name    string
	unknown bool
	n       int
}

// answering is a plugin that answers each case's request, with or without
// unknownField in its body, with the case's saved response, as the module the
// bundle was made from would, and hands every other request to the plugin it
// wraps. A plant scripts one way to fault it.
type answering struct {
	plugintest.Caller
	names   map[string]string
	answers map[string][]byte
	seen    map[string]int
	// plant, when set, is the answer to a case's call, given the saved one.
	plant func(c call, saved []byte) ([]byte, error)
	// lenient answers success to every request that is no case's but the one
	// of an unknown operation, which it leaves to the plugin it wraps.
	lenient bool
}

// answer is p behind the saved answers of cases.
func answer(cases []plugintest.Case, p plugintest.Caller) *answering {
	a := &answering{Caller: p}
	a.save(cases)
	return a
}

// save makes cases' saved answers the ones a answers, as a module built with
// the bundle answers them: a plant that changes a case changes both.
func (a *answering) save(cases []plugintest.Case) {
	a.names, a.answers, a.seen = map[string]string{}, map[string][]byte{}, map[string]int{}
	for _, c := range cases {
		if key, _, ok := keyOf(c.Request); ok {
			a.names[key] = c.Name
			a.answers[c.Name] = c.Response
		}
	}
}

// resave is b with its plugin, where it answers saved answers, answering b's
// cases as they now are.
func resave(b plugintest.Bundle) plugintest.Bundle {
	if a, ok := b.Plugin.(*answering); ok {
		a.save(b.Cases)
	}
	return b
}

func (a *answering) Raw(ctx context.Context, req []byte) ([]byte, error) {
	key, unknown, ok := keyOf(req)
	name, saved := a.names[key]
	switch {
	case ok && saved:
		a.seen[name]++
		if a.plant != nil {
			return a.plant(call{name: name, unknown: unknown, n: a.seen[name]}, a.answers[name])
		}
		return a.answers[name], nil
	case a.lenient && !strings.Contains(string(req), `"nothing"`):
		return []byte(`{"ok":{}}`), nil
	}
	return a.Caller.(plugintest.Raw).Raw(ctx, req)
}

// keyOf is a request in canonical form with unknownField taken out of its
// body, and whether the body held it; ok is false for a request that is no
// envelope with a body.
func keyOf(req []byte) (key string, unknown, ok bool) {
	v, err := canon.Decode(req)
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

// encode is the test engine's codec: a request in canonical form.
func encode(r plugin.Request) ([]byte, error) { return canon.Encode(r.ToCanon(), canon.Canonical) }

// TestEachPlantedCallFailsItsCheckAndNoOther: for C5 and each of C7 to C13, a
// module, an answer or a reading of it that fails that check, red with its
// number, and every other of the 23 as the stand-in has it. The codec's own
// rules are core/wire's and are planted through it in core/api; here the
// engine's reading is scripted, to show which check its refusal lands on.
func TestEachPlantedCallFailsItsCheckAndNoOther(t *testing.T) {
	t.Parallel()
	answers := func(f func(c call, saved []byte) ([]byte, error)) func(*plugintest.Bundle, *plugintest.Engine) {
		return func(b *plugintest.Bundle, _ *plugintest.Engine) { b.Plugin.(*answering).plant = f }
	}
	reads := func(op plugin.Op, path string) func(*plugintest.Bundle, *plugintest.Engine) {
		return func(_ *plugintest.Bundle, e *plugintest.Engine) {
			e.Rules = func(r plugin.Request, _ []byte, _ []string) error {
				if r.Op != op {
					return nil
				}
				return &plugin.AnswerError{Path: path, Why: "planted"}
			}
		}
	}
	for _, c := range []struct {
		name, said string
		n          int
		plant      func(*plugintest.Bundle, *plugintest.Engine)
	}{
		{"a module the host refused", "not WASI preview 1", 5, func(b *plugintest.Bundle, _ *plugintest.Engine) {
			b.Plugin, b.Refused = nil, "the module imports env.clock, which is not WASI preview 1"
		}},
		{"a call that ends in a trap", "exit code 0", 7, answers(func(c call, saved []byte) ([]byte, error) {
			if c.name == "1-entries" {
				return nil, errors.New("exit code 2")
			}
			return saved, nil
		})},
		{"a request the engine does not send", "does not send request.json", 7, func(_ *plugintest.Bundle, e *plugintest.Engine) {
			e.Encode = func(r plugin.Request) ([]byte, error) {
				if r.Op == plugin.OpRank {
					return nil, errors.New("planted")
				}
				return encode(r)
			}
		}},
		{"stdout that is no JSON document", "is not one JSON document", 8, answers(func(c call, saved []byte) ([]byte, error) {
			if c.name == "3-findings" {
				return []byte("ok\n"), nil
			}
			return saved, nil
		})},
		{"stdout that is no answer to its operation", "is no answer to rank", 8, answers(func(c call, saved []byte) ([]byte, error) {
			if c.name == "9-rank" {
				return []byte(`{"ok":{"entries":[]}}`), nil
			}
			return saved, nil
		})},
		{"an answer its rules refuse", "/ok/entries/0", 9, reads(plugin.OpEntries, "/ok/entries/0")},
		{"a plan refused past its steps", "/ok/final", 9, reads(plugin.OpPlan, "/ok/final")},
		{"an answer that is not the saved one", "is not response.json", 10, answers(func(c call, saved []byte) ([]byte, error) {
			if c.name == "9-rank" {
				return []byte(ok(`{"words":["same","same","same"]}`)), nil
			}
			return saved, nil
		})},
		{"a second run with other bytes", "other bytes", 11, answers(func(c call, saved []byte) ([]byte, error) {
			if c.name == "2-findings" && c.n == 2 {
				return append(slices.Clone(saved), ' '), nil
			}
			return saved, nil
		})},
		{"an unknown field that changes the answer", "the answer is another", 12, answers(func(c call, saved []byte) ([]byte, error) {
			if c.name == "1-entries" && c.unknown {
				return []byte(ok(`{"entries":[]}`)), nil
			}
			return saved, nil
		})},
		{"a step whose program is no grant", "/ok/steps/0/program", 13, reads(plugin.OpPlan, "/ok/steps/0/program")},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b, e := withPlugin(t), withEngine()
			c.plant(&b, &e)
			got := runWith(t, e, b)
			for n := 1; n <= plugintest.Checks; n++ {
				switch {
				case n == c.n:
					if got[n].Status != model.CheckFailed || !strings.Contains(strings.Join(got[n].What, "\n"), c.said) {
						t.Errorf("C%d is %s %v, want failed and saying %q", n, got[n].Status, got[n].What, c.said)
					}
				case got[n].Status == model.CheckFailed:
					t.Errorf("C%d failed %v: the plant fails C%d alone", n, got[n].What, c.n)
				}
			}
		})
	}
}

// TestACaseThatFailsAnEarlierCheckIsNotTakenOn: a call that ends in no answer
// fails C7 and is judged by no later check, and a check no case reached is
// not run, never passed.
func TestACaseThatFailsAnEarlierCheckIsNotTakenOn(t *testing.T) {
	t.Parallel()
	b := withPlugin(t)
	b.Plugin.(*answering).plant = func(call, []byte) ([]byte, error) { return nil, errors.New("exit code 2") }
	got := run(t, b)
	if got[7].Status != model.CheckFailed || len(got[7].What) != 9 {
		t.Errorf("C7 is %s %v, want failed for each of the nine cases", got[7].Status, got[7].What)
	}
	for n := 8; n <= 13; n++ {
		if got[n].Status != model.CheckNotRun || !strings.Contains(strings.Join(got[n].What, ""), "none was run") {
			t.Errorf("C%d is %s %v, want not run, as no case reached it", n, got[n].Status, got[n].What)
		}
	}
}
