package journeys

import (
	"fmt"
	"slices"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// The pty journeys run the built binary under a pseudo-terminal, as a person at
// a terminal runs it: the banner and colour mode a terminal gets, the menu a
// terminal can answer, a screen of cols columns. What a person sees at the end
// (the page the program left, with every cursor move applied and no colour) is
// the golden, written only with -update by a person who reads the diff
// (cli.commands.15, tests.generated.5), at 80 columns, and no line of it may
// run past the screen's edge. At 60 and 40 columns the same journeys are held
// to properties against that golden, not to goldens of their own
// (pty_narrow_test.go; tests.journeys.8).

// ptyRunOf runs the case at cols columns and answers what it saw, the
// transcript scrubbed. settled, when it is not nil, answers of a picture's title and
// the screen, scrubbed as the transcript is, "" when the screen is the one that
// picture is to take, and otherwise what it still lacks: each step that takes a picture waits for ""; on
// top of the text it waits for.
func ptyRunOf(t *testing.T, c ptyCase, cols int, settled func(title, screen string) string) ptyResult {
	t.Helper()
	w := c.world(t)
	steps := slices.Clone(c.steps)
	if settled != nil {
		for i := range steps {
			if title := steps[i].snap; title != "" {
				steps[i].until = func(screen string) string { return settled(title, w.scrub(screen)) }
			}
		}
	}
	r := runPTY(t, ptyJourney{dir: w.dir, env: w.env, cols: cols, args: c.args, steps: steps})
	if r.code != c.code {
		t.Fatalf("exit %d, want %d\n%s", r.code, c.code, r.transcript)
	}
	r.transcript = w.scrub(r.transcript)
	return r
}

// runJourney runs every case of one journey at 80 columns against its golden.
func runJourney(t *testing.T, journey string) {
	t.Helper()
	ran := 0
	for _, c := range ptyCases() {
		if c.journey != journey {
			continue
		}
		ran++
		t.Run(c.name+"-80", func(t *testing.T) {
			r := ptyRunOf(t, c, 80, goldenSettled(t, c))
			for _, f := range overrunFindings(80, r.overruns) {
				t.Error(f)
			}
			uitest.Golden(t, "pty-"+c.name+"-80", r.transcript)
		})
	}
	if ran == 0 {
		t.Fatalf("no case for the %s journey: the table was emptied", journey)
	}
}

// TestThePtyFirstRun: the bare command in a folder that is neither a rulebook
// nor a project draws the banner and the menu of what fits there, and backing
// out of it ends the run with nothing said. Risk: a first run that shows a
// person the wrong first thing, or a menu a terminal cannot leave.
func TestThePtyFirstRun(t *testing.T) {
	t.Parallel()
	runJourney(t, "first-run")
}

// TestThePtyHelp: the help a person gets from a terminal, with the banner a
// terminal gets and the pipe does not. Risk: the help breaks at a width or
// loses its banner.
func TestThePtyHelp(t *testing.T) {
	t.Parallel()
	runJourney(t, "help")
}

// TestThePtyMenuInEachContext: the menu in a rulebook, in a project and in a
// folder that is both leads with what fits the place. Risk: a menu that offers
// a command that cannot run where the person is.
func TestThePtyMenuInEachContext(t *testing.T) {
	t.Parallel()
	runJourney(t, "menu")
}

// TestThePtyHappyPath: the journal lists a closed run after the write path made
// it. Risk: the list a person reads after a write.
func TestThePtyHappyPath(t *testing.T) {
	t.Parallel()
	runJourney(t, "happy-path")
}

// TestThePtyRefusal: restoring a run is refused after a file it changed has
// moved on, and the refusal names the file and ends with exit 5, the refused word. Risk: a
// restore that overwrites later work.
func TestThePtyRefusal(t *testing.T) {
	t.Parallel()
	runJourney(t, "refusal")
}

// TestThePtyDecisionPrompt: term retire with no decision asks for its title
// and its text at a terminal, in rule retire's two questions, each naming the
// flag that answers it in a script; a question left empty ends declined,
// naming it. Risk: term retire refusing a person who could have
// answered, or asking in a shape of its own.
func TestThePtyDecisionPrompt(t *testing.T) {
	t.Parallel()
	runJourney(t, "prompt")
}

// TestEveryPtyJourneyFailsOnAPlantedExtraLine: each journey's transcript, with
// one line added, no longer matches its golden at 80 columns, and the clean
// transcript does. The plant is the extra line; the control is the clean run.
func TestEveryPtyJourneyFailsOnAPlantedExtraLine(t *testing.T) {
	t.Parallel()
	for _, c := range ptyCases() {
		t.Run(c.name, func(t *testing.T) {
			got := ptyRunOf(t, c, 80, goldenSettled(t, c)).transcript
			if !matchesGolden(t, "pty-"+c.name+"-80", got) {
				t.Fatalf("control: the clean transcript does not match its golden")
			}
			if matchesGolden(t, "pty-"+c.name+"-80", got+"a planted line\n") {
				t.Errorf("a transcript with an extra line matched its golden")
			}
		})
	}
}

// matchesGolden is whether got is the golden of name, read and never written.
func matchesGolden(t *testing.T, name, got string) bool {
	t.Helper()
	return goldenOf(t, name) == got
}

// overrunFindings is a finding for each line drawn past the screen's edge.
func overrunFindings(cols int, over []overrun) []string {
	var out []string
	for _, o := range over {
		out = append(out, fmt.Sprintf("at %d columns, a line is wider than the screen: row %d of the page (%q) was drawn %d columns wide, %d past the edge",
			cols, o.row, o.text, o.width, o.width-cols))
	}
	return out
}
