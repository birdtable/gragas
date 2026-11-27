package term

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/go-console/ui"

	"github.com/birdtable/gragas/internal/model"
)

// Look is one way of drawing: the width a table fits, whether colour is
// painted, and whether the banner is left out for a reader that does not see
// it. Its methods answer text for Say and never write.
//
// The look is the shared kit's: its roles, its helpers, its banner. Nothing is
// copied from it (arch.shared.7). Where the kit has no draw for a thing, a
// local one is built from the kit's helpers and named here.
type Look struct {
	// Width is the columns drawn to: the terminal's, else COLUMNS, never above
	// 96 (cli.environment.2). 0 is unknown, and nothing is wrapped.
	Width int
	// Rich paints colour. NO_COLOR, a pipe, TERM=dumb and --no-color are not
	// Rich.
	Rich bool
	// Accessible leaves out the banner, which is decoration, and never paints.
	Accessible bool
}

// draw runs f over a session of the kit of its own, writing into a buffer,
// painted or plain as l says, and answers what was written. A session is
// one draw, so two draws share nothing and may run at once.
func (l Look) draw(f func(s *ui.Session)) string {
	var b bytes.Buffer
	s := l.session(&b, false)
	s.Capture(&b, func() { f(s) })
	return b.String()
}

// session is the kit's session over w as l draws: colour only where l
// paints, l's width, at most 96 (cli.environment.2), and the fixed palette
// on a dark ground, as the kit drew before it could be told the ground. The
// terminal draws UTF-8, as the look always has: no locale is read.
// shown is true only for the banner, which the kit draws on a terminal alone.
func (l Look) session(w io.Writer, shown bool) *ui.Session {
	rich := l.Rich && !l.Accessible
	return ui.New(ui.Out(w), ui.Err(w), ui.MaxWidth(maxLine), ui.WithTheme(ui.Contrast()),
		ui.On(ui.Terminal{Colour: rich, ErrColour: rich, Width: widthOf(l), Interactive: shown, Accessible: l.Accessible, Dark: true, Unicode: true}))
}

// wrapSaid is a line Say writes, broken by the kit's Wrap at the spaces so
// that none is wider than l's width in cells: each piece at the line's own
// indent, the spaces inside a piece kept, a word wider than the room left
// whole. A line that fits, and a width of 0, are as they are.
func (l Look) wrapSaid(line string) string {
	body := strings.TrimLeft(line, " ")
	indent := len(line) - len(body)
	return line[:indent] + l.session(io.Discard, false).Wrap(indent, body)
}

// gragas is the identity the banner and the help draw: the name and its row,
// with no picture beside them.
func gragas() ui.Brand {
	return ui.Brand{
		Name: "gragas",
		Row:  "the rulebook, as presets you import",
	}
}

// Banner is the wordmark and the version, or one plain heading line where
// there is no room, no colour or no one to see it.
func (l Look) Banner(version string) string {
	if l.Accessible || !l.Rich {
		return "gragas  " + version + "\n"
	}
	var b bytes.Buffer
	s := l.session(&b, true)
	if s.Capture(&b, func() { s.Arrive(gragas(), version) }); b.Len() > 0 {
		return b.String()
	}
	return "gragas  " + version + "\n"
}

// Column is one column of a Table.
type Column struct {
	Title string
	// Priority orders what goes when the width is short: the lowest goes
	// first, the rightmost of equals first. The last one standing is never
	// dropped; it wraps.
	Priority int
}

// Table is cols and rows as aligned text, two spaces between columns, with no
// rule or box in any mode (cli.look.7). Every cell is a name from outside, so
// its control characters are shown, not sent. A cell too long for its column
// wraps onto lines under it. When the width is short the column with the
// lowest priority is dropped, and the last line says which were not shown.
func (l Look) Table(cols []Column, rows [][]string) string {
	if len(cols) == 0 {
		return ""
	}
	var out string
	l.draw(func(s *ui.Session) {
		out = table(s, cols, rows, widthOf(l))
	})
	return out
}

