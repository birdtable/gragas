package tool_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// TestAStepsPinnedPlaceThatEndsBelowTheRootIsPassedOver (replaces
// TestAPinnedPlaceThatEndsBelowTheRootIsPassedOver and the settings
// row of TestAPinnedFindStartsNothingTheProjectHolds): Bind of a zero Scope
// passes over a settings row or a tools file line whose program lies at or
// below the root, by its own spelling, through a link from outside, or with
// the root itself spelled through a link, and names it; nothing is started,
// and with no other place the step is not ready. The controls: a settings row
// outside the root is found, asked its version and run, and the same program
// below the root is taken by OwnPlace, which holds no root.
// Planted: passedOver answering false (each row red, as the program the
// project holds is asked its version).
func TestAStepsPinnedPlaceThatEndsBelowTheRootIsPassedOver(t *testing.T) {
	t.Parallel()
	real := private(t)
	outside := private(t)
	own := program(t, real+"/tools/lint-a", marking(real+"/tools/lint-a"))
	testenv.Link(t, own, outside+"/lint-a")
	testenv.Link(t, real, outside+"/rt")
	for name, c := range map[string]struct {
		root            string
		settings, tools string
	}{
		"a settings row below the root":                 {root: real, settings: own},
		"a settings row linked into the root":           {root: real, settings: outside + "/lint-a"},
		"a tools file line linked into the root":        {root: real, tools: outside + "/lint-a"},
		"a settings row below a root spelled by a link": {root: outside + "/rt", settings: own},
		"a tools file line below the root":              {root: real, tools: own},
	} {
		// Not subtests: the rows share the program's marker, which each clears.
		func() {
			if err := os.Remove(own + ".started"); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			var paths map[string]map[string]string
			if c.settings != "" {
				paths = map[string]map[string]string{c.root: {"lint-a@1.2.3": c.settings}}
			}
			var machine map[string]string
			if c.tools != "" {
				machine = map[string]string{"lint-a@1.2.3": c.tools}
			}
			tl := stepOver(paths, machine)
			_, err := tl.Bind(t.Context(), stepScope(t, c.root, false))
			endsAs(t, err, "a pinned find passes it over", port.ErrStepNotReady)
			if len(tl.Ran()) != 0 || asked(own) {
				t.Errorf("%s: %d runs started, asked: %v; want none", name, len(tl.Ran()), asked(own))
			}
		}()
	}
	t.Run("the control, a settings row outside the root", func(t *testing.T) {
		t.Parallel()
		root := private(t)
		set := program(t, private(t)+"/lint-a", marking(outside+"/set"))
		tl := stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": set}}, nil)
		r, err := tl.Bind(t.Context(), stepScope(t, root, false))
		if err != nil || r.Tool().Path != set || r.Tool().Step != string(tool.StepSettings) {
			t.Fatalf("Bind = %v, %+v; want the settings row, found", err, r)
		}
		if _, err := stepRunOf(t, r); err != nil {
			t.Errorf("the step's run: %v", err)
		}
		if ran := tl.Ran(); len(ran) != 2 || ran[1].Argv[0] != set || ran[1].Shape != "step" {
			t.Errorf("runs = %+v; want the version run and the step, of %s", ran, set)
		}
	})
	t.Run("the control, OwnPlace holds no root", func(t *testing.T) {
		t.Parallel()
		root := private(t)
		in := program(t, root+"/tools/lint-a", marking(root+"/tools/lint-a"))
		tl := stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": in}}, nil)
		if r, err := tl.Bind(t.Context(), stepScope(t, root, true)); err != nil || r.Tool().Path != in {
			t.Errorf("Bind under OwnPlace = %v; want the program below the root", err)
		}
	})
}

