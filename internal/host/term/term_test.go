package term_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/go-console/shell"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// TestTheTerminalKeepsTheContractOfThePort: the Terminal suite every adapter
// runs, over a scripted input and an output buffer.
func TestTheTerminalKeepsTheContractOfThePort(t *testing.T) {
	t.Parallel()
	contracttest.Terminal(t, func(t *testing.T, lines []string) port.Terminal {
		t.Helper()
		in := ""
		if len(lines) > 0 {
			in = strings.Join(lines, "\n") + "\n"
		}
		return term.New(term.Options{In: strings.NewReader(in), Out: &bytes.Buffer{}})
	})
}

// TestAskWritesTheQuestionThenReadsOneLine: the question reaches the output
// once, and the answer is the next line without its newline or a carriage
// return.
func TestAskWritesTheQuestionThenReadsOneLine(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	tm := term.New(term.Options{In: strings.NewReader("yes\r\nno"), Out: &out})
	got, err := tm.Ask(t.Context(), "apply?")
	if err != nil || got != "yes" || out.String() != "apply?\n" {
		t.Errorf("Ask = %q, %v, wrote %q", got, err, out.String())
	}
	if got, err := tm.Line(t.Context()); err != nil || got != "no" {
		t.Errorf("a last line with no newline = %q, %v, want no", got, err)
	}
	if _, err := tm.Line(t.Context()); !errors.Is(err, io.EOF) {
		t.Errorf("Line at the end = %v, want io.EOF", err)
	}
}

// TestTheSizesAreTheTerminalsAndNeverPastNinetySix: width 0 off a terminal, the
// terminal's up to 96, and a negative number is none (cli.environment.2).
func TestTheSizesAreTheTerminalsAndNeverPastNinetySix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ cols, colors, wantW, wantC int }{
		{0, 0, 0, 0}, {80, 8, 80, 8}, {200, 24, 96, 24}, {-5, -1, 0, 0},
	} {
		tm := term.New(term.Options{Columns: c.cols, ColorBits: c.colors})
		if tm.Width() != c.wantW || tm.Colors() != c.wantC {
			t.Errorf("columns %d, colors %d: Width %d, Colors %d, want %d and %d", c.cols, c.colors, tm.Width(), tm.Colors(), c.wantW, c.wantC)
		}
	}
}

// TestALineWaitingOnInputEndsWithItsContext: a read blocked on a pipe returns
// the context's error when it is cancelled, and the next line is not lost.
// Planted: nothing is written until after the cancel.
func TestALineWaitingOnInputEndsWithItsContext(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	tm := term.New(term.Options{In: r, Out: io.Discard})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := tm.Line(ctx); done <- err }()
	select {
	case <-done:
		t.Fatal("Line returned before anything was written or cancelled")
	case <-time.After(50 * time.Millisecond): // the read is blocked
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Line = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled Line is still blocked on its input")
	}
	if _, err := w.WriteString("later\n"); err != nil {
		t.Fatal(err)
	}
	if got, err := tm.Line(t.Context()); err != nil || got != "later" {
		t.Errorf("the next Line = %q, %v, want later", got, err)
	}
}

// TestSayBreaksALineWiderThanTheTerminalUnderItsIndent: on a terminal of 24
// columns a line of 2 spaces and longer text is said as lines of at most 24
// cells, each under the 2 spaces, every word kept in order; the spaces inside
// a piece are kept, a word wider than the room is left whole, and a wide
// character counts its 2 cells; a line that fits, and every line where the
// width is 0, is said as it is. Planted failure: Say
// writing the text whole, which runs past the edge.
func TestSayBreaksALineWiderThanTheTerminalUnderItsIndent(t *testing.T) {
	t.Parallel()
	const long = "  spare: active, then retired. The term stays in its file"
	for _, tc := range []struct {
		name, text, want string
		cols             int
	}{
		{"wider", long, "  spare: active, then\n  retired. The term\n  stays in its file\n", 24},
		{"fits", "  apply?", "  apply?\n", 24},
		{"no width", long, long + "\n", 0},
		{"spaces inside kept", "    +   enable: errorlint and more", "    +   enable: errorlint\n    and more\n", 25},
		{"a word wider than the room", "  abcdefghijklmnopqrst uv", "  abcdefghijklmnopqrst\n  uv\n", 12},
		{"wide characters by cells", "  日本語の文字 です", "  日本語の文字\n  です\n", 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			tm := term.New(term.Options{In: strings.NewReader(""), Out: &out, Columns: tc.cols})
			if err := tm.Say(tc.text); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Errorf("Say(%q) at %d columns:\n got  %q\n want %q", tc.text, tc.cols, out.String(), tc.want)
			}
		})
	}
}

// planted is a file name that tries to move the cursor, retitle the window and
// say a closing word of its own.
const planted = "a\x1b[2J\x1b]0;owned\x07b\ndone  everything\x00.txt"

