package plugins_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// good is a manifest Judge has nothing against, with a grant of each form:
// the tool as a script, its interpreter pinned, and a second program at a
// path.
func good() plugin.Manifest {
	return plugin.Manifest{
		Needs:        []string{},
		Name:         "lintel",
		Version:      "1.0.0",
		Tool:         "lintel",
		ToolVersions: ">=9.0.0 <10.0.0",
		Targets:      []plugin.Target{plugin.TargetSource},
		Provides:     []plugin.Op{plugin.OpPlan, plugin.OpEntries},
		Preset:       &plugin.ManifestPreset{Path: "lintel.config.mjs", Inputs: []string{"package.json", ".github/lintel-base.mjs"}, Mark: true},
		ResolveFrom:  new("deps"),
		Programs: []plugin.Program{
			{Script: &plugin.ProgramScript{
				ID: "lintel", Versions: ">=9.0.0 <10.0.0", VersionArgv: []string{"--version"}, VersionPattern: new(`v([0-9]+\.[0-9]+\.[0-9]+)`),
				Manifest: "deps/lintel/package.json", ScriptAt: "/main", Interpreter: "runtime",
			}},
			{Found: &plugin.ProgramFound{ID: "runtime", Versions: ">=20.0.0", VersionArgv: []string{"--version"}}},
			{At: &plugin.ProgramAt{ID: "tidy", Versions: ">=3.0.0", VersionArgv: []string{"--version"}, Path: "deps/tidy/bin/tidy.cjs"}},
		},
		Wasm: "sha256:" + strings.Repeat("0", 64),
	}
}

func refusal(t *testing.T, m plugin.Manifest) *plugins.ManifestError {
	t.Helper()
	err := plugins.Judge(m)
	var me *plugins.ManifestError
	if !errors.As(err, &me) {
		t.Fatalf("Judge answered %v, and no refusal", err)
	}
	return me
}

// The good manifest is one the schema takes too: what the generated types
// write of it, they read back. So each planted row below is a manifest the
// schema lets through with one thing wrong, unless the row says otherwise.
func TestTheGoodManifestIsOneTheSchemaTakes(t *testing.T) {
	t.Parallel()
	var back plugin.Manifest
	if err := back.FromCanon(good().ToCanon()); err != nil {
		t.Fatalf("the generated types refuse the good manifest: %v", err)
	}
}

// TestJudgeHasNothingAgainstACountedOfDistinctPaths: paths and
// ats each given once, one a prefix of another, are nothing to Judge.
func TestJudgeHasNothingAgainstACountedOfDistinctPaths(t *testing.T) {
	t.Parallel()
	m := good()
	m.Counted = &plugin.Counted{
		Exclusions: [][]string{{"a", "b"}, {"a"}, {"a", "b", "c"}},
		Loosening: []plugin.Loosening{
			{Values: &plugin.LooseningValues{At: []string{"run", "tests"}, Values: []string{"false"}}},
			{Unless: &plugin.LooseningUnless{At: []string{"run"}, Unless: ""}},
		},
	}
	if err := plugins.Judge(m); err != nil {
		t.Errorf("Judge = %v, want nil", err)
	}
}

func TestJudgeHasNothingAgainstAGoodManifest(t *testing.T) {
	t.Parallel()
	if err := plugins.Judge(good()); err != nil {
		t.Fatalf("the good manifest is refused: %v", err)
	}
	// With no preset, no resolve_from and no pattern: each is optional.
	m := good()
	m.Preset, m.ResolveFrom = nil, nil
	m.Programs[0].Script.VersionPattern = nil
	if err := plugins.Judge(m); err != nil {
		t.Fatalf("the manifest with no optional field is refused: %v", err)
	}
}