// TestAStepsPinnedPathThatKeepsNoFolderStartsNothing (replaces
// TestAPinnedRunWhosePATHKeepsNoFolderStartsNothing and the empty-PATH
// rows of TestAPinnedRunReadsNoPATHFolderInTheProject): a zero
// Scope's Bind on a PATH that is empty, or holds the project's folders alone,
// is not ready, names the PATH, and starts nothing, so no run falls back to a
// default PATH and the folder it runs in; and a step of a runner bound on a
// PATH whose one folder is a link outside the root, once the link is turned
// to a folder below the root, is refused, naming the PATH, with nothing
// started and the program the project holds never run. Planted: within.empty
// answering false (the Bind rows red), and the same in run (the step row red).
func TestAStepsPinnedPathThatKeepsNoFolderStartsNothing(t *testing.T) {
	t.Parallel()
	root := private(t)
	outside := private(t)
	mark := root + "/marked"
	built(t, root+"/"+binDir+"/marknode", configtest.Step{Write: mark, Text: "x"}, lintVersion())
	built(t, root+"/marknode", configtest.Step{Write: mark, Text: "x"}, lintVersion())
	built(t, outside+"/good/marknode", lintVersion())
	native := built(t, outside+"/native/lint-a", configtest.Step{Run: []string{"marknode"}}, lintVersion())
	testenv.Link(t, root+"/"+binDir, outside+"/to-bin")
	testenv.Link(t, root, outside+"/to-root")
	marked := func(claim string) {
		t.Helper()
		if _, err := os.Stat(mark); err == nil {
			t.Errorf("%s: the program the project holds was started", claim)
			if err := os.Remove(mark); err != nil {
				t.Fatal(err)
			}
		}
	}
	paths := map[string]map[string]string{root: {"lint-a@1.2.3": native}}
	for _, p := range []string{root + "/" + binDir, pathList(outside+"/to-bin", "bin", outside+"/to-root/none"), ""} {
		claim := "a PATH of the project's folders alone, " + strconv.Quote(p)
		tl := stepOn(p, paths, nil)
		_, err := tl.Bind(t.Context(), stepScope(t, root, false))
		endsAs(t, err, "keeps no folder outside the project", port.ErrStepNotReady)
		if !strings.Contains(err.Error(), strconv.Quote(p)) && !strings.Contains(err.Error(), p) {
			t.Errorf("%s: Bind = %v; want it to name the PATH", claim, err)
		}
		if len(tl.Ran()) != 0 {
			t.Errorf("%s: Bind started %+v", claim, tl.Ran())
		}
		marked(claim)
	}
	swing := outside + "/swing"
	testenv.Link(t, outside+"/good", swing)
	tl := stepOn(swing, paths, nil)
	r, err := tl.Bind(t.Context(), stepScope(t, root, false))
	if err != nil {
		t.Fatalf("Bind on a PATH of a link outside the root = %v; want it bound", err)
	}
	if _, err := stepRunOf(t, r); err != nil {
		t.Fatalf("the control, a step with the link outside the root: %v; want it run", err)
	}
	if err := os.Remove(swing); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, root+"/"+binDir, swing)
	before := len(tl.Ran())
	_, err = stepRunOf(t, r)
	endsAs(t, err, "keeps no folder outside the project", port.ErrRefused)
	if !strings.Contains(err.Error(), swing) {
		t.Errorf("a step once the PATH's one folder is a link into the root: %v; want it to name the PATH", err)
	}
	if len(tl.Ran()) != before {
		t.Errorf("a refused step started %+v", tl.Ran()[before:])
	}
	marked("a step once the PATH's one folder is a link into the root")
}