// TestAnEscapeSequenceInAFileNameNeverReachesTheTerminal: plant, control and
// refusal. The plant is a name that carries sequences through Say, Ask and
// every block of a Screen, in plain and in colour; the control is a clean name
// that reaches the screen unchanged; the refusal is that no ESC, BEL, newline
// or NUL of the plant reaches the output: each is its escape, \x1b, \a, \n
// and \x00, and every printable letter of the sequences is kept, so the name
// is not shown as another (cli.errors.4).
func TestAnEscapeSequenceInAFileNameNeverReachesTheTerminal(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	for _, mode := range []struct {
		name string
		cols int
		bits int
	}{{"plain", 0, 0}, {"colour", 80, 24}} {
		t.Run(mode.name, func(t *testing.T) {
			var out bytes.Buffer
			tm := term.New(term.Options{In: strings.NewReader("x\n"), Out: &out, Columns: mode.cols, ColorBits: mode.bits})
			if err := tm.Say(planted); err != nil {
				t.Fatal(err)
			}
			if _, err := tm.Ask(t.Context(), planted); err != nil {
				t.Fatal(err)
			}
			c, err := model.NewClosing(model.Refused, planted, planted)
			if err != nil {
				t.Fatal(err)
			}
			s, err := model.NewScreen(model.Heading{Text: planted}, model.Reads{What: planted},
				model.Table{Columns: []model.Column{{Title: planted}}, Rows: [][]string{{planted}}},
				model.List{Items: []string{planted}}, model.Note{Text: planted}, model.DryRunFooter{Flag: planted}, c)
			if err != nil {
				t.Fatal(err)
			}
			if err := tm.Show(s); err != nil {
				t.Fatal(err)
			}
			got := out.String()
			for _, bad := range []string{"\x1b[2J", "\x1b]0;", "\x07", "\x00"} {
				if strings.Contains(got, bad) {
					t.Errorf("the output holds %q from the planted name:\n%q", bad, got)
				}
			}
			if strings.Contains(got, "\ndone  everything") {
				t.Errorf("the planted name said a word of its own:\n%q", got)
			}
			if !strings.Contains(got, `a\x1b[2J\x1b]0;owned\ab\ndone  everything\x00.txt`) {
				t.Errorf("the printable letters of the planted name are gone, or its control characters are not escaped:\n%q", got)
			}
		})
	}
	t.Run("control: a clean name", func(t *testing.T) {
		var out bytes.Buffer
		if err := term.New(term.Options{Out: &out}).Say("rulebook/parts.yaml"); err != nil || out.String() != "rulebook/parts.yaml\n" {
			t.Errorf("Say = %v, wrote %q", err, out.String())
		}
	})
	t.Run("Strip escapes the ESC of an OSC and a CSI as it escapes any control character, and keeps the rest", func(t *testing.T) {
		if got := term.Strip(planted); got != `a\x1b[2J\x1b]0;owned\ab\ndone  everything\x00.txt` {
			t.Errorf("Strip = %q", got)
		}
		if got := term.Strip("café ✓ 日本語"); got != "café ✓ 日本語" {
			t.Errorf("Strip of ordinary text = %q", got)
		}
	})
}

// TestAnEscapeInOneValueNeverTakesTheProgramsWords: plant, one
// row per way an escape opened in an outside value is ended outside it. Each
// line is the program's words with outside values set in them, as a
// diagnostic is formatted before Say sees it. Through Say and Ask, plain and
// at a width, the line is written with every word of the program's own text,
// each ESC and BEL shown as its escape and the rest of the
// sequence as written. With whole-sequence removal put back in Strip, every
// row is red: the issue's row reads "warning: file AB is bad". The control is
// a sequence whole inside one value, which a field shows escaped too and never
// loses.
func TestAnEscapeInOneValueNeverTakesTheProgramsWords(t *testing.T) {
	t.Parallel()
	for _, row := range []struct{ name, line, want string }{
		{
			"an OSC opened in one name and ended by BEL in the next",
			"warning: file " + "A\x1b]" + " is fine; ignore the rest. file " + "\x07B" + " is bad",
			`warning: file A\x1b] is fine; ignore the rest. file \aB is bad`,
		},
		{
			"an OSC opened in one name and ended by ST in the next",
			"warning: file " + "A\x1b]" + " is fine; ignore the rest. file " + "\x1b\\B" + " is bad",
			`warning: file A\x1b] is fine; ignore the rest. file \x1b\B is bad`,
		},
		{
			"one name said twice, its OSC ended by its own BEL the second time",
			"warning: file " + "a\x07b\x1b]c" + " is fine; ignore the rest. file " + "a\x07b\x1b]c" + " is bad",
			`warning: file a\ab\x1b]c is fine; ignore the rest. file a\ab\x1b]c is bad`,
		},
		{
			"a CSI opened at a name's end and ended by the program's next letter",
			"warning: file " + "A\x1b[" + " is bad",
			`warning: file A\x1b[ is bad`,
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			for _, cols := range []int{0, 80} {
				var out bytes.Buffer
				tm := term.New(term.Options{In: strings.NewReader("x\n"), Out: &out, Columns: cols})
				if err := tm.Say(row.line); err != nil {
					t.Fatal(err)
				}
				if _, err := tm.Ask(t.Context(), row.line); err != nil {
					t.Fatal(err)
				}
				if want := row.want + "\n" + row.want + "\n"; out.String() != want {
					t.Errorf("at %d columns, Say and Ask wrote\n %q\nwant\n %q", cols, out.String(), want)
				}
			}
		})
	}
	t.Run("a screen's sentences and a problem keep the program's words", func(t *testing.T) {
		t.Parallel()
		sentenceKeepsTheProgramsWords(t)
	})
	t.Run("control: a screen's field shows a whole sequence escaped", func(t *testing.T) {
		t.Parallel()
		if got, want := term.Strip("A\x1b]0;title\x07B"), `A\x1b]0;title\aB`; got != want {
			t.Errorf("Strip = %q, want %q", got, want)
		}
	})
}

