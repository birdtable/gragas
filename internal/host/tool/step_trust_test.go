package tool_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// TestAStepsToolOthersCouldSwapIsRefusedAndNotPassedOver (replaces
// TestAToolOthersCouldSwapIsRefusedAndNotPassedOver and
// TestAToolsFileToolOthersCouldSwapIsRefused): Bind refuses a program its
// group or others can write, one its owner cannot execute, a folder above it
// they can write, and the same through a link, as a trust failure with the
// line that fixes it (the trust walk's own, walkFix), and does not try the
// next place: the settings row refused with the grant's own place and the
// tools file's both sound, the grant's own place refused with the tools
// file's sound, and a tools file line refused alone, each start nothing. The
// controls: the same at 0700, and a link to it.
// Planted: reach answering not ok for the walk's refusal, so the next place
// is tried (every row red, the next place being found and asked its version).
func TestAStepsToolOthersCouldSwapIsRefusedAndNotPassedOver(t *testing.T) {
	t.Parallel()
	type who = []trusttest.Who
	group, others := who{trusttest.GroupWrites}, who{trusttest.OthersWrite}
	cases := map[string]struct {
		file, dir who
		noExec    bool
		link      bool
	}{
		"the file its group can write":         {file: group},
		"the file others can write":            {file: others},
		"a file its owner cannot execute":      {noExec: true},
		"the folder above its group can write": {dir: group},
		"through a link, the file":             {file: who{trusttest.GroupWrites, trusttest.OthersWrite}, link: true},
		"through a link, the folder above":     {dir: others, link: true},
	}
	// where is the place the unsafe program is named at, and the others
	// beside it are sound programs the find must not fall through to.
	for _, where := range []string{"settings", "own", "tools"} {
		for name, c := range cases {
			t.Run(where+", "+name, func(t *testing.T) {
				t.Parallel()
				if c.noExec && !testenv.KeepsModes(t) {
					return
				}
				root := private(t)
				store := root + "/store"
				p := program(t, store+"/lint-a", echo)
				if c.noExec {
					if err := os.Chmod(p, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				for _, w := range c.file {
					trusttest.Loosen(t, p, w)
				}
				at := p
				if c.link {
					at = root + "/node_modules/.bin/" + filepath.Base(p)
					if err := os.MkdirAll(filepath.Dir(at), 0o700); err != nil {
						t.Fatal(err)
					}
					testenv.Link(t, p, at)
				}
				for _, w := range c.dir {
					trusttest.Loosen(t, store, w)
				}
				sound := program(t, private(t)+"/lint-a", says("1.2.3"))
				m := stepManifest()
				paths, machine := map[string]map[string]string{}, map[string]string{}
				switch where {
				case "settings":
					// The settings row is refused with the grant's own place and
					// the tools file's both sound behind it.
					paths[root] = map[string]string{"lint-a@1.2.3": at}
					m = atGrant()
					program(t, root+"/"+binDir+"/lint-a", says("1.2.3"))
					machine["lint-a@1.2.3"] = sound
				case "own":
					// The grant's own place is refused with the tools file's sound.
					rel, err := filepath.Rel(root, at)
					if err != nil {
						t.Fatal(err)
					}
					m = atGrant()
					m.Programs[0].At.Path = filepath.ToSlash(rel)
					machine["lint-a@1.2.3"] = sound
				case "tools":
					machine["lint-a@1.2.3"] = at
				}
				m.Tool = "lint-a"
				tl := stepOver(paths, machine)
				s := stepScope(t, root, where != "tools")
				s.Manifest = m
				_, err := tl.Bind(t.Context(), s)
				r, ok := errors.AsType[*tool.Refused](err)
				if fix := walkFix(t, at); !ok || !errors.Is(err, port.ErrRefused) || r.Hint() != fix {
					t.Errorf("Bind = %v, want refused with %q", err, fix)
				}
				if len(tl.Ran()) != 0 {
					t.Errorf("a refused tool was run, or the next place was: %+v", tl.Ran())
				}
			})
		}
	}
	t.Run("the control, the same at 0700 and a link to it", func(t *testing.T) {
		t.Parallel()
		root := private(t)
		p := program(t, root+"/store/lint-a", says("1.2.3"))
		at := root + "/node_modules/.bin/lint-a"
		if err := os.MkdirAll(filepath.Dir(at), 0o700); err != nil {
			t.Fatal(err)
		}
		testenv.Link(t, p, at)
		for _, set := range []string{p, at} {
			tl := stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": set}}, nil)
			if r, err := tl.Bind(t.Context(), stepScope(t, root, true)); err != nil || r.Tool().Path != p {
				t.Errorf("the control at %s: %+v, %v; want the file %s", set, r, err, p)
			}
		}
	})
}
