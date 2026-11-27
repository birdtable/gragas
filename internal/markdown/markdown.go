// Package markdown is gragas's one Markdown owner: it reads a guide into its
// headings with anchors, its paragraphs and its fenced code blocks, each with
// the lines and the byte offsets it stands on, finds the fenced blocks whose
// info string is gragas, and lists the prose lines that carry a MUST.
//
// It reads and interprets nothing inside a block: the YAML in a gragas block
// is the format context's to read, through ordered. The parser is goldmark
// (decision A58); no other package imports it.
//
// # Positions
//
// Render, which writes a rule back into its block, promises that every
// byte outside the block it owns comes back unchanged. It relies on these:
//
//   - Line numbers are 1-based and count every "\n" of the source.
//   - Block.Start is the byte offset of the first byte of the opening fence's
//     line, and Block.End the offset just past the closing fence's line,
//     including its newline when it has one. src[Block.Start:Block.End] is
//     the whole block, fences included, and everything outside it is not the
//     block's to touch.
//   - Block.BodyStart and Block.BodyEnd bound the body: the lines between the
//     fences. For a block that is not closed, BodyEnd and End are len(src).
//   - Only blocks directly in the document are reported. A fence inside a
//     list item or a quotation is not a guide block.
package markdown

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// GragasInfo is the info string of a block the format context owns.
const GragasInfo = "gragas"

// ErrUnterminatedFence is the error of a fenced block that is never closed.
var ErrUnterminatedFence = errors.New("a fenced block is not closed")

// Error is a parse error with the line it stands on (go.errors.8). It matches
// the sentinel it wraps under errors.Is.
type Error struct {
	Err  error
	Line int
}

func (e *Error) Error() string { return fmt.Sprintf("markdown: line %d: %v", e.Line, e.Err) }

// Unwrap returns the sentinel.
func (e *Error) Unwrap() error { return e.Err }

// Heading is one heading, in either of Markdown's two forms.
type Heading struct {
	Level  int
	Text   string // the heading as plain text, markup removed
	Anchor string // the heading's anchor, made unique within the document
	Line   int
}

// Paragraph is a run of plain prose lines.
type Paragraph struct {
	Text    string
	Line    int
	EndLine int
}

// Block is one fenced code block directly in the document.
type Block struct {
	Info      string // the info string, trimmed
	Body      string // the lines between the fences
	Line      int    // the opening fence's line
	BodyLine  int    // the first body line, equal to Line+1
	EndLine   int    // the closing fence's line, or the last line when not closed
	Start     int    // byte offset of the opening fence's line
	BodyStart int
	BodyEnd   int
	End       int
	Closed    bool
	Section   string // the anchor of the heading above it, "" before the first
}

// Must is one prose line that carries the word MUST.
type Must struct {
	Line    int
	Text    string
	Section string
}

// Statement is one prose line that carries a strength word: MUST, SHOULD or
// MAY in capitals, as a guide's own convention writes them. Words are the
// distinct strength words of the line in the order they stand, so a line that
// carries two is one a draft step cannot place and can say so.
type Statement struct {
	Line    int
	Text    string
	Section string
	Words   []string
}

// Doc is a parsed guide, in document order within each list.
type Doc struct {
	Headings   []Heading
	Paragraphs []Paragraph
	Blocks     []Block
	Musts      []Must
	// Statements are the lines that carry MUST, SHOULD or MAY: every Must is
	// one of them.
	Statements []Statement
}

// Find returns the blocks whose info string is exactly info.
func (d *Doc) Find(info string) []Block {
	var out []Block
	for _, b := range d.Blocks {
		if b.Info == info {
			out = append(out, b)
		}
	}
	return out
}

var must = regexp.MustCompile(`\bMUST\b`)

var strength = regexp.MustCompile(`\b(MUST|SHOULD|MAY)\b`)

