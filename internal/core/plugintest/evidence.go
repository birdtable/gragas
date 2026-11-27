package plugintest

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/readglob"
)

// kase is a case that read: its request and its saved response, typed by the
// operation the folder names. A case that does not read is C6's and C8's to
// say, so it holds no evidence here.
type kase struct {
	name string
	req  any
	resp plugin.Response
}

// read answers the cases of op that read.
func read(b Bundle, op plugin.Op) []kase {
	var out []kase
	for _, c := range b.Cases {
		if OpOf(c.Name) != string(op) || c.Missing != "" {
			continue
		}
		rv, err := canon.Decode(c.Request)
		if err != nil {
			continue
		}
		var r plugin.Request
		if r.FromCanon(rv) != nil || r.Op != op {
			continue
		}
		body := requestBody(op)
		if body.FromCanon(r.Body) != nil {
			continue
		}
		pv, err := canon.Decode(c.Response)
		if err != nil {
			continue
		}
		var resp plugin.Response
		if resp.FromCanon(pv) != nil {
			continue
		}
		out = append(out, kase{c.Name, body, resp})
	}
	return out
}

// evidence is C18 to C23, each over the operation it names; one a plugin does
// not provide asks for nothing.
func evidence(ctx context.Context, b Bundle, o canon.Object) []model.PluginCheck {
	if o == nil {
		var out []model.PluginCheck
		for n := 18; n <= 23; n++ {
			out = append(out, skipped(n, unread))
		}
		return out
	}
	has := func(op plugin.Op) bool { return slices.Contains(list(o["provides"]), string(op)) }
	run := func(n int, op plugin.Op, f func() []string) model.PluginCheck {
		if !has(op) {
			return notAsked(n, string(op)+" is not in provides")
		}
		return result(n, f())
	}
	return []model.PluginCheck{
		run(18, plugin.OpEntries, func() []string { return recordings(b, read(b, plugin.OpEntries)) }),
		run(19, plugin.OpFindings, func() []string { return findings(read(b, plugin.OpFindings)) }),
		run(20, plugin.OpPlan, func() []string { return plans(read(b, plugin.OpPlan)) }),
		run(21, plugin.OpPreset, func() []string { return presets(read(b, plugin.OpPreset)) }),
		run(22, plugin.OpRank, func() []string { return ranks(read(b, plugin.OpRank)) }),
		roundTrip(ctx, b, o),
	}
}

// recordings is C18: every tool version an entries case asks for has a
// recording folder with a file in it.
func recordings(b Bundle, cases []kase) []string {
	if len(cases) == 0 {
		return []string{"entries is provided and testdata/cases holds no entries case"}
	}
	var what []string
	seen := map[string]bool{}
	for _, c := range cases {
		req, ok := c.req.(*plugin.EntriesRequest)
		if !ok {
			continue
		}
		v := req.ToolVersion
		if seen[v] {
			continue
		}
		seen[v] = true
		i := slices.IndexFunc(b.Recordings, func(r Recording) bool { return r.Version == v })
		if i < 0 || len(b.Recordings[i].Files) == 0 {
			what = append(what, fmt.Sprintf("tool version %s is tested by case %s and testdata/recordings/%s holds no recording", v, c.name, v))
		}
	}
	return what
}

// findings is C19: a case whose output holds a finding, one whose output is
// clean, and one where the tool did not run.
func findings(cases []kase) []string {
	var planted, clean, failed bool
	for _, c := range cases {
		if c.resp.Ok == nil {
			continue
		}
		var r plugin.FindingsResponse
		if r.FromCanon(c.resp.Ok) != nil {
			continue
		}
		switch {
		case !r.Ran:
			failed = failed || r.Why != nil
		case len(r.Findings) > 0:
			planted = true
		default:
			clean = true
		}
	}
	var what []string
	for _, w := range []struct {
		ok   bool
		said string
	}{
		{planted, "no findings case whose planted output answers a finding"},
		{clean, "no findings case whose clean control answers none"},
		{failed, "no findings case where the tool failed and the answer says ran false and why"},
	} {
		if !w.ok {
			what = append(what, w.said)
		}
	}
	return what
}

