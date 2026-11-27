package root

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/exit"
)

// TestTheNoStateFolderRefusalNamesTheVariablesAppdirsGives: the words of the
// refusal are config.StateWords' own, and carry no shell line.
func TestTheNoStateFolderRefusalNamesTheVariablesAppdirsGives(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "gen", "--apply")
	p.Config.StateDir = ""
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.NotReady {
		t.Fatalf("exit %d, want not ready; said:\n%s", code, said)
	}
	state, home := config.StateWords()
	for _, w := range []string{state, home} {
		if w != "" && !strings.Contains(said, w) {
			t.Errorf("the refusal does not name %q:\n%s", w, said)
		}
	}
	if strings.Contains(said, "export") {
		t.Errorf("the refusal holds a shell line:\n%s", said)
	}
}

func TestTheHelpsVariableColumnHoldsTheName(t *testing.T) {
	t.Parallel()
	if got := stateVarHelp(); !strings.HasPrefix(got, helpVar()+" ") || len(got) < 19 {
		t.Errorf("stateVarHelp() = %q for %q", got, helpVar())
	}
}