// Parse reads src. It returns the whole document, and, when a fenced block is
// never closed, also an *Error wrapping ErrUnterminatedFence with the line the
// fence opens on: the block then runs to the end of the source, as Markdown
// reads it. Any other input parses; there is no other error.
func Parse(src []byte) (*Doc, error) {
	s := &source{src: src, text: string(src)}
	s.index()
	root := goldmark.DefaultParser().Parse(text.NewReader(src))
	d := &Doc{}
	seen := map[string]bool{}
	section := ""
	var err error
	claimed := 0
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		switch n := n.(type) {
		case *ast.Heading:
			if h, ok := s.heading(n, seen); ok {
				section = h.Anchor
				d.Headings = append(d.Headings, h)
			}
		case *ast.Paragraph:
			if p, ok := s.paragraph(n); ok {
				d.Paragraphs = append(d.Paragraphs, p)
			}
		case *ast.FencedCodeBlock:
			b := s.block(n, claimed)
			claimed = b.Line
			b.Section = section
			d.Blocks = append(d.Blocks, b)
			if !b.Closed && err == nil {
				err = &Error{Err: ErrUnterminatedFence, Line: b.Line}
			}
		}
	}
	d.Statements = s.statements(root, d)
	for _, st := range d.Statements {
		if must.MatchString(st.Text) {
			d.Musts = append(d.Musts, Must{Line: st.Line, Text: st.Text, Section: st.Section})
		}
	}
	return d, err
}

// source is the text and where its lines begin.
type source struct {
	src    []byte
	text   string
	starts []int // starts[i] is the offset of line i+1
}

func (s *source) index() {
	s.starts = []int{0}
	for i := 0; i < len(s.text); i++ {
		if s.text[i] == '\n' && i+1 < len(s.text) {
			s.starts = append(s.starts, i+1)
		}
	}
}

// lines is the number of lines, a final newline not starting another.
func (s *source) lines() int { return len(s.starts) }

// lineAt is the 1-based line an offset is on.
func (s *source) lineAt(off int) int {
	lo, hi := 0, len(s.starts)
	for lo < hi {
		mid := (lo + hi) / 2
		if s.starts[mid] <= off {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return 1
	}
	return lo
}

// line is the text of line n without its newline, "" past the end.
func (s *source) line(n int) string {
	if n < 1 || n > len(s.starts) {
		return ""
	}
	end := len(s.text)
	if n < len(s.starts) {
		end = s.starts[n] - 1
	}
	return strings.TrimSuffix(s.text[s.starts[n-1]:end], "\r")
}

// offset is where line n begins, len(text) past the end.
func (s *source) offset(n int) int {
	if n < 1 || n > len(s.starts) {
		return len(s.text)
	}
	return s.starts[n-1]
}

// segText is the source a node's lines cover, one line after another.
func (s *source) segText(n ast.Node) (string, int, int, bool) {
	segs := n.Lines()
	if segs.Len() == 0 {
		return "", 0, 0, false
	}
	var parts []string
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		parts = append(parts, strings.TrimRight(s.text[seg.Start:seg.Stop], "\r\n"))
	}
	first := s.lineAt(segs.At(0).Start)
	last := s.lineAt(max(segs.At(segs.Len()-1).Stop-1, segs.At(segs.Len()-1).Start))
	return strings.Join(parts, "\n"), first, last, true
}

func (s *source) paragraph(n *ast.Paragraph) (Paragraph, bool) {
	t, first, last, ok := s.segText(n)
	if !ok {
		return Paragraph{}, false
	}
	return Paragraph{Text: t, Line: first, EndLine: last}, true
}

// heading reads a heading that has text. An empty heading has no anchor and
// no line to stand on, and is left out.
func (s *source) heading(n *ast.Heading, seen map[string]bool) (Heading, bool) {
	_, first, _, ok := s.segText(n)
	if !ok {
		return Heading{}, false
	}
	plain := strings.TrimSpace(s.plain(n))
	return Heading{Level: n.Level, Text: plain, Anchor: unique(Slug(plain), seen), Line: first}, true
}

// plain is the text of a node's inline children, markup removed.
func (s *source) plain(n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(s.src))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		default:
			b.WriteString(s.plain(c))
		}
	}
	return b.String()
}

