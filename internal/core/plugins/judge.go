package plugins

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// The three forms of a grant, by the keys it holds.
const (
	// FormPinned is a program found in the settings and the machine tools
	// file alone.
	FormPinned = "pinned"
	// FormPath is a program also looked for at one place below the project.
	FormPath = "path"
	// FormScript is an interpreted script a package manifest names.
	FormScript = "script"
)

// The folders no path of a manifest may pass through, each compared by
// model.SameName, so by every spelling a file system opens it by.
const (
	gitFolder = ".git"
	binFolder = ".bin"
)

// ManifestError is the refusal of a manifest.
type ManifestError struct {
	// Path is the JSON pointer of the field refused.
	Path string
	// Form is the form the keys of the grant refused fit, or empty when the
	// refusal is of no one grant.
	Form string
	// Why says what is refused, with every word of the manifest's in it
	// written as plugin.Shown writes it.
	Why string
}

// Error names the field and, for a grant, its form in the first line.
func (e *ManifestError) Error() string {
	if e.Form == "" {
		return fmt.Sprintf("the manifest is refused at %s: %s", e.Path, e.Why)
	}
	return fmt.Sprintf("the manifest is refused at %s, in a grant of the %s form: %s", e.Path, e.Form, e.Why)
}

// Judge refuses what the manifest schema cannot hold, with a *ManifestError,
// and answers nil for a manifest it has nothing against. It is pure: it reads
// no file and asks no port, so the opener and the approval screen ask it
// before anything of the bundle is read further, and both get one answer
// (port.ManifestJudge is its type).
//
// It refuses, the first it meets in the manifest's own order:
//
//   - a grant whose id an earlier grant has: a screen that shows the first
//     and a run that takes the last is the hole;
//   - a path that passes through a .git folder, in a grant's path or
//     manifest, the preset's path or inputs, or resolve_from: a repository's
//     own folder holds its hooks, and its config can hold a token;
//   - a script grant whose manifest lies under a .bin folder:
//     what stands there is a shim or a link, never what runs;
//   - a version_pattern that RE2 does not compile, or that has not exactly
//     one group;
//   - the grant whose id is the tool, when its versions are not the
//     manifest's tool_versions: the two would judge one program by two
//     ranges;
//   - a grant's cache_vars outside the schema's bounds: empty,
//     past 8 names, a name outside its pattern or length, a name twice;
//   - a grant that gives cache_vars when needs does not name cache-vars;
//   - a manifest with no grant whose id is the tool;
//   - cache-vars in needs when no grant gives cache_vars;
//   - a counted.exclusions path, or a counted.loosening at, given a second
//     time: the schema's uniqueItems compares whole entries, so
//     it cannot hold one path to one entry, and a path given twice would count
//     its rows twice.
//
// The folder names are compared by model.SameName, so .GIT and ".git." are
// .git on every system. A grant that holds none of the three forms, or more
// than one, is a value no schema read, and is refused where it stands.
func Judge(m plugin.Manifest) error {
	first := map[string]int{}
	tool := false
	for i, p := range m.Programs {
		g, err := grantOf(i, p)
		if err != nil {
			return err
		}
		if j, twice := first[g.id]; twice {
			return g.refuse("id", fmt.Sprintf("the id %s is also grant %d's, and a program is granted once", plugin.Shown(g.id), j))
		}
		first[g.id] = i
		if err := g.judge(); err != nil {
			return err
		}
		if g.id != m.Tool {
			continue
		}
		tool = true
		if g.versions != m.ToolVersions {
			return g.refuse("versions", fmt.Sprintf("the tool's own grant serves %s and tool_versions says %s; the two are one range", plugin.Shown(g.versions), plugin.Shown(m.ToolVersions)))
		}
	}
	if err := judgeCacheFeature(m); err != nil {
		return err
	}
	if !tool {
		return &ManifestError{Path: "/tool", Why: fmt.Sprintf("no grant's id is %s, the tool, so tool_versions would judge no program", plugin.Shown(m.Tool))}
	}
	if err := judgeCounted(m.Counted); err != nil {
		return err
	}
	if m.Preset != nil {
		if err := judgeReadback(m.Preset.Readback); err != nil {
			return err
		}
	}
	return judgePaths(m)
}