// TestAStepsPinnedPathLeavesOutEveryFolderTheProjectCouldHold (replaces
// TestAPinnedRunReadsNoPATHFolderInTheProject, the PATH of
// pinned_test.go and the quote rows of
// TestThePinnedPathWithEveryFolderLeftOutIsRefused): a pinned tool's version
// run and its steps are given the PATH with each folder left out that lies
// below the root, is reached through a link, is not there below a link to the
// root, is relative or has a .. segment, lies below a folder its group can
// write, or is a link through one to a trusted folder, so a program the step
// or an env starts by name, marknode, is the one outside the project: the
// step's own PATH is the one folder kept, and the mark the project's marknode
// would leave is never left. The program is a built one and a script that
// starts env and the name. The control: the marking folder removed, and
// OwnPlace, which is given the whole PATH and starts the project's marknode.
// Planted: pinnedPath keeping every folder (each row red, by the PATH the
// step prints and the mark), and the folder compared with the root by
// spelling alone (the link and chain rows red).
func TestAStepsPinnedPathLeavesOutEveryFolderTheProjectCouldHold(t *testing.T) {
	t.Parallel()
	root := private(t)
	outside := private(t)
	mark := root + "/marked"
	built(t, root+"/"+binDir+"/marknode", configtest.Step{Write: mark, Text: "x"}, lintVersion())
	built(t, outside+"/good/marknode", lintVersion())
	good := outside + "/good"
	// The version run starts marknode by the PATH, then says its version first; a
	// step starts it again and then prints the PATH it was given as its last line.
	second := built(t, outside+"/second/lint-a", configtest.Step{Run: []string{"marknode"}}, lintVersion(), configtest.Vars("PATH"))
	env := script(t, outside+"/tools/lint-a", viaEnv(t, "marknode"))
	testenv.Link(t, root+"/"+binDir, outside+"/to-bin")
	testenv.Link(t, root, outside+"/to-root")
	built(t, outside+"/open/bin/marknode", configtest.Step{Write: mark, Text: "x"})
	trusttest.Loosen(t, outside+"/open", trusttest.GroupWrites)
	built(t, outside+"/mk1/marknode", configtest.Step{Write: mark, Text: "x"})
	built(t, outside+"/mk2/marknode", configtest.Step{Write: mark, Text: "x"})
	testenv.Link(t, outside+"/mk1", outside+"/open/lnk")
	testenv.Link(t, outside+"/mk2", root+"/hop")
	testenv.Link(t, root+"/hop", outside+"/chain")
	marked := func(claim string) {
		t.Helper()
		if _, err := os.Stat(mark); err == nil {
			t.Errorf("%s: the program the project holds was started", claim)
			if err := os.Remove(mark); err != nil {
				t.Fatal(err)
			}
		}
	}
	sep := string(filepath.ListSeparator)
	for _, c := range []struct{ claim, path string }{
		{"the PATH's first folder below the root", pathList(root+"/"+binDir, good)},
		{"a link to a folder below the root", pathList(outside+"/to-bin", good)},
		{"a folder not there, below a link to the root", pathList(outside+"/to-root/none", good)},
		{"a folder with a .. segment", pathList(outside+"/to-root/none/../bin", good)},
		{"a relative entry", pathList("bin", good)},
		{"a folder below one its group can write", pathList(outside+"/open/bin", good)},
		{"a link in a folder its group can write, to a trusted folder", pathList(outside+"/open/lnk", good)},
		{"a chain of links through the root to a trusted folder", pathList(outside+"/chain", good)},
		{"a folder that holds a double quote", pathList(outside+"\"b", good)},
		{"a quoted element that holds the separator", "\"" + outside + sep + "b\"" + sep + good},
		{"the control, the marking folder removed", good},
	} {
		for _, viaScript := range []bool{false, true} {
			prog, claim := second, c.claim+", a built program"
			if viaScript {
				prog, claim = env, c.claim+", a script"
			}
			paths := map[string]map[string]string{root: {"lint-a@1.2.3": prog}}
			tl := stepOn(c.path, paths, nil)
			r, err := tl.Bind(t.Context(), stepScope(t, root, false))
			if viaScript && startsNothing(t, prog) {
				notStarted(t, claim, err)
				marked(claim + ", the version run")
				continue
			}
			if err != nil || r.Tool().Path != prog {
				t.Errorf("%s: Bind = %v, %+v; want the tool found with the project's folders left out", claim, err, r.Tool())
				continue
			}
			marked(claim + ", the version run")
			res, err := stepRunOf(t, r)
			if err != nil {
				t.Errorf("%s: the step: %v; want it run", claim, err)
				continue
			}
			marked(claim + ", the step")
			if !viaScript {
				lines := strings.Split(strings.TrimSpace(text(t, res.Stdout)), "\n")
				// A PATH is in the system's form, so good is spelt so too.
				if got, want := lines[len(lines)-1], filepath.FromSlash(good); got != want {
					t.Errorf("%s: the step's PATH is %q; want the one folder %q", claim, got, want)
				}
			}
		}
	}
	t.Run("the control, OwnPlace is given the whole PATH", func(t *testing.T) {
		t.Parallel()
		tl := stepOn(pathList(root+"/"+binDir, good), map[string]map[string]string{root: {"lint-a@1.2.3": second}}, nil)
		r, err := tl.Bind(t.Context(), stepScope(t, root, true))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(mark); err != nil {
			t.Errorf("OwnPlace: %v; want the project's marknode run, as an unpinned find may", err)
		}
		if err := os.Remove(mark); err != nil {
			t.Fatal(err)
		}
		res, err := stepRunOf(t, r)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(text(t, res.Stdout)), "\n")
		if got := filepath.SplitList(lines[len(lines)-1]); !slices.Contains(got, filepath.FromSlash(root+"/"+binDir)) {
			t.Errorf("OwnPlace: the step's PATH is %q; want the project's folder in it", got)
		}
	})
}