// The six refusals, each a good manifest with one thing planted.
func TestJudgeRefusesWhatTheSchemaCannotHold(t *testing.T) {
	t.Parallel()
	for name, row := range map[string]struct {
		plant            func(m *plugin.Manifest)
		path, form, says string
	}{
		"a grant id given twice": {
			func(m *plugin.Manifest) { m.Programs[2].At.ID = "runtime" },
			"/programs/2/id", plugins.FormPath, "also grant 1's",
		},
		"the tool's id given twice, the second the weaker": {
			func(m *plugin.Manifest) {
				m.Programs = append(m.Programs, plugin.Program{Found: &plugin.ProgramFound{ID: "lintel", Versions: ">=0.0.0", VersionArgv: []string{"-v"}}})
			},
			"/programs/3/id", plugins.FormPinned, "also grant 0's",
		},
		"no grant whose id is the tool": {
			func(m *plugin.Manifest) { m.Tool = "other" },
			"/tool", "", "no grant's id is other",
		},
		"an exclusions path given twice": {
			func(m *plugin.Manifest) {
				m.Counted = &plugin.Counted{Exclusions: [][]string{{"a", "b"}, {"c"}, {"a", "b"}}}
			},
			"/counted/exclusions/2", "", "the path a.b is also exclusions 0's",
		},
		"a loosening at given twice, by values then unless": {
			func(m *plugin.Manifest) {
				m.Counted = &plugin.Counted{Loosening: []plugin.Loosening{
					{Values: &plugin.LooseningValues{At: []string{"run", "tests"}, Values: []string{"false"}}},
					{Unless: &plugin.LooseningUnless{At: []string{"run", "tests"}, Unless: "true"}},
				}}
			},
			"/counted/loosening/1/at", "", "the at run.tests is also loosening 0's",
		},
		"the tool's grant with other versions": {
			func(m *plugin.Manifest) { m.Programs[0].Script.Versions = ">=8.0.0 <10.0.0" },
			"/programs/0/versions", plugins.FormScript, ">=8.0.0 <10.0.0",
		},
		"a script's manifest under .bin": {
			func(m *plugin.Manifest) { m.Programs[0].Script.Manifest = "deps/.bin/package.json" },
			"/programs/0/manifest", plugins.FormScript, ".bin folder",
		},
		"a pattern RE2 does not compile: a look-ahead": {
			func(m *plugin.Manifest) { m.Programs[0].Script.VersionPattern = new(`v(?=[0-9])([0-9.]+)`) },
			"/programs/0/version_pattern", plugins.FormScript, "not a pattern of RE2",
		},
		"a pattern RE2 does not compile: a back-reference": {
			func(m *plugin.Manifest) { m.Programs[1].Found.VersionPattern = new(`(v)\1`) },
			"/programs/1/version_pattern", plugins.FormPinned, "not a pattern of RE2",
		},
		"a pattern that is not closed": {
			func(m *plugin.Manifest) { m.Programs[1].Found.VersionPattern = new(`v([0-9.]+`) },
			"/programs/1/version_pattern", plugins.FormPinned, "not a pattern of RE2",
		},
		"a pattern with no group": {
			func(m *plugin.Manifest) { m.Programs[1].Found.VersionPattern = new(`v[0-9.]+`) },
			"/programs/1/version_pattern", plugins.FormPinned, "has 0 groups",
		},
		"a pattern whose one group does not capture": {
			func(m *plugin.Manifest) { m.Programs[1].Found.VersionPattern = new(`v(?:[0-9.]+)`) },
			"/programs/1/version_pattern", plugins.FormPinned, "has 0 groups",
		},
		"a pattern with two groups": {
			func(m *plugin.Manifest) { m.Programs[2].At.VersionPattern = new(`(v)([0-9.]+)`) },
			"/programs/2/version_pattern", plugins.FormPath, "has 2 groups",
		},
		// The schema asks one character at least; a value that came past
		// no schema is refused all the same.
		"an empty pattern": {
			func(m *plugin.Manifest) { m.Programs[1].Found.VersionPattern = new("") },
			"/programs/1/version_pattern", plugins.FormPinned, "has 0 groups",
		},
		".git in the preset's path": {
			func(m *plugin.Manifest) { m.Preset.Path = ".git/hooks/pre-commit" },
			"/preset/path", "", ".git folder",
		},
		".git in a grant's path": {
			func(m *plugin.Manifest) { m.Programs[2].At.Path = "vendor/.git/tidy" },
			"/programs/2/path", plugins.FormPath, ".git folder",
		},
		".git in resolve_from": {
			func(m *plugin.Manifest) { m.ResolveFrom = new(".git") },
			"/resolve_from", "", ".git folder",
		},
		".git in a preset input": {
			func(m *plugin.Manifest) { m.Preset.Inputs[1] = ".git/config" },
			"/preset/inputs/1", "", ".git folder",
		},
		".git in a script's manifest": {
			func(m *plugin.Manifest) { m.Programs[0].Script.Manifest = ".git/package.json" },
			"/programs/0/manifest", plugins.FormScript, ".git folder",
		},
		// Two values no schema read: the generated types hold a grant to
		// exactly one form, and Judge does not lean on that.
		"a grant of no form": {
			func(m *plugin.Manifest) { m.Programs[1] = plugin.Program{} },
			"/programs/1", "", "holds 0 of the three forms",
		},
		"a grant of two forms": {
			func(m *plugin.Manifest) {
				m.Programs[1].At = &plugin.ProgramAt{ID: "runtime", Versions: ">=20.0.0", VersionArgv: []string{"--version"}, Path: "deps/runtime"}
			},
			"/programs/1", "", "holds 2 of the three forms",
		},
	} {
		m := good()
		row.plant(&m)
		me := refusal(t, m)
		if me.Path != row.path || me.Form != row.form || !strings.Contains(me.Why, row.says) {
			t.Errorf("%s: refused at %q, form %q: %s; want %q, form %q, saying %q", name, me.Path, me.Form, me.Why, row.path, row.form, row.says)
		}
		first, _, _ := strings.Cut(me.Error(), "\n")
		if row.form != "" && !strings.Contains(first, "a grant of the "+row.form+" form") {
			t.Errorf("%s: the first line does not name the form: %s", name, first)
		}
		if !strings.Contains(first, row.path) {
			t.Errorf("%s: the first line does not name the field: %s", name, first)
		}
	}
}

