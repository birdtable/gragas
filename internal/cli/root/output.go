package root

// What a run says to a person: the screens on the Terminal port, the closing
// word each ends on, and the diagnostics on standard error (cli.output.7).
// Nothing here draws; the adapter behind the port does.

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ending is how a run ended: the word, the sentence beside it and the line to
// type, which is the answer's "next" whether or not the screen shows it. said
// is the line to type when the sentence already says it, in backticks: the
// answer's next when next is empty, and not drawn a second time under the word.
type ending struct {
	word             model.Word
	what, next, said string
}

// codeOf is the exit code a word ends the old commands on: the table of
// internal/exit, refused on 5 and declined on 6.
func codeOf(w model.Word) int {
	c, _ := exit.Code(w)
	return c
}

// end draws the lead blocks and the closing word, and returns the exit code.
// Under --json nothing is drawn: the document is the answer. Under --sarif it
// is drawn on standard error, since the SARIF document holds no closing word.
func (s *session) end(e ending, lead ...model.Block) int {
	s.ended = &e
	closing, err := model.NewClosing(e.word, e.what, e.next)
	if err != nil {
		return s.die("the run could not say how it ended: %v. %s", err, e.what)
	}
	// Every screen follows the banner or the reading line, and the look keeps a
	// blank line between that and what the screen says.
	screen, err := model.NewScreen(append(append([]model.Block{gap()}, lead...), closing)...)
	if err != nil {
		return s.die("the run could not draw its ending: %v. %s", err, e.what)
	}
	if term := s.person(); !s.json && term != nil {
		if err := term.Show(screen); err != nil {
			s.warn("the answer could not be written: %v", err)
		}
	}
	return codeOf(e.word)
}

// say writes a line a person reads, apart from a screen; under --json it is
// not written, and under --sarif it is written on standard error. The
// terminal breaks a line wider than it, at the line's indent.
func (s *session) say(line string) {
	term := s.person()
	if s.json || term == nil {
		return
	}
	if err := term.Say(line); err != nil {
		s.warn("the answer could not be written: %v", err)
	}
}

// person is the terminal what a person reads is drawn on: standard error
// under --sarif, where stdout holds the document alone, and stdout otherwise.
func (s *session) person() port.Terminal {
	if s.sarif {
		return s.p.Diagnostics
	}
	return s.p.World.Terminal
}

// warn reports something true that did not stop the run, on standard error.
func (s *session) warn(format string, a ...any) { s.diag("warning: ", fmt.Sprintf(format, a...)) }

// die reports a failure that has no screen, on standard error, and returns the
// code. Under --json the answer says it, and nothing is written here.
func (s *session) die(format string, a ...any) int {
	return s.dieSaying("", format, a...)
}

// dieSaying is die for a failure whose message says the line to type in
// backticks: next is that line, the answer's next under --json.
func (s *session) dieSaying(next, format string, a ...any) int {
	msg := fmt.Sprintf(format, a...)
	s.died, s.fault, s.faultNext = msg, msg, next
	if !s.json {
		s.diag("error: ", msg)
	}
	return exit.Failed
}

// diag writes msg after label on standard error, a continuation line indented
// under the first.
func (s *session) diag(label, msg string) {
	if s.p.Diagnostics == nil {
		return
	}
	indent := strings.Repeat(" ", len(label))
	for i, line := range strings.Split(msg, "\n") {
		if i == 0 {
			line = label + line
		} else {
			line = indent + line
		}
		if err := s.p.Diagnostics.Say(line); err != nil {
			return // standard error is gone; there is nowhere to say so
		}
	}
}

// change records one line of what the run writes, or would write, and gives
// the line a person reads in the gutter the plan uses. The line is also
// handed to the run record when the run really writes.
func (s *session) change(verb, path string) string {
	line := verb + " " + path
	s.changes = append(s.changes, line)
	col := len("quarantine") + 1
	if !s.applied {
		verb, col = "would "+verb, len("would quarantine")+1
	} else {
		s.rec.change(line)
	}
	return verb + strings.Repeat(" ", max(col-len(verb), 1)) + path
}

// gap is the blank line the look keeps between the line that says what was
// read and what follows it: a Note with no text draws as one.
func gap() model.Block { return model.Note{} }
