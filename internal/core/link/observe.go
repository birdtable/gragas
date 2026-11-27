package link

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/ordered"
)

// Observe says, for each entry of the resolved preset, how the entries a
// plugin read from the tool's effective config stand against it: as-set,
// tighter, weaker, absent or excluded, and unmapped for each entry of the
// config no preset entry explains (docs/spec.md 17.4, S33).
//
// It is pure: no file, no tool, no clock. The caller asks the plugin for the
// entries (EntriesCall) and resolves the chain, and the release the preset is
// of is the caller's choice (S14); a read that failed is the caller's Unknown
// with the reason, and no entries. The plugin has already put each entry on
// the engine's terms: whether it is on, and the severity it stands at. What
// stays here is how aspects combine and that off is weaker: an entry that is
// off is below every severity and every value. No tool is named and no
// language has a collector here.
//
// It never guesses. Which of two values or two option sets is the tighter is
// the plugin's rank, not asked in release 1, so a value or
// options that differ carry a Reason and no word, as they do where an adapter
// gave no order. Where one aspect of an entry is looser the entry is weaker
// whatever its other aspects, and where one is tighter and none is looser it
// is tighter. Two preset entries that hold one tool entry (an entry conflict,
// S8) are one observation, made against the first that no overlay excluded;
// the conflict is the rulebook's finding.
func Observe(tool string, target model.TargetType, entries []plugin.Entry, preset []model.PresetEntry) model.Observed {
	held := map[string][]plugin.Entry{}
	for _, e := range entries {
		held[e.Key] = append(held[e.Key], e)
	}
	var keys []string
	groups := map[string][]model.PresetEntry{}
	for _, p := range preset {
		if p.Binding.Tool != tool || p.Binding.Target != target {
			continue
		}
		if _, seen := groups[p.Binding.Key]; !seen {
			keys = append(keys, p.Binding.Key)
		}
		groups[p.Binding.Key] = append(groups[p.Binding.Key], p)
	}
	var out []model.Observation
	for _, key := range keys {
		out = append(out, observeEntry(key, groups[key], held[key]))
	}
	for _, e := range entries {
		if _, named := groups[e.Key]; named || seenKey(out, e.Key) {
			continue
		}
		out = append(out, model.Observation{Key: e.Key, Word: model.ObservedUnmapped})
	}
	return model.Observed{Entries: out}
}

// observeEntry is the observation of one tool entry: the preset entries that
// hold it, and the config entries with its key.
func observeEntry(key string, preset []model.PresetEntry, held []plugin.Entry) model.Observation {
	live := slices.IndexFunc(preset, func(p model.PresetEntry) bool { return p.Excluded == nil })
	if live < 0 {
		// Every rule that holds the entry was dropped: the config shows it
		// as unmapped and says who dropped it, or does not hold it.
		o := model.Observation{Key: key, Word: model.ObservedExcluded, Provenance: provenance(preset[0])}
		if len(held) > 0 {
			o.Word = model.ObservedUnmapped
		}
		return o
	}
	p := preset[live]
	o := model.Observation{Key: key, Provenance: provenance(p)}
	switch {
	case len(held) == 0:
		o.Word = model.ObservedAbsent
	case len(held) > 1:
		o.Reason = "the effective config holds the entry " + fmt.Sprint(len(held)) + " times, so no one of them is the one that runs"
	default:
		o.Word, o.Reason = compare(p.Binding, held[0])
	}
	return o
}

// aspect is how one part of an entry stands against the preset's: looser,
// the same, tighter, or unread with the reason.
type aspect struct {
	rank   int
	reason string
}

const (
	looser  = -1
	same    = 0
	tighter = 1
)

// compare is the word for an entry the config holds once, or the reason none
// can be given.
func compare(b model.Binding, e plugin.Entry) (model.ObservationWord, string) {
	aspects := []aspect{severityAspect(b, e), valueAspect(b, e), optionsAspect(b, e)}
	var reason string
	rank := same
	for _, x := range aspects {
		switch {
		case x.reason != "":
			if reason == "" {
				reason = x.reason
			}
		case x.rank == looser:
			return model.ObservedWeaker, ""
		case x.rank == tighter:
			rank = tighter
		}
	}
	switch {
	case reason != "":
		return "", reason
	case rank == tighter:
		return model.ObservedTighter, ""
	}
	return model.ObservedAsSet, ""
}