// sentenceKeepsTheProgramsWords plants the issue's line in
// each block of a screen that carries a formatted sentence, and in a problem:
// a Heading (conflicts' "tool target: key differs in ..."), a List item, a
// Note (attention's "entity: message"), the closing's What and Next (most
// often err.Error()), and Look.Problem's lines. Plain, in colour and
// accessible, the sentence is drawn with every word of the program's text,
// its ESC and BEL shown as their escapes.
// With those paths back on the whole-line Strip, each is drawn as "warning:
// file AB is bad".
func sentenceKeepsTheProgramsWords(t *testing.T) {
	t.Helper()
	const line = "warning: file " + "A\x1b]" + " is fine; ignore the rest. file " + "\x07B" + " is bad"
	const want = `warning: file A\x1b] is fine; ignore the rest. file \aB is bad`
	closing, err := model.NewClosing(model.Failed, line, line)
	if err != nil {
		t.Fatal(err)
	}
	tail, err := model.NewClosing(model.Done, "the end", "")
	if err != nil {
		t.Fatal(err)
	}
	blocks := []struct {
		name   string
		blocks []model.Block
		count  int
	}{
		{"a heading", []model.Block{model.Heading{Text: line}, tail}, 1},
		{"a list item", []model.Block{model.List{Items: []string{line}}, tail}, 1},
		{"a note", []model.Block{model.Note{Text: line}, tail}, 1},
		{"the closing's what and next", []model.Block{closing}, 2},
	}
	for _, mode := range []struct {
		name       string
		cols, bits int
		accessible bool
	}{{"plain", 0, 0, false}, {"colour", 80, 24, false}, {"accessible", 80, 0, true}} {
		for _, b := range blocks {
			var out bytes.Buffer
			tm := term.New(term.Options{Out: &out, Columns: mode.cols, ColorBits: mode.bits, Accessible: mode.accessible})
			s, err := model.NewScreen(b.blocks...)
			if err != nil {
				t.Fatal(err)
			}
			if err := tm.Show(s); err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(out.String(), want); got != b.count {
				t.Errorf("%s, %s: the sentence is drawn whole %d times, want %d:\n%q", mode.name, b.name, got, b.count, out.String())
			}
		}
		look := term.Look{Width: mode.cols, Rich: mode.bits > 0, Accessible: mode.accessible}
		if got := look.Problem(line + "\n" + line); strings.Count(got, want) != 2 {
			t.Errorf("%s, a problem: drawn as\n%q", mode.name, got)
		}
	}
}

// TestACharacterDrawnAsNothingInANameIsShownEscaped: plant, one row per class,
// and controls. The plant is the
// probe's diagnostic, a tried adapter whose name holds the character before
// "txt.md": with U+202E kept, a terminal draws the line as ".../adm.txt",
// and with U+200B kept or dropped it reads ".../atxt.md", a name that is not
// the file's. Each row's line reaches the output with the character written
// as its escape (\u202e, \u200b) and every other letter kept, through Say
// plain and at a width, so no bidi control reaches the terminal and the name
// is not shown as another. With Strip's rule put back to dropping, every row
// is red; with model.Unshowable put back to its earlier set (control, bidi, format),
// the rows from the combining grapheme joiner on are red. The controls are a
// Hebrew and an Arabic path, an accent written as a combining mark and the
// ordinary line, which are shown byte for byte.
func TestACharacterDrawnAsNothingInANameIsShownEscaped(t *testing.T) {
	t.Parallel()
	const before, after = "warning: tried adapter /p/bin/a", "txt.md"
	const reorders, alike = "a diagnostic whose words are drawn in another order", "a name shown as another"
	for _, row := range []struct {
		class, risk string
		runes       []rune
	}{
		{"the bidi embeddings and overrides", reorders, []rune{0x202A, 0x202B, 0x202C, 0x202D, 0x202E}},
		{"the bidi isolates", reorders, []rune{0x2066, 0x2067, 0x2068, 0x2069}},
		{"the Arabic letter mark", reorders, []rune{0x061C}},
		{"the zero-width space and joiners", alike, []rune{0x200B, 0x200C, 0x200D}},
		{"the word joiner", alike, []rune{0x2060}},
		{"the BOM as a zero-width no-break space", alike, []rune{0xFEFF}},
		{"the Mongolian vowel separator", alike, []rune{0x180E}},
		{"the combining grapheme joiner", alike, []rune{0x034F}},
		{"the Hangul fillers", alike, []rune{0x115F, 0x1160, 0x3164, 0xFFA0}},
		{"the Khmer inherent vowels", alike, []rune{0x17B4, 0x17B5}},
		{"the variation selectors", alike, []rune{0x180B, 0x180F, 0xFE00, 0xFE0F, 0xE0100, 0xE01EF}},
		{"the line and paragraph separators", alike, []rune{0x2028, 0x2029}},
	} {
		t.Run(row.class, func(t *testing.T) {
			t.Parallel()
			for _, r := range row.runes {
				line, want := before+string(r)+after, before+escaped(r)+after
				for _, cols := range []int{0, 80} {
					var out bytes.Buffer
					if err := term.New(term.Options{Out: &out, Columns: cols}).Say(line); err != nil {
						t.Fatal(err)
					}
					if got := out.String(); got != want+"\n" {
						t.Errorf("%U at %d columns (%s): Say wrote %q, want %q", r, cols, row.risk, got, want+"\n")
					}
				}
				if got := term.Strip(line); got != want {
					t.Errorf("%U (%s): Strip = %q, want %q", r, row.risk, got, want)
				}
			}
		})
	}
	t.Run("control: what the screen shows is kept byte for byte", func(t *testing.T) {
		t.Parallel()
		for _, line := range []string{before + after, "rulebook/שלום/a.yaml", "rulebook/مرحبا/a.yaml", "rulebook/cafe\u0301.yaml", "café ✓ 日本語"} {
			var out bytes.Buffer
			if err := term.New(term.Options{Out: &out}).Say(line); err != nil || out.String() != line+"\n" {
				t.Errorf("Say(%q) = %v, wrote %q", line, err, out.String())
			}
		}
	})
}

