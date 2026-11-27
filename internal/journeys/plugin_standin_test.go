package journeys

import (
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// TestPluginOnStandinRefusesWhatTheHostWillNotOpen: the standin plugin, built
// from this tree and run by no tool of the network, is refused with its closing
// words at 80 and 40 columns in each case the host holds it back: added and not
// approved, locked and approved on another machine (a clone), a manifest of
// another contract (refused at add, so nothing is installed), and a manifest
// that needs a feature the engine lacks. Each ends on the line to type or on
// the reason, and writes nothing. Risk: a plugin that loads without the yes of
// this machine, or a refusal that names no next step.
func TestPluginOnStandinRefusesWhatTheHostWillNotOpen(t *testing.T) {
	t.Parallel()
	record := []string{"plugin", "record", authorFolder, "--op", "entries", "--apply"}
	add := []string{"plugin", "add", authorFolder, "--apply"}
	for _, tc := range []struct {
		name  string
		opts  recordOpts
		clone bool
		args  []string
		code  int
		wants []string
	}{
		{"plugin-standin-not-approved", recordOpts{unapproved: true}, false, record, 5, []string{"not approved", "gragas plugin approve standin@1.0.0"}},
		{"plugin-standin-a-clone", recordOpts{}, true, record, 5, []string{"not approved", "gragas plugin approve standin@1.0.0"}},
		{"plugin-standin-wrong-contract", recordOpts{contract: "2", unadded: true}, false, add, 1, []string{"contract 2", "engine's is 1"}},
		{"plugin-standin-missing-feature", recordOpts{needs: "[nonesuch]"}, false, record, 1, []string{"needs a feature the engine lacks", "nonesuch"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, env := recordProject(t, tc.opts)
			if tc.clone {
				fresh, _ := folders(t)
				env[stateKey], env[cacheKey] = fresh[stateKey], fresh[cacheKey]
			}
			for _, cols := range []string{"80", "40"} {
				env["COLUMNS"] = cols
				r := run(t, dir, env, tc.args...)
				said := strings.Join(strings.Fields(r.stdout+r.stderr), " ")
				if r.code != tc.code {
					t.Errorf("at %s: exit %d, want %d\n%s%s", cols, r.code, tc.code, r.stdout, r.stderr)
				}
				for _, w := range tc.wants {
					if !strings.Contains(said, w) {
						t.Errorf("at %s: output lacks %q\n%s%s", cols, w, r.stdout, r.stderr)
					}
				}
				uitest.Golden(t, tc.name+"-"+cols, recordMasked(tc.args, r))
			}
			if !strings.Contains(tc.name, "contract") {
				return
			}
			if got := run(t, dir, env, "plugin", "list"); !strings.Contains(got.stdout, "no plugin") && strings.Contains(got.stdout, "standin") {
				t.Errorf("a refused add left a plugin listed\n%s", got.stdout)
			}
		})
	}
}
