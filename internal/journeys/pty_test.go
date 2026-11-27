package journeys

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/ui"
	"github.com/birdtable/go-console/uitest"
)

// The pseudo-terminal is go-console's uitest.StartPty on all three systems: a pty on linux and darwin, a ConPTY on
// windows, its output read by a terminal emulator that answers the program's questions as a dark terminal does. The
// emulator is uitest.PtyScreenCols wide, wider than any journey's screen, so a line the program draws past the
// journey's last column stays one row to be measured (tests.journeys.8); and it keeps ptyScrollback
// rows that scroll off the top of its 24, so the page a journey compares is every row the program drew.

// ptyWait bounds a whole pty journey, so a program that hangs fails the test
// with what it had printed and does not hold the suite.
const ptyWait = 20 * time.Second

// ptyQuiet is how long the screen must hold still before a step is taken, so a
// frame the program draws in two writes is read whole.
const ptyQuiet = 200 * time.Millisecond

// ptyRows is how high a journey's screen is.
const ptyRows = 24

// ptyScrollback is how many rows that scroll off the top of the screen are kept: far more than any journey draws, so
// the page is the whole of what the program drew, as the page of gragas's own emulator was before it.
const ptyScrollback = 1000

// ptyStep is one thing a pty journey does to a running program: wait for text
// on the screen and for the screen to hold still, take a picture of it if asked,
// and send keys.
//
// A step that takes a picture waits for words of the picture's last line. A
// quiet screen is not a finished one: under load the program can stop for
// longer than ptyQuiet between two frames. A menu, for one, draws its first
// frame before it knows the screen's width, each row cut at the edge, and its
// whole frame only after. Words that only the end of the finished
// frame holds are on the screen only once that frame is drawn to its end.
type ptyStep struct {
	waitFor string // text that must be on the screen; "" for none
	// until, when set, must also hold of the screen before the step goes on: it answers "" when the screen is the
	// one the step is after, and otherwise what it still lacks. A quiet
	// screen alone does not say it: the program draws its menu twice, the rows
	// cut at the edge first and wrapped after, and under load it is quiet for
	// longer than ptyQuiet between the two. The wait has the journey's deadline
	// (ptyWait), and past it the step fails with the screen it last had and what that lacked.
	until func(screen string) string
	snap  string // a title; the screen is added to the transcript under it
	send  string // the keys to send
}

// ptyJourney is the binary run under a pseudo-terminal of cols by ptyRows rows,
// in the isolated environment, with its own home and temp folder.
type ptyJourney struct {
	dir   string
	env   map[string]string
	cols  int
	args  []string
	steps []ptyStep
}

// ptyResult is what a person at the terminal saw: the transcript of the pictures
// and the screen the program left, how it ended, and every line of them drawn
// past the screen's edge.
type ptyResult struct {
	transcript string
	code       int
	overruns   []overrun
}

// ptyLauncher is what a pty journey needs before it can start the program: the
// check that the binary is the tree under test, and the pty. They are values so
// that a test can hand a launcher recording fakes and see the order they are
// used in.
type ptyLauncher struct {
	fresh func() error
	open  func(ctx context.Context, cmd *exec.Cmd, cols, rows int) (*uitest.Pty, error)
}

// startPty starts cmd at go-console's pty of cols by rows, answering the
// program's questions, with the scrollback a journey's page needs.
func startPty(ctx context.Context, cmd *exec.Cmd, cols, rows int) (*uitest.Pty, error) {
	p, err := uitest.StartPty(ctx, cmd, cols, rows, false, uitest.PtyScrollback(ptyScrollback))
	if err != nil {
		return nil, fmt.Errorf("start the program at a pty of %d by %d: %w", cols, rows, err)
	}
	return p, nil
}

// newPtyLauncher is the launcher every journey uses: the real freshness check
// and the real pty.
func newPtyLauncher(t *testing.T) ptyLauncher {
	t.Helper()
	return newPtyLauncherFor(gragasBin(t))
}

// newPtyLauncherFor is that launcher for the binary at bin.
func newPtyLauncherFor(bin string) ptyLauncher {
	return ptyLauncher{fresh: func() error { return freshAt(bin) }, open: startPty}
}

// launch is the freshness check, and only if it passes cmd started at the pty
// of the journey's size. A stale binary opens nothing and starts nothing.
func (l ptyLauncher) launch(ctx context.Context, j ptyJourney, cmd *exec.Cmd) (*uitest.Pty, error) {
	if err := l.fresh(); err != nil {
		return nil, err
	}
	return l.open(ctx, cmd, j.cols, ptyRows)
}