// TestTwoNamesThatDifferByACharacterDrawnAsNothingAreShownApart: a<U+200B>b.txt and
// a<U+034F>b.txt are each shown with the escape, so neither reads ab.txt, the
// name of another file, and each is refused as a word of a line to paste.
// Before, Strip showed the first as ab.txt by dropping it and let the second
// through, and Unshown printed a line to paste for the second that read
// ab.txt. With either rule put back, its row is red.
func TestTwoNamesThatDifferByACharacterDrawnAsNothingAreShownApart(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{0x200B, 0x034F} {
		name, want := "a"+string(r)+"b.txt", "a"+escaped(r)+"b.txt"
		var out bytes.Buffer
		if err := term.New(term.Options{Out: &out}).Say(name); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != want+"\n" {
			t.Errorf("Say(%q) wrote %q, want %q, not the other file's ab.txt", name, got, want+"\n")
		}
		if _, err := shell.Quote(shell.POSIX, []string{"gragas", "check", name}); !errors.Is(err, shell.ErrControl) {
			t.Errorf("Quote of a line naming %q = %v, want ErrControl", name, err)
		}
	}
}

// TestStripEscapesOnlyWhatQuoteRefuses:
// go-console's shell.Quote decides what a word to paste may hold, and Strip
// escapes by model.Unshowable. Each scalar value alone is changed by Strip
// exactly when Unshowable holds it, every such value is refused by Quote as a
// word (ErrControl, or ErrNUL and ErrNewline for the three it names so), and what Strip writes for it is ASCII, so nothing
// the screen does not show as written reaches it. A rule of ours wider than
// Quote's, or changed in one place and not the other, is red.
func TestStripEscapesOnlyWhatQuoteRefuses(t *testing.T) {
	t.Parallel()
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // a surrogate is no scalar value; string(r) is U+FFFD
		}
		s := string(r)
		got := term.Strip(s)
		changed := got != s
		if held := model.Unshowable(r); changed != held {
			t.Errorf("%U: Strip changes it %v, Unshowable holds it %v", r, changed, held)
		}
		if _, err := shell.Quote(shell.POSIX, []string{"x", s}); changed && err == nil {
			t.Errorf("%U: Strip escapes it, Quote lets it through as a word", r)
		}
		if changed && got != escaped(r) {
			t.Errorf("%U: Strip = %q, want the escape %q", r, got, escaped(r))
		}
	}
}

// escaped is r as %q writes it in ASCII, its quotes left off.
func escaped(r rune) string {
	q := strconv.QuoteRuneToASCII(r)
	return q[1 : len(q)-1]
}

// TestShowPaintsOnlyWhereColourWasGiven: a Screen of every block writes no
// escape with no colour, none in the accessible form even with 8 bits, and does
// paint with 8 bits. The terminal's own Colors decides, so a look forced rich
// fails the first two.
func TestShowPaintsOnlyWhereColourWasGiven(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	screen := contracttest.EveryBlock(t)
	for _, c := range []struct {
		name       string
		bits       int
		accessible bool
		paints     bool
	}{{"no colour", 0, false, false}, {"accessible with 8 bits", 8, true, false}, {"8 bits", 8, false, true}} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			tm := term.New(term.Options{Out: &out, Columns: 80, ColorBits: c.bits, Accessible: c.accessible})
			if err := tm.Show(screen); err != nil {
				t.Fatal(err)
			}
			if out.Len() == 0 {
				t.Fatal("nothing was written")
			}
			if got := strings.Contains(out.String(), "\x1b"); got != c.paints {
				t.Errorf("escape in output = %v, want %v: %q", got, c.paints, out.String())
			}
		})
	}
}