// maxDefaults is the bound the schema states on preset.readback.defaults.
const maxDefaults = 64

// judgeReadback holds preset.readback to its limits: the schema
// cannot say that the keys are in byte order, and a value built in Go and not
// read from a document has met no schema, so it also refuses what the schema
// refuses. It refuses, naming the member: a readback with no member; an empty
// defaults list (leave the key out); a list past its bound; a key that is
// empty, past 128 bytes, or holding a control, format, line-separator or
// paragraph-separator character; a key given twice or out of byte order.
func judgeReadback(r *plugin.PresetReadback) error {
	if r == nil {
		return nil
	}
	const at = "/preset/readback"
	if r.Severity == nil && r.Defaults == nil {
		return &ManifestError{Path: at, Why: "holds no member: leave readback out, or say severity or defaults"}
	}
	if r.Defaults == nil {
		return nil
	}
	if len(r.Defaults) == 0 {
		return &ManifestError{Path: at + "/defaults", Why: "is empty: leave defaults out"}
	}
	if len(r.Defaults) > maxDefaults {
		return &ManifestError{Path: at + "/defaults", Why: fmt.Sprintf("holds %d keys, and the bound is %d", len(r.Defaults), maxDefaults)}
	}
	for i, k := range r.Defaults {
		path := fmt.Sprintf("%s/defaults/%d", at, i)
		if err := judgeDefaultKey(path, k); err != nil {
			return err
		}
		if i == 0 {
			continue
		}
		switch prev := r.Defaults[i-1]; {
		case k == prev:
			return &ManifestError{Path: path, Why: fmt.Sprintf("the key %s is also defaults %d's, and a key is named once", plugin.Shown(k), i-1)}
		case k < prev:
			return &ManifestError{Path: path, Why: fmt.Sprintf("the key %s comes after %s: defaults are in byte order", plugin.Shown(k), plugin.Shown(prev))}
		}
	}
	return nil
}

// maxDefaultKey is the bound the schema states on one key of
// preset.readback.defaults, in UTF-8 bytes as the contract counts every cap,
// and as the generated decoder holds it.
const maxDefaultKey = 128

// judgeDefaultKey refuses a defaults key at path that is empty, longer than
// maxDefaultKey bytes, or holds a character a screen does not show as
// written (the length is the schema's, held here for a value built in Go).
func judgeDefaultKey(path, k string) error {
	if k == "" {
		return &ManifestError{Path: path, Why: "is empty: a key names a linter or a rule"}
	}
	if len(k) > maxDefaultKey {
		return &ManifestError{Path: path, Why: fmt.Sprintf("is %d bytes, and the bound is %d", len(k), maxDefaultKey)}
	}
	for _, c := range k {
		if model.Unshowable(c) {
			return &ManifestError{Path: path, Why: fmt.Sprintf("the key %s holds a character a screen does not show as written", plugin.Shown(k))}
		}
	}
	return nil
}

// judgeCounted refuses a key path counted.exclusions gives twice, and an at
// counted.loosening gives twice, naming the second and the first.
func judgeCounted(c *plugin.Counted) error {
	if c == nil {
		return nil
	}
	seen := map[string]int{}
	for i, p := range c.Exclusions {
		key := strings.Join(p, "\x00")
		if j, twice := seen[key]; twice {
			return &ManifestError{Path: fmt.Sprintf("/counted/exclusions/%d", i), Why: fmt.Sprintf("the path %s is also exclusions %d's, and a path is counted once", plugin.Shown(strings.Join(p, ".")), j)}
		}
		seen[key] = i
	}
	clear(seen)
	for i, l := range c.Loosening {
		var at []string
		switch {
		case l.Values != nil:
			at = l.Values.At
		case l.Unless != nil:
			at = l.Unless.At
		}
		key := strings.Join(at, "\x00")
		if j, twice := seen[key]; twice {
			return &ManifestError{Path: fmt.Sprintf("/counted/loosening/%d/at", i), Why: fmt.Sprintf("the at %s is also loosening %d's, and a value is counted once", plugin.Shown(strings.Join(at, ".")), j)}
		}
		seen[key] = i
	}
	return nil
}

