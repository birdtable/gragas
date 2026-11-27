package tool_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/proc"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// inRange is the versions every grant of these rows serves.
const inRange = ">=1.0.0 <2.0.0"

// foundGrant is a manifest granting lint-a as a Found program: no path, no script.
func foundGrant() plugin.Manifest {
	return plugin.Manifest{Programs: []plugin.Program{{Found: &plugin.ProgramFound{ID: "lint-a", Versions: inRange, VersionArgv: []string{"--version"}}}}}
}

// atGrant is a manifest granting lint-a at bin/lint-a below the project.
func atGrant() plugin.Manifest {
	return plugin.Manifest{Programs: []plugin.Program{{At: &plugin.ProgramAt{ID: "lint-a", Versions: inRange, VersionArgv: []string{"--version"}, Path: binDir + "/lint-a"}}}}
}

// withPattern is m with its one program reading its version by pattern.
func withPattern(m plugin.Manifest, pattern string) plugin.Manifest {
	switch p := &m.Programs[0]; {
	case p.Found != nil:
		p.Found.VersionPattern = &pattern
	case p.At != nil:
		p.At.VersionPattern = &pattern
	}
	return m
}

// scriptGrant is a manifest granting lint-a as a script: the string at
// pointer in the manifest at manifest, run by node.
func scriptGrant(manifest, pointer string) plugin.Manifest {
	return plugin.Manifest{Programs: []plugin.Program{{Script: &plugin.ProgramScript{
		ID: "lint-a", Versions: inRange, VersionArgv: []string{"--version"},
		Manifest: manifest, ScriptAt: pointer, Interpreter: "node",
	}}}}
}

// elsewhere is a program at a folder of its own, outside any project,
// saying lint-a v for --version.
func elsewhere(t *testing.T, v string) string {
	t.Helper()
	return program(t, private(t)+"/lint-a", says(v))
}

// TestAStepNamesItsProgramByGrant: a program the
// manifest does not grant matches port.ErrGrant, and nothing is run.
func TestAStepNamesItsProgramByGrant(t *testing.T) {
	t.Parallel()
	root := private(t)
	tl := over(map[string]map[string]string{root: {"other@1.2.3": elsewhere(t, "1.2.3")}}, nil)
	_, err := tool.LocateGrant(tl, t.Context(), model.Root(root), foundGrant(), "other", false)
	if !errors.Is(err, port.ErrGrant) {
		t.Fatalf("LocateGrant of an ungranted id = %v, want port.ErrGrant", err)
	}
	if len(tl.Ran()) != 0 {
		t.Errorf("an ungranted program was run: %+v", tl.Ran())
	}
}

// TestAGrantIsFoundInItsOrder (item 2): the settings before the grant's own
// place, the grant's place before the tools file, the tools file when it is
// the only one; a pinned find never takes the grant's place and names it as
// not tried; a Found grant is found in the settings and the tools file alone.
func TestAGrantIsFoundInItsOrder(t *testing.T) {
	t.Parallel()
	root := private(t)
	own := program(t, root+"/"+binDir+"/lint-a", says("1.2.3"))
	set, pin := elsewhere(t, "1.2.3"), elsewhere(t, "1.2.3")
	settings := map[string]map[string]string{root: {"lint-a@1.2.3": set}}
	machine := map[string]string{"lint-a@1.2.3": pin}
	for name, c := range map[string]struct {
		m       plugin.Manifest
		paths   map[string]map[string]string
		machine map[string]string
		pinned  bool
		path    string
		step    tool.Step
	}{
		"the settings first":                      {atGrant(), settings, machine, false, set, tool.StepSettings},
		"the grant's place before the tools file": {atGrant(), nil, machine, false, own, "grant"},
		"the tools file last":                     {foundGrant(), nil, machine, false, pin, tool.StepToolsFile},
		"a pinned find passes the grant's place":  {atGrant(), nil, machine, true, pin, tool.StepToolsFile},
		"a Found grant takes the settings":        {foundGrant(), settings, machine, false, set, tool.StepSettings},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tl := over(c.paths, c.machine)
			f, err := tool.LocateGrant(tl, t.Context(), model.Root(root), c.m, "lint-a", c.pinned)
			if err != nil || f.Path != c.path || f.Step != string(c.step) || f.Version != "1.2.3" {
				t.Fatalf("LocateGrant = %+v, %v; want %s at step %s, 1.2.3", f, err, c.path, c.step)
			}
		})
	}
	// Two versions of one tool named in the settings for one root are
	// refused, naming both, and nothing is started: the find never picks one
	// of them by its order.
	t.Run("two versions of the tool in the settings for one root", func(t *testing.T) {
		t.Parallel()
		tl := over(map[string]map[string]string{root: {"lint-a@1.2.3": set, "lint-a@1.4.0": elsewhere(t, "1.4.0")}}, machine)
		_, err := tool.LocateGrant(tl, t.Context(), model.Root(root), atGrant(), "lint-a", false)
		endsAs(t, err, "the settings name lint-a at lint-a@1.2.3 and lint-a@1.4.0 for "+root+": a project runs one version of a tool", port.ErrRefused)
		if len(tl.Ran()) != 0 {
			t.Errorf("a refused find started a program: %+v", tl.Ran())
		}
	})
	t.Run("a pinned find with the grant's place alone", func(t *testing.T) {
		t.Parallel()
		tl := over(nil, nil)
		_, err := tool.LocateGrant(tl, t.Context(), model.Root(root), atGrant(), "lint-a", true)
		var nf *tool.NotFound
		if !errors.As(err, &nf) || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(strings.Join(nf.Tried, "\n"), "grant "+own+" (not tried") {
			t.Fatalf("LocateGrant = %v, want a NotFound naming the grant's place as not tried", err)
		}
		if len(tl.Ran()) != 0 {
			t.Errorf("a pinned find ran the grant's place: %+v", tl.Ran())
		}
	})
}

