// Package plugintest holds gragas plugin test's checks C1 to C23 of the plugin
// design's 24, each numbered so a failure names its check. C24 holds gragas itself and is the host's.
//
// It reads bytes it is handed, never a file: the core's operation reads the
// bundle through the port. The contract number, the features the engine speaks
// and the ends of a tool version range arrive as an argument, since a context
// never imports another and the codec that owns them is core/wire.
//
// The checks that need the wasm host, C5, C7 to C17 and the round trip of
// C23, read the module the core had the host try (port.Plugins.Try): its
// refusal (Bundle.Refused) or the plugin it opened (Bundle.Plugin). The
// engine's codec writes each request and holds each answer to its rules
// (Engine.Encode, Engine.Rules), the codec every plugin call is read by. A
// check that could not run says why and never reads as passed.
package plugintest

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/ordered"
)

// Engine is what the engine speaks: the contract number and the features it
// has, the one value of the codec.
type Engine struct {
	Contract int
	Features []string
	// Ends answers, for a tool version range, the versions it serves at its
	// ends and versions just outside it (C16). The range grammar is the link
	// context's, so the core passes it in.
	Ends func(spec string) (served, outside []string, err error)
	// Encode writes a request as the engine sends one, in canonical form and
	// held to its caps (the codec's EncodeRequest).
	Encode func(r plugin.Request) ([]byte, error)
	// Rules holds a call's answer, one that passed the response schema, to
	// the codec's rules past it, as a call of r is read: the order of what it
	// lists (C9) and, for a plan, each step's program among grants and its
	// paths the contract's (C13). A plugin's own error is a *plugin.CallError;
	// a refusal is a *plugin.AnswerError, whose path says where. With Encode
	// nil or Rules nil, C7 to C13 are not run.
	Rules func(r plugin.Request, answer []byte, grants []string) error
}

// Case is one folder of testdata/cases: its name, <n>-<op>, its request and
// its saved response.
type Case struct {
	Name     string
	Request  []byte
	Response []byte
	// Missing is the file of the case that is not in the bundle, "" when it is.
	Missing string
}

// Recording is one folder of testdata/recordings: the tool version it is named
// for and the files of raw tool output it holds.
type Recording struct {
	Version string
	Files   []string
}

// Bundle is what the core read of one bundle folder, plugins/<name>/<version>.
type Bundle struct {
	// Name and Version are the folders the bundle sits in.
	Name, Version string
	// Manifest is plugin.yaml, nil when the bundle has none.
	Manifest []byte
	// Wasm is plugin.wasm and HasWasm says it is there.
	Wasm    []byte
	HasWasm bool
	// Cases are the folders of testdata/cases, by name.
	Cases []Case
	// Recordings are the folders of testdata/recordings.
	Recordings []Recording
	// Plugin is the module the host tried (port.Plugins.Try), which the
	// checks C7 to C17 and C23 call; nil where there is no module or the host
	// refused it, and they are then not run.
	Plugin Caller
	// Refused is why the host refused the module, "" where it did not (C5).
	Refused string
}

// The names of the thirteen checks and of the rest of the 24, by number.
func name(n int) string {
	return [...]string{
		1:  "the manifest passes its schema",
		2:  "the contract is the engine's and every need is a feature it has",
		3:  "name, version and wasm digest are the bundle's",
		4:  "provides is not empty and grant ids are unique",
		5:  "the module imports nothing but WASI preview 1",
		6:  "each request passes its schema",
		7:  "each call ends within the limits with exit code 0",
		8:  "each stdout is one JSON document that passes the response schema",
		9:  "each answer keeps every order rule",
		10: "each answer equals the saved answer in canonical form",
		11: "a second run gives the same bytes",
		12: "a request with one unknown field gives the same answer",
		13: "every run step names a granted program and passes the path rules",
		14: "an unknown operation, target or for answers unsupported",
		15: "an operation the plugin does not provide answers unsupported",
		16: "both ends of the tool version range are served and one outside it is unsupported",
		17: "a malformed request ends in a bug error or a crash, never in success",
		18: "entries has a recording for each tool version tested",
		19: "findings has a planted output, a clean control and a tool failure",
		20: "plan has a one-round case and a refused case with a path",
		21: "preset has a golden, the mark on its first line and a refused option",
		22: "rank has a pair each for tighter, weaker and same, and a swap swaps the answer",
		23: "a preset, recorded, reads back as the same entries",
	}[n]
}

// Checks is how many checks this command runs, C1 to C23.
const Checks = model.PluginChecks

