package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"
	"github.com/birdtable/gragas/internal/ordered"
)

// The node tools run no script at install, and a new one fails the install
// loudly. task tools installs
// without --ignore-scripts, so every install, its own as much as one that adds
// or bumps a tool, meets strictDepBuilds, which makes pnpm fail on a package
// with a build script it was not told about, where ignoreScripts
// would skip every script silently and a new one would go unseen. So
// ignoreScripts is not set, and every package allowBuilds names is set false,
// never run. Without ignoreScripts, pnpm runs the workspace's own lifecycle
// scripts, so the workspace's package.json carries none.

const (
	nodeWorkspace = "tools/.node/pnpm-workspace.yaml"
	nodePackage   = "tools/.node/package.json"
)

// lifecycleScripts are the scripts pnpm runs for the workspace's own package
// on an install (ruled on 1127).
func lifecycleScripts() []string {
	return []string{"pnpm:devPreinstall", "preinstall", "install", "postinstall", "preprepare", "prepare", "postprepare"}
}

// quietBuildKeys are the settings that let a build script run, or skip one
// unseen, outside allowBuilds: none is set.
func quietBuildKeys() []string {
	return []string{"ignoreScripts", "onlyBuiltDependencies", "onlyBuiltDependenciesFile", "neverBuiltDependencies", "ignoredBuiltDependencies", "dangerouslyAllowAllBuilds"}
}

// buildProblems are the findings on a pnpm-workspace.yaml's text: no
// strictDepBuilds: true, a quiet build key, an allowBuilds that is not a
// block of names each written false, or a key written twice. Each value is
// read as the line writes it too, so a YAML 1.1 "off" or "no", which pnpm
// may not read as false, is red.
func buildProblems(path, text string) []string {
	v, err := ordered.ParseYAML([]byte(text))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		return []string{path + " is not a mapping"}
	}
	lines := strings.Split(text, "\n")
	written := func(m *ordered.Map, k string) string {
		n := m.KeyLine(k)
		if n < 1 || n > len(lines) {
			return ""
		}
		_, val, _ := strings.Cut(lines[n-1], ":")
		val, _, _ = strings.Cut(val, "#")
		return strings.TrimSpace(val)
	}
	var bad []string
	for _, r := range m.Repeats() {
		bad = append(bad, fmt.Sprintf("%s writes %s twice (lines %d and %d)", path, r.Key, r.First, r.Again))
	}
	if got, _ := m.Get("strictDepBuilds"); got != true || written(m, "strictDepBuilds") != "true" {
		bad = append(bad, path+" does not set strictDepBuilds: true, so an install that meets a new build script does not fail")
	}
	for _, k := range quietBuildKeys() {
		if _, ok := m.Get(k); ok {
			bad = append(bad, fmt.Sprintf("%s sets %s, which runs a build script or skips one unseen: name the package under allowBuilds, set false", path, k))
		}
	}
	if raw, ok := m.Get("allowBuilds"); ok {
		builds, ok := raw.(*ordered.Map)
		if !ok {
			return append(bad, path+": allowBuilds is not a mapping of package names")
		}
		for _, r := range builds.Repeats() {
			bad = append(bad, fmt.Sprintf("%s: allowBuilds writes %s twice (lines %d and %d)", path, r.Key, r.First, r.Again))
		}
		for _, name := range builds.Keys() {
			if got, _ := builds.Get(name); got != false || written(builds, name) != "false" {
				bad = append(bad, fmt.Sprintf("%s: allowBuilds lets %s run its build script: every entry is written false, one to a line", path, name))
			}
		}
	}
	return bad
}