// runPTY runs the journey and answers what it saw.
func runPTY(t *testing.T, j ptyJourney) ptyResult {
	t.Helper()
	env := map[string]string{"TERM": "xterm-256color", "COLUMNS": strconv.Itoa(j.cols), "LINES": strconv.Itoa(ptyRows)}
	maps.Copy(env, j.env)
	// command makes the isolated command, bound to the test's context: it dies with the test on every system.
	p, err := newPtyLauncher(t).launch(t.Context(), j, command(t, j.dir, env, j.args...))
	if err != nil {
		t.Fatalf("could not start the pty journey: %v", err)
	}
	// Close ends the program if it still runs, and the pty; uitest's Close and a ConPTY's are safe to call after Wait.
	defer p.Close()
	pageOf := func(screen string) string { return page(t, p.Scrollback(), screen) }

	journey, cancel := context.WithTimeout(t.Context(), ptyWait)
	defer cancel()
	deadline, _ := journey.Deadline()
	var out bytes.Buffer
	var over []overrun
	out.WriteString(strings.TrimSpace("$ gragas "+strings.Join(j.args, " ")) + "\n--- " + strconv.Itoa(j.cols) + " columns\n")
	for _, s := range j.steps {
		shot, lacking, ok := settle(p, pageOf, time.Until(deadline), s.waitFor, s.until)
		if !ok {
			t.Fatalf("waited %s for %q on the screen, and for the step's condition (%s):\n%s", ptyWait, s.waitFor, lacking, pageOf(p.Screen()))
		}
		if s.snap != "" {
			// shot is the screen settle judged, so the picture is the one that was held to the condition.
			if f := stepFinding(s, shot); f != "" {
				t.Error(f)
			}
			over = addOverruns(over, overrunsOf(shot, j.cols))
			out.WriteString("--- " + s.snap + "\n" + shot)
		}
		if s.send != "" {
			if err := p.Send(s.send); err != nil {
				t.Fatalf("could not send keys: %v", err)
			}
		}
	}
	// Wait is the exit, and then every byte the program wrote read: go-console marks the end of the output on a
	// pty and closes a ConPTY to flush it, so nothing races the exit. It has the rest
	// of the journey's deadline for the exit and a fresh ptyWait for the reads after it, so it is never open-ended.
	exited := make(chan int, 1)
	go func() { exited <- p.Wait() }()
	ended := time.NewTimer(time.Until(deadline) + ptyWait)
	defer ended.Stop()
	var code int
	select {
	case code = <-exited:
	case <-ended.C:
		p.Close()
		t.Fatalf("gragas did not end, or its output was not read to its end, in time; the screen:\n%s", pageOf(p.Screen()))
	}
	if code < 0 {
		t.Fatalf("gragas was ended by a signal or by the test's context; the screen:\n%s", pageOf(p.Screen()))
	}
	end := pageOf(p.Screen())
	over = addOverruns(over, overrunsOf(end, j.cols))
	out.WriteString("--- screen at the end\n" + end + "--- exit " + strconv.Itoa(code) + "\n")
	return ptyResult{transcript: out.String(), code: code, overruns: over}
}

// page is what a person can read of the terminal, scrolled back to its top: the rows that scrolled off it and then
// the screen, each without the spaces at its end and ended by a line break, and no empty rows at the end.
func page(tb testing.TB, scrollback, screen string) string {
	tb.Helper()
	text := stateRowMasked(tb, screen)
	if scrollback != "" {
		scrollback = stateRowMasked(tb, scrollback)
		text = scrollback + "\n" + screen
	}
	if text = strings.TrimRight(text, "\n"); text == "" {
		return ""
	}
	return text + "\n"
}

// waiter is what a step waits on: uitest.Pty's WaitFor, which answers the screen once ok says yes of it and of the
// time since the pty last gave a byte. It is a value so that a test can hand settle frames of its own.
type waiter interface {
	WaitFor(timeout time.Duration, ok func(screen string, idle time.Duration) bool) (string, bool)
}

// settle waits until waitFor is on the page, until (when it is not nil) holds of it, and the screen has held still
// for ptyQuiet. It answers the page it judged, so the picture a step takes is the one that was held to the
// condition; or, when timeout passes first, false and what the last page still lacked, "" when it was never quiet.
// pageOf makes the page of a screen.
func settle(w waiter, pageOf func(screen string) string, timeout time.Duration, waitFor string, until func(screen string) string) (shot, lacking string, ok bool) {
	_, ok = w.WaitFor(timeout, func(screen string, idle time.Duration) bool {
		if idle < ptyQuiet {
			return false
		}
		shown := pageOf(screen)
		switch {
		case !strings.Contains(shown, waitFor):
			lacking = "the text " + strconv.Quote(waitFor) + " is not on the screen"
			return false
		case until != nil:
			if lacking = until(shown); lacking != "" {
				return false
			}
		}
		shot = shown
		return true
	})
	if !ok {
		return "", lacking, false
	}
	return shot, "", true
}

