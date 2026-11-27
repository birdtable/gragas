package tool

// The find of a program a plugin's step names: by the grant's id,
// in the order settings, the grant's own place, the tools file, never from
// what the project says the tool is; and the version read by running it,
// every time, held to the key it was chosen by or to the grant's range.

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// stepGrant is the step of a grant's own place: its path, or its script's
// manifest, below the project. It is not a step of the locate order.
const stepGrant Step = "grant"

// grant is one program a plugin's manifest grants, read for its find.
type grant struct {
	id string
	// versions is the range as the manifest writes it, and rng it parsed.
	versions string
	rng      model.VersionRange
	// argv is what the program is asked its version by, and pattern the
	// version_pattern that reads it, nil for the first major.minor.patch.
	argv    []string
	pattern *regexp.Regexp
	// at is a ProgramAt's path below the project, "" for none.
	at string
	// script is a ProgramScript, nil for none.
	script *plugin.ProgramScript
	// cache is the grant's cache_vars, nil for none.
	cache []string
}

// grantOf is the grant of m whose id is id. A program m does not grant
// matches port.ErrGrant, before anything is looked at (item 1). A grant whose
// versions or version_pattern does not read is refused: the opener judges
// both, and this is the host's own hold of what it runs.
func grantOf(m plugin.Manifest, id string) (grant, error) {
	for _, p := range m.Programs {
		g, pattern := programOf(p)
		if g.id != id {
			continue
		}
		g.cache = p.CacheVars()
		rng, err := model.ParseRange(g.versions)
		if err != nil {
			return grant{}, refused("the grant "+hosttrust.Quote(id)+" serves the versions "+hosttrust.Quote(g.versions)+", which is no range: "+err.Error(), "write the grant's versions as one comparator, or a lower and an upper bound")
		}
		g.rng = rng
		if pattern != nil {
			re, err := regexp.Compile(*pattern)
			if err != nil || re.NumSubexp() != 1 {
				return grant{}, refused("the grant "+hosttrust.Quote(id)+" reads its version by "+hosttrust.Quote(*pattern)+", which is no RE2 pattern of one group", "write a version_pattern whose one group holds the version")
			}
			g.pattern = re
		}
		return g, nil
	}
	return grant{}, fmt.Errorf("the step names the program %s: %w", hosttrust.Quote(id), port.ErrGrant)
}

// programOf is the fields of a grant's one form, and its version_pattern.
func programOf(p plugin.Program) (grant, *string) {
	switch {
	case p.Found != nil:
		f := p.Found
		return grant{id: f.ID, versions: f.Versions, argv: f.VersionArgv}, f.VersionPattern
	case p.At != nil:
		a := p.At
		return grant{id: a.ID, versions: a.Versions, argv: a.VersionArgv, at: a.Path}, a.VersionPattern
	case p.Script != nil:
		s := p.Script
		return grant{id: s.ID, versions: s.Versions, argv: s.VersionArgv, script: s}, s.VersionPattern
	}
	return grant{}, nil
}

// locateGrant is the program of g, found at root as the locate order finds
// one (reach), checked and asked its version by the grant's own argv. A place
// that holds nothing, and one a pinned find passes over, give way to the
// next; the first that holds a program is the find, and its version is held:
// to the tools file's key it was chosen by, or to the grant's range (holds).
// One that is not what it says, or outside the range, ends the find and
// never gives way to the next place. With no place holding the program, a
// tools file that pins the id at no version inside the range makes the step
// not ready, naming the range and the versions pinned; with none pinned at
// all, the find is a NotFound. note journals the version ask of the program
// found before it is started; when it fails, nothing is started and its
// error ends the find.
func (t *Tool) locateGrant(ctx context.Context, root model.Root, g grant, pinned bool, note func(context.Context, model.Found) error) (model.Found, error) {
	if fault := pathFault(string(root)); fault != "" {
		return model.Found{}, refused("the root "+hosttrust.Quote(string(root))+" "+fault, "open the project through the Tree port")
	}
	places, nf, late, err := t.grantPlaces(root, g, pinned)
	if err != nil {
		return model.Found{}, err
	}
	in := t.within(root, pinned)
	if in.empty() {
		return model.Found{}, &NotFound{Tool: g.id, Version: g.versions, Reason: emptyPinnedPath(t.cfg.Path) + ", so nothing was started; " + emptyPinnedPathFix}
	}
	for _, p := range places {
		f, ok, err := t.reach(root, p, in, pinned, nf)
		if err != nil {
			return model.Found{}, err
		}
		if !ok {
			continue
		}
		if err := note(ctx, f); err != nil {
			return model.Found{}, fmt.Errorf("the version ask of %s at %s is not journalled, so it is not started: %w", g.id, hosttrust.Quote(f.Path), err)
		}
		got, err := t.versionBy(ctx, root, f, versionAsk{name: g.id, want: cmp.Or(p.want, g.versions), argv: g.argv, pattern: g.pattern, env: in.path})
		if err != nil {
			return model.Found{}, err
		}
		if err := g.holds(p, f.Path, got); err != nil {
			return model.Found{}, err
		}
		f.Version, f.Step = got, string(p.step)
		return f, nil
	}
	if late != nil {
		return model.Found{}, late
	}
	return model.Found{}, nf
}

