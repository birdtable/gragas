package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// TestPluginApproveRefusalsAndScreenGoldens: the approve screen and each of its
// refusals as a pipe at 80 and 40 columns, each against its golden. A refusal
// ends on the line to type; the no-digest text has no empty slot; every grant
// is a block of rows that holds its versions, version asked and command line
// at both widths. Risk: a refusal with no next line, a digest
// slot printed empty, or a grant column dropped on a narrow screen.
func TestPluginApproveRefusalsAndScreenGoldens(t *testing.T) {
	t.Parallel()
	dir, ds := pluginRulebook(t,
		lockRow{name: "lint-a", version: "1.0.0", source: "../lint-a"},
		lockRow{name: "lint-a", version: "1.1.0", source: "../lint-a"},
		lockRow{name: "lint-b", version: "1.0.0", source: "../lint-b"})
	env, _ := folders(t)
	b := filepath.Join(dir, "plugins", "lint-b", "1.0.0", "plugin.yaml")
	body, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(strings.Replace(string(body), `"warning": true`, `"warning": false`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	wrong := strings.Repeat("0", len(ds["lint-a@1.0.0"]))
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"screen", []string{"plugin", "approve", "lint-a@1.0.0"}, 0},
		{"refused-no-digest", []string{"plugin", "approve", "lint-a@1.0.0", "--apply"}, -1},
		{"refused-wrong-digest", []string{"plugin", "approve", "lint-a@1.0.0", "--digest", wrong, "--apply"}, -1},
		{"refused-two-versions", []string{"plugin", "approve", "lint-a"}, -1},
		{"refused-unknown", []string{"plugin", "approve", "lint-z"}, -1},
		{"refused-changed-bundle", []string{"plugin", "approve", "lint-b@1.0.0"}, -1},
	}
	for _, cols := range []string{"80", "40"} {
		for _, c := range cases {
			r := run(t, dir, map[string]string{"COLUMNS": cols, stateKey: env[stateKey], cacheKey: env[cacheKey]}, c.args...)
			if (c.code == 0) != (r.code == 0) {
				t.Fatalf("%s at %s: exit %d\n%s", c.name, cols, r.code, transcript(c.args, r))
			}
			tr := scrub(transcript(c.args, r), dir)
			for k, v := range ds {
				tr = strings.ReplaceAll(tr, v, "<"+k+">")
			}
			uitest.Golden(t, "plugin-approve-"+c.name+"-"+cols, strings.ReplaceAll(tr, wrong, "<wrong>"))
		}
	}
}