// judgePaths refuses a .git folder in the paths the manifest holds outside
// its grants.
func judgePaths(m plugin.Manifest) error {
	if m.Preset != nil {
		if step, ok := passes(m.Preset.Path, gitFolder); ok {
			return &ManifestError{Path: "/preset/path", Why: gitWhy(m.Preset.Path, step)}
		}
		for i, in := range m.Preset.Inputs {
			if step, ok := passes(in, gitFolder); ok {
				return &ManifestError{Path: fmt.Sprintf("/preset/inputs/%d", i), Why: gitWhy(in, step)}
			}
		}
	}
	if m.ResolveFrom == nil {
		return nil
	}
	if step, ok := passes(*m.ResolveFrom, gitFolder); ok {
		return &ManifestError{Path: "/resolve_from", Why: gitWhy(*m.ResolveFrom, step)}
	}
	return nil
}

// grant is one grant of whichever form, with what Judge reads of it.
type grant struct {
	// at is the JSON pointer of the grant, and form the form it holds.
	at, form     string
	id, versions string
	pattern      *string
	// path is the path form's place, and manifest the script form's package
	// manifest; each is empty in a form that has none.
	path, manifest string
	// cacheVars is the grant's cache_vars, nil for none.
	cacheVars []string
}

// grantOf is the grant p holds, or the refusal of one that holds no form or
// more than one.
func grantOf(i int, p plugin.Program) (grant, error) {
	g := grant{at: fmt.Sprintf("/programs/%d", i)}
	n := 0
	if f := p.Found; f != nil {
		g.form, g.id, g.versions, g.pattern = FormPinned, f.ID, f.Versions, f.VersionPattern
		n++
	}
	if a := p.At; a != nil {
		g.form, g.id, g.versions, g.pattern, g.path = FormPath, a.ID, a.Versions, a.VersionPattern, a.Path
		n++
	}
	if s := p.Script; s != nil {
		g.form, g.id, g.versions, g.pattern, g.manifest = FormScript, s.ID, s.Versions, s.VersionPattern, s.Manifest
		n++
	}
	if n != 1 {
		return grant{}, &ManifestError{Path: g.at, Why: fmt.Sprintf("the grant holds %d of the three forms, and a grant is one", n)}
	}
	g.cacheVars = p.CacheVars()
	return g, nil
}

func (g grant) refuse(field, why string) error {
	return &ManifestError{Path: g.at + "/" + field, Form: g.form, Why: why}
}

// judge refuses what one grant holds on its own: a reserved folder in its
// paths, and a pattern that gives no one version.
func (g grant) judge() error {
	for _, p := range []struct{ field, path string }{{"path", g.path}, {"manifest", g.manifest}} {
		if step, ok := passes(p.path, gitFolder); ok {
			return g.refuse(p.field, gitWhy(p.path, step))
		}
	}
	if step, ok := passes(g.manifest, binFolder); ok {
		return g.refuse("manifest", fmt.Sprintf("%s passes through %s, a .bin folder, which holds shims and links and never the script that runs", plugin.Shown(g.manifest), plugin.Shown(step)))
	}
	if err := g.judgeCacheVars(); err != nil {
		return err
	}
	if g.pattern == nil {
		return nil
	}
	re, err := regexp.Compile(*g.pattern)
	if err != nil {
		return g.refuse("version_pattern", "it is not a pattern of RE2: "+plugin.Shown(err.Error()))
	}
	if n := re.NumSubexp(); n != 1 {
		return g.refuse("version_pattern", fmt.Sprintf("%s has %d groups, and the version is what its one group holds", plugin.Shown(*g.pattern), n))
	}
	return nil
}