// plans is C20: a case that ends in one round, and a refusal naming a path
// where the plan refuses a config. The refusal must be of a config's content
// before any step: a request for config at round 0 that gives the config's
// data, answered refused at that data or under it. A
// config that reaches beyond its one file is the plan's to refuse, as only the
// plugin knows what its tool's reader takes for extends, plugins or an import,
// and what an escape reads as; a plugin that has no such case has shown it
// refuses none. Which content a case refuses is the plugin's review: no check
// here knows the tool's format.
func plans(cases []kase) []string {
	var one, refused, byContent bool
	for _, c := range cases {
		req, ok := c.req.(*plugin.PlanRequest)
		if !ok {
			continue
		}
		if c.resp.Error != nil {
			if c.resp.Error.Code == plugin.CodeRefused && c.resp.Error.Path != nil && req.For == plugin.ForConfig {
				refused = true
				byContent = byContent || refusesContent(req, *c.resp.Error.Path)
			}
			continue
		}
		var r plugin.PlanResponse
		if r.FromCanon(c.resp.Ok) == nil && r.Final && req.Round == 0 {
			one = true
		}
	}
	var what []string
	if !one {
		what = append(what, "no plan case ends in one round: a request for round 0 answering final")
	}
	switch {
	case !refused:
		what = append(what, "no plan case refuses a config with a path: for config, answering refused with a path")
	case !byContent:
		what = append(what, "no plan case refuses a config by its content before any step: for config at round 0, a config given with its data, answering refused with a path at /config/<n>/data or under it")
	}
	return what
}

// refusesContent is whether at, the path of a refusal of req, points at the
// data of a config req gives, at round 0, so the plan refused what the config
// holds before any step ran.
func refusesContent(req *plugin.PlanRequest, at string) bool {
	if req.Round != 0 {
		return false
	}
	for i, c := range req.Config {
		data := fmt.Sprintf("/config/%d/data", i)
		if c.Data != nil && (at == data || strings.HasPrefix(at, data+"/")) {
			return true
		}
	}
	return false
}

// presets is C21: a golden, the mark on the first line of each file it writes,
// and a refusal naming an option.
func presets(cases []kase) []string {
	var golden, refused bool
	var what []string
	for _, c := range cases {
		req, ok := c.req.(*plugin.PresetRequest)
		if !ok {
			continue
		}
		if c.resp.Error != nil {
			refused = refused || c.resp.Error.Code == plugin.CodeRefused && c.resp.Error.Path != nil
			continue
		}
		var r plugin.PresetResponse
		if r.FromCanon(c.resp.Ok) != nil {
			continue
		}
		golden = true
		for _, f := range r.Files {
			first, _, _ := strings.Cut(f.Text, "\n")
			if req.Mark == nil || !strings.Contains(first, *req.Mark) {
				what = append(what, fmt.Sprintf("case %s: the first line of %s does not hold the mark", c.name, f.Path))
			}
		}
	}
	if !golden {
		what = append(what, "no preset case holds a golden")
	}
	if !refused {
		what = append(what, "no preset case refuses an option with a path")
	}
	return what
}

// ranks is C22: each word of tighter, weaker and same is answered, and a pair
// swapped answers the swapped word.
func ranks(cases []kase) []string {
	type key struct {
		key    string
		aspect plugin.Aspect
		a, b   string
	}
	answers := map[key]plugin.Word{}
	seen := map[plugin.Word]bool{}
	var what []string
	for _, c := range cases {
		req, ok := c.req.(*plugin.RankRequest)
		if !ok {
			continue
		}
		var r plugin.RankResponse
		if c.resp.Ok == nil || r.FromCanon(c.resp.Ok) != nil {
			continue
		}
		if len(r.Words) != len(req.Pairs) {
			what = append(what, fmt.Sprintf("case %s: %d words answer %d pairs", c.name, len(r.Words), len(req.Pairs)))
			continue
		}
		for i, p := range req.Pairs {
			answers[key{p.Key, p.Aspect, enc(p.Preset), enc(p.Found)}] = r.Words[i]
			seen[r.Words[i]] = true
		}
	}
	for _, w := range []plugin.Word{plugin.WordTighter, plugin.WordWeaker, plugin.WordSame} {
		if !seen[w] {
			what = append(what, fmt.Sprintf("no rank pair is answered %s", w))
		}
	}
	swap := map[plugin.Word]plugin.Word{plugin.WordTighter: plugin.WordWeaker, plugin.WordWeaker: plugin.WordTighter, plugin.WordSame: plugin.WordSame}
	var lines []string
	for k, w := range answers {
		want, ok := swap[w]
		if !ok {
			continue
		}
		got, found := answers[key{k.key, k.aspect, k.b, k.a}]
		switch {
		case !found:
			lines = append(lines, fmt.Sprintf("the pair for %s (%s) answered %s has no swapped pair", k.key, k.aspect, w))
		case got != want:
			lines = append(lines, fmt.Sprintf("the pair for %s (%s) answered %s, swapped it answers %s, not %s", k.key, k.aspect, w, got, want))
		}
	}
	sort.Strings(lines)
	return append(what, lines...)
}