// cached is the good manifest with the feature cache-vars needed and a
// cache_vars in a grant of each form.
func cached() plugin.Manifest {
	m := good()
	m.Needs = []string{plugin.FeatureCacheVars}
	m.Programs[0].Script.CacheVars = []string{"LINTEL_CACHE"}
	m.Programs[1].Found.CacheVars = []string{"RUNTIME_CACHE_DIR", "CACHE"}
	m.Programs[2].At.CacheVars = []string{"TOOL_CACHE_HOME"}
	return m
}

// TestJudgeHasNothingAgainstCacheVarsTheFeatureCovers: names in each form,
// with the feature needed, are nothing to Judge, and the schema takes them
// too. Refusing a name the engine sets itself is the opener's, from
// the step runner's one list, not Judge's: this package holds no copy of it.
func TestJudgeHasNothingAgainstCacheVarsTheFeatureCovers(t *testing.T) {
	t.Parallel()
	m := cached()
	var back plugin.Manifest
	if err := back.FromCanon(m.ToCanon()); err != nil {
		t.Fatalf("the generated types refuse the manifest with cache_vars: %v", err)
	}
	if err := plugins.Judge(m); err != nil {
		t.Fatalf("Judge = %v, want nil", err)
	}
}

// TestJudgeHoldsCacheVarsToTheSchemaAndTheFeature: each bound
// the schema states, for a value built in Go that met none, and both halves
// of the feature: cache_vars with no cache-vars in needs, and cache-vars in
// needs with no cache_vars.
func TestJudgeHoldsCacheVarsToTheSchemaAndTheFeature(t *testing.T) {
	t.Parallel()
	for name, row := range map[string]struct {
		plant            func(m *plugin.Manifest)
		path, form, says string
	}{
		"cache_vars with no cache-vars in needs": {
			func(m *plugin.Manifest) { m.Needs = []string{} },
			"/programs/0/cache_vars", plugins.FormScript, "needs does not name cache-vars",
		},
		"cache-vars in needs with no cache_vars": {
			func(m *plugin.Manifest) {
				m.Programs[0].Script.CacheVars, m.Programs[1].Found.CacheVars, m.Programs[2].At.CacheVars = nil, nil, nil
			},
			"/needs/0", "", "no grant gives cache_vars",
		},
		"an empty list": {
			func(m *plugin.Manifest) { m.Programs[1].Found.CacheVars = []string{} },
			"/programs/1/cache_vars", plugins.FormPinned, "is empty",
		},
		"nine names": {
			func(m *plugin.Manifest) {
				m.Programs[2].At.CacheVars = []string{"A_CACHE", "B_CACHE", "C_CACHE", "D_CACHE", "E_CACHE", "F_CACHE", "G_CACHE", "H_CACHE", "I_CACHE"}
			},
			"/programs/2/cache_vars", plugins.FormPath, "holds 9 names, and the bound is 8",
		},
		"a name given twice": {
			func(m *plugin.Manifest) { m.Programs[1].Found.CacheVars = []string{"CACHE", "GOCACHE", "CACHE"} },
			"/programs/1/cache_vars/2", plugins.FormPinned, "also cache_vars 0's",
		},
		"a name without CACHE": {
			func(m *plugin.Manifest) { m.Programs[0].Script.CacheVars = []string{"PATH"} },
			"/programs/0/cache_vars/0", plugins.FormScript, "no cache variable's name",
		},
		"a name in small letters": {
			func(m *plugin.Manifest) { m.Programs[0].Script.CacheVars = []string{"gocache"} },
			"/programs/0/cache_vars/0", plugins.FormScript, "no cache variable's name",
		},
		"a name starting with a digit": {
			func(m *plugin.Manifest) { m.Programs[0].Script.CacheVars = []string{"1_CACHE"} },
			"/programs/0/cache_vars/0", plugins.FormScript, "no cache variable's name",
		},
		"a name that gives a value": {
			func(m *plugin.Manifest) { m.Programs[0].Script.CacheVars = []string{"GOCACHE=cache"} },
			"/programs/0/cache_vars/0", plugins.FormScript, "no cache variable's name",
		},
		"a name past 64 bytes": {
			func(m *plugin.Manifest) {
				m.Programs[0].Script.CacheVars = []string{"CACHE_" + strings.Repeat("X", 59)}
			},
			"/programs/0/cache_vars/0", plugins.FormScript, "no cache variable's name",
		},
		"an empty name": {
			func(m *plugin.Manifest) { m.Programs[0].Script.CacheVars = []string{""} },
			"/programs/0/cache_vars/0", plugins.FormScript, "no cache variable's name",
		},
	} {
		m := cached()
		row.plant(&m)
		me := refusal(t, m)
		if me.Path != row.path || me.Form != row.form || !strings.Contains(me.Why, row.says) {
			t.Errorf("%s: refused at %q, form %q: %s; want %q, form %q, saying %q", name, me.Path, me.Form, me.Why, row.path, row.form, row.says)
		}
	}
}

