package journeys

import (
	"strings"
	"testing"
)

// TestThePtyOpensAndShowsTheBannerOnEverySystem: the binary run under
// go-console's pseudo-terminal, a pty on linux and darwin and a ConPTY on
// windows, draws the banner a terminal gets and ends with exit 0. It holds no
// golden, so when a journey's golden differs this says whether the pty itself
// works. Risk: a pty that opens and shows nothing, so that every journey on a
// system waits out its time.
func TestThePtyOpensAndShowsTheBannerOnEverySystem(t *testing.T) {
	t.Parallel()
	env, _ := folders(t)
	r := runPTY(t, ptyJourney{
		dir:   t.TempDir(),
		env:   env,
		cols:  80,
		args:  []string{"--help"},
		steps: []ptyStep{{waitFor: "the rulebook, as presets you import"}},
	})
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\n%s", r.code, r.transcript)
	}
	if !strings.Contains(r.transcript, "the rulebook, as presets you import") {
		t.Errorf("the banner is not on the screen the program left:\n%s", r.transcript)
	}
}

// TestAPluginApproveOnATerminalPrintsNoBanner: a command that is not help and
// not the bare name starts with its own output, in a terminal that paints
// A script of five commands drew five banners before. Risk: a
// banner back on every command.
func TestAPluginApproveOnATerminalPrintsNoBanner(t *testing.T) {
	t.Parallel()
	env, _ := folders(t)
	r := runPTY(t, ptyJourney{
		dir:   t.TempDir(),
		env:   env,
		cols:  80,
		args:  []string{"plugin", "approve", "nosuch"},
		steps: []ptyStep{{waitFor: "nosuch"}},
	})
	for _, banner := range []string{"the rulebook, as presets you import", "v1.2.3 (abc1234)"} {
		if strings.Contains(r.transcript, banner) {
			t.Errorf("plugin approve drew the banner (%q):\n%s", banner, r.transcript)
		}
	}
}