// widthOf is the width the look draws to: its own, at most 96.
func widthOf(l Look) int { return min(max(l.Width, 0), maxLine) }

const (
	gap = 2
	// minLast is the least a last column is given before another column goes.
	minLast = 16
)

// grid is a table being fitted: the columns and rows, and which columns stay.
type grid struct {
	cols []Column
	rows [][]string
	keep []int
}

func (g *grid) cell(r []string, i int) string {
	if i < len(r) {
		return Strip(r[i])
	}
	return ""
}

func (g *grid) natural(i int) int {
	n := utf8.RuneCountInString(Strip(g.cols[i].Title))
	for _, r := range g.rows {
		n = max(n, utf8.RuneCountInString(g.cell(r, i)))
	}
	return n
}

// total is the width the kept columns need; the last column wraps, so it
// needs only room to wrap in.
func (g *grid) total() int {
	n := gap * (len(g.keep) - 1)
	for k, i := range g.keep {
		if k == len(g.keep)-1 {
			n += min(g.natural(i), minLast)
			continue
		}
		n += g.natural(i)
	}
	return n
}

// fit drops the lowest-priority column, the later one on a tie, until the rest
// fit width, and answers the titles dropped, last dropped first.
func (g *grid) fit(width int) []string {
	var dropped []string
	for width > 0 && len(g.keep) > 1 && g.total() > width {
		drop := g.keep[0]
		for _, i := range g.keep {
			if g.cols[i].Priority <= g.cols[drop].Priority {
				drop = i
			}
		}
		dropped = append(dropped, Strip(g.cols[drop].Title))
		g.keep = slices.DeleteFunc(g.keep, func(i int) bool { return i == drop })
	}
	slices.Reverse(dropped)
	return dropped
}

// widths are the kept columns' widths; the last takes what is left, wrapped,
// so a line is never wider than the screen.
func (g *grid) widths(width int) []int {
	widths := make([]int, len(g.keep))
	for k, i := range g.keep {
		widths[k] = g.natural(i)
	}
	if width > 0 {
		left := width - gap*(len(g.keep)-1)
		for _, w := range widths[:len(widths)-1] {
			left -= w
		}
		if last := len(widths) - 1; left < widths[last] {
			widths[last] = max(left, 1)
		}
	}
	return widths
}

func table(s *ui.Session, cols []Column, rows [][]string, width int) string {
	g := &grid{cols: cols, rows: rows, keep: make([]int, len(cols))}
	for i := range g.keep {
		g.keep[i] = i
	}
	dropped := g.fit(width)
	widths := g.widths(width)
	var b strings.Builder
	heads := make([]string, len(g.keep))
	for k, i := range g.keep {
		heads[k] = Strip(cols[i].Title)
	}
	b.WriteString(rowText(s, heads, widths, true))
	for _, r := range rows {
		texts := make([]string, len(g.keep))
		for k, i := range g.keep {
			texts[k] = g.cell(r, i)
		}
		b.WriteString(rowText(s, texts, widths, false))
	}
	if len(dropped) > 0 {
		b.WriteString(s.Note(fmt.Sprintf("%s not shown: %s", model.Plural(len(dropped), "column"), strings.Join(dropped, ", "))) + "\n")
	}
	return b.String()
}

// rowText is one row, each cell wrapped to its width, as lines.
func rowText(s *ui.Session, texts []string, widths []int, head bool) string {
	wrapped := make([][]string, len(texts))
	height := 1
	for k, t := range texts {
		wrapped[k] = wrap(t, widths[k])
		height = max(height, len(wrapped[k]))
	}
	var b strings.Builder
	for h := range height {
		var parts []string
		for k := range texts {
			cell := ""
			if h < len(wrapped[k]) {
				cell = wrapped[k][h]
			}
			if head && cell != "" {
				cell = s.Bold(cell)
			}
			if k < len(texts)-1 {
				cell = ui.Column(cell, widths[k]+gap) // the kit pads by display cells, so a bold head lines up with its rows
			}
			parts = append(parts, cell)
		}
		b.WriteString(strings.TrimRight(strings.Join(parts, ""), " ") + "\n")
	}
	return b.String()
}

