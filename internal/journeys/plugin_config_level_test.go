package journeys

import (
	"slices"
	"strings"
	"testing"
)

// TestAManifestWhoseConfigLoadsCodeRaisesAJSONConfigsReadToSandboxed: the standin, its manifest without config_loads_code, reads a
// standin.json config isolated, as a data name gives, and the import's dry run
// names the config's entries; with config_loads_code: true the same read asks
// the sandboxed level, which this build has no adapter for, so the import ends
// failed saying the scratch folder could not be made, and writes nothing though
// --apply is given. Risk: a flag that does not tighten (a data config whose tool
// loads code read isolated), or a manifest without it whose reads change.
func TestAManifestWhoseConfigLoadsCodeRaisesAJSONConfigsReadToSandboxed(t *testing.T) {
	t.Parallel()
	args := importArgs("--tool", "standin", "--target", "source", "--config", "standin.json")
	for _, tc := range []struct {
		name  string
		loads bool
		args  []string
		code  int
		wants []string
		not   string
	}{
		{"without-the-flag", false, args, 6, []string{"the entry errcheck of standin.json", "the entry vet of standin.json", "Add --apply"}, "could not be made"},
		{"with-the-flag", true, append(slices.Clip(args), "--apply"), 1, []string{"the scratch folder for standin could not be made", "the sandboxed level"}, "errcheck"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, env := recordProject(t, recordOpts{loadsCode: tc.loads})
			guideInto(t, dir)
			put(t, dir, map[string]string{"standin.json": "errcheck\nvet warning\n"})
			env["COLUMNS"] = "200"
			r := run(t, dir, env, tc.args...)
			said := strings.Join(strings.Fields(r.stdout+r.stderr), " ")
			if r.code != tc.code || strings.Contains(said, tc.not) || histN(t, dir) != 0 {
				t.Errorf("exit %d (want %d), history %d (want 0), output holds %q: %v\n%s%s", r.code, tc.code, histN(t, dir), tc.not, strings.Contains(said, tc.not), r.stdout, r.stderr)
			}
			for _, w := range tc.wants {
				if !strings.Contains(said, w) {
					t.Errorf("output lacks %q\n%s%s", w, r.stdout, r.stderr)
				}
			}
		})
	}
}
