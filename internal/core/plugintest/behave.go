package plugintest

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// Caller is an opened plugin: what port.Plugin offers the behaviour checks.
// The core hands one in when a host opened the bundle's module; none is faked
// here.
type Caller interface {
	Plan(ctx context.Context, in plugin.PlanRequest) (plugin.PlanResponse, error)
	Entries(ctx context.Context, in plugin.EntriesRequest) (plugin.EntriesResponse, error)
	Findings(ctx context.Context, in plugin.FindingsRequest) (plugin.FindingsResponse, error)
	Rank(ctx context.Context, in plugin.RankRequest) (plugin.RankResponse, error)
	Preset(ctx context.Context, in plugin.PresetRequest) (plugin.PresetResponse, error)
}

// Raw is a Caller that takes a request as bytes, so a case's request is sent
// as the engine writes it, and a request no typed call can build, an operation
// of no name or a body that is not the operation's, can be sent. It answers
// what the module wrote to stdout when it exited with code 0, unread; any
// other end, a crash or a limit, is an error (port.Trial).
type Raw interface {
	Raw(ctx context.Context, request []byte) ([]byte, error)
}

// rawResponse is the response the module answered request with: an error for
// a call that did not end with code 0 or a stdout that is no response.
func rawResponse(ctx context.Context, raw Raw, request []byte) (plugin.Response, error) {
	var r plugin.Response
	out, err := raw.Raw(ctx, request)
	if err != nil {
		return r, err
	}
	v, err := canon.Decode(out)
	if err != nil {
		return r, fmt.Errorf("stdout is not one JSON document: %w", err)
	}
	if err := r.FromCanon(v); err != nil {
		return r, fmt.Errorf("stdout is not a response: %w", err)
	}
	return r, nil
}

// allOps is every operation of the contract.
func allOps() []plugin.Op {
	return []plugin.Op{plugin.OpPlan, plugin.OpEntries, plugin.OpFindings, plugin.OpRank, plugin.OpPreset}
}

// behaviour is C14 to C17, which call the plugin.
func behaviour(ctx context.Context, e Engine, b Bundle, o canon.Object) []model.PluginCheck {
	var out []model.PluginCheck
	switch {
	case o == nil:
		for n := 14; n <= 17; n++ {
			out = append(out, skipped(n, unread))
		}
	case b.Plugin == nil:
		for n := 14; n <= 17; n++ {
			out = append(out, skipped(n, unopened(b)))
		}
	default:
		out = append(out, checkUnknown(ctx, e, b.Plugin, o), checkNotProvided(ctx, e, b.Plugin, o), checkRange(ctx, e, b.Plugin, o), checkMalformed(ctx, b.Plugin))
	}
	return out
}

// code is the wire code a call answered, "" for a call that succeeded and
// crashed for one that ended in something else than an answer.
func code(err error) (c plugin.Code, crashed bool) {
	if err == nil {
		return "", false
	}
	if ce, ok := errors.AsType[*plugin.CallError](err); ok {
		return ce.Code, false
	}
	return "", true
}

// probe calls op with a request that is valid but for what the caller changes.
func probe(ctx context.Context, c Caller, op plugin.Op, version string, t plugin.Target, f plugin.For) error {
	var err error
	switch op {
	case plugin.OpPlan:
		_, err = c.Plan(ctx, plugin.PlanRequest{ToolVersion: version, Config: []plugin.Config{}, Have: []plugin.Result{}, For: f, Target: t, Round: 0, Fixtures: []string{}})
	case plugin.OpEntries:
		_, err = c.Entries(ctx, plugin.EntriesRequest{ToolVersion: version, Config: []plugin.Config{}, Have: []plugin.Result{}, Target: t})
	case plugin.OpFindings:
		_, err = c.Findings(ctx, plugin.FindingsRequest{ToolVersion: version, Config: []plugin.Config{}, Have: []plugin.Result{}, Fixtures: []string{}})
	case plugin.OpRank:
		// One pair, or the SDK answers bug to the empty request before the plugin
		// can say unsupported.
		_, err = c.Rank(ctx, plugin.RankRequest{ToolVersion: version, Pairs: []plugin.Pair{{Key: "probe", Aspect: plugin.AspectValue, Preset: canon.String("a"), Found: canon.String("b")}}})
	default:
		_, err = c.Preset(ctx, plugin.PresetRequest{ToolVersion: version, Inputs: []plugin.File{}, Entries: []plugin.PresetEntry{}})
	}
	return err
}

// list is the strings of an array the manifest holds.
func list(v canon.Value) []string {
	var out []string
	if a, ok := v.(canon.Array); ok {
		for _, x := range a {
			out = append(out, text(x))
		}
	}
	return out
}

// firstVersion is a tool version the manifest's range serves, for a probe that
// asks nothing about versions.
// It is never a fixed one: with none the range serves, ok is false and the
// check is not run.
func firstVersion(e Engine, o canon.Object) (v string, ok bool) {
	if e.Ends == nil {
		return "", false
	}
	served, _, err := e.Ends(text(o["tool_versions"]))
	if err != nil || len(served) == 0 {
		return "", false
	}
	return served[0], true
}

const noVersion = "no tool version the range serves can be named, so no probe can be sent"

