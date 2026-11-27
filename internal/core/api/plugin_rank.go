package coreapi

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// UnservedTools is a read ended by a value of a tool that no installed plugin
// serves: what the value is worth against another is the plugin's word alone
// (its rank op), and no adapter of the engine answers in its
// place. It matches ErrNotReady, and the ending names the install.
type UnservedTools struct {
	Tools []string
}

func (e *UnservedTools) Error() string {
	reads := ", so a value of it cannot be ranked"
	if len(e.Tools) > 1 {
		reads = ", so a value of theirs cannot be ranked"
	}
	return "no plugin is installed for " + strings.Join(e.Tools, ", ") + reads
}

// Unwrap is ErrNotReady.
func (e *UnservedTools) Unwrap() error { return ErrNotReady }

// PluginNext is the line to type after a read ended by the plugins: the
// install of a tool no plugin serves, or the approval of one Open refuses;
// false for any other error.
func PluginNext(err error) (string, bool) {
	if _, ok := errors.AsType[*UnservedTools](err); ok {
		return "gragas plugin add <folder>", true
	}
	if u, ok := errors.AsType[*UnapprovedPlugin](err); ok {
		return "gragas plugin approve " + u.Ref.Name + "@" + u.Ref.Version, true
	}
	return "", false
}

// rankers are the rank ops of the plugins installed under root, by the tool
// each serves. Nothing is opened until a value is ranked: a read whose values
// are all the same as text calls no plugin. The first rank opens every
// installed plugin and keeps one of each tool: the one whose range serves the
// version the tested set names, else the first in the order List gives. Two
// that both serve the tested version are an error, for which one ranks is not
// the engine's to pick. The rest are closed; close closes those kept. A plugin that is refused, fails to
// open or fails a rank is the error the read ends on, the first one kept: a
// rank it did not give is unknown, never the same (fail closed).
type rankers struct {
	plugins port.Plugins
	root    model.Root
	// tested is the tested set of the book being read, whose version of a
	// tool the plugin is asked to rank at.
	tested *model.Tested
	// handWritten are the tools the book's identity lists as hand-written:
	// no plugin is asked to rank a value of one, and none is wanted, so a
	// pair of one ranks unknown and never ends the read not ready.
	handWritten []string

	opened   bool
	by       map[string]port.Plugin
	unserved []string
	err      error
}

func newRankers(plugins port.Plugins, root model.Root) *rankers {
	return &rankers{plugins: plugins, root: root}
}

// reading is r for the book of id: its tested set and its hand-written tools.
func (r *rankers) reading(id model.RulebookIdentity) *rankers {
	r.tested, r.handWritten = id.Tested, id.HandWrittenPresets
	return r
}

// of is the Rank of each tool's plugin, over ctx. A tool no plugin serves
// ranks every pair unknown and is kept, to end the read not ready.
func (r *rankers) of(ctx context.Context) func(tool string) link.Rank {
	return func(tool string) link.Rank {
		return func(key string, preset, found canon.Value) plugin.Word {
			return r.rank(ctx, tool, key, preset, found)
		}
	}
}

func (r *rankers) rank(ctx context.Context, tool, key string, preset, found canon.Value) plugin.Word {
	if slices.Contains(r.handWritten, tool) {
		return plugin.WordUnknown
	}
	r.open(ctx)
	if r.err != nil {
		return plugin.WordUnknown
	}
	pl, ok := r.by[tool]
	if !ok {
		if !slices.Contains(r.unserved, tool) {
			r.unserved = append(r.unserved, tool)
		}
		return plugin.WordUnknown
	}
	m := pl.Manifest()
	version, err := r.versionOf(tool, m.ToolVersions)
	if err != nil {
		r.err = fmt.Errorf("the plugin %s %s serves no version of %s to rank at: %w", m.Name, m.Version, tool, err)
		return plugin.WordUnknown
	}
	resp, err := pl.Rank(ctx, plugin.RankRequest{ToolVersion: version,
		Pairs: []plugin.Pair{{Key: key, Aspect: plugin.AspectValue, Preset: preset, Found: found}}})
	if err == nil && len(resp.Words) != 1 {
		err = fmt.Errorf("it gave %d words for one pair", len(resp.Words))
	}
	if err != nil {
		r.err = fmt.Errorf("the plugin %s %s could not rank %s at %s: %w", m.Name, m.Version, tool, key, err)
		return plugin.WordUnknown
	}
	return resp.Words[0]
}

// versionOf is the version of tool the plugin ranks at: the tested set's,
// where it names one the plugin's range serves, else the first version the
// range serves at its ends.
func (r *rankers) versionOf(tool, spec string) (string, error) {
	vr, err := link.ParseRange(spec)
	if err != nil {
		return "", err
	}
	if r.tested != nil {
		for _, v := range r.tested.Tools {
			if v.Name != tool {
				continue
			}
			if in, err := link.InRange(vr, v.Version); err == nil && in {
				return v.Version, nil
			}
		}
	}
	served, _, err := rangeEnds(spec)
	if err != nil {
		return "", err
	}
	if len(served) == 0 {
		return "", fmt.Errorf("the range %s serves no version", spec)
	}
	return served[0], nil
}

// open opens the installed plugins once, keeping the one of each tool that
// chosenPlugins chooses.
func (r *rankers) open(ctx context.Context) {
	if r.opened {
		return
	}
	r.opened = true
	r.by, r.err = chosenPlugins(ctx, r.plugins, r.root, r.tested)
	if r.by == nil {
		r.by = map[string]port.Plugin{}
	}
}

// close closes every plugin kept open, and is the error the read ends on: the
// first a plugin gave, else a tool no plugin serves, else one of the closes.
func (r *rankers) close() error {
	var closed error
	for _, tool := range slices.Sorted(maps.Keys(r.by)) {
		pl := r.by[tool]
		if err := pl.Close(); err != nil && closed == nil {
			m := pl.Manifest()
			closed = fmt.Errorf("the plugin %s %s could not be closed: %w", m.Name, m.Version, err)
		}
	}
	r.by = nil
	switch {
	case r.err != nil:
		return r.err
	case len(r.unserved) > 0:
		return &UnservedTools{Tools: slices.Sorted(slices.Values(r.unserved))}
	}
	return closed
}