// Run holds b to checks C1 to C23, in order. A check that calls the plugin,
// where there is none, is not run and says why.
func Run(ctx context.Context, e Engine, b Bundle) []model.PluginCheck {
	out := make([]model.PluginCheck, 0, Checks)
	raw, c1 := readManifest(b)
	out = append(out, c1)
	out = append(out, checkEngine(e, raw), checkIdentity(b, raw), checkProvides(raw))
	out = append(out, checkModule(b))
	out = append(out, checkRequests(b))
	out = append(out, calls(ctx, e, b, raw)...)
	out = append(out, behaviour(ctx, e, b, raw)...)
	return append(out, evidence(ctx, b, raw)...)
}

// ManifestOf is the manifest of a bundle as its generated type, and whether
// C1 reads it as one: what the core hands the host with the module it tries,
// the grants a plan's step is held to.
func ManifestOf(manifest []byte) (plugin.Manifest, bool) {
	o, c1 := readManifest(Bundle{Manifest: manifest})
	var m plugin.Manifest
	if c1.Status != model.CheckPassed || m.FromCanon(o) != nil {
		return plugin.Manifest{}, false
	}
	return m, true
}

func result(n int, what []string) model.PluginCheck {
	c := model.PluginCheck{N: n, Name: name(n), Status: model.CheckPassed}
	if len(what) > 0 {
		c.Status, c.What = model.CheckFailed, what
	}
	return c
}

// skipped is a check whose input C1 could not read.
func skipped(n int, why string) model.PluginCheck {
	return model.PluginCheck{N: n, Name: name(n), Status: model.CheckNotRun, What: []string{why}}
}

// notAsked is a check for an operation the manifest does not provide: it asks
// nothing of the bundle, says which operation, and counts toward done.
func notAsked(n int, why string) model.PluginCheck {
	return model.PluginCheck{N: n, Name: name(n), Status: model.CheckNotAsked, What: []string{why}}
}

const unread = "the manifest was not read (see C1)"

// readManifest is C1: the manifest read as one strict YAML document, then as
// the manifest's generated type, which refuses an unknown key and every other
// thing its schema says. It answers the manifest as an object for C2 to C4 when
// there is one.
//
// C2 and C4 own two things the type refuses as well: a contract other than the
// engine's and a provides with nothing in it. Each is set aside before the
// type reads, so a manifest that breaks one of them fails that check and not C1.
func readManifest(b Bundle) (canon.Object, model.PluginCheck) {
	if b.Manifest == nil {
		return nil, result(1, []string{"plugin.yaml is not in the bundle"})
	}
	doc, err := ordered.ParseStrict(b.Manifest)
	if err != nil {
		return nil, result(1, []string{"plugin.yaml is not one plain YAML document: " + err.Error()})
	}
	v, err := toCanon(doc)
	if err != nil {
		return nil, result(1, []string{"plugin.yaml " + err.Error()})
	}
	o, ok := v.(canon.Object)
	if !ok {
		return nil, result(1, []string{"plugin.yaml is not a mapping"})
	}
	read := canon.Object{}
	maps.Copy(read, o)
	// The schema holds contract at 1, the one number the generated type reads.
	if _, ok := o["contract"].(canon.Int); ok {
		read["contract"] = canon.Int(1)
	}
	if a, ok := o["provides"].(canon.Array); ok && len(a) == 0 {
		read["provides"] = canon.Array{canon.String(plugin.OpPlan)}
	}
	var m plugin.Manifest
	if err := m.FromCanon(read); err != nil {
		return o, result(1, []string{err.Error()})
	}
	return o, result(1, nil)
}

// toCanon turns the plain YAML value into canon's: a value of no JSON form, a
// timestamp or a NaN, is refused.
func toCanon(v any) (canon.Value, error) {
	switch x := v.(type) {
	case nil:
		return canon.Null{}, nil
	case bool:
		return canon.Bool(x), nil
	case int64:
		return canon.Int(x), nil
	case float64:
		return canon.Float(x), nil
	case string:
		return canon.String(x), nil
	case []any:
		a := make(canon.Array, len(x))
		for i, e := range x {
			c, err := toCanon(e)
			if err != nil {
				return nil, err
			}
			a[i] = c
		}
		return a, nil
	case *ordered.Map:
		o := make(canon.Object, x.Len())
		for _, k := range x.Keys() {
			c, err := toCanon(x.Val(k))
			if err != nil {
				return nil, err
			}
			o[k] = c
		}
		return o, nil
	}
	return nil, fmt.Errorf("holds a value with no JSON form (%T)", v)
}

// checkEngine is C2: the manifest's contract is the engine's and every need is
// a feature the engine has.
func checkEngine(e Engine, o canon.Object) model.PluginCheck {
	if o == nil {
		return skipped(2, unread)
	}
	var what []string
	switch c := o["contract"].(type) {
	case canon.Int:
		if int(c) != e.Contract {
			what = append(what, fmt.Sprintf("the manifest is for contract %d and this engine speaks contract %d", int64(c), e.Contract))
		}
	default:
		what = append(what, "contract is not a number")
	}
	if a, ok := o["needs"].(canon.Array); ok {
		for _, n := range a {
			if s, ok := n.(canon.String); ok && !slices.Contains(e.Features, string(s)) {
				what = append(what, fmt.Sprintf("needs the feature %q, which this engine does not have", string(s)))
			}
		}
	}
	return result(2, what)
}