// endsAs fails t unless err says says and matches is but not the other of
// port.ErrRefused and port.ErrStepNotReady: it asserts errors.Is both ways,
// so a trust failure never reads as not ready,
// nor a missing version as a refusal.
func endsAs(t *testing.T, err error, says string, is error) {
	t.Helper()
	other := port.ErrStepNotReady
	if errors.Is(is, port.ErrStepNotReady) {
		other = port.ErrRefused
	}
	if !errors.Is(err, is) || errors.Is(err, other) || !strings.Contains(err.Error(), says) {
		t.Fatalf("LocateGrant = %v; want one that matches %v and not %v, saying %q", err, is, other, says)
	}
}

// TestAPinnedFindTakesTheNewestKeyInTheRange: of the tools file's keys for the
// id, the newest inside the range, compared in full; with none inside, the
// step is not ready, naming the range and the versions pinned; a pinned
// program that prints another version than its key is refused, a trust
// failure; a settings path outside the range is not ready.
func TestAPinnedFindTakesTheNewestKeyInTheRange(t *testing.T) {
	t.Parallel()
	root := private(t)
	for name, c := range map[string]struct {
		paths   map[string]map[string]string
		machine map[string]string
		want    string
		says    string
		is      error
	}{
		"two keys in the range, the newer taken": {nil, map[string]string{"lint-a@1.2.3": elsewhere(t, "1.2.3"), "lint-a@1.4.0": elsewhere(t, "1.4.0")}, "1.4.0", "", nil},
		"1.10.0 after 1.9.0":                     {nil, map[string]string{"lint-a@1.9.0": elsewhere(t, "1.9.0"), "lint-a@1.10.0": elsewhere(t, "1.10.0")}, "1.10.0", "", nil},
		"a key outside the range never taken":    {nil, map[string]string{"lint-a@1.2.3": elsewhere(t, "1.2.3"), "lint-a@2.1.0": elsewhere(t, "2.1.0")}, "1.2.3", "", nil},
		"the only key outside the range":         {nil, map[string]string{"lint-a@2.1.0": elsewhere(t, "2.1.0")}, "", "pins lint-a at 2.1.0, and none is inside the grant's versions '" + inRange + "'", port.ErrStepNotReady},
		"a pinned program that prints another":   {nil, map[string]string{"lint-a@1.4.0": elsewhere(t, "1.3.0")}, "", "pins lint-a 1.4.0 at", port.ErrRefused},
		"a settings path outside the range":      {map[string]map[string]string{root: {"lint-a@1.2.3": elsewhere(t, "2.5.0")}}, nil, "", "is 2.5.0, which is outside the grant's versions '" + inRange + "'", port.ErrStepNotReady},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tl := over(c.paths, c.machine)
			f, err := tool.LocateGrant(tl, t.Context(), model.Root(root), foundGrant(), "lint-a", true)
			if c.is == nil {
				if err != nil || f.Version != c.want || f.Path != c.machine["lint-a@"+c.want] {
					t.Fatalf("LocateGrant = %+v, %v; want the key lint-a@%s", f, err, c.want)
				}
				return
			}
			endsAs(t, err, c.says, c.is)
		})
	}
}

