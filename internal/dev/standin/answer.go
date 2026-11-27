// Command standin is the source of standin.wasm, the one plugin gragas keeps:
// a small program for wasip1 that reads one request on
// stdin and writes one response on stdout, the wire of plugin contract 1. It
// serves no tool. Its answers are deterministic functions of the request, the
// same ones the in-memory fake of internal/port/testing gives, and the suite of
// internal/port/contracttest holds the two to the same cases. It cannot use the
// SDK, which is another repository: it speaks the wire through the engine's own
// generated types and writes them in the codec's canonical form.
//
// Host builds the module from this folder, and gate rebuilds it and
// compares the digest. The program is built for wasip1 by being
// targeted: nothing here is marked for a platform.
package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/wire"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// Serve is the whole program: one request document in, one response document
// out. A document that is no request is answered with the plugin's own error,
// since a plugin has no other way to say it.
func Serve(in []byte) []byte {
	var req struct {
		Op   string          `json:"op"`
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(in, &req); err != nil || req.Op == "" {
		return failure(plugin.CodeBug, "", "%s", "the request is no document of the wire")
	}
	return Answer(req.Op, req.Body)
}

// Answer is the response to one operation's request body, as the wire's
// response document: {"ok": ...} or {"error": ...}, in canonical form.
func Answer(op string, body []byte) []byte {
	var b obj
	if err := json.Unmarshal(body, &b); err != nil {
		return failure(plugin.CodeBug, "", "%s", "the body is no object")
	}
	if !slices.Contains([]plugin.Op{plugin.OpPlan, plugin.OpEntries, plugin.OpFindings, plugin.OpRank, plugin.OpPreset}, plugin.Op(op)) {
		return failure(plugin.CodeUnsupported, "", "the standin serves no %s", op)
	}
	if leaves != "" && op == leaves {
		// The SDK checks a request before its plugin sees it, rank's for at
		// least one pair, so a plugin that leaves rank out answers bug to a
		// request with none (gragas#2297): the module does the same.
		if plugin.Op(op) == plugin.OpRank && len(get[list](b["pairs"])) == 0 {
			return failure(plugin.CodeBug, "/pairs", "%s", "rank is asked for no pairs")
		}
		return failure(plugin.CodeUnsupported, "", "the standin serves no %s", op)
	}
	if refused := unsupportedBy(b); refused != nil {
		return refused
	}
	if refused := malformed(plugin.Op(op), body); refused != nil {
		return refused
	}
	switch plugin.Op(op) {
	case plugin.OpPlan:
		return plan(b)
	case plugin.OpEntries:
		return entries(b)
	case plugin.OpFindings:
		return findings(b)
	case plugin.OpRank:
		return rank(b)
	case plugin.OpPreset:
		return preset(b)
	}
	return failure(plugin.CodeUnsupported, "", "the standin serves no %s", op)
}

// served is the tool major the standin serves: its manifest's range is
// ">=1.0.0 <2.0.0" (C16).
const served = 1

// unsupportedBy is the answer for a request that names what the standin does
// not serve: a tool version outside its range, a target it does not read, a for
// the contract does not name (C14, C16). A member that is absent or ill-typed
// is not named here: malformed answers it.
func unsupportedBy(b obj) []byte {
	if v, ok := str(b, "tool_version"); ok {
		major, _, _ := strings.Cut(v, ".")
		if n, err := strconv.Atoi(major); err == nil && n != served {
			return failure(plugin.CodeUnsupported, "/tool_version", "the standin serves tool versions >=1.0.0 <2.0.0, not %s", v)
		}
	}
	if t, ok := str(b, "target"); ok && t != string(plugin.TargetSource) {
		return failure(plugin.CodeUnsupported, "/target", "%s", "the standin serves source only")
	}
	if f, ok := str(b, "for"); ok && f != string(plugin.ForConfig) && f != string(plugin.ForRun) {
		return failure(plugin.CodeUnsupported, "/for", "the standin serves no for %q", f)
	}
	return nil
}

// malformed is the bug error for a body that is no request of op: a member the
// request's schema requires is missing or has the wrong type (C17). The body
// has already passed unsupportedBy, so a value of the contract's enums that the
// standin does not serve is not read here as malformed.
func malformed(op plugin.Op, body []byte) []byte {
	v, err := canon.Decode(body)
	if err != nil {
		return failure(plugin.CodeBug, "", "the body is no request: %v", err)
	}
	var rerr error
	switch op {
	case plugin.OpPlan:
		rerr = (&plugin.PlanRequest{}).FromCanon(v)
	case plugin.OpEntries:
		rerr = (&plugin.EntriesRequest{}).FromCanon(v)
	case plugin.OpFindings:
		rerr = (&plugin.FindingsRequest{}).FromCanon(v)
	case plugin.OpRank:
		rerr = (&plugin.RankRequest{}).FromCanon(v)
	default:
		rerr = (&plugin.PresetRequest{}).FromCanon(v)
	}
	if rerr != nil {
		return failure(plugin.CodeBug, "", "the body is no request of %s: %v", op, rerr)
	}
	return nil
}

// obj is a JSON object read as it is, and list a JSON array.
type (
	obj  = map[string]any
	list = []any
)

// doc is the document of a successful answer: the canonical form of a typed
// response, which a caller passes as wire.Canonical(x.ToCanon()).
func doc(body []byte, err error) []byte {
	if err != nil {
		return failure(plugin.CodeBug, "", "the answer cannot be written: %v", err)
	}
	return []byte(`{"ok":` + string(body) + "}\n")
}

// failure is the document of the plugin's own error.
func failure(code plugin.Code, path, format string, args ...any) []byte {
	e := plugin.Error{Code: code, Message: fmt.Sprintf(format, args...)}
	if path != "" {
		e.Path = &path
	}
	body, err := wire.Canonical(e.ToCanon())
	if err != nil {
		return []byte(`{"error":{"code":"bug","message":"the error cannot be written"}}` + "\n")
	}
	return []byte(`{"error":` + string(body) + "}\n")
}

// get is v as a T, and T's zero value where it is none: an absent or ill-typed member
// of a request reads as empty here, and the schema of the request is the engine's to hold.
func get[T any](v any) T {
	t, ok := v.(T)
	if !ok {
		var zero T
		return zero
	}
	return t
}

// str is the string at k of o, and whether it is one.
func str(o obj, k string) (string, bool) {
	s, ok := o[k].(string)
	return s, ok
}

// text is the text of a bytes object, and whether it is text (an absent
// object is empty text).
func text(v any) (string, bool) {
	o, present := v.(obj)
	if !present {
		return "", v == nil
	}
	return str(o, "text")
}

// configFile is the file the standin reads as its config: a plan that is given
// a name neither it nor dataFile refuses it with the path (C20).
const configFile = "standin.conf"

// dataFile is the same config under a name the engine reads as data, so that a
// journey reads a config at the level a data name and the manifest give it.
const dataFile = "standin.json"

func plan(b obj) []byte {
	if get[string](b["target"]) != string(plugin.TargetSource) {
		return failure(plugin.CodeUnsupported, "/target", "%s", "the standin serves source only")
	}
	done := plugin.PlanResponse{Steps: []plugin.Step{}, Final: true}
	if get[string](b["for"]) == string(plugin.ForRun) {
		return doc(wire.Canonical(plugin.PlanResponse{Final: true, Steps: []plugin.Step{{Run: &plugin.Run{Program: "standin", Argv: []string{"--list"}}}}}.ToCanon()))
	}
	configs := get[list](b["config"])
	for i, c := range configs {
		if p := get[string](get[obj](c)["path"]); p != configFile && p != dataFile {
			return failure(plugin.CodeRefused, "/config/"+strconv.Itoa(i)+"/path", "the standin reads %s or %s, not %s", configFile, dataFile, p)
		}
		if s, isText := text(get[obj](c)["data"]); isText {
			if why := reaches(s); why != "" {
				return failure(plugin.CodeRefused, "/config/"+strconv.Itoa(i)+"/data", "the standin hands no tool this config: %s", why)
			}
		}
	}
	for _, c := range configs {
		if get[obj](c)["data"] != nil {
			return doc(wire.Canonical(done.ToCanon()))
		}
	}
	if len(get[list](b["have"])) > 0 {
		return doc(wire.Canonical(done.ToCanon()))
	}
	return doc(wire.Canonical(plugin.PlanResponse{Steps: []plugin.Step{{Read: &plugin.Read{Glob: "standin.conf"}}}}.ToCanon()))
}

// reaches is why the plan refuses a config, "" for none: a config that reaches
// beyond its one file, as a real plugin's plan refuses one before anything runs
// (gragas#2240, gap 2). The standin's format reaches by a line whose key is
// extends, plugins or import; a key written with an escape, a backslash, may be
// read by a tool as another key; and a control character, all of Cc but tab,
// line feed and carriage return, may be too. The in-memory standin gives the
// same reasons.
func reaches(s string) string {
	if strings.ContainsFunc(s, func(r rune) bool {
		return r != '\t' && r != '\n' && r != '\r' && (r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f)
	}) {
		return "a key holds a control character, which a tool may read as another key"
	}
	for line := range strings.SplitSeq(s, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		key := strings.TrimPrefix(f[0], "-")
		if slices.Contains([]string{"extends", "plugins", "import"}, key) {
			return "it sets " + key + ", which reaches beyond its one file"
		}
		if strings.Contains(key, `\`) {
			return "a key is written with an escape, which a tool may read as another key"
		}
	}
	return ""
}

// sourceIn is the text of the first member of the list b[member] that has
// member[key], the pointer to it, and whether one was found. A member that is no
// text is the answer to give.
func sourceIn(b obj, member, key, what string) (body, at string, found bool, refused []byte) {
	for i, c := range get[list](b[member]) {
		data := get[obj](c)[key]
		if data == nil {
			continue
		}
		at = "/" + member + "/" + strconv.Itoa(i) + "/" + key
		s, isText := text(data)
		if !isText {
			return "", at, true, failure(plugin.CodeUnreadable, at, "the %s is not text", what)
		}
		return s, at, true, nil
	}
	return "", "", false, nil
}

// source is the text the standin reads its entries from and the pointer to it:
// the first config that has data, else the first result that has stdout. A
// source that is no text is the answer to give.
func source(b obj) (body, at string, refused []byte) {
	if body, at, found, refused := sourceIn(b, "config", "data", "config"); found {
		return body, at, refused
	}
	body, at, _, refused = sourceIn(b, "have", "stdout", "output")
	return body, at, refused
}

// entryOf is one line of the standin's config as an entry, and the answer to
// give where it is none.
func entryOf(line, at string) (plugin.Entry, []byte) {
	f := strings.Fields(line)
	e := plugin.Entry{On: true}
	switch {
	case len(f) == 1 && strings.HasPrefix(f[0], "-") && len(f[0]) > 1:
		e.Key, e.On = f[0][1:], false
	case len(f) == 1:
		e.Key = f[0]
	case len(f) == 2 && slices.Contains([]string{"error", "warning", "info"}, f[1]):
		s := plugin.Severity(f[1])
		e.Key, e.Severity = f[0], &s
	default:
		return e, failure(plugin.CodeUnreadable, at, "the line %q is not an entry", line)
	}
	return e, nil
}

func entries(b obj) []byte {
	if get[string](b["target"]) != string(plugin.TargetSource) {
		return failure(plugin.CodeUnsupported, "/target", "%s", "the standin serves source only")
	}
	body, at, refused := source(b)
	if refused != nil {
		return refused
	}
	out := []plugin.Entry{}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e, refused := entryOf(line, at)
		if refused != nil {
			return refused
		}
		if seen[e.Key] {
			return failure(plugin.CodeUnreadable, at, "the key %q is there twice", e.Key)
		}
		seen[e.Key] = true
		out = append(out, e)
	}
	slices.SortFunc(out, func(x, y plugin.Entry) int { return strings.Compare(x.Key, y.Key) })
	return doc(wire.Canonical(plugin.EntriesResponse{Entries: out}.ToCanon()))
}

// findingOf is one line "path:line:key:message" as a finding, and the answer to
// give where it is none.
func findingOf(line, at string) (plugin.Finding, []byte) {
	f := strings.SplitN(line, ":", 4)
	if len(f) != 4 {
		return plugin.Finding{}, failure(plugin.CodeUnreadable, at, "the line %q is not path:line:key:message", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(f[1]))
	if err != nil || n < 1 {
		return plugin.Finding{}, failure(plugin.CodeUnreadable, at, "the line %q is not path:line:key:message", line)
	}
	path := f[0]
	return plugin.Finding{Key: f[2], Message: strings.TrimSpace(f[3]), Path: &path, Line: &n}, nil
}

// compareFindings orders findings by path, line, key and message.
func compareFindings(x, y plugin.Finding) int {
	if c := strings.Compare(*x.Path, *y.Path); c != 0 {
		return c
	}
	if c := *x.Line - *y.Line; c != 0 {
		return c
	}
	if c := strings.Compare(x.Key, y.Key); c != 0 {
		return c
	}
	return strings.Compare(x.Message, y.Message)
}

func findings(b obj) []byte {
	for i, h := range get[list](b["have"]) {
		o := get[obj](h)
		if o["kind"] != string(plugin.KindRun) || o["stdout"] == nil {
			continue
		}
		at := "/have/" + strconv.Itoa(i) + "/stdout"
		body, isText := text(o["stdout"])
		if !isText {
			return failure(plugin.CodeUnreadable, at, "%s", "the output is not text")
		}
		out := []plugin.Finding{}
		for line := range strings.SplitSeq(body, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			f, refused := findingOf(line, at)
			if refused != nil {
				return refused
			}
			out = append(out, f)
		}
		slices.SortFunc(out, compareFindings)
		return doc(wire.Canonical(plugin.FindingsResponse{Ran: true, Findings: out}.ToCanon()))
	}
	why := "nothing ran"
	return doc(wire.Canonical(plugin.FindingsResponse{Ran: false, Why: &why, Findings: []plugin.Finding{}}.ToCanon()))
}

// whole is the integer a JSON number is, and whether it is one.
func whole(v any) (int64, bool) {
	f, ok := v.(float64)
	if !ok || f != float64(int64(f)) {
		return 0, false
	}
	return int64(f), true
}

func rank(b obj) []byte {
	pairs := get[list](b["pairs"])
	words := make([]plugin.Word, len(pairs))
	for i, p := range pairs {
		o := get[obj](p)
		preset, a := whole(o["preset"])
		found, c := whole(o["found"])
		switch {
		case !a || !c:
			words[i] = plugin.WordUnknown
		case found < preset:
			words[i] = plugin.WordTighter
		case found > preset:
			words[i] = plugin.WordWeaker
		default:
			words[i] = plugin.WordSame
		}
	}
	return doc(wire.Canonical(plugin.RankResponse{Words: words}.ToCanon()))
}

// presetEntry is the key and severity of one entry of a preset request.
type presetEntry struct{ key, severity string }

// presetEntries reads the entries of a preset request, and the answer to give
// for one the standin refuses.
func presetEntries(b obj) ([]presetEntry, []byte) {
	var es []presetEntry
	for i, r := range get[list](b["entries"]) {
		o := get[obj](r)
		at := "/entries/" + strconv.Itoa(i)
		key := get[string](o["key"])
		switch {
		case get[string](o["target"]) != string(plugin.TargetSource):
			return nil, failure(plugin.CodeUnsupported, at+"/target", "%s", "the standin serves source only")
		case o["options"] != nil:
			return nil, failure(plugin.CodeRefused, at+"/options", "%s", "the standin writes no options")
		case key == "bug":
			return nil, failure(plugin.CodeBug, "", "%s", "the standin was asked for the key that is its own bug")
		}
		es = append(es, presetEntry{key, get[string](o["severity"])})
	}
	return es, nil
}

func preset(b obj) []byte {
	es, refused := presetEntries(b)
	if refused != nil {
		return refused
	}
	slices.SortFunc(es, func(x, y presetEntry) int { return strings.Compare(x.key, y.key) })
	var sb strings.Builder
	if mark, present := str(b, "mark"); present {
		sb.WriteString("# " + mark + "\n")
	}
	for _, e := range es {
		sb.WriteString(e.key)
		if e.severity != "" {
			sb.WriteString(" " + e.severity)
		}
		sb.WriteString("\n")
	}
	return doc(wire.Canonical(plugin.PresetResponse{Files: []plugin.PresetFile{{Path: "standin.conf", Text: sb.String()}}}.ToCanon()))
}