// checkIdentity is C3: name and version equal the folders, and wasm equals the
// digest of plugin.wasm.
func checkIdentity(b Bundle, o canon.Object) model.PluginCheck {
	if o == nil {
		return skipped(3, unread)
	}
	var what []string
	for _, f := range []struct{ key, folder string }{{"name", b.Name}, {"version", b.Version}} {
		if got := text(o[f.key]); got != f.folder {
			what = append(what, fmt.Sprintf("%s is %q and the folder is %q", f.key, got, f.folder))
		}
	}
	switch want := text(o["wasm"]); {
	case !b.HasWasm:
		what = append(what, "plugin.wasm is not in the bundle")
	case want != digest.Prefixed(b.Wasm):
		what = append(what, fmt.Sprintf("wasm is %q and plugin.wasm is %s", want, digest.Prefixed(b.Wasm)))
	}
	return result(3, what)
}

// checkProvides is C4: provides is not empty and no two grants have one id.
func checkProvides(o canon.Object) model.PluginCheck {
	if o == nil {
		return skipped(4, unread)
	}
	var what []string
	if a, ok := o["provides"].(canon.Array); !ok || len(a) == 0 {
		what = append(what, "provides is empty: a plugin that answers no operation is no plugin")
	}
	seen := map[string]bool{}
	if progs, ok := o["programs"].(canon.Array); ok {
		for _, p := range progs {
			g, ok := p.(canon.Object)
			if !ok {
				continue
			}
			switch id := text(g["id"]); {
			case id == "":
			case seen[id]:
				what = append(what, fmt.Sprintf("the grant id %q is given twice", id))
			default:
				seen[id] = true
			}
		}
	}
	return result(4, what)
}

// text is the string a value holds, or "" for a value that is not one.
func text(v canon.Value) string {
	if s, ok := v.(canon.String); ok {
		return string(s)
	}
	return ""
}

// checkRequests is C6: each case's request passes the request schema, the
// envelope and the body of the operation the case's folder names.
func checkRequests(b Bundle) model.PluginCheck {
	if len(b.Cases) == 0 {
		return skipped(6, "testdata/cases holds no case")
	}
	var what []string
	for _, c := range b.Cases {
		if c.Missing != "" {
			what = append(what, fmt.Sprintf("case %s: %s is not in the bundle", c.Name, c.Missing))
			continue
		}
		if err := checkRequest(c); err != nil {
			what = append(what, fmt.Sprintf("case %s: %v", c.Name, err))
		}
	}
	return result(6, what)
}

func checkRequest(c Case) error {
	v, err := canon.Decode(c.Request)
	if err != nil {
		return fmt.Errorf("request.json is not one canonical JSON document: %w", err)
	}
	var r plugin.Request
	if err := r.FromCanon(v); err != nil {
		return fmt.Errorf("request.json %w", err)
	}
	if want := OpOf(c.Name); want != "" && string(r.Op) != want {
		return fmt.Errorf("the folder names the operation %s and request.json asks for %s", want, r.Op)
	}
	if err := requestBody(r.Op).FromCanon(r.Body); err != nil {
		return fmt.Errorf("the body of %s %w", r.Op, err)
	}
	return nil
}

// bodyReader is a body of the contract, to read canon's tree into.
type bodyReader interface{ FromCanon(canon.Value) error }

// requestBody is an empty request body of the operation, to read into.
func requestBody(op plugin.Op) bodyReader {
	in, _ := bodiesOf(op)
	return in
}

// responseBody is an empty response body of the operation, to read into.
func responseBody(op plugin.Op) bodyReader {
	_, out := bodiesOf(op)
	return out
}

// bodiesOf is an empty request body and an empty response body of the
// operation: one table, so the two never name different operations.
func bodiesOf(op plugin.Op) (request, response bodyReader) {
	switch op {
	case plugin.OpPlan:
		return &plugin.PlanRequest{}, &plugin.PlanResponse{}
	case plugin.OpEntries:
		return &plugin.EntriesRequest{}, &plugin.EntriesResponse{}
	case plugin.OpFindings:
		return &plugin.FindingsRequest{}, &plugin.FindingsResponse{}
	case plugin.OpRank:
		return &plugin.RankRequest{}, &plugin.RankResponse{}
	default:
		return &plugin.PresetRequest{}, &plugin.PresetResponse{}
	}
}

// OpOf is the operation a case folder <n>-<op> names, "" when it names none.
func OpOf(name string) string {
	_, op, ok := strings.Cut(path.Base(name), "-")
	if !ok || !plugin.Op(op).Valid() {
		return ""
	}
	return op
}