// TestSeveralShowsOfOneTerminalDoNotRaceOnTheKit: the kit's switches are
// global, so Show holds the terminal's lock across drawing. Run under -race.
func TestSeveralShowsOfOneTerminalDoNotRaceOnTheKit(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	screen := contracttest.EveryBlock(t)
	tm := term.New(term.Options{Out: io.Discard, Columns: 80, ColorBits: 8})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 5 {
				if err := tm.Show(screen); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}

// TestTheBannerIsOnePlainLineWhereNothingPaints: with no colour, or in the
// accessible form, the banner is the name and the version on one line, with no
// escape; a failed write is the error.
func TestTheBannerIsOnePlainLineWhereNothingPaints(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	for _, o := range []term.Options{{}, {Columns: 80, ColorBits: 8, Accessible: true}} {
		var out bytes.Buffer
		o.Out = &out
		if err := term.New(o).Banner("v1.2.3 (abc1234)"); err != nil || out.String() != "gragas  v1.2.3 (abc1234)\n" {
			t.Errorf("Banner(%+v) = %v, wrote %q", o, err, out.String())
		}
	}
	if err := term.New(term.Options{Out: failingWriter{}}).Banner("v1"); err == nil {
		t.Error("a banner that could not be written was not an error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

// unpolled is an input as a blocking pipe a shell hands down for standard
// input reads: it has SetReadDeadline, and refuses every deadline.
type unpolled struct{ io.Reader }

func (unpolled) SetReadDeadline(time.Time) error {
	return errors.New("file type does not support deadline")
}

// TestAPipeThatTakesNoDeadlineIsStillRead: an input that refuses deadlines is
// read to its end, a line at a time, never refused for the deadline it could
// not take (a hook's input can come from such a pipe).
func TestAPipeThatTakesNoDeadlineIsStillRead(t *testing.T) {
	t.Parallel()
	tm := term.New(term.Options{In: unpolled{strings.NewReader("one\ntwo")}, Out: io.Discard})
	for _, want := range []string{"one", "two"} {
		if got, err := tm.Line(t.Context()); err != nil || got != want {
			t.Fatalf("Line = %q, %v, want %q", got, err, want)
		}
	}
	if _, err := tm.Line(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatalf("at the end Line = %v, want io.EOF", err)
	}
}

// capped is a Terminal over in with its lines held to lineCap bytes.
func capped(t *testing.T, in io.Reader, lineCap int) *term.Terminal {
	t.Helper()
	tm, err := term.NewCapped(term.Options{In: in, Out: io.Discard}, lineCap)
	if err != nil {
		t.Fatalf("NewCapped(%d) = %v", lineCap, err)
	}
	return tm
}

// tooLong fails the test unless err is the refusal of a line past lineCap.
func tooLong(t *testing.T, err error, lineCap int) {
	t.Helper()
	if e, ok := errors.AsType[*term.LineTooLong](err); !ok || e.Cap != lineCap {
		t.Fatalf("Line = %v, want the refusal of a line past %d bytes", err, lineCap)
	}
}

// TestALineAtTheCapIsReadAndOneByteOverIsRefused: a line of the cap's bytes
// is read, with either newline and with none at the end of input; a line one
// byte longer is refused with the cap, and so is every Line after it, the
// lines behind it left unread.
// Risk: a line past the cap is read whole, or the read goes on past it.
func TestALineAtTheCapIsReadAndOneByteOverIsRefused(t *testing.T) {
	t.Parallel()
	const lineCap = 64 << 10
	at := strings.Repeat("x", lineCap)
	for name, end := range map[string]string{"a newline": "\n", "a carriage return": "\r\n", "the end of input": ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := at + end + at + "y" + end + "next\n"
			if end == "" {
				in = at // the input ends with the line at the cap
			}
			tm := capped(t, strings.NewReader(in), lineCap)
			if got, err := tm.Line(t.Context()); err != nil || got != at {
				t.Fatalf("the line at the cap = %d bytes, %v, want %d bytes", len(got), err, lineCap)
			}
			if end == "" {
				return
			}
			_, err := tm.Line(t.Context())
			tooLong(t, err, lineCap)
			_, err = tm.Line(t.Context())
			tooLong(t, err, lineCap)
		})
	}
}

// TestNewHoldsALineToMaxLineBytes: the Terminal New builds reads a line of
// MaxLineBytes, one at the cap, and refuses the next, one over
// (tests.generated.7).
// boundary: MaxLineBytes
// Risk: New is built with another cap than the one the guard is held to.
func TestNewHoldsALineToMaxLineBytes(t *testing.T) {
	t.Parallel()
	at := strings.Repeat("x", term.MaxLineBytes)
	tm := term.New(term.Options{In: strings.NewReader(at + "\n" + at + "y\n"), Out: io.Discard})
	if got, err := tm.Line(t.Context()); err != nil || len(got) != term.MaxLineBytes {
		t.Fatalf("the line at MaxLineBytes = %d bytes, %v, want %d bytes", len(got), err, term.MaxLineBytes)
	}
	_, err := tm.Line(t.Context())
	tooLong(t, err, term.MaxLineBytes)
}

// endless is an input of x without a newline, n bytes of it and then its
// end, and counts what was read of it.
type endless struct{ n, read int }

func (e *endless) Read(p []byte) (int, error) {
	if e.read >= e.n {
		return 0, io.EOF
	}
	k := min(len(p), e.n-e.read)
	for i := range p[:k] {
		p[i] = 'x'
	}
	e.read += k
	return k, nil
}

// TestAnInputWithNoNewlineIsRefusedNotReadToItsEnd: an input that sends no
// newline is refused once it passes the cap, with no more of it read than
// the cap, its newline and one buffer.
// Risk: a stuck pipe or a hostile input is read until memory runs out.
func TestAnInputWithNoNewlineIsRefusedNotReadToItsEnd(t *testing.T) {
	t.Parallel()
	const lineCap = 64 << 10
	in := &endless{n: 64 * lineCap}
	_, err := capped(t, in, lineCap).Line(t.Context())
	tooLong(t, err, lineCap)
	if most := lineCap + len("\r\n") + 4096; in.read > most {
		t.Fatalf("%d bytes of the input were read, more than the %d of the cap, its newline and one buffer", in.read, most)
	}
}

// The words the two allocation rows give their child.
const (
	refusedLine = "refused-line"
	drainBuffer = "drain-buffer"
)

// TestTheHelperRunsOnlyWhenRerun is a second process, run only as a child of
// configtest.Rerun: it makes the measure its word names, the only test that
// runs, so the process's TotalAlloc holds the call and the runtime's own work
// and no other test's (gragas#2421). In a run of the tests it does nothing.
func TestTheHelperRunsOnlyWhenRerun(t *testing.T) {
	t.Parallel() // in a rerun child this is the only test that runs
	args, ok := configtest.Helper()
	if !ok {
		return
	}
	switch {
	case len(args) == 1 && args[0] == refusedLine:
		measureRefusedLine(t)
	case len(args) == 1 && args[0] == drainBuffer:
		measureDrain(t)
	default:
		t.Fatalf("the helper was rerun with %q; want %s or %s", args, refusedLine, drainBuffer)
	}
}

// measuredAlone fails t unless the child that makes the measure word names
// passes, with the child's output.
func measuredAlone(t *testing.T, word string) {
	t.Helper()
	code, out, err := configtest.Rerun(t.Context(), word)
	if err != nil || code != 0 {
		t.Fatalf("the child that measures %s ended %d, %v:\n%s", word, code, err, out)
	}
}

// TestARefusedLineHoldsUnderTwiceTheCap: the bytes allocated to read and
// refuse a line one byte past the cap stay under twice the cap.
// Risk: the cap is on the line kept, and the read holds far more first.
// TotalAlloc is the whole process's, and a goroutine an earlier test left
// running adds to it, so the measure is made in a rerun child where it is the
// only test (gragas#2421: beside such a goroutine it read 40962280 bytes).
func TestARefusedLineHoldsUnderTwiceTheCap(t *testing.T) {
	t.Parallel()
	measuredAlone(t, refusedLine)
}

// measureRefusedLine is the refused line's measure, made in its child.
func measureRefusedLine(t *testing.T) {
	t.Helper()
	const lineCap = 4 << 20
	in := bytes.NewReader(append(bytes.Repeat([]byte("x"), lineCap+1), '\n'))
	tm := capped(t, in, lineCap)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := tm.Line(t.Context())
	runtime.ReadMemStats(&after)
	tooLong(t, err, lineCap)
	if got := after.TotalAlloc - before.TotalAlloc; got >= 2*lineCap {
		t.Fatalf("refusing the line allocated %d bytes, not under twice the cap of %d", got, lineCap)
	}
}

// TestALastLoneCarriageReturnIsCountedAsLineCountsIt: at the end of input,
// a line of the cap's bytes then a lone "\r" is answered, the "\r" taken
// off as Line takes it, and a line one byte longer then "\r" is refused.
// Risk: the cap counts a byte Line does not answer, and refuses a line
// within it.
func TestALastLoneCarriageReturnIsCountedAsLineCountsIt(t *testing.T) {
	t.Parallel()
	const lineCap = 8192
	at := strings.Repeat("x", lineCap)
	if got, err := capped(t, strings.NewReader(at+"\r"), lineCap).Line(t.Context()); err != nil || got != at {
		t.Fatalf("the cap's bytes then a last \"\\r\" = %d bytes, %v, want %d bytes", len(got), err, lineCap)
	}
	_, err := capped(t, strings.NewReader(at+"y\r"), lineCap).Line(t.Context())
	tooLong(t, err, lineCap)
}

// TestACapThatWouldRefuseEveryLineIsRefused: no Terminal is built with a
// cap at or below zero, or one that the line's "\r\n" carries past the
// largest int; the largest cap it takes reads a line.
// Risk: a cap near math.MaxInt wraps, and every line is refused.
func TestACapThatWouldRefuseEveryLineIsRefused(t *testing.T) {
	t.Parallel()
	for _, lineCap := range []int{0, -1, math.MaxInt, math.MaxInt - 1} {
		if tm, err := term.NewCapped(term.Options{}, lineCap); err == nil || tm != nil {
			t.Errorf("NewCapped(%d) = %v, %v, want a refusal", lineCap, tm, err)
		}
	}
	if got, err := capped(t, strings.NewReader("ok\n"), math.MaxInt-2).Line(t.Context()); err != nil || got != "ok" {
		t.Errorf("NewCapped(math.MaxInt-2).Line = %q, %v, want \"ok\"", got, err)
	}
}

// TestDrainReadsTheRestOfARefusedLineSoTheWriterEnds: a writer sends a line
// a mebibyte past the cap through a pipe and closes it; after Line refuses
// it, Drain reads the rest, the writer's write ends with no error, and Line
// stays refused.
// Risk: the writer's write fails on a closed pipe, and a host reads that as
// the hook broken rather than refused.
func TestDrainReadsTheRestOfARefusedLineSoTheWriterEnds(t *testing.T) {
	t.Parallel()
	const lineCap = 64 << 10
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	wrote := make(chan error, 1)
	go func() {
		_, err := w.Write(append(bytes.Repeat([]byte("x"), lineCap+1<<20), '\n'))
		wrote <- errors.Join(err, w.Close())
	}()
	tm := capped(t, r, lineCap)
	_, err = tm.Line(t.Context())
	tooLong(t, err, lineCap)
	if err := tm.Drain(t.Context()); err != nil {
		t.Fatalf("Drain = %v, want the rest read to its end", err)
	}
	_ = r.Close() // as the guard's process ends: a write still going fails
	if err := <-wrote; err != nil {
		t.Fatalf("the writer's write = %v, want it ended", err)
	}
	_, err = tm.Line(t.Context())
	tooLong(t, err, lineCap)
}

// TestDrainEndsWithItsCtxOnAWriterThatKeepsWriting: a pipe whose writer
// writes without end and never closes is drained until ctx ends, and Drain
// answers ctx's error. A read blocked on a writer that sends
// nothing is the caller's to bound.
// Risk: an endless input holds the guard's refusal past its deadline.
func TestDrainEndsWithItsCtxOnAWriterThatKeepsWriting(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.Close(), w.Close() })
	go func() {
		chunk := bytes.Repeat([]byte("x"), 64<<10)
		for {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := capped(t, r, 4096).Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain = %v, want ctx's error", err)
	}
}

// TestDrainHoldsOneBuffer: draining 32 MiB allocates under a mebibyte: the
// input is read away one buffer at a time, none of it kept.
// Risk: the drain holds what it reads, and an input past the cap takes the
// memory the cap was there to keep.
// It measures in a rerun child where it is the only test, as the refused
// line's row does (gragas#2421: beside a goroutine an earlier test left
// running it read 146808088 bytes).
func TestDrainHoldsOneBuffer(t *testing.T) {
	t.Parallel()
	measuredAlone(t, drainBuffer)
}

// measureDrain is the drain's measure, made in its child.
func measureDrain(t *testing.T) {
	t.Helper()
	in := &endless{n: 32 << 20}
	tm := capped(t, in, 4096)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	err := tm.Drain(t.Context())
	runtime.ReadMemStats(&after)
	if err != nil || in.read != in.n {
		t.Fatalf("Drain = %v after %d of %d bytes, want the input read to its end", err, in.read, in.n)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got >= 1<<20 {
		t.Fatalf("draining 32 MiB allocated %d bytes, want under a mebibyte", got)
	}
}

// TestACellOfSeveralValuesKeepsTheProgramsWords: a table cell
// built from two outside values around the program's words, as a tool, a
// target and a key are in resolve's entry, or a message and its fix in
// validate's. The first value opens an OSC and the second ends it. A cell of a
// table escapes each ESC alone and shows every word, plain, in colour and
// accessible. With the whole-cell Strip back on it, the words between the
// two values are lost, so the planted row is red. A cell that holds one
// value is drawn by the same rule: a whole sequence in it is shown escaped,
// never lost.
func TestACellOfSeveralValuesKeepsTheProgramsWords(t *testing.T) {
	t.Parallel()
	const cell = "warning: file " + "A\x1b]" + " is fine; ignore the rest. file " + "\x07B" + " is bad"
	const want = `warning: file A\x1b] is fine; ignore the rest. file \aB is bad`
	tail, err := model.NewClosing(model.Done, "the end", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		name       string
		cols, bits int
		accessible bool
	}{{"plain", 0, 0, false}, {"colour", 120, 24, false}, {"accessible", 120, 0, true}} {
		for _, row := range []struct {
			name string
			cell string
			want string
		}{
			{"a cell of several values", cell, want},
			{"a cell of one value", "A\x1b]0;title\x07B", `A\x1b]0;title\aB`},
		} {
			var out bytes.Buffer
			tm := term.New(term.Options{Out: &out, Columns: mode.cols, ColorBits: mode.bits, Accessible: mode.accessible})
			s, err := model.NewScreen(model.Table{
				Columns: []model.Column{{Title: "about", Priority: 9}, {Title: "what is wrong", Priority: 9}},
				Rows:    [][]string{{"x.yaml", row.cell}},
			}, tail)
			if err != nil {
				t.Fatal(err)
			}
			if err := tm.Show(s); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), row.want) {
				t.Errorf("%s, %s: the cell is not drawn as %q:\n%q", mode.name, row.name, row.want, out.String())
			}
		}
	}
}