// TestJudgeHoldsCacheVarsAtTheirBounds: each cap of a grant's
// cache_vars is held at its boundary: one under and the cap
// itself pass, one over is refused and names the field.
//
// boundary: maxCacheVars (7 and 8 names pass, 9 are refused)
// boundary: maxCacheVarName (63 and 64 bytes pass, 65 are refused)
func TestJudgeHoldsCacheVarsAtTheirBounds(t *testing.T) {
	t.Parallel()
	names := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = string(rune('A'+i)) + "_CACHE"
		}
		return out
	}
	long := func(n int) []string { return []string{"CACHE_" + strings.Repeat("X", n-len("CACHE_"))} }
	for name, row := range map[string]struct {
		vars []string
		path string
	}{
		"7 names":        {names(7), ""},
		"8 names":        {names(8), ""},
		"9 names":        {names(9), "/programs/2/cache_vars"},
		"a 63-byte name": {long(63), ""},
		"a 64-byte name": {long(64), ""},
		"a 65-byte name": {long(65), "/programs/2/cache_vars/0"},
	} {
		m := cached()
		m.Programs[2].At.CacheVars = row.vars
		if row.path == "" {
			if err := plugins.Judge(m); err != nil {
				t.Errorf("%s: Judge = %v, want nil", name, err)
			}
			continue
		}
		if me := refusal(t, m); me.Path != row.path {
			t.Errorf("%s: refused at %q: %s; want %q", name, me.Path, me.Why, row.path)
		}
	}
}

// Judge answers the first refusal in the manifest's own order, so the same
// manifest is refused at the same field every time it is judged.
func TestJudgeAnswersTheFirstRefusalInTheManifestsOrder(t *testing.T) {
	t.Parallel()
	m := good()
	m.Programs[2].At.Path = ".git/tidy"
	m.Programs[1].Found.VersionPattern = new("no group")
	m.Preset.Path = ".git/x"
	for range 20 {
		if me := refusal(t, m); me.Path != "/programs/1/version_pattern" {
			t.Fatalf("refused at %s, and grant 1 comes first", me.Path)
		}
	}
}