// target is the first target the manifest serves.
func target(o canon.Object) plugin.Target {
	if t := list(o["targets"]); len(t) > 0 {
		return plugin.Target(t[0])
	}
	return plugin.TargetSource
}

// unsupportedWant says what is wrong with an answer that should be unsupported.
func unsupportedWant(what string, err error) string {
	c, crashed := code(err)
	switch {
	case crashed:
		return fmt.Sprintf("%s ended in a crash, not unsupported: %v", what, err)
	case c == "":
		return what + " succeeded, and must answer unsupported"
	case c != plugin.CodeUnsupported:
		return fmt.Sprintf("%s answered %s, and must answer unsupported", what, c)
	}
	return ""
}

// checkUnknown is C14: a target and a for no contract names, and an operation
// of no name, each answer unsupported.
func checkUnknown(ctx context.Context, e Engine, c Caller, o canon.Object) model.PluginCheck {
	raw, ok := c.(Raw)
	if !ok {
		return skipped(14, "the opened plugin takes no raw request, so an unknown operation cannot be sent")
	}
	var what []string
	add := func(s string) {
		if s != "" {
			what = append(what, s)
		}
	}
	v, named := firstVersion(e, o)
	if !named {
		return skipped(14, noVersion)
	}
	add(unsupportedWant("entries for an unknown target", probe(ctx, c, plugin.OpEntries, v, "nowhere", plugin.ForConfig)))
	add(unsupportedWant("plan for an unknown target", probe(ctx, c, plugin.OpPlan, v, "nowhere", plugin.ForConfig)))
	add(unsupportedWant("plan for an unknown for", probe(ctx, c, plugin.OpPlan, v, target(o), "nothing")))
	req := []byte(`{"body":{},"contract":1,"features":[],"op":"nothing"}`)
	r, err := rawResponse(ctx, raw, req)
	switch {
	case err != nil:
		add(fmt.Sprintf("an unknown operation ended in no answer, not unsupported: %v", err))
	case r.Error == nil:
		add("an unknown operation succeeded, and must answer unsupported")
	case r.Error.Code != plugin.CodeUnsupported:
		add(fmt.Sprintf("an unknown operation answered %s, and must answer unsupported", r.Error.Code))
	}
	return result(14, what)
}

// checkNotProvided is C15: each operation outside provides answers unsupported.
func checkNotProvided(ctx context.Context, e Engine, c Caller, o canon.Object) model.PluginCheck {
	provided := list(o["provides"])
	var what []string
	v, named := firstVersion(e, o)
	for _, op := range allOps() {
		if slices.Contains(provided, string(op)) {
			continue
		}
		if !named {
			return skipped(15, noVersion)
		}
		if w := unsupportedWant(string(op)+", which provides leaves out,", probe(ctx, c, op, v, target(o), plugin.ForConfig)); w != "" {
			what = append(what, w)
		}
	}
	return result(15, what)
}

// checkRange is C16: the tool version range's ends are served and a version
// outside it is unsupported. The probe is the first operation the plugin
// provides.
func checkRange(ctx context.Context, e Engine, c Caller, o canon.Object) model.PluginCheck {
	if e.Ends == nil {
		return skipped(16, "the engine reads no tool version range here")
	}
	served, outside, err := e.Ends(text(o["tool_versions"]))
	if err != nil {
		return result(16, []string{"tool_versions: " + err.Error()})
	}
	if len(served) == 0 {
		return skipped(16, "the range serves no version that can be named: "+noVersion)
	}
	provides := list(o["provides"])
	if len(provides) == 0 {
		return skipped(16, "provides lists no operation to probe (see C3)")
	}
	op := plugin.Op(provides[0])
	var what []string
	for _, v := range served {
		cd, crashed := code(probe(ctx, c, op, v, target(o), plugin.ForConfig))
		if crashed || cd == plugin.CodeUnsupported {
			what = append(what, fmt.Sprintf("tool version %s is in the range and %s does not serve it", v, op))
		}
	}
	for _, v := range outside {
		if w := unsupportedWant("tool version "+v+", outside the range,", probe(ctx, c, op, v, target(o), plugin.ForConfig)); w != "" {
			what = append(what, w)
		}
	}
	return result(16, what)
}

// checkMalformed is C17: a request that is no request ends in a bug error or a
// crash, never in success.
func checkMalformed(ctx context.Context, c Caller) model.PluginCheck {
	raw, ok := c.(Raw)
	if !ok {
		return skipped(17, "the opened plugin takes no raw request")
	}
	var what []string
	for _, bad := range []string{
		`{`,
		`{}`,
		`{"contract":1,"features":[],"op":"entries"}`,
		`{"body":{},"contract":1,"features":[],"op":"entries"}`,
		`{"body":[],"contract":1,"features":[],"op":"rank"}`,
	} {
		r, err := rawResponse(ctx, raw, []byte(bad))
		switch {
		case err != nil:
		case r.Error == nil:
			what = append(what, fmt.Sprintf("the request %s ended in success", bad))
		case r.Error.Code != plugin.CodeBug:
			what = append(what, fmt.Sprintf("the request %s answered %s, and must answer bug", bad, r.Error.Code))
		}
	}
	return result(17, what)
}
