// Package term is the adapter of port.Terminal over the shared terminal kit,
// and the one package that imports the kit's look (cli.look.1, arch.ports.6).
// It draws the banner, a table, the closing words and an error with the line
// to type, as text a caller writes through Say, and it strips control
// sequences from what the program did not write (cli.errors.4).
package term

import (
	"context"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/birdtable/go-console/ui"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// maxLine is the widest the look gets: past it lines are harder to read, so a
// wide terminal gets margin (cli.environment.2).
const maxLine = 96

// Options is what the adapter is built over. The environment is read in
// config and handed here (cli.config.3).
type Options struct {
	// In is the standard input Ask and Line read lines from.
	In io.Reader
	// Out is where Ask and Say write.
	Out io.Writer
	// Err is the standard error. A menu is offered only when In, Out and Err
	// are all a terminal; nil is none.
	Err io.Writer
	// Env is the environment a menu is drawn under: the kit reads TERM and
	// the like from it, never from the process. Config hands it here.
	Env []string
	// Columns is the width of the terminal Out is, in columns, 0 when Out is
	// not a terminal.
	Columns int
	// PipeColumns is the width COLUMNS gives, which a look off a terminal is
	// drawn to; 0 is none, and nothing is wrapped.
	PipeColumns int
	// ColorBits is the colour depth of Out, 0 for none: NO_COLOR, a pipe,
	// TERM=dumb and CI all give 0.
	ColorBits int
	// Accessible is the accessible form: no banner, no colour.
	Accessible bool
}

// MaxLineBytes is the most one line of input holds, its newline left out
// (cli.process.1, held to input). The largest real input is the guard's
// hook input, one JSON line that guard edit refuses past
// coreapi.MaxHookInput, 16 MiB: the cap is that, so every line the guard
// would take is read and no longer one is held (TestTheLineCapHoldsTheHooksInput
// in cmd/gragas keeps the two in step).
const MaxLineBytes = 16 << 20

// LineTooLong is the refusal of a line of input longer than Cap bytes.
type LineTooLong struct{ Cap int }

func (e *LineTooLong) Error() string {
	return "a line of input is longer than the " + strconv.Itoa(e.Cap) + " bytes read, so the input is not read further"
}

// Terminal is a port.Terminal. One line of input is read at a time.
type Terminal struct {
	opt Options

	mu    sync.Mutex // one writer at a time
	rmu   sync.Mutex // one reader at a time; a read waiting on input never holds up a write
	lines *ui.Lines
	// cap is the most one line holds; refused, once set, is every later
	// Line's answer.
	cap     int
	refused error
}

var _ port.Terminal = (*Terminal)(nil)

// New is the Terminal over opt, its lines held to MaxLineBytes. An input of
// nil is an input at its end; an output of nil is nowhere.
func New(opt Options) *Terminal { return over(opt, MaxLineBytes) }

// NewCapped is the Terminal over opt, its lines held to lineCap bytes in
// place of MaxLineBytes. A cap at or below zero is refused, and so is one
// that the line's "\r\n" would carry past the largest int: either would
// refuse every line.
func NewCapped(opt Options, lineCap int) (*Terminal, error) {
	if lineCap <= 0 || lineCap > math.MaxInt-len("\r\n") {
		return nil, errors.New("the cap on a line of input is " + strconv.Itoa(lineCap) + " bytes: it must be above zero and at most " + strconv.Itoa(math.MaxInt-len("\r\n")))
	}
	return over(opt, lineCap), nil
}

// over is the Terminal over opt with lines held to lineCap, above zero.
func over(opt Options, lineCap int) *Terminal {
	if opt.In == nil {
		opt.In = strings.NewReader("")
	}
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	if opt.Err == nil {
		opt.Err = io.Discard
	}
	return &Terminal{opt: opt, lines: ui.NewLines(opt.In, lineCap), cap: lineCap}
}

// Width is the terminal's columns, at most 96, and 0 when output is not a
// terminal.
func (t *Terminal) Width() int { return min(max(t.opt.Columns, 0), maxLine) }

// Colors is the colour depth in bits, 0 for none.
func (t *Terminal) Colors() int { return max(t.opt.ColorBits, 0) }

// Ask writes question, then reads one line of the answer.
func (t *Terminal) Ask(ctx context.Context, question string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := t.Say(question); err != nil {
		return "", err
	}
	return t.Line(ctx)
}

// Say writes text and a newline. Every character the screen does not show as
// written (model.Unshowable) is written as the escape %q gives it, ESC among
// them, so a name a file chose cannot move the cursor or say something the
// program did not, and a name is never shown as another. On a
// terminal of a width, text wider than it is broken at spaces, each line under
// the text's own indent, as a screen's lines are.
//
// Say matches no escape sequence: each ESC is escaped alone, and the rest of
// a sequence is shown as written, by Strip, the one rule of every field.
// text is a line already formatted, the program's words and
// the outside values set in them, and Say cannot see where one value ends. A sequence matched across the line could start in one value
// and end in the next, and take the program's words between them off the
// screen. Escaping one character at a time drops none. A
// screen's sentences (a heading, a list item, a note, the closing's what and
// next) and a problem's lines escape the same way.
func (t *Terminal) Say(text string) error {
	text = Strip(text)
	if w := t.Width(); w > 0 {
		text = Look{Width: w}.wrapSaid(text)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := io.WriteString(t.opt.Out, text+"\n")
	return err
}

// Line reads one line of input without its newline. At the end of input it
// answers io.EOF. A ctx that ends first is its own error, and the line the
// read goes on to get answers the next Line: the kit's Lines is the read, on
// every system, so no deadline is set on the input.
//
// A line longer than the cap (MaxLineBytes, or NewCapped's) is refused with
// a *LineTooLong, and so is every later Line: the read is closed, and the
// rest of the input is not read. Draining would read without end an input
// that sends no newline or a pipe that is stuck, the fault the cap is there
// to stop: Drain is the caller's, under a deadline of its own. The kit holds
// no more of a line than the cap and one buffer.
func (t *Terminal) Line(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	t.rmu.Lock()
	defer t.rmu.Unlock()
	if t.refused != nil {
		return "", t.refused
	}
	s, err := t.lines.Next(ctx)
	if err == nil {
		return s, nil
	}
	if cerr := ctx.Err(); cerr != nil {
		return "", cerr
	}
	if errors.Is(err, io.EOF) {
		return "", io.EOF
	}
	if errors.Is(err, ui.ErrLineTooLong) {
		return "", t.refuse()
	}
	return "", err
}

// Drain reads the rest of the input and keeps none of it, through the kit's
// Lines.Discard, to its end. It is for a caller whose input was refused while the writer
// may still be writing it, so the writer's write ends and does not fail on a
// closed pipe. It drains what Line has read ahead too, and
// leaves Line as it was: a refused Line stays refused. A ctx that ends first
// is its own error, and Drain returns on it at once, even while a read is
// blocked on an input that sends nothing: the read goes on behind it, as
// Line's does, and a later Drain joins it and starts no second one.
func (t *Terminal) Drain(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.rmu.Lock()
	defer t.rmu.Unlock()
	return t.lines.Discard(ctx)
}

// refuse closes the read: every later Line answers the same refusal.
func (t *Terminal) refuse() error {
	t.refused = &LineTooLong{Cap: t.cap}
	return t.refused
}

// Strip is text with every character the screen does not show as written
// (model.Unshowable) written, one by one, as the escape %q gives it: \t,
// \x1b, \u200b, \U000e0100. Every other character is kept as it is. Text the
// program did not write goes through it before it is displayed. So nothing
// the terminal acts on reaches it, no bidi override draws a name's letters in
// another order, and a name that differs from another by a
// character drawn as nothing, a<U+200B>b.txt or a<U+034F>b.txt, is never shown
// as ab.txt: the escape is on the screen and in the text copied off it
// It escapes by model.Unshowable, every rune of which shell.Quote refuses too (a test holds it), spelling
// characters included.
//
// Strip matches no escape sequence and drops no character. ESC is one of the
// characters it escapes, so a sequence never survives it whole, and the rest
// of the sequence is shown as the printable text it is: a<ESC>[2Jb.txt is
// shown as a\x1b[2Jb.txt, never as ab.txt, so two names never look like one.
// It is the one rule for every field and every sentence: a
// table's cell, a column title, a closing's word, an outside value set into
// a line, and a line built from several values, whose sequence could start in
// one value and end in the next. Removing a whole sequence did
// harm in both: across a line it took the program's words between two values,
// and inside one value it hid letters of a name. Escaping each ESC alone
// does neither.
//
// A backslash is not escaped, so a name that spells an escape, a\u200bb.txt
// with a real backslash, is shown like a<U+200B>b.txt. shell.Quote refuses the
// real character, so only the screen is confused, and only by such a name;
// escaping the backslash would double every Windows path.
func Strip(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if !model.Unshowable(r) {
			b.WriteRune(r)
			continue
		}
		q := strconv.QuoteRuneToASCII(r)
		b.WriteString(q[1 : len(q)-1])
	}
	return b.String()
}

// Look is the way this terminal is drawn to: its width (the terminal's, else
// COLUMNS, at most 96), whether it paints, and whether it is the accessible
// form.
func (t *Terminal) Look() Look {
	width := t.Width()
	if width == 0 {
		width = min(max(t.opt.PipeColumns, 0), maxLine)
	}
	return Look{Width: width, Rich: t.Colors() > 0 && !t.opt.Accessible, Accessible: t.opt.Accessible}
}

// Draw writes text a Look drew, as it is: the escape sequences in it are the
// look's own colours, so Say's stripping would take them out. Text the
// program did not write never comes here without going through the look, which
// strips it (cli.errors.4).
func (t *Terminal) Draw(drawn string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := io.WriteString(t.opt.Out, drawn)
	return err
}