// overrun is a line the program drew past the screen's last column: the row of
// the page it is on, counted from one, what the row holds and how many columns
// it takes (tests.journeys.8, no line wider than the screen).
type overrun struct {
	row, width int
	text       string
}

// overrunsOf is each row of the page wider than cols, measured in the cells a
// terminal gives it.
func overrunsOf(pg string, cols int) []overrun {
	var out []overrun
	for i, row := range strings.Split(strings.TrimSuffix(pg, "\n"), "\n") {
		if w := cells(row); w > cols {
			out = append(out, overrun{row: i + 1, width: w, text: row})
		}
	}
	return out
}

// cells is how many cells of a terminal s takes, by go-console's own measure,
// the one the program pads its columns with: ui.Column pads s to a width wider
// than any row the emulator holds, and the padding is what s leaves of it.
func cells(s string) int {
	const room = uitest.PtyScreenCols + 1
	return room - (len(ui.Column(s, room)) - len(s))
}

// addOverruns is to with each of more it does not already hold: a row that
// stays on the screen across pictures is one overrun.
func addOverruns(to, more []overrun) []overrun {
	for _, o := range more {
		if !slices.Contains(to, o) {
			to = append(to, o)
		}
	}
	return to
}

// answerMenu waits for the menu to be drawn whole, takes a picture of it, and
// backs out: an answer with nothing to say. It waits for the last words of the
// menu's last row, not for its note: the menu's first frame is drawn before it
// knows the screen's width, with each row cut at the edge, and that frame holds
// the note too. It is here, beside ptyStep, because
// TestAMenuStepPicturesTheMenuAtItsWidth reads it.
func answerMenu() []ptyStep {
	return []ptyStep{{waitFor: "write them", snap: "menu", send: "\x03"}}
}

// stepFinding is whether a step that took a picture waited for words of the
// picture's last line that holds a word, as a finding, or "". A step that waits
// for words above it can take its picture of a frame the program has not drawn
// to its end.
func stepFinding(s ptyStep, shot string) string {
	lines := strings.Split(strings.TrimSuffix(shot, "\n"), "\n")
	last := ""
	for _, l := range slices.Backward(lines) {
		if len(unframed(l)) > 0 {
			last = l
			break
		}
	}
	if s.waitFor == "" || !strings.Contains(last, s.waitFor) {
		return fmt.Sprintf("the step that takes the picture %q waits for %q, which is not on the picture's last line (%q): it can take its picture of a frame not drawn to its end",
			s.snap, s.waitFor, strings.TrimSpace(last))
	}
	return ""
}

// frames is a waiter whose screen is shown[0] from its start and then shown[i] from at[i] on, as a program held off
// the CPU draws its frames; idle is the time since the frame on the screen was drawn. It looks every 10 ms, as
// uitest's WaitFor does, and its time is its looks, 10 ms each: it reads no clock, and a look the machine is late
// for moves the frames no further than one look.
type frames struct {
	shown []string
	at    []time.Duration
}

const frameLook = 10 * time.Millisecond

func (f frames) WaitFor(timeout time.Duration, ok func(screen string, idle time.Duration) bool) (string, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	look := time.NewTicker(frameLook)
	defer look.Stop()
	for looks := 0; ; looks++ {
		since := time.Duration(looks) * frameLook
		i := 0
		for k := range f.at {
			if f.at[k] <= since {
				i = k
			}
		}
		if ok(f.shown[i], since-f.at[i]) {
			return f.shown[i], true
		}
		select {
		case <-look.C:
		case <-deadline.C:
			return f.shown[i], false
		}
	}
}

