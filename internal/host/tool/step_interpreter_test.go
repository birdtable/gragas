package tool_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// bindScript is Bind of a read whose tool lint-a is the script at, named in
// the settings for root, on the PATH path, with the grant's own place taken
// (OwnPlace) as an unpinned find takes it: the whole PATH, no root held.
func bindScript(t *testing.T, root, at, path string) (*tool.Tool, error) {
	t.Helper()
	tl := stepOn(path, map[string]map[string]string{root: {"lint-a@1.2.3": at}}, nil)
	_, err := tl.Bind(t.Context(), stepScope(t, root, true))
	return tl, err
}

// TestAStepsScriptsInterpreterIsHeldToTheSameWalkAsTheScript (replaces
// TestAScriptsInterpreterIsHeldToTheSameWalkAsTheScript): a
// tool that is a script starting /usr/bin/env and a name runs the first
// program of that name on the PATH a run is given, so Bind judges that program
// like the script; one a group can write, an env option, a relative
// interpreter, a name on no folder of the PATH, an empty or relative entry of
// the PATH and a .. after a link each refuse the tool by name, a shim's
// interpreter is judged too, and a trusted interpreter is the control that is
// found. Planted: follow taking the interpreter with no walk (the
// group-writable rows red), pathFolderHeld taking a relative entry (the empty
// and relative entry rows red), and hop passing a line the system would not
// run (the relative interpreter row red).
func TestAStepsScriptsInterpreterIsHeldToTheSameWalkAsTheScript(t *testing.T) {
	t.Parallel()
	root := private(t)
	built(t, root+"/good/node", lintVersion())
	built(t, root+"/bad/node", lintVersion())
	trusttest.Loosen(t, root+"/bad", trusttest.GroupWrites)
	script(t, root+"/shim/node", viaEnv(t, "bash"))
	built(t, root+"/bad/bash", lintVersion())
	built(t, root+"/fine/bash", lintVersion())
	built(t, root+"/"+binDir+"/node", lintVersion())
	built(t, root+"/w/bin/node", lintVersion())
	if err := os.MkdirAll(root+"/w/sub", 0o700); err != nil {
		t.Fatal(err)
	}
	trusttest.Loosen(t, root+"/w", trusttest.GroupWrites)
	testenv.Link(t, root+"/w/sub", root+"/lnk")
	trusted := built(t, root+"/abs/sh", lintVersion())
	for _, c := range []struct {
		claim, first, path string
		want               string // "" for found, else a part of the refusal
	}{
		{"a trusted node on the PATH is found", viaEnv(t, "node"), root + "/good", ""},
		{"a node in a folder a group can write, first on the PATH, is refused", viaEnv(t, "node"), pathList(root+"/bad", root+"/good"), "the interpreter at"},
		{"a name on no folder of the PATH is refused", viaEnv(t, "node"), root + "/empty", "is in no folder of the PATH"},
		{"an option of env is refused", viaEnv(t, "-S node"), root + "/good", "only env and one program name are read"},
		{"a relative interpreter is refused", "#!node", root + "/good", "the system would not run"},
		{"an absolute interpreter a group can write is refused", "#!" + root + "/bad/node", root + "/good", "the interpreter at"},
		{"an absolute trusted interpreter is found", "#!" + trusted, "", ""},
		{"a leading empty entry of the PATH is refused", viaEnv(t, "node"), pathList("", root+"/good"), "empty or relative"},
		{"a relative entry of the PATH is refused", viaEnv(t, "node"), "." + pathList("", root+"/good"), "empty or relative"},
		{"an unclean absolute entry is judged where env reaches it", viaEnv(t, "node"), root + "/good/./", ""},
		{"an entry with a .. after a link is refused, as env reaches it through the link", viaEnv(t, "node"), root + "/lnk/../bin", "a .. segment"},
		{"a shim whose interpreter is in a folder a group can write is refused", viaEnv(t, "node"), pathList(root+"/shim", root+"/bad", root+"/fine"), "the interpreter at"},
		{"a shim whose interpreter is trusted is found", viaEnv(t, "node"), pathList(root+"/shim", root+"/fine"), ""},
	} {
		if err := os.MkdirAll(root+"/empty", 0o700); err != nil {
			t.Fatal(err)
		}
		at := script(t, root+"/tools/lint-a", c.first)
		tl, err := bindScript(t, root, at, c.path)
		if startsNothing(t, at) {
			notStarted(t, c.claim, err)
			continue
		}
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: Bind = %v; want it found", c.claim, err)
			}
			continue
		}
		if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Bind = %v; want a refusal with %q", c.claim, err, c.want)
		}
		if len(tl.Ran()) > 1 {
			t.Errorf("%s: a refused interpreter was followed by %+v", c.claim, tl.Ran())
		}
	}
}

