package journeys

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestTheBuiltNamesToolTakesTheAccountsHomeFromTheSystem: main() hands the run the system's record of the account's
// home, and nothing but the built program shows it. The run is the isolated
// one, whose home is a folder of the test's own, not the account's: the tool
// ends not ready (exit 4) and says to unset the variable, and it does not end
// in a panic. Risk: red if main() loses its AccountHome (a nil call ends in a
// panic, exit 2). This journey holds that one case only. A SystemHome that
// answers the variable is held by TestSystemHomeIsTheSystemsAnswerAndNotTheVariable
// (internal/config) and by the pointer check of TestMainWiresTheSystemsHomeAndTheOwnedRead
// (cmd/gragas-names), not by this one (on).
func TestTheBuiltNamesToolTakesTheAccountsHomeFromTheSystem(t *testing.T) {
	t.Parallel()
	built := testenv.Build("cmd/gragas-names").Trimpath()
	bin := built.Path()
	if bin == "" {
		t.Fatalf("could not build gragas-names:\n%s", configtest.GoSaid(built.Run().Stderr))
	}
	out, err := commandOf(t, bin, t.TempDir(), nil, "-not-asked").CombinedOutput()
	code := 0
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("could not run gragas-names: %v", err)
	}
	if code != 4 || !strings.Contains(string(out), "unset") || strings.Contains(string(out), "panic") {
		t.Errorf("exit %d, want 4 and the variable to unset:\n%s", code, out)
	}
}
