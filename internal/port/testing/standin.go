package porttest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// NewStandinPlugin is the plugin every core test uses:
// a manifest, and an answer to each of the five operations that is a small
// deterministic function of the request, so a test drives the engine through it
// and needs no scripting. It serves no tool: the "config" it reads is lines of
// "key", "key severity" or "-key" (off), and the "run" it asks for lists them.
// The wasm standin of internal/dev/standin gives the same answer to the same
// request, and the suite of internal/port/contracttest holds the two to the
// same cases. A test that needs another answer overwrites a field.
func NewStandinPlugin() *StandinPlugin {
	return &StandinPlugin{
		Facts: plugin.Manifest{
			Name: "standin", Version: "1.0.0", Tool: "standin", ToolVersions: ">=1.0.0 <2.0.0",
			Targets: []plugin.Target{plugin.TargetSource},
			Provides: []plugin.Op{
				plugin.OpPlan, plugin.OpEntries, plugin.OpFindings, plugin.OpRank, plugin.OpPreset,
			},
			Extends: true, Warning: true,
		},
		OnPlan: standinPlan, OnEntries: standinEntries, OnFindings: standinFindings,
		OnRank: standinRank, OnPreset: standinPreset,
	}
}

// standinError is a plugin's own error with a path.
func standinError(code plugin.Code, path, format string, args ...any) error {
	return &plugin.CallError{Code: code, Message: fmt.Sprintf(format, args...), Path: path}
}

func standinPlan(in plugin.PlanRequest) (plugin.PlanResponse, error) {
	if in.Target != plugin.TargetSource {
		return plugin.PlanResponse{}, standinError(plugin.CodeUnsupported, "/target", "the standin serves source only")
	}
	if in.For == plugin.ForRun {
		return plugin.PlanResponse{Final: true, Steps: []plugin.Step{{Run: &plugin.Run{Program: "standin", Argv: []string{"--list"}}}}}, nil
	}
	for i, c := range in.Config {
		if text, isText := textOf(c.Data); isText {
			if why := standinReaches(text); why != "" {
				return plugin.PlanResponse{}, standinError(plugin.CodeRefused, "/config/"+strconv.Itoa(i)+"/data", "the standin hands no tool this config: %s", why)
			}
		}
	}
	for _, c := range in.Config {
		if c.Data != nil {
			return plugin.PlanResponse{Steps: []plugin.Step{}, Final: true}, nil
		}
	}
	if len(in.Have) > 0 {
		return plugin.PlanResponse{Steps: []plugin.Step{}, Final: true}, nil
	}
	return plugin.PlanResponse{Steps: []plugin.Step{{Read: &plugin.Read{Glob: "standin.conf"}}}}, nil
}

// standinReaches is why the standin's plan refuses a config, "" for none: a
// config that reaches beyond its one file, as a real plugin's plan refuses one
// before anything runs. The standin's format reaches by a
// line whose key is extends, plugins or import; a key written with an escape,
// a backslash, may be read by a tool as another key; and a control character,
// all of Cc but tab, line feed and carriage return, may be too. The wasm
// standin gives the same reasons.
func standinReaches(text string) string {
	if strings.ContainsFunc(text, func(r rune) bool {
		return r != '\t' && r != '\n' && r != '\r' && (r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f)
	}) {
		return "a key holds a control character, which a tool may read as another key"
	}
	for line := range strings.SplitSeq(text, "\n") {
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

// textOf is the text of bytes, and false for bytes that are not text.
func textOf(b *plugin.Bytes) (string, bool) {
	if b == nil || b.Text == nil {
		return "", b == nil
	}
	return *b.Text, true
}

// sourceOf is the text the standin reads its entries from and the pointer to
// it: the first config that has data, else the first result that has stdout.
func sourceOf(in plugin.EntriesRequest) (text, at string, err error) {
	for i, c := range in.Config {
		if c.Data != nil {
			at = "/config/" + strconv.Itoa(i) + "/data"
			s, ok := textOf(c.Data)
			if !ok {
				return "", at, standinError(plugin.CodeUnreadable, at, "the config is not text")
			}
			return s, at, nil
		}
	}
	for i, h := range in.Have {
		if h.Stdout != nil {
			at = "/have/" + strconv.Itoa(i) + "/stdout"
			s, ok := textOf(h.Stdout)
			if !ok {
				return "", at, standinError(plugin.CodeUnreadable, at, "the output is not text")
			}
			return s, at, nil
		}
	}
	return "", "", nil
}

// entryOf is one line of the standin's config as an entry.
func entryOf(line, at string) (plugin.Entry, error) {
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
		return e, standinError(plugin.CodeUnreadable, at, "the line %q is not an entry", line)
	}
	return e, nil
}

func standinEntries(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
	if in.Target != plugin.TargetSource {
		return plugin.EntriesResponse{}, standinError(plugin.CodeUnsupported, "/target", "the standin serves source only")
	}
	text, at, err := sourceOf(in)
	if err != nil {
		return plugin.EntriesResponse{}, err
	}
	entries := []plugin.Entry{}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e, err := entryOf(line, at)
		if err != nil {
			return plugin.EntriesResponse{}, err
		}
		if seen[e.Key] {
			return plugin.EntriesResponse{}, standinError(plugin.CodeUnreadable, at, "the key %q is there twice", e.Key)
		}
		seen[e.Key] = true
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b plugin.Entry) int { return strings.Compare(a.Key, b.Key) })
	return plugin.EntriesResponse{Entries: entries}, nil
}