// The bounds the schema states on cache_vars, held here for a
// value built in Go, which met no schema.
const (
	maxCacheVars    = 8
	minCacheVarName = 5
	maxCacheVarName = 64
)

// cacheVarName is the schema's pattern of one cache_vars name: capitals,
// digits and _, starting with a capital and holding CACHE.
var cacheVarName = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)?CACHE[A-Z0-9_]*$`)

// judgeCacheFeature holds cache_vars and the feature cache-vars to each other: a grant that gives cache_vars when needs does not name the
// feature is refused at its cache_vars, so an engine without the feature
// refuses the plugin and never passes the names over; the feature in needs
// when no grant gives cache_vars is refused at needs, since a feature is
// needed only by a manifest that uses it. Each grant was judged before.
func judgeCacheFeature(m plugin.Manifest) error {
	need := slices.Index(m.Needs, plugin.FeatureCacheVars)
	held := false
	for i, p := range m.Programs {
		if p.CacheVars() == nil {
			continue
		}
		if need < 0 {
			g, err := grantOf(i, p)
			if err != nil {
				return err
			}
			return g.refuse("cache_vars", fmt.Sprintf("the grant gives cache_vars and needs does not name %s: a manifest that holds cache_vars needs the feature, so an engine without it refuses the plugin and does not pass the names over", plugin.Shown(plugin.FeatureCacheVars)))
		}
		held = true
	}
	if need >= 0 && !held {
		return &ManifestError{Path: fmt.Sprintf("/needs/%d", need), Why: fmt.Sprintf("needs names %s and no grant gives cache_vars: a feature is needed only by a manifest that uses it", plugin.Shown(plugin.FeatureCacheVars))}
	}
	return nil
}

// judgeCacheVars holds a grant's cache_vars to the schema's bounds: a list
// that is empty (leave the key out) or past its bound, a name outside the
// pattern or its length, and a name given twice are refused. Which names the
// engine sets itself is not this list's to say: the opener holds those, from the one list the step runner sets them by.
func (g grant) judgeCacheVars() error {
	if g.cacheVars == nil {
		return nil
	}
	switch n := len(g.cacheVars); {
	case n == 0:
		return g.refuse("cache_vars", "is empty: leave cache_vars out")
	case n > maxCacheVars:
		return g.refuse("cache_vars", fmt.Sprintf("holds %d names, and the bound is %d", n, maxCacheVars))
	}
	seen := map[string]int{}
	for i, name := range g.cacheVars {
		field := fmt.Sprintf("cache_vars/%d", i)
		if len(name) < minCacheVarName || len(name) > maxCacheVarName || !cacheVarName.MatchString(name) {
			return g.refuse(field, fmt.Sprintf("%s is no cache variable's name: %d to %d capitals, digits and _, starting with a capital and holding CACHE", plugin.Shown(name), minCacheVarName, maxCacheVarName))
		}
		if j, twice := seen[name]; twice {
			return g.refuse(field, fmt.Sprintf("the name %s is also cache_vars %d's, and a name is given once", plugin.Shown(name), j))
		}
		seen[name] = i
	}
	return nil
}

// passes says whether a slash path has a step that names folder, and which.
// The last step counts too: a path that ends at the folder names it.
func passes(path, folder string) (string, bool) {
	for step := range strings.SplitSeq(path, "/") {
		if model.SameName(step, folder) {
			return step, true
		}
	}
	return "", false
}

func gitWhy(path, step string) string {
	return fmt.Sprintf("%s passes through %s, a .git folder: a repository's own folder is never read, written or run from", plugin.Shown(path), plugin.Shown(step))
}
