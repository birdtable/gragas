package journeys

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

// The cases of the pty journeys and the golden helpers they and their tests read: no terminal and no
// pseudo-terminal in any of them, so they build on every host and the settle tests of pty_settle_test.go
// run native on Windows too. The journeys themselves, which run the binary under a
// pseudo-terminal, are pty_journeys_test.go's.

// ptyWorld is the place a pty journey runs in: the folder, the environment on
// top of the isolated one, and what hides what moves with the machine.
type ptyWorld struct {
	dir   string
	env   map[string]string
	scrub func(string) string
}

// ptyCase is one run of a journey.
type ptyCase struct {
	journey string // which of the six
	name    string // the golden's name before the width
	args    []string
	steps   []ptyStep
	code    int
	world   func(t *testing.T) ptyWorld
}

func inFolder(mk func(t *testing.T) string) func(t *testing.T) ptyWorld {
	return func(t *testing.T) ptyWorld {
		t.Helper()
		env, _ := folders(t)
		dir := mk(t)
		return ptyWorld{dir: dir, env: env, scrub: func(s string) string { return scrub(s, dir) }}
	}
}

func journalWorldOf(set func(w *journalWorld)) func(t *testing.T) ptyWorld {
	return func(t *testing.T) ptyWorld {
		t.Helper()
		w := newJournalWorld(t)
		set(w)
		return ptyWorld{dir: w.repo, env: w.env, scrub: func(s string) string { return w.scrubbed(scrub(s, w.repo)) }}
	}
}

// ptyCases are the six journeys: first run in neither context, help, the menu
// in each context, one happy path, one refusal and one decision prompt.
func ptyCases() []ptyCase {
	rulebook := inFolder(func(t *testing.T) string {
		t.Helper()
		return fixture(t)
	})
	project := inFolder(func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		pinProject(t, dir)
		return dir
	})
	both := inFolder(func(t *testing.T) string {
		t.Helper()
		dir := fixture(t)
		pinProject(t, dir)
		return dir
	})
	neither := inFolder(func(t *testing.T) string {
		t.Helper()
		return t.TempDir()
	})
	closedRun := journalWorldOf(func(w *journalWorld) {
		w.line("r1", "intent", 1779000000, "")
		w.copy("r1", "a.txt", "old", true, 1779000001)
		w.closed("r1", "done", "a.txt", "old", "new", 1779000010)
		w.write(map[string]string{"a.txt": "new"})
	})
	movedFile := journalWorldOf(func(w *journalWorld) {
		closedAndOpen(w, "half")
		w.line("r4", "intent", 1779000300, "")
		w.copy("r4", "a.txt", "new", true, 1779000301)
		w.closed("r4", "done", "a.txt", "new", "newer", 1779000310)
		w.write(map[string]string{"a.txt": "newer"})
	})
	return []ptyCase{
		{journey: "first-run", name: "first-run", steps: answerMenu(), world: neither},
		{journey: "help", name: "help", args: []string{"--help"}, world: rulebook},
		{journey: "menu", name: "menu-rulebook", steps: answerMenu(), world: rulebook},
		{journey: "menu", name: "menu-project", steps: answerMenu(), world: project},
		{journey: "menu", name: "menu-both", steps: answerMenu(), world: both},
		{journey: "happy-path", name: "journal-list", args: []string{"journal", "list"}, world: closedRun},
		{journey: "refusal", name: "journal-restore-refused", args: []string{"journal", "restore", "r1"}, code: 5, world: movedFile},
		{journey: "prompt", name: "term-retire-asked", args: []string{"term", "retire", "spare"}, steps: answerTitleOnly(), code: 6, world: rulebook},
	}
}

// answerTitleOnly answers the first of the two questions a change on a
// decision asks when none is given, and leaves the second empty, so the run
// ends declined (6) before the book is read. The plan needs a name to record,
// which only git gives, and inside a user namespace that maps this account and
// not root, the trust walk refuses every path to git, as / shows as the overflow
// account's, and go-files' Who.Overflow, which trusts it there,
// is a test binary's alone and refused in the built gragas;
// the plan a decision leads to is the piped term-retire-declined golden's.
func answerTitleOnly() []ptyStep {
	return []ptyStep{
		{waitFor: "What is the decision called?", send: "It no longer holds\r"},
		{waitFor: "What did you decide, and why?", send: "\r"},
	}
}

// transcriptPart is one part of a pty transcript: the title line that opens it
// ("--- menu", "--- screen at the end", "--- exit 0") and the screen lines under
// it.
type transcriptPart struct {
	title string
	lines []string
}

// partsOf splits a transcript into its command line, its width line and its
// parts.
func partsOf(transcript string) (command, width string, parts []transcriptPart) {
	lines := strings.Split(strings.TrimSuffix(transcript, "\n"), "\n")
	if len(lines) > 0 {
		command, lines = lines[0], lines[1:]
	}
	if len(lines) > 0 {
		width, lines = lines[0], lines[1:]
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "--- ") {
			parts = append(parts, transcriptPart{title: l})
			continue
		}
		if len(parts) == 0 {
			parts = append(parts, transcriptPart{})
		}
		parts[len(parts)-1].lines = append(parts[len(parts)-1].lines, l)
	}
	return command, width, parts
}

// goldenOf is the golden of name, read and never written.
func goldenOf(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".golden")
	if err != nil {
		t.Fatalf("no golden %s: %v", name, err)
	}
	return string(b)
}

// goldenSettled is the condition the 80-column runs wait for: the screen's part of that title is the golden's
// part, line for line. The golden at 80 is the oracle a person read, so a picture taken of anything else, such as
// the first frame a menu draws before it is whole, can only fail the comparison it is then made for; waiting for
// the golden's own part first leaves that comparison to judge a program that draws something else, after the
// journey's deadline, naming the first line that differs. With -update there is no golden to wait
// for yet, and it answers nil.
func goldenSettled(t *testing.T, c ptyCase) func(title, screen string) string {
	t.Helper()
	if updating(os.Args[1:]) {
		return nil
	}
	_, _, want := partsOf(goldenOf(t, "pty-"+c.name+"-80"))
	return func(title, screen string) string {
		got := strings.Split(strings.TrimSuffix(screen, "\n"), "\n")
		for _, p := range want {
			if p.title != "--- "+title {
				continue
			}
			if slices.Equal(got, p.lines) {
				return ""
			}
			for i := range max(len(got), len(p.lines)) {
				g, w := "<none>", "<none>"
				if i < len(got) {
					g = got[i]
				}
				if i < len(p.lines) {
					w = p.lines[i]
				}
				if g != w {
					return fmt.Sprintf("the screen is not the golden's %s yet: line %d reads %q, the golden's %q", title, i+1, g, w)
				}
			}
		}
		return ""
	}
}

// updating says the test binary was run with -update, as uitest's own flag reads it: -update, --update,
// -update=true or --update=1. Only cmd/ and a few files may import flag (cli.config.7), so the arguments are read.
func updating(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if len(a) == len(name) || name != "update" {
			continue
		}
		return !hasValue || value == "true" || value == "1" || value == "t" || value == "T" || value == "TRUE" || value == "True"
	}
	return false
}