// A reserved folder is refused by every spelling a file system opens it by,
// at every depth, in every path the manifest holds.
func TestJudgeRefusesAReservedFolderByEverySpelling(t *testing.T) {
	t.Parallel()
	fields := map[string]func(m *plugin.Manifest, path string){
		"/preset/path":         func(m *plugin.Manifest, p string) { m.Preset.Path = p },
		"/preset/inputs/0":     func(m *plugin.Manifest, p string) { m.Preset.Inputs[0] = p },
		"/resolve_from":        func(m *plugin.Manifest, p string) { m.ResolveFrom = new(p) },
		"/programs/2/path":     func(m *plugin.Manifest, p string) { m.Programs[2].At.Path = p },
		"/programs/0/manifest": func(m *plugin.Manifest, p string) { m.Programs[0].Script.Manifest = p },
	}
	for _, spelling := range []string{".git", ".GIT", ".Git", ".gIt", ".git.", ".GIT.", ".git..", ".Git..."} {
		for _, path := range []string{spelling, spelling + "/config", "a/" + spelling + "/hooks/x", "a/b/" + spelling} {
			for field, set := range fields {
				m := good()
				set(&m, path)
				if me := refusal(t, m); me.Path != field || !strings.Contains(me.Why, ".git folder") {
					t.Errorf("%s as %q: refused at %q: %s", field, path, me.Path, me.Why)
				}
			}
		}
	}
	for _, spelling := range []string{".bin", ".BIN", ".Bin", ".bin.", ".BIN.", ".Bin.."} {
		for _, path := range []string{"deps/" + spelling + "/package.json", spelling + "/package.json", "a/" + spelling} {
			m := good()
			m.Programs[0].Script.Manifest = path
			if me := refusal(t, m); me.Path != "/programs/0/manifest" || !strings.Contains(me.Why, ".bin folder") {
				t.Errorf("a manifest at %q: refused at %q: %s", path, me.Path, me.Why)
			}
		}
	}
}

// A name that only looks near a reserved one is a path like any other, and a
// .bin folder is refused for a script's manifest alone.
func TestJudgeLeavesANearNameAlone(t *testing.T) {
	t.Parallel()
	for _, path := range []string{".github/workflows/x.yml", ".gitignore", "a/.gitattributes", "git/config", "x.git/config", ".git-hooks/x", "..git/x", ".gitx./y"} {
		m := good()
		m.Preset.Path, m.Preset.Inputs[0], m.ResolveFrom = path, path, new(path)
		m.Programs[2].At.Path, m.Programs[0].Script.Manifest = path, path
		if err := plugins.Judge(m); err != nil {
			t.Errorf("%q is refused: %v", path, err)
		}
	}
	for _, path := range []string{"deps/bin/package.json", "deps/.bind/package.json", "deps/x.bin/package.json"} {
		m := good()
		m.Programs[0].Script.Manifest = path
		if err := plugins.Judge(m); err != nil {
			t.Errorf("a manifest at %q is refused: %v", path, err)
		}
	}
	m := good()
	m.Programs[2].At.Path = "deps/.bin/tidy"
	m.Preset.Inputs[0] = "deps/.bin/x"
	if err := plugins.Judge(m); err != nil {
		t.Errorf(".bin outside a script's manifest is refused: %v", err)
	}
}

// What a refusal says of the manifest's own words is escaped, so a manifest
// that came past no schema cannot write to the screen through its refusal.
func TestARefusalEscapesTheManifestsWords(t *testing.T) {
	t.Parallel()
	const esc, turn = "\x1b", "\u202e"
	for name, plant := range map[string]func(m *plugin.Manifest){
		"the tool": func(m *plugin.Manifest) { m.Tool = "lin" + esc + "[2Jtel" },
		"an id twice": func(m *plugin.Manifest) {
			m.Programs[1].Found.ID, m.Programs[2].At.ID = "run"+esc+"time", "run"+esc+"time"
		},
		"the versions":      func(m *plugin.Manifest) { m.Programs[0].Script.Versions = ">=9.0.0" + esc + "[2J" },
		"the tool versions": func(m *plugin.Manifest) { m.ToolVersions = ">=9.0.0" + turn },
		"a pattern":         func(m *plugin.Manifest) { m.Programs[1].Found.VersionPattern = new("(" + esc + "[2J") },
		"two groups":        func(m *plugin.Manifest) { m.Programs[1].Found.VersionPattern = new("(" + esc + ")(a)") },
		"a path":            func(m *plugin.Manifest) { m.ResolveFrom = new("a" + esc + "[2J/.git") },
		"a .bin path":       func(m *plugin.Manifest) { m.Programs[0].Script.Manifest = "a" + turn + "/.bin/package.json" },
	} {
		m := good()
		plant(&m)
		me := refusal(t, m)
		if strings.ContainsAny(me.Error(), esc+turn) {
			t.Errorf("%s: the refusal holds the character itself: %q", name, me.Error())
		}
	}
}