// Slug is the anchor of a heading's text: lower case, letters, digits,
// hyphens and underscores kept, each space a hyphen, all else dropped.
func Slug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte('-')
		}
	}
	return b.String()
}

// unique makes an anchor unique: a second "intro" is "intro-1", a third
// "intro-2", and never an anchor already taken.
func unique(slug string, used map[string]bool) string {
	cand := slug
	for i := 1; used[cand]; i++ {
		cand = fmt.Sprintf("%s-%d", slug, i)
	}
	used[cand] = true
	return cand
}

var fenceOpen = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")

// block reads a fenced block. after is the line of the previous block's
// opener, the earliest line this one can open on.
func (s *source) block(n *ast.FencedCodeBlock, after int) Block {
	open := s.openLine(n, after)
	fence := fenceOpen.FindStringSubmatch(s.line(open))
	b := Block{Line: open, BodyLine: open + 1, Start: s.offset(open)}
	if n.Info != nil {
		b.Info = strings.TrimSpace(string(n.Info.Segment.Value(s.src)))
	}
	last := open
	if segs := n.Lines(); segs.Len() > 0 {
		last = s.lineAt(max(segs.At(segs.Len()-1).Stop-1, segs.At(segs.Len()-1).Start))
	}
	if fence != nil && last < s.lines() && closes(s.line(last+1), fence[1]) {
		b.Closed = true
		b.EndLine = last + 1
		b.BodyStart, b.BodyEnd, b.End = s.offset(open+1), s.offset(last+1), s.offset(last+2)
	} else {
		b.EndLine = s.lines()
		b.BodyStart, b.BodyEnd, b.End = min(s.offset(open+1), len(s.text)), len(s.text), len(s.text)
	}
	b.Body = s.text[b.BodyStart:b.BodyEnd]
	return b
}

// openLine is the line a fenced block opens on: the line before its first
// body line, or the line its info string is on, or, for a block with neither,
// the first fence after the previous block.
func (s *source) openLine(n *ast.FencedCodeBlock, after int) int {
	if n.Info != nil {
		return s.lineAt(n.Info.Segment.Start)
	}
	if segs := n.Lines(); segs.Len() > 0 {
		return s.lineAt(segs.At(0).Start) - 1
	}
	for l := after + 1; l <= s.lines(); l++ {
		if fenceOpen.MatchString(s.line(l)) {
			return l
		}
	}
	return max(after, 1)
}

// closes is whether a line closes a fence opened with open: the same
// character, at least as long, indented by at most three spaces, and nothing
// after it but spaces.
func closes(line, open string) bool {
	l := strings.TrimLeft(line, " ")
	if len(line)-len(l) > 3 {
		return false
	}
	n := 0
	for n < len(l) && l[n] == open[0] {
		n++
	}
	return n >= len(open) && strings.TrimSpace(l[n:]) == ""
}

// statements lists the prose lines that carry MUST, SHOULD or MAY, anywhere in
// the document and outside every code block, each under the section it stands
// in.
func (s *source) statements(root ast.Node, d *Doc) []Statement {
	var out []Statement
	heads := d.Headings
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph, *ast.TextBlock:
			segs := n.Lines()
			for i := 0; i < segs.Len(); i++ {
				seg := segs.At(i)
				line := strings.TrimSpace(s.text[seg.Start:seg.Stop])
				if words := wordsOf(line); len(words) > 0 {
					ln := s.lineAt(seg.Start)
					out = append(out, Statement{Line: ln, Text: line, Section: sectionAt(heads, ln), Words: words})
				}
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil
	}
	return out
}

// sectionAt is the anchor of the last heading on or before a line.
func sectionAt(heads []Heading, line int) string {
	a := ""
	for _, h := range heads {
		if h.Line > line {
			break
		}
		a = h.Anchor
	}
	return a
}

// wordsOf is the distinct strength words of a line, in the order they stand.
func wordsOf(line string) []string {
	var out []string
	for _, w := range strength.FindAllString(line, -1) {
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}
