package link

import (
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// Coverage is a rulebook's coverage both ways, counted at the upstream unit
// (S47): an upstream preset adopted whole at rung 0 is one covered set under
// its rung-0 rule, expandable, and an entry the upstream config holds that no
// binding or adopted set explains is unmapped. held is Derive's rows and up is
// the flattened set of each tool by name; a tool with no entry has none, which
// counts zero unmapped. Rules held counts effective bindings only, so a new
// binding never lowers it.
func Coverage(book model.Book, held []model.Held, up map[string]model.Upstream) model.Coverage {
	var out model.Coverage
	standing := map[model.RuleID]model.Held{}
	for _, h := range held {
		standing[h.Rule] = h
	}
	tl := tally{up: up, units: map[string]*model.CoverEntry{}, adopted: map[string]map[string]bool{}, bound: map[string]map[string]bool{}}
	for _, r := range book.Rules {
		h, ok := standing[r.ID]
		if !ok || h.Standing == model.StandingProposed || h.Standing == model.StandingRetired {
			continue
		}
		row := model.CoverRule{Rule: r.ID, Standing: h.Standing, Held: h.Standing == model.StandingEnforced || h.Standing == model.StandingReviewed, Entries: []string{}, ZeroBy: r.ZeroBy}
		for _, b := range r.Bindings {
			row.Entries = append(row.Entries, b.Tool+" "+string(b.Target)+": "+b.Key)
			tl.add(r.ID, b)
		}
		out.Rules = append(out.Rules, row)
	}
	slices.Sort(tl.order)
	for _, k := range tl.order {
		slices.Sort(tl.units[k].Rules)
		out.Entries = append(out.Entries, *tl.units[k])
	}
	out.Unmapped = unmapped(up, tl.adopted, tl.bound)
	return out
}

// tally is the entries the bindings name so far: each as one unit with the
// rules that hold it, the entries each adopted set explains, and the keys
// bound, by tool.
type tally struct {
	up      map[string]model.Upstream
	units   map[string]*model.CoverEntry
	order   []string
	adopted map[string]map[string]bool
	bound   map[string]map[string]bool
}

// add counts binding b of rule id: its entry is one unit, and a rung-0 binding
// of a preset adopts the whole set as that unit.
func (tl *tally) add(id model.RuleID, b model.Binding) {
	entry := string(b.Target) + ": " + b.Key
	k := b.Tool + "\x00" + entry
	u, seen := tl.units[k]
	if !seen {
		u = &model.CoverEntry{Tool: b.Tool, Entry: entry, Rules: []model.RuleID{}}
		tl.units[k], tl.order = u, append(tl.order, k)
	}
	if !slices.Contains(u.Rules, id) {
		u.Rules = append(u.Rules, id)
	}
	mark(tl.bound, b.Tool, b.Key)
	if set, ok := adoptedSet(tl.up[b.Tool], b); ok {
		u.Set, u.Members = set.Preset, slices.Clone(set.Entries)
		for _, e := range set.Entries {
			mark(tl.adopted, b.Tool, e)
		}
	}
}

// mark records entry under tool in m.
func mark(m map[string]map[string]bool, tool, entry string) {
	if m[tool] == nil {
		m[tool] = map[string]bool{}
	}
	m[tool][entry] = true
}

// adoptedSet is the set of up that b adopts whole: a rung-0 binding whose key
// is the preset's name.
func adoptedSet(up model.Upstream, b model.Binding) (model.UpstreamSet, bool) {
	if b.Rung != 0 {
		return model.UpstreamSet{}, false
	}
	i := slices.IndexFunc(up.Sets, func(s model.UpstreamSet) bool { return s.Preset == b.Key })
	if i < 0 {
		return model.UpstreamSet{}, false
	}
	return up.Sets[i], true
}

// unmapped is each observed entry no adopted set lists and no binding names,
// by tool then entry.
func unmapped(up map[string]model.Upstream, adopted, bound map[string]map[string]bool) []model.CoverUnmapped {
	tools := make([]string, 0, len(up))
	for t := range up {
		tools = append(tools, t)
	}
	slices.Sort(tools)
	out := []model.CoverUnmapped{}
	for _, t := range tools {
		obs := slices.Clone(up[t].Observed)
		slices.Sort(obs)
		for _, e := range obs {
			if !adopted[t][e] && !bound[t][e] {
				out = append(out, model.CoverUnmapped{Tool: t, Entry: e})
			}
		}
	}
	return out
}