// severityAspect is the entry's severity against the preset's. An entry that
// is off is below every severity; an entry the plugin gives no severity is
// one of a tool that has none, which holds nothing to compare.
func severityAspect(b model.Binding, e plugin.Entry) aspect {
	if b.Severity == "" {
		return aspect{}
	}
	want := severityRank(b.Severity)
	if want < 0 {
		return aspect{reason: fmt.Sprintf("the preset's severity %q is no severity of the engine", b.Severity)}
	}
	var got int
	switch {
	case !e.On:
		got = 0
	case e.Severity == nil:
		return aspect{}
	default:
		got = severityRank(model.Severity(*e.Severity))
		if got < 0 {
			return aspect{reason: fmt.Sprintf("the severity %q is no severity of the engine", string(*e.Severity))}
		}
	}
	return aspect{rank: sign(got - want)}
}

// valueAspect is how the entry's value stands against the preset's: the same,
// off (weaker), or, where they differ, no word and the reason: which value is
// the tighter is the plugin's rank, not asked before release 2.
//
// The entry's value is compared as the plugin typed it, never its text read
// again: the text yes is text, not true. A value the config
// gives as null, or does not print, against a preset that sets one is its own
// case, with no word.
func valueAspect(b model.Binding, e plugin.Entry) aspect {
	_, null := e.Value.(canon.Null)
	if b.Value == nil {
		// A config that sets the value to null where the preset sets none
		// is its own case: the tool's default for a null is the tool's, so
		// it is no word.
		if null {
			return aspect{reason: "the entry's value is null, and the preset sets no value, so what the tool does with a null is not known"}
		}
		// A binding with no value is on: it holds the entry, whatever its
		// value, unless the entry reads as off.
		if !e.On {
			return aspect{rank: looser}
		}
		return aspect{}
	}
	if !e.On {
		return aspect{rank: looser}
	}
	if e.Value == nil || null {
		return aspect{reason: "the entry's value is null or not printed, and the preset sets " + stable(b.Value) + ", so what the tool does with it is not known"}
	}
	want, got := stable(b.Value), stable(plain(e.Value))
	if got == want {
		return aspect{}
	}
	return aspect{reason: "the value " + got + " differs from the preset's " + want + ", and which is the tighter is the plugin's rank, not asked yet"}
}

// optionsAspect is the same or no word: options that differ, or are set on one
// side only, are for the plugin's rank (release 2).
func optionsAspect(b model.Binding, e plugin.Entry) aspect {
	want, got := "", ""
	if len(b.Options) > 0 {
		want = stable(b.Options)
	}
	if o := plain(e.Options); !empty(o) {
		got = stable(o)
	}
	switch {
	case want == got:
		return aspect{}
	case want == "" || got == "":
		return aspect{reason: "the options are set on one side only, and what the other side's default allows is the tool's"}
	}
	return aspect{reason: "the options differ from the preset's, and which are the tighter is the plugin's rank, not asked yet"}
}

// empty is whether a plain value holds nothing: none, null, or a map or list
// with nothing in it.
func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

// plain is a canon value as the plain Go value stable reads: a null and a
// value not given are nil.
func plain(v canon.Value) any {
	switch x := v.(type) {
	case nil, canon.Null:
		return nil
	case canon.Bool:
		return bool(x)
	case canon.Int:
		return int64(x)
	case canon.Float:
		return float64(x)
	case canon.String:
		return string(x)
	case canon.Array:
		out := make([]any, len(x))
		for i, m := range x {
			out[i] = plain(m)
		}
		return out
	case canon.Object:
		out := make(map[string]any, len(x))
		for k, m := range x {
			out[k] = plain(m)
		}
		return out
	}
	return nil
}

func seenKey(out []model.Observation, key string) bool {
	return slices.ContainsFunc(out, func(o model.Observation) bool { return o.Key == key })
}

// provenance is where a preset entry came from.
func provenance(p model.PresetEntry) model.Provenance {
	return model.Provenance{Layer: p.Layer, Decision: p.Decision, ExcludedBy: p.Excluded}
}

// severityRank is the engine's order, with off below every severity.
func severityRank(s model.Severity) int {
	switch s {
	case model.SeverityError:
		return 3
	case model.SeverityWarning:
		return 2
	case model.SeverityInfo:
		return 1
	}
	return -1
}

func sign(n int) int {
	switch {
	case n < 0:
		return looser
	case n > 0:
		return tighter
	}
	return same
}

// stable is v as one text that equal values share: numbers by their value,
// object members in key order, text quoted. v is a value of the ordered
// package's parse, or a Go value of a binding.
func stable(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case string:
		return strconv.Quote(x)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		// A whole binary64 inside int64's range is that whole number, so 2
		// and 2.0 are one value, and a whole number is compared exactly,
		// never through binary64.
		if x >= -(1<<63) && x < 1<<63 && x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = stable(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *ordered.Map:
		return stable(ordered.Plain(x))
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = strconv.Quote(k) + ":" + stable(x[k])
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return fmt.Sprintf("%T:%v", v, v)
}