// findingOf is one line "path:line:key:message" as a finding.
func findingOf(line, at string) (plugin.Finding, error) {
	f := strings.SplitN(line, ":", 4)
	if len(f) != 4 {
		return plugin.Finding{}, standinError(plugin.CodeUnreadable, at, "the line %q is not path:line:key:message", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(f[1]))
	if err != nil || n < 1 {
		return plugin.Finding{}, standinError(plugin.CodeUnreadable, at, "the line %q is not path:line:key:message", line)
	}
	path := f[0]
	return plugin.Finding{Key: f[2], Message: strings.TrimSpace(f[3]), Path: &path, Line: &n}, nil
}

// compareFindings orders findings by path, line, key and message.
func compareFindings(a, b plugin.Finding) int {
	if c := strings.Compare(*a.Path, *b.Path); c != 0 {
		return c
	}
	if c := *a.Line - *b.Line; c != 0 {
		return c
	}
	if c := strings.Compare(a.Key, b.Key); c != 0 {
		return c
	}
	return strings.Compare(a.Message, b.Message)
}

func standinFindings(in plugin.FindingsRequest) (plugin.FindingsResponse, error) {
	for i, h := range in.Have {
		if h.Kind != plugin.KindRun || h.Stdout == nil {
			continue
		}
		at := "/have/" + strconv.Itoa(i) + "/stdout"
		text, ok := textOf(h.Stdout)
		if !ok {
			return plugin.FindingsResponse{}, standinError(plugin.CodeUnreadable, at, "the output is not text")
		}
		out := []plugin.Finding{}
		for line := range strings.SplitSeq(text, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			f, err := findingOf(line, at)
			if err != nil {
				return plugin.FindingsResponse{}, err
			}
			out = append(out, f)
		}
		slices.SortFunc(out, compareFindings)
		return plugin.FindingsResponse{Ran: true, Findings: out}, nil
	}
	why := "nothing ran"
	return plugin.FindingsResponse{Ran: false, Why: &why, Findings: []plugin.Finding{}}, nil
}

func standinRank(in plugin.RankRequest) (plugin.RankResponse, error) {
	words := make([]plugin.Word, len(in.Pairs))
	for i, p := range in.Pairs {
		preset, a := p.Preset.(canon.Int)
		found, b := p.Found.(canon.Int)
		switch {
		case !a || !b:
			words[i] = plugin.WordUnknown
		case found < preset:
			words[i] = plugin.WordTighter
		case found > preset:
			words[i] = plugin.WordWeaker
		default:
			words[i] = plugin.WordSame
		}
	}
	return plugin.RankResponse{Words: words}, nil
}

func standinPreset(in plugin.PresetRequest) (plugin.PresetResponse, error) {
	entries := slices.Clone(in.Entries)
	for i, e := range entries {
		at := "/entries/" + strconv.Itoa(i)
		switch {
		case e.Target != plugin.TargetSource:
			return plugin.PresetResponse{}, standinError(plugin.CodeUnsupported, at+"/target", "the standin serves source only")
		case e.Options != nil:
			return plugin.PresetResponse{}, standinError(plugin.CodeRefused, at+"/options", "the standin writes no options")
		case e.Key == "bug":
			return plugin.PresetResponse{}, standinError(plugin.CodeBug, "", "the standin was asked for the key that is its own bug")
		}
	}
	slices.SortFunc(entries, func(a, b plugin.PresetEntry) int { return strings.Compare(a.Key, b.Key) })
	var b strings.Builder
	if in.Mark != nil {
		b.WriteString("# " + *in.Mark + "\n")
	}
	for _, e := range entries {
		b.WriteString(e.Key)
		if e.Severity != nil {
			b.WriteString(" " + string(*e.Severity))
		}
		b.WriteString("\n")
	}
	return plugin.PresetResponse{Files: []plugin.PresetFile{{Path: "standin.conf", Text: b.String()}}}, nil
}