// roundTrip is C23: the entries of each preset golden, written and read back by
// the plugin, are the same entries.
func roundTrip(ctx context.Context, b Bundle, o canon.Object) model.PluginCheck {
	for _, op := range []plugin.Op{plugin.OpEntries, plugin.OpPreset} {
		if !slices.Contains(list(o["provides"]), string(op)) {
			return notAsked(23, string(op)+" is not in provides")
		}
	}
	if b.Plugin == nil {
		return skipped(23, unopened(b))
	}
	rule := readbackOf(o)
	var what []string
	golden := 0
	for _, c := range read(b, plugin.OpPreset) {
		var r plugin.PresetResponse
		if c.resp.Ok == nil || r.FromCanon(c.resp.Ok) != nil {
			continue
		}
		req, ok := c.req.(*plugin.PresetRequest)
		if !ok {
			continue
		}
		golden++
		have, reads, found := recordedHave(b, req.ToolVersion)
		if !found {
			return skipped(23, fmt.Sprintf("no entries case for tool version %s holds a recorded run (have) to read %s back with", req.ToolVersion, c.name))
		}
		if _, _, ok := readPlace(have); reads && !ok {
			return skipped(23, fmt.Sprintf("the recorded runs for tool version %s end at round 2 step 15, where no step is left to hand the files of %s over in", req.ToolVersion, c.name))
		}
		what = append(what, rule.readBack(ctx, b.Plugin, c.name, req, recording{have, reads}, r)...)
	}
	if golden == 0 {
		return skipped(23, "testdata/cases holds no preset golden to read back")
	}
	return result(23, what)
}

// readbackRule is what the manifest's preset.readback lets C23 leave out of the
// comparison: whether a severity is compared, and which keys the tool turns on
// by default.
type readbackRule struct {
	severity bool
	defaults map[string]bool
}

// readbackOf reads the declaration from the manifest; a manifest with none
// compares severity and excuses no default.
func readbackOf(o canon.Object) readbackRule {
	rule := readbackRule{severity: true}
	p, ok := o["preset"].(canon.Object)
	if !ok || p["readback"] == nil {
		return rule
	}
	var r plugin.PresetReadback
	if r.FromCanon(p["readback"]) != nil {
		return rule
	}
	if r.Severity != nil {
		rule.severity = *r.Severity
	}
	if len(r.Defaults) > 0 {
		rule.defaults = map[string]bool{}
		for _, k := range r.Defaults {
			rule.defaults[k] = true
		}
	}
	return rule
}

// recordedHave is the run's results the first entries case of a tool version
// holds in its have: the recording that stands in for the tool's run, which a
// plugin whose entries come from the tool's own output reads. Only the results
// of running the tool are kept: a read result is the recording's
// read of other files, which a plugin that reads files would read in place of
// the preset's, so a recording of reads alone gives the read-back no result.
// reads is whether that have held a read result: the plugin reads files, so the
// read-back hands it the preset's files as a read; a plugin whose
// recording holds none reads no files, and is given the preset in its config
// alone, once, as a check would, so the preset is not counted twice against the
// entries cap. found is false where no case of that version holds a have.
func recordedHave(b Bundle, version string) (have []plugin.Result, reads, found bool) {
	for _, c := range read(b, plugin.OpEntries) {
		if req, ok := c.req.(*plugin.EntriesRequest); ok && req.ToolVersion == version && len(req.Have) > 0 {
			for _, r := range req.Have {
				switch r.Kind {
				case plugin.KindRun:
					have = append(have, r)
				case plugin.KindRead:
					reads = true
				}
			}
			return have, reads, true
		}
	}
	return nil, false, false
}