// TestJudgeHoldsPresetReadbackToItsLimits: the order of defaults is a thing
// the schema cannot say, and a manifest built in Go has met no schema, so each
// limit is refused here and names the key. Each cap is held at its boundary:
// one under and the cap itself pass, one over is
// refused.
//
// boundary: maxDefaults (63 and 64 keys pass, 65 are refused)
// boundary: maxDefaultKey (127 and 128 bytes pass, 129 are refused)
func TestJudgeHoldsPresetReadbackToItsLimits(t *testing.T) {
	t.Parallel()
	with := func(r plugin.PresetReadback) func(m *plugin.Manifest) {
		return func(m *plugin.Manifest) { m.Preset.Readback = &r }
	}
	over := make([]string, 65)
	for i := range over {
		over[i] = string(rune('a'+i/26)) + string(rune('a'+i%26))
	}
	for name, row := range map[string]struct {
		plant     func(m *plugin.Manifest)
		path, say string
	}{
		"a readback with no member":               {with(plugin.PresetReadback{}), "/preset/readback", "holds no member"},
		"an empty defaults":                       {with(plugin.PresetReadback{Defaults: []string{}}), "/preset/readback/defaults", "is empty"},
		"more defaults than the bound":            {with(plugin.PresetReadback{Defaults: over}), "/preset/readback/defaults", "the bound is 64"},
		"an empty name":                           {with(plugin.PresetReadback{Defaults: []string{""}}), "/preset/readback/defaults/0", "is empty"},
		"a control character in a name":           {with(plugin.PresetReadback{Defaults: []string{"a", "b\x1b[31m"}}), "/preset/readback/defaults/1", "does not show as written"},
		"a name past 128 bytes":                   {with(plugin.PresetReadback{Defaults: []string{"a", strings.Repeat("b", 129)}}), "/preset/readback/defaults/1", "the bound is 128"},
		"a name past 128 bytes, in 65 characters": {with(plugin.PresetReadback{Defaults: []string{"a", strings.Repeat("é", 65)}}), "/preset/readback/defaults/1", "is 130 bytes"},
		"a name out of byte order":                {with(plugin.PresetReadback{Defaults: []string{"b", "a"}}), "/preset/readback/defaults/1", "byte order"},
		"upper case after lower case":             {with(plugin.PresetReadback{Defaults: []string{"a", "B"}}), "/preset/readback/defaults/1", "byte order"},
		"a name given twice":                      {with(plugin.PresetReadback{Defaults: []string{"a", "b", "b"}}), "/preset/readback/defaults/2", "also defaults 1's"},
	} {
		m := good()
		row.plant(&m)
		me := refusal(t, m)
		if me.Path != row.path || !strings.Contains(me.Why, row.say) {
			t.Errorf("%s: refused at %q: %s; want %q saying %q", name, me.Path, me.Why, row.path, row.say)
		}
		if strings.ContainsRune(me.Error(), 0x1b) {
			t.Errorf("%s: the refusal carries a raw escape: %q", name, me.Error())
		}
	}
	for name, r := range map[string]plugin.PresetReadback{
		"severity alone":                     {Severity: new(false)},
		"defaults in byte order":             {Defaults: []string{"B", "a", "b"}},
		"severity and defaults":              {Severity: new(true), Defaults: []string{"gofmt", "govet"}},
		"one under the bound":                {Defaults: over[:63]},
		"exactly the bound":                  {Defaults: over[:64]},
		"a name of 127 bytes":                {Defaults: []string{strings.Repeat("a", 127)}},
		"a name of 128 bytes":                {Defaults: []string{strings.Repeat("a", 128)}},
		"a name of 64 characters, 128 bytes": {Defaults: []string{strings.Repeat("é", 64)}},
	} {
		m := good()
		m.Preset.Readback = &r
		if err := plugins.Judge(m); err != nil {
			t.Errorf("%s: Judge refused a readback within its limits: %v", name, err)
		}
	}
}
