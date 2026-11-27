package main

import (
	"context"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/wire"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// inProcess is port.Plugin over the standin's own Answer, called in this
// process with no wasm runtime: the request is written in the codec's canonical
// form, answered, and the answer is read through the codec, which holds it to the
// schema and the rules of the wire as the host does. A plugin's own error
// comes back as the CallError the port promises.
type inProcess struct {
	codec *wire.Codec
}

func (inProcess) Manifest() plugin.Manifest { return plugin.Manifest{Name: "standin"} }
func (inProcess) Close() error              { return nil }

func (p inProcess) Plan(_ context.Context, in plugin.PlanRequest) (plugin.PlanResponse, error) {
	b, err := wire.Canonical(in.ToCanon())
	if err != nil {
		return plugin.PlanResponse{}, err
	}
	r, err := p.codec.DecodePlan(Answer(string(plugin.OpPlan), b), in.Round, []string{"standin"})
	return r, err
}

func (p inProcess) Entries(_ context.Context, in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
	b, err := wire.Canonical(in.ToCanon())
	if err != nil {
		return plugin.EntriesResponse{}, err
	}
	r, err := p.codec.DecodeEntries(Answer(string(plugin.OpEntries), b))
	return r, err
}

func (p inProcess) Findings(_ context.Context, in plugin.FindingsRequest) (plugin.FindingsResponse, error) {
	b, err := wire.Canonical(in.ToCanon())
	if err != nil {
		return plugin.FindingsResponse{}, err
	}
	r, err := p.codec.DecodeFindings(Answer(string(plugin.OpFindings), b))
	return r, err
}

func (p inProcess) Rank(_ context.Context, in plugin.RankRequest) (plugin.RankResponse, error) {
	b, err := wire.Canonical(in.ToCanon())
	if err != nil {
		return plugin.RankResponse{}, err
	}
	r, err := p.codec.DecodeRank(Answer(string(plugin.OpRank), b), len(in.Pairs))
	return r, err
}

func (p inProcess) Preset(_ context.Context, in plugin.PresetRequest) (plugin.PresetResponse, error) {
	b, err := wire.Canonical(in.ToCanon())
	if err != nil {
		return plugin.PresetResponse{}, err
	}
	mark := ""
	if in.Mark != nil {
		mark = *in.Mark
	}
	r, err := p.codec.DecodePreset(Answer(string(plugin.OpPreset), b), mark)
	return r, err
}

var _ port.Plugin = inProcess{}

// TestTheStandinSourceAnswersTheSuiteOfCases: the Go source of standin.wasm, its
// Answer called in this process, gives each case of internal/port/contracttest
// the answer the in-memory fake gives it (gragas#2023); the codec that reads
// each answer is the engine's own, so an answer that breaks the schema or the
// order of the wire is red here and not first in the host.
//
// What it costs when this fails: the module host builds from this source answers
// a request otherwise than the fake every core test uses, and a core test that
// passes on the fake fails, or passes wrongly, on the real thing.
func TestTheStandinSourceAnswersTheSuiteOfCases(t *testing.T) {
	t.Parallel()
	names := porttest.NewWorld(t).Ports().FileNames
	contracttest.Plugin(t, func(t *testing.T) port.Plugin {
		t.Helper()
		return inProcess{codec: wire.New(names)}
	})
}

// TestServeWrapsAnswerAndRefusesWhatIsNoRequest: Serve reads the whole request
// document, and a document that is none is the plugin's own bug and not a crash.
func TestServeWrapsAnswerAndRefusesWhatIsNoRequest(t *testing.T) {
	t.Parallel()
	got := Serve([]byte(`{"contract":1,"features":[],"op":"rank","body":{"tool_version":"1.0.0","pairs":[{"key":"k","aspect":"value","preset":2,"found":1}]}}`))
	if want := `{"ok":{"words":["tighter"]}}` + "\n"; string(got) != want {
		t.Errorf("Serve = %s, want %s", got, want)
	}
	if got := Serve([]byte(`not json`)); string(got[:9]) != `{"error":` {
		t.Errorf("Serve of no request = %s, want the plugin's error", got)
	}
	if got := Serve([]byte(`{"op":"nope","body":{}}`)); string(got) == "" || string(got[:9]) != `{"error":` {
		t.Errorf("Serve of an operation it does not serve = %s, want the plugin's error", got)
	}
}

// TestTheStandinAnswersWhatPluginTestAsksOfAnyPlugin (gragas#2244): each row is
// one claim of C14 to C17 or C20 of plugin test, sent to Answer as the check
// sends it, and the code it must answer. Risk: the one plugin gragas keeps
// fails the test gragas holds every plugin to.
func TestTheStandinAnswersWhatPluginTestAsksOfAnyPlugin(t *testing.T) {
	t.Parallel()
	const v = `"tool_version":"1.0.0"`
	for _, c := range []struct{ claim, op, body, want string }{
		{"C14: a plan for an unknown for is unsupported", "plan", `{` + v + `,"config":[],"have":[],"for":"nothing","target":"source","round":0,"fixtures":[]}`, "unsupported"},
		{"C14: an entries for an unknown target is unsupported", "entries", `{` + v + `,"config":[],"have":[],"target":"nowhere"}`, "unsupported"},
		{"C14: an unknown operation is unsupported", "nothing", `{}`, "unsupported"},
		{"C16: a tool version below the range is unsupported", "entries", `{"tool_version":"0.0.0","config":[],"have":[],"target":"source"}`, "unsupported"},
		{"C16: a tool version above the range is unsupported", "rank", `{"tool_version":"2.0.0","pairs":[]}`, "unsupported"},
		{"C17: a body with no member the schema requires is a bug", "entries", `{}`, "bug"},
		{"C17: a body that is no object is a bug", "rank", `[]`, "bug"},
		{"C20: a plan for a config the standin does not read is refused with its path", "plan", `{` + v + `,"config":[{"path":"other.yml","data":{"text":"x"}}],"have":[],"for":"config","target":"source","round":0,"fixtures":[]}`, "refused"},
	} {
		got := string(Answer(c.op, []byte(c.body)))
		if want := `{"error":{"code":"` + c.want + `"`; !strings.Contains(got, want) {
			t.Errorf("%s: Answer(%s) = %s, want an error with code %s", c.claim, c.op, got, c.want)
		}
	}
	for _, c := range []struct{ claim, op, body string }{
		{"C16: the lowest version of the range is served", "entries", `{"tool_version":"1.0.0","config":[],"have":[],"target":"source"}`},
		{"C16: a version inside the range is served", "rank", `{"tool_version":"1.99.9","pairs":[{"key":"k","aspect":"value","preset":1,"found":1}]}`},
	} {
		if got := string(Answer(c.op, []byte(c.body))); !strings.HasPrefix(got, `{"ok":`) {
			t.Errorf("%s: Answer(%s) = %s, want an answer", c.claim, c.op, got)
		}
	}
}
