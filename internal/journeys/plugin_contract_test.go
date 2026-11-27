package journeys

import (
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// TestPluginContractPrintsTheNumberInAFolderWithNothingInIt: in an empty
// folder, with no rulebook, no plugin and no approval, the plain form is one
// line and --json is one document held by a golden, the same at any width.
// Risk: the print reading anything but the codec's value.
func TestPluginContractPrintsTheNumberInAFolderWithNothingInIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"plugin-contract", []string{"plugin", "contract"}},
		{"json-plugin-contract", []string{"plugin", "contract", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, dir, nil, tc.args...)
			if r.code != 0 {
				t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name, scrub(r.stdout, ""))
		})
	}
}