// scriptProblems are the findings on a package.json's text: a lifecycle
// script pnpm runs on an install, a scripts value that is not an object, a
// pnpm settings field (they live in pnpm-workspace.yaml, in plain view), or
// text that is not one JSON object.
func scriptProblems(path string, text []byte) []string {
	var pkg map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(text))
	if err := dec.Decode(&pkg); err != nil || dec.More() {
		return []string{path + " is not one JSON object"}
	}
	var bad []string
	if _, ok := pkg["pnpm"]; ok {
		bad = append(bad, path+" has a pnpm field: the install settings are pnpm-workspace.yaml's")
	}
	raw, ok := pkg["scripts"]
	if !ok {
		return bad
	}
	var scripts map[string]json.RawMessage
	if err := json.Unmarshal(raw, &scripts); err != nil || scripts == nil {
		return append(bad, path+": scripts is not an object")
	}
	for _, name := range lifecycleScripts() {
		if _, ok := scripts[name]; ok {
			bad = append(bad, fmt.Sprintf("%s has a %s script, which pnpm runs on an install: the node tools run no script", path, name))
		}
	}
	return bad
}

// TestTheNodeToolsRunNoInstallScript: tools/.node's workspace fails an
// install that meets a build script and runs none, and its package.json
// has no lifecycle script.
func TestTheNodeToolsRunNoInstallScript(t *testing.T) {
	t.Parallel()
	for _, p := range buildProblems(nodeWorkspace, read(t, nodeWorkspace)) {
		t.Error(p)
	}
	for _, p := range scriptProblems(nodePackage, []byte(read(t, nodePackage))) {
		t.Error(p)
	}
}