// TestAStepsInterpreterChainIsHeldAtItsBoundary (replaces
// TestAnInterpreterChainIsHeldAtItsBoundary and the chain rows of
// TestAScriptsInterpreterIsHeldToTheSameWalkAsTheScript): a tool that is the
// first of MaxHops scripts is bound, and one that is the first of MaxHops+1 is
// refused as past the chain a walk follows, the chains counted from the files
// written. boundary: maxHops. Planted: the bound one higher (the past-four row
// red), and envTakes following the program env takes at depth 0, so the
// levels are not counted together (the same row red).
func TestAStepsInterpreterChainIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		scripts int
		want    string // "" for found
	}{
		{tool.MaxHops, ""},
		{tool.MaxHops + 1, "more than " + strconv.Itoa(tool.MaxHops) + " deep"},
	} {
		root := private(t)
		at, path, shebangs := chainOfScripts(t, root, c.scripts)
		if shebangs != c.scripts {
			t.Fatalf("a chain of %d scripts wrote %d files that start with #!", c.scripts, shebangs)
		}
		_, err := bindScript(t, root, at, path)
		if startsNothing(t, at) {
			notStarted(t, "a chain of "+strconv.Itoa(c.scripts)+" scripts", err)
			continue
		}
		if c.want == "" {
			if err != nil {
				t.Errorf("a chain of %d scripts: Bind = %v; want it found", c.scripts, err)
			}
			continue
		}
		if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("a chain of %d scripts: Bind = %v; want a refusal with %q", c.scripts, err, c.want)
		}
	}
}

// TestAStepsEnvThatIsItselfAScriptIsRefused (replaces
// TestAnEnvThatIsItselfAScriptIsRefused): a script whose interpreter is named
// env but is a script of its own starts a chain whose env is not the last
// program, so the program env takes from the PATH would go unjudged, and
// Bind refuses the tool, as the old Find did. Where the system reads no #! line
// the script is judged alone. Planted: hop passing a line the system would
// not run, by swallowing gotrust.Interpreters' error (the row red where the
// system refuses a script as an interpreter itself; chainHeld's own
// env-not-last refusal is the second guard where it reads the chain).
func TestAStepsEnvThatIsItselfAScriptIsRefused(t *testing.T) {
	t.Parallel()
	root := private(t)
	trusted := built(t, root+"/abs/sh", lintVersion())
	script(t, root+"/envs/env", "#!"+trusted)
	built(t, root+"/"+binDir+"/lint", lintVersion())
	at := script(t, root+"/tools/lint-a", "#!"+root+"/envs/env lint")
	_, err := bindScript(t, root, at, root+"/"+binDir)
	if startsNothing(t, at) {
		notStarted(t, "an env that is itself a script", err)
		return
	}
	if !errors.Is(err, port.ErrRefused) {
		t.Errorf("Bind = %v; want a refusal", err)
	}
}