// wrap is text broken at its U+0020 spaces into lines of at most n runes; a
// word longer than n is cut. Empty text is one empty line. Only U+0020
// breaks: text comes here through Strip, which escapes every control
// character, so the spaces left are U+0020 and the spaces of Zs, a no-break
// space U+00A0 or an ideographic space U+3000 among them. Those stay inside
// their word, so one never splits a name across two lines and is never
// written again as U+0020.
func wrap(text string, n int) []string {
	if n <= 0 || utf8.RuneCountInString(text) <= n {
		return []string{text}
	}
	var out []string
	cur := ""
	flush := func() {
		if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	for w := range strings.SplitSeq(text, " ") {
		if w == "" {
			continue
		}
		for utf8.RuneCountInString(w) > n {
			flush()
			r := []rune(w)
			out = append(out, string(r[:n]))
			w = string(r[n:])
		}
		switch {
		case cur == "":
			cur = w
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) <= n:
			cur += " " + w
		default:
			flush()
			cur = w
		}
	}
	flush()
	return out
}

// Ending is how a run ends as the look draws it: the word, the sentence
// beside it, and the line to type when the word has one.
type Ending struct {
	Word model.Word
	What string
	Next string
}

// Closing is e as the last lines of a run: the word in its colour, the
// sentence beside it, the line to type under it (cli.look.5).
//
// The kit's own ending takes gragas's eight (ui.Endings), but it prints every
// word but done on stderr and names the command's help as the
// line to type on a refusal that gives none, so each of the eight is drawn here
// from the kit's helpers, under the waiver of cli.look.5 in
// gate/exemptions.yaml: a gap, the word in a gutter eleven wide with the
// sentence beside it, and the line to type indented under. Done is good;
// findings, refused and failed are bad; the four that wait on somebody or
// something are amber. A word outside the eight is drawn as failed: the closed
// set stays closed.
func (l Look) Closing(e Ending) string {
	if l.Accessible {
		return l.draw(func(s *ui.Session) { accessibleEnding(s, e) })
	}
	return l.draw(func(s *ui.Session) {
		if !s.EndsBlank() {
			s.Log("")
		}
		w := displayWord(e.Word)
		var paint func(string) string
		switch w {
		case model.Done:
			paint = s.Good
		case model.Findings, model.Refused, model.Failed:
			paint = s.Bad
		case model.Usage, model.Busy, model.NotReady, model.Declined:
			paint = s.Attn
		}
		s.Info("%s%s", ui.Column(paint(string(w)), 11), e.What)
		if e.Next != "" {
			s.Log("    %s", s.Action(e.Next))
		}
	})
}

// accessibleEnding draws any of the eight words with its symbol beside it, in
// the same gutter as the others, so a reader who cannot see the colour has the
// symbol and the word. Nothing is painted: the look is plain.
func accessibleEnding(s *ui.Session, e Ending) {
	if !s.EndsBlank() {
		s.Log("")
	}
	s.Info("%s%s", ui.Column(closingSymbol(e.Word)+" "+string(displayWord(e.Word)), 13), Strip(e.What))
	if e.Next != "" {
		s.Log("    %s", Strip(e.Next))
	}
}

// displayWord is the word as drawn: outside the eight it is failed.
func displayWord(w model.Word) model.Word {
	if model.Known(w) {
		return w
	}
	return model.Failed
}

// Problem is a message with the line to type in it, as it reaches stderr: the
// message wrapped to the width, and each line in backticks shown in the action
// colour.
func (l Look) Problem(msg string) string {
	return l.draw(func(s *ui.Session) {
		for line := range strings.SplitSeq(msg, "\n") {
			s.Log("%s", Strip(line))
		}
	})
}