// readPlace is where the read of the preset's files stands among the recorded
// results, inside the contract's bounds (a round is 0 to 2, a step 0 to 15): in
// the last recorded round at the step after its last one while that is at most
// 15, else at the start of the next round while that is at most 2. ok is false
// where no step is left.
func readPlace(have []plugin.Result) (round, step int, ok bool) {
	const lastRound, lastStep = 2, 15
	if len(have) == 0 {
		return 0, 0, true
	}
	for _, h := range have {
		if h.Round > round || (h.Round == round && h.Step > step) {
			round, step = h.Round, h.Step
		}
	}
	if step < lastStep {
		return round, step + 1, true
	}
	if round < lastRound {
		return round + 1, 0, true
	}
	return 0, 0, false
}

// recording is what the first entries case of a tool version holds of a run:
// the run results of its have, and whether it also held a read.
type recording struct {
	have  []plugin.Result
	reads bool
}

// split is what a read-back of target t hands the plugin: the preset's files
// as the config and the have. A plugin whose recording held no read gets the
// preset in its config alone, once. One whose recording held a read is asked
// for its plan, with the preset as the config, and the files its read globs
// match are handed over as a read result, in the first place after the
// recorded runs. The config is always the whole preset, as the engine sends it
// (rounds.go entriesRequest sends the config unchanged beside every read
// result), so a file a read takes is in the request twice, as it is in the
// engine's, and a preset the read takes whole over half the entries cap fails
// here as it does there (the engine does not yet leave a file its read
// returned out of the config). The declarative SDK reads its rule files from
// the read and from nowhere else. A plan that asks for no read, or fails, gets no read.
func (rec recording) split(ctx context.Context, p Caller, version string, t plugin.Target, r plugin.PresetResponse) ([]plugin.Config, []plugin.Result) {
	all := make([]plugin.Config, 0, len(r.Files))
	for _, f := range r.Files {
		tx := f.Text
		all = append(all, plugin.Config{Path: f.Path, Data: &plugin.Bytes{Text: &tx}})
	}
	if !rec.reads {
		return all, rec.have
	}
	round, step, _ := readPlace(rec.have)
	globs := rec.globs(ctx, p, version, t, all, round)
	if globs == nil {
		return all, rec.have
	}
	var files []plugin.File
	for _, f := range r.Files {
		if matches(globs, f.Path) {
			tx := f.Text
			files = append(files, plugin.File{Path: f.Path, Data: plugin.Bytes{Text: &tx}})
		}
	}
	return all, append(slices.Clone(rec.have), plugin.Result{Round: round, Step: step, Kind: plugin.KindRead, Files: files})
}

// globs is the read globs the plugin's plan asks for target t, in the round the
// read stands in, with the preset as the config and, as the engine asks round
// r's plan, only the recorded runs of the rounds before r as the have: the
// steps of a round are planned together, so round 0 sees none. It is nil where
// the plan fails or asks for none, as no glob is then to match.
func (rec recording) globs(ctx context.Context, p Caller, version string, t plugin.Target, cfg []plugin.Config, round int) [][]string {
	before := slices.DeleteFunc(slices.Clone(rec.have), func(h plugin.Result) bool { return h.Round >= round })
	plan, err := p.Plan(ctx, plugin.PlanRequest{ToolVersion: version, Config: cfg, Have: before, For: plugin.ForConfig, Target: t, Round: round})
	if err != nil {
		return nil
	}
	var out [][]string
	for _, s := range plan.Steps {
		if s.Read == nil {
			continue
		}
		if parts, err := readglob.Parts(s.Read.Glob); err == nil {
			out = append(out, parts)
		}
	}
	return out
}