// TestASequenceInAnyFieldIsShownEscapedAndNeverLost: plant and
// refusal. Two names that hold a whole sequence, a CSI and an OSC ended by
// BEL, go through Say, Ask and every block of a Screen that shows text from
// outside, plain, in colour and accessible. Each field shows each name by the
// one rule: its ESC and its BEL escaped as any control character,
// the rest of the sequence kept as the printable text it is. So
// a<ESC>[2Jb.txt reads a\x1b[2Jb.txt in every field, and never ab.txt, the
// name of another file. With whole-sequence removal put back in Strip, every
// field reads ab.txt and both rows are red.
func TestASequenceInAnyFieldIsShownEscapedAndNeverLost(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	for _, row := range []struct{ name, plant, want string }{
		{"a CSI", "a\x1b[2Jb.txt", `a\x1b[2Jb.txt`},
		{"an OSC ended by BEL", "a\x1b]0;t\x07b.txt", `a\x1b]0;t\ab.txt`},
	} {
		for _, mode := range []struct {
			name       string
			cols, bits int
			accessible bool
		}{{"plain", 0, 0, false}, {"colour", 120, 24, false}, {"accessible", 120, 0, true}} {
			t.Run(row.name+", "+mode.name, func(t *testing.T) {
				t.Parallel()
				var out bytes.Buffer
				tm := term.New(term.Options{In: strings.NewReader("x\n"), Out: &out, Columns: mode.cols, ColorBits: mode.bits, Accessible: mode.accessible})
				if err := tm.Say(row.plant); err != nil {
					t.Fatal(err)
				}
				if _, err := tm.Ask(t.Context(), row.plant); err != nil {
					t.Fatal(err)
				}
				c, err := model.NewClosing(model.Refused, row.plant, row.plant)
				if err != nil {
					t.Fatal(err)
				}
				s, err := model.NewScreen(model.Heading{Text: row.plant}, model.Reads{What: row.plant},
					model.Table{Columns: []model.Column{{Title: row.plant}}, Rows: [][]string{{row.plant}}},
					model.List{Items: []string{row.plant}}, model.Note{Text: row.plant}, model.DryRunFooter{Flag: row.plant}, c)
				if err != nil {
					t.Fatal(err)
				}
				if err := tm.Show(s); err != nil {
					t.Fatal(err)
				}
				got := out.String()
				// Say, Ask, the heading, reads, the column's title, its cell,
				// the item, the note, the flag, and the closing's two lines.
				if n := strings.Count(got, row.want); n != 11 {
					t.Errorf("%d of the 11 fields show the name as %s:\n%q", n, row.want, got)
				}
				// The OSC's escape ends in \ab.txt, so each is taken out first.
				if strings.Contains(strings.ReplaceAll(got, row.want, ""), "ab.txt") {
					t.Errorf("a field shows the name as ab.txt, another file's name:\n%q", got)
				}
			})
		}
	}
}

