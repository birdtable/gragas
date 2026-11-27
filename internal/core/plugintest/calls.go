package plugintest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// checkModule is C5: the host compiled the module and held it to what a
// plugin's module is (port.Plugins.Try), so it imports nothing but WASI
// preview 1. The section reading is the host's, the one that holds an
// installed plugin at Open; this check reads its verdict.
func checkModule(b Bundle) model.PluginCheck {
	switch {
	case !b.HasWasm:
		return skipped(5, "plugin.wasm is not in the bundle (see C3)")
	case b.Refused != "":
		return result(5, []string{b.Refused})
	case b.Plugin == nil:
		return skipped(5, "the module was not opened")
	}
	return result(5, nil)
}

// unopened says why a check that calls the plugin did not run.
func unopened(b Bundle) string {
	switch {
	case !b.HasWasm:
		return "plugin.wasm is not in the bundle (see C3)"
	case b.Refused != "":
		return "the module was refused (see C5)"
	}
	return "the module was not opened"
}

// unknownField is the member C12 adds to a request's body: no contract
// names it, and a body is open, so a plugin must answer as if it were not
// there.
const unknownField = "unknown_to_the_contract"

// noCase says why a check of the calls judged no case.
const noCase = "no case passed C6, so none was run"

// calls is C7 to C13: each case that passes C6 is sent to the plugin, its
// request written by the engine's codec, and what it answers is held, check
// by check. A case that fails one check is not taken on to the checks that
// read its answer, and a check no case reached is not run and says why.
func calls(ctx context.Context, e Engine, b Bundle, o canon.Object) []model.PluginCheck {
	raw, isRaw := b.Plugin.(Raw)
	why := ""
	switch {
	case b.Plugin == nil:
		why = unopened(b)
	case !isRaw:
		why = "the opened plugin takes no raw request"
	case e.Encode == nil || e.Rules == nil:
		why = "the engine handed in no codec to write a request and read an answer with"
	}
	if why != "" {
		out := make([]model.PluginCheck, 0, 7)
		for n := 7; n <= 13; n++ {
			out = append(out, skipped(n, why))
		}
		return out
	}
	j := &judged{what: map[int][]string{}, reached: map[int]int{}}
	g := grantIDs(o)
	for _, c := range b.Cases {
		if c.Missing != "" || checkRequest(c) != nil {
			continue
		}
		j.run(ctx, e, raw, c, g)
	}
	out := make([]model.PluginCheck, 0, 7)
	for n := 7; n <= 13; n++ {
		switch {
		case n == 13 && o == nil:
			out = append(out, skipped(13, unread))
		case n == 13 && !slices.Contains(list(o["provides"]), string(plugin.OpPlan)):
			out = append(out, notAsked(13, "plan is not in provides"))
		case j.reached[n] == 0:
			out = append(out, skipped(n, noCase))
		default:
			out = append(out, result(n, j.what[n]))
		}
	}
	return out
}