// TestABuildScriptMakesTheToolsInstallRed: task tools installs tools/.node
// with no --ignore-scripts, so a tool that brings a build script makes the
// install red under the workspace's own settings, and the script does not
// run. The pinned pnpm installs from a fake registry on the loopback, as
// TestPnpmRefusesALockedPinYoungerThanTheWindow does, and its script leaves a
// file in its package's folder when it runs. The controls:
//   - the same tool listed under allowBuilds, set false, installs and runs
//     nothing;
//   - set true, it runs, so the file is the script's mark;
//   - a tool with no build script installs.
//
// The line this replaced, with --ignore-scripts, installed the plant green and
// said nothing: the gap this closes. Without that flag, a setting from outside
// the repository beats the workspace file: the shell's
// pnpm_config_dangerously_allow_all_builds, or dangerouslyAllowAllBuilds in
// the person's global pnpm config.yaml, each run the script, and the shell's
// pnpm_config_strict_dep_builds=false makes the refusal a warning. The line's
// two --config flags beat all three, so each is red under it. Each control,
// the same install with no flag, shows pnpm read the setting: the script runs,
// or the install passes. It sets PATH, so it runs alone.
func TestABuildScriptMakesTheToolsInstallRed(t *testing.T) {
	pnpm, path := pinnedPnpm(t)
	settings := read(t, nodeWorkspace)
	const plant = "gragas-plant-builds"
	reg, ok := npmRegistry(t, map[string]npmPackage{
		plant:                {published: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), scripts: map[string]string{"postinstall": `node -e "require('fs').writeFileSync('ran', '')"`}},
		"gragas-plant-plain": {published: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
	})
	if !ok {
		return
	}
	proxy, seen, ok := npmProxy(t)
	if !ok {
		return
	}
	p := pnpmRun{pnpm: pnpm, path: path, reg: reg}
	if err := p.asks(t, pnpmRegistryKey+reg, proxy); err != nil {
		t.Fatalf("%v: nothing is installed", err)
	}
	defer func() {
		if s := seen(); len(s) > 0 {
			t.Errorf("pnpm asked for a host outside the loopback, and the test's proxy refused it: %v", s)
		}
	}()
	// Settings from outside the repository: a variable of the person's shell,
	// and their global pnpm config, each of which beats the workspace file.
	// The global file's strictDepBuilds loses to the workspace file's, so only
	// the shell's variable can loosen it.
	const allBuilds = "pnpm_config_dangerously_allow_all_builds=true"
	const loose = "pnpm_config_strict_dep_builds=false"
	global := "dangerouslyAllowAllBuilds: true\n"
	bare := []string{"install", "--frozen-lockfile"}
	for name, c := range map[string]struct {
		dep, settings string
		// scripts: tools/.node's line; else the line it replaced. args: a control's line instead.
		scripts, refused, runs bool
		args                   []string
		// env: variables of the shell; global: the global pnpm config.yaml.
		env    []string
		global string
	}{
		"a tool with a build script":                                {dep: plant, settings: settings, scripts: true, refused: true},
		"the same tool under allowBuilds, set false, the control":   {dep: plant, settings: settings + "allowBuilds:\n  " + plant + ": false\n", scripts: true},
		"the same tool allowed to build, the mark's control":        {dep: plant, settings: settings + "allowBuilds:\n  " + plant + ": true\n", scripts: true, runs: true},
		"a tool with no build script, the control":                  {dep: "gragas-plant-plain", settings: settings, scripts: true},
		"the line this replaced, --ignore-scripts, lets it through": {dep: plant, settings: settings},
		"all builds allowed by the shell, held by the line":         {dep: plant, settings: settings, scripts: true, refused: true, env: []string{allBuilds}},
		"the same with no --config flag, the control":               {dep: plant, settings: settings, args: bare, runs: true, env: []string{allBuilds}},
		"loosened by the global pnpm config, held by the line":      {dep: plant, settings: settings, scripts: true, refused: true, global: global},
		"the same with no --config flag, the global's control":      {dep: plant, settings: settings, args: bare, runs: true, global: global},
		"the refusal made a warning by the shell, held by the line": {dep: plant, settings: settings, scripts: true, refused: true, env: []string{loose}},
		"the same with no --config flag, the warning's control":     {dep: plant, settings: settings, args: bare, env: []string{loose}},
	} {
		t.Run(name, func(t *testing.T) {
			p := p
			p.env = c.env
			if c.global != "" {
				p.home = t.TempDir()
				for _, cfg := range configHomes(p.home) {
					if err := os.MkdirAll(filepath.Join(cfg, "pnpm"), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(cfg, "pnpm", "config.yaml"), []byte(c.global), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			dir, out, err := p.frozen(t, proxy, freeze{dep: c.dep, lockWith: settings, settings: c.settings, scripts: c.scripts, args: c.args})
			switch {
			case c.refused && (err == nil || !strings.Contains(out, "ERR_PNPM_IGNORED_BUILDS") || !strings.Contains(out, plant+"@1.0.0")):
				t.Errorf("the install did not refuse the build script by name: %v\n%s", err, out)
			case !c.refused && err != nil:
				t.Errorf("the control did not install: %v\n%s", err, out)
			}
			_, err = os.Stat(filepath.Join(dir, "node_modules", c.dep, "ran"))
			if ran := err == nil; ran != c.runs {
				t.Errorf("the build script ran: %v, want %v\n%s", ran, c.runs, out)
			}
		})
	}
}

// TestABuildSettingThatRunsAScriptIsFound plants each loosening of the
// workspace file, each red, beside the controls, each green.
func TestABuildSettingThatRunsAScriptIsFound(t *testing.T) {
	t.Parallel()
	const base = "strictDepBuilds: true\nsaveExact: true\n"
	if got := buildProblems("plant.yaml", base); len(got) != 0 {
		t.Fatalf("the control finds %q", got)
	}
	for name, c := range map[string]struct{ text, says string }{
		"strictDepBuilds deleted":       {strings.Replace(base, "strictDepBuilds: true\n", "", 1), "does not set strictDepBuilds: true"},
		"strictDepBuilds false":         {strings.Replace(base, "strictDepBuilds: true", "strictDepBuilds: false", 1), "does not set strictDepBuilds: true"},
		"strictDepBuilds yes":           {strings.Replace(base, "strictDepBuilds: true", "strictDepBuilds: yes", 1), "does not set strictDepBuilds: true"},
		"strictDepBuilds written twice": {base + "strictDepBuilds: false\n", "writes strictDepBuilds twice"},
		"ignoreScripts back":            {base + "ignoreScripts: true\n", "sets ignoreScripts"},
		"onlyBuiltDependencies":         {base + "onlyBuiltDependencies:\n  - esbuild\n", "sets onlyBuiltDependencies"},
		"neverBuiltDependencies":        {base + "neverBuiltDependencies:\n  - esbuild\n", "sets neverBuiltDependencies"},
		"dangerouslyAllowAllBuilds":     {base + "dangerouslyAllowAllBuilds: true\n", "sets dangerouslyAllowAllBuilds"},
		"an entry set true":             {base + "allowBuilds:\n  esbuild: false\n  plant: true\n", "lets plant run"},
		"an entry set off":              {base + "allowBuilds:\n  plant: off\n", "lets plant run"},
		"an entry with no value":        {base + "allowBuilds:\n  plant:\n", "lets plant run"},
		"an entry written twice":        {base + "allowBuilds:\n  plant: false\n  plant: true\n", "allowBuilds writes plant twice"},
		"a flow mapping":                {base + "allowBuilds: {plant: false}\n", "lets plant run"},
		"allowBuilds a list":            {base + "allowBuilds:\n  - plant\n", "allowBuilds is not a mapping"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bad := buildProblems("plant.yaml", c.text)
			if !slices.ContainsFunc(bad, func(s string) bool { return strings.Contains(s, c.says) }) {
				t.Errorf("the plant was not caught: want a finding saying %q, got %q", c.says, bad)
			}
		})
	}
	for name, text := range map[string]string{
		"an entry set false":         base + "allowBuilds:\n  esbuild: false\n",
		"two entries, a comment":     base + "allowBuilds:\n  esbuild: false # its script only checks a binary\n  '@plant/x': false\n",
		"an empty allowBuilds block": base + "allowBuilds: {}\n",
	} {
		t.Run("control: "+name, func(t *testing.T) {
			t.Parallel()
			if got := buildProblems("plant.yaml", text); len(got) != 0 {
				t.Errorf("want nothing found, got %q", got)
			}
		})
	}
}

// TestALifecycleScriptIsFound plants each script pnpm runs on an install,
// each red, beside the controls, each green.
func TestALifecycleScriptIsFound(t *testing.T) {
	t.Parallel()
	pkg := func(scripts string) []byte {
		return []byte(`{"name": "plant", "private": true` + scripts + `, "devDependencies": {"prettier": "3.9.10"}}`)
	}
	for _, name := range lifecycleScripts() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bad := scriptProblems("package.json", pkg(`, "scripts": {"lint": "eslint .", "`+name+`": "node plant.js"}`))
			if !slices.ContainsFunc(bad, func(s string) bool { return strings.Contains(s, "has a "+name+" script") }) {
				t.Errorf("the plant was not caught: got %q", bad)
			}
		})
	}
	for name, c := range map[string]struct {
		text []byte
		says string
	}{
		"scripts a string":         {pkg(`, "scripts": "node plant.js"`), "scripts is not an object"},
		"scripts null":             {pkg(`, "scripts": null`), "scripts is not an object"},
		"a pnpm field":             {pkg(`, "pnpm": {"onlyBuiltDependencies": ["plant"]}`), "has a pnpm field"},
		"a second object after":    {append(pkg(""), `{"scripts": {"install": "node plant.js"}}`...), "is not one JSON object"},
		"the script written twice": {pkg(`, "scripts": {}, "scripts": {"postinstall": "node plant.js"}`), "has a postinstall script"},
		"not JSON":                 {[]byte("scripts:\n  install: node plant.js\n"), "is not one JSON object"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bad := scriptProblems("package.json", c.text)
			if !slices.ContainsFunc(bad, func(s string) bool { return strings.Contains(s, c.says) }) {
				t.Errorf("the plant was not caught: want a finding saying %q, got %q", c.says, bad)
			}
		})
	}
	for name, text := range map[string][]byte{
		"no scripts": pkg(""),
		"scripts that pnpm never runs on an install": pkg(`, "scripts": {"lint": "eslint .", "test": "node --test", "prepublishOnly": "true"}`),
	} {
		t.Run("control: "+name, func(t *testing.T) {
			t.Parallel()
			if got := scriptProblems("package.json", text); len(got) != 0 {
				t.Errorf("want nothing found, got %q", got)
			}
		})
	}
}

// configHomes are the folders beneath home that go-proc's HomeEnv gives as a program's configuration home, where
// pnpm reads its global config: the case's own, so a global config planted there is the only one it can read.
func configHomes(home string) []string {
	var out []string
	for _, kv := range proc.HomeEnv(home) {
		if k, v, _ := strings.Cut(kv, "="); strings.HasSuffix(k, "_CONFIG_HOME") {
			out = append(out, v)
		}
	}
	return out
}