// grantPlaces is every place of g's find at root, in order (item 2): the one
// path the settings give the id for root, of any version, since a settings
// find chooses no key; the grant's own place below the project, its path or
// its script's manifest, which a pinned find never takes and names in the
// NotFound; and the tools file's newest key for the id inside the range
// (pinnedKey). A grant with neither a path nor a script is found in the
// settings and the tools file alone. late is the not-ready a find that
// reaches no program ends in when the tools file pins the id outside the
// range alone.
func (t *Tool) grantPlaces(root model.Root, g grant, pinned bool) (out []place, nf *NotFound, late, err error) {
	nf = &NotFound{Tool: g.id, Version: g.versions}
	var named []string
	for k := range t.cfg.Paths[string(root)] {
		if id, _, ok := strings.Cut(k, "@"); ok && id == g.id {
			named = append(named, k)
		}
	}
	slices.Sort(named)
	switch len(named) {
	case 0:
	case 1:
		out = append(out, place{step: StepSettings, path: t.cfg.Paths[string(root)][named[0]]})
	default:
		return nil, nil, nil, refused("the settings name "+g.id+" at "+strings.Join(named, " and ")+" for "+string(root)+
			": a project runs one version of a tool", "keep one of them under tool-paths in the settings file")
	}
	own, ok, err := t.grantPlace(root, g)
	if err != nil {
		return nil, nil, nil, err
	}
	if ok && pinned {
		nf.Tried = append(nf.Tried, string(stepGrant)+" "+own.path+" (not tried: a pinned find takes no place below the project)")
	} else if ok {
		out = append(out, own)
	}
	newest, all := pinnedKey(t.cfg.Machine, g)
	switch {
	case newest != "":
		out = append(out, place{step: StepToolsFile, path: t.cfg.Machine[g.id+"@"+newest], want: newest})
	case len(all) > 0:
		late = notReady("the tools file pins "+g.id+" at "+strings.Join(all, ", ")+", and none is inside the grant's versions "+hosttrust.Quote(g.versions),
			"pin "+g.id+" at a version inside "+g.versions+" in the tools file")
	}
	return out, nf, late, nil
}

// grantPlace is g's own place below root: its path, with the system's suffix
// for a program's file added (FileName), or its script's manifest. A path
// that is no relative slash path, or that climbs out of the project, is
// refused before anything is looked at; the manifest is manifestScript's to
// judge.
func (t *Tool) grantPlace(root model.Root, g grant) (place, bool, error) {
	switch {
	case g.at != "":
		at := path.Join(string(root), path.Dir(g.at), t.cfg.FileName(path.Base(g.at)))
		if relFault(g.at) || !below(string(root), at) {
			return place{}, false, refused("the grant "+hosttrust.Quote(g.id)+" names the path "+hosttrust.Quote(g.at)+", which is empty, not a relative slash path or outside the project",
				"name the program by a path below the project")
		}
		return place{step: stepGrant, path: at}, true, nil
	case g.script != nil:
		return place{step: stepGrant, path: path.Join(string(root), g.script.Manifest), script: g.script}, true, nil
	}
	return place{}, false, nil
}

// pinnedKey is the newest version the tools file pins for g's id that is
// inside g's range, compared in full (1.10.0 after 1.9.0), "" for none; and
// every version it pins for the id, in order, for a refusal. A key outside
// the range is never taken, even when it is the only one; nor is a key whose
// version is no major.minor.patch.
func pinnedKey(machine map[string]string, g grant) (newest string, all []string) {
	for k := range machine {
		id, v, ok := strings.Cut(k, "@")
		if !ok || id != g.id {
			continue
		}
		all = append(all, v)
		if in, err := model.InRange(g.rng, v); err != nil || !in {
			continue
		}
		if newest == "" || model.CompareVersions(v, newest) > 0 {
			newest = v
		}
	}
	slices.SortFunc(all, func(a, b string) int {
		return cmp.Or(model.CompareVersions(a, b), strings.Compare(a, b))
	})
	return newest, all
}

// holds judges the version got that the program at target printed against
// what the place it was found at says:
// a pinned program that prints another version than the tools file's key it
// was chosen by is not what it says it is, a trust failure, and refused; a
// program of the settings or the grant's own place outside the grant's
// range is a version this machine lacks, and not ready.
func (g grant) holds(p place, target, got string) error {
	if p.want != "" {
		if got != p.want {
			return refused("the tools file pins "+g.id+" "+p.want+" at "+hosttrust.Quote(target)+", and it prints "+got+": a pinned program that is not what it says it is",
				"put "+g.id+" "+p.want+" where the tools file points, or pin the version that is there")
		}
		return nil
	}
	if in, err := model.InRange(g.rng, got); err != nil || !in {
		return notReady(g.id+" at "+hosttrust.Quote(target)+" ("+string(p.step)+") is "+got+", which is outside the grant's versions "+hosttrust.Quote(g.versions),
			"point "+string(p.step)+" at "+g.id+" at a version inside "+g.versions)
	}
	return nil
}