// grantIDs is the ids of the manifest's programs, the grants a plan's run
// step may name; none where the manifest was not read.
func grantIDs(o canon.Object) []string {
	ids := []string{}
	progs, ok := o["programs"].(canon.Array)
	if !ok {
		return ids
	}
	for _, p := range progs {
		if g, ok := p.(canon.Object); ok {
			if id := text(g["id"]); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// judged is what the calls found: what is wrong, by check, and how many cases
// each check judged.
type judged struct {
	what    map[int][]string
	reached map[int]int
}

// fail says what is wrong with case c under check n.
func (j *judged) fail(n int, c Case, format string, args ...any) {
	j.what[n] = append(j.what[n], "case "+c.Name+": "+fmt.Sprintf(format, args...))
}

// run sends case c, whose request passed C6, and holds what the plugin
// answers to C7 to C13.
func (j *judged) run(ctx context.Context, e Engine, raw Raw, c Case, grants []string) {
	v, err := canon.Decode(c.Request)
	if err != nil {
		return
	}
	var r plugin.Request
	if r.FromCanon(v) != nil {
		return
	}
	j.reached[7]++
	req, err := e.Encode(r)
	if err != nil {
		j.fail(7, c, "the engine does not send request.json: %v", err)
		return
	}
	out, err := raw.Raw(ctx, req)
	if err != nil {
		j.fail(7, c, "the call did not end with exit code 0 within the limits: %v", err)
		return
	}
	j.reached[8]++
	if err := responseOf(r.Op, out); err != nil {
		j.fail(8, c, "stdout %v", err)
		return
	}
	j.rules(e, r, c, out, grants)
	j.reached[10]++
	if why := differ(out, c.Response); why != "" {
		j.fail(10, c, "%s", why)
	}
	j.reached[11]++
	switch again, err := raw.Raw(ctx, req); {
	case err != nil:
		j.fail(11, c, "the second run did not end with exit code 0: %v", err)
	case string(again) != string(out):
		j.fail(11, c, "the second run gave other bytes than the first")
	}
	j.reached[12]++
	if why := j.unknown(ctx, e, raw, r, out); why != "" {
		j.fail(12, c, "%s", why)
	}
}

// rules holds an answer that passed its schema to the codec's rules past it:
// a refusal of a plan's step, its program a grant and its paths the
// contract's, is C13's; any other is C9's. A plugin's own error is an answer
// the codec reads, no refusal.
func (j *judged) rules(e Engine, r plugin.Request, c Case, out []byte, grants []string) {
	j.reached[9]++
	if r.Op == plugin.OpPlan {
		j.reached[13]++
	}
	err := e.Rules(r, out, grants)
	if err == nil {
		return
	}
	if _, own := errors.AsType[*plugin.CallError](err); own {
		return
	}
	if ae, ok := errors.AsType[*plugin.AnswerError](err); ok && r.Op == plugin.OpPlan && strings.HasPrefix(ae.Path, "/ok/steps/") {
		j.fail(13, c, "%v", err)
		return
	}
	j.fail(9, c, "%v", err)
}

// unknown is C12: the request with unknownField added to its body is answered
// as the request was, in canonical form; "" when it is.
func (j *judged) unknown(ctx context.Context, e Engine, raw Raw, r plugin.Request, first []byte) string {
	if _, taken := r.Body[unknownField]; taken {
		return "the body already holds " + unknownField + ", so no field can be added that it lacks"
	}
	with := r
	with.Body = maps.Clone(r.Body)
	with.Body[unknownField] = canon.Bool(true)
	req, err := e.Encode(with)
	if err != nil {
		return fmt.Sprintf("the engine does not send the request with %s: %v", unknownField, err)
	}
	out, err := raw.Raw(ctx, req)
	if err != nil {
		return fmt.Sprintf("with %s the call did not end with exit code 0: %v", unknownField, err)
	}
	a, errA := canonical(first)
	b, errB := canonical(out)
	if errB != nil {
		return fmt.Sprintf("with %s stdout is not one JSON document: %v", unknownField, errB)
	}
	if errA != nil || string(a) != string(b) {
		return "with " + unknownField + " in the body the answer is another"
	}
	return ""
}

// responseOf is nil for out, a call's stdout, that is one JSON document of
// the response schema whose ok is the body of op's response, and what is
// wrong otherwise.
func responseOf(op plugin.Op, out []byte) error {
	v, err := canon.Decode(out)
	if err != nil {
		return fmt.Errorf("is not one JSON document: %w", err)
	}
	var r plugin.Response
	if err := r.FromCanon(v); err != nil {
		return fmt.Errorf("is not a response: %w", err)
	}
	if r.Error != nil {
		return nil
	}
	if err := responseBody(op).FromCanon(r.Ok); err != nil {
		return fmt.Errorf("is no answer to %s: %w", op, err)
	}
	return nil
}

// canonical is the canonical form of one JSON document, the bytes two answers
// that say the same share.
func canonical(doc []byte) ([]byte, error) {
	v, err := canon.Decode(doc)
	if err != nil {
		return nil, err
	}
	return canon.Encode(v, canon.Canonical)
}

// differ is C10: "" when out equals saved, response.json, in canonical form,
// and what differs otherwise.
func differ(out, saved []byte) string {
	want, err := canonical(saved)
	if err != nil {
		return "response.json is not one JSON document: " + err.Error()
	}
	got, err := canonical(out)
	if err != nil {
		return "stdout is not one JSON document: " + err.Error()
	}
	if string(got) != string(want) {
		return fmt.Sprintf("the answer is not response.json in canonical form: it answered %s", plugin.Shown(clip(string(got))))
	}
	return ""
}

// clipAt is how much of an answer a failure shows.
const clipAt = 200

// clip is s cut to clipAt bytes, marked where it is cut.
func clip(s string) string {
	if len(s) <= clipAt {
		return s
	}
	return s[:clipAt] + "..."
}