// matches is whether any of the globs, split at their slashes, matches path.
func matches(globs [][]string, path string) bool {
	parts := strings.Split(path, "/")
	return slices.ContainsFunc(globs, func(g []string) bool { return readglob.Match(g, parts) })
}

// readBack writes the files of a preset golden as config, has the plugin read
// them for each target, and says where the entries differ from the preset's.
func (rb readbackRule) readBack(ctx context.Context, p Caller, name string, req *plugin.PresetRequest, rec recording, r plugin.PresetResponse) []string {
	byTarget := map[plugin.Target][]plugin.PresetEntry{}
	trial := map[plugin.Target]map[string]bool{}
	for _, e := range req.Entries {
		if e.Trial {
			if trial[e.Target] == nil {
				trial[e.Target] = map[string]bool{}
			}
			trial[e.Target][e.Key] = true
			continue
		}
		byTarget[e.Target] = append(byTarget[e.Target], e)
	}
	var what []string
	for t, want := range byTarget {
		cfg, have := rec.split(ctx, p, req.ToolVersion, t, r)
		got, err := p.Entries(ctx, plugin.EntriesRequest{ToolVersion: req.ToolVersion, Config: cfg, Have: have, Target: t})
		if err != nil {
			what = append(what, fmt.Sprintf("case %s: reading the preset back for %s failed: %v", name, t, err))
			continue
		}
		what = append(what, diff(name, t, want, got.Entries, trial[t], rb)...)
	}
	return what
}

// enc is a value's canonical text, "" for one that does not encode.
func enc(v canon.Value) string {
	if v == nil {
		v = canon.Null{}
	}
	out, err := canon.Encode(v, canon.Canonical)
	if err != nil {
		return ""
	}
	return string(out)
}

// severityNotPlaced names each entry read back on with a severity, which a plugin
// whose manifest says it places none cannot have placed.
func severityNotPlaced(name string, t plugin.Target, got []plugin.Entry) []string {
	var what []string
	for _, e := range got {
		if e.On && e.Severity != nil {
			what = append(what, fmt.Sprintf("case %s: %s of %s is read back with a severity, and the manifest's readback says the plugin places none", name, e.Key, t))
		}
	}
	return what
}

// diff says where the entries read back are not the preset's. A key in trial is
// one the preset request carries as a trial entry: the preset leaves it out of
// its files on purpose, so a tool that has it on is not an entry "read back and
// not in the preset". A trial key is excused on that side alone;
// it stays red if it is also a non-trial entry the preset does not read back.
//
// The declaration (preset.readback) narrows the comparison and no
// more: with severity false no severity is compared, and one read back is red,
// since a plugin that says it places none cannot place one; a key in defaults
// is the tool's own default, excused on the "read back and not in the preset"
// side alone. An entry the preset holds and the plugin drops or changes is red
// whatever the declaration says.
func diff(name string, t plugin.Target, want []plugin.PresetEntry, got []plugin.Entry, trial map[string]bool, rb readbackRule) []string {
	sev := func(s *plugin.Severity) string {
		if s == nil || !rb.severity {
			return ""
		}
		return string(*s)
	}
	on := map[string]plugin.Entry{}
	for _, e := range got {
		if e.On {
			on[e.Key] = e
		}
	}
	var what []string
	if !rb.severity {
		what = severityNotPlaced(name, t, got)
	}
	for _, w := range want {
		g, ok := on[w.Key]
		switch {
		case !ok:
			what = append(what, fmt.Sprintf("case %s: %s of %s is in the preset and not read back", name, w.Key, t))
		case enc(g.Value) != enc(w.Value) || enc(g.Options) != enc(w.Options) || sev(g.Severity) != sev(w.Severity):
			what = append(what, fmt.Sprintf("case %s: %s of %s reads back with another value, options or severity", name, w.Key, t))
		}
		delete(on, w.Key)
	}
	for k := range on {
		if trial[k] || rb.defaults[k] {
			continue
		}
		what = append(what, fmt.Sprintf("case %s: %s of %s is read back and not in the preset", name, k, t))
	}
	sort.Strings(what)
	return what
}