// TestAStepsChainsOfEveryEnvLevelAreCountedTogether (replaces
// TestTheChainsOfEveryEnvLevelAreCountedTogether): the
// bound of maxHops is on the programs of the whole walk, not on each level's
// own chain, so Bind of a tool that starts a shim whose line starts env, whose
// program is a script that starts env in turn, is refused when the programs
// the system starts are more than four, though each level's chain is short;
// the same walk without the shim is the control that is bound. Planted: the
// depth reset to 0 at each env level in follow (red where the system reads a
// chain of two for the shim, as the old test; elsewhere the boundary test
// above is the one red), and hop swallowing the system's refusal (the shim
// row red here).
func TestAStepsChainsOfEveryEnvLevelAreCountedTogether(t *testing.T) {
	t.Parallel()
	root := private(t)
	leaf := built(t, root+"/leaf/sh", lintVersion())
	script(t, root+"/chain/n1", viaEnv(t, "n2"))
	script(t, root+"/chain/n2", viaEnv(t, "n3"))
	script(t, root+"/chain/n3", viaEnv(t, "n4"))
	script(t, root+"/chain/n4", "#!"+leaf)
	shim := script(t, root+"/shim/s", viaEnv(t, "n1"))
	find := func(first string) error {
		at := script(t, root+"/tools/lint-a", first)
		_, err := bindScript(t, root, at, root+"/chain")
		return err
	}
	// The control: env's program n2, then n3, n4 and the leaf are four programs,
	// the bound, so the walk without the shim is bound.
	if err := find(viaEnv(t, "n2")); !startsNothing(t, root+"/tools/lint-a") && err != nil {
		t.Errorf("env, n2, n3, n4 and the leaf: Bind = %v; want it found", err)
	}
	err := find("#!" + shim)
	chain, cerr := gotrust.Interpreters(filepath.FromSlash(root + "/tools/lint-a"))
	switch {
	case cerr == nil && len(chain) >= 2:
		if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "more than "+strconv.Itoa(tool.MaxHops)+" deep") {
			t.Errorf("a shim, env, n1, n2, n3, n4 and the leaf: Bind = %v; want a refusal as more than %d deep", err, tool.MaxHops)
		}
	case cerr != nil:
		if !errors.Is(err, port.ErrRefused) {
			t.Errorf("a shim the system would not run: Bind = %v; want a refusal", err)
		}
	default:
		if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "is not a program file") {
			t.Errorf("a system that starts no chain: Bind = %v; want a refusal as a file that is not a program", err)
		}
	}
}

// TestAStepsPinnedEnvIsHeldToTheRoot (replaces
// TestAPinnedFindsEnvIsHeldToTheRoot): the env hop of a pinned find is held to
// the root besides the filtered PATH. Bind on a zero Scope is given the PATH
// with the project's folders left out, so the hop never sees one (the
// script's run in TestAStepsPinnedPathLeavesOutEveryFolderTheProjectCouldHold
// is the end to end hold); the hop's own guard, which a pinned PATH cannot
// reach through Bind, is held on the interpreter check the pinned find runs
// (tool.Interpreter), over a Tool with no adapter: given a PATH whose first
// folder holding the name lies at or below the root, by its spelling or
// through a link, it refuses the tool by name; held to no root, as an unpinned
// find is, it passes. Planted: envOutside never reporting (the held rows red).
func TestAStepsPinnedEnvIsHeldToTheRoot(t *testing.T) {
	t.Parallel()
	root := private(t)
	outside := private(t)
	built(t, root+"/"+binDir+"/marknode", lintVersion())
	env := script(t, outside+"/tools/lint-a", viaEnv(t, "marknode"))
	testenv.Link(t, root+"/"+binDir, outside+"/to-bin")
	for _, p := range []string{root + "/" + binDir, outside + "/to-bin"} {
		err := tool.Interpreter(stepOn(p, nil, nil), env, p, root)
		if startsNothing(t, env) {
			if err != nil {
				t.Errorf("env over %s, held to the root: %v; want the script judged alone, as the system starts no interpreter", p, err)
			}
		} else if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "env would take its program from") {
			t.Errorf("env over %s, held to the root: %v; want the tool refused by name", p, err)
		}
		if err := tool.Interpreter(stepOn(p, nil, nil), env, p, ""); err != nil {
			t.Errorf("the control, env over %s held to no root: %v; want it passed", p, err)
		}
	}
}