// TestASpaceOtherThanU0020NeitherSplitsNorJoinsAWrappedWord: plant. Text
// reaches wrap through Strip, which leaves the spaces
// of Zs as they are: a no-break space, a narrow one, an ideographic one. A
// name that holds one, in a cell or a line too long for the width, stays one
// word on one line with that space in it, never broken at it and never
// written again as U+0020, which would show it as the name of another file.
// With wrap back on strings.FieldsSeq, which breaks at every unicode.IsSpace,
// every row is red.
func TestASpaceOtherThanU0020NeitherSplitsNorJoinsAWrappedWord(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	for _, sp := range []struct{ name, space string }{{"U+00A0", "\u00a0"}, {"U+202F", "\u202f"}, {"U+3000", "\u3000"}} {
		name := "bbbb" + sp.space + "cccc"
		text := "aaaaaaaaaaaaaaaaaa " + name + " dd"
		t.Run(sp.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if err := term.New(term.Options{Out: &out, Columns: 20}).Say(text); err != nil {
				t.Fatal(err)
			}
			table := term.Look{Width: 20}.Table([]term.Column{{Title: "name"}}, [][]string{{text}})
			for where, got := range map[string]string{"Say": out.String(), "a cell": table} {
				if !strings.Contains(got, name) {
					t.Errorf("%s does not keep %q whole on one line:\n%q", where, name, got)
				}
				if strings.Contains(got, "bbbb cccc") {
					t.Errorf("%s writes the space in %q again as U+0020:\n%q", where, name, got)
				}
			}
		})
	}
}