// The two frames the menu drew at 40 columns in the loaded run that reddened
// menu-rulebook-40, 227 ms apart, as the screen showed them. The
// first is drawn before the menu knows the screen's width: each row is one
// line, cut at the edge. The second is the menu at its width, its rows wrapped.
const (
	menuUnsized = "┃ /\n" +
		"┃ Or answer with --gragas <command>\n" +
		"┃ > check    Check a project against its\n" +
		"┃   help     Show the help of a command\n" +
		"┃   version  Say which build this is\n" +
		"┃   gen      Say which generated files a\n" +
		"┃\n┃"
	menuSized = "┃ /\n" +
		"┃ Or answer with --gragas <command>\n" +
		"┃ > check    Check a project against its\n" +
		"┃   rules, or a rulebook against its\n" +
		"┃   gate\n" +
		"┃   help     Show the help of a command\n" +
		"┃   version  Say which build this is\n" +
		"┃   gen      Say which generated files\n" +
		"┃   are out of date, and write them\n" +
		"┃\n┃"
)

// pictureOf takes the step's picture of a 40-column screen the menu draws its
// two frames on, the second after a pause longer than ptyQuiet, as a program
// held off the CPU under load pauses.
func pictureOf(t *testing.T, s ptyStep) string {
	t.Helper()
	f := frames{shown: []string{menuUnsized, menuSized}, at: []time.Duration{0, 2 * ptyQuiet}}
	shot, lacking, ok := settle(f, func(screen string) string { return page(t, "", screen) }, ptyWait, s.waitFor, s.until)
	if !ok {
		t.Fatalf("the screen never settled with %q on it (%s)", s.waitFor, lacking)
	}
	return shot
}

// TestAMenuStepPicturesTheMenuAtItsWidth: the menu step waits through the
// frame the menu draws before it knows its width, however long the program
// pauses before the next, and pictures the menu with every row wrapped; its
// wait is on the picture's last line. The plant is the step as it was, waiting
// for the menu's note: it pictures the first frame, with "rules," cut off the
// check row as in an earlier red, and stepFinding names it. Risk: a pty
// journey that pictures a frame the program has not finished, on a loaded
// machine only.
func TestAMenuStepPicturesTheMenuAtItsWidth(t *testing.T) {
	t.Parallel()
	const wrapped = "┃   rules, or a rulebook against its"
	t.Run("the menu step", func(t *testing.T) {
		t.Parallel()
		s := answerMenu()[0]
		shot := pictureOf(t, s)
		if !strings.Contains(shot, wrapped) || !strings.Contains(shot, "write them") {
			t.Errorf("the step pictured a frame not drawn at its width:\n%s", shot)
		}
		if f := stepFinding(s, shot); f != "" {
			t.Error(f)
		}
	})
	t.Run("plant: the step waits for the note", func(t *testing.T) {
		t.Parallel()
		s := ptyStep{waitFor: "answer with", snap: "menu", send: "\x03"}
		shot := pictureOf(t, s)
		if strings.Contains(shot, "rules,") {
			t.Errorf("plant: the step waiting for the note pictured the menu at its width:\n%s", shot)
		}
		f := stepFinding(s, shot)
		for _, w := range []string{`"menu"`, `waits for "answer with"`, `last line ("┃   gen      Say which generated files a")`} {
			if !strings.Contains(f, w) {
				t.Errorf("plant: the finding %q does not say %q", f, w)
			}
		}
	})
}

// TestARowWiderThanTheScreenIsAnOverrun: each row of a page that takes more
// cells than the screen has columns is an overrun, with its row, its text and
// its width; a row that fills the screen exactly is not, and a frame
// character takes one cell. The emulator is wider than the screen, so a line
// drawn past the edge is one row here. Risk: a narrow screen
// that wraps a line too wide for it and calls the result two rows.
func TestARowWiderThanTheScreenIsAnOverrun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		page string
		want []overrun
	}{
		{"a full row", "abcd\nef\n", nil},
		{"framed and full", "┃ ab\n", nil},
		{"two columns past", "abcdef\n", []overrun{{row: 1, width: 6, text: "abcdef"}}},
		{"on the second row", "ab\nabcdef\n", []overrun{{row: 2, width: 6, text: "abcdef"}}},
		{"two rows past", "abcde\nab\nabcdef\n", []overrun{{row: 1, width: 5, text: "abcde"}, {row: 3, width: 6, text: "abcdef"}}},
		{"framed and past", "┃ abc\n", []overrun{{row: 1, width: 5, text: "┃ abc"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := overrunsOf(tc.page, 4); !slices.Equal(got, tc.want) {
				t.Errorf("overruns %+v, want %+v", got, tc.want)
			}
		})
	}
	if got := addOverruns([]overrun{{row: 1, width: 5, text: "abcde"}}, []overrun{{row: 1, width: 5, text: "abcde"}, {row: 2, width: 6, text: "abcdef"}}); len(got) != 2 {
		t.Errorf("a row seen in two pictures is one overrun: %+v", got)
	}
}