// TestAVersionPatternReadsTheVersion (item 8): a grant's version_pattern
// reads the version from its one group, where the first major.minor.patch
// alone would read another; a pattern that does not match is refused, and so
// is one of two groups, before anything runs.
func TestAVersionPatternReadsTheVersion(t *testing.T) {
	t.Parallel()
	root := private(t)
	prints := program(t, private(t)+"/lint-a", printing("lint-a build 9.9.9 version=1.3.0\n", 0))
	paths := map[string]map[string]string{root: {"lint-a@1.3.0": prints}}
	for name, c := range map[string]struct {
		m          plugin.Manifest
		want, says string
		is         error
	}{
		"the pattern's group":       {withPattern(foundGrant(), `version=(\d+\.\d+\.\d+)`), "1.3.0", "", nil},
		"the first triple, unasked": {foundGrant(), "", "is 9.9.9, which is outside", port.ErrStepNotReady},
		"a pattern that misses":     {withPattern(foundGrant(), `release (\d+\.\d+\.\d+)`), "", "lint-a", port.ErrRefused},
		"a pattern of two groups":   {withPattern(foundGrant(), `(\d+)\.(\d+\.\d+)`), "", "no RE2 pattern of one group", port.ErrRefused},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tl := over(paths, nil)
			f, err := tool.LocateGrant(tl, t.Context(), model.Root(root), c.m, "lint-a", false)
			if c.is == nil {
				if err != nil || f.Version != c.want {
					t.Fatalf("LocateGrant = %+v, %v; want %s", f, err, c.want)
				}
				return
			}
			endsAs(t, err, c.says, c.is)
		})
	}
}

// TestAScriptGrantRunsItsScript (item 3): the script a manifest names at
// script_at runs as the grant's interpreter, found on the PATH, with the
// script first; one outside the manifest's folder by its spelling or through
// a link is refused, and so is a manifest below a .bin folder in each
// spelling model.SameName reads as one, before anything is read.
func TestAScriptGrantRunsItsScript(t *testing.T) {
	t.Parallel()
	const pj = "node_modules/lint-a/package.json"
	// the scripts are keyed under tools, not bin, so no pointer reads as a
	// rooted path of a system (the os-knowledge gate, kind D)
	body := `{"tools": {"lint-a": "bin/lint.js", "up": "../up.js", "deep": {"x": 1}}}`
	t.Run("the script first", func(t *testing.T) {
		t.Parallel()
		root, tl := pkg(t, body)
		f, err := tool.LocateGrant(tl, t.Context(), model.Root(root), scriptGrant(pj, "/tools/lint-a"), "lint-a", false)
		script := root + "/node_modules/lint-a/bin/lint.js"
		if err != nil || f.Path != root+"/runtime/"+proc.LocalFileName("node") || f.Script != script || f.Version != "1.2.3" || f.Step != "grant" {
			t.Fatalf("LocateGrant = %+v, %v; want node and the script %s", f, err, script)
		}
	})
	for name, c := range map[string]struct{ manifest, pointer, link, says string }{
		"outside the folder by its spelling": {pj, "/tools/up", "", "which lies outside the manifest's folder"},
		"outside the folder through a link":  {pj, "/tools/lint-a", "link", "and it is a link to"},
		"a pointer at nothing":               {pj, "/tools/none", "", "holds nothing at its script_at"},
		"a pointer at no string":             {pj, "/tools/deep", "", "holds no string at its script_at"},
		"below .bin":                         {"node_modules/.bin/package.json", "/tools/lint-a", "", "a .bin folder"},
		"below .BIN":                         {"node_modules/.BIN/package.json", "/tools/lint-a", "", "a .bin folder"},
		"below .Bin":                         {"node_modules/.Bin/package.json", "/tools/lint-a", "", "a .bin folder"},
		"below .bin.":                        {"node_modules/.bin./package.json", "/tools/lint-a", "", "a .bin folder"},
		"a manifest that climbs":             {"../package.json", "/tools/lint-a", "", "outside"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, tl := pkg(t, body)
			if c.link != "" {
				out := root + "/outside.js"
				if err := os.WriteFile(out, []byte("// out\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				l := root + "/node_modules/lint-a/bin/lint.js"
				if err := os.Remove(l); err != nil {
					t.Fatal(err)
				}
				testenv.Link(t, out, l)
			}
			_, err := tool.LocateGrant(tl, t.Context(), model.Root(root), scriptGrant(c.manifest, c.pointer), "lint-a", false)
			if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("LocateGrant = %v, want refused saying %q", err, c.says)
			}
			if len(tl.Ran()) != 0 {
				t.Errorf("a refused script was run: %+v", tl.Ran())
			}
		})
	}
}
