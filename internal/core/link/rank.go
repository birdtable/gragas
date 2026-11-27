package link

import (
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// Rank is how the value found at the entry key stands against the preset
// value, in the words of the plugin that serves the tool, by its rank op:
// tighter, weaker, the same (two spellings of one value, off
// among them), or unknown. Every failure of the plugin is unknown here: the
// caller that holds the plugin keeps the error and ends on it. A nil Rank
// ranks nothing, so two values that differ as text are unknown.
type Rank func(key string, preset, found canon.Value) plugin.Word

// rankOf is how found stands against preset at key: the same for the same
// text, with no call; else r's word, and unknown with no r or for a value
// canon cannot hold. What cannot be ranked is never taken as the same.
func rankOf(r Rank, key string, preset, found any) plugin.Word {
	if stable(preset) == stable(found) {
		return plugin.WordSame
	}
	if r == nil {
		return plugin.WordUnknown
	}
	p, perr := toCanon(preset)
	f, ferr := toCanon(found)
	if perr != nil || ferr != nil {
		return plugin.WordUnknown
	}
	return r(key, p, f)
}

// valueNotLooser is whether the child's value at key is the parent's, or one
// the plugin ranks tighter; a value it cannot rank is not tighter.
func valueNotLooser(r Rank, key string, child, parent any) bool {
	w := rankOf(r, key, parent, child)
	return w == plugin.WordSame || w == plugin.WordTighter
}
